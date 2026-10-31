package interest

import (
	"math"
	"testing"
	"time"
)

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func near(t *testing.T, what string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %.6f, want %.6f", what, got, want)
	}
}

func TestMonthlyRateCompoundsToAPY(t *testing.T) {
	m := MonthlyRate(4.35)
	near(t, "monthly rate", m, 0.0035547, 0.0000001)
	near(t, "compounded twelve times", math.Pow(1+m, 12)-1, 0.0435, 1e-12)
	if MonthlyRate(0) != 0 {
		t.Error("0% APY should earn nothing")
	}
}

func TestFullMonthOneRate(t *testing.T) {
	rates := []Rate{{From: day("2026-01-01"), APY: 4.35}}
	// $10,000 at 4.35% APY: 10000 × 0.0035547 ≈ $35.55.
	got := ForMonth(day("2026-09-01"), 10000, rates, day("2026-01-01"))
	near(t, "September interest", got, 35.55, 0.001)
}

func TestFirstPartialMonth(t *testing.T) {
	// Account and rate both start Sep 24: 7 of 30 days earn.
	rates := []Rate{{From: day("2026-09-24"), APY: 4.35}}
	got := ForMonth(day("2026-09-01"), 10000, rates, day("2026-09-24"))
	near(t, "7/30 of a month", got, round2(10000*MonthlyRate(4.35)*7/30), 0.001)

	// A rate set before the account existed still only counts from its start.
	early := []Rate{{From: day("2026-01-01"), APY: 4.35}}
	near(t, "clamped to account start", ForMonth(day("2026-09-01"), 10000, early, day("2026-09-24")), got, 0.001)
}

func TestRateChangeMidMonth(t *testing.T) {
	rates := []Rate{
		{From: day("2026-10-12"), APY: 4.35}, // out of order on purpose
		{From: day("2026-01-01"), APY: 4.00},
	}
	// October has 31 days: the 1st–11th at 4.00%, the 12th–31st at 4.35%.
	want := 10000 * (11*MonthlyRate(4.00) + 20*MonthlyRate(4.35)) / 31
	near(t, "blended October", ForMonth(day("2026-10-01"), 10000, rates, day("2026-01-01")), round2(want), 0.001)
}

func TestEachDayEarnsOnItsOwnBalance(t *testing.T) {
	rates := []Rate{{From: day("2026-01-01"), APY: 4.00}}
	// September: $1,000 until a $9,000 deposit on the 21st.
	balances := make([]float64, 30)
	for i := range balances {
		balances[i] = 1000
		if i >= 20 {
			balances[i] = 10000
		}
	}
	want := (20*1000 + 10*10000) * MonthlyRate(4.00) / 30
	near(t, "20 days at $1,000, 10 at $10,000", ForDays(day("2026-09-01"), balances, rates, day("2026-01-01")), round2(want), 0.001)

	// A balance that never moves earns what the month's rate says.
	level := make([]float64, 30)
	for i := range level {
		level[i] = 10000
	}
	near(t, "level balance", ForDays(day("2026-09-01"), level, rates, day("2026-01-01")),
		round2(10000*MonthlyRate(4.00)), 0.001)

	// Overdrawn days earn nothing; they don't take interest back.
	balances[0], balances[1] = -500, 0
	want = (18*1000 + 10*10000) * MonthlyRate(4.00) / 30
	near(t, "two days at or below zero", ForDays(day("2026-09-01"), balances, rates, day("2026-01-01")), round2(want), 0.001)

	// Fewer balances than days: the missing days earn nothing.
	near(t, "ten days of balances", ForDays(day("2026-09-01"), level[:10], rates, day("2026-01-01")),
		round2(10*10000*MonthlyRate(4.00)/30), 0.001)
}

func TestNothingEarned(t *testing.T) {
	rates := []Rate{{From: day("2026-01-01"), APY: 4.35}}
	if got := ForMonth(day("2026-09-01"), 0, rates, day("2026-01-01")); got != 0 {
		t.Errorf("zero balance earned %v", got)
	}
	if got := ForMonth(day("2026-09-01"), -50, rates, day("2026-01-01")); got != 0 {
		t.Errorf("overdrawn balance earned %v", got)
	}
	if got := ForMonth(day("2026-09-01"), 1000, nil, day("2026-01-01")); got != 0 {
		t.Errorf("no rates earned %v", got)
	}
	future := []Rate{{From: day("2026-10-01"), APY: 4.35}}
	if got := ForMonth(day("2026-09-01"), 1000, future, day("2026-01-01")); got != 0 {
		t.Errorf("a rate starting next month earned %v this month", got)
	}
}

func TestMonthBoundaries(t *testing.T) {
	if MonthEnd(day("2026-02-01")) != day("2026-02-28") || MonthEnd(day("2028-02-01")) != day("2028-02-29") {
		t.Error("February month end wrong")
	}
	if MonthStart(day("2026-09-24")) != day("2026-09-01") {
		t.Error("MonthStart wrong")
	}
}

func TestPayDay(t *testing.T) {
	cases := []struct {
		month       string
		moneyMarket bool
		want        string
	}{
		{"2026-10-01", false, "2026-10-31"}, // a bank credits the last day, Saturday or not
		{"2026-10-01", true, "2026-10-30"},  // Saturday the 31st: Friday
		{"2026-11-01", true, "2026-11-30"},  // a Monday
		{"2026-05-01", true, "2026-05-29"},  // Sunday the 31st: Friday
		{"2027-05-01", true, "2027-05-28"},  // Monday the 31st is Memorial Day
		{"2024-03-01", true, "2024-03-28"},  // Good Friday the 29th, then a weekend
		{"2028-03-01", true, "2028-03-31"},  // Good Friday is April 14th
		{"2027-12-01", true, "2027-12-31"},  // New Year's on a Saturday isn't observed the 31st
	}
	for _, c := range cases {
		m, _ := time.Parse("2006-01-02", c.month)
		if got := PayDay(m, c.moneyMarket).Format("2006-01-02"); got != c.want {
			t.Errorf("PayDay(%s, %v) = %s, want %s", c.month, c.moneyMarket, got, c.want)
		}
	}
}

func TestEaster(t *testing.T) {
	for y, want := range map[int]string{2024: "2024-03-31", 2025: "2025-04-20", 2026: "2026-04-05", 2027: "2027-03-28"} {
		if got := easter(y).Format("2006-01-02"); got != want {
			t.Errorf("easter(%d) = %s, want %s", y, got, want)
		}
	}
}
