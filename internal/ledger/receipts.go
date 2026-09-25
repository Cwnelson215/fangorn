package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"github.com/cwnelson/fangorn/internal/models"
)

// Receipts are written by two kinds of caller at once: the upload request, which
// tries to read the receipt while the phone waits, and the scheduler, which picks
// up whatever the upload did not finish. Either may lose a race with the other,
// or with a second container. Two things keep that safe:
//
//   - A claim. ClaimReceipt flips pending -> processing in one statement, so only
//     one caller gets to spend money asking the model about a given receipt. The
//     claim is a lease: one abandoned mid-call can be taken over after it expires.
//   - A fence. Every write that ends a claim matches on the claim_seq the claim
//     returned. A worker whose lease was taken over, or whose receipt was deleted
//     mid-call, matches no row and rolls back rather than posting.

// ErrLostClaim means a receipt was re-claimed or deleted while this caller was
// working on it. The other claimant owns the outcome; this one does nothing.
var ErrLostClaim = errors.New("receipt claim lost")

const receiptSelect = `
	SELECT id, status, review_reasons, media_type, byte_size,
	       merchant, purchased_on, currency, txn_type, subtotal, tax, tip, total,
	       tender, card_last4, category_suggested, line_items, model,
	       account_id, category_id, transaction_id, extract_error, created_at, via_shortcut
	FROM receipts`

func scanReceipt(row interface{ Scan(...any) error }) (models.Receipt, error) {
	var r models.Receipt
	var merchant, currency, txnType, tender, last4, suggested, model, extractErr sql.NullString
	var subtotal, tax, tip, total sql.NullFloat64
	var accountID, categoryID, txnID sql.NullInt64
	var purchasedOn sql.NullTime
	var createdAt time.Time
	var lineItems []byte

	err := row.Scan(
		&r.ID, &r.Status, pq.Array(&r.ReviewReasons), &r.MediaType, &r.ByteSize,
		&merchant, &purchasedOn, &currency, &txnType, &subtotal, &tax, &tip, &total,
		&tender, &last4, &suggested, &lineItems, &model,
		&accountID, &categoryID, &txnID, &extractErr, &createdAt, &r.ViaShortcut,
	)
	if err != nil {
		return r, err
	}
	if r.ReviewReasons == nil {
		r.ReviewReasons = []string{}
	}
	r.Merchant = strPtr(merchant)
	r.PurchasedOn = dateStrPtr(purchasedOn)
	r.Currency = strPtr(currency)
	r.TxnType = strPtr(txnType)
	r.Subtotal = floatPtr(subtotal)
	r.Tax = floatPtr(tax)
	r.Tip = floatPtr(tip)
	r.Total = floatPtr(total)
	r.Tender = strPtr(tender)
	r.CardLast4 = strPtr(last4)
	r.CategorySuggested = strPtr(suggested)
	if len(lineItems) > 0 {
		r.LineItems = lineItems
	}
	r.Model = strPtr(model)
	r.AccountID = intPtr(accountID)
	r.CategoryID = intPtr(categoryID)
	r.TransactionID = intPtr(txnID)
	r.ExtractError = strPtr(extractErr)
	r.CreatedAt = createdAt.Format(time.RFC3339)
	return r, nil
}

// NewReceipt is an uploaded photo, already type-checked and hashed by the caller.
type NewReceipt struct {
	Image     []byte
	MediaType string
	SHA256    []byte
	// ViaShortcut marks an upload from the iPhone Shortcut, which category
	// accounts don't apply to.
	ViaShortcut bool
}

