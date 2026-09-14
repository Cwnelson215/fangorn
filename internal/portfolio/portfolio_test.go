package portfolio

import (
	"errors"
	"math"
	"testing"
	"time"
)

func d(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func near(t *testing.T, what string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

// trade builds a Trade whose amount is the default CashAmount (no fees).
func trade(seq int, sym, side, date string, shares, price float64) Trade {
	return Trade{Seq: seq, Symbol: sym, Side: side, Date: d(date), Shares: shares, Price: price,
		Amount: CashAmount(side, shares, price, 0)}
}

func replay(t *testing.T, trades ...Trade) map[string]Position {
	t.Helper()
	positions, err := Replay(trades)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]Position{}
	for _, p := range positions {
		out[p.Symbol] = p
	}
	return out
}

func wantOversell(t *testing.T, trades ...Trade) *OversellError {
	t.Helper()
	_, err := Replay(trades)
	var over *OversellError
	if !errors.As(err, &over) {
		t.Fatalf("want OversellError, got %v", err)
	}
	return over
}

func TestAverageCostAcrossBuysAndASell(t *testing.T) {
	p := replay(t,
		trade(1, "FZROX", SideBuy, "2026-01-05", 10, 20),
		trade(2, "FZROX", SideBuy, "2026-02-05", 10, 30),
		// avg cost is now 25; selling 5 at 40 realizes (40-25)*5 = 75
		trade(3, "FZROX", SideSell, "2026-03-05", 5, 40),
	)["FZROX"]
	near(t, "shares", p.Shares, 15)
	near(t, "cost basis", p.CostBasis, 375)
	near(t, "avg cost", p.AvgCost(), 25)
	near(t, "realized", p.RealizedGain, 75)
}

// Basis comes from the amount actually paid, not shares × price: a $500 order
// that bought 25.123 shares at $19.905 cost $500, not $500.07.
func TestBasisUsesAmountPaid(t *testing.T) {
	buy := trade(1, "FZROX", SideBuy, "2026-01-05", 25.123, 19.905)
	buy.Amount = 500
	p := replay(t, buy)["FZROX"]
	near(t, "cost basis", p.CostBasis, 500)
}

func TestFeesAddToBasisAndReduceProceeds(t *testing.T) {
	buy := trade(1, "VOO", SideBuy, "2026-01-05", 2, 500)
	buy.Amount = CashAmount(SideBuy, 2, 500, 10) // 1010
	sell := trade(2, "VOO", SideSell, "2026-01-06", 2, 510)
	sell.Amount = CashAmount(SideSell, 2, 510, 10) // 1010
	p := replay(t, buy, sell)["VOO"]
	near(t, "shares", p.Shares, 0)
	near(t, "cost basis", p.CostBasis, 0)
	near(t, "realized", p.RealizedGain, 0)
}

func TestOpeningAndReinvestAddShares(t *testing.T) {
	p := replay(t,
		trade(1, "FZILX", SideOpening, "2026-01-01", 100, 12),
		trade(2, "FZILX", SideReinvest, "2026-12-20", 2.5, 14),
	)["FZILX"]
	near(t, "shares", p.Shares, 102.5)
	near(t, "cost basis", p.CostBasis, 1235)
}

func TestSellMoreThanHeldIsRejected(t *testing.T) {
	over := wantOversell(t,
		trade(1, "AAPL", SideBuy, "2026-01-05", 3, 200),
		trade(2, "AAPL", SideSell, "2026-01-06", 4, 210),
	)
	near(t, "held", over.Held, 3)
	if over.Error() != "you held 3 shares of AAPL on 2026-01-06, so you can't sell 4" {
		t.Errorf("message = %q", over.Error())
	}
	one := &OversellError{Symbol: "VOO", Date: d("2026-06-02"), Held: 1, Selling: 2}
	if one.Error() != "you held 1 share of VOO on 2026-06-02, so you can't sell 2" {
		t.Errorf("message = %q", one.Error())
	}
}

// A sell dated before the buy that would cover it is an oversell, even though the
// log as a whole nets out positive — it was entered later but happened earlier.
func TestOrderIsByDateNotEntry(t *testing.T) {
	wantOversell(t,
		trade(1, "AAPL", SideBuy, "2026-02-01", 5, 200),
		trade(2, "AAPL", SideSell, "2026-01-15", 5, 190),
	)
}

func TestSameDaySellCoveredByBuyEnteredLater(t *testing.T) {
	replay(t,
		trade(1, "AAPL", SideSell, "2026-01-15", 5, 190),
		trade(2, "AAPL", SideBuy, "2026-01-15", 5, 200),
	)
}

func TestRemovingABuyInvalidatesLaterSell(t *testing.T) {
	replay(t,
		trade(1, "VTI", SideBuy, "2026-01-05", 5, 300),
		trade(2, "VTI", SideSell, "2026-02-05", 5, 310),
	)
	wantOversell(t, trade(2, "VTI", SideSell, "2026-02-05", 5, 310))
}

func TestSellingFractionalPositionToZeroLeavesNothing(t *testing.T) {
	p := replay(t,
		trade(1, "FZROX", SideBuy, "2026-01-05", 0.1, 20),
		trade(2, "FZROX", SideBuy, "2026-01-06", 0.2, 20),
		// 0.1 + 0.2 is 0.30000000000000004 in float64
		trade(3, "FZROX", SideSell, "2026-01-07", 0.3, 21),
	)["FZROX"]
	if p.Shares != 0 || p.CostBasis != 0 {
		t.Errorf("want an empty position, got %+v", p)
	}
	near(t, "realized", p.RealizedGain, 0.3)
}

func TestSymbolsAreIndependent(t *testing.T) {
	wantOversell(t,
		trade(1, "VOO", SideBuy, "2026-01-05", 10, 500),
		trade(2, "VTI", SideSell, "2026-01-06", 1, 300),
	)
}

func TestSoldOutPositionKeepsRealizedGain(t *testing.T) {
	positions, err := Replay([]Trade{
		trade(1, "VTI", SideBuy, "2026-01-05", 1, 100),
		trade(2, "VTI", SideSell, "2026-01-06", 1, 150),
		trade(3, "AAPL", SideBuy, "2026-01-06", 1, 200),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(positions) != 2 || positions[0].Symbol != "AAPL" || positions[1].Symbol != "VTI" {
		t.Fatalf("positions = %+v, want AAPL then VTI", positions)
	}
	near(t, "VTI realized", positions[1].RealizedGain, 50)
}

func TestSharesAsOf(t *testing.T) {
	trades := []Trade{
		trade(1, "VOO", SideBuy, "2026-01-05", 10, 0),
		trade(2, "VOO", SideSell, "2026-01-10", 4, 0),
		trade(3, "VTI", SideBuy, "2026-01-10", 1, 0),
		trade(4, "VTI", SideSell, "2026-01-11", 1, 0),
	}
	if got := SharesAsOf(trades, d("2026-01-04")); len(got) != 0 {
		t.Errorf("before any trade: %v", got)
	}
	got := SharesAsOf(trades, d("2026-01-10"))
	near(t, "VOO on the 10th", got["VOO"], 6)
	near(t, "VTI on the 10th", got["VTI"], 1)
	if _, ok := SharesAsOf(trades, d("2026-01-11"))["VTI"]; ok {
		t.Error("a sold-out symbol should be omitted")
	}
}

func TestCashAmount(t *testing.T) {
	cases := []struct {
		name                string
		side                string
		shares, price, fees float64
		want                float64
	}{
		{"buy adds fees", SideBuy, 2, 100, 5, 205},
		{"sell subtracts fees", SideSell, 2, 100, 5, 195},
		{"opening ignores fees", SideOpening, 2, 100, 5, 200},
		{"half cent rounds up exactly", SideBuy, 1, 1.005, 0, 1.01},
		{"fractional fund shares", SideBuy, 25.123, 19.905, 0, 500.07},
		{"below half a cent rounds down", SideBuy, 3, 0.3331, 0, 1.00},
	}
	for _, c := range cases {
		near(t, c.name, CashAmount(c.side, c.shares, c.price, c.fees), c.want)
	}
	near(t, "buy is money out", SignedCash(SideBuy, 10), -10)
	near(t, "sell is money in", SignedCash(SideSell, 10), 10)
	near(t, "reinvest moves nothing", SignedCash(SideReinvest, 10), 0)
}

func TestFormatShares(t *testing.T) {
	for in, want := range map[float64]string{12.5: "12.5", 3: "3", 0.1 + 0.2: "0.3", 25.123: "25.123"} {
		if got := FormatShares(in); got != want {
			t.Errorf("FormatShares(%v) = %q, want %q", in, got, want)
		}
	}
}
