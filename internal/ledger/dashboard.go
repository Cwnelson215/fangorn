package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/cwnelson/fangorn/internal/models"
)

type CategorySpend struct {
	CategoryID   *int    `json:"category_id"`
	CategoryName string  `json:"category_name"`
	Color        *string `json:"color"`
	Amount       float64 `json:"amount"`
}

// WeekSpend is one week of expenses. Week is the Monday it starts on.
type WeekSpend struct {
	Week   string  `json:"week"`
	Amount float64 `json:"amount"`
}

// trendWeeks is how far back the dashboard's spending trend reaches.
const trendWeeks = 12

// InvestmentsGlance is the one line the dashboard shows about investments.
type InvestmentsGlance struct {
	TotalValue   float64  `json:"total_value"`
	DayChange    float64  `json:"day_change"`
	DayChangePct *float64 `json:"day_change_pct"`
	AsOf         *string  `json:"as_of"`
}

type Dashboard struct {
	From string `json:"from"`
	To   string `json:"to"`

	Income   float64 `json:"income"`
	Expenses float64 `json:"expenses"`
	Net      float64 `json:"net"`

	TotalAssets      float64 `json:"total_assets"`
	TotalLiabilities float64 `json:"total_liabilities"`
	NetWorth         float64 `json:"net_worth"`

	Accounts        []models.Account       `json:"accounts"`
	Categories      []CategorySpend        `json:"categories"`
	WeeklySpending  []WeekSpend            `json:"weekly_spending"`
	NetWorthHistory []models.NetWorthPoint `json:"net_worth_history"`
	Budgets         []models.Budget        `json:"budgets"`
	// Investments is nil when the household has no investment accounts.
	Investments *InvestmentsGlance  `json:"investments"`
	Goals       []models.Goal       `json:"goals"`
	Upcoming    []models.Occurrence `json:"upcoming"`
}

// Dashboard assembles everything the landing page needs in one round trip.
//
// Transfers and trades are excluded from income and expenses throughout: moving
// $200 from checking to savings is not earning $200 and not spending $200, and
// neither is buying $200 of an index fund. Counting any of them would make every
// summary wrong.
func (s *Service) Dashboard(ctx context.Context, householdID int, from, to string) (Dashboard, error) {
	if from == "" {
		from = time.Now().AddDate(0, -1, 0).Format(models.DateOnly)
	}
	if to == "" {
		to = time.Now().Format(models.DateOnly)
	}
	d := Dashboard{From: from, To: to}

	// Income and expenses. Amounts are signed, so income is simply the positive
	// side and expenses the negative side, reported as a positive magnitude.
	//
	// The kinds are listed rather than excluded: transfers and trade cash legs
	// both move money without earning or spending it, and a kind added later
	// should have to opt in to these totals rather than leak into them.
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(amount) FILTER (WHERE amount > 0), 0),
		        COALESCE(-SUM(amount) FILTER (WHERE amount < 0), 0)
		 FROM transactions
		 WHERE household_id = $1 AND kind IN ('income','expense') AND date BETWEEN $2 AND $3`,
		householdID, from, to,
	).Scan(&d.Income, &d.Expenses)
	if err != nil {
		return d, fmt.Errorf("computing totals: %w", err)
	}
	d.Net = d.Income - d.Expenses

	accounts, err := s.ListAccounts(ctx, householdID, false)
	if err != nil {
		return d, err
	}
	d.Accounts = accounts
	for _, a := range accounts {
		if a.Class == models.ClassLiability {
			// Liability balances are negative; report the debt as a positive figure.
			d.TotalLiabilities += -a.Balance
		} else {
			d.TotalAssets += a.Balance
		}
	}
	d.NetWorth = d.TotalAssets - d.TotalLiabilities

	// Stored prices only: the dashboard is one round trip and must not wait on
	// the quote provider. The scheduler keeps them within a tick of current.
	for _, a := range accounts {
		if a.Type != models.AccountInvestment {
			continue
		}
		summary, err := s.InvestmentsSummary(ctx, householdID)
		if err != nil {
			return d, err
		}
		d.Investments = &InvestmentsGlance{
			TotalValue: summary.TotalValue, DayChange: summary.DayChange,
			DayChangePct: summary.DayChangePct, AsOf: summary.AsOf,
		}
		break
	}

	if d.Categories, err = s.categorySpend(ctx, householdID, from, to); err != nil {
		return d, err
	}
	if d.WeeklySpending, err = s.weeklySpending(ctx, householdID, to); err != nil {
		return d, err
	}
	if d.NetWorthHistory, err = s.NetWorthHistory(ctx, householdID, 365); err != nil {
		return d, err
	}
	if d.Budgets, err = s.ListBudgets(ctx, householdID, ""); err != nil {
		return d, err
	}
	if d.Goals, err = s.ListGoals(ctx, householdID); err != nil {
		return d, err
	}
	if d.Upcoming, err = s.Upcoming(ctx, householdID, 30); err != nil {
		return d, err
	}
	return d, nil
}

// weeklySpending totals expenses per week for the trendWeeks weeks ending with
// the one containing `to`. The weeks come from generate_series rather than from
// the transactions, so a week with no spending is a zero instead of a gap the
// chart would draw straight across.
func (s *Service) weeklySpending(ctx context.Context, householdID int, to string) ([]WeekSpend, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT w.week::date, COALESCE(-SUM(t.amount), 0)
		 FROM generate_series(
		        date_trunc('week', $2::date - ($3 * INTERVAL '1 week')),
		        date_trunc('week', $2::date),
		        INTERVAL '1 week'
		      ) AS w(week)
		 LEFT JOIN transactions t
		   ON t.household_id = $1
		  AND t.kind = 'expense'
		  AND t.date >= w.week
		  AND t.date < w.week + INTERVAL '1 week'
		  AND t.date <= $2::date
		 GROUP BY w.week
		 ORDER BY w.week`,
		householdID, to, trendWeeks-1)
	if err != nil {
		return nil, fmt.Errorf("computing weekly spending: %w", err)
	}
	defer rows.Close()

	out := []WeekSpend{}
	for rows.Next() {
		var ws WeekSpend
		var week time.Time
		if err := rows.Scan(&week, &ws.Amount); err != nil {
			return nil, fmt.Errorf("scanning weekly spending: %w", err)
		}
		ws.Week = dateStr(week)
		out = append(out, ws)
	}
	return out, rows.Err()
}

func (s *Service) categorySpend(ctx context.Context, householdID int, from, to string) ([]CategorySpend, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.category_id, COALESCE(c.name, 'Uncategorized'), c.color, -SUM(t.amount)
		 FROM transactions t
		 LEFT JOIN categories c ON c.id = t.category_id
		 WHERE t.household_id = $1 AND t.kind = 'expense' AND t.date BETWEEN $2 AND $3
		 GROUP BY t.category_id, c.name, c.color
		 ORDER BY -SUM(t.amount) DESC`,
		householdID, from, to)
	if err != nil {
		return nil, fmt.Errorf("computing category breakdown: %w", err)
	}
	defer rows.Close()

	out := []CategorySpend{}
	for rows.Next() {
		var cs CategorySpend
		var id sql.NullInt64
		var color sql.NullString
		if err := rows.Scan(&id, &cs.CategoryName, &color, &cs.Amount); err != nil {
			return nil, fmt.Errorf("scanning category spend: %w", err)
		}
		cs.CategoryID = intPtr(id)
		cs.Color = strPtr(color)
		out = append(out, cs)
	}
	return out, rows.Err()
}
