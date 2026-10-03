package ledger_test

import (
	"testing"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
)

func (f *fixture) debts(month string) map[string]models.DebtLine {
	f.t.Helper()
	bm, err := f.svc.BudgetMonth(f.ctx, f.hh, month)
	if err != nil {
		f.t.Fatal(err)
	}
	out := map[string]models.DebtLine{}
	for _, l := range bm.Debts {
		out[l.AccountName] = l
	}
	return out
}

// Paying a card counts toward the month only beyond what was charged to it:
// the purchases already counted as spending in their own categories.
func TestDebtPaydown(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Checking", models.AccountChecking, 5000)
	visa := f.account("Visa", models.AccountCreditCard, -1000)
	other := f.account("Other card", models.AccountCreditCard, 0)
	loan := f.account("Car loan", models.AccountLoan, -8000)
	groceries := f.category("Groceries", models.KindExpense)

	// March: $400 charged, $700 paid — $300 of old balance paid down.
	f.txn(visa.ID, models.KindExpense, "2026-03-05", 400, &groceries.ID)
	payment := f.transfer(checking.ID, visa.ID, 700, "2026-03-20")
	f.transfer(checking.ID, loan.ID, 350, "2026-03-15")
	// Moving a balance between cards pays nothing down.
	f.transfer(visa.ID, other.ID, 100, "2026-03-25")
	// April: paid less than was charged.
	f.txn(visa.ID, models.KindExpense, "2026-04-05", 400, &groceries.ID)
	f.transfer(checking.ID, visa.ID, 200, "2026-04-20")

	march := f.debts("2026-03")
	if got := march["Visa"]; got.Paid != 700 || got.Charged != 500 || got.Paydown != 200 {
		t.Errorf("Visa in March = %+v, want paid 700, charged 500 (400 + 100 moved off), paydown 200", got)
	}
	if got := march["Car loan"]; got.Paid != 350 || got.Paydown != 350 {
		t.Errorf("loan in March = %+v, want the whole 350 as paydown", got)
	}
	if got := march["Other card"]; got.Paid != 0 || got.Paydown != 0 {
		t.Errorf("other card in March = %+v, want nothing paid by a balance transfer", got)
	}
	if got := f.debts("2026-04")["Visa"]; got.Paid != 200 || got.Paydown != 0 {
		t.Errorf("Visa in April = %+v, want paid 200, paydown 0", got)
	}

	// Both legs of the payment are marked, the purchase is not.
	txns, err := f.svc.ListTransactions(f.ctx, f.hh, ledger.TransactionFilter{From: "2026-03-01", To: "2026-03-31"})
	if err != nil {
		t.Fatal(err)
	}
	marked := 0
	for _, txn := range txns {
		if txn.DebtPayment {
			marked++
			if txn.TransferGroupID == nil || (*txn.TransferGroupID != payment.GroupID && txn.Amount != 350 && txn.Amount != -350) {
				t.Errorf("%s %v on %s marked as a debt payment", txn.Kind, txn.Amount, txn.AccountName)
			}
		}
	}
	if marked != 4 {
		t.Errorf("%d rows marked as debt payments, want 4 (two legs each of two payments)", marked)
	}
	register, err := f.svc.Register(f.ctx, f.hh, visa.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, txn := range register {
		if want := txn.Kind == models.KindTransfer && txn.Amount > 0; txn.DebtPayment != want {
			t.Errorf("register row %s %v: debt payment = %v, want %v", txn.Kind, txn.Amount, txn.DebtPayment, want)
		}
	}
}

// A planned paydown applies from its month on; earlier months keep theirs.
func TestDebtPlan(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Checking", models.AccountChecking, 5000)
	visa := f.account("Visa", models.AccountCreditCard, -1000)
	set := func(accountID int, amount float64, month string) error {
		return f.svc.SetDebtPlan(f.ctx, f.hh, ledger.DebtPlanInput{AccountID: accountID, Amount: &amount, EffectiveFrom: month})
	}

	if err := set(visa.ID, 300, "2026-03"); err != nil {
		t.Fatal(err)
	}
	if err := set(visa.ID, 500, "2026-05"); err != nil {
		t.Fatal(err)
	}
	for month, want := range map[string]float64{"2026-02": 0, "2026-03": 300, "2026-04": 300, "2026-05": 500} {
		if got := f.debts(month)["Visa"].Monthly; got != want {
			t.Errorf("plan in %s = %v, want %v", month, got, want)
		}
	}
	if err := set(visa.ID, 0, "2026-06"); err != nil {
		t.Fatal(err)
	}
	if got := f.debts("2026-07")["Visa"].Monthly; got != 0 {
		t.Errorf("plan after stopping = %v, want 0", got)
	}
	if err := set(checking.ID, 100, "2026-03"); err == nil {
		t.Error("a checking account took a planned paydown")
	}
}
