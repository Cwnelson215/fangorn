// Package portfolio turns a trade log into positions.
//
// It is pure — no database, no clock, no prices fetched — for the same reason
// internal/recurring is: this is the arithmetic every holdings figure rests on,
// and it is far easier to pin down with table tests than through SQL.
//
// Cost basis uses the average-cost method. It is what Fidelity and most other
// brokerages default to for mutual funds, it needs no lot bookkeeping, and for a
// household tracking how its investments are doing (rather than filing a return)
// it gives the figure people expect: "I've put $X in, it's worth $Y".
package portfolio

import (
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"time"
)

// Trade sides. buy and sell move cash; reinvest and opening only add shares.
const (
	SideBuy      = "buy"
	SideSell     = "sell"
	SideReinvest = "reinvest"
	SideOpening  = "opening"
)

// ValidSide reports whether s is a supported trade side.
func ValidSide(s string) bool {
	switch s {
	case SideBuy, SideSell, SideReinvest, SideOpening:
		return true
	}
	return false
}

// MovesCash reports whether a trade side writes a cash leg.
func MovesCash(side string) bool {
	return side == SideBuy || side == SideSell
}

// shareEpsilon absorbs float noise in share counts. Shares are stored to 8
// decimal places, so anything smaller is an artifact — and without it, selling
// "all 12.345 shares" could leave 1e-15 of a share behind, or be refused.
const shareEpsilon = 5e-9

// RoundShares rounds to the 8 decimal places the database stores.
func RoundShares(x float64) float64 {
	return math.Round(x*1e8) / 1e8
}

// CashAmount is the default dollar amount of a trade, rounded to cents:
// what a buy cost including fees, what a sell brought in after fees, and the
// cost basis of an opening position or reinvestment.
//
// It is only a default. A mutual fund order is usually placed in dollars
// ("$500 of FZROX") and the broker works out the shares, so shares × price is
// not exactly what left the account — the trade form lets the user enter the
// real figure instead.
//
// The multiplication is done exactly. In float64, 1.005 × 1 rounds to 1.00
// because 1.005 is really 1.00499999…; starting from the shortest decimal form
// of each input gives the 1.01 a person would write down.
func CashAmount(side string, shares, price, fees float64) float64 {
	total := new(big.Rat).Mul(rat(shares), rat(price))
	switch side {
	case SideBuy:
		total.Add(total, rat(fees))
	case SideSell:
		total.Sub(total, rat(fees))
	}
	return roundCents(total)
}

// MarketValue is shares × price rounded to cents, computed exactly. The SQL
// balance projection rounds each position the same way, so an account's
// holdings_value and the holdings view add up to the same figure.
func MarketValue(shares, price float64) float64 {
	return roundCents(new(big.Rat).Mul(rat(RoundShares(shares)), rat(price)))
}

// SignedCash applies the account-relative sign to a trade's amount: a buy is
// money out of the account's cash, a sell is money in.
func SignedCash(side string, amount float64) float64 {
	switch side {
	case SideBuy:
		return -amount
	case SideSell:
		return amount
	}
	return 0
}

func rat(f float64) *big.Rat {
	r, ok := new(big.Rat).SetString(strconv.FormatFloat(f, 'f', -1, 64))
	if !ok {
		return new(big.Rat)
	}
	return r
}

