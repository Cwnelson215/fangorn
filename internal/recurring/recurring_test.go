package recurring

import (
	"testing"
	"time"
)

func d(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("bad test date %q: %v", s, err)
	}
	return v
}

func ptr[T any](v T) *T { return &v }

// dates runs the rule from its start and returns the first n occurrences as strings.
func dates(t *testing.T, r Rule, n int) []string {
	t.Helper()
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		v, ok := r.Nth(i)
		if !ok {
			break
		}
		out = append(out, v.Format("2006-01-02"))
	}
	return out
}

func assertDates(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d dates %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("occurrence %d: got %s, want %s (full: %v)", i, got[i], want[i], got)
		}
	}
}

func TestDaily(t *testing.T) {
	r := Rule{Frequency: Daily, IntervalCount: 1, StartDate: d(t, "2026-01-30")}
	assertDates(t, dates(t, r, 4), []string{"2026-01-30", "2026-01-31", "2026-02-01", "2026-02-02"})
}

func TestDailyInterval(t *testing.T) {
	r := Rule{Frequency: Daily, IntervalCount: 10, StartDate: d(t, "2026-01-01")}
	assertDates(t, dates(t, r, 4), []string{"2026-01-01", "2026-01-11", "2026-01-21", "2026-01-31"})
}

func TestWeeklyUsesStartWeekdayWhenUnset(t *testing.T) {
	// 2026-08-25 is a Tuesday.
	r := Rule{Frequency: Weekly, IntervalCount: 1, StartDate: d(t, "2026-08-25")}
	assertDates(t, dates(t, r, 3), []string{"2026-08-25", "2026-09-01", "2026-09-08"})
}

func TestWeeklyAdvancesToRequestedWeekday(t *testing.T) {
	// Start Tuesday 2026-08-25, ask for Friday (5) -> first occurrence 2026-08-28.
	r := Rule{Frequency: Weekly, IntervalCount: 1, DayOfWeek: ptr(5), StartDate: d(t, "2026-08-25")}
	assertDates(t, dates(t, r, 3), []string{"2026-08-28", "2026-09-04", "2026-09-11"})
}

func TestWeeklyStartAlreadyOnRequestedWeekday(t *testing.T) {
	// Tuesday = 2. The start date itself must be the first occurrence, not +7.
	r := Rule{Frequency: Weekly, IntervalCount: 1, DayOfWeek: ptr(2), StartDate: d(t, "2026-08-25")}
	assertDates(t, dates(t, r, 2), []string{"2026-08-25", "2026-09-01"})
}

func TestBiweekly(t *testing.T) {
	r := Rule{Frequency: Biweekly, IntervalCount: 1, StartDate: d(t, "2026-08-25")}
	assertDates(t, dates(t, r, 3), []string{"2026-08-25", "2026-09-08", "2026-09-22"})
}

func TestMonthly(t *testing.T) {
	r := Rule{Frequency: Monthly, IntervalCount: 1, StartDate: d(t, "2026-03-15")}
	assertDates(t, dates(t, r, 3), []string{"2026-03-15", "2026-04-15", "2026-05-15"})
}

// The headline case: a rule anchored on the 31st must clamp into short months
// and then return to the 31st, rather than sticking at the clamped day.
func TestMonthlyClampsToShortMonthsAndRecovers(t *testing.T) {
	r := Rule{Frequency: Monthly, IntervalCount: 1, DayOfMonth: ptr(31), StartDate: d(t, "2026-01-31")}
	assertDates(t, dates(t, r, 6), []string{
		"2026-01-31",
		"2026-02-28", // clamped
		"2026-03-31", // back to the anchor
		"2026-04-30", // clamped
		"2026-05-31",
		"2026-06-30",
	})
}

func TestMonthlyClampsToFeb29InLeapYear(t *testing.T) {
	// 2028 is a leap year.
	r := Rule{Frequency: Monthly, IntervalCount: 1, DayOfMonth: ptr(31), StartDate: d(t, "2028-01-31")}
	assertDates(t, dates(t, r, 3), []string{"2028-01-31", "2028-02-29", "2028-03-31"})
}

func TestMonthlyStartsNextMonthWhenAnchorAlreadyPassed(t *testing.T) {
	// Anchor is the 5th but the rule starts on the 20th, so January is skipped.
	r := Rule{Frequency: Monthly, IntervalCount: 1, DayOfMonth: ptr(5), StartDate: d(t, "2026-01-20")}
	assertDates(t, dates(t, r, 3), []string{"2026-02-05", "2026-03-05", "2026-04-05"})
}

