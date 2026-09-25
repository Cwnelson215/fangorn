package prices

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/cwnelson/fangorn/internal/quotes"
)

// A money market fund's yield changes a little every day, and an investment
// account linked to one (its cash_fund) earns at that yield. The scheduler asks
// for each linked fund's yield at most every yieldTTL and records one value per
// day; the ledger turns that history into the month's dividend.
const (
	yieldTTL = 12 * time.Hour
	// yieldRetry spaces out attempts for a fund whose lookup failed, so an outage
	// doesn't mean a request every scheduler tick.
	yieldRetry = 30 * time.Minute
)

func (r *Refresher) yieldProvider() (quotes.YieldProvider, bool) {
	if !r.Enabled() {
		return nil, false
	}
	yp, ok := r.provider.(quotes.YieldProvider)
	return yp, ok
}

// RefreshYields looks up the yield of every cash fund the household's accounts
// are linked to that hasn't been fetched in yieldTTL. today is the household's
// date the value is recorded under. Failures are joined for logging; the
// dividend falls back to the last recorded yield either way.
func (r *Refresher) RefreshYields(ctx context.Context, householdID int, today time.Time) error {
	if _, ok := r.yieldProvider(); !ok {
		return nil
	}
	due, err := r.svc.CashFundsDue(ctx, householdID, r.now().Add(-yieldTTL))
	if err != nil {
		return err
	}
	var errs []error
	for _, symbol := range due {
		r.yieldMu.Lock()
		recent := r.now().Sub(r.yieldTried[symbol]) < yieldRetry
		if !recent {
			r.yieldTried[symbol] = r.now()
		}
		r.yieldMu.Unlock()
		if recent {
			continue
		}
		if err := r.FetchYield(ctx, symbol, today); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// FetchYield looks up one fund's yield now and records it for today. Linking a
// fund calls it straight away so the account page has a figure to show.
func (r *Refresher) FetchYield(ctx context.Context, symbol string, today time.Time) error {
	yp, ok := r.yieldProvider()
	if !ok {
		return nil
	}
	pct, err := yp.Yield(ctx, symbol)
	if err != nil {
		log.Printf("prices: yield for %s: %v", symbol, err)
		return err
	}
	return r.svc.SaveFundYield(ctx, symbol, today, pct)
}
