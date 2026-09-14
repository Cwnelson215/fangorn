package portfolio

import (
	"testing"
)

func pointsByDate(points []Point) map[string]Point {
	out := map[string]Point{}
	for _, p := range points {
		out[p.Date] = p
	}
	return out
}

// 2026-09-11 is a Friday.
func TestValueSeriesCarriesFridayCloseOverWeekend(t *testing.T) {
	l := Ledger{
		StartingCash: 0, StartingDate: d("2026-09-01"),
		Trades: []Trade{trade(1, "FZROX", SideOpening, "2026-09-01", 10, 20)},
	}
	closes := map[string][]Close{"FZROX": {
		{Date: d("2026-09-10"), Price: 21},
		{Date: d("2026-09-11"), Price: 22},
		{Date: d("2026-09-14"), Price: 23},
	}}
	series := ValueSeries(l, closes, nil, d("2026-09-10"), d("2026-09-14"))
	if len(series) != 5 {
		t.Fatalf("want one point per calendar day, got %d: %+v", len(series), series)
	}
	pts := pointsByDate(series)
	near(t, "thursday", pts["2026-09-10"].Value, 210)
	near(t, "friday", pts["2026-09-11"].Value, 220)
	near(t, "saturday", pts["2026-09-12"].Value, 220)
	near(t, "sunday", pts["2026-09-13"].Value, 220)
	near(t, "monday", pts["2026-09-14"].Value, 230)
}

func TestValueSeriesBuyMovesCashIntoHoldings(t *testing.T) {
	buy := trade(1, "VOO", SideBuy, "2026-03-03", 2, 500)
	l := Ledger{
		StartingCash: 1000, StartingDate: d("2026-03-01"),
		// The buy's cash leg, as the ledger records it.
		Cash:   []DailyCash{{Date: d("2026-03-03"), Amount: -1000}},
		Trades: []Trade{buy},
	}
	closes := map[string][]Close{"VOO": {{Date: d("2026-03-03"), Price: 500}, {Date: d("2026-03-04"), Price: 510}}}
	pts := pointsByDate(ValueSeries(l, closes, nil, d("2026-03-01"), d("2026-03-04")))

	near(t, "cash before", pts["2026-03-02"].Cash, 1000)
	near(t, "holdings before", pts["2026-03-02"].Holdings, 0)
	near(t, "cash on buy day", pts["2026-03-03"].Cash, 0)
	near(t, "holdings on buy day", pts["2026-03-03"].Holdings, 1000)
	near(t, "value unchanged by the buy", pts["2026-03-03"].Value, 1000)
	near(t, "value after the price moves", pts["2026-03-04"].Value, 1020)
}

func TestValueSeriesSellOut(t *testing.T) {
	l := Ledger{
		StartingDate: d("2026-01-01"),
		Cash:         []DailyCash{{Date: d("2026-01-05"), Amount: 330}},
		Trades: []Trade{
			trade(1, "FZILX", SideOpening, "2026-01-01", 30, 10),
			trade(2, "FZILX", SideSell, "2026-01-05", 30, 11),
		},
	}
	closes := map[string][]Close{"FZILX": {{Date: d("2026-01-02"), Price: 11}, {Date: d("2026-01-06"), Price: 99}}}
	pts := pointsByDate(ValueSeries(l, closes, nil, d("2026-01-01"), d("2026-01-06")))

	near(t, "held", pts["2026-01-04"].Holdings, 330)
	near(t, "sold day holdings", pts["2026-01-05"].Holdings, 0)
	near(t, "sold day value", pts["2026-01-05"].Value, 330)
	near(t, "a later price change doesn't matter once sold", pts["2026-01-06"].Value, 330)
}

func TestValueSeriesFallsBackToTradePrice(t *testing.T) {
	l := Ledger{
		StartingDate: d("2026-05-01"),
		Trades:       []Trade{trade(1, "NEWFUND", SideOpening, "2026-05-01", 4, 25)},
	}
	pts := pointsByDate(ValueSeries(l, nil, nil, d("2026-05-01"), d("2026-05-03")))
	near(t, "no closes: trade price", pts["2026-05-02"].Value, 100)
}

func TestValueSeriesLastPointUsesLatestPrice(t *testing.T) {
	l := Ledger{
		StartingCash: 50, StartingDate: d("2026-09-01"),
		Trades: []Trade{trade(1, "FZROX", SideOpening, "2026-09-01", 3.5, 20)},
	}
	closes := map[string][]Close{"FZROX": {{Date: d("2026-09-02"), Price: 21}}}
	series := ValueSeries(l, closes, map[string]float64{"FZROX": 21.333}, d("2026-09-01"), d("2026-09-03"))
	pts := pointsByDate(series)

	near(t, "yesterday at the close", pts["2026-09-02"].Value, 50+73.5)
	// 3.5 × 21.333 = 74.6655 → 74.67, the same rounding as MarketValue.
	near(t, "today at the latest price", pts["2026-09-03"].Holdings, 74.67)
	near(t, "today total", series[len(series)-1].Value, 124.67)
}

func TestValueSeriesStartsAtFirstActivity(t *testing.T) {
	l := Ledger{
		StartingCash: 100, StartingDate: d("2026-06-10"),
		Trades: []Trade{trade(1, "VOO", SideOpening, "2026-06-08", 1, 500)},
	}
	series := ValueSeries(l, nil, nil, d("2026-01-01"), d("2026-06-12"))
	if len(series) == 0 || series[0].Date != "2026-06-08" {
		t.Fatalf("series should start on the earliest trade, got %+v", series)
	}
	if len(series) != 5 {
		t.Errorf("want 5 days, got %d", len(series))
	}

	if got := ValueSeries(l, nil, nil, d("2026-07-01"), d("2026-06-30")); len(got) != 0 {
		t.Errorf("an empty range should give no points, got %+v", got)
	}
}

func TestSumSeries(t *testing.T) {
	a := []Point{{Date: "2026-01-01", Cash: 1, Holdings: 2, Value: 3}, {Date: "2026-01-02", Cash: 1, Holdings: 3, Value: 4}}
	b := []Point{{Date: "2026-01-02", Cash: 0.1, Holdings: 0.2, Value: 0.3}}
	got := SumSeries(a, b)
	if len(got) != 2 || got[0].Date != "2026-01-01" || got[1].Date != "2026-01-02" {
		t.Fatalf("got %+v", got)
	}
	near(t, "first day is only a", got[0].Value, 3)
	near(t, "second day adds b", got[1].Value, 4.3)
	near(t, "cash", got[1].Cash, 1.1)
}
