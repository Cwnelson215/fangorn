package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/cwnelson/fangorn/internal/models"
)

const txnSelect = `
	SELECT t.id, t.account_id, a.name, t.date, t.amount, t.kind, t.description,
	       t.merchant, t.category_id, c.name, t.notes, t.transfer_group_id,
	       t.recurring_rule_id, t.trade_id, t.source, t.created_at
	FROM transactions t
	JOIN accounts a ON a.id = t.account_id
	LEFT JOIN categories c ON c.id = t.category_id`

func scanTxn(rows interface{ Scan(...any) error }) (models.Transaction, error) {
	var t models.Transaction
	var merchant, categoryName, notes, groupID sql.NullString
	var categoryID, ruleID, tradeID sql.NullInt64
	var date, createdAt time.Time

	err := rows.Scan(
		&t.ID, &t.AccountID, &t.AccountName, &date, &t.Amount, &t.Kind, &t.Description,
		&merchant, &categoryID, &categoryName, &notes, &groupID, &ruleID, &tradeID,
		&t.Source, &createdAt,
	)
	if err != nil {
		return t, err
	}
	t.Date = dateStr(date)
	t.CreatedAt = createdAt.Format(time.RFC3339)
	t.Merchant = strPtr(merchant)
	t.CategoryID = intPtr(categoryID)
	t.CategoryName = strPtr(categoryName)
	t.Notes = strPtr(notes)
	t.TransferGroupID = strPtr(groupID)
	t.RecurringRuleID = intPtr(ruleID)
	t.TradeID = intPtr(tradeID)
	return t, nil
}

// TransactionFilter mirrors the query params accepted by GET /api/transactions.
// Empty fields are omitted from the WHERE clause.
type TransactionFilter struct {
	AccountID  int
	CategoryID int
	Kind       string
	From       string
	To         string
	Search     string
	Limit      int
}

