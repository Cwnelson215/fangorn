package ledger_test

import (
	"errors"
	"testing"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
)

// Closing an account the way the app does it: move what's left out with a
// transfer, then delete. Net worth never moves, and the surviving account keeps
// its side of every transfer.
func TestCloseAccountKeepsNetWorth(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Checking", models.AccountChecking, 1000)
	savings := f.account("Old savings", models.AccountSavings, 0)

	f.transfer(checking.ID, savings.ID, 300, "2026-02-01")
	// Remove remaining balance.
	f.transfer(savings.ID, checking.ID, 300, "2026-03-01")
	if got := f.balance(savings.ID); got != 0 {
		t.Fatalf("savings after moving its balance out = %v, want 0", got)
	}

	if err := f.svc.DeleteAccount(f.ctx, f.hh, savings.ID); err != nil {
		t.Fatal(err)
	}
	if got := f.balance(checking.ID); got != 1000 {
		t.Errorf("checking = %v after the delete, want 1000", got)
	}
	if got := f.dashboard("2026-01-01", "2026-12-31").NetWorth; got != 1000 {
		t.Errorf("net worth = %v, want 1000", got)
	}

	transfers, err := f.svc.ListTransfers(f.ctx, f.hh, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(transfers) != 2 {
		t.Fatalf("got %d transfers, want both kept on checking", len(transfers))
	}
	for _, tr := range transfers {
		if tr.FromAccount != ledger.DeletedAccountName && tr.ToAccount != ledger.DeletedAccountName {
			t.Errorf("transfer %s → %s doesn't name the deleted side", tr.FromAccount, tr.ToAccount)
		}
	}
}

// Dropping a balance takes it off net worth without it becoming spending,
// income or anything a budget sees.
func TestDropBalance(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Checking", models.AccountChecking, 1000)
	jar := f.account("Cash jar", models.AccountCash, 80)
	card := f.account("Old card", models.AccountCreditCard, -40)
	food := f.category("Groceries", models.KindExpense)
	f.txn(jar.ID, models.KindExpense, "2026-03-02", 30, &food.ID)

	before := f.dashboard("2026-01-01", "2026-12-31")

	drop, err := f.svc.DropBalance(f.ctx, f.hh, jar.ID, "2026-03-10")
	if err != nil {
		t.Fatal(err)
	}
	if drop.Kind != models.KindAdjustment || drop.Amount != -50 || drop.CategoryID != nil {
		t.Errorf("drop = %+v, want a -50 uncategorized adjustment", drop)
	}
	if got := f.balance(jar.ID); got != 0 {
		t.Errorf("jar = %v after the drop, want 0", got)
	}
	// A debt dropped the other way: the card's -40 is written back up to 0.
	if _, err := f.svc.DropBalance(f.ctx, f.hh, card.ID, "2026-03-10"); err != nil {
		t.Fatal(err)
	}
	if got := f.balance(card.ID); got != 0 {
		t.Errorf("card = %v after the drop, want 0", got)
	}

	after := f.dashboard("2026-01-01", "2026-12-31")
	if after.Expenses != before.Expenses || after.Income != before.Income {
		t.Errorf("totals moved: expenses %v→%v income %v→%v", before.Expenses, after.Expenses, before.Income, after.Income)
	}
	if want := before.NetWorth - 50 + 40; after.NetWorth != want {
		t.Errorf("net worth = %v, want %v", after.NetWorth, want)
	}
	spend, err := f.svc.SpendingBreakdown(f.ctx, f.hh, ledger.BreakdownFilter{Side: "expense", GroupBy: "category"})
	if err != nil {
		t.Fatal(err)
	}
	if len(spend) != 1 || spend[0].Amount != 30 {
		t.Errorf("spending = %+v, want only the $30 of groceries", spend)
	}

	_, err = f.svc.DropBalance(f.ctx, f.hh, jar.ID, "2026-03-11")
	wantInvalid(t, err)
	_, err = f.svc.UpdateTransaction(f.ctx, f.hh, drop.ID, ledger.TransactionInput{
		AccountID: jar.ID, Kind: models.KindExpense, Date: "2026-03-10", Amount: 50, Description: "x",
	})
	wantInvalid(t, err)

	other := newFixture(t)
	if _, err := other.svc.DropBalance(other.ctx, other.hh, checking.ID, "2026-03-10"); !errors.Is(err, ledger.ErrNotFound) {
		t.Errorf("another household dropped a balance: %v", err)
	}
}
