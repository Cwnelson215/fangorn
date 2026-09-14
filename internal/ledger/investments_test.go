package ledger_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
	"github.com/cwnelson/fangorn/internal/portfolio"
	"github.com/cwnelson/fangorn/internal/quotes"
)

// securities is shared by every household, and tests run concurrently against
// one database, so each test prices symbols of its own.
func (f *fixture) sym(base string) string {
	return fmt.Sprintf("%s%d", base, f.hh)
}

func (f *fixture) trade(accountID int, side, symbol, date string, shares, price float64) models.Trade {
	f.t.Helper()
	tr, err := f.svc.CreateTrade(f.ctx, f.hh, accountID, ledger.TradeInput{
		Symbol: symbol, Side: side, TradeDate: date, Shares: shares, Price: price,
	})
	if err != nil {
		f.t.Fatalf("CreateTrade(%s %v %s): %v", side, shares, symbol, err)
	}
	return tr
}

func (f *fixture) price(symbol string, price, previousClose float64) {
	f.t.Helper()
	err := f.svc.SaveQuote(f.ctx, quotes.Quote{
		Symbol: symbol, Name: symbol + " Fund", QuoteType: "MUTUALFUND", Currency: "USD",
		Price: price, PreviousClose: previousClose, PriceTime: time.Now(),
	}, time.Now())
	if err != nil {
		f.t.Fatalf("SaveQuote: %v", err)
	}
}

func (f *fixture) accountByID(id int) models.Account {
	f.t.Helper()
	a, err := f.svc.GetAccount(f.ctx, f.hh, id)
	if err != nil {
		f.t.Fatalf("GetAccount(%d): %v", id, err)
	}
	return a
}

func TestBuyMovesCashIntoHoldings(t *testing.T) {
	f := newFixture(t)
	brokerage := f.account("Brokerage", models.AccountInvestment, 1000)
	fund := f.sym("FZROX")

	f.trade(brokerage.ID, portfolio.SideBuy, fund, "2026-02-01", 10, 50)

	a := f.accountByID(brokerage.ID)
	money(t, "cash", a.CashBalance, 500)
	// Seeded from the trade price until a quote arrives.
	money(t, "holdings", a.HoldingsValue, 500)
	money(t, "balance", a.Balance, 1000)

	f.price(fund, 60, 58)
	a = f.accountByID(brokerage.ID)
	money(t, "holdings after the price moves", a.HoldingsValue, 600)
	money(t, "balance after the price moves", a.Balance, 1100)

	h, err := f.svc.Holdings(f.ctx, f.hh, brokerage.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Positions) != 1 {
		t.Fatalf("positions = %+v", h.Positions)
	}
	p := h.Positions[0]
	money(t, "cost basis", p.CostBasis, 500)
	money(t, "unrealized gain", p.UnrealizedGain, 100)
	if p.DayChange == nil {
		t.Fatal("day change missing")
	}
	money(t, "day change", *p.DayChange, 20)
	money(t, "account day change", h.DayChange, 20)
	money(t, "total", h.TotalValue, 1100)
	if h.Seeded || h.AsOf == nil {
		t.Errorf("a quoted position should not be seeded (seeded=%v, as_of=%v)", h.Seeded, h.AsOf)
	}
}

func TestSellReturnsCashAndRealizesGain(t *testing.T) {
	f := newFixture(t)
	brokerage := f.account("Brokerage", models.AccountInvestment, 1000)
	etf := f.sym("VOO")

	f.trade(brokerage.ID, portfolio.SideBuy, etf, "2026-02-01", 2, 400)
	f.trade(brokerage.ID, portfolio.SideSell, etf, "2026-03-01", 1, 450)

	a := f.accountByID(brokerage.ID)
	money(t, "cash", a.CashBalance, 1000-800+450)

	h, err := f.svc.Holdings(f.ctx, f.hh, brokerage.ID)
	if err != nil {
		t.Fatal(err)
	}
	money(t, "realized", h.RealizedGain, 50)
	money(t, "cost basis of what's left", h.CostBasis, 400)
}

