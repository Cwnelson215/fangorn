package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cwnelson/fangorn/internal/models"
)

// goalMoney is which of a goal account's transactions count as money added:
// transfers in and out, and income deposited there directly — but not interest
// or money market dividends (source 'interest'), which the account earned by
// itself, and not market growth, which isn't a transaction at all.
const goalMoney = `(t.kind = 'transfer' OR (t.kind = 'income' AND t.source <> 'interest'))`

// A goal's target is how much to ADD, not a balance to reach. There are two
// kinds: a long-term goal counts from the day it started, and a monthly goal
// (goals.month set) counts only inside its month. Progress comes from one of two
// places depending on how the goal is set up:
//
//   - Linked to an account: the money added to that account in the goal's
//     window (goalMoney) — transfers in less transfers out, plus income
//     deposited there. Distributing income from the income account to savings
//     is a transfer, so the goal tracks itself.
//   - Unlinked: the sum of explicit contributions, for goals spread across
//     accounts or held partly in cash.
//
// A long-term goal's monthly_amount is its latest plan row: the share it takes
// from each month's budget from monthly_from on (see goal_plans).
const goalSelect = `
	SELECT g.id, g.name, g.target_amount, g.target_date, g.account_id, a.name AS account_name,
	       g.notes, g.achieved_at IS NOT NULL AS achieved,
	       CASE
	         WHEN g.account_id IS NOT NULL
	           THEN COALESCE((
	                  SELECT SUM(t.amount) FROM transactions t
	                  WHERE t.household_id = g.household_id AND t.account_id = g.account_id
	                    AND ` + goalMoney + `
	                    AND t.date >= COALESCE(g.month, g.started_on)
	                    AND (g.month IS NULL OR t.date < g.month + INTERVAL '1 month')
	                ), 0)
	         ELSE COALESCE((
	                SELECT SUM(gc.amount) FROM goal_contributions gc
	                WHERE gc.goal_id = g.id
	                  AND (g.month IS NULL
	                       OR (gc.date >= g.month AND gc.date < g.month + INTERVAL '1 month'))
	              ), 0)
	       END AS saved,
	       g.started_on, p.amount AS monthly_amount, p.effective_from AS monthly_from, g.month
	FROM goals g
	LEFT JOIN accounts a ON a.id = g.account_id
	LEFT JOIN LATERAL (
	  SELECT gp.amount, gp.effective_from FROM goal_plans gp
	  WHERE gp.goal_id = g.id ORDER BY gp.effective_from DESC LIMIT 1
	) p ON true`

func scanGoal(rows interface{ Scan(...any) error }) (models.Goal, error) {
	var g models.Goal
	var accountName, notes sql.NullString
	var accountID sql.NullInt64
	var targetDate sql.NullTime
	var startedOn time.Time
	var monthly sql.NullFloat64
	var monthlyFrom, month sql.NullTime

	err := rows.Scan(&g.ID, &g.Name, &g.TargetAmount, &targetDate, &accountID,
		&accountName, &notes, &g.Achieved, &g.Saved, &startedOn, &monthly, &monthlyFrom, &month)
	if err != nil {
		return g, err
	}
	g.StartedOn = dateStr(startedOn)
	g.Month = dateStrPtr(month)
	// A stop row leaves the latest plan with no amount: no monthly share.
	if monthly.Valid {
		m := monthly.Float64
		g.MonthlyAmount = &m
		g.MonthlyFrom = dateStrPtr(monthlyFrom)
	}
	g.TargetDate = dateStrPtr(targetDate)
	g.AccountID = intPtr(accountID)
	g.AccountName = strPtr(accountName)
	g.Notes = strPtr(notes)
	return g, nil
}

