package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/lib/pq"

	"github.com/cwnelson/fangorn/internal/models"
	"github.com/cwnelson/fangorn/internal/portfolio"
)

// maxHistoryDays caps how far back a value history reaches, and is what "all"
// means.
const maxHistoryDays = 3650

// closeLookback is how far before the first requested day closes are loaded, so
// the first days of a range have a price to carry forward across a weekend or a
// long holiday.
const closeLookback = 14

// ValueHistory returns the combined daily value (cash + holdings) of investment
// accounts over the last `days` days, ending today in the household's timezone.
// accountIDs must all be investment accounts in the household; nil means every
// non-archived one. days <= 0 means all history.
//
// Unlike net worth, this is not read from snapshots: it is rebuilt from the
// trade log and stored daily closes, so it covers the time before the app was
// tracking the account, and it is right the moment a back-dated trade is saved.
// It is only as good as the closes, which prices.Refresher.BackfillHistory keeps
// filled in.
func (s *Service) ValueHistory(ctx context.Context, householdID int, accountIDs []int, days int) ([]portfolio.Point, error) {
	household, err := s.household(ctx, householdID)
	if err != nil {
		return nil, err
	}
	if days <= 0 || days > maxHistoryDays {
		days = maxHistoryDays
	}
	today := household.Today()
	from := today.AddDate(0, 0, -days)

	ledgers, err := s.investmentLedgers(ctx, householdID, accountIDs)
	if err != nil {
		return nil, err
	}
	if len(ledgers) == 0 {
		return []portfolio.Point{}, nil
	}

	symbolSet := map[string]bool{}
	for _, l := range ledgers {
		for _, t := range l.Trades {
			symbolSet[t.Symbol] = true
		}
	}
	symbols := make([]string, 0, len(symbolSet))
	for sym := range symbolSet {
		symbols = append(symbols, sym)
	}

	closes, err := s.closes(ctx, symbols, from.AddDate(0, 0, -closeLookback), today)
	if err != nil {
		return nil, err
	}
	securities, err := s.securitiesBySymbol(ctx, symbols)
	if err != nil {
		return nil, err
	}
	latest := make(map[string]float64, len(securities))
	for sym, sec := range securities {
		latest[sym] = sec.Price
	}

	series := make([][]portfolio.Point, 0, len(ledgers))
	for _, l := range ledgers {
		series = append(series, portfolio.ValueSeries(l, closes, latest, from, today))
	}
	if len(series) == 1 {
		return series[0], nil
	}
	return portfolio.SumSeries(series...), nil
}

// investmentLedgers loads the cash and trade history of investment accounts.
func (s *Service) investmentLedgers(ctx context.Context, householdID int, accountIDs []int) ([]portfolio.Ledger, error) {
	q := `SELECT id, type, starting_balance, starting_balance_date FROM accounts WHERE household_id = $1`
	args := []any{householdID}
	if accountIDs == nil {
		q += ` AND type IN ('` + models.AccountInvestment + `','` + models.AccountRetirement + `') AND archived_at IS NULL`
	} else {
		q += ` AND id = ANY($2)`
		args = append(args, pq.Array(accountIDs))
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY id`, args...)
	if err != nil {
		return nil, fmt.Errorf("loading investment accounts: %w", err)
	}
	defer rows.Close()

	byID := map[int]*portfolio.Ledger{}
	var ids []int
	for rows.Next() {
		var id int
		var typ string
		var l portfolio.Ledger
		if err := rows.Scan(&id, &typ, &l.StartingCash, &l.StartingDate); err != nil {
			return nil, fmt.Errorf("scanning account: %w", err)
		}
		if !models.HoldsSecurities(typ) {
			return nil, invalid("value history is only kept for investment and retirement accounts")
		}
		byID[id] = &l
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if accountIDs != nil && len(ids) != len(uniqueInts(accountIDs)) {
		return nil, ErrNotFound
	}
	if len(ids) == 0 {
		return nil, nil
	}

	cashRows, err := s.db.QueryContext(ctx,
		`SELECT account_id, date, SUM(amount) FROM transactions
		 WHERE household_id = $1 AND account_id = ANY($2)
		 GROUP BY account_id, date`, householdID, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("loading daily cash: %w", err)
	}
	defer cashRows.Close()
	for cashRows.Next() {
		var id int
		var c portfolio.DailyCash
		if err := cashRows.Scan(&id, &c.Date, &c.Amount); err != nil {
			return nil, fmt.Errorf("scanning daily cash: %w", err)
		}
		byID[id].Cash = append(byID[id].Cash, c)
	}
	if err := cashRows.Err(); err != nil {
		return nil, err
	}

	out := make([]portfolio.Ledger, 0, len(ids))
	for _, id := range ids {
		l := byID[id]
		if l.Trades, err = loadTrades(ctx, s.db, id); err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, nil
}

// closes loads stored daily closes for the symbols between two dates.
func (s *Service) closes(ctx context.Context, symbols []string, from, to time.Time) (map[string][]portfolio.Close, error) {
	out := map[string][]portfolio.Close{}
	if len(symbols) == 0 {
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT symbol, price_date, close FROM security_prices
		 WHERE symbol = ANY($1) AND price_date BETWEEN $2 AND $3
		 ORDER BY symbol, price_date`,
		pq.Array(symbols), from.Format(models.DateOnly), to.Format(models.DateOnly))
	if err != nil {
		return nil, fmt.Errorf("loading closes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var sym string
		var c portfolio.Close
		if err := rows.Scan(&sym, &c.Date, &c.Price); err != nil {
			return nil, fmt.Errorf("scanning close: %w", err)
		}
		out[sym] = append(out[sym], c)
	}
	return out, rows.Err()
}

func uniqueInts(in []int) map[int]bool {
	out := make(map[int]bool, len(in))
	for _, v := range in {
		out[v] = true
	}
	return out
}
