package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/cwnelson/fangorn/internal/models"
)

// ListBudgets returns the budget in force for the month containing `month`,
// along with that month's spend against each category. An empty month means the
// current month in the household's timezone.
func (s *Service) ListBudgets(ctx context.Context, householdID int, month string) ([]models.Budget, error) {
	monthStart, today, err := s.resolveBudgetMonth(ctx, householdID, month)
	if err != nil {
		return nil, err
	}
	return s.listBudgets(ctx, householdID, monthStart, today)
}

// BudgetMonth is ListBudgets plus what the budgets page needs around it: the
// month that was actually resolved, and the spend that no budget covers.
func (s *Service) BudgetMonth(ctx context.Context, householdID int, month string) (models.BudgetMonth, error) {
	monthStart, today, err := s.resolveBudgetMonth(ctx, householdID, month)
	if err != nil {
		return models.BudgetMonth{}, err
	}
	budgets, err := s.listBudgets(ctx, householdID, monthStart, today)
	if err != nil {
		return models.BudgetMonth{}, err
	}

	// Refunds are summed alongside expenses and are positive, so a returned
	// purchase subtracts itself here rather than needing its own term.
	var spent, income float64
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(-SUM(amount) FILTER (WHERE kind IN ('expense','refund')), 0),
		        COALESCE(SUM(amount) FILTER (WHERE kind = 'income'), 0)
		 FROM transactions
		 WHERE household_id = $1
		   AND date >= $2::date
		   AND date < ($2::date + INTERVAL '1 month')`,
		householdID, monthStart).Scan(&spent, &income)
	if err != nil {
		return models.BudgetMonth{}, fmt.Errorf("totalling the month: %w", err)
	}

	// Each budget's actual is exactly its category's for the month, so whatever
	// is left over — other categories and uncategorized — is outside the plan.
	unbudgeted, unplanned := spent, income
	for _, b := range budgets {
		if b.Kind == models.KindIncome {
			unplanned -= b.Spent
		} else {
			unbudgeted -= b.Spent
		}
	}
	savings, err := s.savingsLines(ctx, householdID, monthStart)
	if err != nil {
		return models.BudgetMonth{}, err
	}
	var expected float64
	for _, b := range budgets {
		if b.Kind == models.KindIncome {
			expected += b.Amount
		}
	}
	thisMonth := today.Format("2006-01") + "-01"
	shortfall, err := s.savingsShortfallFor(ctx, householdID, monthStart, monthStart >= thisMonth, expected, savings)
	if err != nil {
		return models.BudgetMonth{}, err
	}
	return models.BudgetMonth{
		Month:            monthStart,
		Budgets:          budgets,
		UnbudgetedSpent:  round2(unbudgeted),
		IncomeReceived:   round2(income),
		UnplannedIncome:  round2(unplanned),
		Savings:          savings,
		SavingsShortfall: shortfall,
	}, nil
}

// savingsLines is each open goal with a monthly amount, and what went toward it
// in the month: money added to its account (goalMoney, from the day the goal
// started), or
// contributions for a goal with no account. A transfer counts toward the month
// and the goal alike, so "Add money" on the budgets page is one transfer.
func (s *Service) savingsLines(ctx context.Context, householdID int, monthStart string) ([]models.SavingsLine, error) {
	rows, err := s.db.QueryContext(ctx,
		`WITH g AS (`+goalSelect+` WHERE g.household_id = $1
		              AND g.monthly_amount IS NOT NULL AND g.achieved_at IS NULL)
		 SELECT g.id, g.name, g.account_id, g.account_name, g.monthly_amount, g.target_amount, g.saved,
		        CASE WHEN g.account_id IS NOT NULL THEN COALESCE((
		               SELECT SUM(t.amount) FROM transactions t
		               WHERE t.household_id = $1 AND t.account_id = g.account_id AND `+goalMoney+`
		                 AND t.date >= GREATEST(g.started_on, $2::date)
		                 AND t.date < ($2::date + INTERVAL '1 month')), 0)
		             ELSE COALESCE((
		               SELECT SUM(gc.amount) FROM goal_contributions gc
		               WHERE gc.goal_id = g.id
		                 AND gc.date >= $2::date AND gc.date < ($2::date + INTERVAL '1 month')), 0)
		        END
		 FROM g ORDER BY g.name`,
		householdID, monthStart)
	if err != nil {
		return nil, fmt.Errorf("loading savings lines: %w", err)
	}
	defer rows.Close()
	out := []models.SavingsLine{}
	for rows.Next() {
		var l models.SavingsLine
		var accountID sql.NullInt64
		var accountName sql.NullString
		if err := rows.Scan(&l.GoalID, &l.Name, &accountID, &accountName, &l.Monthly, &l.Target,
			&l.Saved, &l.Moved); err != nil {
			return nil, fmt.Errorf("scanning savings line: %w", err)
		}
		l.AccountID = intPtr(accountID)
		l.AccountName = strPtr(accountName)
		out = append(out, l)
	}
	return out, rows.Err()
}

// listBudgets finds, per category, the newest row whose effective_from is on or
// before the month — DISTINCT ON is Postgres's direct way to express that, and it
// keeps old budget rows around as history instead of overwriting them when an
// amount changes.
//
// archived_at is filtered *after* picking the newest row, not before: an archived
// row is a tombstone left by StopBudget, and it has to win over the older rows
// beneath it so the budget stays stopped from that month on.
func (s *Service) listBudgets(ctx context.Context, householdID int, monthStart string, today time.Time) ([]models.Budget, error) {
	rows, err := s.db.QueryContext(ctx,
		`WITH active AS (
		   SELECT DISTINCT ON (b.category_id)
		          b.id, b.category_id, b.period, b.amount, b.effective_from, b.archived_at
		   FROM budgets b
		   WHERE b.household_id = $1
		     AND b.effective_from <= $2::date
		   ORDER BY b.category_id, b.effective_from DESC
		 )
		 SELECT active.id, active.category_id, c.name, c.color, c.kind, active.period,
		        active.amount, active.effective_from,
		        COALESCE((
		          SELECT CASE WHEN c.kind = 'income' THEN SUM(t.amount) ELSE -SUM(t.amount) END
		          FROM transactions t
		          WHERE t.household_id = $1
		            AND t.category_id = active.category_id
		            AND (CASE WHEN c.kind = 'income' THEN t.kind = 'income'
		                      ELSE t.kind IN ('expense','refund') END)
		            AND t.date >= $2::date
		            AND t.date < ($2::date + INTERVAL '1 month')
		        ), 0) AS spent
		 FROM active
		 JOIN categories c ON c.id = active.category_id AND c.archived_at IS NULL
		 WHERE active.archived_at IS NULL
		 ORDER BY c.kind DESC, c.name`,
		householdID, monthStart)
	if err != nil {
		return nil, fmt.Errorf("listing budgets: %w", err)
	}
	defer rows.Close()

	out := []models.Budget{}
	for rows.Next() {
		var b models.Budget
		var color sql.NullString
		var effective time.Time
		if err := rows.Scan(&b.ID, &b.CategoryID, &b.CategoryName, &color, &b.Kind,
			&b.Period, &b.Amount, &effective, &b.Spent); err != nil {
			return nil, fmt.Errorf("scanning budget: %w", err)
		}
		b.CategoryColor = strPtr(color)
		b.EffectiveFrom = dateStr(effective)
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}

	scheduled, err := s.scheduledSpend(ctx, householdID, monthStart, today)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Scheduled = scheduled[out[i].CategoryID]
	}
	return out, nil
}

// scheduledSpend totals, per category, the recurring expenses — and recurring
// income, for expected-income budgets — that fall in the month but have not
// posted yet. Categories are one kind or the other, so one map holds both.
//
// Dates come from the rules through the date engine, not from
// recurring_occurrences. Those rows only exist out to the scheduler's horizon,
// vanish for a tick after a rule is edited, and linger for paused rules, so
// they would be wrong for exactly the months someone looks ahead to. What stops
// a date being counted twice is the same boundary the scheduler uses: anything
// on or before the rule's last posted/skipped occurrence is already history
// (and, if posted, already in Spent, since posted rows carry the due date).
//
// A due-but-unposted charge in the current month still counts — it is owed, the
// scheduler just hasn't reached it. Past months report nothing: an unposted
// charge there belongs to a manual rule nobody logged, and that's not a budget
// commitment any more.
func (s *Service) scheduledSpend(ctx context.Context, householdID int, monthStart string, today time.Time) (map[int]float64, error) {
	month, err := models.ParseDate(monthStart)
	if err != nil {
		return nil, err
	}
	if month.Before(time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)) {
		return nil, nil
	}
	monthEnd := month.AddDate(0, 1, -1)

	rules, err := s.ListRules(ctx, householdID)
	if err != nil {
		return nil, err
	}
	handled, err := s.lastHandledByRule(ctx, householdID)
	if err != nil {
		return nil, err
	}

	out := map[int]float64{}
	for _, rule := range rules {
		if rule.Paused || rule.CategoryID == nil ||
			(rule.Kind != models.KindExpense && rule.Kind != models.KindIncome) {
			continue
		}
		spec, err := RuleSpec(rule)
		if err != nil {
			return nil, err
		}
		from := month
		if last, ok := handled[rule.ID]; ok && !last.Before(from) {
			from = last.AddDate(0, 0, 1)
		}
		if n := len(spec.Occurrences(from, monthEnd, 0)); n > 0 {
			out[*rule.CategoryID] += float64(n) * rule.Amount
		}
	}
	for id, v := range out {
		out[id] = round2(v)
	}
	return out, nil
}

// lastHandledByRule is LastHandledOccurrence for every rule in a household at
// once. Rules with nothing posted or skipped are absent.
func (s *Service) lastHandledByRule(ctx context.Context, householdID int) (map[int]time.Time, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT o.rule_id, MAX(o.due_date)
		 FROM recurring_occurrences o
		 JOIN recurring_rules r ON r.id = o.rule_id
		 WHERE r.household_id = $1 AND o.status <> 'scheduled'
		 GROUP BY o.rule_id`, householdID)
	if err != nil {
		return nil, fmt.Errorf("reading rule history: %w", err)
	}
	defer rows.Close()

	out := map[int]time.Time{}
	for rows.Next() {
		var id int
		var last time.Time
		if err := rows.Scan(&id, &last); err != nil {
			return nil, fmt.Errorf("scanning rule history: %w", err)
		}
		out[id] = last
	}
	return out, rows.Err()
}