func TestMonthlyEveryOtherMonth(t *testing.T) {
	r := Rule{Frequency: Monthly, IntervalCount: 2, StartDate: d(t, "2026-01-10")}
	assertDates(t, dates(t, r, 3), []string{"2026-01-10", "2026-03-10", "2026-05-10"})
}

func TestQuarterly(t *testing.T) {
	r := Rule{Frequency: Quarterly, IntervalCount: 1, StartDate: d(t, "2026-01-15")}
	assertDates(t, dates(t, r, 5), []string{
		"2026-01-15", "2026-04-15", "2026-07-15", "2026-10-15", "2027-01-15",
	})
}

func TestQuarterlyClamps(t *testing.T) {
	// Nov 30 -> Feb 28 -> May 30. The Feb clamp must not stick.
	r := Rule{Frequency: Quarterly, IntervalCount: 1, DayOfMonth: ptr(30), StartDate: d(t, "2026-11-30")}
	assertDates(t, dates(t, r, 3), []string{"2026-11-30", "2027-02-28", "2027-05-30"})
}

func TestSemimonthlyDefaults(t *testing.T) {
	r := Rule{Frequency: Semimonthly, IntervalCount: 1, StartDate: d(t, "2026-01-01")}
	assertDates(t, dates(t, r, 5), []string{
		"2026-01-01", "2026-01-15", "2026-02-01", "2026-02-15", "2026-03-01",
	})
}

func TestSemimonthlySkipsPastDatesInStartMonth(t *testing.T) {
	// Starting on the 10th, the 1st has passed but the 15th has not.
	r := Rule{Frequency: Semimonthly, IntervalCount: 1, StartDate: d(t, "2026-01-10")}
	assertDates(t, dates(t, r, 4), []string{
		"2026-01-15", "2026-02-01", "2026-02-15", "2026-03-01",
	})
}

func TestSemimonthlySkipsWholeStartMonth(t *testing.T) {
	// Both anchors already passed, so the series opens in February.
	r := Rule{Frequency: Semimonthly, IntervalCount: 1, StartDate: d(t, "2026-01-20")}
	assertDates(t, dates(t, r, 3), []string{"2026-02-01", "2026-02-15", "2026-03-01"})
}

func TestSemimonthlyCustomDaysAreOrdered(t *testing.T) {
	// Given out of order (25th then 10th), the series must still run 10th, 25th.
	r := Rule{
		Frequency: Semimonthly, IntervalCount: 1,
		DayOfMonth: ptr(25), SecondDayOfMonth: ptr(10),
		StartDate: d(t, "2026-01-01"),
	}
	assertDates(t, dates(t, r, 4), []string{
		"2026-01-10", "2026-01-25", "2026-02-10", "2026-02-25",
	})
}

func TestSemimonthlyClampsBothAnchors(t *testing.T) {
	r := Rule{
		Frequency: Semimonthly, IntervalCount: 1,
		DayOfMonth: ptr(15), SecondDayOfMonth: ptr(31),
		StartDate: d(t, "2026-01-15"),
	}
	assertDates(t, dates(t, r, 4), []string{
		"2026-01-15", "2026-01-31", "2026-02-15", "2026-02-28",
	})
}

func TestYearly(t *testing.T) {
	r := Rule{Frequency: Yearly, IntervalCount: 1, StartDate: d(t, "2026-07-04")}
	assertDates(t, dates(t, r, 3), []string{"2026-07-04", "2027-07-04", "2028-07-04"})
}

func TestYearlyLeapDayClampsInNonLeapYears(t *testing.T) {
	// 2028 is a leap year; 2029-2031 are not; 2032 is.
	r := Rule{
		Frequency: Yearly, IntervalCount: 1,
		MonthOfYear: ptr(2), DayOfMonth: ptr(29),
		StartDate: d(t, "2028-02-29"),
	}
	assertDates(t, dates(t, r, 5), []string{
		"2028-02-29", "2029-02-28", "2030-02-28", "2031-02-28", "2032-02-29",
	})
}

