package portfolio

import (
	"math"
	"sort"
	"time"
)

// DailyCash is the net of one day's cash transactions on an account.
type DailyCash struct {
	Date   time.Time
	Amount float64
}

// Close is a security's closing price on one trading day.
type Close struct {
	Date  time.Time
	Price float64
}

// Point is an account's worth at the end of one calendar day.
type Point struct {
	Date     string  `json:"date"`
	Cash     float64 `json:"cash"`
	Holdings float64 `json:"holdings"`
	Value    float64 `json:"value"`
}

// Ledger is everything about one investment account that its value on a past
// day depends on, apart from prices.
type Ledger struct {
	StartingCash float64
	StartingDate time.Time
	Cash         []DailyCash
	Trades       []Trade
}

// firstActivity is the earliest day anything happened on the account. A series
// starting before it would just draw a flat line at zero.
func (l Ledger) firstActivity() time.Time {
	first := civil(l.StartingDate)
	for _, c := range l.Cash {
		if d := civil(c.Date); d.Before(first) {
			first = d
		}
	}
	for _, t := range l.Trades {
		if d := civil(t.Date); d.Before(first) {
			first = d
		}
	}
	return first
}

// ValueSeries returns the account's value at the end of each calendar day from
// `from` (or its first activity, if later) through `to`.
//
// A day's holdings are the shares held that day times the latest close on or
// before it, so weekends and holidays carry Friday's price forward. A symbol
// with no close yet falls back to its earliest trade price. The last day uses
// `latest` instead — the current stored price — so the series ends on exactly
// the figure the holdings view shows.
//
// It walks the days with cursors over cash, trades and closes rather than
// replaying the log per day, so a ten-year series costs no more than its rows.
func ValueSeries(l Ledger, closes map[string][]Close, latest map[string]float64, from, to time.Time) []Point {
	from, to = civil(from), civil(to)
	if first := l.firstActivity(); first.After(from) {
		from = first
	}
	if from.After(to) {
		return []Point{}
	}

	cash := sortedCopy(l.Cash, func(c DailyCash) time.Time { return c.Date })
	trades := sortedCopy(l.Trades, func(t Trade) time.Time { return t.Date })

	fallback := map[string]float64{}
	for _, t := range trades {
		if _, ok := fallback[t.Symbol]; !ok {
			fallback[t.Symbol] = t.Price
		}
	}
	sortedCloses := map[string][]Close{}
	for sym, cs := range closes {
		sortedCloses[sym] = sortedCopy(cs, func(c Close) time.Time { return c.Date })
	}

	var (
		cashTotal = l.StartingCash
		shares    = map[string]float64{}
		price     = map[string]float64{}
		cashIdx   int
		tradeIdx  int
		closeIdx  = map[string]int{}
	)
	out := make([]Point, 0, int(to.Sub(from).Hours()/24)+1)
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		for ; cashIdx < len(cash) && !civil(cash[cashIdx].Date).After(day); cashIdx++ {
			cashTotal += cash[cashIdx].Amount
		}
		for ; tradeIdx < len(trades) && !civil(trades[tradeIdx].Date).After(day); tradeIdx++ {
			t := trades[tradeIdx]
			if t.Side == SideSell {
				shares[t.Symbol] -= t.Shares
			} else {
				shares[t.Symbol] += t.Shares
			}
		}
		for sym, cs := range sortedCloses {
			i := closeIdx[sym]
			for ; i < len(cs) && !civil(cs[i].Date).After(day); i++ {
				price[sym] = cs[i].Price
			}
			closeIdx[sym] = i
		}

		var holdings float64
		for sym, n := range shares {
			if n <= shareEpsilon {
				continue
			}
			p, ok := price[sym]
			if !ok {
				p = fallback[sym]
			}
			if day.Equal(to) {
				if lp, ok := latest[sym]; ok {
					p = lp
				}
			}
			holdings += MarketValue(n, p)
		}

		c, h := round2(cashTotal), round2(holdings)
		out = append(out, Point{Date: day.Format(dateLayout), Cash: c, Holdings: h, Value: round2(c + h)})
	}
	return out
}

// SumSeries adds several accounts' series together by date. An account with no
// point on a date (it hadn't started yet) contributes nothing to it.
func SumSeries(series ...[]Point) []Point {
	byDate := map[string]*Point{}
	for _, s := range series {
		for _, p := range s {
			acc, ok := byDate[p.Date]
			if !ok {
				acc = &Point{Date: p.Date}
				byDate[p.Date] = acc
			}
			acc.Cash += p.Cash
			acc.Holdings += p.Holdings
		}
	}
	out := make([]Point, 0, len(byDate))
	for _, p := range byDate {
		p.Cash, p.Holdings = round2(p.Cash), round2(p.Holdings)
		p.Value = round2(p.Cash + p.Holdings)
		out = append(out, *p)
	}
	// YYYY-MM-DD sorts chronologically as a string.
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}

const dateLayout = "2006-01-02"

// civil strips a date down to its calendar day at midnight UTC, whatever
// location it was scanned with, so day comparisons can't be thrown by zones.
func civil(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func sortedCopy[T any](in []T, date func(T) time.Time) []T {
	out := make([]T, len(in))
	copy(out, in)
	sort.SliceStable(out, func(i, j int) bool { return civil(date(out[i])).Before(civil(date(out[j]))) })
	return out
}
