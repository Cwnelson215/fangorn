package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"

	"github.com/cwnelson/fangorn/internal/models"
	"github.com/cwnelson/fangorn/internal/quotes"
)

// securities and security_prices are the one exception to household scoping: a
// fund's price is public market data, shared by everyone who holds it. Nothing
// in them says who holds what — that lives in trades, which is scoped normally.

const securitySelect = `
	SELECT symbol, name, quote_type, currency, exchange, last_price, previous_close,
	       price_time, fetch_error
	FROM securities`

func scanSecurity(row interface{ Scan(...any) error }) (models.Security, error) {
	var sec models.Security
	var name, quoteType, exchange, fetchError sql.NullString
	var prev sql.NullFloat64
	var priceTime sql.NullTime
	err := row.Scan(&sec.Symbol, &name, &quoteType, &sec.Currency, &exchange, &sec.Price,
		&prev, &priceTime, &fetchError)
	if err != nil {
		return sec, err
	}
	sec.Name = strPtr(name)
	sec.QuoteType = strPtr(quoteType)
	sec.Exchange = strPtr(exchange)
	sec.PreviousClose = floatPtr(prev)
	sec.FetchError = strPtr(fetchError)
	if priceTime.Valid {
		s := priceTime.Time.UTC().Format(time.RFC3339)
		sec.PriceTime = &s
	}
	return sec, nil
}

func (s *Service) GetSecurity(ctx context.Context, symbol string) (models.Security, error) {
	sec, err := scanSecurity(s.db.QueryRowContext(ctx,
		securitySelect+` WHERE symbol = $1`, NormalizeSymbol(symbol)))
	if err == sql.ErrNoRows {
		return sec, ErrNotFound
	}
	if err != nil {
		return sec, fmt.Errorf("fetching security: %w", err)
	}
	return sec, nil
}

func (s *Service) securitiesBySymbol(ctx context.Context, symbols []string) (map[string]models.Security, error) {
	rows, err := s.db.QueryContext(ctx, securitySelect+` WHERE symbol = ANY($1)`, pq.Array(symbols))
	if err != nil {
		return nil, fmt.Errorf("loading securities: %w", err)
	}
	defer rows.Close()
	out := map[string]models.Security{}
	for rows.Next() {
		sec, err := scanSecurity(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning security: %w", err)
		}
		out[sec.Symbol] = sec
	}
	return out, rows.Err()
}

// SecurityState is what the price refresher needs to decide whether a symbol is
// due for a fetch.
type SecurityState struct {
	Symbol    string
	QuoteType string
	FetchedAt *time.Time
	FailedAt  *time.Time
}

// SecurityStates returns the refresh state of each symbol that has a row.
// Symbols with no row are simply absent.
func (s *Service) SecurityStates(ctx context.Context, symbols []string) ([]SecurityState, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT symbol, COALESCE(quote_type, ''), fetched_at, failed_at
		 FROM securities WHERE symbol = ANY($1) ORDER BY symbol`, pq.Array(symbols))
	if err != nil {
		return nil, fmt.Errorf("loading security states: %w", err)
	}
	defer rows.Close()
	var out []SecurityState
	for rows.Next() {
		var st SecurityState
		var fetched, failed sql.NullTime
		if err := rows.Scan(&st.Symbol, &st.QuoteType, &fetched, &failed); err != nil {
			return nil, err
		}
		if fetched.Valid {
			st.FetchedAt = &fetched.Time
		}
		if failed.Valid {
			st.FailedAt = &failed.Time
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// SaveQuote records a fetched quote and its recent daily closes, and clears any
// earlier fetch failure. It creates the securities row if there isn't one.
func (s *Service) SaveQuote(ctx context.Context, q quotes.Quote, fetchedAt time.Time) error {
	return s.inTx(func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO securities
			   (symbol, name, quote_type, currency, exchange, last_price, previous_close,
			    price_time, fetched_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			 ON CONFLICT (symbol) DO UPDATE SET
			   name = EXCLUDED.name, quote_type = EXCLUDED.quote_type,
			   currency = EXCLUDED.currency, exchange = EXCLUDED.exchange,
			   last_price = EXCLUDED.last_price, previous_close = EXCLUDED.previous_close,
			   price_time = EXCLUDED.price_time, fetched_at = EXCLUDED.fetched_at,
			   failed_at = NULL, fetch_error = NULL, updated_at = NOW()`,
			NormalizeSymbol(q.Symbol), q.Name, q.QuoteType, q.Currency, q.Exchange, q.Price,
			q.PreviousClose, q.PriceTime, fetchedAt,
		)
		if err != nil {
			return fmt.Errorf("saving quote: %w", err)
		}
		return upsertCloses(ctx, tx, NormalizeSymbol(q.Symbol), q.Closes)
	})
}

// SaveHistory records backfilled daily closes and how far back they reach.
func (s *Service) SaveHistory(ctx context.Context, symbol string, from time.Time, closes []quotes.Close) error {
	symbol = NormalizeSymbol(symbol)
	return s.inTx(func(tx *sql.Tx) error {
		if err := upsertCloses(ctx, tx, symbol, closes); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`UPDATE securities SET history_from = LEAST(COALESCE(history_from, $2::date), $2::date)
			 WHERE symbol = $1`, symbol, from.Format(models.DateOnly))
		return err
	})
}

func upsertCloses(ctx context.Context, tx *sql.Tx, symbol string, closes []quotes.Close) error {
	for _, c := range closes {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO security_prices (symbol, price_date, close) VALUES ($1, $2, $3)
			 ON CONFLICT (symbol, price_date) DO UPDATE SET close = EXCLUDED.close`,
			symbol, c.Date.Format(models.DateOnly), c.Price)
		if err != nil {
			return fmt.Errorf("saving close: %w", err)
		}
	}
	return nil
}

// MarkQuoteFailed records a failed fetch so the refresher can back off instead
// of retrying a delisted or mistyped symbol on every poll. The last good price
// is left alone.
func (s *Service) MarkQuoteFailed(ctx context.Context, symbol string, at time.Time, msg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE securities SET failed_at = $2, fetch_error = $3, updated_at = NOW() WHERE symbol = $1`,
		NormalizeSymbol(symbol), at, msg)
	if err != nil {
		return fmt.Errorf("recording fetch failure: %w", err)
	}
	return nil
}