func TestYearlyStartsNextYearWhenAnchorPassed(t *testing.T) {
	r := Rule{
		Frequency: Yearly, IntervalCount: 1,
		MonthOfYear: ptr(3), DayOfMonth: ptr(1),
		StartDate: d(t, "2026-06-01"),
	}
	assertDates(t, dates(t, r, 2), []string{"2027-03-01", "2028-03-01"})
}

func TestEndDateStopsTheSeries(t *testing.T) {
	end := d(t, "2026-03-31")
	r := Rule{Frequency: Monthly, IntervalCount: 1, StartDate: d(t, "2026-01-15"), EndDate: &end}
	assertDates(t, dates(t, r, 10), []string{"2026-01-15", "2026-02-15", "2026-03-15"})
}

func TestEndDateIsInclusive(t *testing.T) {
	end := d(t, "2026-03-15")
	r := Rule{Frequency: Monthly, IntervalCount: 1, StartDate: d(t, "2026-01-15"), EndDate: &end}
	assertDates(t, dates(t, r, 10), []string{"2026-01-15", "2026-02-15", "2026-03-15"})
}

func TestNextIsStrictlyAfter(t *testing.T) {
	r := Rule{Frequency: Monthly, IntervalCount: 1, StartDate: d(t, "2026-01-15")}

	// A cutoff landing exactly on an occurrence must return the following one.
	got, ok := r.Next(d(t, "2026-02-15"))
	if !ok {
		t.Fatal("expected an occurrence after 2026-02-15")
	}
	if want := "2026-03-15"; got.Format("2006-01-02") != want {
		t.Errorf("Next(2026-02-15) = %s, want %s", got.Format("2006-01-02"), want)
	}

	// A cutoff before the series starts returns the first occurrence.
	got, ok = r.Next(d(t, "2025-12-01"))
	if !ok {
		t.Fatal("expected an occurrence after 2025-12-01")
	}
	if want := "2026-01-15"; got.Format("2006-01-02") != want {
		t.Errorf("Next(2025-12-01) = %s, want %s", got.Format("2006-01-02"), want)
	}
}

func TestNextReturnsFalsePastEndDate(t *testing.T) {
	end := d(t, "2026-03-31")
	r := Rule{Frequency: Monthly, IntervalCount: 1, StartDate: d(t, "2026-01-15"), EndDate: &end}
	if _, ok := r.Next(d(t, "2026-04-01")); ok {
		t.Error("expected no occurrence after the rule's end date")
	}
}

func TestNextIgnoresTimeOfDay(t *testing.T) {
	r := Rule{Frequency: Daily, IntervalCount: 1, StartDate: d(t, "2026-01-01")}
	afternoon := time.Date(2026, 1, 5, 15, 30, 0, 0, time.FixedZone("MST", -7*3600))
	got, ok := r.Next(afternoon)
	if !ok {
		t.Fatal("expected an occurrence")
	}
	if want := "2026-01-06"; got.Format("2006-01-02") != want {
		t.Errorf("Next(%s) = %s, want %s", afternoon, got.Format("2006-01-02"), want)
	}
}

func TestOccurrencesWindowIsInclusive(t *testing.T) {
	r := Rule{Frequency: Monthly, IntervalCount: 1, StartDate: d(t, "2026-01-15")}
	got := r.Occurrences(d(t, "2026-02-15"), d(t, "2026-04-15"), 0)

	want := []string{"2026-02-15", "2026-03-15", "2026-04-15"}
	if len(got) != len(want) {
		t.Fatalf("got %d occurrences, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Format("2006-01-02") != want[i] {
			t.Errorf("occurrence %d: got %s, want %s", i, got[i].Format("2006-01-02"), want[i])
		}
	}
}

func TestOccurrencesRespectsLimit(t *testing.T) {
	r := Rule{Frequency: Daily, IntervalCount: 1, StartDate: d(t, "2026-01-01")}
	if got := r.Occurrences(d(t, "2026-01-01"), d(t, "2026-12-31"), 5); len(got) != 5 {
		t.Errorf("got %d occurrences, want 5", len(got))
	}
}

