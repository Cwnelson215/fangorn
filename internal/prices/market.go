package prices

import (
	"time"

	"github.com/cwnelson/fangorn/internal/ledger"
)

// The server imports time/tzdata, so this cannot fail in the container; the
// fallback only matters for a stripped-down test environment.
var eastern = func() *time.Location {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return time.FixedZone("EST", -5*60*60)
	}
	return loc
}()

// MarketOpen reports whether US exchanges are in regular trading hours:
// weekdays 9:30–16:00 Eastern. Holidays are not modelled — on one, prices are
// just checked more often than they change, which costs a few requests.
func MarketOpen(now time.Time) bool {
	t := now.In(eastern)
	if wd := t.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return false
	}
	minutes := t.Hour()*60 + t.Minute()
	return minutes >= 9*60+30 && minutes < 16*60
}

const (
	// quietTTL is how stale a price may get outside market hours, and for mutual
	// funds at any time. Nothing trades then, but a fund's NAV lands some time
	// after the close — sometimes not until the next morning — so prices are
	// still rechecked on a slow cadence rather than once at 4 PM.
	quietTTL = 15 * time.Minute
	// failureBackoff stops a delisted or mistyped symbol from being refetched on
	// every poll from every open tab.
	failureBackoff = 5 * time.Minute
)

// Stale decides whether a symbol's price is due for a fetch.
//
// marketTTL is the freshness wanted for things that trade continuously while
// the market is open — stocks and ETFs. Mutual funds price once a day, so
// polling one every minute would only ever fetch the same NAV. Crypto trades
// around the clock and ignores market hours entirely.
func Stale(st ledger.SecurityState, now time.Time, marketTTL time.Duration) bool {
	if st.FailedAt != nil && now.Sub(*st.FailedAt) < failureBackoff &&
		(st.FetchedAt == nil || st.FailedAt.After(*st.FetchedAt)) {
		return false
	}
	if st.FetchedAt == nil {
		return true
	}
	age := now.Sub(*st.FetchedAt)

	switch st.QuoteType {
	case "CRYPTOCURRENCY":
		return age >= marketTTL
	case "MUTUALFUND", "MONEYMARKET":
		return age >= quietTTL
	}
	if MarketOpen(now) {
		return age >= marketTTL
	}
	return age >= quietTTL
}
