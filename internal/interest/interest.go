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

// PayDay is the day the month starting at monthStart pays what it earned. A
// bank credits savings interest on the month's last day. A money market fund
// (SPAXX, VMFXX) pays its dividend on the month's last business day, covering
// the days after it too: October 2026 ends on a Saturday, and Fidelity paid
// that month's SPAXX dividend on Friday the 30th.
func PayDay(monthStart time.Time, moneyMarket bool) time.Time {
	d := MonthEnd(monthStart)
	if !moneyMarket {
		return d
	}
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday || marketHoliday(d) {
		d = d.AddDate(0, 0, -1)
	}
	return d
}

// marketHoliday reports the NYSE holidays that can fall on a month's last
// weekday: Good Friday (March 20 to April 23, so it can end March) and
// Memorial Day (the last Monday of May). No other one can — New Year's Day on
// a Saturday is not observed on December 31st.
func marketHoliday(d time.Time) bool {
	if d.Month() == time.May && d.Weekday() == time.Monday && d.AddDate(0, 0, 7).Month() != time.May {
		return true
	}
	return d.Equal(easter(d.Year()).AddDate(0, 0, -2))
}

// easter is Easter Sunday in the Gregorian calendar (the anonymous Gregorian
// algorithm).
func easter(y int) time.Time {
	a, b, c := y%19, y/100, y%100
	d, e := b/4, b%4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i, k := c/4, c%4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	n := h + l - 7*m + 114
	return time.Date(y, time.Month(n/31), n%31+1, 0, 0, 0, 0, time.UTC)
}