// ListTransactions builds its WHERE clause incrementally with a running
// placeholder counter — the same approach the pre-pivot handler used, kept
// because it stays readable as filters are added.
func (s *Service) ListTransactions(ctx context.Context, householdID int, f TransactionFilter) ([]models.Transaction, error) {
	where := []string{"t.household_id = $1"}
	args := []any{householdID}
	n := 1

	// bind registers an argument and returns its placeholder, e.g. "$3".
	bind := func(val any) string {
		n++
		args = append(args, val)
		return "$" + strconv.Itoa(n)
	}

	if f.AccountID > 0 {
		where = append(where, "t.account_id = "+bind(f.AccountID))
	}
	if f.CategoryID > 0 {
		where = append(where, "t.category_id = "+bind(f.CategoryID))
	}
	if f.Kind != "" {
		where = append(where, "t.kind = "+bind(f.Kind))
	}
	if f.From != "" {
		where = append(where, "t.date >= "+bind(f.From))
	}
	if f.To != "" {
		where = append(where, "t.date <= "+bind(f.To))
	}
	if f.Search != "" {
		// One argument, referenced from two positions — Postgres allows this.
		p := bind("%" + f.Search + "%")
		where = append(where, "(t.description ILIKE "+p+" OR t.merchant ILIKE "+p+")")
	}

	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	n++
	args = append(args, limit)

	q := txnSelect + " WHERE " + strings.Join(where, " AND ") +
		" ORDER BY t.date DESC, t.id DESC LIMIT $" + strconv.Itoa(n)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listing transactions: %w", err)
	}
	defer rows.Close()

	out := []models.Transaction{}
	for rows.Next() {
		t, err := scanTxn(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning transaction: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Service) GetTransaction(ctx context.Context, householdID, id int) (models.Transaction, error) {
	row := s.db.QueryRowContext(ctx,
		txnSelect+` WHERE t.household_id = $1 AND t.id = $2`, householdID, id)

	t, err := scanTxn(row)
	if err == sql.ErrNoRows {
		return t, ErrNotFound
	}
	if err != nil {
		return t, fmt.Errorf("fetching transaction: %w", err)
	}
	return t, nil
}

// TransactionInput is what the entry form submits.
//
// Amount is accepted as a POSITIVE magnitude and the sign is applied from Kind.
// Asking someone logging groceries to type "-84.20" is a mistake waiting to
// happen, so the API takes the magnitude and owns the convention.
type TransactionInput struct {
	AccountID   int     `json:"account_id"`
	Date        string  `json:"date"`
	Amount      float64 `json:"amount"`
	Kind        string  `json:"kind"`
	Description string  `json:"description"`
	Merchant    *string `json:"merchant"`
	CategoryID  *int    `json:"category_id"`
	Notes       *string `json:"notes"`
}

// signedAmount converts the submitted magnitude into the stored signed value.
//
// A refund is money arriving, so it keeps the positive sign income has. What
// makes it reduce spending rather than raise earnings is its kind, which the
// spending queries read — not its sign.
func (in *TransactionInput) signedAmount() float64 {
	amt := math.Abs(in.Amount)
	if in.Kind == models.KindExpense {
		return -amt
	}
	return amt
}

func (in *TransactionInput) normalize() error {
	in.Description = strings.TrimSpace(in.Description)
	if in.AccountID <= 0 {
		return invalid("account_id is required")
	}
	if in.Description == "" {
		return invalid("description is required")
	}
	switch in.Kind {
	case models.KindIncome, models.KindExpense:
	case models.KindRefund:
		// A refund with no category is a positive number sitting outside every
		// spending total: it would raise the balance and cancel nothing.
		if in.CategoryID == nil {
			return invalid("a refund needs the category the money is coming back from")
		}
	default:
		return invalid("kind must be %q, %q or %q; use the transfer endpoint to move money between accounts",
			models.KindIncome, models.KindExpense, models.KindRefund)
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
	return nil
}

func (s *Service) CreateTransaction(ctx context.Context, householdID int, in TransactionInput) (models.Transaction, error) {
	if err := in.normalize(); err != nil {
		return models.Transaction{}, err
	}
	if err := s.assertAccount(ctx, householdID, in.AccountID); err != nil {
		return models.Transaction{}, err
	}
	if err := s.assertCategory(ctx, householdID, in.CategoryID, in.Kind); err != nil {
		return models.Transaction{}, err
	}

	var id int
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO transactions
		   (household_id, account_id, date, amount, kind, description, merchant, category_id, notes, source)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'manual')
		 RETURNING id`,
		householdID, in.AccountID, in.Date, in.signedAmount(), in.Kind,
		in.Description, nullStr(in.Merchant), nullInt(in.CategoryID), nullStr(in.Notes),
	).Scan(&id)
	if err != nil {
		return models.Transaction{}, fmt.Errorf("creating transaction: %w", err)
	}
	return s.GetTransaction(ctx, householdID, id)
}

func (s *Service) UpdateTransaction(ctx context.Context, householdID, id int, in TransactionInput) (models.Transaction, error) {
	if err := in.normalize(); err != nil {
		return models.Transaction{}, err
	}
	if err := s.assertAccount(ctx, householdID, in.AccountID); err != nil {
		return models.Transaction{}, err
	}
	if err := s.assertCategory(ctx, householdID, in.CategoryID, in.Kind); err != nil {
		return models.Transaction{}, err
	}

	// Editing one leg of a transfer in isolation would break the pairing, so
	// transfers are routed through UpdateTransfer instead.
	existing, err := s.GetTransaction(ctx, householdID, id)
	if err != nil {
		return models.Transaction{}, err
	}
	if existing.Kind == models.KindTransfer {
		return models.Transaction{}, invalid("this is one leg of a transfer; edit it from the transfers view")
	}
	if existing.Kind == models.KindTrade {
		return models.Transaction{}, invalid("this is the cash side of a trade; edit the trade from its account page")
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE transactions SET
		   account_id = $1, date = $2, amount = $3, kind = $4, description = $5,
		   merchant = $6, category_id = $7, notes = $8, updated_at = NOW()
		 WHERE household_id = $9 AND id = $10`,
		in.AccountID, in.Date, in.signedAmount(), in.Kind, in.Description,
		nullStr(in.Merchant), nullInt(in.CategoryID), nullStr(in.Notes), householdID, id,
	)
	if err != nil {
		return models.Transaction{}, fmt.Errorf("updating transaction: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.Transaction{}, ErrNotFound
	}
	return s.GetTransaction(ctx, householdID, id)
}

// DeleteTransaction removes a single transaction. Deleting either leg of a
// transfer removes both, so a half-transfer can never be left behind. Deleting
// the cash side of a trade deletes the trade, which re-checks that no later sell
// depended on its shares.
func (s *Service) DeleteTransaction(ctx context.Context, householdID, id int) error {
	existing, err := s.GetTransaction(ctx, householdID, id)
	if err != nil {
		return err
	}
	if existing.TransferGroupID != nil {
		return s.DeleteTransfer(ctx, householdID, *existing.TransferGroupID)
	}
	if existing.TradeID != nil {
		return s.DeleteTrade(ctx, householdID, *existing.TradeID)
	}

	res, err := s.db.ExecContext(ctx,
		`DELETE FROM transactions WHERE household_id = $1 AND id = $2`, householdID, id)
	if err != nil {
		return fmt.Errorf("deleting transaction: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// ownership checks
//
// Foreign keys alone would let a caller attach another household's account or
// category, since the FK does not know about tenancy. These verify scope first.
// ---------------------------------------------------------------------------

func (s *Service) assertAccount(ctx context.Context, householdID, accountID int) error {
	var ok bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM accounts WHERE id = $1 AND household_id = $2)`,
		accountID, householdID).Scan(&ok)
	if err != nil {
		return fmt.Errorf("checking account: %w", err)
	}
	if !ok {
		return invalid("account %d does not exist", accountID)
	}
	return nil
}

// assertCategory checks that a category belongs to the household and that its
// kind matches what the transaction is doing with it.
//
// The kind half is not pedantry. Every spending query filters on the
// transaction's kind and joins the category, so an expense filed against an
// income category is accepted, moves the account balance, and then appears in no
// budget, no category breakdown and no spending chart. The money is gone and
// nothing says where — the worst kind of wrong, because it looks like nothing
// happened. Catching it at the write is the only cheap moment.
func (s *Service) assertCategory(ctx context.Context, householdID int, categoryID *int, txnKind string) error {
	if categoryID == nil {
		return nil
	}
	var catKind string
	err := s.db.QueryRowContext(ctx,
		`SELECT kind FROM categories WHERE id = $1 AND household_id = $2`,
		*categoryID, householdID).Scan(&catKind)
	if err == sql.ErrNoRows {
		return invalid("category %d does not exist", *categoryID)
	}
	if err != nil {
		return fmt.Errorf("checking category: %w", err)
	}
	if want := models.CategoryKindFor(txnKind); want != "" && catKind != want {
		return invalid("a %s needs an %s category, and that one is %s", txnKind, want, catKind)
	}
	return nil
}