// CreateReceipt stores a photo as a pending receipt. The same photo uploaded
// twice is the same receipt: the second call returns the first one with
// created=false rather than an error, since a double tap is not a mistake worth
// reporting.
func (s *Service) CreateReceipt(ctx context.Context, householdID int, in NewReceipt) (r models.Receipt, created bool, err error) {
	var id int
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO receipts (household_id, image, media_type, byte_size, image_sha256, via_shortcut)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (household_id, image_sha256) DO NOTHING
		 RETURNING id`,
		householdID, in.Image, in.MediaType, len(in.Image), in.SHA256, in.ViaShortcut,
	).Scan(&id)
	switch {
	case err == nil:
		created = true
	case errors.Is(err, sql.ErrNoRows):
		if err := s.db.QueryRowContext(ctx,
			`SELECT id FROM receipts WHERE household_id = $1 AND image_sha256 = $2`,
			householdID, in.SHA256,
		).Scan(&id); err != nil {
			return r, false, fmt.Errorf("finding existing receipt: %w", err)
		}
	default:
		return r, false, fmt.Errorf("creating receipt: %w", err)
	}
	r, err = s.GetReceipt(ctx, householdID, id)
	return r, created, err
}

func (s *Service) GetReceipt(ctx context.Context, householdID, id int) (models.Receipt, error) {
	r, err := scanReceipt(s.db.QueryRowContext(ctx,
		receiptSelect+` WHERE household_id = $1 AND id = $2`, householdID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, fmt.Errorf("fetching receipt: %w", err)
	}
	return r, nil
}

// ListReceipts returns receipts newest first, optionally only those in one
// status. The image column is never selected here.
func (s *Service) ListReceipts(ctx context.Context, householdID int, status string, limit int) ([]models.Receipt, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		receiptSelect+` WHERE household_id = $1 AND ($2 = '' OR status = $2)
		 ORDER BY created_at DESC, id DESC LIMIT $3`,
		householdID, status, limit)
	if err != nil {
		return nil, fmt.Errorf("listing receipts: %w", err)
	}
	defer rows.Close()

	out := []models.Receipt{}
	for rows.Next() {
		r, err := scanReceipt(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning receipt: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ReceiptImage returns the stored photo and its media type.
func (s *Service) ReceiptImage(ctx context.Context, householdID, id int) ([]byte, string, error) {
	var img []byte
	var mediaType string
	err := s.db.QueryRowContext(ctx,
		`SELECT image, media_type FROM receipts WHERE household_id = $1 AND id = $2`,
		householdID, id).Scan(&img, &mediaType)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("fetching receipt image: %w", err)
	}
	return img, mediaType, nil
}

// ---------------------------------------------------------------------------
// claiming and finishing
// ---------------------------------------------------------------------------

// Claim is one caller's hold on a receipt.
type Claim struct {
	ReceiptID int
	Seq       int
	Failures  int
	Image     []byte
	MediaType string
	// ViaShortcut: uploaded by the iPhone Shortcut, so matched without
	// category accounts.
	ViaShortcut bool
}

// ClaimReceipt takes a receipt for processing, if it is claimable: pending and
// past any backoff, or processing under a lease older than lease. ok is false
// when it is not — someone else has it, it is finished, or it does not exist.
//
// The row lock the UPDATE takes is what makes this safe: a second concurrent
// claimer waits for the first to commit, re-checks the WHERE against the new
// row, and matches nothing.
func (s *Service) ClaimReceipt(ctx context.Context, householdID, id int, lease time.Duration) (c Claim, ok bool, err error) {
	err = s.db.QueryRowContext(ctx,
		`UPDATE receipts
		 SET status = 'processing', claimed_at = NOW(), claim_seq = claim_seq + 1, updated_at = NOW()
		 WHERE household_id = $1 AND id = $2 AND (
		   (status = 'pending' AND (retry_after IS NULL OR retry_after <= NOW()))
		   OR (status = 'processing' AND claimed_at < NOW() - make_interval(secs => $3))
		 )
		 RETURNING id, claim_seq, failures, image, media_type, via_shortcut`,
		householdID, id, lease.Seconds(),
	).Scan(&c.ReceiptID, &c.Seq, &c.Failures, &c.Image, &c.MediaType, &c.ViaShortcut)
	if errors.Is(err, sql.ErrNoRows) {
		return c, false, nil
	}
	if err != nil {
		return c, false, fmt.Errorf("claiming receipt: %w", err)
	}
	return c, true, nil
}

// ClaimableReceipts lists receipts that ClaimReceipt would currently accept,
// oldest first, for the scheduler to work through.
func (s *Service) ClaimableReceipts(ctx context.Context, householdID int, lease time.Duration, limit int) ([]int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM receipts
		 WHERE household_id = $1 AND (
		   (status = 'pending' AND (retry_after IS NULL OR retry_after <= NOW()))
		   OR (status = 'processing' AND claimed_at < NOW() - make_interval(secs => $2))
		 )
		 ORDER BY created_at, id LIMIT $3`,
		householdID, lease.Seconds(), limit)
	if err != nil {
		return nil, fmt.Errorf("listing claimable receipts: %w", err)
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ReceiptFields is what was read off a receipt, cleaned and ready to store.
type ReceiptFields struct {
	Merchant          *string
	PurchasedOn       *string
	Currency          *string
	TxnType           *string
	Subtotal          *float64
	Tax               *float64
	Tip               *float64
	Total             *float64
	Tender            *string
	CardLast4         *string
	CategorySuggested *string
	LineItems         []byte // JSON, or nil
	Model             *string
}

// ReceiptOutcome is how a claim ends. Post is set when the receipt should become
// a transaction now; otherwise the receipt is held with Reasons.
type ReceiptOutcome struct {
	Fields     *ReceiptFields // nil when nothing was extracted
	AccountID  *int
	CategoryID *int
	Reasons    []string
	Post       *TransactionInput
}

// FinishReceipt ends a claim. When the outcome posts, the transaction and the
// status change are one database transaction, so a receipt is never marked
// posted without its transaction or vice versa. Returns ErrLostClaim if the claim
// was taken over or the receipt deleted in the meantime.
func (s *Service) FinishReceipt(ctx context.Context, householdID int, c Claim, out ReceiptOutcome) error {
	status := models.ReceiptNeedsReview
	if out.Post != nil {
		status = models.ReceiptPosted
		out.Post.Kind = models.KindExpense
		if err := out.Post.normalize(); err != nil {
			return err
		}
		if err := s.assertAccount(ctx, householdID, out.Post.AccountID); err != nil {
			return err
		}
		if err := s.assertCategory(ctx, householdID, out.Post.CategoryID, out.Post.Kind); err != nil {
			return err
		}
	}
	f := out.Fields
	if f == nil {
		f = &ReceiptFields{}
	}
	reasons := out.Reasons
	if reasons == nil {
		reasons = []string{}
	}

	return s.inTx(func(tx *sql.Tx) error {
		var txnID any
		if out.Post != nil {
			id, err := insertTxn(ctx, tx, householdID, *out.Post, models.SourceReceipt)
			if err != nil {
				return err
			}
			txnID = id
		}
		// Extracted fields are only overwritten when there are new ones, so a
		// failure after a successful read does not erase what was read.
		res, err := tx.ExecContext(ctx,
			`UPDATE receipts SET
			   status = $4, review_reasons = $5, transaction_id = $6,
			   account_id = $7, category_id = $8,
			   merchant = CASE WHEN $9 THEN $10 ELSE merchant END,
			   purchased_on = CASE WHEN $9 THEN $11::date ELSE purchased_on END,
			   currency = CASE WHEN $9 THEN $12 ELSE currency END,
			   txn_type = CASE WHEN $9 THEN $13 ELSE txn_type END,
			   subtotal = CASE WHEN $9 THEN $14::numeric ELSE subtotal END,
			   tax = CASE WHEN $9 THEN $15::numeric ELSE tax END,
			   tip = CASE WHEN $9 THEN $16::numeric ELSE tip END,
			   total = CASE WHEN $9 THEN $17::numeric ELSE total END,
			   tender = CASE WHEN $9 THEN $18 ELSE tender END,
			   card_last4 = CASE WHEN $9 THEN $19 ELSE card_last4 END,
			   category_suggested = CASE WHEN $9 THEN $20 ELSE category_suggested END,
			   line_items = CASE WHEN $9 THEN $21::jsonb ELSE line_items END,
			   model = CASE WHEN $9 THEN $22 ELSE model END,
			   claimed_at = NULL, retry_after = NULL, extract_error = NULL, updated_at = NOW()
			 WHERE household_id = $1 AND id = $2 AND status = 'processing' AND claim_seq = $3`,
			householdID, c.ReceiptID, c.Seq,
			status, pq.Array(reasons), txnID, nullInt(out.AccountID), nullInt(out.CategoryID),
			out.Fields != nil, nullStr(f.Merchant), nullStr(f.PurchasedOn), nullStr(f.Currency),
			nullStr(f.TxnType), nullFloat(f.Subtotal), nullFloat(f.Tax), nullFloat(f.Tip),
			nullFloat(f.Total), nullStr(f.Tender), nullStr(f.CardLast4), nullStr(f.CategorySuggested),
			nullJSON(f.LineItems), nullStr(f.Model),
		)
		if err != nil {
			return fmt.Errorf("finishing receipt: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrLostClaim
		}
		return nil
	})
}

// RecordReceiptFailure ends a claim after a failed extraction. With a holdReason
// the receipt goes to a person; without one it waits until retryAfter to be
// tried again. Either way the failure is counted and cause is kept.
func (s *Service) RecordReceiptFailure(ctx context.Context, householdID int, c Claim, cause, holdReason string, retryAfter time.Time) error {
	status, reasons := models.ReceiptPending, []string{}
	if holdReason != "" {
		status, reasons = models.ReceiptNeedsReview, []string{holdReason}
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE receipts SET
		   status = $4, review_reasons = $5, failures = failures + 1,
		   failed_at = NOW(), extract_error = $6, retry_after = $7,
		   claimed_at = NULL, updated_at = NOW()
		 WHERE household_id = $1 AND id = $2 AND status = 'processing' AND claim_seq = $3`,
		householdID, c.ReceiptID, c.Seq, status, pq.Array(reasons), cause, retryAfter)
	if err != nil {
		return fmt.Errorf("recording receipt failure: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrLostClaim
	}
	return nil
}

// ReleaseReceipt hands a claim back without counting anything against the
// receipt — for a caller that was stopped (shutdown, a cancelled context) rather
// than one that failed. The next claimer can take it immediately.
func (s *Service) ReleaseReceipt(ctx context.Context, householdID int, c Claim) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE receipts SET status = 'pending', claimed_at = NULL, updated_at = NOW()
		 WHERE household_id = $1 AND id = $2 AND status = 'processing' AND claim_seq = $3`,
		householdID, c.ReceiptID, c.Seq)
	if err != nil {
		return fmt.Errorf("releasing receipt: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// what a person does with a receipt
// ---------------------------------------------------------------------------

// PostHeldReceipt turns a receipt waiting for review into a transaction, using
// what the person confirmed rather than what was read. It may be an expense or,
// for a return receipt, a refund.
func (s *Service) PostHeldReceipt(ctx context.Context, householdID, id int, in TransactionInput) (models.Transaction, error) {
	if in.Kind != models.KindExpense && in.Kind != models.KindRefund {
		return models.Transaction{}, invalid("a receipt posts as an expense or a refund")
	}
	if err := in.normalize(); err != nil {
		return models.Transaction{}, err
	}
	if err := s.assertAccount(ctx, householdID, in.AccountID); err != nil {
		return models.Transaction{}, err
	}
	if err := s.assertCategory(ctx, householdID, in.CategoryID, in.Kind); err != nil {
		return models.Transaction{}, err
	}

	var txnID int
	err := s.inTx(func(tx *sql.Tx) error {
		// Lock the receipt first so a second click, or a second tab, waits here
		// and then sees it already posted.
		var status string
		err := tx.QueryRowContext(ctx,
			`SELECT status FROM receipts WHERE household_id = $1 AND id = $2 FOR UPDATE`,
			householdID, id).Scan(&status)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("locking receipt: %w", err)
		}
		if status != models.ReceiptNeedsReview {
			return receiptStateError(status)
		}

		txnID, err = insertTxn(ctx, tx, householdID, in, models.SourceReceipt)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx,
			`UPDATE receipts SET status = 'posted', transaction_id = $3,
			   account_id = $4, category_id = $5, updated_at = NOW()
			 WHERE household_id = $1 AND id = $2`,
			householdID, id, txnID, in.AccountID, nullInt(in.CategoryID))
		if err != nil {
			return fmt.Errorf("marking receipt posted: %w", err)
		}
		return nil
	})
	if err != nil {
		return models.Transaction{}, err
	}
	return s.GetTransaction(ctx, householdID, txnID)
}

// RetryReceipt sends a held receipt back to be read again from scratch.
func (s *Service) RetryReceipt(ctx context.Context, householdID, id int) (models.Receipt, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE receipts SET status = 'pending', review_reasons = '{}', failures = 0,
		   retry_after = NULL, extract_error = NULL, updated_at = NOW()
		 WHERE household_id = $1 AND id = $2 AND status = 'needs_review'`,
		householdID, id)
	if err != nil {
		return models.Receipt{}, fmt.Errorf("retrying receipt: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		r, err := s.GetReceipt(ctx, householdID, id)
		if err != nil {
			return r, err
		}
		return r, receiptStateError(r.Status)
	}
	return s.GetReceipt(ctx, householdID, id)
}

// DeleteReceipt removes a receipt that has not become a transaction. A posted
// receipt goes by deleting its transaction, which takes the receipt with it —
// deleting only the photo would leave an expense nobody can check.
func (s *Service) DeleteReceipt(ctx context.Context, householdID, id int) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM receipts WHERE household_id = $1 AND id = $2 AND status <> 'posted'`,
		householdID, id)
	if err != nil {
		return fmt.Errorf("deleting receipt: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := s.GetReceipt(ctx, householdID, id); err != nil {
			return err
		}
		return invalid("this receipt is already a transaction; delete the transaction instead")
	}
	return nil
}

func receiptStateError(status string) error {
	switch status {
	case models.ReceiptPosted:
		return invalid("this receipt has already been posted")
	case models.ReceiptPending, models.ReceiptProcessing:
		return invalid("this receipt is still being read")
	}
	return invalid("this receipt is %s", status)
}

// ---------------------------------------------------------------------------
// what the processor needs to decide
// ---------------------------------------------------------------------------

// ReceiptAccount is the part of an account a receipt can be matched against.
type ReceiptAccount struct {
	ID   int
	Type string
	Mask string
}

// ReceiptCategory is an expense category a receipt could be filed under.
type ReceiptCategory struct {
	ID   int
	Name string
	// AccountID is the open account this category's spending goes on, if any.
	AccountID *int
}

// ReceiptContext is the household state a receipt is resolved against: its
// open accounts and its active expense categories.
type ReceiptContext struct {
	Accounts   []ReceiptAccount
	Categories []ReceiptCategory
}

func (s *Service) ReceiptContext(ctx context.Context, householdID int) (ReceiptContext, error) {
	var rc ReceiptContext
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, type, COALESCE(mask, '') FROM accounts
		 WHERE household_id = $1 AND archived_at IS NULL ORDER BY id`, householdID)
	if err != nil {
		return rc, fmt.Errorf("loading accounts for receipt: %w", err)
	}
	for rows.Next() {
		var a ReceiptAccount
		if err := rows.Scan(&a.ID, &a.Type, &a.Mask); err != nil {
			rows.Close()
			return rc, err
		}
		rc.Accounts = append(rc.Accounts, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return rc, err
	}

	rows, err = s.db.QueryContext(ctx,
		`SELECT c.id, c.name, a.id FROM categories c
		 LEFT JOIN accounts a ON a.id = c.default_account_id AND a.household_id = c.household_id
		   AND a.archived_at IS NULL
		 WHERE c.household_id = $1 AND c.kind = 'expense' AND c.archived_at IS NULL ORDER BY c.name`,
		householdID)
	if err != nil {
		return rc, fmt.Errorf("loading categories for receipt: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c ReceiptCategory
		var account sql.NullInt64
		if err := rows.Scan(&c.ID, &c.Name, &account); err != nil {
			return rc, err
		}
		c.AccountID = intPtr(account)
		rc.Categories = append(rc.Categories, c)
	}
	return rc, rows.Err()
}

// SimilarExpenseExists reports whether the account already has an expense of
// exactly this amount within a day of date. The ledger is kept by hand, so a
// photographed receipt has often already been typed in.
func (s *Service) SimilarExpenseExists(ctx context.Context, householdID, accountID int, date string, amount float64) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (
		   SELECT 1 FROM transactions
		   WHERE household_id = $1 AND account_id = $2 AND kind = 'expense'
		     AND amount = $3::numeric
		     AND date BETWEEN $4::date - 1 AND $4::date + 1
		 )`,
		householdID, accountID, -amount, date).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking for a similar expense: %w", err)
	}
	return exists, nil
}

// GetHousehold loads one household, for callers outside this package that need
// its "today".
func (s *Service) GetHousehold(ctx context.Context, id int) (Household, error) {
	return s.household(ctx, id)
}

func nullFloat(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}

func nullJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}
