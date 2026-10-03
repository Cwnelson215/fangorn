package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"regexp"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
)

// Every tool reads; none writes. Each is a thin wrapper over a ledger method the
// app's own pages use, so the assistant's numbers are the pages' numbers —
// balances, budgets, goal progress and investment values are never recomputed
// here.

// tool is one thing the model can look up.
type tool struct {
	name        string
	description string
	schema      string
	// label is what the app shows while the tool runs.
	label string
	run   func(ctx context.Context, svc *ledger.Service, householdID int, input []byte) (any, error)
}

// maxResultBytes bounds one tool result. A result over it is refused with a
// hint to narrow the query, rather than cut off: a truncated list would be
// summed as if it were complete.
const maxResultBytes = 80 << 10

var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
var monthPattern = regexp.MustCompile(`^\d{4}-\d{2}$`)

// tools is fixed and in a fixed order: the tool list is the front of the cached
// prompt, and any change to it re-bills every conversation's history.
var tools = []tool{
	{
		name: "list_accounts",
		description: "Every account with its current balance, type, class (asset or liability), " +
			"institution and id. Liability balances (credit cards, loans) are negative: the amount owed. " +
			"Investment and retirement balances include holdings at the latest stored prices.",
		schema: `{"type":"object","properties":{
			"include_archived":{"type":"boolean","description":"Also list archived (closed) accounts."}
		},"additionalProperties":false}`,
		label: "Checking accounts",
		run: func(ctx context.Context, svc *ledger.Service, hh int, input []byte) (any, error) {
			var in struct {
				IncludeArchived bool `json:"include_archived"`
			}
			if err := decodeInput(input, &in); err != nil {
				return nil, err
			}
			return svc.ListAccounts(ctx, hh, in.IncludeArchived)
		},
	},
	{
		name:        "list_categories",
		description: "Every income and expense category with its id, kind, and the account it always goes on, if any.",
		schema:      `{"type":"object","properties":{},"additionalProperties":false}`,
		label:       "Checking categories",
		run: func(ctx context.Context, svc *ledger.Service, hh int, input []byte) (any, error) {
			if err := decodeInput(input, &struct{}{}); err != nil {
				return nil, err
			}
			return svc.ListCategories(ctx, hh, false)
		},
	},
	{
		name: "search_transactions",
		description: "Individual transactions, newest first, matching every filter given. " +
			"Amounts are signed relative to the account: positive is money in, negative money out. " +
			"kind is income, expense, refund, transfer, trade or adjustment (a balance dropped when closing an account; " +
			"neither spending nor income). A transfer appears as two rows, one per account. " +
			"Returns at most `limit` rows (default 50, max 200) and says whether more matched; " +
			"to total money over many transactions use spending_breakdown instead of adding these up.",
		schema: `{"type":"object","properties":{
			"from":{"type":"string","description":"First date, YYYY-MM-DD, inclusive."},
			"to":{"type":"string","description":"Last date, YYYY-MM-DD, inclusive."},
			"account_id":{"type":"integer"},
			"category_id":{"type":"integer"},
			"kind":{"type":"string","enum":["income","expense","refund","transfer","trade","adjustment"]},
			"search":{"type":"string","description":"Case-insensitive text matched against description and merchant."},
			"limit":{"type":"integer","minimum":1,"maximum":200}
		},"additionalProperties":false}`,
		label: "Searching transactions",
		run:   searchTransactions,
	},
	{
		name: "spending_breakdown",
		description: "Totals spending or income over a date range, grouped by category, merchant, month, week or account, " +
			"with a transaction count per group. Spending is net of refunds and shown as a positive amount. " +
			"Transfers between the household's own accounts and investment trades never count as either. " +
			"Use this for any question about how much was spent or earned.",
		schema: `{"type":"object","properties":{
			"side":{"type":"string","enum":["expense","income"]},
			"group_by":{"type":"string","enum":["category","merchant","month","week","account"]},
			"from":{"type":"string","description":"First date, YYYY-MM-DD, inclusive."},
			"to":{"type":"string","description":"Last date, YYYY-MM-DD, inclusive."},
			"account_id":{"type":"integer"},
			"category_id":{"type":"integer"}
		},"required":["side","group_by"],"additionalProperties":false}`,
		label: "Adding up spending",
		run: func(ctx context.Context, svc *ledger.Service, hh int, input []byte) (any, error) {
			var in struct {
				Side       string `json:"side"`
				GroupBy    string `json:"group_by"`
				From       string `json:"from"`
				To         string `json:"to"`
				AccountID  int    `json:"account_id"`
				CategoryID int    `json:"category_id"`
			}
			if err := decodeInput(input, &in); err != nil {
				return nil, err
			}
			if err := checkDates(in.From, in.To); err != nil {
				return nil, err
			}
			rows, err := svc.SpendingBreakdown(ctx, hh, ledger.BreakdownFilter{
				Side: in.Side, GroupBy: in.GroupBy, From: in.From, To: in.To,
				AccountID: in.AccountID, CategoryID: in.CategoryID,
			})
			if err != nil {
				return nil, err
			}
			var total float64
			for _, r := range rows {
				total += r.Amount
			}
			return map[string]any{"groups": rows, "total": round2(total)}, nil
		},
	},
	{
		name: "get_budget_month",
		description: "One month's budget as the Budgets page shows it: each category's budget, what was spent (or, " +
			"for income categories, received) and what is still scheduled; expected and received income; " +
			"the savings goals planned for the month and how much has been moved to each; any savings shortfall " +
			"(planned savings that were spent instead); and each credit card and loan: paid, newly charged, and the " +
			"paydown (paid beyond new charges) that counts toward the month.",
		schema: `{"type":"object","properties":{
			"month":{"type":"string","description":"YYYY-MM. Defaults to the current month."}
		},"additionalProperties":false}`,
		label: "Checking the budget",
		run: func(ctx context.Context, svc *ledger.Service, hh int, input []byte) (any, error) {
			var in struct {
				Month string `json:"month"`
			}
			if err := decodeInput(input, &in); err != nil {
				return nil, err
			}
			if in.Month != "" && !monthPattern.MatchString(in.Month) {
				return nil, fmt.Errorf("%w: month must be YYYY-MM", errBadInput)
			}
			return svc.BudgetMonth(ctx, hh, in.Month)
		},
	},
	{
		name: "list_goals",
		description: "Long-term savings goals: target (how much to add, not a balance to reach), how much has been " +
			"saved toward it since it started, its account, target date and current monthly plan. " +
			"One-month goals appear in get_budget_month instead.",
		schema: `{"type":"object","properties":{},"additionalProperties":false}`,
		label:  "Checking goals",
		run: func(ctx context.Context, svc *ledger.Service, hh int, input []byte) (any, error) {
			if err := decodeInput(input, &struct{}{}); err != nil {
				return nil, err
			}
			return svc.ListGoals(ctx, hh)
		},
	},
	{
		name: "list_recurring",
		description: "Recurring rules (subscriptions, bills, paychecks, scheduled transfers) with their amount, " +
			"frequency and accounts, plus the occurrences coming due in the next `days` days.",
		schema: `{"type":"object","properties":{
			"days":{"type":"integer","minimum":1,"maximum":365,"description":"How far ahead to list upcoming occurrences. Default 30."}
		},"additionalProperties":false}`,
		label: "Checking recurring bills",
		run: func(ctx context.Context, svc *ledger.Service, hh int, input []byte) (any, error) {
			var in struct {
				Days int `json:"days"`
			}
			if err := decodeInput(input, &in); err != nil {
				return nil, err
			}
			if in.Days <= 0 || in.Days > 365 {
				in.Days = 30
			}
			rules, err := svc.ListRules(ctx, hh)
			if err != nil {
				return nil, err
			}
			upcoming, err := svc.Upcoming(ctx, hh, in.Days)
			if err != nil {
				return nil, err
			}
			return map[string]any{"rules": rules, "upcoming": upcoming}, nil
		},
	},
	{
		name: "get_investments",
		description: "Holdings at the latest stored prices: shares, value, cost basis, unrealized and realized gain and " +
			"day change per position, with totals. Without account_id, every investment and retirement account combined " +
			"(with a per-account value list); with it, that one account.",
		schema: `{"type":"object","properties":{
			"account_id":{"type":"integer"}
		},"additionalProperties":false}`,
		label: "Checking investments",
		run: func(ctx context.Context, svc *ledger.Service, hh int, input []byte) (any, error) {
			var in struct {
				AccountID int `json:"account_id"`
			}
			if err := decodeInput(input, &in); err != nil {
				return nil, err
			}
			if in.AccountID > 0 {
				return svc.Holdings(ctx, hh, in.AccountID)
			}
			return svc.InvestmentsSummary(ctx, hh)
		},
	},
	{
		name:        "net_worth_history",
		description: "Daily net worth snapshots (total assets, total liabilities, net worth), oldest first.",
		schema: `{"type":"object","properties":{
			"days":{"type":"integer","minimum":1,"maximum":1100,"description":"How many of the most recent days. Default 365."}
		},"additionalProperties":false}`,
		label: "Checking net worth",
		run: func(ctx context.Context, svc *ledger.Service, hh int, input []byte) (any, error) {
			var in struct {
				Days int `json:"days"`
			}
			if err := decodeInput(input, &in); err != nil {
				return nil, err
			}
			if in.Days <= 0 || in.Days > 1100 {
				in.Days = 365
			}
			return svc.NetWorthHistory(ctx, hh, in.Days)
		},
	},
}

