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

// RefreshDividends looks up the dividends and splits of every symbol the
// household's open accounts have traded that hasn't been checked in
// dividendTTL, then the pay date of any recent dividend still missing one.
// Failures are joined for logging; the ledger raises what is stored either way.
func (r *Refresher) RefreshDividends(ctx context.Context, householdID int) error {
	if !r.Enabled() {
		return nil
	}
	ep, ok := r.provider.(quotes.EventsProvider)
	if !ok {
		return nil
	}
	due, err := r.svc.DividendSymbolsDue(ctx, householdID, r.now().Add(-dividendTTL))
	if err != nil {
		return err
	}
	var (
		errs    []error
		checked []string
	)
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
		events, err := ep.Events(ctx, symbol, r.now().Add(-dividendWindow))
		if err != nil && !errors.Is(err, quotes.ErrNotFound) {
			log.Printf("prices: dividends for %s: %v", symbol, err)
			errs = append(errs, err)
			continue
		}
		if err := r.svc.SaveSplits(ctx, symbol, events.Splits); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := r.svc.SaveDividends(ctx, symbol, events.Dividends, r.now()); err != nil {
			errs = append(errs, err)
			continue
		}
		checked = append(checked, symbol)
	}
	errs = append(errs, r.refreshPayDates(ctx, checked))
	return errors.Join(errs...)
}

// refreshPayDates asks when the latest dividend of each just-checked symbol is
// paid, for those with a recent dividend whose pay date isn't known. It rides
// on the dividend check, so a pay date not announced yet is asked for again
// with the next one. A symbol with none published (every mutual fund) is not
// an error.
func (r *Refresher) refreshPayDates(ctx context.Context, symbols []string) error {
	pp, ok := r.provider.(quotes.PayDateProvider)
	if !ok || len(symbols) == 0 {
		return nil
	}
	wanted, err := r.svc.PayDatesWanted(ctx, symbols, r.now().Add(-dividendWindow))
	if err != nil {
		return err
	}
	var errs []error
	for _, symbol := range wanted {
		ex, pay, err := pp.DividendPayDate(ctx, symbol)
		if errors.Is(err, quotes.ErrNotFound) {
			continue
		}
		if err != nil {
			log.Printf("prices: pay date for %s: %v", symbol, err)
			errs = append(errs, err)
			continue
		}
		if err := r.svc.SaveDividendPayDate(ctx, symbol, ex, pay); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
