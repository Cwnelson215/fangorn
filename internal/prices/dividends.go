package prices

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/cwnelson/fangorn/internal/quotes"
)

// Dividends are declared a few times a year, so each symbol is asked about at
// most every dividendTTL. The lookup reaches back dividendWindow, comfortably
// past how far the ledger looks when it raises one.
const (
	dividendTTL    = 12 * time.Hour
	dividendRetry  = 30 * time.Minute
	dividendWindow = 120 * 24 * time.Hour
)

// RefreshDividends looks up the declared dividends of every symbol the
// household's open accounts have traded that hasn't been checked in
// dividendTTL. Failures are joined for logging; the ledger raises dividends
// from whatever is stored either way.
func (r *Refresher) RefreshDividends(ctx context.Context, householdID int) error {
	if !r.Enabled() {
		return nil
	}
	dp, ok := r.provider.(quotes.DividendProvider)
	if !ok {
		return nil
	}
	due, err := r.svc.DividendSymbolsDue(ctx, householdID, r.now().Add(-dividendTTL))
	if err != nil {
		return err
	}
	var errs []error
	for _, symbol := range due {
		r.dividendMu.Lock()
		recent := r.now().Sub(r.dividendTried[symbol]) < dividendRetry
		if !recent {
			r.dividendTried[symbol] = r.now()
		}
		r.dividendMu.Unlock()
		if recent {
			continue
		}
		events, err := dp.Dividends(ctx, symbol, r.now().Add(-dividendWindow))
		if err != nil && !errors.Is(err, quotes.ErrNotFound) {
			log.Printf("prices: dividends for %s: %v", symbol, err)
			errs = append(errs, err)
			continue
		}
		if err := r.svc.SaveDividends(ctx, symbol, events, r.now()); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