// compactTxn is a transaction with only what answers questions, so a page of
// them costs a fraction of the full record.
type compactTxn struct {
	ID          int     `json:"id"`
	Date        string  `json:"date"`
	Account     string  `json:"account"`
	Amount      float64 `json:"amount"`
	Kind        string  `json:"kind"`
	Description string  `json:"description"`
	Merchant    *string `json:"merchant,omitempty"`
	Category    *string `json:"category,omitempty"`
	Notes       *string `json:"notes,omitempty"`
	Source      string  `json:"source,omitempty"`
}

func searchTransactions(ctx context.Context, svc *ledger.Service, hh int, input []byte) (any, error) {
	var in struct {
		From       string `json:"from"`
		To         string `json:"to"`
		AccountID  int    `json:"account_id"`
		CategoryID int    `json:"category_id"`
		Kind       string `json:"kind"`
		Search     string `json:"search"`
		Limit      int    `json:"limit"`
	}
	if err := decodeInput(input, &in); err != nil {
		return nil, err
	}
	if err := checkDates(in.From, in.To); err != nil {
		return nil, err
	}
	if in.Limit <= 0 {
		in.Limit = 50
	}
	if in.Limit > 200 {
		in.Limit = 200
	}
	// One extra row says whether there is more without a second count query.
	txns, err := svc.ListTransactions(ctx, hh, ledger.TransactionFilter{
		AccountID: in.AccountID, CategoryID: in.CategoryID, Kind: in.Kind,
		From: in.From, To: in.To, Search: in.Search, Limit: in.Limit + 1,
	})
	if err != nil {
		return nil, err
	}
	more := len(txns) > in.Limit
	if more {
		txns = txns[:in.Limit]
	}
	out := make([]compactTxn, len(txns))
	for i, t := range txns {
		out[i] = compact(t)
	}
	return map[string]any{"transactions": out, "more_matched": more}, nil
}

