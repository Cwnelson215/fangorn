// Package prices keeps security prices current.
//
// It sits between the quote provider (network, no database) and the ledger
// (database, no network), and owns the one decision neither should make: when a
// stored price is old enough to be worth fetching again. Both the scheduler and
// the holdings endpoints call it; staleness is judged from what is in the
// database, so the two never duplicate each other's fetches.
package prices

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
	"github.com/cwnelson/fangorn/internal/quotes"
)

// maxConcurrentFetches bounds how hard one refresh leans on the provider.
const maxConcurrentFetches = 4

type Refresher struct {
	svc       *ledger.Service
	provider  quotes.Provider
	marketTTL time.Duration
	now       func() time.Time

	// sem admits one refresh at a time. Several open tabs polling at once would
	// otherwise all see the same stale rows and fetch the same symbols in
	// parallel. It is a channel rather than a mutex so a request whose deadline
	// passes while waiting can give up and serve cached prices.
	sem chan struct{}
}

// New builds a refresher. A nil provider disables fetching: trades still work,
// and holdings are valued at the price each symbol was seeded with.
func New(svc *ledger.Service, provider quotes.Provider, marketTTL time.Duration) *Refresher {
	if marketTTL <= 0 {
		marketTTL = time.Minute
	}
	return &Refresher{
		svc: svc, provider: provider, marketTTL: marketTTL,
		now: time.Now, sem: make(chan struct{}, 1),
	}
}

// Enabled reports whether a provider is configured.
func (r *Refresher) Enabled() bool { return r != nil && r.provider != nil }

// RefreshStale fetches whichever of the given symbols are stale and saves them.
// A failure for one symbol is recorded against it and does not stop the rest.
// The returned error joins the individual failures, for logging — callers carry
// on with whatever prices are stored either way.
func (r *Refresher) RefreshStale(ctx context.Context, symbols []string) error {
	if !r.Enabled() || len(symbols) == 0 {
		return nil
	}
	select {
	case r.sem <- struct{}{}:
		defer func() { <-r.sem }()
	case <-ctx.Done():
		return ctx.Err()
	}

	// Read state only once inside: whoever held the semaphore may have just
	// refreshed these very symbols.
	states, err := r.svc.SecurityStates(ctx, symbols)
	if err != nil {
		return err
	}
	now := r.now()
	var due []string
	for _, st := range states {
		if Stale(st, now, r.marketTTL) {
			due = append(due, st.Symbol)
		}
	}
	if len(due) == 0 {
		return nil
	}

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
		gate = make(chan struct{}, maxConcurrentFetches)
	)
	for _, sym := range due {
		wg.Add(1)
		go func() {
			defer wg.Done()
			gate <- struct{}{}
			defer func() { <-gate }()
			if err := r.fetch(ctx, sym); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

func (r *Refresher) fetch(ctx context.Context, symbol string) error {
	q, err := r.provider.Quote(ctx, symbol)
	if err != nil {
		// A request that ran out of time says nothing about the symbol, so it
		// must not trigger the backoff meant for broken ones.
		if ctx.Err() == nil {
			if markErr := r.svc.MarkQuoteFailed(context.WithoutCancel(ctx), symbol, r.now(), err.Error()); markErr != nil {
				log.Printf("prices: recording failure for %s: %v", symbol, markErr)
			}
		}
		return fmt.Errorf("%s: %w", symbol, err)
	}
	if err := r.svc.SaveQuote(ctx, q, r.now()); err != nil {
		return fmt.Errorf("%s: %w", symbol, err)
	}
	return nil
}

// EnsureSecurity makes sure a symbol is known before a trade is logged against
// it, fetching a first quote if there is no row yet.
//
// Only a definite answer blocks the trade: the provider saying the symbol does
// not exist, or that it is priced in a currency the ledger doesn't handle. If
// the provider is merely unreachable the trade goes ahead, seeded with its own
// price, and the next refresh fills in the real quote.
func (r *Refresher) EnsureSecurity(ctx context.Context, symbol string) error {
	symbol = ledger.NormalizeSymbol(symbol)
	if _, err := r.svc.GetSecurity(ctx, symbol); err == nil {
		return nil
	} else if !errors.Is(err, ledger.ErrNotFound) {
		return err
	}
	if !r.Enabled() {
		return nil
	}

	q, err := r.provider.Quote(ctx, symbol)
	switch {
	case errors.Is(err, quotes.ErrNotFound):
		return ledger.ErrInvalid{Msg: fmt.Sprintf("couldn't find a stock or fund with the symbol %s", symbol)}
	case err != nil:
		log.Printf("prices: first quote for %s unavailable, seeding from the trade: %v", symbol, err)
		return nil
	case q.Currency != "" && q.Currency != "USD":
		return ledger.ErrInvalid{Msg: fmt.Sprintf("%s is priced in %s; only US dollar securities are supported", symbol, q.Currency)}
	}
	return r.svc.SaveQuote(ctx, q, r.now())
}

// Quote returns a symbol's current price, fetching it first if it's new or stale.
// It is what the trade form uses to prefill the price field.
func (r *Refresher) Quote(ctx context.Context, symbol string) (models.Security, error) {
	symbol = ledger.NormalizeSymbol(symbol)
	if err := r.EnsureSecurity(ctx, symbol); err != nil {
		return models.Security{}, err
	}
	if err := r.RefreshStale(ctx, []string{symbol}); err != nil {
		log.Printf("prices: refreshing %s: %v", symbol, err)
	}
	sec, err := r.svc.GetSecurity(ctx, symbol)
	if errors.Is(err, ledger.ErrNotFound) {
		// New symbol and the provider couldn't be reached.
		return models.Security{}, quotes.ErrUnavailable
	}
	return sec, err
}

// Search passes a symbol search through to the provider.
func (r *Refresher) Search(ctx context.Context, query string) ([]quotes.Match, error) {
	if !r.Enabled() {
		return nil, quotes.ErrUnavailable
	}
	return r.provider.Search(ctx, query)
}
