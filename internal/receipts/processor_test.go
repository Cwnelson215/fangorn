package receipts

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
	"github.com/cwnelson/fangorn/internal/testdb"
	"github.com/cwnelson/fangorn/internal/vision"
)

// fakeExtractor stands in for the model. block, when set, holds every call until
// it is closed or the call's context ends.
type fakeExtractor struct {
	mu     sync.Mutex
	result vision.Extraction
	err    error
	block  chan struct{}
	calls  int
}

func (f *fakeExtractor) Extract(ctx context.Context, img []byte, mediaType string, h vision.Hints) (vision.Extraction, vision.Meta, error) {
	f.mu.Lock()
	f.calls++
	block, result, err := f.block, f.result, f.err
	f.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return vision.Extraction{}, vision.Meta{}, fmt.Errorf("%w: %v", vision.ErrUnavailable, ctx.Err())
		}
	}
	return result, vision.Meta{Model: "fake-model"}, err
}

func (f *fakeExtractor) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fixture struct {
	t        *testing.T
	ctx      context.Context
	svc      *ledger.Service
	hh       int
	ext      *fakeExtractor
	p        *Processor
	card     models.Account
	grocery  models.Category
	uploaded int
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := testdb.Open(t)
	svc := ledger.New(db)
	ctx := context.Background()
	hh := testdb.Household(t, db, "America/Denver")

	mask := "4821"
	card, err := svc.CreateAccount(ctx, hh, ledger.AccountInput{
		Name: "Visa", Type: models.AccountCreditCard, Mask: &mask, StartingBalanceDate: "2026-01-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	grocery, err := svc.CreateCategory(ctx, hh, ledger.CategoryInput{Name: "Groceries", Kind: models.KindExpense})
	if err != nil {
		t.Fatal(err)
	}

	f := &fixture{t: t, ctx: ctx, svc: svc, hh: hh, ext: &fakeExtractor{}, card: card, grocery: grocery}
	f.p = New(svc, f.ext)
	f.ext.result = f.readable()
	return f
}

// readable is an extraction that posts, dated within the household's window.
func (f *fixture) readable() vision.Extraction {
	h, _ := f.svc.GetHousehold(f.ctx, f.hh)
	date := h.Today().AddDate(0, 0, -1).Format(models.DateOnly)
	return vision.Extraction{
		IsReceipt: true, Merchant: str("King Soopers"), PurchaseDate: &date,
		Currency: str("USD"), TransactionType: "purchase", Total: num(84.2),
		Tender: "card", CardLast4: str("4821"), Category: str("Groceries"),
		LineItems: []vision.LineItem{{Description: "MILK", Amount: 4.29}},
	}
}

// upload stores a new, distinct photo and returns the receipt id.
func (f *fixture) upload() int {
	f.t.Helper()
	f.uploaded++
	img := []byte(fmt.Sprintf("photo %s %d", f.t.Name(), f.uploaded))
	sum := sha256.Sum256(img)
	r, created, err := f.svc.CreateReceipt(f.ctx, f.hh, ledger.NewReceipt{Image: img, MediaType: "image/jpeg", SHA256: sum[:]})
	if err != nil || !created {
		f.t.Fatalf("CreateReceipt: created=%v err=%v", created, err)
	}
	return r.ID
}

func (f *fixture) receipt(id int) models.Receipt {
	f.t.Helper()
	r, err := f.svc.GetReceipt(f.ctx, f.hh, id)
	if err != nil {
		f.t.Fatalf("GetReceipt(%d): %v", id, err)
	}
	return r
}

func (f *fixture) process(id int) {
	f.t.Helper()
	if err := f.p.Process(f.ctx, f.hh, id); err != nil {
		f.t.Fatalf("Process(%d): %v", id, err)
	}
}

func (f *fixture) receiptTxnCount() int {
	f.t.Helper()
	var n int
	if err := f.svc.DB().QueryRow(
		`SELECT COUNT(*) FROM transactions WHERE household_id = $1 AND source = 'receipt'`, f.hh,
	).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func (f *fixture) exec(q string, args ...any) {
	f.t.Helper()
	if _, err := f.svc.DB().Exec(q, args...); err != nil {
		f.t.Fatal(err)
	}
}

func TestProcessPosts(t *testing.T) {
	f := newFixture(t)
	id := f.upload()
	f.process(id)

	r := f.receipt(id)
	if r.Status != models.ReceiptPosted || r.TransactionID == nil {
		t.Fatalf("status = %s reasons = %v", r.Status, r.ReviewReasons)
	}
	if r.Model == nil || *r.Model != "fake-model" || r.CardLast4 == nil || len(r.LineItems) == 0 {
		t.Errorf("extracted fields not stored: %+v", r)
	}
	txn, err := f.svc.GetTransaction(f.ctx, f.hh, *r.TransactionID)
	if err != nil {
		t.Fatal(err)
	}
	if txn.Source != models.SourceReceipt || txn.Amount != -84.2 || txn.Kind != models.KindExpense ||
		txn.AccountID != f.card.ID || *txn.CategoryID != f.grocery.ID {
		t.Errorf("transaction = %+v", txn)
	}
	if txn.ReceiptID == nil || *txn.ReceiptID != id {
		t.Errorf("transaction receipt_id = %v, want %d", txn.ReceiptID, id)
	}

	// The register is a separate query; it must carry the link too.
	reg, err := f.svc.Register(f.ctx, f.hh, f.card.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(reg) != 1 || reg[0].ReceiptID == nil || *reg[0].ReceiptID != id {
		t.Errorf("register row receipt_id = %+v", reg)
	}
}

// The upload request and the scheduler can both reach a receipt at once. Only
// one may pay for a model call, and only one transaction may result.
func TestConcurrentProcessCallsModelOnce(t *testing.T) {
	f := newFixture(t)
	f.ext.block = make(chan struct{})
	id := f.upload()

	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := f.p.Process(f.ctx, f.hh, id); err != nil {
				t.Error(err)
			}
		}()
	}
	// Let the loser's claim attempt fail, then release the winner.
	time.Sleep(200 * time.Millisecond)
	close(f.ext.block)
	wg.Wait()

	if n := f.ext.callCount(); n != 1 {
		t.Errorf("model called %d times, want 1", n)
	}
	if n := f.receiptTxnCount(); n != 1 {
		t.Errorf("%d transactions written, want 1", n)
	}
}

// A worker that died mid-call leaves its claim behind. Once the lease expires
// another may take over — and the original, if it was only slow, must not post
// a second time when it finally returns.
func TestExpiredLeaseIsTakenOverAndFenced(t *testing.T) {
	f := newFixture(t)
	id := f.upload()

	stale, ok, err := f.svc.ClaimReceipt(f.ctx, f.hh, id, Lease)
	if err != nil || !ok {
		t.Fatalf("first claim: ok=%v err=%v", ok, err)
	}
	// Still leased: nobody else may have it.
	f.process(id)
	if f.ext.callCount() != 0 {
		t.Fatal("a live lease was taken over")
	}

	f.exec(`UPDATE receipts SET claimed_at = NOW() - interval '10 minutes' WHERE id = $1`, id)
	f.process(id)
	if r := f.receipt(id); r.Status != models.ReceiptPosted {
		t.Fatalf("after takeover status = %s", r.Status)
	}

	in := ledger.TransactionInput{AccountID: f.card.ID, Date: "2026-09-01", Amount: 1, Description: "stale"}
	err = f.svc.FinishReceipt(f.ctx, f.hh, stale, ledger.ReceiptOutcome{Post: &in})
	if !errors.Is(err, ledger.ErrLostClaim) {
		t.Errorf("stale finish: want ErrLostClaim, got %v", err)
	}
	if n := f.receiptTxnCount(); n != 1 {
		t.Errorf("%d transactions, want 1 — the stale finish should have rolled back", n)
	}
}

func TestTransientFailureBacksOffThenGivesUp(t *testing.T) {
	f := newFixture(t)
	f.ext.err = fmt.Errorf("%w: overloaded", vision.ErrUnavailable)
	id := f.upload()

	f.process(id)
	r := f.receipt(id)
	if r.Status != models.ReceiptPending || r.ExtractError == nil {
		t.Fatalf("after one failure: status %s, error %v", r.Status, r.ExtractError)
	}
	// Backing off: not claimable, so no second call.
	f.process(id)
	if n := f.ext.callCount(); n != 1 {
		t.Fatalf("retried during backoff: %d calls", n)
	}

	f.exec(`UPDATE receipts SET failures = $2, retry_after = NULL WHERE id = $1`, id, maxFailures-1)
	f.process(id)
	r = f.receipt(id)
	if r.Status != models.ReceiptNeedsReview || !slices.Contains(r.ReviewReasons, models.ReasonExtractionFailed) {
		t.Errorf("after the last failure: status %s, reasons %v", r.Status, r.ReviewReasons)
	}
}

func TestUnreadableGoesStraightToReview(t *testing.T) {
	f := newFixture(t)
	f.ext.err = fmt.Errorf("%w: declined", vision.ErrUnreadable)
	id := f.upload()
	f.process(id)
	if r := f.receipt(id); r.Status != models.ReceiptNeedsReview || !slices.Contains(r.ReviewReasons, models.ReasonUnreadable) {
		t.Errorf("status %s, reasons %v", r.Status, r.ReviewReasons)
	}
}

// Shutting down mid-call says nothing about the receipt. It must go back to
// pending uncounted, ready for the next claimer.
func TestCancellationIsNotAFailure(t *testing.T) {
	f := newFixture(t)
	f.ext.block = make(chan struct{})
	id := f.upload()

	ctx, cancel := context.WithCancel(f.ctx)
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	if err := f.p.Process(ctx, f.hh, id); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}

	var status string
	var failures int
	if err := f.svc.DB().QueryRow(`SELECT status, failures FROM receipts WHERE id = $1`, id).Scan(&status, &failures); err != nil {
		t.Fatal(err)
	}
	if status != models.ReceiptPending || failures != 0 {
		t.Errorf("status %s failures %d, want pending with none counted", status, failures)
	}
}

func TestDisabledHoldsForReview(t *testing.T) {
	f := newFixture(t)
	f.p = New(f.svc, nil)
	id := f.upload()
	f.process(id)
	if r := f.receipt(id); r.Status != models.ReceiptNeedsReview || !slices.Contains(r.ReviewReasons, models.ReasonExtractionDisabled) {
		t.Errorf("status %s, reasons %v", r.Status, r.ReviewReasons)
	}
}

func TestSamePhotoIsOneReceipt(t *testing.T) {
	f := newFixture(t)
	img := []byte("the same photo twice")
	sum := sha256.Sum256(img)
	in := ledger.NewReceipt{Image: img, MediaType: "image/jpeg", SHA256: sum[:]}

	a, created, err := f.svc.CreateReceipt(f.ctx, f.hh, in)
	if err != nil || !created {
		t.Fatal(created, err)
	}
	b, created, err := f.svc.CreateReceipt(f.ctx, f.hh, in)
	if err != nil || created || b.ID != a.ID {
		t.Errorf("second upload: created=%v id=%d err=%v, want the first receipt back", created, b.ID, err)
	}
}

// The ledger is kept by hand, so a receipt is often photographed after the
// expense was already typed in.
func TestAlreadyLoggedIsHeldAsDuplicate(t *testing.T) {
	f := newFixture(t)
	x := f.readable()
	if _, err := f.svc.CreateTransaction(f.ctx, f.hh, ledger.TransactionInput{
		AccountID: f.card.ID, Date: *x.PurchaseDate, Amount: 84.2, Kind: models.KindExpense, Description: "groceries",
	}); err != nil {
		t.Fatal(err)
	}
	id := f.upload()
	f.process(id)
	if r := f.receipt(id); r.Status != models.ReceiptNeedsReview || !slices.Contains(r.ReviewReasons, models.ReasonPossibleDuplicate) {
		t.Errorf("status %s, reasons %v", r.Status, r.ReviewReasons)
	}
}

func TestPostHeldReceipt(t *testing.T) {
	f := newFixture(t)
	f.ext.result.CardLast4 = str("0000")
	id := f.upload()
	f.process(id)
	if r := f.receipt(id); r.Status != models.ReceiptNeedsReview {
		t.Fatalf("status %s, want held for the unknown card", r.Status)
	}

	in := ledger.TransactionInput{
		AccountID: f.card.ID, Date: *f.readable().PurchaseDate, Amount: 84.2,
		Kind: models.KindExpense, Description: "King Soopers", CategoryID: &f.grocery.ID,
	}
	txn, err := f.svc.PostHeldReceipt(f.ctx, f.hh, id, in)
	if err != nil {
		t.Fatal(err)
	}
	if txn.Source != models.SourceReceipt || txn.ReceiptID == nil || *txn.ReceiptID != id {
		t.Errorf("posted transaction = %+v", txn)
	}

	// A second click, or a second tab, must not post it again.
	var invalid ledger.ErrInvalid
	if _, err := f.svc.PostHeldReceipt(f.ctx, f.hh, id, in); !errors.As(err, &invalid) {
		t.Errorf("second post: want ErrInvalid, got %v", err)
	}
	if n := f.receiptTxnCount(); n != 1 {
		t.Errorf("%d transactions, want 1", n)
	}
	// Posted receipts go by deleting their transaction, not directly.
	if err := f.svc.DeleteReceipt(f.ctx, f.hh, id); !errors.As(err, &invalid) {
		t.Errorf("deleting a posted receipt: want ErrInvalid, got %v", err)
	}
}

func TestDeletingTransactionDeletesReceipt(t *testing.T) {
	f := newFixture(t)
	id := f.upload()
	f.process(id)
	r := f.receipt(id)
	if err := f.svc.DeleteTransaction(f.ctx, f.hh, *r.TransactionID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.GetReceipt(f.ctx, f.hh, id); !errors.Is(err, ledger.ErrNotFound) {
		t.Errorf("receipt should be gone with its transaction, got %v", err)
	}
}

func TestRetry(t *testing.T) {
	f := newFixture(t)
	f.ext.err = fmt.Errorf("%w: declined", vision.ErrUnreadable)
	id := f.upload()
	f.process(id)

	f.ext.mu.Lock()
	f.ext.err = nil
	f.ext.mu.Unlock()
	if _, err := f.svc.RetryReceipt(f.ctx, f.hh, id); err != nil {
		t.Fatal(err)
	}
	f.process(id)
	if r := f.receipt(id); r.Status != models.ReceiptPosted {
		t.Errorf("after retry status %s, reasons %v", r.Status, r.ReviewReasons)
	}
}

// Start is the upload path: detached from the request, finished even if nobody
// waits for it, and drained by Wait.
func TestStartAndWait(t *testing.T) {
	f := newFixture(t)
	id := f.upload()
	done := f.p.Start(f.hh, id)
	if done == nil {
		t.Fatal("Start refused with free slots")
	}
	ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
	defer cancel()
	f.p.Wait(ctx)
	if r := f.receipt(id); r.Status != models.ReceiptPosted {
		t.Errorf("status %s", r.Status)
	}
}
