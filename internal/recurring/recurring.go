// Package recurring computes the dates a recurring rule fires on.
//
// Everything here is pure: no database, no clock, no I/O. Callers pass in dates
// and get dates back, which is what makes the whole thing testable.
//
// The central design choice is that occurrences are computed by INDEX from a
// fixed anchor, never by adding an interval to the previous occurrence. That
// distinction matters for month-end rules: a rule anchored on the 31st fires on
// Feb 28, and then must return to Mar 31 — not stay stuck on the 28th. Deriving
// each date independently from the anchor makes that fall out for free, and also
// means no accumulated drift over years of occurrences.
package recurring

import (
	"errors"
	"fmt"
	"time"
)

// maxScan bounds the linear search in Next/Occurrences. A daily rule running for
// 30 years is ~11k occurrences, so this is far above any real ledger while still
// guaranteeing termination if a rule is somehow malformed.
const maxScan = 20000

// Rule is the subset of a recurring_rules row that determines its dates.
type Rule struct {
	Frequency        string
	IntervalCount    int
	DayOfMonth       *int
	SecondDayOfMonth *int
	DayOfWeek        *int // 0 = Sunday .. 6 = Saturday
	MonthOfYear      *int // 1 = January .. 12 = December
	StartDate        time.Time
	EndDate          *time.Time
}

// Frequency values, mirroring the recurring_frequency_check constraint.
const (
	Daily       = "daily"
	Weekly      = "weekly"
	Biweekly    = "biweekly"
	Semimonthly = "semimonthly"
	Monthly     = "monthly"
	Quarterly   = "quarterly"
	Yearly      = "yearly"
)

// Validate reports whether the rule can produce dates at all.
func (r Rule) Validate() error {
	switch r.Frequency {
	case Daily, Weekly, Biweekly, Semimonthly, Monthly, Quarterly, Yearly:
	default:
		return fmt.Errorf("unknown frequency %q", r.Frequency)
	}
	if r.IntervalCount < 1 {
		return errors.New("interval_count must be at least 1")
	}
	if r.StartDate.IsZero() {
		return errors.New("start_date is required")
	}
	if r.EndDate != nil && r.EndDate.Before(r.StartDate) {
		return errors.New("end_date must not be before start_date")
	}
	if d := r.DayOfMonth; d != nil && (*d < 1 || *d > 31) {
		return fmt.Errorf("day_of_month %d out of range", *d)
	}
	if d := r.SecondDayOfMonth; d != nil && (*d < 1 || *d > 31) {
		return fmt.Errorf("second_day_of_month %d out of range", *d)
	}
	if d := r.DayOfWeek; d != nil && (*d < 0 || *d > 6) {
		return fmt.Errorf("day_of_week %d out of range", *d)
	}
	if m := r.MonthOfYear; m != nil && (*m < 1 || *m > 12) {
		return fmt.Errorf("month_of_year %d out of range", *m)
	}
	return nil
}

// Nth returns the nth occurrence (0-based) of the rule. ok is false when n is
// negative, the rule is invalid, or the date would fall past EndDate.
func (r Rule) Nth(n int) (time.Time, bool) {
	if n < 0 || r.Validate() != nil {
		return time.Time{}, false
	}

	var d time.Time
	switch r.Frequency {
	case Daily:
		d = day(r.StartDate).AddDate(0, 0, n*r.IntervalCount)
	case Weekly:
		d = r.weeklyAnchor().AddDate(0, 0, n*7*r.IntervalCount)
	case Biweekly:
		d = r.weeklyAnchor().AddDate(0, 0, n*14*r.IntervalCount)
	case Monthly:
		d = r.monthly(n, r.IntervalCount)
	case Quarterly:
		d = r.monthly(n, 3*r.IntervalCount)
	case Semimonthly:
		d = r.semimonthly(n)
	case Yearly:
		d = r.yearly(n)
	default:
		return time.Time{}, false
	}

	if r.EndDate != nil && d.After(day(*r.EndDate)) {
		return time.Time{}, false
	}
	return d, true
}

// Next returns the first occurrence strictly after `after`. ok is false if the
// rule has no further occurrences (it ended, or it is malformed).
func (r Rule) Next(after time.Time) (time.Time, bool) {
	cutoff := day(after)
	for n := 0; n < maxScan; n++ {
		d, ok := r.Nth(n)
		if !ok {
			return time.Time{}, false
		}
		if d.After(cutoff) {
			return d, true
		}
	}
	return time.Time{}, false
}

