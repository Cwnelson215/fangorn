package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"math"

	"github.com/cwnelson/fangorn/internal/models"
)

// DropBalance brings an account's cash to zero with one adjustment transaction
// dated date: the money is dropped from the books without being counted as
// spending, income or a transfer, so it lowers (or, for a debt, raises) net
// worth and nothing else. It is for closing an account whose remaining money
// went nowhere the ledger tracks; moving it to another account is a transfer.
//
// The account row is locked while the balance is read, so two drops at once
// can't both write the full amount.
func (s *Service) DropBalance(ctx context.Context, householdID, accountID int, date string) (models.Transaction, error) {
	if _, err := models.ParseDate(date); err != nil {
		return models.Transaction{}, invalid("date must be YYYY-MM-DD")
	}

	var id int
	err := s.inTx(func(tx *sql.Tx) error {
		var name string
		var starting float64
		err := tx.QueryRowContext(ctx,
			`SELECT name, starting_balance FROM accounts WHERE id = $1 AND household_id = $2 FOR UPDATE`,
			accountID, householdID).Scan(&name, &starting)
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("locking account: %w", err)
		}

		var total float64
		if err := tx.QueryRowContext(ctx,
			`SELECT COALESCE(SUM(amount), 0) FROM transactions WHERE account_id = $1`, accountID,
		).Scan(&total); err != nil {
			return fmt.Errorf("reading balance: %w", err)
		}
		cash := math.Round((starting+total)*100) / 100
		if math.Abs(cash) < 0.005 {
			return invalid("the balance is already $0")
		}

		return tx.QueryRowContext(ctx,
			`INSERT INTO transactions (household_id, account_id, date, amount, kind, description, source)
			 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
			householdID, accountID, date, -cash, models.KindAdjustment,
			"Remaining balance dropped", models.SourceManual,
		).Scan(&id)
	})
	if err != nil {
		return models.Transaction{}, err
	}
	return s.GetTransaction(ctx, householdID, id)
}
