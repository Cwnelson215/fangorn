package ledger

import (
	"context"
	"math"
	"time"

	"github.com/cwnelson/fangorn/internal/portfolio"
)

// HoldingPosition is one symbol an investment account holds, valued at the
// latest known price.
type HoldingPosition struct {
	Symbol    string  `json:"symbol"`
	Name      *string `json:"name"`
	QuoteType *string `json:"quote_type"`

	Shares    float64 `json:"shares"`
	AvgCost   float64 `json:"avg_cost"`
	CostBasis float64 `json:"cost_basis"`

	Price         float64  `json:"price"`
	PreviousClose *float64 `json:"previous_close"`
	MarketValue   float64  `json:"market_value"`

	// DayChange is nil when there is no previous close to measure against.
	DayChange    *float64 `json:"day_change"`
	DayChangePct *float64 `json:"day_change_pct"`
	// UnrealizedGainPct is nil when the cost basis is zero (e.g. gifted shares).
	UnrealizedGain    float64  `json:"unrealized_gain"`
	UnrealizedGainPct *float64 `json:"unrealized_gain_pct"`
	RealizedGain      float64  `json:"realized_gain"`
	// Weight is this position's share of the account's holdings value, 0–1.
	Weight float64 `json:"weight"`

	PriceTime  *string `json:"price_time"`
	FetchError *string `json:"fetch_error"`
}

// Holdings is an investment account's positions plus its cash.
type Holdings struct {
	AccountID     int     `json:"account_id"`
	Cash          float64 `json:"cash"`
	HoldingsValue float64 `json:"holdings_value"`
	TotalValue    float64 `json:"total_value"`

	CostBasis      float64  `json:"cost_basis"`
	UnrealizedGain float64  `json:"unrealized_gain"`
	RealizedGain   float64  `json:"realized_gain"`
	DayChange      float64  `json:"day_change"`
	DayChangePct   *float64 `json:"day_change_pct"`

	// AsOf is the oldest price time among the positions, so "as of" is never
	// more optimistic than the stalest number on the page. Nil when nothing is
	// held or no position has ever been quoted.
	AsOf *string `json:"as_of"`
	// Seeded is true when some position is valued at its trade price because no
	// quote has been fetched for it yet.
	Seeded bool `json:"seeded"`

	Positions []HoldingPosition `json:"positions"`
}

// Holdings replays an account's trade log and values each open position at the
// latest stored price. It never fetches prices itself — the handler refreshes
// stale ones first, and the scheduler keeps them current in the background.
func (s *Service) Holdings(ctx context.Context, householdID, accountID int) (Holdings, error) {
	account, err := s.GetAccount(ctx, householdID, accountID)
	if err != nil {
		return Holdings{}, err
	}
	h := Holdings{AccountID: accountID, Cash: account.CashBalance, Positions: []HoldingPosition{}}

	trades, err := loadTrades(ctx, s.db, accountID)
	if err != nil {
		return h, err
	}
	positions, err := portfolio.Replay(trades)
	if err != nil {
		return h, err
	}

	symbols := make([]string, 0, len(positions))
	for _, p := range positions {
		symbols = append(symbols, p.Symbol)
	}
	securities, err := s.securitiesBySymbol(ctx, symbols)
	if err != nil {
		return h, err
	}

	var previousValue float64
	var oldest *time.Time
	for _, p := range positions {
		h.RealizedGain += p.RealizedGain
		if p.Shares == 0 {
			continue
		}
		sec := securities[p.Symbol]
		hp := HoldingPosition{
			Symbol: p.Symbol, Name: sec.Name, QuoteType: sec.QuoteType,
			Shares: portfolio.RoundShares(p.Shares), AvgCost: p.AvgCost(), CostBasis: round2(p.CostBasis),
			Price: sec.Price, PreviousClose: sec.PreviousClose,
			MarketValue:  portfolio.MarketValue(p.Shares, sec.Price),
			RealizedGain: round2(p.RealizedGain),
			PriceTime:    sec.PriceTime, FetchError: sec.FetchError,
		}
		hp.UnrealizedGain = round2(hp.MarketValue - hp.CostBasis)
		if hp.CostBasis > 0 {
			hp.UnrealizedGainPct = ptr(hp.UnrealizedGain / hp.CostBasis)
		}
		if sec.PreviousClose != nil && *sec.PreviousClose > 0 {
			prevValue := portfolio.MarketValue(p.Shares, *sec.PreviousClose)
			hp.DayChange = ptr(round2(hp.MarketValue - prevValue))
			hp.DayChangePct = ptr(sec.Price / *sec.PreviousClose - 1)
			h.DayChange += *hp.DayChange
			previousValue += prevValue
		} else {
			// No previous close: count the position as unchanged so the account's
			// day change percentage isn't skewed by a missing figure.
			previousValue += hp.MarketValue
		}

		if sec.PriceTime == nil {
			h.Seeded = true
		} else if t, err := time.Parse(time.RFC3339, *sec.PriceTime); err == nil && (oldest == nil || t.Before(*oldest)) {
			oldest = &t
		}

		h.HoldingsValue += hp.MarketValue
		h.CostBasis += hp.CostBasis
		h.Positions = append(h.Positions, hp)
	}

	h.HoldingsValue = round2(h.HoldingsValue)
	h.CostBasis = round2(h.CostBasis)
	h.UnrealizedGain = round2(h.HoldingsValue - h.CostBasis)
	h.RealizedGain = round2(h.RealizedGain)
	h.DayChange = round2(h.DayChange)
	h.TotalValue = round2(h.Cash + h.HoldingsValue)
	if previousValue > 0 {
		h.DayChangePct = ptr(h.DayChange / previousValue)
	}
	if oldest != nil {
		s := oldest.UTC().Format(time.RFC3339)
		h.AsOf = &s
	}
	for i := range h.Positions {
		if h.HoldingsValue > 0 {
			h.Positions[i].Weight = h.Positions[i].MarketValue / h.HoldingsValue
		}
	}
	return h, nil
}

// round2 tidies a sum of already-rounded cent amounts, where float addition can
// leave 1234.5600000000001 behind.
func round2(v float64) float64 { return math.Round(v*100) / 100 }

func ptr[T any](v T) *T { return &v }
