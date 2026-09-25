package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/cwnelson/fangorn/internal/models"
)

// A goal's target is how much to ADD, not a balance to reach. Progress comes
// from one of two places depending on how the goal is set up:
//
//   - Linked to an account: the money moved into that account since the goal
//     started — transfers in less transfers out. Distributing income from the
//     income account to savings is exactly that, so the goal tracks itself.
//     Interest and market growth aren't money added, so they don't count.
//   - Unlinked: the sum of explicit contributions, for goals spread across
//     accounts or held partly in cash.
const goalSelect = `
	SELECT g.id, g.name, g.target_amount, g.target_date, g.account_id, a.name AS account_name,
	       g.notes, g.achieved_at IS NOT NULL AS achieved,
	       CASE
	         WHEN g.account_id IS NOT NULL
	           THEN COALESCE((
	                  SELECT SUM(t.amount) FROM transactions t
	                  WHERE t.household_id = g.household_id AND t.account_id = g.account_id
	                    AND t.kind = 'transfer' AND t.date >= g.started_on
	                ), 0)
	         ELSE COALESCE((
	                SELECT SUM(gc.amount) FROM goal_contributions gc WHERE gc.goal_id = g.id
	              ), 0)
	       END AS saved,
	       g.started_on, g.monthly_amount
	FROM goals g
	LEFT JOIN accounts a ON a.id = g.account_id`

func scanGoal(rows interface{ Scan(...any) error }) (models.Goal, error) {
	var g models.Goal
	var accountName, notes sql.NullString
	var accountID sql.NullInt64
	var targetDate sql.NullTime
	var startedOn time.Time
	var monthly sql.NullFloat64

	err := rows.Scan(&g.ID, &g.Name, &g.TargetAmount, &targetDate, &accountID,
		&accountName, &notes, &g.Achieved, &g.Saved, &startedOn, &monthly)
	if err != nil {
		return g, err
	}
	g.StartedOn = dateStr(startedOn)
	if monthly.Valid {
		m := monthly.Float64
		g.MonthlyAmount = &m
	}
	g.TargetDate = dateStrPtr(targetDate)
	g.AccountID = intPtr(accountID)
	g.AccountName = strPtr(accountName)
	g.Notes = strPtr(notes)
	return g, nil
}

func (s *Service) ListGoals(ctx context.Context, householdID int) ([]models.Goal, error) {
	rows, err := s.db.QueryContext(ctx,
		goalSelect+` WHERE g.household_id = $1
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
	// MonthlyAmount puts the goal in the monthly budget; nil for none.
	MonthlyAmount *float64 `json:"monthly_amount"`
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

	var id int
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO goals (household_id, name, target_amount, target_date, account_id, notes,
		                    started_on, monthly_amount)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		householdID, in.Name, in.TargetAmount, nullStr(in.TargetDate),
		nullInt(in.AccountID), nullStr(in.Notes),
		household.Today().Format(models.DateOnly), nullFloat(in.MonthlyAmount),
	).Scan(&id)
	if err != nil {
		return models.Goal{}, fmt.Errorf("creating goal: %w", err)
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

	res, err := s.db.ExecContext(ctx,
		`UPDATE goals SET name = $1, target_amount = $2, target_date = $3,
		        account_id = $4, notes = $5, monthly_amount = $6, updated_at = NOW()
		 WHERE household_id = $7 AND id = $8`,
		in.Name, in.TargetAmount, nullStr(in.TargetDate), nullInt(in.AccountID),
		nullStr(in.Notes), nullFloat(in.MonthlyAmount), householdID, id)
	if err != nil {
		return models.Goal{}, fmt.Errorf("updating goal: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.Goal{}, ErrNotFound
	}
	return s.GetGoal(ctx, householdID, id)
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