// ListGoals returns the long-term goals. A monthly goal lives in its month's
// budget (BudgetMonth.Savings), not here.
func (s *Service) ListGoals(ctx context.Context, householdID int) ([]models.Goal, error) {
	rows, err := s.db.QueryContext(ctx,
		goalSelect+` WHERE g.household_id = $1 AND g.month IS NULL
		 ORDER BY g.achieved_at IS NOT NULL, g.target_date NULLS LAST, g.name`,
		householdID)
	if err != nil {
		return nil, fmt.Errorf("listing goals: %w", err)
	}
	defer rows.Close()

	out := []models.Goal{}
	for rows.Next() {
		g, err := scanGoal(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning goal: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Service) GetGoal(ctx context.Context, householdID, id int) (models.Goal, error) {
	row := s.db.QueryRowContext(ctx,
		goalSelect+` WHERE g.household_id = $1 AND g.id = $2`, householdID, id)

	g, err := scanGoal(row)
	if err == sql.ErrNoRows {
		return g, ErrNotFound
	}
	if err != nil {
		return g, fmt.Errorf("fetching goal: %w", err)
	}
	return g, nil
}

type GoalInput struct {
	Name         string  `json:"name"`
	TargetAmount float64 `json:"target_amount"`
	TargetDate   *string `json:"target_date"`
	AccountID    *int    `json:"account_id"`
	Notes        *string `json:"notes"`
	// Month ("YYYY-MM" or a date in it) makes a monthly goal for that month
	// only; nil for a long-term goal. A goal's kind can't change once created.
	Month *string `json:"month"`
	// MonthlyAmount is a long-term goal's monthly share of the budget, applying
	// from MonthlyFrom (default: this month) until changed; nil for none.
	MonthlyAmount *float64 `json:"monthly_amount"`
	MonthlyFrom   *string  `json:"monthly_from"`
}

func (in *GoalInput) normalize() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return invalid("name is required")
	}
	if in.TargetAmount <= 0 {
		return invalid("target_amount must be greater than zero")
	}
	if in.MonthlyAmount != nil && *in.MonthlyAmount <= 0 {
		in.MonthlyAmount = nil
	}
	in.Month = trimmedOrNil(in.Month)
	if in.Month != nil && in.MonthlyAmount != nil {
		return invalid("a monthly goal is its month's amount; it has no monthly share of its own")
	}
	if in.TargetDate != nil && *in.TargetDate != "" {
		if _, err := models.ParseDate(*in.TargetDate); err != nil {
			return invalid("target_date must be YYYY-MM-DD")
		}
	} else {
		in.TargetDate = nil
	}
	return nil
}

