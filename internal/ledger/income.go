package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Expected income is one amount for the whole household each month. Every
// income transaction counts toward it, whatever its category: the plan is about
// how much comes in, not which bucket it's filed under.

// incomePlan is the expected income in force for a month (income_plans, newest
// row on or before it), 0 when there is none or it was stopped.
func (s *Service) incomePlan(ctx context.Context, householdID int, monthStart string) (float64, error) {
	var amount sql.NullFloat64
	err := s.db.QueryRowContext(ctx,
		`SELECT amount FROM income_plans WHERE household_id = $1 AND effective_from <= $2::date
		 ORDER BY effective_from DESC LIMIT 1`, householdID, monthStart).Scan(&amount)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("reading expected income: %w", err)
	}
	return amount.Float64, nil
}

type IncomePlanInput struct {
	// Amount is the income expected each month; nil or 0 stops the plan.
	Amount        *float64 `json:"amount"`
	EffectiveFrom string   `json:"effective_from"`
}

// SetIncomePlan sets the expected monthly income from a month on, the way
// SetDebtPlan does: saving what is already in force writes nothing, and a real
// change drops any later ones, since it's meant "from here on".
func (s *Service) SetIncomePlan(ctx context.Context, householdID int, in IncomePlanInput) error {
	if in.Amount != nil && *in.Amount < 0 {
		return invalid("amount can't be negative")
	}
	if in.Amount != nil && round2(*in.Amount) == 0 {
		in.Amount = nil
	}
	from, _, err := s.resolveBudgetMonth(ctx, householdID, in.EffectiveFrom)
	if err != nil {
		return err
	}

	return s.inTx(func(tx *sql.Tx) error {
		var current sql.NullFloat64
		err := tx.QueryRowContext(ctx,
			`SELECT amount FROM income_plans WHERE household_id = $1 AND effective_from <= $2::date
			 ORDER BY effective_from DESC LIMIT 1`, householdID, from).Scan(&current)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("reading expected income: %w", err)
		}
		if (in.Amount == nil && !current.Valid) ||
			(in.Amount != nil && current.Valid && round2(*in.Amount) == round2(current.Float64)) {
			return nil
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM income_plans WHERE household_id = $1 AND effective_from > $2::date`,
			householdID, from); err != nil {
			return fmt.Errorf("clearing later expected income: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO income_plans (household_id, effective_from, amount)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (household_id, effective_from) DO UPDATE SET amount = EXCLUDED.amount`,
			householdID, from, nullFloat(in.Amount)); err != nil {
			return fmt.Errorf("saving expected income: %w", err)
		}
		return nil
	})
}
