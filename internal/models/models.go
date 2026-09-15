// Package models holds the shared domain types for the family ledger.
//
// Sign convention, used everywhere:
//
//	Transaction.Amount is signed RELATIVE TO THE ACCOUNT.
//	positive = money in, negative = money out.
//
// Liability accounts (credit_card, loan) therefore carry negative balances, which
// makes balance = StartingBalance + sum(Amount) and netWorth = sum(balance) correct
// for every account type without special-casing.
package models

import "time"

// Account types and the class each implies.
const (
	AccountChecking   = "checking"
	AccountSavings    = "savings"
	AccountCash       = "cash"
	AccountInvestment = "investment"
	AccountCreditCard = "credit_card"
	AccountLoan       = "loan"

	ClassAsset     = "asset"
	ClassLiability = "liability"
)

// Transaction and rule kinds.
const (
	KindIncome   = "income"
	KindExpense  = "expense"
	KindTransfer = "transfer"
	// KindTrade is the cash side of a buy or sell in an investment account. It is
	// only ever written by the trade endpoints, and like a transfer it is neither
	// income nor spending.
	KindTrade = "trade"
)

// Recurrence frequencies.
const (
	FreqDaily       = "daily"
	FreqWeekly      = "weekly"
	FreqBiweekly    = "biweekly"
	FreqSemimonthly = "semimonthly"
	FreqMonthly     = "monthly"
	FreqQuarterly   = "quarterly"
	FreqYearly      = "yearly"
)

// Occurrence statuses.
const (
	OccurrenceScheduled = "scheduled"
	OccurrencePosted    = "posted"
	OccurrenceSkipped   = "skipped"
)

// Transaction sources.
const (
	SourceManual    = "manual"
	SourceRecurring = "recurring"
)

// ClassForType returns the balance class implied by an account type.
// Unknown types are treated as assets; the DB CHECK constraint is the real guard.
func ClassForType(accountType string) string {
	switch accountType {
	case AccountCreditCard, AccountLoan:
		return ClassLiability
	default:
		return ClassAsset
	}
}

// ValidAccountType reports whether t is one of the supported account types.
func ValidAccountType(t string) bool {
	switch t {
	case AccountChecking, AccountSavings, AccountCash,
		AccountInvestment, AccountCreditCard, AccountLoan:
		return true
	}
	return false
}

type Account struct {
	ID                  int     `json:"id"`
	HouseholdID         int     `json:"household_id"`
	Name                string  `json:"name"`
	InstitutionName     *string `json:"institution_name"`
	Type                string  `json:"type"`
	Class               string  `json:"class"`
	Mask                *string `json:"mask"`
	StartingBalance     float64 `json:"starting_balance"`
	StartingBalanceDate string  `json:"starting_balance_date"`
	Currency            string  `json:"currency"`
	Color               *string `json:"color"`
	Notes               *string `json:"notes"`
	Archived            bool    `json:"archived"`

	// CashBalance is StartingBalance plus every posted transaction on the account.
	// HoldingsValue is the market value of an investment account's positions at
	// the latest known prices (0 for every other type). Balance is their sum, and
	// is the figure net worth adds up. All three are computed on read.
	CashBalance   float64 `json:"cash_balance"`
	HoldingsValue float64 `json:"holdings_value"`
	Balance       float64 `json:"balance"`
}

type Category struct {
	ID       int     `json:"id"`
	Name     string  `json:"name"`
	Kind     string  `json:"kind"`
	Color    *string `json:"color"`
	ParentID *int    `json:"parent_id"`
	Archived bool    `json:"archived"`
}

type Transaction struct {
	ID              int     `json:"id"`
	AccountID       int     `json:"account_id"`
	AccountName     string  `json:"account_name,omitempty"`
	Date            string  `json:"date"`
	Amount          float64 `json:"amount"`
	Kind            string  `json:"kind"`
	Description     string  `json:"description"`
	Merchant        *string `json:"merchant"`
	CategoryID      *int    `json:"category_id"`
	CategoryName    *string `json:"category_name,omitempty"`
	Notes           *string `json:"notes"`
	TransferGroupID *string `json:"transfer_group_id"`
	RecurringRuleID *int    `json:"recurring_rule_id"`
	TradeID         *int    `json:"trade_id"`
	Source          string  `json:"source"`
	CreatedAt       string  `json:"created_at"`

	// RunningBalance is only populated by the per-account register view.
	RunningBalance *float64 `json:"running_balance,omitempty"`
}

// Transfer is the paired view of two transactions sharing a transfer_group_id.
type Transfer struct {
	GroupID       string  `json:"group_id"`
	FromAccountID int     `json:"from_account_id"`
	FromAccount   string  `json:"from_account"`
	ToAccountID   int     `json:"to_account_id"`
	ToAccount     string  `json:"to_account"`
	Amount        float64 `json:"amount"`
	Date          string  `json:"date"`
	Description   string  `json:"description"`
	Notes         *string `json:"notes"`
}