func (s *Service) CreateGoal(ctx context.Context, householdID int, in GoalInput) (models.Goal, error) {
	if err := in.normalize(); err != nil {
		return models.Goal{}, err
	}
	if in.AccountID != nil {
		if err := s.assertAccount(ctx, householdID, *in.AccountID); err != nil {
			return models.Goal{}, err
		}
	}

	// A goal counts what's added from today, in the household's calendar.
	household, err := s.household(ctx, householdID)
	if err != nil {
		return models.Goal{}, err
	}
	today := household.Today()
	month, from, err := goalMonths(in, today)
	if err != nil {
		return models.Goal{}, err
	}

	var id int
	err = s.inTx(func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx,
			`INSERT INTO goals (household_id, name, target_amount, target_date, account_id, notes,
			                    started_on, month)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
			householdID, in.Name, in.TargetAmount, nullStr(in.TargetDate),
			nullInt(in.AccountID), nullStr(in.Notes), today.Format(models.DateOnly), nullStr(month),
		).Scan(&id); err != nil {
			return fmt.Errorf("creating goal: %w", err)
		}
		if month == nil {
			return setGoalPlan(ctx, tx, id, from, in.MonthlyAmount)
		}
		return nil
	})
	if err != nil {
		return models.Goal{}, err
	}
	return s.GetGoal(ctx, householdID, id)
}

func (s *Service) UpdateGoal(ctx context.Context, householdID, id int, in GoalInput) (models.Goal, error) {
	if err := in.normalize(); err != nil {
		return models.Goal{}, err
	}
	if in.AccountID != nil {
		if err := s.assertAccount(ctx, householdID, *in.AccountID); err != nil {
			return models.Goal{}, err
		}
	}

	household, err := s.household(ctx, householdID)
	if err != nil {
		return models.Goal{}, err
	}
	month, from, err := goalMonths(in, household.Today())
	if err != nil {
		return models.Goal{}, err
	}

	err = s.inTx(func(tx *sql.Tx) error {
		var current sql.NullTime
		err := tx.QueryRowContext(ctx,
			`SELECT month FROM goals WHERE household_id = $1 AND id = $2 FOR UPDATE`,
			householdID, id).Scan(&current)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("loading goal: %w", err)
		}
		if current.Valid != (month != nil) {
			return invalid("a goal can't switch between long-term and a single month; make a new one instead")
		}

		if _, err := tx.ExecContext(ctx,
			`UPDATE goals SET name = $1, target_amount = $2, target_date = $3,
			        account_id = $4, notes = $5, month = $6, updated_at = NOW()
			 WHERE household_id = $7 AND id = $8`,
			in.Name, in.TargetAmount, nullStr(in.TargetDate), nullInt(in.AccountID),
			nullStr(in.Notes), nullStr(month), householdID, id); err != nil {
			return fmt.Errorf("updating goal: %w", err)
		}
		if month == nil {
			return setGoalPlan(ctx, tx, id, from, in.MonthlyAmount)
		}
		return nil
	})
	if err != nil {
		return models.Goal{}, err
	}
	return s.GetGoal(ctx, householdID, id)
}

// goalMonths resolves a goal input's months against the household's today: the
// month a monthly goal belongs to (nil for long-term), and the month a
// long-term goal's plan change applies from (default: this month).
func goalMonths(in GoalInput, today time.Time) (month *string, from string, err error) {
	if in.Month != nil {
		m, err := monthStartOf(*in.Month, today)
		if err != nil {
			return nil, "", err
		}
		return &m, "", nil
	}
	f := ""
	if in.MonthlyFrom != nil {
		f = *in.MonthlyFrom
	}
	from, err = monthStartOf(f, today)
	return nil, from, err
}

// setGoalPlan sets a long-term goal's monthly share from a month on, the way a
// budget change applies: from that month until the next change. Setting what's
// already in force writes nothing — so saving a goal to rename it doesn't
// disturb its plan — and a real change drops any later ones, since it's meant
// "from here on". A nil amount stops the monthly share from that month.
func setGoalPlan(ctx context.Context, tx *sql.Tx, goalID int, from string, amount *float64) error {
	var current sql.NullFloat64
	err := tx.QueryRowContext(ctx,
		`SELECT amount FROM goal_plans WHERE goal_id = $1 AND effective_from <= $2::date
		 ORDER BY effective_from DESC LIMIT 1`, goalID, from).Scan(&current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("reading goal plan: %w", err)
	}
	if (amount == nil && !current.Valid) || (amount != nil && current.Valid && round2(*amount) == round2(current.Float64)) {
		return nil
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM goal_plans WHERE goal_id = $1 AND effective_from > $2::date`, goalID, from); err != nil {
		return fmt.Errorf("clearing later plan changes: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO goal_plans (goal_id, effective_from, amount) VALUES ($1, $2, $3)
		 ON CONFLICT (goal_id, effective_from) DO UPDATE SET amount = EXCLUDED.amount`,
		goalID, from, nullFloat(amount)); err != nil {
		return fmt.Errorf("saving goal plan: %w", err)
	}
	return nil
}

func (s *Service) SetGoalAchieved(ctx context.Context, householdID, id int, achieved bool) error {
	q := `UPDATE goals SET achieved_at = NULL, updated_at = NOW() WHERE household_id = $1 AND id = $2`
	if achieved {
		q = `UPDATE goals SET achieved_at = NOW(), updated_at = NOW() WHERE household_id = $1 AND id = $2`
	}
	res, err := s.db.ExecContext(ctx, q, householdID, id)
	if err != nil {
		return fmt.Errorf("updating goal: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) DeleteGoal(ctx context.Context, householdID, id int) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM goals WHERE household_id = $1 AND id = $2`, householdID, id)
	if err != nil {
		return fmt.Errorf("deleting goal: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

type ContributionInput struct {
	Date   string  `json:"date"`
	Amount float64 `json:"amount"`
	Note   *string `json:"note"`
}

// AddContribution records manual progress toward an unlinked goal.
func (s *Service) AddContribution(ctx context.Context, householdID, goalID int, in ContributionInput) (models.Goal, error) {
	goal, err := s.GetGoal(ctx, householdID, goalID)
	if err != nil {
		return models.Goal{}, err
	}
	if goal.AccountID != nil {
		return models.Goal{}, invalid("this goal counts transfers into its account; move the money with a transfer instead")
	}
	if in.Amount == 0 {
		return models.Goal{}, invalid("amount must not be zero")
	}
	if in.Date == "" {
		in.Date = time.Now().Format(models.DateOnly)
	}
	if _, err := models.ParseDate(in.Date); err != nil {
		return models.Goal{}, invalid("date must be YYYY-MM-DD")
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO goal_contributions (goal_id, date, amount, note) VALUES ($1,$2,$3,$4)`,
		goalID, in.Date, in.Amount, nullStr(in.Note))
	if err != nil {
		return models.Goal{}, fmt.Errorf("recording contribution: %w", err)
	}
	return s.GetGoal(ctx, householdID, goalID)
}
