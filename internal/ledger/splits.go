package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/cwnelson/fangorn/internal/portfolio"
	"github.com/cwnelson/fangorn/internal/quotes"
)

// A stock split is found and confirmed like a dividend. The provider restates
// a symbol's whole price history in post-split terms once it splits, so
// confirming does the same to the account's trades from before the split date:
// shares × the ratio, price ÷ it, dollar amounts untouched. Holdings, average
// cost and the value chart then agree with the prices again. Until it is
// confirmed the position is valued at the old share count and the new price.

// Split is one split waiting on an account.
type Split struct {
	ID          int     `json:"id"`
	AccountID   int     `json:"account_id"`
	AccountName string  `json:"account_name"`
	Symbol      string  `json:"symbol"`
	Name        *string `json:"name"`
	SplitDate   string  `json:"split_date"`
	Numerator   float64 `json:"numerator"`
	Denominator float64 `json:"denominator"`
	// Shares is what the trades said was held going into the split, and
	// SharesAfter what confirming turns that into.
	Shares      float64 `json:"shares"`
	SharesAfter float64 `json:"shares_after"`
	Status      string  `json:"status"`
}

// SaveSplits records a symbol's splits. A split not seen before also marks the
// symbol's price history to be fetched again: the closes stored before it are
// in pre-split terms, and the provider's are now adjusted.
func (s *Service) SaveSplits(ctx context.Context, symbol string, splits []quotes.Split) error {
	symbol = NormalizeSymbol(symbol)
	return s.inTx(func(tx *sql.Tx) error {
		fresh := false
		for _, sp := range splits {
			if sp.Numerator <= 0 || sp.Denominator <= 0 || sp.Numerator == sp.Denominator {
				continue
			}
			res, err := tx.ExecContext(ctx,
				`INSERT INTO security_splits (symbol, split_date, numerator, denominator)
				 VALUES ($1,$2,$3,$4) ON CONFLICT (symbol, split_date) DO NOTHING`,
				symbol, dateStr(sp.Date), sp.Numerator, sp.Denominator)
			if err != nil {
				return fmt.Errorf("saving %s split: %w", symbol, err)
			}
			if n, _ := res.RowsAffected(); n > 0 {
				fresh = true
			}
		}
		if !fresh {
			return nil
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE securities SET history_from = NULL WHERE symbol = $1`, symbol); err != nil {
			return fmt.Errorf("marking %s history stale: %w", symbol, err)
		}
		return nil
	})
}

// FindSplits raises a pending split for every recent split date an open
// account held shares going into. Idempotent like FindDividends, and over the
// same lookback. It returns how many it raised.
func (s *Service) FindSplits(ctx context.Context, householdID int, today time.Time) (int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT t.account_id, sp.symbol, sp.split_date, sp.numerator, sp.denominator
		 FROM security_splits sp
		 JOIN trades t ON t.symbol = sp.symbol AND t.trade_date < sp.split_date
		 JOIN accounts a ON a.id = t.account_id
		 WHERE t.household_id = $1 AND a.archived_at IS NULL
		   AND sp.split_date BETWEEN $2 AND $3
		   AND NOT EXISTS (SELECT 1 FROM splits x
		                   WHERE x.account_id = t.account_id AND x.symbol = sp.symbol
		                     AND x.split_date = sp.split_date)
		 ORDER BY sp.split_date, sp.symbol, t.account_id`,
		householdID, dateStr(today.AddDate(0, 0, -dividendLookback)), dateStr(today))
	if err != nil {
		return 0, fmt.Errorf("listing splits: %w", err)
	}
	type found struct {
		accountID int
		symbol    string
		date      time.Time
		num, den  float64
	}
	var all []found
	for rows.Next() {
		var f found
		if err := rows.Scan(&f.accountID, &f.symbol, &f.date, &f.num, &f.den); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scanning split: %w", err)
		}
		all = append(all, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	raised := 0
	logs := map[int][]portfolio.Trade{}
	for _, f := range all {
		trades, ok := logs[f.accountID]
		if !ok {
			if trades, err = loadTrades(ctx, s.db, f.accountID); err != nil {
				return raised, err
			}
			logs[f.accountID] = trades
		}
		shares := portfolio.RoundShares(portfolio.SharesAsOf(trades, f.date.AddDate(0, 0, -1))[f.symbol])
		if shares <= 0 {
			continue
		}
		res, err := s.db.ExecContext(ctx,
			`INSERT INTO splits (household_id, account_id, symbol, split_date, numerator, denominator, shares)
			 VALUES ($1,$2,$3,$4,$5,$6,$7)
			 ON CONFLICT (account_id, symbol, split_date) DO NOTHING`,
			householdID, f.accountID, f.symbol, dateStr(f.date), f.num, f.den, shares)
		if err != nil {
			return raised, fmt.Errorf("raising %s split: %w", f.symbol, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			raised++
		}
	}
	return raised, nil
}

// ListSplits returns the household's pending splits, oldest first — one
// account's when accountID is given.
func (s *Service) ListSplits(ctx context.Context, householdID int, accountID *int) ([]Split, error) {
	q := `SELECT sp.id, sp.account_id, a.name, sp.symbol, s.name, sp.split_date,
	             sp.numerator, sp.denominator, sp.shares, sp.status
	      FROM splits sp
	      JOIN accounts a ON a.id = sp.account_id
	      JOIN securities s ON s.symbol = sp.symbol
	      WHERE sp.household_id = $1 AND sp.status = 'pending' AND a.archived_at IS NULL`
	args := []any{householdID}
	if accountID != nil {
		q += ` AND sp.account_id = $2`
		args = append(args, *accountID)
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY sp.split_date, sp.symbol, sp.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("listing splits: %w", err)
	}
	defer rows.Close()
	out := []Split{}
	for rows.Next() {
		var (
			sp   Split
			name sql.NullString
			date time.Time
		)
		if err := rows.Scan(&sp.ID, &sp.AccountID, &sp.AccountName, &sp.Symbol, &name, &date,
			&sp.Numerator, &sp.Denominator, &sp.Shares, &sp.Status); err != nil {
			return nil, fmt.Errorf("scanning split: %w", err)
		}
		sp.Name, sp.SplitDate = strPtr(name), dateStr(date)
		sp.SharesAfter = portfolio.RoundShares(sp.Shares * sp.Numerator / sp.Denominator)
		out = append(out, sp)
	}
	return out, rows.Err()
}

// ConfirmSplit restates the account's trades in the symbol from before the
// split date in post-split terms, in the same database transaction that marks
// the split confirmed — so a second tap can't apply the ratio twice. The cash
// legs are left alone: the dollars that moved didn't change.
func (s *Service) ConfirmSplit(ctx context.Context, householdID, id int) error {
	return s.inTx(func(tx *sql.Tx) error {
		var (
			accountID int
			symbol    string
			date      time.Time
			num, den  float64
		)
		err := tx.QueryRowContext(ctx,
			`UPDATE splits SET status = 'confirmed', resolved_at = NOW()
			 WHERE id = $1 AND household_id = $2 AND status = 'pending'
			 RETURNING account_id, symbol, split_date, numerator, denominator`, id, householdID).
			Scan(&accountID, &symbol, &date, &num, &den)
		if err == sql.ErrNoRows {
			return s.splitGone(ctx, tx, householdID, id)
		}
		if err != nil {
			return fmt.Errorf("claiming split: %w", err)
		}
		if err := lockInvestmentAccount(ctx, tx, householdID, accountID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE trades SET
			   shares = ROUND(shares * $4::numeric / $5::numeric, 8),
			   price  = ROUND(price * $5::numeric / $4::numeric, 6),
			   updated_at = NOW()
			 WHERE account_id = $1 AND symbol = $2 AND trade_date < $3`,
			accountID, symbol, dateStr(date), num, den); err != nil {
			return fmt.Errorf("restating trades: %w", err)
		}
		// A reverse split can round a position down past a later sell.
		trades, err := loadTrades(ctx, tx, accountID)
		if err != nil {
			return err
		}
		return validateLog(trades)
	})
}

// DismissSplit drops a pending split without touching the trades — they were
// already entered in post-split shares. It isn't raised again.
func (s *Service) DismissSplit(ctx context.Context, householdID, id int) error {
	return s.inTx(func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE splits SET status = 'dismissed', resolved_at = NOW()
			 WHERE id = $1 AND household_id = $2 AND status = 'pending'`, id, householdID)
		if err != nil {
			return fmt.Errorf("dismissing split: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return s.splitGone(ctx, tx, householdID, id)
		}
		return nil
	})
}

func (s *Service) splitGone(ctx context.Context, tx *sql.Tx, householdID, id int) error {
	var status string
	err := tx.QueryRowContext(ctx,
		`SELECT status FROM splits WHERE id = $1 AND household_id = $2`, id, householdID).Scan(&status)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("loading split: %w", err)
	}
	return invalid("that split was already %s", status)
}
