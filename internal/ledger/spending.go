package ledger

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// BreakdownFilter selects what SpendingBreakdown totals. Side is "expense"
// (spending net of refunds) or "income"; GroupBy is one of the keys of
// breakdownKeys. The rest are optional.
type BreakdownFilter struct {
	Side       string
	GroupBy    string
	From       string
	To         string
	AccountID  int
	CategoryID int
}

// BreakdownRow is one group's total. Amount is positive on both sides: money
// spent, or money received.
type BreakdownRow struct {
	Key    string  `json:"key"`
	Amount float64 `json:"amount"`
	Count  int     `json:"count"`
}

// breakdownKeys are the only expressions a breakdown can group by — GroupBy is
// looked up here, never interpolated.
var breakdownKeys = map[string]string{
	"category": `COALESCE(c.name, 'Uncategorized')`,
	"merchant": `COALESCE(NULLIF(TRIM(t.merchant), ''), t.description)`,
	"month":    `to_char(t.date, 'YYYY-MM')`,
	"week":     `to_char(date_trunc('week', t.date), 'YYYY-MM-DD')`,
	"account":  `a.name`,
}

// SpendingBreakdown totals income or spending by category, merchant, month,
// week or account. It follows the same rules as the dashboard and budgets:
// kinds are listed rather than excluded, so transfers and trade legs never
// count, and refunds sit on the spending side, cancelling what they came back
// from.
func (s *Service) SpendingBreakdown(ctx context.Context, householdID int, f BreakdownFilter) ([]BreakdownRow, error) {
	key, ok := breakdownKeys[f.GroupBy]
	if !ok {
		return nil, invalid("group_by must be one of category, merchant, month, week, account")
	}
	var kinds, sign string
	switch f.Side {
	case "expense":
		kinds, sign = `t.kind IN ('expense','refund')`, "-"
	case "income":
		kinds, sign = `t.kind = 'income'`, ""
	default:
		return nil, invalid("side must be expense or income")
	}

	where := []string{"t.household_id = $1", kinds}
	args := []any{householdID}
	bind := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	if f.From != "" {
		where = append(where, "t.date >= "+bind(f.From))
	}
	if f.To != "" {
		where = append(where, "t.date <= "+bind(f.To))
	}
	if f.AccountID > 0 {
		where = append(where, "t.account_id = "+bind(f.AccountID))
	}
	if f.CategoryID > 0 {
		where = append(where, "t.category_id = "+bind(f.CategoryID))
	}

	// Time groups read in order; the rest read biggest first.
	order := "amount DESC, key"
	if f.GroupBy == "month" || f.GroupBy == "week" {
		order = "key"
	}

	q := `SELECT ` + key + ` AS key, ` + sign + `SUM(t.amount) AS amount, COUNT(*)
	        FROM transactions t
	        JOIN accounts a ON a.id = t.account_id
	        LEFT JOIN categories c ON c.id = t.category_id
	       WHERE ` + strings.Join(where, " AND ") + `
	       GROUP BY 1 ORDER BY ` + order + ` LIMIT 200`

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("spending breakdown: %w", err)
	}
	defer rows.Close()

	out := []BreakdownRow{}
	for rows.Next() {
		var r BreakdownRow
		if err := rows.Scan(&r.Key, &r.Amount, &r.Count); err != nil {
			return nil, fmt.Errorf("scanning breakdown: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
