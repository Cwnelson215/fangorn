package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/cwnelson/fangorn/internal/models"
)

func (s *Service) ListCategories(ctx context.Context, householdID int, includeArchived bool) ([]models.Category, error) {
	q := `SELECT id, name, kind, color, parent_id, archived_at IS NOT NULL
	      FROM categories WHERE household_id = $1`
	if !includeArchived {
		q += ` AND archived_at IS NULL`
	}
	q += ` ORDER BY kind DESC, name`

	rows, err := s.db.QueryContext(ctx, q, householdID)
	if err != nil {
		return nil, fmt.Errorf("listing categories: %w", err)
	}
	defer rows.Close()

	out := []models.Category{}
	for rows.Next() {
		var c models.Category
		var color sql.NullString
		var parent sql.NullInt64
		if err := rows.Scan(&c.ID, &c.Name, &c.Kind, &color, &parent, &c.Archived); err != nil {
			return nil, fmt.Errorf("scanning category: %w", err)
		}
		c.Color = strPtr(color)
		c.ParentID = intPtr(parent)
		out = append(out, c)
	}
	return out, rows.Err()
}

type CategoryInput struct {
	Name     string  `json:"name"`
	Kind     string  `json:"kind"`
	Color    *string `json:"color"`
	ParentID *int    `json:"parent_id"`
}

func (in *CategoryInput) normalize() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return invalid("name is required")
	}
	if in.Kind != models.KindIncome && in.Kind != models.KindExpense {
		return invalid("kind must be %q or %q", models.KindIncome, models.KindExpense)
	}
	return nil
}

func (s *Service) CreateCategory(ctx context.Context, householdID int, in CategoryInput) (models.Category, error) {
	if err := in.normalize(); err != nil {
		return models.Category{}, err
	}

	var c models.Category
	var color sql.NullString
	var parent sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO categories (household_id, name, kind, color, parent_id)
		 VALUES ($1,$2,$3,$4,$5)
		 RETURNING id, name, kind, color, parent_id, false`,
		householdID, in.Name, in.Kind, nullStr(in.Color), nullInt(in.ParentID),
	).Scan(&c.ID, &c.Name, &c.Kind, &color, &parent, &c.Archived)
	if err != nil {
		if isUniqueViolation(err) {
			return c, invalid("a category named %q already exists", in.Name)
		}
		return c, fmt.Errorf("creating category: %w", err)
	}
	c.Color = strPtr(color)
	c.ParentID = intPtr(parent)
	return c, nil
}

func (s *Service) UpdateCategory(ctx context.Context, householdID, id int, in CategoryInput) (models.Category, error) {
	if err := in.normalize(); err != nil {
		return models.Category{}, err
	}

	var c models.Category
	var color sql.NullString
	var parent sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`UPDATE categories SET name = $1, kind = $2, color = $3, parent_id = $4
		 WHERE household_id = $5 AND id = $6
		 RETURNING id, name, kind, color, parent_id, archived_at IS NOT NULL`,
		in.Name, in.Kind, nullStr(in.Color), nullInt(in.ParentID), householdID, id,
	).Scan(&c.ID, &c.Name, &c.Kind, &color, &parent, &c.Archived)
	if err == sql.ErrNoRows {
		return c, ErrNotFound
	}
	if err != nil {
		if isUniqueViolation(err) {
			return c, invalid("a category named %q already exists", in.Name)
		}
		return c, fmt.Errorf("updating category: %w", err)
	}
	c.Color = strPtr(color)
	c.ParentID = intPtr(parent)
	return c, nil
}

// DeleteCategory archives rather than deletes when the category is in use, so
// historical transactions keep their labels. An unused category is removed
// outright.
func (s *Service) DeleteCategory(ctx context.Context, householdID, id int) error {
	var inUse bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM transactions WHERE category_id = $1)
		     OR EXISTS (SELECT 1 FROM recurring_rules WHERE category_id = $1)
		     OR EXISTS (SELECT 1 FROM budgets WHERE category_id = $1)`, id).Scan(&inUse)
	if err != nil {
		return fmt.Errorf("checking category usage: %w", err)
	}

	q := `DELETE FROM categories WHERE household_id = $1 AND id = $2`
	if inUse {
		q = `UPDATE categories SET archived_at = NOW() WHERE household_id = $1 AND id = $2`
	}

	res, err := s.db.ExecContext(ctx, q, householdID, id)
	if err != nil {
		return fmt.Errorf("removing category: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
