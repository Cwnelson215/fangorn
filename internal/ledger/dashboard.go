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
	// Month is the first of the month shown; From–To is the part of it covered
	// (through today for the current month).
	Month string `json:"month"`
	From  string `json:"from"`
	To    string `json:"to"`
	// Current is true for the month containing the household's today.
	Current bool `json:"current"`

	Income   float64 `json:"income"`
	Expenses float64 `json:"expenses"`
	Net      float64 `json:"net"`

	// The previous month to the same point: through the same day of the month
	// when this is the current month (clamped to its end), the whole month
	// otherwise. PrevFrom–PrevTo is that range.
	PrevFrom     string  `json:"prev_from"`
	PrevTo       string  `json:"prev_to"`
	PrevIncome   float64 `json:"prev_income"`
	PrevExpenses float64 `json:"prev_expenses"`

	// ExpectedIncome is the month's expected income (the budgets page's
	// Income line); 0 when none is set.
	ExpectedIncome float64 `json:"expected_income"`
	// What recurring rules still have to post this month — all income, and
	// every expense with or without a category. Both 0 for a past month.
	ScheduledIncome   float64 `json:"scheduled_income"`
	ScheduledExpenses float64 `json:"scheduled_expenses"`
	// Left is what the month leaves once those post: Net + ScheduledIncome −
	// ScheduledExpenses. For a past month it is just Net.
	Left float64 `json:"left"`

	TotalAssets      float64 `json:"total_assets"`
	TotalLiabilities float64 `json:"total_liabilities"`
	NetWorth         float64 `json:"net_worth"`
	// RetirementValue is the part of NetWorth held in retirement accounts —
	// money that is counted but can't be spent without penalties.
	RetirementValue float64 `json:"retirement_value"`

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
//
// It covers one calendar month (`month`, YYYY-MM or a date in it; empty for the
// current one, in the household's timezone) so its totals line up with the
// budgets page. The current month runs through today. A month after the
// current one is refused: nothing has happened in it. An explicit from/to
// overrides the range the totals and category breakdown cover.
func (s *Service) Dashboard(ctx context.Context, householdID int, month, from, to string) (Dashboard, error) {
	monthStart, today, err := s.resolveBudgetMonth(ctx, householdID, month)
	if err != nil {
		return Dashboard{}, err
	}
	start, _ := models.ParseDate(monthStart)
	end := start.AddDate(0, 1, -1)
	thisMonth := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	if start.After(thisMonth) {
		return Dashboard{}, invalid("that month hasn't started yet")
	}
	current := start.Equal(thisMonth)
	if from == "" {
		from = monthStart
	}
	if to == "" {
		to = dateStr(end)
		if current {
			to = dateStr(today)
		}
	}
	d := Dashboard{Month: monthStart, From: from, To: to, Current: current}

	// Last month to the same point: the 20th for the 20th, or its last day when
	// it's shorter (the 30th of March compares to February 28th).
	prevStart := start.AddDate(0, -1, 0)
	prevEnd := start.AddDate(0, 0, -1)
	d.PrevFrom, d.PrevTo = dateStr(prevStart), dateStr(prevEnd)
	if current {
		if same := prevStart.AddDate(0, 0, today.Day()-1); same.Before(prevEnd) {
			d.PrevTo = dateStr(same)
		}
	}

	if d.Income, d.Expenses, err = s.incomeAndSpending(ctx, householdID, from, to); err != nil {
		return d, err
	}
	d.Net = d.Income - d.Expenses
	if d.PrevIncome, d.PrevExpenses, err = s.incomeAndSpending(ctx, householdID, d.PrevFrom, d.PrevTo); err != nil {
		return d, err
	}

	if d.ExpectedIncome, err = s.incomePlan(ctx, householdID, monthStart); err != nil {
		return d, err
	}
	scheduled, scheduledIncome, err := s.scheduledSpend(ctx, householdID, monthStart, today)
	if err != nil {
		return d, err
	}
	for _, items := range scheduled {
		for _, it := range items {
			d.ScheduledExpenses += it.Amount
		}
	}
	for _, it := range scheduledIncome {
		d.ScheduledIncome += it.Amount
	}
	d.ScheduledExpenses, d.ScheduledIncome = round2(d.ScheduledExpenses), round2(d.ScheduledIncome)
	d.Left = round2(d.Net + d.ScheduledIncome - d.ScheduledExpenses)

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
		if a.Type == models.AccountRetirement {
			d.RetirementValue += a.Balance
		}
	}
	d.NetWorth = d.TotalAssets - d.TotalLiabilities

	if err := s.dashboardRest(ctx, householdID, &d, monthStart); err != nil {
		return d, err
	}
	return d, nil
}

