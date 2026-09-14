package prices

import (
	"testing"
	"time"

	"github.com/cwnelson/fangorn/internal/ledger"
)

func et(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, eastern)
	if err != nil {
		panic(err)
	}
	return t
}

func TestMarketOpen(t *testing.T) {
	cases := map[string]bool{
		"2026-09-14 09:29": false, // Monday, before the bell
		"2026-09-14 09:30": true,
		"2026-09-14 15:59": true,
		"2026-09-14 16:00": false,
		"2026-09-12 12:00": false, // Saturday
		"2026-11-02 10:00": true,  // Monday after the DST change
	}
	for at, want := range cases {
		if got := MarketOpen(et(at)); got != want {
			t.Errorf("MarketOpen(%s ET) = %v, want %v", at, got, want)
		}
	}
	// The same instant expressed in UTC gives the same answer.
	if !MarketOpen(et("2026-09-14 10:00").UTC()) {
		t.Error("MarketOpen should not depend on the zone of its argument")
	}
}

func TestStale(t *testing.T) {
	ago := func(now time.Time, d time.Duration) *time.Time { v := now.Add(-d); return &v }
	open := et("2026-09-14 11:00")
	closed := et("2026-09-14 21:00")
	ttl := time.Minute

	cases := []struct {
		name string
		st   ledger.SecurityState
		now  time.Time
		want bool
	}{
		{"never fetched", ledger.SecurityState{QuoteType: "ETF"}, open, true},
		{"ETF fetched 30s ago, market open", ledger.SecurityState{QuoteType: "ETF", FetchedAt: ago(open, 30*time.Second)}, open, false},
		{"ETF fetched 2m ago, market open", ledger.SecurityState{QuoteType: "ETF", FetchedAt: ago(open, 2*time.Minute)}, open, true},
		{"stock fetched 2m ago, market closed", ledger.SecurityState{QuoteType: "EQUITY", FetchedAt: ago(closed, 2*time.Minute)}, closed, false},
		{"stock fetched 20m ago, market closed", ledger.SecurityState{QuoteType: "EQUITY", FetchedAt: ago(closed, 20*time.Minute)}, closed, true},
		{"fund fetched 2m ago, market open", ledger.SecurityState{QuoteType: "MUTUALFUND", FetchedAt: ago(open, 2*time.Minute)}, open, false},
		{"fund fetched 20m ago", ledger.SecurityState{QuoteType: "MUTUALFUND", FetchedAt: ago(open, 20*time.Minute)}, open, true},
		{"crypto on a weekend", ledger.SecurityState{QuoteType: "CRYPTOCURRENCY", FetchedAt: ago(et("2026-09-12 12:00"), 2*time.Minute)}, et("2026-09-12 12:00"), true},
		{"failed a minute ago", ledger.SecurityState{QuoteType: "ETF", FailedAt: ago(open, time.Minute)}, open, false},
		{"failed ten minutes ago", ledger.SecurityState{QuoteType: "ETF", FailedAt: ago(open, 10*time.Minute)}, open, true},
		{
			"an old failure superseded by a success",
			ledger.SecurityState{QuoteType: "ETF", FailedAt: ago(open, 3*time.Minute), FetchedAt: ago(open, 2*time.Minute)},
			open, true,
		},
	}
	for _, c := range cases {
		if got := Stale(c.st, c.now, ttl); got != c.want {
			t.Errorf("%s: Stale = %v, want %v", c.name, got, c.want)
		}
	}
}
