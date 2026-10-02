// Package interest works out a month's interest on an account's cash.
// It is pure: no database, no clock. The ledger supplies each day's closing
// balance and the account's rate history; this decides what the month earned.
//
// It accrues a day at a time, the way a bank or a money market fund does:
//
//	interest = the sum, over the month's days, of that day's balance × that day's rate
//
// where a day's rate is the monthly equivalent of the APY in effect that day,
// divided by the days in the month. So a balance that never moves earns exactly
// balance × the monthly rate, money deposited on the 20th earns from the 20th,
// and a rate that changes from 4.00% to 4.35% on the 12th counts 11 days at
// 4.00% and the rest at 4.35%. Days before the first rate (or before the
// account existed) earn nothing, which is also what makes a first, partial
// month come out right.
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

// ForMonth is the interest earned in the month starting at monthStart by a
// balance that stays the same all month, rounded to the cent.
func ForMonth(monthStart time.Time, balance float64, rates []Rate, accruesFrom time.Time) float64 {
	balances := make([]float64, MonthEnd(monthStart).Day())
	for i := range balances {
		balances[i] = balance
	}
	return ForDays(monthStart, balances, rates, accruesFrom)
}

// ForDays is the interest earned in the month starting at monthStart, rounded
// to the cent. balances holds each day's closing balance, the 1st first; a day
// past its end earns nothing. accruesFrom is the first day that can earn
// anything (the account's starting-balance date). A day whose balance is at or
// below zero earns nothing rather than costing interest.
func ForDays(monthStart time.Time, balances []float64, rates []Rate, accruesFrom time.Time) float64 {
	if len(rates) == 0 {
		return 0
	}
	sorted := append([]Rate(nil), rates...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].From.Before(sorted[j].From) })

	end := MonthEnd(monthStart)
	days := float64(end.Day())
	var total float64
	for i, d := 0, monthStart; !d.After(end) && i < len(balances); i, d = i+1, d.AddDate(0, 0, 1) {
		if d.Before(accruesFrom) || balances[i] <= 0 {
			continue
		}
		if r, ok := rateOn(sorted, d); ok {
			total += balances[i] * MonthlyRate(r.APY) / days
		}
	}
	return round2(total)
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