func compact(t models.Transaction) compactTxn {
	c := compactTxn{
		ID: t.ID, Date: t.Date, Account: t.AccountName, Amount: t.Amount, Kind: t.Kind,
		Description: t.Description, Merchant: t.Merchant, Category: t.CategoryName, Notes: t.Notes,
	}
	if t.Source != models.SourceManual {
		c.Source = t.Source
	}
	return c
}

// errBadInput marks a tool call the model got wrong. Its message goes back to
// the model so it can correct itself.
var errBadInput = errors.New("invalid input")

// decodeInput reads a tool's arguments strictly: with eager input streaming the
// API no longer validates them against the schema, so this is the check.
func decodeInput(input []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(input))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("%w: %v", errBadInput, err)
	}
	return nil
}

func checkDates(dates ...string) error {
	for _, d := range dates {
		if d != "" && !datePattern.MatchString(d) {
			return fmt.Errorf("%w: dates must be YYYY-MM-DD, got %q", errBadInput, d)
		}
	}
	return nil
}

func round2(f float64) float64 {
	return math.Round(f*100) / 100
}

var toolsByName = func() map[string]*tool {
	m := make(map[string]*tool, len(tools))
	for i := range tools {
		m[tools[i].name] = &tools[i]
	}
	return m
}()

// toolDefs is the tool list as sent to the API.
var toolDefs = func() []toolDef {
	defs := make([]toolDef, len(tools))
	for i, t := range tools {
		var compacted bytes.Buffer
		if err := json.Compact(&compacted, []byte(t.schema)); err != nil {
			panic(fmt.Sprintf("assistant: tool %s has an invalid schema: %v", t.name, err))
		}
		defs[i] = toolDef{
			Name: t.name, Description: t.description,
			InputSchema: compacted.Bytes(), EagerInputStreaming: true,
		}
	}
	return defs
}()

// runTool executes one tool call and returns its result as the text of a
// tool_result block, and whether that result is an error.
func runTool(ctx context.Context, svc *ledger.Service, householdID int, name string, input []byte) (string, bool) {
	t, ok := toolsByName[name]
	if !ok {
		return fmt.Sprintf("There is no tool named %q.", name), true
	}
	result, err := t.run(ctx, svc, householdID, input)
	if err != nil {
		var invalid ledger.ErrInvalid
		switch {
		case errors.Is(err, errBadInput):
			return err.Error(), true
		case errors.As(err, &invalid):
			return invalid.Msg, true
		case errors.Is(err, ledger.ErrNotFound):
			return "Not found.", true
		default:
			log.Printf("assistant: tool %s failed: %v", name, err)
			return "The lookup failed. Tell the user you couldn't get this information right now.", true
		}
	}
	out, err := json.Marshal(result)
	if err != nil {
		log.Printf("assistant: encoding %s result: %v", name, err)
		return "The lookup failed.", true
	}
	if len(out) > maxResultBytes {
		return fmt.Sprintf("The result is too large (%d KB). Narrow it: a shorter date range, a filter, "+
			"a smaller limit, or spending_breakdown for totals.", len(out)>>10), true
	}
	return string(out), false
}
