package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/cwnelson/fangorn/internal/models"
)

// A transfer is not its own table. It is two transaction rows sharing a
// transfer_group_id: a negative leg on the source account and a positive leg on
// the destination. That means each side shows up in its own account's register
// with no special-casing, balances stay correct by the same SUM that handles
// everything else, and the only extra work is keeping the pair in step — which
// is why every mutation below runs in a single transaction over the whole group.
//
// Income and expense totals exclude kind = 'transfer' so moving your own money
// never registers as earning or spending it.

// DeletedAccountName stands in for the side of a transfer whose account was
// deleted.
const DeletedAccountName = "Deleted account"

// The date is the source leg's: when the money left. The destination leg carries
// the completion date, when it lands — usually the same day, later for a move
// between institutions. Until then the leg doesn't count (see landed).
const transferSelect = `
	SELECT t.transfer_group_id,
	       COALESCE(MIN(t.date) FILTER (WHERE t.amount < 0), MIN(t.date)) AS date,
	       MAX(t.date) FILTER (WHERE t.amount > 0) AS completion_date,
	       COALESCE(BOOL_OR(t.amount > 0 AND NOT ` + landed + `), false) AS in_transit,
	       MAX(CASE WHEN t.amount < 0 THEN t.account_id END) AS from_id,
	       MAX(CASE WHEN t.amount < 0 THEN a.name END)       AS from_name,
	       MAX(CASE WHEN t.amount > 0 THEN t.account_id END) AS to_id,
	       MAX(CASE WHEN t.amount > 0 THEN a.name END)       AS to_name,
	       MAX(ABS(t.amount)) AS amount,
	       MIN(t.description) AS description,
	       MIN(t.notes) AS notes
	FROM transactions t
	JOIN accounts a ON a.id = t.account_id
	JOIN households h ON h.id = t.household_id
	WHERE t.household_id = $1 AND t.transfer_group_id IS NOT NULL`

func scanTransfer(rows interface{ Scan(...any) error }) (models.Transfer, error) {
	var tr models.Transfer
	var notes sql.NullString
	var date time.Time
	var completion sql.NullTime
	var fromID, toID sql.NullInt64
	var fromName, toName sql.NullString

	err := rows.Scan(&tr.GroupID, &date, &completion, &tr.InTransit, &fromID, &fromName, &toID, &toName,
		&tr.Amount, &tr.Description, &notes)
	if err != nil {
		return tr, err
	}
	tr.Date = dateStr(date)
	if completion.Valid && !completion.Time.Equal(date) {
		c := dateStr(completion.Time)
		tr.CompletionDate = &c
	}
	tr.Notes = strPtr(notes)
	if fromID.Valid {
		tr.FromAccountID = int(fromID.Int64)
	}
	if toID.Valid {
		tr.ToAccountID = int(toID.Int64)
	}
	// Deleting an account takes its legs with it but leaves the other side on
	// the account that still exists, so its balance stays true. The missing
	// side is named rather than left blank.
	tr.FromAccount, tr.ToAccount = fromName.String, toName.String
	if !fromName.Valid {
		tr.FromAccount = DeletedAccountName
	}
	if !toName.Valid {
		tr.ToAccount = DeletedAccountName
	}
	return tr, nil
}

func (s *Service) ListTransfers(ctx context.Context, householdID, limit int) ([]models.Transfer, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		transferSelect+`
		 GROUP BY t.transfer_group_id
		 ORDER BY MIN(t.date) DESC, MIN(t.id) DESC
		 LIMIT $2`,
		householdID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing transfers: %w", err)
	}
	defer rows.Close()

	out := []models.Transfer{}
	for rows.Next() {
		tr, err := scanTransfer(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning transfer: %w", err)
		}
		out = append(out, tr)
	}
	return out, rows.Err()
}

func (s *Service) GetTransfer(ctx context.Context, householdID int, groupID string) (models.Transfer, error) {
	row := s.db.QueryRowContext(ctx,
		transferSelect+` AND t.transfer_group_id = $2 GROUP BY t.transfer_group_id`,
		householdID, groupID)

	tr, err := scanTransfer(row)
	if err == sql.ErrNoRows {
		return tr, ErrNotFound
	}
	if err != nil {
		return tr, fmt.Errorf("fetching transfer: %w", err)
	}
	return tr, nil
}

type TransferInput struct {
	FromAccountID int     `json:"from_account_id"`
	ToAccountID   int     `json:"to_account_id"`
	Amount        float64 `json:"amount"`
	Date          string  `json:"date"`
	// CompletionDate is when the money reaches the destination, for a move
	// between institutions that takes days. Empty means the same day as Date.
	CompletionDate *string `json:"completion_date"`
	Description    string  `json:"description"`
	Notes          *string `json:"notes"`
}

// arrives is the destination leg's date.
func (in TransferInput) arrives() string {
	if in.CompletionDate != nil {
		return *in.CompletionDate
	}
	return in.Date
}