// The two ways of computing an account's worth — the SQL projection behind
// balances and net worth, and the Go replay behind the holdings view — must
// agree to the cent, including with fractional shares and awkward prices.
func TestOneBalanceDefinition(t *testing.T) {
	f := newFixture(t)
	f.account("Checking", models.AccountChecking, 2000)
	brokerage := f.account("Brokerage", models.AccountInvestment, 5000)
	a, b := f.sym("FZROX"), f.sym("FZILX")

	f.trade(brokerage.ID, portfolio.SideBuy, a, "2026-01-05", 25.123, 19.905)
	f.trade(brokerage.ID, portfolio.SideBuy, a, "2026-01-20", 3.3333, 20.017)
	f.trade(brokerage.ID, portfolio.SideOpening, b, "2026-01-01", 101.017, 11.1)
	f.trade(brokerage.ID, portfolio.SideSell, a, "2026-02-02", 7.777, 21.35)
	f.price(a, 26.7133, 26.49)
	f.price(b, 12.345, 12.3)

	account := f.accountByID(brokerage.ID)
	h, err := f.svc.Holdings(f.ctx, f.hh, brokerage.ID)
	if err != nil {
		t.Fatal(err)
	}
	money(t, "holdings view vs account", h.HoldingsValue, account.HoldingsValue)
	money(t, "holdings cash vs account", h.Cash, account.CashBalance)

	goal, err := f.svc.CreateGoal(f.ctx, f.hh, ledger.GoalInput{
		Name: "Retirement", TargetAmount: 100000, AccountID: &brokerage.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	money(t, "goal progress", goal.Saved, account.Balance)

	if err := f.svc.SnapshotNetWorth(f.ctx, f.hh, time.Now()); err != nil {
		t.Fatal(err)
	}
	history, err := f.svc.NetWorthHistory(f.ctx, f.hh, 1)
	if err != nil || len(history) != 1 {
		t.Fatalf("history = %v, %v", history, err)
	}
	money(t, "snapshot assets", history[0].TotalAssets, 2000+account.Balance)
	money(t, "dashboard assets", f.dashboard("2026-01-01", "2026-12-31").TotalAssets, 2000+account.Balance)
}

func TestTradesAreNotIncomeOrExpense(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Checking", models.AccountChecking, 3000)
	brokerage := f.account("Brokerage", models.AccountInvestment, 0)
	groceries := f.category("Groceries", "expense")
	f.txn(checking.ID, models.KindExpense, "2026-03-03", 80, &groceries.ID)

	f.transfer(checking.ID, brokerage.ID, 1000, "2026-03-01")
	fund := f.sym("FZROX")
	f.trade(brokerage.ID, portfolio.SideBuy, fund, "2026-03-02", 10, 60)
	f.trade(brokerage.ID, portfolio.SideSell, fund, "2026-03-04", 5, 70)

	d := f.dashboard("2026-03-01", "2026-03-31")
	money(t, "income", d.Income, 0)
	money(t, "expenses", d.Expenses, 80)
	if len(d.Categories) != 1 {
		t.Errorf("category breakdown = %+v, want groceries only", d.Categories)
	}
	var weekly float64
	for _, w := range d.WeeklySpending {
		weekly += w.Amount
	}
	money(t, "weekly spending", weekly, 80)
}

func TestOversellRejected(t *testing.T) {
	f := newFixture(t)
	brokerage := f.account("Brokerage", models.AccountInvestment, 10000)
	sym := f.sym("AAPL")

	buy := f.trade(brokerage.ID, portfolio.SideBuy, sym, "2026-02-01", 10, 200)
	sell := f.trade(brokerage.ID, portfolio.SideSell, sym, "2026-03-01", 10, 210)
	cashBefore := f.accountByID(brokerage.ID).CashBalance

	sellInput := func(date string, shares float64) ledger.TradeInput {
		return ledger.TradeInput{Symbol: sym, Side: portfolio.SideSell, TradeDate: date, Shares: shares, Price: 210}
	}

	t.Run("selling more than held", func(t *testing.T) {
		_, err := f.svc.CreateTrade(f.ctx, f.hh, brokerage.ID, sellInput("2026-03-05", 1))
		wantInvalid(t, err)
	})
	t.Run("a sell dated before the buy", func(t *testing.T) {
		_, err := f.svc.CreateTrade(f.ctx, f.hh, brokerage.ID, sellInput("2026-01-15", 1))
		wantInvalid(t, err)
	})
	t.Run("deleting the buy a later sell needed", func(t *testing.T) {
		wantInvalid(t, f.svc.DeleteTrade(f.ctx, f.hh, buy.ID))
	})
	t.Run("shrinking the buy below the sell", func(t *testing.T) {
		_, err := f.svc.UpdateTrade(f.ctx, f.hh, buy.ID, ledger.TradeInput{
			Symbol: sym, Side: portfolio.SideBuy, TradeDate: "2026-02-01", Shares: 9, Price: 200,
		})
		wantInvalid(t, err)
	})
	t.Run("moving the sell before the buy", func(t *testing.T) {
		_, err := f.svc.UpdateTrade(f.ctx, f.hh, sell.ID, sellInput("2026-01-01", 10))
		wantInvalid(t, err)
	})

	money(t, "cash after rejected writes", f.accountByID(brokerage.ID).CashBalance, cashBefore)
	trades, err := f.svc.ListTrades(f.ctx, f.hh, brokerage.ID)
	if err != nil || len(trades) != 2 {
		t.Fatalf("trades = %d, %v; want the original 2", len(trades), err)
	}
}

func TestTradeLegMovesWithTrade(t *testing.T) {
	f := newFixture(t)
	brokerage := f.account("Brokerage", models.AccountInvestment, 1000)
	sym := f.sym("VTI")
	tr := f.trade(brokerage.ID, portfolio.SideBuy, sym, "2026-02-01", 2, 100)

	register, err := f.svc.Register(f.ctx, f.hh, brokerage.ID, 0)
	if err != nil || len(register) != 1 {
		t.Fatalf("register = %+v, %v", register, err)
	}
	leg := register[0]
	if leg.Kind != models.KindTrade || leg.TradeID == nil || *leg.TradeID != tr.ID {
		t.Fatalf("leg = %+v", leg)
	}
	money(t, "leg amount", leg.Amount, -200)

	t.Run("editing the leg directly is refused", func(t *testing.T) {
		_, err := f.svc.UpdateTransaction(f.ctx, f.hh, leg.ID, ledger.TransactionInput{
			AccountID: brokerage.ID, Kind: models.KindExpense, Date: "2026-02-01", Amount: 5, Description: "x",
		})
		wantInvalid(t, err)
	})

	t.Run("updating the trade rewrites the leg", func(t *testing.T) {
		amount := 450.0
		_, err := f.svc.UpdateTrade(f.ctx, f.hh, tr.ID, ledger.TradeInput{
			Symbol: sym, Side: portfolio.SideBuy, TradeDate: "2026-02-03", Shares: 4.5, Price: 100, Amount: &amount,
		})
		if err != nil {
			t.Fatal(err)
		}
		money(t, "cash", f.accountByID(brokerage.ID).CashBalance, 550)
	})

	t.Run("turning a buy into an opening position drops the leg", func(t *testing.T) {
		_, err := f.svc.UpdateTrade(f.ctx, f.hh, tr.ID, ledger.TradeInput{
			Symbol: sym, Side: portfolio.SideOpening, TradeDate: "2026-02-03", Shares: 4.5, Price: 100,
		})
		if err != nil {
			t.Fatal(err)
		}
		money(t, "cash", f.accountByID(brokerage.ID).CashBalance, 1000)
	})

	t.Run("deleting the leg deletes the trade", func(t *testing.T) {
		_, err := f.svc.UpdateTrade(f.ctx, f.hh, tr.ID, ledger.TradeInput{
			Symbol: sym, Side: portfolio.SideBuy, TradeDate: "2026-02-03", Shares: 1, Price: 100,
		})
		if err != nil {
			t.Fatal(err)
		}
		register, _ := f.svc.Register(f.ctx, f.hh, brokerage.ID, 0)
		if err := f.svc.DeleteTransaction(f.ctx, f.hh, register[0].ID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.GetTrade(f.ctx, f.hh, tr.ID); err != ledger.ErrNotFound {
			t.Fatalf("trade still there: %v", err)
		}
		money(t, "cash", f.accountByID(brokerage.ID).CashBalance, 1000)
	})
}

func TestTradeCashRounding(t *testing.T) {
	f := newFixture(t)
	brokerage := f.account("Brokerage", models.AccountInvestment, 1000)
	sym := f.sym("FZROX")

	f.trade(brokerage.ID, portfolio.SideBuy, sym, "2026-02-01", 25.123, 19.905)
	money(t, "cash after a computed amount", f.accountByID(brokerage.ID).CashBalance, 1000-500.07)

	amount := 500.0
	tr, err := f.svc.CreateTrade(f.ctx, f.hh, brokerage.ID, ledger.TradeInput{
		Symbol: sym, Side: portfolio.SideBuy, TradeDate: "2026-02-02", Shares: 25.123, Price: 19.905, Amount: &amount,
	})
	if err != nil {
		t.Fatal(err)
	}
	money(t, "stored amount", tr.Amount, 500)
	money(t, "cash after an entered amount", f.accountByID(brokerage.ID).CashBalance, 1000-500.07-500)
}

func TestOpeningAndReinvestHaveNoCashLeg(t *testing.T) {
	f := newFixture(t)
	brokerage := f.account("Roth IRA", models.AccountInvestment, 0)
	sym := f.sym("FZILX")

	f.trade(brokerage.ID, portfolio.SideOpening, sym, "2026-01-01", 100, 12)
	f.trade(brokerage.ID, portfolio.SideReinvest, sym, "2026-06-20", 1.5, 13)

	a := f.accountByID(brokerage.ID)
	money(t, "cash", a.CashBalance, 0)
	register, err := f.svc.Register(f.ctx, f.hh, brokerage.ID, 0)
	if err != nil || len(register) != 0 {
		t.Fatalf("register = %+v, %v; want empty", register, err)
	}
	h, err := f.svc.Holdings(f.ctx, f.hh, brokerage.ID)
	if err != nil {
		t.Fatal(err)
	}
	money(t, "cost basis", h.CostBasis, 1219.5)
}

func TestTradesRequireInvestmentAccount(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Checking", models.AccountChecking, 1000)
	_, err := f.svc.CreateTrade(f.ctx, f.hh, checking.ID, ledger.TradeInput{
		Symbol: f.sym("VOO"), Side: portfolio.SideBuy, TradeDate: "2026-02-01", Shares: 1, Price: 500,
	})
	wantInvalid(t, err)
}

func TestAccountTypeLockedWithTrades(t *testing.T) {
	f := newFixture(t)
	brokerage := f.account("Brokerage", models.AccountInvestment, 1000)
	f.trade(brokerage.ID, portfolio.SideBuy, f.sym("VOO"), "2026-02-01", 1, 500)

	_, err := f.svc.UpdateAccount(f.ctx, f.hh, brokerage.ID, ledger.AccountInput{
		Name: "Brokerage", Type: models.AccountSavings, StartingBalance: 1000, StartingBalanceDate: "2026-01-01",
	})
	wantInvalid(t, err)

	_, err = f.svc.UpdateAccount(f.ctx, f.hh, brokerage.ID, ledger.AccountInput{
		Name: "Fidelity Brokerage", Type: models.AccountInvestment, StartingBalance: 1000, StartingBalanceDate: "2026-01-01",
	})
	if err != nil {
		t.Fatalf("renaming should still work: %v", err)
	}
}

func TestTradesHouseholdScoped(t *testing.T) {
	f := newFixture(t)
	other := newFixture(t)
	mine := f.account("Brokerage", models.AccountInvestment, 1000)
	theirs := other.account("Brokerage", models.AccountInvestment, 1000)
	theirTrade := other.trade(theirs.ID, portfolio.SideBuy, other.sym("VOO"), "2026-02-01", 1, 500)

	if _, err := f.svc.GetTrade(f.ctx, f.hh, theirTrade.ID); err != ledger.ErrNotFound {
		t.Errorf("GetTrade across households: %v", err)
	}
	if err := f.svc.DeleteTrade(f.ctx, f.hh, theirTrade.ID); err != ledger.ErrNotFound {
		t.Errorf("DeleteTrade across households: %v", err)
	}
	_, err := f.svc.CreateTrade(f.ctx, f.hh, theirs.ID, ledger.TradeInput{
		Symbol: f.sym("VOO"), Side: portfolio.SideBuy, TradeDate: "2026-02-01", Shares: 1, Price: 500,
	})
	wantInvalid(t, err)
	if _, err := f.svc.Holdings(f.ctx, f.hh, theirs.ID); err != ledger.ErrNotFound {
		t.Errorf("Holdings across households: %v", err)
	}
	if _, err := f.svc.ListTrades(f.ctx, f.hh, mine.ID); err != nil {
		t.Errorf("listing own trades: %v", err)
	}
}

func TestTradeInputValidation(t *testing.T) {
	f := newFixture(t)
	brokerage := f.account("Brokerage", models.AccountInvestment, 1000)
	valid := ledger.TradeInput{Symbol: f.sym("VOO"), Side: portfolio.SideBuy, TradeDate: "2026-02-01", Shares: 1, Price: 500}

	cases := map[string]func(in *ledger.TradeInput){
		"no symbol":       func(in *ledger.TradeInput) { in.Symbol = " " },
		"not a ticker":    func(in *ledger.TradeInput) { in.Symbol = "vanguard s&p" },
		"bad side":        func(in *ledger.TradeInput) { in.Side = "short" },
		"bad date":        func(in *ledger.TradeInput) { in.TradeDate = "02/01/2026" },
		"zero shares":     func(in *ledger.TradeInput) { in.Shares = 0 },
		"negative price":  func(in *ledger.TradeInput) { in.Price = -1 },
		"buy of nothing":  func(in *ledger.TradeInput) { in.Price = 0 },
		"too many shares": func(in *ledger.TradeInput) { in.Shares = 1e12 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			in := valid
			mutate(&in)
			_, err := f.svc.CreateTrade(f.ctx, f.hh, brokerage.ID, in)
			wantInvalid(t, err)
		})
	}

	in := valid
	in.Symbol = " " + f.sym("voo") + " "
	tr, err := f.svc.CreateTrade(f.ctx, f.hh, brokerage.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Symbol != f.sym("VOO") {
		t.Errorf("symbol = %q, want it trimmed and upper-cased", tr.Symbol)
	}
}

func TestDeletingAccountCascadesTrades(t *testing.T) {
	f := newFixture(t)
	brokerage := f.account("Brokerage", models.AccountInvestment, 1000)
	tr := f.trade(brokerage.ID, portfolio.SideBuy, f.sym("VOO"), "2026-02-01", 1, 500)
	if err := f.svc.DeleteAccount(f.ctx, f.hh, brokerage.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.GetTrade(f.ctx, f.hh, tr.ID); err != ledger.ErrNotFound {
		t.Errorf("trade survived its account: %v", err)
	}
}

func TestTradeKindConstraint(t *testing.T) {
	f := newFixture(t)
	brokerage := f.account("Brokerage", models.AccountInvestment, 1000)
	_, err := f.svc.DB().Exec(
		`INSERT INTO transactions (household_id, account_id, date, amount, kind, description)
		 VALUES ($1, $2, '2026-02-01', -5, 'trade', 'orphan leg')`, f.hh, brokerage.ID)
	if err == nil {
		t.Fatal("a trade-kind transaction with no trade was accepted")
	}
}

func TestHeldSymbols(t *testing.T) {
	f := newFixture(t)
	a := f.account("Brokerage", models.AccountInvestment, 10000)
	b := f.account("Roth", models.AccountInvestment, 10000)
	voo, vti := f.sym("VOO"), f.sym("VTI")
	f.trade(a.ID, portfolio.SideBuy, voo, "2026-02-01", 1, 500)
	f.trade(b.ID, portfolio.SideBuy, vti, "2026-02-01", 1, 300)
	f.trade(b.ID, portfolio.SideSell, vti, "2026-02-02", 1, 310)

	all, err := f.svc.HouseholdSymbols(f.ctx, f.hh)
	if err != nil || len(all) != 1 || all[0] != voo {
		t.Errorf("household symbols = %v, %v; want only %s (VTI was sold out)", all, err, voo)
	}
	inB, err := f.svc.AccountSymbols(f.ctx, f.hh, b.ID)
	if err != nil || len(inB) != 0 {
		t.Errorf("account symbols = %v, %v; want none", inB, err)
	}
}