type BudgetInput struct {
	CategoryID    int     `json:"category_id"`
	Amount        float64 `json:"amount"`
	EffectiveFrom string  `json:"effective_from"`
}

func (s *Service) SetBudget(ctx context.Context, householdID int, in BudgetInput) (models.Budget, error) {
	if in.CategoryID <= 0 {
		return models.Budget{}, invalid("category_id is required")
	}
	if in.Amount <= 0 {
		return models.Budget{}, invalid("amount must be greater than zero")
	}
	if err := s.assertBudgetCategory(ctx, householdID, in.CategoryID); err != nil {
		return models.Budget{}, err
	}

	effective, today, err := s.resolveBudgetMonth(ctx, householdID, in.EffectiveFrom)
	if err != nil {
		return models.Budget{}, err
	}

	// Re-setting a budget for a month it already covers replaces that row rather
	// than stacking another one on the same date. Clearing archived_at restarts a
	// budget that was stopped in that same month.
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO budgets (household_id, category_id, period, amount, effective_from)
		 VALUES ($1, $2, 'monthly', $3, $4)
		 ON CONFLICT (household_id, category_id, effective_from)
		 DO UPDATE SET amount = EXCLUDED.amount, archived_at = NULL, updated_at = NOW()`,
		householdID, in.CategoryID, in.Amount, effective)
	if err != nil {
		return models.Budget{}, fmt.Errorf("saving budget: %w", err)
	}

	budgets, err := s.listBudgets(ctx, householdID, effective, today)
	if err != nil {
		return models.Budget{}, err
	}
	for _, b := range budgets {
		if b.CategoryID == in.CategoryID {
			return b, nil
		}
	}
	return models.Budget{}, ErrNotFound
}

// StopBudget ends a budget from `month` onward while leaving earlier months
// budgeted as they were.
//
// Deleting the row would be wrong twice over: the row in force is usually dated
// some earlier month, so deleting it rewrites that history, and any older row
// beneath it would silently come back into force. Instead it drops changes dated
// after the month and writes an archived tombstone at the month, which
// listBudgets treats as "no budget from here on".
func (s *Service) StopBudget(ctx context.Context, householdID, id int, month string) error {
	monthStart, _, err := s.resolveBudgetMonth(ctx, householdID, month)
	if err != nil {
		return err
	}

	return s.inTx(func(tx *sql.Tx) error {
		var categoryID int
		var amount float64
		err := tx.QueryRowContext(ctx,
			`SELECT category_id, amount FROM budgets WHERE household_id = $1 AND id = $2`,
			householdID, id).Scan(&categoryID, &amount)
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("loading budget: %w", err)
		}

		if _, err := tx.ExecContext(ctx,
			`DELETE FROM budgets
			 WHERE household_id = $1 AND category_id = $2 AND effective_from > $3::date`,
			householdID, categoryID, monthStart); err != nil {
			return fmt.Errorf("clearing later budget changes: %w", err)
		}

		// amount is carried over only because the column requires a positive one;
		// a tombstone's amount is never read.
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO budgets (household_id, category_id, period, amount, effective_from, archived_at)
			 VALUES ($1, $2, 'monthly', $3, $4, NOW())
			 ON CONFLICT (household_id, category_id, effective_from)
			 DO UPDATE SET archived_at = NOW(), updated_at = NOW()`,
			householdID, categoryID, amount, monthStart); err != nil {
			return fmt.Errorf("stopping budget: %w", err)
		}
		return nil
	})
}