func (in *TransferInput) normalize() error {
	in.Description = strings.TrimSpace(in.Description)
	if in.Description == "" {
		in.Description = "Transfer"
	}
	if in.FromAccountID <= 0 || in.ToAccountID <= 0 {
		return invalid("both a source and a destination account are required")
	}
	if in.FromAccountID == in.ToAccountID {
		return invalid("source and destination must be different accounts")
	}
	if math.Abs(in.Amount) < 0.005 {
		return invalid("amount must be greater than zero")
	}
	if in.Date == "" {
		return invalid("date is required")
	}
	if _, err := models.ParseDate(in.Date); err != nil {
		return invalid("date must be YYYY-MM-DD")
	}
	if in.CompletionDate != nil {
		c := strings.TrimSpace(*in.CompletionDate)
		if c == "" || c == in.Date {
			in.CompletionDate = nil
		} else if _, err := models.ParseDate(c); err != nil {
			return invalid("completion date must be YYYY-MM-DD")
		} else if c < in.Date {
			return invalid("the completion date can't be before the transfer date")
		} else {
			in.CompletionDate = &c
		}
	}
	in.Amount = math.Abs(in.Amount)
	return nil
}

func (s *Service) CreateTransfer(ctx context.Context, householdID int, in TransferInput) (models.Transfer, error) {
	if err := in.normalize(); err != nil {
		return models.Transfer{}, err
	}
	if err := s.assertAccount(ctx, householdID, in.FromAccountID); err != nil {
		return models.Transfer{}, err
	}
	if err := s.assertAccount(ctx, householdID, in.ToAccountID); err != nil {
		return models.Transfer{}, err
	}

	var groupID string
	err := s.inTx(func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `SELECT gen_random_uuid()::text`).Scan(&groupID); err != nil {
			return fmt.Errorf("generating transfer id: %w", err)
		}
		return insertTransferLegs(ctx, tx, householdID, groupID, in, nil)
	})
	if err != nil {
		return models.Transfer{}, err
	}
	return s.GetTransfer(ctx, householdID, groupID)
}

// UpdateTransfer replaces both legs. Rewriting the pair is simpler and safer than
// trying to patch two rows in place — there is no state on a transfer worth
// preserving, and it removes any chance of the legs disagreeing.
func (s *Service) UpdateTransfer(ctx context.Context, householdID int, groupID string, in TransferInput) (models.Transfer, error) {
	if err := in.normalize(); err != nil {
		return models.Transfer{}, err
	}
	if err := s.assertAccount(ctx, householdID, in.FromAccountID); err != nil {
		return models.Transfer{}, err
	}
	if err := s.assertAccount(ctx, householdID, in.ToAccountID); err != nil {
		return models.Transfer{}, err
	}

	err := s.inTx(func(tx *sql.Tx) error {
		// Preserve the originating rule, if this transfer was posted by one, so
		// editing it does not orphan the occurrence that created it.
		var ruleID sql.NullInt64
		err := tx.QueryRowContext(ctx,
			`SELECT MAX(recurring_rule_id) FROM transactions
			 WHERE household_id = $1 AND transfer_group_id = $2`,
			householdID, groupID).Scan(&ruleID)
		if err != nil {
			return fmt.Errorf("reading transfer: %w", err)
		}

		res, err := tx.ExecContext(ctx,
			`DELETE FROM transactions WHERE household_id = $1 AND transfer_group_id = $2`,
			householdID, groupID)
		if err != nil {
			return fmt.Errorf("clearing transfer legs: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return insertTransferLegs(ctx, tx, householdID, groupID, in, intPtr(ruleID))
	})
	if err != nil {
		return models.Transfer{}, err
	}
	return s.GetTransfer(ctx, householdID, groupID)
}

func (s *Service) DeleteTransfer(ctx context.Context, householdID int, groupID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM transactions WHERE household_id = $1 AND transfer_group_id = $2`,
		householdID, groupID)
	if err != nil {
		return fmt.Errorf("deleting transfer: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// insertTransferLegs writes the debit and credit rows for a transfer group.
func insertTransferLegs(ctx context.Context, tx *sql.Tx, householdID int, groupID string, in TransferInput, ruleID *int) error {
	source := models.SourceManual
	if ruleID != nil {
		source = models.SourceRecurring
	}

	legs := []struct {
		accountID int
		date      string
		amount    float64
	}{
		{in.FromAccountID, in.Date, -in.Amount},
		{in.ToAccountID, in.arrives(), in.Amount},
	}

	for _, leg := range legs {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO transactions
			   (household_id, account_id, date, amount, kind, description, notes,
			    transfer_group_id, recurring_rule_id, source)
			 VALUES ($1,$2,$3,$4,'transfer',$5,$6,$7,$8,$9)`,
			householdID, leg.accountID, leg.date, leg.amount, in.Description,
			nullStr(in.Notes), groupID, nullInt(ruleID), source,
		)
		if err != nil {
			return fmt.Errorf("inserting transfer leg: %w", err)
		}
	}
	return nil
}
