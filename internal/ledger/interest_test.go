package ledger_test

import (
	"math"
	"testing"
	"time"

	"github.com/cwnelson/fangorn/internal/interest"
	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
)

func (f *fixture) highYield(name string, starting float64, since string, apy float64) models.Account {
	f.t.Helper()
	a, err := f.svc.CreateAccount(f.ctx, f.hh, ledger.AccountInput{
		Name: name, Type: models.AccountHighYieldSavings, APY: &apy,
		StartingBalance: starting, StartingBalanceDate: since,
	})
	if err != nil {
		f.t.Fatalf("CreateAccount(%s): %v", name, err)
	}
	return a
}

func (f *fixture) postInterest(today string) int {
	f.t.Helper()
	n, err := f.svc.PostInterest(f.ctx, f.hh, day(today))
	if err != nil {
		f.t.Fatalf("PostInterest(%s): %v", today, err)
	}
	return n
}

func (f *fixture) interestTxns(accountID int) []models.Transaction {
	f.t.Helper()
	txns, err := f.svc.ListTransactions(f.ctx, f.hh, ledger.TransactionFilter{AccountID: accountID})
	if err != nil {
		f.t.Fatal(err)
	}
	var out []models.Transaction
	for _, t := range txns {
		if t.Source == models.SourceInterest {
			out = append(out, t)
		}
	}
	// Oldest first, to read like the months they cover.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func day(s string) time.Time {
	t, err := time.Parse(models.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestInterestPostsFinishedMonths(t *testing.T) {
	f := newFixture(t)
	hysa := f.highYield("Ally Savings", 10000, "2026-06-15", 4.35)
	if hysa.APY == nil || *hysa.APY != 4.35 {
		t.Fatalf("apy = %v, want 4.35", hysa.APY)
	}

	// Early September: June (from the 15th), July and August are over;
	// September is still earning.
	if n := f.postInterest("2026-09-10"); n != 3 {
		t.Fatalf("posted %d months, want 3", n)
	}
	txns := f.interestTxns(hysa.ID)
	if len(txns) != 3 {
		t.Fatalf("interest transactions = %+v", txns)
	}

	rates := []interest.Rate{{From: day("2026-06-15"), APY: 4.35}}
	balance := 10000.0
	for i, m := range []string{"2026-06-01", "2026-07-01", "2026-08-01"} {
		want := interest.ForMonth(day(m), balance, rates, day("2026-06-15"))
		got := txns[i]
		if got.Date != interest.MonthEnd(day(m)).Format(models.DateOnly) {
			t.Errorf("%s interest dated %s, want the month's last day", m, got.Date)
		}
		money(t, m+" interest", got.Amount, want)
		if got.Kind != models.KindIncome || got.CategoryName == nil || *got.CategoryName != "Interest" {
			t.Errorf("%s interest = %+v, want income under Interest", m, got)
		}
		balance += want // compounds into the next month's balance
	}
	// June only earned from the 15th: 16 of 30 days.
	money(t, "June is 16/30 of a month", txns[0].Amount, 18.96)

	money(t, "balance", f.accountByID(hysa.ID).Balance, balance)

	// Running again — the next tick, or a second process — changes nothing.
	if n := f.postInterest("2026-09-10"); n != 0 {
		t.Errorf("second pass posted %d", n)
	}
	if n := f.postInterest("2026-09-30"); n != 0 {
		t.Errorf("September posted before it was over (%d)", n)
	}
	if n := f.postInterest("2026-10-01"); n != 1 {
		t.Errorf("October 1st posted %d, want September's", n)
	}
}

func TestInterestUsesMonthEndBalance(t *testing.T) {
	f := newFixture(t)
	hysa := f.highYield("HYSA", 1000, "2026-08-01", 4.00)
	income := f.category("Paycheck", models.KindIncome)
	// Deposited on the last day: month-end balance counts it for the whole month.
	f.txn(hysa.ID, models.KindIncome, "2026-08-31", 9000, &income.ID)

	f.postInterest("2026-09-02")
	txns := f.interestTxns(hysa.ID)
	if len(txns) != 1 {
		t.Fatalf("interest = %+v", txns)
	}
	money(t, "August on $10,000", txns[0].Amount, interest.ForMonth(day("2026-08-01"), 10000,
		[]interest.Rate{{From: day("2026-08-01"), APY: 4.00}}, day("2026-08-01")))
}

func TestInterestRateChangeMidMonth(t *testing.T) {
	f := newFixture(t)
	hysa := f.highYield("HYSA", 10000, "2026-07-01", 4.00)
	if _, err := f.svc.AddSavingsRate(f.ctx, f.hh, hysa.ID, ledger.SavingsRateInput{
		APY: 4.35, EffectiveFrom: "2026-08-12",
	}); err != nil {
		t.Fatal(err)
	}
	f.postInterest("2026-09-01")
	txns := f.interestTxns(hysa.ID)
	if len(txns) != 2 {
		t.Fatalf("interest = %+v", txns)
	}
	rates := []interest.Rate{{From: day("2026-07-01"), APY: 4.00}, {From: day("2026-08-12"), APY: 4.35}}
	july := interest.ForMonth(day("2026-07-01"), 10000, rates, day("2026-07-01"))
	money(t, "July at 4.00%", txns[0].Amount, july)
	money(t, "August blended", txns[1].Amount, interest.ForMonth(day("2026-08-01"), 10000+july, rates, day("2026-07-01")))

	if a := f.accountByID(hysa.ID); a.APY == nil || *a.APY != 4.35 {
		t.Errorf("current apy = %v, want 4.35", a.APY)
	}
}

// Deleting a posted interest transaction means "that didn't happen" — the month
// is not worked out again on the next tick.
func TestDeletedInterestIsNotReposted(t *testing.T) {
	f := newFixture(t)
	hysa := f.highYield("HYSA", 5000, "2026-08-01", 4.35)
	f.postInterest("2026-09-01")
	txns := f.interestTxns(hysa.ID)
	if len(txns) != 1 {
		t.Fatalf("interest = %+v", txns)
	}
	if err := f.svc.DeleteTransaction(f.ctx, f.hh, txns[0].ID); err != nil {
		t.Fatal(err)
	}
	if n := f.postInterest("2026-09-02"); n != 0 {
		t.Errorf("reposted %d after the transaction was deleted", n)
	}
}

func TestInterestReusesExistingCategory(t *testing.T) {
	f := newFixture(t)
	existing := f.category("interest", models.KindIncome)
	hysa := f.highYield("HYSA", 5000, "2026-08-01", 4.35)
	f.postInterest("2026-09-01")
	txns := f.interestTxns(hysa.ID)
	if len(txns) != 1 || txns[0].CategoryID == nil || *txns[0].CategoryID != existing.ID {
		t.Fatalf("interest = %+v, want it filed under the existing category %d", txns, existing.ID)
	}
}

func TestSavingsRateRules(t *testing.T) {
	f := newFixture(t)
	apy := 4.35

	_, err := f.svc.CreateAccount(f.ctx, f.hh, ledger.AccountInput{
		Name: "No rate", Type: models.AccountHighYieldSavings, StartingBalanceDate: "2026-01-01",
	})
	wantInvalid(t, err)
	_, err = f.svc.CreateAccount(f.ctx, f.hh, ledger.AccountInput{
		Name: "Plain", Type: models.AccountSavings, APY: &apy, StartingBalanceDate: "2026-01-01",
	})
	wantInvalid(t, err)
	bad := 120.0
	_, err = f.svc.CreateAccount(f.ctx, f.hh, ledger.AccountInput{
		Name: "Too high", Type: models.AccountHighYieldSavings, APY: &bad, StartingBalanceDate: "2026-01-01",
	})
	wantInvalid(t, err)

	savings := f.account("Savings", models.AccountSavings, 100)
	_, err = f.svc.AddSavingsRate(f.ctx, f.hh, savings.ID, ledger.SavingsRateInput{APY: 1, EffectiveFrom: "2026-02-01"})
	wantInvalid(t, err)

	hysa := f.highYield("HYSA", 100, "2026-01-01", 4.35)
	_, err = f.svc.UpdateAccount(f.ctx, f.hh, hysa.ID, ledger.AccountInput{
		Name: "HYSA", Type: models.AccountSavings, StartingBalance: 100, StartingBalanceDate: "2026-01-01",
	})
	wantInvalid(t, err) // would strand its rate history
	_, err = f.svc.UpdateAccount(f.ctx, f.hh, hysa.ID, ledger.AccountInput{
		Name: "HYSA", Type: models.AccountHighYieldSavings, APY: &apy,
		StartingBalance: 100, StartingBalanceDate: "2026-01-01",
	})
	wantInvalid(t, err) // rate changes go through the history
	if _, err = f.svc.UpdateAccount(f.ctx, f.hh, hysa.ID, ledger.AccountInput{
		Name: "Ally HYSA", Type: models.AccountHighYieldSavings, StartingBalance: 100, StartingBalanceDate: "2026-01-01",
	}); err != nil {
		t.Errorf("renaming: %v", err)
	}

	rates, err := f.svc.ListSavingsRates(f.ctx, f.hh, hysa.ID)
	if err != nil || len(rates) != 1 {
		t.Fatalf("rates = %+v, %v", rates, err)
	}
	wantInvalid(t, f.svc.DeleteSavingsRate(f.ctx, f.hh, hysa.ID, rates[0].ID)) // the only one

	// Same day again replaces rather than duplicates.
	if _, err := f.svc.AddSavingsRate(f.ctx, f.hh, hysa.ID, ledger.SavingsRateInput{APY: 4.1, EffectiveFrom: "2026-01-01"}); err != nil {
		t.Fatal(err)
	}
	if rates, _ = f.svc.ListSavingsRates(f.ctx, f.hh, hysa.ID); len(rates) != 1 || rates[0].APY != 4.1 {
		t.Errorf("rates after same-day change = %+v", rates)
	}
}

func TestSavingsOutlook(t *testing.T) {
	f := newFixture(t)
	hysa := f.highYield("HYSA", 12000, "2026-01-01", 4.35)
	out, err := f.svc.SavingsOutlookFor(f.ctx, f.hh, hysa.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Rates) != 1 || out.ProjectedDate == "" || out.ProjectedAmount <= 0 {
		t.Errorf("outlook = %+v", out)
	}
	savings := f.account("Savings", models.AccountSavings, 100)
	_, err = f.svc.SavingsOutlookFor(f.ctx, f.hh, savings.ID)
	wantInvalid(t, err)
}

// An investment account's uninvested cash sits in a money market fund (SPAXX)
// that pays a monthly dividend. With a cash yield set, that posts like savings
// interest — on the cash only, not the holdings — and is filed as a dividend.
func TestCashYieldOnInvestmentAccount(t *testing.T) {
	f := newFixture(t)
	brokerage, err := f.svc.CreateAccount(f.ctx, f.hh, ledger.AccountInput{
		Name: "Fidelity", Type: models.AccountInvestment, StartingBalance: 3000, StartingBalanceDate: "2026-07-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.trade(brokerage.ID, "buy", f.sym("FXAIX"), "2026-07-10", 10, 200) // $2,000 out of cash

	// No yield set: nothing is posted, and that's allowed.
	if n := f.postInterest("2026-09-01"); n != 0 {
		t.Fatalf("posted %d with no yield set", n)
	}

	if _, err := f.svc.AddSavingsRate(f.ctx, f.hh, brokerage.ID, ledger.SavingsRateInput{
		APY: 4.0, EffectiveFrom: "2026-08-01",
	}); err != nil {
		t.Fatalf("setting a cash yield: %v", err)
	}
	if n := f.postInterest("2026-09-01"); n != 1 {
		t.Fatalf("posted %d, want August's", n)
	}
	txns := f.interestTxns(brokerage.ID)
	if len(txns) != 1 {
		t.Fatalf("dividends = %+v", txns)
	}
	d := txns[0]
	want := interest.ForMonth(day("2026-08-01"), 1000, []interest.Rate{{From: day("2026-08-01"), APY: 4.0}}, day("2026-07-01"))
	money(t, "dividend on $1,000 cash, not $3,000", d.Amount, want)
	if d.Description != "Money market dividend" || d.CategoryName == nil || *d.CategoryName != "Dividends" {
		t.Errorf("dividend = %+v, want a Money market dividend under Dividends", d)
	}
	if d.Date != "2026-08-31" {
		t.Errorf("dated %s", d.Date)
	}

	// The yield is optional on an investment account: its last rate can go,
	// which turns the dividend off.
	rates, _ := f.svc.ListSavingsRates(f.ctx, f.hh, brokerage.ID)
	if err := f.svc.DeleteSavingsRate(f.ctx, f.hh, brokerage.ID, rates[0].ID); err != nil {
		t.Errorf("removing the only cash yield: %v", err)
	}
}

func TestCashYieldOnRetirementAccount(t *testing.T) {
	f := newFixture(t)
	roth := f.retirement("Roth IRA", models.TaxRoth, 500)
	if _, err := f.svc.AddSavingsRate(f.ctx, f.hh, roth.ID, ledger.SavingsRateInput{
		APY: 4.0, EffectiveFrom: "2026-08-01",
	}); err != nil {
		t.Fatalf("setting a cash yield on a Roth IRA: %v", err)
	}
	f.postInterest("2026-09-01")
	if txns := f.interestTxns(roth.ID); len(txns) != 1 || txns[0].Amount <= 0 {
		t.Errorf("Roth dividends = %+v", txns)
	}
}

// A linked cash fund takes over the account's yield from the day it's linked:
// hand-entered rates still cover the months before, the fund's daily published
// yields everything after, each a simple annual rate that earns yield ÷ 12.
func TestCashFundYieldTakesOverFromLink(t *testing.T) {
	f := newFixture(t)
	brokerage, err := f.svc.CreateAccount(f.ctx, f.hh, ledger.AccountInput{
		Name: "Fidelity", Type: models.AccountInvestment, StartingBalance: 1000, StartingBalanceDate: "2026-06-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.AddSavingsRate(f.ctx, f.hh, brokerage.ID, ledger.SavingsRateInput{
		APY: 4.0, EffectiveFrom: "2026-06-01",
	}); err != nil {
		t.Fatal(err)
	}

	fund := f.sym("SPAXX")
	f.price(fund, 1, 1) // the fund is a known security
	if err := f.svc.SetCashFund(f.ctx, f.hh, brokerage.ID, fund, day("2026-08-10")); err != nil {
		t.Fatalf("linking: %v", err)
	}
	for asOf, y := range map[string]float64{"2026-08-10": 3.3, "2026-08-20": 3.6} {
		if err := f.svc.SaveFundYield(f.ctx, fund, day(asOf), y); err != nil {
			t.Fatal(err)
		}
	}

	_, err = f.svc.AddSavingsRate(f.ctx, f.hh, brokerage.ID, ledger.SavingsRateInput{APY: 5, EffectiveFrom: "2026-09-01"})
	wantInvalid(t, err) // the fund owns the yield now

	if n := f.postInterest("2026-09-02"); n != 3 {
		t.Fatalf("posted %d, want June, July and August", n)
	}
	txns := f.interestTxns(brokerage.ID)
	if len(txns) != 3 {
		t.Fatalf("dividends = %+v", txns)
	}
	if want := fund + " dividend"; txns[2].Description != want {
		t.Errorf("description = %q, want %q", txns[2].Description, want)
	}

	simple := func(y float64) float64 { return (math.Pow(1+y/100/12, 12) - 1) * 100 }
	rates := []interest.Rate{
		{From: day("2026-06-01"), APY: 4.0},
		{From: day("2026-08-10"), APY: simple(3.3)},
		{From: day("2026-08-20"), APY: simple(3.6)},
	}
	balance := 1000 + txns[0].Amount + txns[1].Amount
	money(t, "July still at the manual 4%", txns[1].Amount,
		interest.ForMonth(day("2026-07-01"), 1000+txns[0].Amount, rates[:1], day("2026-06-01")))
	money(t, "August: manual to the 9th, then the fund's yields", txns[2].Amount,
		interest.ForMonth(day("2026-08-01"), balance, rates, day("2026-06-01")))
	// A 3.6% simple yield earns exactly 0.3% a month.
	near := interest.MonthlyRate(simple(3.6))
	if math.Abs(near-0.003) > 1e-12 {
		t.Errorf("3.6%% simple yield earns %.6f a month, want 0.003", near)
	}

	out, err := f.svc.SavingsOutlookFor(f.ctx, f.hh, brokerage.ID)
	if err != nil {
		t.Fatal(err)
	}
	if out.CashFund == nil || *out.CashFund != fund || out.FundYield == nil || *out.FundYield != 3.6 {
		t.Errorf("outlook = %+v", out)
	}

	due, err := f.svc.CashFundsDue(f.ctx, f.hh, time.Now().Add(time.Hour))
	if err != nil || len(due) != 1 || due[0] != fund {
		t.Errorf("due after an hour = %v, %v", due, err)
	}
	if due, _ = f.svc.CashFundsDue(f.ctx, f.hh, time.Now().Add(-time.Hour)); len(due) != 0 {
		t.Errorf("fetched just now but due: %v", due)
	}

	_, err = f.svc.UpdateAccount(f.ctx, f.hh, brokerage.ID, ledger.AccountInput{
		Name: "Fidelity", Type: models.AccountChecking, StartingBalance: 1000, StartingBalanceDate: "2026-06-01",
	})
	wantInvalid(t, err)

	if err := f.svc.SetCashFund(f.ctx, f.hh, brokerage.ID, "", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.AddSavingsRate(f.ctx, f.hh, brokerage.ID, ledger.SavingsRateInput{APY: 3.4, EffectiveFrom: "2026-09-01"}); err != nil {
		t.Errorf("manual rate after unlinking: %v", err)
	}
}

func TestCashFundRules(t *testing.T) {
	f := newFixture(t)
	savings := f.account("Savings", models.AccountSavings, 100)
	wantInvalid(t, f.svc.SetCashFund(f.ctx, f.hh, savings.ID, "SPAXX", day("2026-09-01")))

	brokerage := f.account("Brokerage", models.AccountInvestment, 100)
	// A symbol that was never looked up can't be linked.
	wantInvalid(t, f.svc.SetCashFund(f.ctx, f.hh, brokerage.ID, f.sym("NOPE"), day("2026-09-01")))
}
