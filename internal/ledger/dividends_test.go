package ledger_test

import (
	"testing"
	"time"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
	"github.com/cwnelson/fangorn/internal/portfolio"
	"github.com/cwnelson/fangorn/internal/quotes"
)


func ymd(t time.Time) string { return t.Format("2006-01-02") }

func (f *fixture) householdToday() time.Time {
	f.t.Helper()
	h, err := f.svc.GetHousehold(f.ctx, f.hh)
	if err != nil {
		f.t.Fatal(err)
	}
	return h.Today()
}

func (f *fixture) declare(symbol string, ex time.Time, perShare float64) {
	f.t.Helper()
	err := f.svc.SaveDividends(f.ctx, symbol, []quotes.Dividend{{ExDate: ex, Amount: perShare}}, time.Now())
	if err != nil {
		f.t.Fatalf("SaveDividends: %v", err)
	}
}

func (f *fixture) findDividends(today time.Time) int {
	f.t.Helper()
	n, err := f.svc.FindDividends(f.ctx, f.hh, today)
	if err != nil {
		f.t.Fatalf("FindDividends: %v", err)
	}
	return n
}

func (f *fixture) pendingDividends() []ledger.Dividend {
	f.t.Helper()
	out, err := f.svc.ListDividends(f.ctx, f.hh, nil)
	if err != nil {
		f.t.Fatalf("ListDividends: %v", err)
	}
	return out
}

// A dividend is raised for the shares held going into the ex-date, once, and
// one tap posts it as income under Dividends without touching the holdings.
func TestDividendFoundAndConfirmedAsCash(t *testing.T) {
	f := newFixture(t)
	today := f.householdToday()
	sym := f.sym("META")
	f.price(sym, 700, 700)
	brokerage := f.account("Brokerage", models.AccountInvestment, 1000)
	f.trade(brokerage.ID, portfolio.SideOpening, sym, ymd(today.AddDate(0, 0, -40)), 3, 650)
	ex := today.AddDate(0, 0, -10)
	// Bought on the ex-date: too late for this dividend.
	f.trade(brokerage.ID, portfolio.SideBuy, sym, ymd(ex), 1, 100)
	cash := f.balance(brokerage.ID)

	f.declare(sym, ex, 0.525)
	// Long before the lookback, and before anything was held.
	f.declare(sym, today.AddDate(0, 0, -200), 0.5)

	if n := f.findDividends(today); n != 1 {
		t.Fatalf("raised %d dividends, want 1", n)
	}
	if n := f.findDividends(today); n != 0 {
		t.Fatalf("a second pass raised %d more", n)
	}
	pending := f.pendingDividends()
	if len(pending) != 1 {
		t.Fatalf("pending = %+v", pending)
	}
	d := pending[0]
	if d.Symbol != sym || d.Shares != 3 || d.Amount != 1.58 || d.ExDate != ymd(ex) || d.AccountID != brokerage.ID {
		t.Fatalf("dividend = %+v", d) // 3 × 0.525 = 1.575, rounded
	}

	if err := f.svc.ConfirmDividend(f.ctx, f.hh, d.ID, ledger.DividendConfirmation{}); err != nil {
		t.Fatalf("ConfirmDividend: %v", err)
	}
	if got := f.balance(brokerage.ID); math2(got-cash) != 1.58 {
		t.Errorf("balance moved by %v, want 1.58", got-cash)
	}
	txns, err := f.svc.ListTransactions(f.ctx, f.hh, ledger.TransactionFilter{AccountID: brokerage.ID, Kind: models.KindIncome})
	if err != nil {
		t.Fatal(err)
	}
	if len(txns) != 1 || txns[0].Source != models.SourceDividend || txns[0].Description != sym+" dividend" ||
		txns[0].Date != ymd(today) || txns[0].CategoryID == nil {
		t.Fatalf("income = %+v", txns)
	}
	if len(f.pendingDividends()) != 0 {
		t.Error("still pending after confirming")
	}
	// A second tap is refused rather than posted again.
	wantInvalid(t, f.svc.ConfirmDividend(f.ctx, f.hh, d.ID, ledger.DividendConfirmation{}))

	// Deleting the income doesn't bring the prompt back.
	if err := f.svc.DeleteTransaction(f.ctx, f.hh, txns[0].ID); err != nil {
		t.Fatal(err)
	}
	if n := f.findDividends(today); n != 0 || len(f.pendingDividends()) != 0 {
		t.Error("a deleted dividend was raised again")
	}
}