// First returns the rule's first occurrence.
func (r Rule) First() (time.Time, bool) { return r.Nth(0) }

// Occurrences returns every occurrence in [from, to] inclusive, in order,
// stopping at limit results (limit <= 0 means no cap beyond maxScan).
func (r Rule) Occurrences(from, to time.Time, limit int) []time.Time {
	lo, hi := day(from), day(to)
	var out []time.Time
	for n := 0; n < maxScan; n++ {
		d, ok := r.Nth(n)
		if !ok || d.After(hi) {
			break
		}
		if d.Before(lo) {
			continue
		}
		out = append(out, d)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// per-frequency anchors
// ---------------------------------------------------------------------------

// weeklyAnchor is the first date on or after StartDate landing on DayOfWeek.
// With no DayOfWeek the rule simply repeats from StartDate's own weekday.
func (r Rule) weeklyAnchor() time.Time {
	start := day(r.StartDate)
	if r.DayOfWeek == nil {
		return start
	}
	delta := (*r.DayOfWeek - int(start.Weekday()) + 7) % 7
	return start.AddDate(0, 0, delta)
}

// monthly handles monthly and quarterly: same anchor logic, different step.
func (r Rule) monthly(n, step int) time.Time {
	start := day(r.StartDate)
	anchor := start.Day()
	if r.DayOfMonth != nil {
		anchor = *r.DayOfMonth
	}

	y, m := start.Year(), start.Month()
	// If the anchor day in the start month already passed, the series begins the
	// following period instead.
	if clampDay(y, m, anchor).Before(start) {
		y, m = addMonths(y, m, step)
	}
	y, m = addMonths(y, m, n*step)
	return clampDay(y, m, anchor)
}

// semimonthly fires twice a month, on DayOfMonth and SecondDayOfMonth
// (defaulting to the 1st and the 15th), stepping IntervalCount months.
func (r Rule) semimonthly(n int) time.Time {
	start := day(r.StartDate)
	d1, d2 := 1, 15
	if r.DayOfMonth != nil {
		d1 = *r.DayOfMonth
	}
	if r.SecondDayOfMonth != nil {
		d2 = *r.SecondDayOfMonth
	}
	if d2 < d1 {
		d1, d2 = d2, d1
	}
	step := r.IntervalCount

	y, m := start.Year(), start.Month()
	// Drop whichever of the month's two dates already passed before StartDate.
	skip := 0
	if clampDay(y, m, d1).Before(start) {
		skip++
	}
	if clampDay(y, m, d2).Before(start) {
		skip++
	}
	if skip == 2 {
		y, m = addMonths(y, m, step)
		skip = 0
	}

	idx := n + skip
	y, m = addMonths(y, m, (idx/2)*step)
	if idx%2 == 0 {
		return clampDay(y, m, d1)
	}
	return clampDay(y, m, d2)
}

func (r Rule) yearly(n int) time.Time {
	start := day(r.StartDate)
	month := start.Month()
	if r.MonthOfYear != nil {
		month = time.Month(*r.MonthOfYear)
	}
	anchor := start.Day()
	if r.DayOfMonth != nil {
		anchor = *r.DayOfMonth
	}

	y := start.Year()
	if clampDay(y, month, anchor).Before(start) {
		y += r.IntervalCount
	}
	return clampDay(y+n*r.IntervalCount, month, anchor)
}

// ---------------------------------------------------------------------------
// date helpers
// ---------------------------------------------------------------------------

// day strips any time-of-day and zone so all arithmetic happens on UTC midnights.
// Everything in this app is a calendar date; keeping them normalized avoids
// DST-shifted comparisons.
func day(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// daysInMonth uses the day-0-of-next-month trick.
func daysInMonth(year int, m time.Month) int {
	return time.Date(year, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// clampDay pins a day-of-month to the end of a short month: the 31st becomes
// Feb 28 (29 in a leap year), Apr 30, and so on.
func clampDay(year int, m time.Month, d int) time.Time {
	if last := daysInMonth(year, m); d > last {
		d = last
	}
	return time.Date(year, m, d, 0, 0, 0, 0, time.UTC)
}

// addMonths advances a (year, month) pair by n months without the day-overflow
// that time.AddDate(0, n, 0) would cause (Jan 31 + 1 month = Mar 3).
func addMonths(year int, m time.Month, n int) (int, time.Month) {
	total := int(m) - 1 + n
	y := year + total/12
	mm := total % 12
	if mm < 0 {
		mm += 12
		y--
	}
	return y, time.Month(mm + 1)
}
