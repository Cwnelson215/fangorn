package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/cwnelson/fangorn/internal/models"
)

// ListBudgets returns the budget in force for the month containing `month`,
// along with that month's spend against each category.
//
// "In force" means the newest row whose effective_from is on or before the month
// — DISTINCT ON is Postgres's direct way to express that, and it keeps old budget
// rows around as history instead of overwriting them when an amount changes.
func (s *Service) ListBudgets(ctx context.Context, householdID int, month string) ([]models.Budget, error) {
	monthStart, err := monthStartOf(month)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx,
		`WITH active AS (
		   SELECT DISTINCT ON (b.category_id)
		          b.id, b.category_id, b.period, b.amount, b.effective_from
		   FROM budgets b
		   WHERE b.household_id = $1
		     AND b.archived_at IS NULL
		     AND b.effective_from <= $2::date
		   ORDER BY b.category_id, b.effective_from DESC
		 )
		 SELECT active.id, active.category_id, c.name, c.color, active.period,
		        active.amount, active.effective_from,
		        COALESCE((
		          SELECT -SUM(t.amount) FROM transactions t
		          WHERE t.household_id = $1
		            AND t.category_id = active.category_id
		            AND t.kind = 'expense'
		            AND t.date >= $2::date
		            AND t.date < ($2::date + INTERVAL '1 month')
		        ), 0) AS spent
		 FROM active
		 JOIN categories c ON c.id = active.category_id
		 ORDER BY c.name`,
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
		if err := rows.Scan(&b.ID, &b.CategoryID, &b.CategoryName, &color,
			&b.Period, &b.Amount, &effective, &b.Spent); err != nil {
			return nil, fmt.Errorf("scanning budget: %w", err)
		}
		b.CategoryColor = strPtr(color)
		b.EffectiveFrom = dateStr(effective)
		out = append(out, b)
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
	if err := s.assertCategory(ctx, householdID, &in.CategoryID); err != nil {
		return models.Budget{}, err
	}

	effective, err := monthStartOf(in.EffectiveFrom)
	if err != nil {
		return models.Budget{}, err
	}

	// Re-setting a budget for a month it already covers replaces that row rather
	// than stacking another one on the same date.
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO budgets (household_id, category_id, period, amount, effective_from)
		 VALUES ($1, $2, 'monthly', $3, $4)
		 ON CONFLICT (household_id, category_id, effective_from)
		 DO UPDATE SET amount = EXCLUDED.amount, archived_at = NULL, updated_at = NOW()`,
		householdID, in.CategoryID, in.Amount, effective)
	if err != nil {
		return models.Budget{}, fmt.Errorf("saving budget: %w", err)
	}

	budgets, err := s.ListBudgets(ctx, householdID, effective)
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

func (s *Service) DeleteBudget(ctx context.Context, householdID, id int) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM budgets WHERE household_id = $1 AND id = $2`, householdID, id)
	if err != nil {
		return fmt.Errorf("deleting budget: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// monthStartOf normalizes a date or "YYYY-MM" to the first of that month.
// An empty string means the current month.
func monthStartOf(s string) (string, error) {
	if s == "" {
		now := time.Now()
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format(models.DateOnly), nil
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
