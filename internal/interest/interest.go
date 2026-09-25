// Package interest works out a high-yield savings account's monthly interest.
// It is pure: no database, no clock. The ledger supplies the month-end balance
// and the account's rate history; this decides what the month earned.
//
// The method is deliberately simple and checkable by hand:
//
//	interest = month-end balance × the month's rate
//
// where the month's rate is the monthly equivalent of the APY, weighted by the
// days each rate was in effect. A rate that changes from 4.00% to 4.35% on the
// 12th counts 11 days at 4.00% and the rest at 4.35%. Days before the first
// rate (or before the account existed) earn nothing, which is also what makes a
// first, partial month come out right.
package interest

import (
	"math"
	"sort"
	"time"
)

// Rate is an APY, in percent (4.35 means 4.35%), in effect from a date until
// the next rate starts.
type Rate struct {
	From time.Time
	APY  float64
}

// MonthlyRate turns an APY in percent into the rate that, compounded twelve
// times, gives that APY: 4.35% APY is about 0.3554% a month.
func MonthlyRate(apyPercent float64) float64 {
	return math.Pow(1+apyPercent/100, 1.0/12) - 1
}

// MonthStart returns the first day of t's month.
func MonthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// MonthEnd returns the last day of the month that starts at monthStart.
func MonthEnd(monthStart time.Time) time.Time {
	return monthStart.AddDate(0, 1, -1)
}

// ForMonth is the interest earned in the month starting at monthStart, on a
// month-end balance, rounded to the cent. accruesFrom is the first day that can
// earn anything (the account's starting-balance date). A balance at or below
// zero earns nothing.
func ForMonth(monthStart time.Time, balance float64, rates []Rate, accruesFrom time.Time) float64 {
	if balance <= 0 || len(rates) == 0 {
		return 0
	}
	return round2(balance * WeightedMonthlyRate(monthStart, rates, accruesFrom))
}

// WeightedMonthlyRate is the month's rate: each day contributes the monthly
// rate in effect that day divided by the days in the month.
func WeightedMonthlyRate(monthStart time.Time, rates []Rate, accruesFrom time.Time) float64 {
	sorted := append([]Rate(nil), rates...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].From.Before(sorted[j].From) })

	end := MonthEnd(monthStart)
	days := end.Day()
	var total float64
	for d := monthStart; !d.After(end); d = d.AddDate(0, 0, 1) {
		if d.Before(accruesFrom) {
			continue
		}
		if r, ok := rateOn(sorted, d); ok {
			total += MonthlyRate(r.APY)
		}
	}
	return total / float64(days)
}

// rateOn is the latest rate starting on or before d.
func rateOn(sorted []Rate, d time.Time) (Rate, bool) {
	var found Rate
	ok := false
	for _, r := range sorted {
		if r.From.After(d) {
			break
		}
		found, ok = r, true
	}
	return found, ok
}

// round2 rounds half away from zero to the cent.
func round2(x float64) float64 {
	return math.Round(x*100) / 100
}
