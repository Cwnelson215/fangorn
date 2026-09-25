package ledger_test

import (
	"testing"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
)

func TestCategoryDefaultAccount(t *testing.T) {
	f := newFixture(t)
	visa := f.account("Visa", models.AccountCreditCard, 0)

	c, err := f.svc.CreateCategory(f.ctx, f.hh, ledger.CategoryInput{
		Name: "Gas", Kind: models.KindExpense, DefaultAccountID: &visa.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.DefaultAccountID == nil || *c.DefaultAccountID != visa.ID {
		t.Fatalf("default account = %v, want %d", c.DefaultAccountID, visa.ID)
	}

	other := newFixture(t)
	theirs := other.account("Their card", models.AccountCreditCard, 0)
	_, err = f.svc.UpdateCategory(f.ctx, f.hh, c.ID, ledger.CategoryInput{
		Name: "Gas", Kind: models.KindExpense, DefaultAccountID: &theirs.ID,
	})
	wantInvalid(t, err)

	old := f.account("Old card", models.AccountCreditCard, 0)
	if err := f.svc.SetAccountArchived(f.ctx, f.hh, old.ID, true); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.UpdateCategory(f.ctx, f.hh, c.ID, ledger.CategoryInput{
		Name: "Gas", Kind: models.KindExpense, DefaultAccountID: &old.ID,
	})
	wantInvalid(t, err)

	// Deleting the account clears the routing rather than the category.
	if err := f.svc.DeleteAccount(f.ctx, f.hh, visa.ID); err != nil {
		t.Fatal(err)
	}
	cats, err := f.svc.ListCategories(f.ctx, f.hh, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, got := range cats {
		if got.ID == c.ID && got.DefaultAccountID != nil {
			t.Errorf("default account = %d after the account was deleted", *got.DefaultAccountID)
		}
	}
}
