package scheduler

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
	"github.com/cwnelson/fangorn/internal/receipts"
	"github.com/cwnelson/fangorn/internal/vision"
)

type readsEverything struct{ x vision.Extraction }

func (r readsEverything) Extract(context.Context, []byte, string, vision.Hints) (vision.Extraction, vision.Meta, error) {
	return r.x, vision.Meta{Model: "fake"}, nil
}

// A receipt the upload did not finish — the phone gave up waiting, or the
// server restarted mid-call — is finished by the next pass, before the
// net worth snapshot.
func TestRunHouseholdFinishesReceipts(t *testing.T) {
	f := newFixture(t)
	mask := "4821"
	card, err := f.svc.CreateAccount(f.ctx, f.household.ID, ledger.AccountInput{
		Name: "Visa", Type: models.AccountCreditCard, Mask: &mask, StartingBalanceDate: "2025-01-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateCategory(f.ctx, f.household.ID, ledger.CategoryInput{Name: "Groceries", Kind: models.KindExpense}); err != nil {
		t.Fatal(err)
	}

	date := f.today().AddDate(0, 0, -1).Format(models.DateOnly)
	merchant, last4, category, total := "King Soopers", "4821", "Groceries", 42.5
	ext := readsEverything{vision.Extraction{
		IsReceipt: true, Merchant: &merchant, PurchaseDate: &date, TransactionType: "purchase",
		Total: &total, Tender: "card", CardLast4: &last4, Category: &category,
	}}
	f.sched = New(f.svc, nil, receipts.New(f.svc, ext), time.Minute, 60)

	img := []byte("photo " + t.Name())
	sum := sha256.Sum256(img)
	r, _, err := f.svc.CreateReceipt(f.ctx, f.household.ID, ledger.NewReceipt{Image: img, MediaType: "image/jpeg", SHA256: sum[:]})
	if err != nil {
		t.Fatal(err)
	}

	f.run()

	if got, _ := f.svc.GetReceipt(f.ctx, f.household.ID, r.ID); got.Status != models.ReceiptPosted {
		t.Fatalf("receipt status = %s, reasons %v", got.Status, got.ReviewReasons)
	}
	acct, err := f.svc.GetAccount(f.ctx, f.household.ID, card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if acct.Balance != -42.5 {
		t.Errorf("card balance = %v, want -42.5", acct.Balance)
	}
}
