package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Settings are the household-wide choices the app works from.
type Settings struct {
	// IncomeAccountID is where income is logged by default — the account money
	// is distributed from to savings goals. Nil until chosen.
	IncomeAccountID *int `json:"income_account_id"`
}

func (s *Service) GetSettings(ctx context.Context, householdID int) (Settings, error) {
	var out Settings
	var income sql.NullInt64
	// An archived income account no longer counts as chosen.
	err := s.db.QueryRowContext(ctx,
		`SELECT a.id FROM households h
		 LEFT JOIN accounts a ON a.id = h.income_account_id AND a.household_id = h.id
		   AND a.archived_at IS NULL
		 WHERE h.id = $1`, householdID).Scan(&income)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, fmt.Errorf("loading settings: %w", err)
	}
	out.IncomeAccountID = intPtr(income)
	return out, nil
}

// UpdateSettings replaces the household's settings. An income account has to
// be one of the household's open accounts.
func (s *Service) UpdateSettings(ctx context.Context, householdID int, in Settings) (Settings, error) {
	if in.IncomeAccountID != nil {
		var archived bool
		err := s.db.QueryRowContext(ctx,
			`SELECT archived_at IS NOT NULL FROM accounts WHERE id = $1 AND household_id = $2`,
			*in.IncomeAccountID, householdID).Scan(&archived)
		if errors.Is(err, sql.ErrNoRows) {
			return Settings{}, invalid("account %d does not exist", *in.IncomeAccountID)
		}
		if err != nil {
			return Settings{}, fmt.Errorf("checking income account: %w", err)
		}
		if archived {
			return Settings{}, invalid("that account is archived")
		}
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE households SET income_account_id = $1 WHERE id = $2`,
		nullInt(in.IncomeAccountID), householdID); err != nil {
		return Settings{}, fmt.Errorf("saving settings: %w", err)
	}
	return s.GetSettings(ctx, householdID)
}
