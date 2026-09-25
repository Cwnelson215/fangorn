package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/cwnelson/fangorn/internal/models"
)

// categoryCols is what every category read returns, in scanCategory's order.
const categoryCols = `id, name, kind, color, parent_id, default_account_id, archived_at IS NOT NULL`

func scanCategory(row interface{ Scan(...any) error }) (models.Category, error) {
	var c models.Category
	var color sql.NullString
	var parent, account sql.NullInt64
	if err := row.Scan(&c.ID, &c.Name, &c.Kind, &color, &parent, &account, &c.Archived); err != nil {
		return c, err
	}
	c.Color = strPtr(color)
	c.ParentID = intPtr(parent)
	c.DefaultAccountID = intPtr(account)
	return c, nil
}

func (s *Service) ListCategories(ctx context.Context, householdID int, includeArchived bool) ([]models.Category, error) {
	q := `SELECT ` + categoryCols + ` FROM categories WHERE household_id = $1`
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
		c, err := scanCategory(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning category: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type CategoryInput struct {
	Name     string  `json:"name"`
	Kind     string  `json:"kind"`
	Color    *string `json:"color"`
	ParentID *int    `json:"parent_id"`
	// DefaultAccountID routes the category's spending to one account; nil for none.
	DefaultAccountID *int `json:"default_account_id"`
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

// checkDefaultAccount keeps a category from routing spending to another
// household's account, or to one that can't take new entries.
func (s *Service) checkDefaultAccount(ctx context.Context, householdID int, in CategoryInput) error {
	if in.DefaultAccountID == nil {
		return nil
	}
	var archived bool
	err := s.db.QueryRowContext(ctx,
		`SELECT archived_at IS NOT NULL FROM accounts WHERE id = $1 AND household_id = $2`,
		*in.DefaultAccountID, householdID).Scan(&archived)
	if errors.Is(err, sql.ErrNoRows) {
		return invalid("account %d does not exist", *in.DefaultAccountID)
	}
	if err != nil {
		return fmt.Errorf("checking category account: %w", err)
	}
	if archived {
		return invalid("that account is archived")
	}
	return nil
}

func (s *Service) CreateCategory(ctx context.Context, householdID int, in CategoryInput) (models.Category, error) {
	if err := in.normalize(); err != nil {
		return models.Category{}, err
	}
	if err := s.checkDefaultAccount(ctx, householdID, in); err != nil {
		return models.Category{}, err
	}

	c, err := scanCategory(s.db.QueryRowContext(ctx,
		`INSERT INTO categories (household_id, name, kind, color, parent_id, default_account_id)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 RETURNING `+categoryCols,
		householdID, in.Name, in.Kind, nullStr(in.Color), nullInt(in.ParentID), nullInt(in.DefaultAccountID),
	))
	if err != nil {
		if isUniqueViolation(err) {
			return c, invalid("a category named %q already exists", in.Name)
		}
		return c, fmt.Errorf("creating category: %w", err)
	}
	return c, nil
}

func (s *Service) UpdateCategory(ctx context.Context, householdID, id int, in CategoryInput) (models.Category, error) {
	if err := in.normalize(); err != nil {
		return models.Category{}, err
	}
	if err := s.checkDefaultAccount(ctx, householdID, in); err != nil {
		return models.Category{}, err
	}

	c, err := scanCategory(s.db.QueryRowContext(ctx,
		`UPDATE categories SET name = $1, kind = $2, color = $3, parent_id = $4, default_account_id = $5
		 WHERE household_id = $6 AND id = $7
		 RETURNING `+categoryCols,
		in.Name, in.Kind, nullStr(in.Color), nullInt(in.ParentID), nullInt(in.DefaultAccountID), householdID, id,
	))
	if err == sql.ErrNoRows {
		return c, ErrNotFound
	}
	if err != nil {
		if isUniqueViolation(err) {
			return c, invalid("a category named %q already exists", in.Name)
		}
		return c, fmt.Errorf("updating category: %w", err)
	}
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

// UnarchiveCategory undoes the archive that DeleteCategory falls back to. Without
// it an archived name is unusable for good: it is hidden from the pickers, yet
// the unique index still rejects creating a new category with that name.
func (s *Service) UnarchiveCategory(ctx context.Context, householdID, id int) (models.Category, error) {
	c, err := scanCategory(s.db.QueryRowContext(ctx,
		`UPDATE categories SET archived_at = NULL
		 WHERE household_id = $1 AND id = $2
		 RETURNING `+categoryCols,
		householdID, id,
	))
	if err == sql.ErrNoRows {
		return c, ErrNotFound
	}
	if err != nil {
		return c, fmt.Errorf("restoring category: %w", err)
	}
	return c, nil
}
