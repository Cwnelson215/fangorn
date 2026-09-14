// Package quotes fetches market prices for securities.
//
// Callers depend on Provider, not on Yahoo. The Yahoo implementation is free and
// needs no key, but it is an unofficial API that can change shape or rate-limit
// without notice — so it sits behind an interface that a keyed provider could
// replace without touching the ledger or the refresher.
package quotes

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound means the provider does not know the symbol at all, as opposed to
// failing to answer. Only this one is safe to show a user as "no such symbol".
var ErrNotFound = errors.New("symbol not found")

// ErrUnavailable wraps every other failure: a rate limit, a timeout, a changed
// response format. The caller should fall back to the last price it has.
var ErrUnavailable = errors.New("price service unavailable")

// Quote is a security's current price plus its recent daily closes.
type Quote struct {
	Symbol    string
	Name      string
	QuoteType string // EQUITY, ETF, MUTUALFUND, …
	Currency  string
	Exchange  string

	Price float64
	// PreviousClose is the close the day's change is measured against. For a
	// mutual fund that is the NAV before the latest one, not the prior weekday.
	PreviousClose float64
	// PriceTime is when Price was set. A mutual fund's NAV is struck once a day
	// after the close, so this can be many hours old and still be current.
	PriceTime time.Time

	// Recent daily closes, oldest first. Refreshing upserts these so the daily
	// history stays filled in without a separate backfill pass.
	Closes []Close
}

// Close is one day's closing price. Date is the trading day at midnight UTC,
// matching how the rest of the app represents a DATE.
type Close struct {
	Date  time.Time
	Price float64
}

// Match is one symbol search result.
type Match struct {
	Symbol    string `json:"symbol"`
	Name      string `json:"name"`
	QuoteType string `json:"quote_type"`
	Exchange  string `json:"exchange"`
}

type Provider interface {
	Quote(ctx context.Context, symbol string) (Quote, error)
	// History returns daily closes from `from` to today, oldest first.
	History(ctx context.Context, symbol string, from time.Time) ([]Close, error)
	Search(ctx context.Context, query string) ([]Match, error)
}
