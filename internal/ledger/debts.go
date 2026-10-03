package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"github.com/cwnelson/fangorn/internal/models"
)

// A debt payment is a transfer into a credit card or loan from an account that
// isn't one. It stays a transfer — balances and net worth are unchanged, and it
// is neither income nor spending — but what it pays down beyond the month's new
// charges counts toward the month's budget.
//
// debtPaymentGroup is true for either leg of such a transfer, written against
// the transactions alias t. Moving a balance from one card to another pays
// nothing down, so a transfer out of a debt account doesn't count; one whose
// source account was deleted still does.
const debtPaymentGroup = `(t.kind = 'transfer'
	AND EXISTS (
	  SELECT 1 FROM transactions o JOIN accounts oa ON oa.id = o.account_id
	  WHERE o.transfer_group_id = t.transfer_group_id AND o.amount > 0 AND oa.class = 'liability')
	AND NOT EXISTS (
	  SELECT 1 FROM transactions o JOIN accounts oa ON oa.id = o.account_id
	  WHERE o.transfer_group_id = t.transfer_group_id AND o.amount < 0 AND oa.class = 'liability'))`

// paydown is how much of a month's payments went to debt that was already
// there: what was paid beyond what was newly charged. Purchases on a card count
// as spending in their own categories when they are logged, so paying those off
// isn't counted a second time. Refunds outweighing purchases don't turn into a
// payment.
func paydown(paid, charged float64) float64 {
	return round2(math.Max(0, paid-math.Max(0, charged)))
}

// debtLines is the month's debt: every open credit card and loan (and a closed
// one that still had activity that month) with what was paid to it, what was
// newly charged, and the planned paydown in force (debt_plans, newest row on or
// before the month).
func (s *Service) debtLines(ctx context.Context, householdID int, monthStart string) ([]models.DebtLine, error) {
	accounts, err := s.ListAccounts(ctx, householdID, true)
	if err != nil {
		return nil, err
	}

	type month struct{ paid, charged float64 }
	months := map[int]month{}
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.account_id,
		        COALESCE(SUM(t.amount) FILTER (WHERE `+debtPaymentGroup+`), 0),
		        COALESCE(-SUM(t.amount) FILTER (WHERE t.kind <> 'adjustment' AND NOT `+debtPaymentGroup+`), 0)
		 FROM transactions t
		 JOIN accounts a ON a.id = t.account_id
		 WHERE t.household_id = $1 AND a.class = 'liability'
		   AND t.date >= $2::date AND t.date < ($2::date + INTERVAL '1 month')
		 GROUP BY t.account_id`,
		householdID, monthStart)
	if err != nil {
		return nil, fmt.Errorf("totalling debt payments: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var m month
		if err := rows.Scan(&id, &m.paid, &m.charged); err != nil {
			return nil, fmt.Errorf("scanning debt payments: %w", err)
		}
		months[id] = m
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	plans := map[int]float64{}
	planRows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT ON (account_id) account_id, COALESCE(amount, 0) FROM debt_plans
		 WHERE household_id = $1 AND effective_from <= $2::date
		 ORDER BY account_id, effective_from DESC`,
		householdID, monthStart)
	if err != nil {
		return nil, fmt.Errorf("loading debt plans: %w", err)
	}
	defer planRows.Close()
	for planRows.Next() {
		var id int
		var amount float64
		if err := planRows.Scan(&id, &amount); err != nil {
			return nil, fmt.Errorf("scanning debt plan: %w", err)
		}
		plans[id] = amount
	}
	if err := planRows.Err(); err != nil {
		return nil, err
	}

	out := []models.DebtLine{}
	for _, a := range accounts {
		if a.Class != models.ClassLiability {
			continue
		}
		m, active := months[a.ID]
		if a.Archived && !active {
			continue
		}
		line := models.DebtLine{
			AccountID:   a.ID,
			AccountName: a.Name,
			AccountType: a.Type,
			Paid:        round2(m.paid),
			Charged:     round2(m.charged),
			Paydown:     paydown(m.paid, m.charged),
			Owed:        round2(math.Max(0, -a.Balance)),
		}
		if !a.Archived {
			line.Monthly = plans[a.ID]
		}
		out = append(out, line)
	}
	return out, nil
}

type DebtPlanInput struct {
	AccountID int `json:"account_id"`
	// Amount is the paydown planned each month; nil or 0 stops the plan.
	Amount        *float64 `json:"amount"`
	EffectiveFrom string   `json:"effective_from"`
}

// SetDebtPlan sets a card or loan's planned monthly paydown from a month on,
// the way setGoalPlan does for a goal: saving what is already in force writes
// nothing, and a real change drops any later ones, since it's meant "from here
// on". Earlier months keep the plan they had.
func (s *Service) SetDebtPlan(ctx context.Context, householdID int, in DebtPlanInput) error {
	if in.Amount != nil && *in.Amount < 0 {
		return invalid("amount can't be negative")
	}
	if in.Amount != nil && round2(*in.Amount) == 0 {
		in.Amount = nil
	}
	account, err := s.GetAccount(ctx, householdID, in.AccountID)
	if errors.Is(err, ErrNotFound) {
		return invalid("account %d does not exist", in.AccountID)
	}
	if err != nil {
		return err
	}
	if account.Class != models.ClassLiability {
		return invalid("only a credit card or loan can have a planned paydown")
	}
	from, _, err := s.resolveBudgetMonth(ctx, householdID, in.EffectiveFrom)
	if err != nil {
		return err
	}

	return s.inTx(func(tx *sql.Tx) error {
		var current sql.NullFloat64
		err := tx.QueryRowContext(ctx,
			`SELECT amount FROM debt_plans WHERE account_id = $1 AND effective_from <= $2::date
			 ORDER BY effective_from DESC LIMIT 1`, in.AccountID, from).Scan(&current)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("reading debt plan: %w", err)
		}
		if (in.Amount == nil && !current.Valid) ||
			(in.Amount != nil && current.Valid && round2(*in.Amount) == round2(current.Float64)) {
			return nil
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM debt_plans WHERE account_id = $1 AND effective_from > $2::date`,
			in.AccountID, from); err != nil {
			return fmt.Errorf("clearing later plan changes: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO debt_plans (household_id, account_id, effective_from, amount)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (account_id, effective_from) DO UPDATE SET amount = EXCLUDED.amount`,
			householdID, in.AccountID, from, nullFloat(in.Amount)); err != nil {
			return fmt.Errorf("saving debt plan: %w", err)
		}
		return nil
	})
}