type RecurringRule struct {
	ID               int     `json:"id"`
	HouseholdID      int     `json:"household_id"`
	Name             string  `json:"name"`
	Vendor           *string `json:"vendor"`
	Kind             string  `json:"kind"`
	AccountID        int     `json:"account_id"`
	AccountName      string  `json:"account_name,omitempty"`
	ToAccountID      *int    `json:"to_account_id"`
	ToAccountName    *string `json:"to_account_name,omitempty"`
	CategoryID       *int    `json:"category_id"`
	CategoryName     *string `json:"category_name,omitempty"`
	Amount           float64 `json:"amount"`
	Frequency        string  `json:"frequency"`
	IntervalCount    int     `json:"interval_count"`
	DayOfMonth       *int    `json:"day_of_month"`
	SecondDayOfMonth *int    `json:"second_day_of_month"`
	DayOfWeek        *int    `json:"day_of_week"`
	MonthOfYear      *int    `json:"month_of_year"`
	StartDate        string  `json:"start_date"`
	EndDate          *string `json:"end_date"`
	NextDueDate      *string `json:"next_due_date"`
	AutoPost         bool    `json:"auto_post"`
	ReminderLeadDays *int    `json:"reminder_lead_days"`
	Paused           bool    `json:"paused"`
	Notes            *string `json:"notes"`
}

type Occurrence struct {
	ID            int     `json:"id"`
	RuleID        int     `json:"rule_id"`
	RuleName      string  `json:"rule_name,omitempty"`
	Kind          string  `json:"kind,omitempty"`
	Amount        float64 `json:"amount,omitempty"`
	AccountName   string  `json:"account_name,omitempty"`
	ToAccountName *string `json:"to_account_name,omitempty"`
	DueDate       string  `json:"due_date"`
	Status        string  `json:"status"`
	TransactionID *int    `json:"transaction_id"`
}

type Budget struct {
	ID            int     `json:"id"`
	CategoryID    int     `json:"category_id"`
	CategoryName  string  `json:"category_name,omitempty"`
	CategoryColor *string `json:"category_color,omitempty"`
	Period        string  `json:"period"`
	Amount        float64 `json:"amount"`
	EffectiveFrom string  `json:"effective_from"`

	// Spent is the month-to-date spend against this category, as a positive number.
	Spent float64 `json:"spent"`
}

// BudgetMonth is one month of budgets as the budgets page shows it.
type BudgetMonth struct {
	Month   string   `json:"month"` // first of the month, YYYY-MM-DD
	Budgets []Budget `json:"budgets"`

	// UnbudgetedSpent is the month's expense spend in categories with no budget,
	// including uncategorized transactions, as a positive number.
	UnbudgetedSpent float64 `json:"unbudgeted_spent"`
}

type Goal struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	TargetAmount float64 `json:"target_amount"`
	TargetDate   *string `json:"target_date"`
	AccountID    *int    `json:"account_id"`
	AccountName  *string `json:"account_name,omitempty"`
	Notes        *string `json:"notes"`
	Achieved     bool    `json:"achieved"`
	Saved        float64 `json:"saved"`
}

// Trade is one entry in an investment account's trade log. Side is one of the
// portfolio.Side* constants. Amount is the dollar figure (see portfolio.CashAmount).
type Trade struct {
	ID           int     `json:"id"`
	AccountID    int     `json:"account_id"`
	Symbol       string  `json:"symbol"`
	SecurityName *string `json:"security_name"`
	Side         string  `json:"side"`
	TradeDate    string  `json:"trade_date"`
	Shares       float64 `json:"shares"`
	Price        float64 `json:"price"`
	Fees         float64 `json:"fees"`
	Amount       float64 `json:"amount"`
	Notes        *string `json:"notes"`
	CreatedAt    string  `json:"created_at"`
}

// Security is the latest known price for a symbol. It is market data shared by
// every household, not something any one household owns.
type Security struct {
	Symbol        string   `json:"symbol"`
	Name          *string  `json:"name"`
	QuoteType     *string  `json:"quote_type"`
	Currency      string   `json:"currency"`
	Exchange      *string  `json:"exchange"`
	Price         float64  `json:"price"`
	PreviousClose *float64 `json:"previous_close"`
	// PriceTime is nil when the price was seeded from a trade and no quote has
	// ever been fetched for the symbol.
	PriceTime  *string `json:"price_time"`
	FetchError *string `json:"fetch_error"`
}

type NetWorthPoint struct {
	Date             string  `json:"date"`
	TotalAssets      float64 `json:"total_assets"`
	TotalLiabilities float64 `json:"total_liabilities"`
	NetWorth         float64 `json:"net_worth"`
}

// DateOnly is the wire and storage format for every date in the app.
const DateOnly = "2006-01-02"

// ParseDate parses a YYYY-MM-DD string into a UTC-midnight time.
func ParseDate(s string) (time.Time, error) {
	return time.Parse(DateOnly, s)
}
