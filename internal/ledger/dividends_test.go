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

// A reinvested dividend is income that bought shares the same day: it counts
// as a dividend like any other, and the cash nets to nothing.
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
	var bought *models.Trade
	for i := range trades {
		if trades[i].Side == portfolio.SideBuy {
			bought = &trades[i]
		}
	}
	if bought == nil || bought.Amount != 30.12 || bought.TradeDate != ymd(ex) {
		t.Fatalf("reinvestment = %+v", bought)
	}
	income, err := f.svc.ListTransactions(f.ctx, f.hh, ledger.TransactionFilter{AccountID: brokerage.ID, Kind: models.KindIncome})
	if err != nil {
		t.Fatal(err)
	}
	if len(income) != 1 || income[0].Amount != 30.12 || income[0].Source != models.SourceDividend {
		t.Fatalf("income = %+v", income)
	}
}

// With a published pay date, one tap waits for it and dates the dividend then.
func TestDividendPayDate(t *testing.T) {
	f := newFixture(t)
	today := f.householdToday()
	paid, upcoming := f.sym("META"), f.sym("CRM")
	f.price(paid, 700, 700)
	f.price(upcoming, 200, 200)
	brokerage := f.account("Brokerage", models.AccountInvestment, 0)
	since := ymd(today.AddDate(0, 0, -40))
	f.trade(brokerage.ID, portfolio.SideOpening, paid, since, 1, 650)
	f.trade(brokerage.ID, portfolio.SideOpening, upcoming, since, 1, 190)
	ex := today.AddDate(0, 0, -12)
	f.declare(paid, ex, 0.525)
	f.declare(upcoming, ex, 0.44)
	for sym, pay := range map[string]time.Time{paid: today.AddDate(0, 0, -5), upcoming: today.AddDate(0, 0, 2)} {
		if err := f.svc.SaveDividendPayDate(f.ctx, sym, ex, pay); err != nil {
			t.Fatal(err)
		}
	}
	f.findDividends(today)
	bysymbol := map[string]ledger.Dividend{}
	for _, d := range f.pendingDividends() {
		bysymbol[d.Symbol] = d
	}
	if d := bysymbol[paid]; d.PayDate == nil || *d.PayDate != ymd(today.AddDate(0, 0, -5)) {
		t.Fatalf("pay date = %v", d.PayDate)
	}

	// Not paid yet: one tap is refused, and the dividend stays pending.
	wantInvalid(t, f.svc.ConfirmDividend(f.ctx, f.hh, bysymbol[upcoming].ID, ledger.DividendConfirmation{}))
	if err := f.svc.ConfirmDividend(f.ctx, f.hh, bysymbol[paid].ID, ledger.DividendConfirmation{}); err != nil {
		t.Fatalf("ConfirmDividend: %v", err)
	}
	income, err := f.svc.ListTransactions(f.ctx, f.hh, ledger.TransactionFilter{AccountID: brokerage.ID, Kind: models.KindIncome})
	if err != nil {
		t.Fatal(err)
	}
	if len(income) != 1 || income[0].Date != ymd(today.AddDate(0, 0, -5)) || income[0].Amount != 0.53 {
		t.Fatalf("income = %+v", income)
	}
	if left := f.pendingDividends(); len(left) != 1 || left[0].Symbol != upcoming {
		t.Fatalf("pending = %+v", left)
	}
}

// Confirming a split restates the trades before it in post-split shares and
// prices: the position, its cost and the cash are what they were, in new units.
func TestSplitFoundAndConfirmed(t *testing.T) {
	f := newFixture(t)
	today := f.householdToday()
	sym := f.sym("NVDA")
	f.price(sym, 120, 120) // already quoted post-split
	brokerage := f.account("Brokerage", models.AccountInvestment, 5000)
	f.trade(brokerage.ID, portfolio.SideOpening, sym, ymd(today.AddDate(0, 0, -50)), 2, 1000)
	f.trade(brokerage.ID, portfolio.SideBuy, sym, ymd(today.AddDate(0, 0, -30)), 1, 1100)
	splitDay := today.AddDate(0, 0, -10)
	f.trade(brokerage.ID, portfolio.SideBuy, sym, ymd(splitDay), 5, 118) // bought after: already post-split
	cash := f.accountByID(brokerage.ID).CashBalance

	split := []quotes.Split{{Date: splitDay, Numerator: 10, Denominator: 1}}
	if err := f.svc.SaveSplits(f.ctx, sym, split); err != nil {
		t.Fatal(err)
	}
	find := func() int {
		n, err := f.svc.FindSplits(f.ctx, f.hh, today)
		if err != nil {
			t.Fatalf("FindSplits: %v", err)
		}
		return n
	}
	if n := find(); n != 1 {
		t.Fatalf("raised %d splits, want 1", n)
	}
	if n := find(); n != 0 {
		t.Fatalf("a second pass raised %d more", n)
	}
	pending, err := f.svc.ListSplits(f.ctx, f.hh, &brokerage.ID)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending = %+v, %v", pending, err)
	}
	sp := pending[0]
	if sp.Symbol != sym || sp.Shares != 3 || sp.SharesAfter != 30 || sp.SplitDate != ymd(splitDay) {
		t.Fatalf("split = %+v", sp)
	}

	if err := f.svc.ConfirmSplit(f.ctx, f.hh, sp.ID); err != nil {
		t.Fatalf("ConfirmSplit: %v", err)
	}
	h, err := f.svc.Holdings(f.ctx, f.hh, brokerage.ID)
	if err != nil {
		t.Fatal(err)
	}
	// 3 became 30, plus the 5 bought after; what was paid hasn't changed.
	if len(h.Positions) != 1 || h.Positions[0].Shares != 35 || h.Positions[0].CostBasis != 2000+1100+590 {
		t.Fatalf("position = %+v", h.Positions)
	}
	if h.Cash != cash {
		t.Errorf("cash went %v -> %v", cash, h.Cash)
	}
	// Twice would apply the ratio twice.
	wantInvalid(t, f.svc.ConfirmSplit(f.ctx, f.hh, sp.ID))
	if n := find(); n != 0 {
		t.Error("a confirmed split was raised again")
	}

	// Another household can't reach it, and a dismissed one changes nothing.
	g := newFixture(t)
	if err := g.svc.DismissSplit(g.ctx, g.hh, sp.ID); err != ledger.ErrNotFound {
		t.Errorf("cross-household dismiss: %v", err)
	}
}

