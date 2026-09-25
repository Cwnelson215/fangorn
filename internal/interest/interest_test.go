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