// incomeAndSpending totals income and spending (net of refunds) between two
// dates, inclusive.
//
// The kinds are listed rather than excluded: transfers and trade cash legs
// both move money without earning or spending it, and a kind added later
// should have to opt in to these totals rather than leak into them.
//
// The split is by kind rather than by sign, which matters for refunds: they
// are positive like income but belong on the spending side, where they cancel
// the expense they came back from. Splitting on sign would file every return
// as earnings and overstate both halves.
func (s *Service) incomeAndSpending(ctx context.Context, householdID int, from, to string) (income, spent float64, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(amount) FILTER (WHERE kind = 'income'), 0),
		        COALESCE(-SUM(amount) FILTER (WHERE kind IN ('expense','refund')), 0)
		 FROM transactions
		 WHERE household_id = $1 AND kind IN ('income','expense','refund') AND date BETWEEN $2 AND $3`,
		householdID, from, to,
	).Scan(&income, &spent)
	if err != nil {
		return 0, 0, fmt.Errorf("computing totals: %w", err)
	}
	return income, spent, nil
}

// dashboardRest fills in everything that isn't the month's totals.
func (s *Service) dashboardRest(ctx context.Context, householdID int, d *Dashboard, monthStart string) error {
	accounts := d.Accounts
	var err error

	// Stored prices only: the dashboard is one round trip and must not wait on
	// the quote provider. The scheduler keeps them within a tick of current.
	for _, a := range accounts {
		if !models.HoldsSecurities(a.Type) {
			continue
		}
		summary, err := s.InvestmentsSummary(ctx, householdID)
		if err != nil {
			return err
		}
		d.Investments = &InvestmentsGlance{
			TotalValue: summary.TotalValue, DayChange: summary.DayChange,
			DayChangePct: summary.DayChangePct, AsOf: summary.AsOf,
		}
		break
	}

	if d.Categories, err = s.categorySpend(ctx, householdID, d.From, d.To); err != nil {
		return err
	}
	if d.WeeklySpending, err = s.weeklySpending(ctx, householdID, d.To); err != nil {
		return err
	}
	if d.NetWorthHistory, err = s.NetWorthHistory(ctx, householdID, 365); err != nil {
		return err
	}
	if d.Budgets, err = s.ListBudgets(ctx, householdID, monthStart); err != nil {
		return err
	}
	if d.Goals, err = s.ListGoals(ctx, householdID); err != nil {
		return err
	}
	if d.Upcoming, err = s.Upcoming(ctx, householdID, 30); err != nil {
		return err
	}
	return nil
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
		  AND t.kind IN ('expense','refund')
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
		// HAVING drops categories a refund has cancelled out: a month where the only
		// activity was returning something nets to zero or below, which is not a
		// slice the breakdown can draw and not spending worth listing.
		`SELECT t.category_id, COALESCE(c.name, 'Uncategorized'), c.color, -SUM(t.amount)
		 FROM transactions t
		 LEFT JOIN categories c ON c.id = t.category_id
		 WHERE t.household_id = $1 AND t.kind IN ('expense','refund') AND t.date BETWEEN $2 AND $3
		 GROUP BY t.category_id, c.name, c.color
		 HAVING -SUM(t.amount) > 0
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