// A rule that started well in the past must backfill every missed date exactly
// once — this is what makes scheduler catch-up correct after downtime.
func TestOccurrencesBackfillsFromThePast(t *testing.T) {
	r := Rule{Frequency: Monthly, IntervalCount: 1, StartDate: d(t, "2026-02-10")}
	got := r.Occurrences(d(t, "2026-02-10"), d(t, "2026-08-25"), 0)

	want := []string{
		"2026-02-10", "2026-03-10", "2026-04-10",
		"2026-05-10", "2026-06-10", "2026-07-10", "2026-08-10",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d occurrences %v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i].Format("2006-01-02") != want[i] {
			t.Errorf("occurrence %d: got %s, want %s", i, got[i].Format("2006-01-02"), want[i])
		}
	}
}

func TestValidate(t *testing.T) {
	start := d(t, "2026-01-01")
	earlier := d(t, "2025-01-01")

	tests := []struct {
		name    string
		rule    Rule
		wantErr bool
	}{
		{"valid", Rule{Frequency: Monthly, IntervalCount: 1, StartDate: start}, false},
		{"unknown frequency", Rule{Frequency: "fortnightly", IntervalCount: 1, StartDate: start}, true},
		{"zero interval", Rule{Frequency: Monthly, IntervalCount: 0, StartDate: start}, true},
		{"negative interval", Rule{Frequency: Monthly, IntervalCount: -1, StartDate: start}, true},
		{"missing start", Rule{Frequency: Monthly, IntervalCount: 1}, true},
		{"end before start", Rule{Frequency: Monthly, IntervalCount: 1, StartDate: start, EndDate: &earlier}, true},
		{"day 0", Rule{Frequency: Monthly, IntervalCount: 1, StartDate: start, DayOfMonth: ptr(0)}, true},
		{"day 32", Rule{Frequency: Monthly, IntervalCount: 1, StartDate: start, DayOfMonth: ptr(32)}, true},
		{"weekday 7", Rule{Frequency: Weekly, IntervalCount: 1, StartDate: start, DayOfWeek: ptr(7)}, true},
		{"month 13", Rule{Frequency: Yearly, IntervalCount: 1, StartDate: start, MonthOfYear: ptr(13)}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.rule.Validate()
			if tc.wantErr && err == nil {
				t.Error("expected an error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestInvalidRuleProducesNoDates(t *testing.T) {
	r := Rule{Frequency: "nonsense", IntervalCount: 1, StartDate: d(t, "2026-01-01")}
	if _, ok := r.Nth(0); ok {
		t.Error("invalid rule should not produce an occurrence")
	}
	if _, ok := r.Next(d(t, "2026-01-01")); ok {
		t.Error("invalid rule should have no next occurrence")
	}
	if got := r.Occurrences(d(t, "2026-01-01"), d(t, "2027-01-01"), 0); len(got) != 0 {
		t.Errorf("invalid rule produced %d occurrences", len(got))
	}
}

func TestNegativeIndexIsRejected(t *testing.T) {
	r := Rule{Frequency: Daily, IntervalCount: 1, StartDate: d(t, "2026-01-01")}
	if _, ok := r.Nth(-1); ok {
		t.Error("Nth(-1) should not be ok")
	}
}

func TestAddMonthsAcrossYearBoundaries(t *testing.T) {
	tests := []struct {
		year  int
		month time.Month
		n     int
		wantY int
		wantM time.Month
	}{
		{2026, time.January, 0, 2026, time.January},
		{2026, time.December, 1, 2027, time.January},
		{2026, time.January, 12, 2027, time.January},
		{2026, time.January, 25, 2028, time.February},
		{2026, time.January, -1, 2025, time.December},
		{2026, time.January, -13, 2024, time.December},
	}
	for _, tc := range tests {
		y, m := addMonths(tc.year, tc.month, tc.n)
		if y != tc.wantY || m != tc.wantM {
			t.Errorf("addMonths(%d, %s, %d) = (%d, %s), want (%d, %s)",
				tc.year, tc.month, tc.n, y, m, tc.wantY, tc.wantM)
		}
	}
}

func TestDaysInMonth(t *testing.T) {
	tests := []struct {
		year int
		m    time.Month
		want int
	}{
		{2026, time.January, 31},
		{2026, time.February, 28},
		{2028, time.February, 29}, // leap
		{2100, time.February, 28}, // century, not a leap year
		{2000, time.February, 29}, // divisible by 400, is a leap year
		{2026, time.April, 30},
	}
	for _, tc := range tests {
		if got := daysInMonth(tc.year, tc.m); got != tc.want {
			t.Errorf("daysInMonth(%d, %s) = %d, want %d", tc.year, tc.m, got, tc.want)
		}
	}
}