func math2(v float64) float64 { return float64(int64(v*100+0.5)) / 100 }

// A reinvested dividend becomes a reinvest trade: more shares, no cash.
func TestDividendConfirmedAsReinvestment(t *testing.T) {
	f := newFixture(t)
	today := f.householdToday()
	sym := f.sym("FZROX")
	f.price(sym, 25, 25)
	brokerage := f.account("Brokerage", models.AccountInvestment, 0)
	f.trade(brokerage.ID, portfolio.SideOpening, sym, ymd(today.AddDate(0, 0, -40)), 100, 20)
	ex := today.AddDate(0, 0, -5)
	f.declare(sym, ex, 0.3)
	f.findDividends(today)
	d := f.pendingDividends()[0]

	// Shares are required, and it can't have been paid before the ex-date.
	wantInvalid(t, f.svc.ConfirmDividend(f.ctx, f.hh, d.ID, ledger.DividendConfirmation{Reinvested: true}))
	shares, amount := 1.2, 30.12
	wantInvalid(t, f.svc.ConfirmDividend(f.ctx, f.hh, d.ID, ledger.DividendConfirmation{
		Reinvested: true, Shares: &shares, Date: ymd(ex.AddDate(0, 0, -1)),
	}))
	if len(f.pendingDividends()) != 1 {
		t.Fatal("a refused confirmation used the dividend up")
	}

	err := f.svc.ConfirmDividend(f.ctx, f.hh, d.ID, ledger.DividendConfirmation{
		Reinvested: true, Shares: &shares, Amount: &amount, Date: ymd(ex),
	})
	if err != nil {
		t.Fatalf("ConfirmDividend: %v", err)
	}
	h, err := f.svc.Holdings(f.ctx, f.hh, brokerage.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Positions) != 1 || h.Positions[0].Shares != 101.2 || h.Cash != 0 {
		t.Fatalf("holdings = %+v", h)
	}
	trades, _ := f.svc.ListTrades(f.ctx, f.hh, brokerage.ID)
	var reinvest *models.Trade
	for i := range trades {
		if trades[i].Side == portfolio.SideReinvest {
			reinvest = &trades[i]
		}
	}
	if reinvest == nil || reinvest.Amount != 30.12 || reinvest.TradeDate != ymd(ex) {
		t.Fatalf("reinvest trade = %+v", reinvest)
	}
}

// One already typed in — income of the same amount since the ex-date — isn't
// asked about, and a dismissed one stays gone.
func TestDividendAlreadyLoggedOrDismissed(t *testing.T) {
	f := newFixture(t)
	today := f.householdToday()
	logged, other := f.sym("CAKE"), f.sym("CRM")
	f.price(logged, 100, 100)
	f.price(other, 200, 200)
	brokerage := f.account("Brokerage", models.AccountInvestment, 0)
	since := ymd(today.AddDate(0, 0, -40))
	f.trade(brokerage.ID, portfolio.SideOpening, logged, since, 2, 50)
	f.trade(brokerage.ID, portfolio.SideOpening, other, since, 1, 190)
	ex := today.AddDate(0, 0, -8)
	f.declare(logged, ex, 0.27)
	f.declare(other, ex, 0.416)

	dividends := f.category("Dividends", models.KindIncome)
	f.txn(brokerage.ID, models.KindIncome, ymd(today.AddDate(0, 0, -2)), 0.54, &dividends.ID)

	if n := f.findDividends(today); n != 1 {
		t.Fatalf("raised %d, want only the one not logged", n)
	}
	pending := f.pendingDividends()
	if len(pending) != 1 || pending[0].Symbol != other || pending[0].Amount != 0.42 {
		t.Fatalf("pending = %+v", pending)
	}
	before := f.balance(brokerage.ID)
	if err := f.svc.DismissDividend(f.ctx, f.hh, pending[0].ID); err != nil {
		t.Fatal(err)
	}
	if n := f.findDividends(today); n != 0 || len(f.pendingDividends()) != 0 {
		t.Error("a dismissed dividend came back")
	}
	if got := f.balance(brokerage.ID); got != before {
		t.Errorf("balance went %v -> %v; dismissing must post nothing", before, got)
	}

	// Another household can't reach it.
	g := newFixture(t)
	if err := g.svc.DismissDividend(g.ctx, g.hh, pending[0].ID); err != ledger.ErrNotFound {
		t.Errorf("cross-household dismiss: %v", err)
	}
}