// assertBudgetCategory checks a budget's category: the household's own, not
// retired, and income or expense — a limit on spending or income expected.
func (s *Service) assertBudgetCategory(ctx context.Context, householdID, categoryID int) error {
	var kind string
	var archived bool
	err := s.db.QueryRowContext(ctx,
		`SELECT kind, archived_at IS NOT NULL FROM categories WHERE id = $1 AND household_id = $2`,
		categoryID, householdID).Scan(&kind, &archived)
	if err == sql.ErrNoRows {
		return invalid("category %d does not exist", categoryID)
	}
	if err != nil {
		return fmt.Errorf("checking category: %w", err)
	}
	if kind != models.KindExpense && kind != models.KindIncome {
		return invalid("budgets can only be set on income or expense categories")
	}
	if archived {
		return invalid("category is archived")
	}
	return nil
}

// resolveBudgetMonth resolves a requested month against the household's own
// calendar, so "this month" flips at midnight where the family lives rather than
// at midnight UTC. It also returns that "today", which scheduled spend needs.
func (s *Service) resolveBudgetMonth(ctx context.Context, householdID int, month string) (string, time.Time, error) {
	household, err := s.household(ctx, householdID)
	if err != nil {
		return "", time.Time{}, err
	}
	today := household.Today()
	monthStart, err := monthStartOf(month, today)
	return monthStart, today, err
}

// monthStartOf normalizes a date or "YYYY-MM" to the first of that month.
// An empty string means the month containing today.
func monthStartOf(s string, today time.Time) (string, error) {
	if s == "" {
		return time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC).Format(models.DateOnly), nil
	}
	if len(s) == 7 {
		s += "-01"
	}
	t, err := models.ParseDate(s)
	if err != nil {
		return "", invalid("month must be YYYY-MM or YYYY-MM-DD")
	}
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC).Format(models.DateOnly), nil
}