func TestSplitDismissed(t *testing.T) {
	f := newFixture(t)
	today := f.householdToday()
	sym := f.sym("AMD")
	f.price(sym, 60, 60)
	brokerage := f.account("Brokerage", models.AccountInvestment, 0)
	f.trade(brokerage.ID, portfolio.SideOpening, sym, ymd(today.AddDate(0, 0, -50)), 10, 55)
	// A reverse split, and one from before the lookback that is never raised.
	err := f.svc.SaveSplits(f.ctx, sym, []quotes.Split{
		{Date: today.AddDate(0, 0, -3), Numerator: 1, Denominator: 4},
		{Date: today.AddDate(0, 0, -300), Numerator: 2, Denominator: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := f.svc.FindSplits(f.ctx, f.hh, today); err != nil || n != 1 {
		t.Fatalf("FindSplits = %d, %v", n, err)
	}
	pending, _ := f.svc.ListSplits(f.ctx, f.hh, nil)
	if len(pending) != 1 || pending[0].SharesAfter != 2.5 {
		t.Fatalf("pending = %+v", pending)
	}
	if err := f.svc.DismissSplit(f.ctx, f.hh, pending[0].ID); err != nil {
		t.Fatal(err)
	}
	h, _ := f.svc.Holdings(f.ctx, f.hh, brokerage.ID)
	if h.Positions[0].Shares != 10 {
		t.Errorf("shares = %v; dismissing must leave the trades alone", h.Positions[0].Shares)
	}
	if left, _ := f.svc.ListSplits(f.ctx, f.hh, nil); len(left) != 0 {
		t.Error("still pending after dismissing")
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

// A goal on a Roth counts what was put in, not what its holdings paid: neither
// a confirmed dividend nor one typed in under Dividends fills it.
func TestGoalIgnoresDividends(t *testing.T) {
	f := newFixture(t)
	today := f.householdToday()
	sym := f.sym("META")
	f.price(sym, 700, 700)
	checking := f.account("Checking", models.AccountChecking, 5000)
	roth := f.retirement("Roth IRA", models.TaxRoth, 0)
	f.trade(roth.ID, portfolio.SideOpening, sym, ymd(today.AddDate(0, 0, -40)), 10, 650)

	goal, err := f.svc.CreateGoal(f.ctx, f.hh, ledger.GoalInput{
		Name: "Max the Roth", TargetAmount: 7000, AccountID: &roth.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.transfer(checking.ID, roth.ID, 500, ymd(today))

	f.declare(sym, today.AddDate(0, 0, -3), 0.525)
	f.findDividends(today)
	if err := f.svc.ConfirmDividend(f.ctx, f.hh, f.pendingDividends()[0].ID, ledger.DividendConfirmation{}); err != nil {
		t.Fatal(err)
	}
	dividends, err := f.svc.ListCategories(f.ctx, f.hh, false)
	if err != nil {
		t.Fatal(err)
	}
	var dividendsID int
	for _, c := range dividends {
		if c.Name == "Dividends" {
			dividendsID = c.ID
		}
	}
	f.txn(roth.ID, models.KindIncome, ymd(today), 12.34, &dividendsID) // typed in by hand

	goal, err = f.svc.GetGoal(f.ctx, f.hh, goal.ID)
	if err != nil {
		t.Fatal(err)
	}
	money(t, "only the contribution", goal.Saved, 500)
}
