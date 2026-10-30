package ledger_test

import (
	"math"
	"testing"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
)

// A transfer between institutions leaves the source on its date and lands on
// its completion date: until then the destination's balance doesn't have it.
func TestTransferCompletionDate(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Gesa Checking", models.AccountChecking, 1000)
	brokerage := f.account("Fidelity", models.AccountSavings, 0)

	today := day(f.today())
	sent := today.AddDate(0, 0, -1).Format(models.DateOnly)
	lands := today.AddDate(0, 0, 2).Format(models.DateOnly)

	tr, err := f.svc.CreateTransfer(f.ctx, f.hh, ledger.TransferInput{
		FromAccountID: checking.ID, ToAccountID: brokerage.ID, Amount: 400,
		Date: sent, CompletionDate: &lands,
	})
	if err != nil {
		t.Fatal(err)
	}
	if tr.Date != sent || tr.CompletionDate == nil || *tr.CompletionDate != lands || !tr.InTransit {
		t.Fatalf("transfer = %+v, want sent %s landing %s in transit", tr, sent, lands)
	}
	money(t, "source", f.balance(checking.ID), 600)
	money(t, "destination before landing", f.balance(brokerage.ID), 0)

	// Dropping the destination's balance would be undone when the money lands.
	if _, err := f.svc.DropBalance(f.ctx, f.hh, brokerage.ID, f.today()); !isInvalid(err) {
		t.Fatalf("DropBalance with money in transit: err = %v, want invalid", err)
	}

	// Moving the completion date to today lands it.
	todayStr := f.today()
	tr, err = f.svc.UpdateTransfer(f.ctx, f.hh, tr.GroupID, ledger.TransferInput{
		FromAccountID: checking.ID, ToAccountID: brokerage.ID, Amount: 400,
		Date: sent, CompletionDate: &todayStr,
	})
	if err != nil {
		t.Fatal(err)
	}
	if tr.InTransit || tr.Date != sent {
		t.Fatalf("transfer = %+v, want landed, sent %s", tr, sent)
	}
	money(t, "destination after landing", f.balance(brokerage.ID), 400)

	// Same day as the transfer is the same as none.
	same := sent
	tr, err = f.svc.UpdateTransfer(f.ctx, f.hh, tr.GroupID, ledger.TransferInput{
		FromAccountID: checking.ID, ToAccountID: brokerage.ID, Amount: 400,
		Date: sent, CompletionDate: &same,
	})
	if err != nil {
		t.Fatal(err)
	}
	if tr.CompletionDate != nil {
		t.Fatalf("completion date = %v, want none", *tr.CompletionDate)
	}

	before := today.AddDate(0, 0, -5).Format(models.DateOnly)
	_, err = f.svc.CreateTransfer(f.ctx, f.hh, ledger.TransferInput{
		FromAccountID: checking.ID, ToAccountID: brokerage.ID, Amount: 1,
		Date: sent, CompletionDate: &before,
	})
	wantInvalid(t, err)
}

// The destination earns on the money from the day it lands, not the day it
// was sent; the source earns on it until it leaves.
func TestTransferEarnsFromCompletionDate(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Gesa Checking", models.AccountChecking, 10000)
	hysa := f.highYield("Ally Savings", 0, "2026-06-01", 6)

	lands := "2026-06-21"
	if _, err := f.svc.CreateTransfer(f.ctx, f.hh, ledger.TransferInput{
		FromAccountID: checking.ID, ToAccountID: hysa.ID, Amount: 3000,
		Date: "2026-06-16", CompletionDate: &lands,
	}); err != nil {
		t.Fatal(err)
	}
	f.postInterest("2026-07-02")
	txns := f.interestTxns(hysa.ID)
	if len(txns) != 1 {
		t.Fatalf("interest = %+v, want June only", txns)
	}

	// Compare with the same money landing on the 16th: 5 fewer days of it.
	g := newFixture(t)
	checking2 := g.account("Gesa Checking", models.AccountChecking, 10000)
	hysa2 := g.highYield("Ally Savings", 0, "2026-06-01", 6)
	g.transfer(checking2.ID, hysa2.ID, 3000, "2026-06-16")
	g.postInterest("2026-07-02")
	early := g.interestTxns(hysa2.ID)
	if len(early) != 1 || early[0].Amount <= txns[0].Amount {
		t.Fatalf("landing the 21st earned %v, the 16th %+v — want the 21st less", txns[0].Amount, early)
	}
	// June has 30 days: the 21st through the 30th is 10 of the 15 days from the 16th.
	// Each is rounded to the cent, so allow one.
	if want := early[0].Amount * 10 / 15; math.Abs(txns[0].Amount-want) > 0.011 {
		t.Fatalf("June interest = %v, want about %v", txns[0].Amount, want)
	}
}