// roundCents rounds half away from zero to two decimal places.
func roundCents(r *big.Rat) float64 {
	cents := new(big.Rat).Mul(r, big.NewRat(100, 1))
	num, den := cents.Num(), cents.Denom()
	q, m := new(big.Int).QuoRem(num, den, new(big.Int))
	// |remainder| * 2 >= denominator means round away from zero.
	if new(big.Int).Mul(new(big.Int).Abs(m), big.NewInt(2)).Cmp(den) >= 0 {
		if num.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	f, _ := new(big.Rat).SetFrac(q, big.NewInt(100)).Float64()
	return f
}

type Trade struct {
	// Seq orders same-day trades. It is the trade's ID; a trade not yet saved
	// uses math.MaxInt so it sorts after everything already entered.
	Seq    int
	Symbol string
	Side   string
	Date   time.Time
	Shares float64
	Price  float64
	// Amount is the trade's dollar figure (see CashAmount), and is what cost
	// basis and realized gain are computed from.
	Amount float64
}

// Position is one symbol's state after replaying every trade in the log.
type Position struct {
	Symbol string
	Shares float64
	// CostBasis is what the shares still held cost, fees included.
	CostBasis float64
	// RealizedGain is the running total of sale proceeds minus the average cost
	// of the shares sold. It survives selling out of a position entirely.
	RealizedGain float64
}

// AvgCost is the cost per share still held, or 0 when nothing is held.
func (p Position) AvgCost() float64 {
	if p.Shares <= shareEpsilon {
		return 0
	}
	return p.CostBasis / p.Shares
}

// OversellError is returned when a sell exceeds the shares held at that point
// in the log. Held is what was actually there, so the message can say so.
type OversellError struct {
	Symbol  string
	Date    time.Time
	Held    float64
	Selling float64
}

func (e *OversellError) Error() string {
	return fmt.Sprintf("you held %s of %s on %s, so you can't sell %s",
		sharesNoun(e.Held), e.Symbol, e.Date.Format("2006-01-02"), FormatShares(e.Selling))
}

func sharesNoun(n float64) string {
	if RoundShares(n) == 1 {
		return "1 share"
	}
	return FormatShares(n) + " shares"
}

// FormatShares renders a share count without trailing zeros: 12.5, not 12.50000000.
func FormatShares(v float64) string {
	return strconv.FormatFloat(RoundShares(v), 'f', -1, 64)
}

// Replay applies trades in date order and returns the resulting position per
// symbol, sorted by symbol. Symbols that have since been sold out are included
// (with Shares == 0) so their realized gain is not lost.
//
// Within a day, shares coming in apply before shares going out, then entry
// order. The log only records dates, not times, so a buy and a sell on the same
// day logged "backwards" are not an oversell — refusing them would be the app
// being pedantic about information it never asked for.
func Replay(trades []Trade) ([]Position, error) {
	sorted := make([]Trade, len(trades))
	copy(sorted, trades)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if !a.Date.Equal(b.Date) {
			return a.Date.Before(b.Date)
		}
		if aSell, bSell := a.Side == SideSell, b.Side == SideSell; aSell != bSell {
			return bSell
		}
		return a.Seq < b.Seq
	})

	bySymbol := map[string]*Position{}
	for _, t := range sorted {
		p, ok := bySymbol[t.Symbol]
		if !ok {
			p = &Position{Symbol: t.Symbol}
			bySymbol[t.Symbol] = p
		}

		switch t.Side {
		case SideBuy, SideReinvest, SideOpening:
			p.Shares += t.Shares
			p.CostBasis += t.Amount

		case SideSell:
			if t.Shares > p.Shares+shareEpsilon {
				return nil, &OversellError{Symbol: t.Symbol, Date: t.Date, Held: p.Shares, Selling: t.Shares}
			}
			basisOut := p.CostBasis * math.Min(t.Shares/p.Shares, 1)
			p.RealizedGain += t.Amount - basisOut
			p.Shares -= t.Shares
			p.CostBasis -= basisOut
			if p.Shares <= shareEpsilon {
				p.Shares, p.CostBasis = 0, 0
			}

		default:
			return nil, fmt.Errorf("unknown trade side %q", t.Side)
		}
	}

	out := make([]Position, 0, len(bySymbol))
	for _, p := range bySymbol {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Symbol < out[j].Symbol })
	return out, nil
}

// SharesAsOf returns shares held per symbol at the end of the given day. It
// assumes the log is valid (Replay has accepted it), so it does not re-check for
// overselling, and it omits symbols with nothing held.
func SharesAsOf(trades []Trade, day time.Time) map[string]float64 {
	out := map[string]float64{}
	for _, t := range trades {
		if t.Date.After(day) {
			continue
		}
		if t.Side == SideSell {
			out[t.Symbol] -= t.Shares
		} else {
			out[t.Symbol] += t.Shares
		}
	}
	for sym, n := range out {
		if n <= shareEpsilon {
			delete(out, sym)
		}
	}
	return out
}
