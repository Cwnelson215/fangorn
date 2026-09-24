// Package vision reads receipts out of photographs.
//
// Callers depend on Extractor, not on Anthropic, the same way the price code
// depends on quotes.Provider rather than Yahoo. It keeps the network client
// testable against a fake server, and lets the receipts processor be tested with
// no network at all.
//
// Nothing this package returns is trusted. The model transcribes a photo taken
// by whoever is holding the phone, and the photo can say anything — so every
// value is validated again by the caller before it touches the ledger.
package vision

import (
	"context"
	"errors"
)

// ErrUnavailable means the extraction did not happen but might if tried again:
// a rate limit, an overloaded API, a timeout, a network failure, a bad key.
var ErrUnavailable = errors.New("vision service unavailable")

// ErrUnreadable means trying again with the same photo will not help: the model
// declined, ran out of room, or the request itself was rejected. The receipt
// should go to a person instead.
var ErrUnreadable = errors.New("receipt could not be read")

// Extractor turns one receipt photo into structured data.
type Extractor interface {
	Extract(ctx context.Context, image []byte, mediaType string, hints Hints) (Extraction, Meta, error)
}

// Hints is what the model is told besides the photo. It is deliberately small:
// nothing about accounts, card numbers or balances goes to the model, because
// none of that is needed to read a receipt.
type Hints struct {
	// Today is the household's current date, YYYY-MM-DD. Receipts print
	// two-digit years and no year at all often enough that this matters.
	Today string
	// Categories are the household's expense category names, so the model picks
	// one of them rather than inventing its own.
	Categories []string
}

// Extraction is what the model read. Pointer fields are nil when the receipt
// did not show that value; the model is told to leave them null rather than
// guess.
type Extraction struct {
	IsReceipt       bool       `json:"is_receipt"`
	Merchant        *string    `json:"merchant"`
	PurchaseDate    *string    `json:"purchase_date"`
	Currency        *string    `json:"currency"`
	TransactionType string     `json:"transaction_type"`
	Subtotal        *float64   `json:"subtotal"`
	Tax             *float64   `json:"tax"`
	Tip             *float64   `json:"tip"`
	Total           *float64   `json:"total"`
	Tender          string     `json:"tender"`
	CardLast4       *string    `json:"card_last4"`
	Category        *string    `json:"category"`
	LineItems       []LineItem `json:"line_items"`
}

type LineItem struct {
	Description string   `json:"description"`
	Quantity    *float64 `json:"quantity"`
	Amount      float64  `json:"amount"`
}

// Meta describes the call that produced an extraction.
type Meta struct {
	// Model is the model that actually answered, which is not always the one
	// asked for: a declined request can be re-served by a fallback model.
	Model        string
	InputTokens  int
	OutputTokens int
}
