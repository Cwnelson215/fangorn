package ledger

import (
	"testing"
	"time"
)

func TestMonthStartOf(t *testing.T) {
	// The last evening of January where the household lives: "this month" must be
	// January even though UTC has already moved on.
	today := time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC)

	for in, want := range map[string]string{
		"":           "2026-01-01",
		"2026-02":    "2026-02-01",
		"2026-02-17": "2026-02-01",
	} {
		got, err := monthStartOf(in, today)
		if err != nil || got != want {
			t.Errorf("monthStartOf(%q) = %q, %v; want %q", in, got, err, want)
		}
	}

	if _, err := monthStartOf("February", today); err == nil {
		t.Error("monthStartOf(\"February\") succeeded, want an error")
	}
}
