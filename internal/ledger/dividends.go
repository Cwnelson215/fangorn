package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/lib/pq"

	"github.com/cwnelson/fangorn/internal/models"
	"github.com/cwnelson/fangorn/internal/portfolio"
	"github.com/cwnelson/fangorn/internal/quotes"
)

// Dividends from held stocks and funds are found, not posted. The provider
// publishes an ex-date and an amount per share, which says one is owed (shares
// held going into the ex-date × the amount) but not the day the cash lands or
// how many shares a reinvestment bought. So each waits as pending until someone
// confirms it — as cash, or as a reinvestment with the shares the statement
// shows — or dismisses it.

const (
	DividendPending   = "pending"
	DividendConfirmed = "confirmed"
	DividendDismissed = "dismissed"

	// dividendLookback is how far back an ex-date can be and still be raised.
	// Long enough to cover the gap between ex-date and pay date and a server
	// that was down for a while; short enough that turning this on doesn't ask
	// about years of dividends already logged by hand.
	dividendLookback = 60
)

// Dividend is one dividend waiting on an account.
type Dividend struct {
	ID          int     `json:"id"`
	AccountID   int     `json:"account_id"`
	AccountName string  `json:"account_name"`
	Symbol      string  `json:"symbol"`
	Name        *string `json:"name"`
	QuoteType   *string `json:"quote_type"`
	ExDate      string  `json:"ex_date"`
	// PayDate is the day it is paid, when the provider publishes one. Confirming
	// waits for it, and dates the dividend then.
	PayDate  *string `json:"pay_date"`
	PerShare float64 `json:"per_share"`
	Shares   float64 `json:"shares"`
	// Amount is the estimate: shares × per-share, to the cent.
	Amount float64 `json:"amount"`
	// Price is the symbol's latest price, to suggest how many shares a
	// reinvestment bought.
	Price  float64 `json:"price"`
	Status string  `json:"status"`
}

// DividendSymbolsDue lists the symbols the household's open accounts have ever
// traded whose dividends haven't been looked up since staleBefore. Money market
// funds are left out: theirs is the monthly cash dividend, posted from the yield.
func (s *Service) DividendSymbolsDue(ctx context.Context, householdID int, staleBefore time.Time) ([]string, error) {
	return s.heldSymbols(ctx,
		`SELECT DISTINCT t.symbol FROM trades t
		 JOIN accounts a ON a.id = t.account_id
		 JOIN securities s ON s.symbol = t.symbol
		 WHERE t.household_id = $1 AND a.archived_at IS NULL
		   AND COALESCE(s.quote_type, '') <> 'MONEYMARKET'
		   AND (s.dividends_checked_at IS NULL OR s.dividends_checked_at < $2)
		 ORDER BY t.symbol`, householdID, staleBefore)
}

// SaveDividends records a symbol's declared dividends and that it was checked.
func (s *Service) SaveDividends(ctx context.Context, symbol string, events []quotes.Dividend, checkedAt time.Time) error {
	symbol = NormalizeSymbol(symbol)
	return s.inTx(func(tx *sql.Tx) error {
		for _, e := range events {
			if e.Amount <= 0 {
				continue
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO security_dividends (symbol, ex_date, amount) VALUES ($1,$2,$3)
				 ON CONFLICT (symbol, ex_date) DO UPDATE SET amount = EXCLUDED.amount`,
				symbol, dateStr(e.ExDate), e.Amount); err != nil {
				return fmt.Errorf("saving %s dividend: %w", symbol, err)
			}
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE securities SET dividends_checked_at = $2 WHERE symbol = $1`, symbol, checkedAt); err != nil {
			return fmt.Errorf("marking %s dividends checked: %w", symbol, err)
		}
		return nil
	})
}

// SaveDividendPayDate records when a declared dividend is paid. A dividend the
// ledger doesn't have (the next one, not yet gone ex) is ignored.
func (s *Service) SaveDividendPayDate(ctx context.Context, symbol string, exDate, payDate time.Time) error {
	if payDate.Before(exDate) {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE security_dividends SET pay_date = $3 WHERE symbol = $1 AND ex_date = $2`,
		NormalizeSymbol(symbol), dateStr(exDate), dateStr(payDate))
	if err != nil {
		return fmt.Errorf("saving %s pay date: %w", symbol, err)
	}
	return nil
}

// PayDatesWanted lists the given symbols that have a dividend with an ex-date
// since `since` whose pay date isn't known yet.
func (s *Service) PayDatesWanted(ctx context.Context, symbols []string, since time.Time) ([]string, error) {
	return s.heldSymbols(ctx,
		`SELECT DISTINCT symbol FROM security_dividends
		 WHERE symbol = ANY($1) AND ex_date >= $2 AND pay_date IS NULL ORDER BY symbol`,
		pq.Array(symbols), dateStr(since))
}

// FindDividends raises a pending dividend for every recent ex-date an open
// account held shares going into. It is idempotent — one row per account,
// symbol and ex-date — and reads only what is stored, so it runs whether or not
// the provider answered. A dividend that looks already logged (income of the
// same amount on the account since the ex-date, or a reinvestment of the symbol)
// is recorded as dismissed rather than asked about. It returns how many it raised.
func (s *Service) FindDividends(ctx context.Context, householdID int, today time.Time) (int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, COALESCE(cash_fund, '') FROM accounts
		 WHERE household_id = $1 AND archived_at IS NULL AND EXISTS (SELECT 1 FROM trades t WHERE t.account_id = accounts.id)`,
		householdID)
	if err != nil {
		return 0, fmt.Errorf("listing accounts with trades: %w", err)
	}
	type account struct {
		id   int
		fund string
	}
	var accounts []account
	for rows.Next() {
		var a account
		if err := rows.Scan(&a.id, &a.fund); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scanning account: %w", err)
		}
		accounts = append(accounts, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	from := today.AddDate(0, 0, -dividendLookback)
	raised := 0
	for _, a := range accounts {
		n, err := s.findAccountDividends(ctx, householdID, a.id, a.fund, from, today)
		raised += n
		if err != nil {
			return raised, fmt.Errorf("account %d: %w", a.id, err)
		}
	}
	return raised, nil
}

func (s *Service) findAccountDividends(ctx context.Context, householdID, accountID int, cashFund string, from, today time.Time) (int, error) {
	trades, err := loadTrades(ctx, s.db, accountID)
	if err != nil {
		return 0, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT d.symbol, d.ex_date, d.amount FROM security_dividends d
		 WHERE d.ex_date BETWEEN $2 AND $3
		   AND d.symbol IN (SELECT symbol FROM trades WHERE account_id = $1)
		   AND NOT EXISTS (SELECT 1 FROM dividends x
		                   WHERE x.account_id = $1 AND x.symbol = d.symbol AND x.ex_date = d.ex_date)
		 ORDER BY d.ex_date, d.symbol`,
		accountID, dateStr(from), dateStr(today))
	if err != nil {
		return 0, fmt.Errorf("listing dividends: %w", err)
	}
	type event struct {
		symbol   string
		ex       time.Time
		perShare float64
	}
	var events []event
	for rows.Next() {
		var e event
		if err := rows.Scan(&e.symbol, &e.ex, &e.perShare); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scanning dividend: %w", err)
		}
		events = append(events, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	raised := 0
	for _, e := range events {
		if e.symbol == cashFund {
			continue
		}
		// Shares bought on the ex-date itself don't get the dividend.
		shares := portfolio.RoundShares(portfolio.SharesAsOf(trades, e.ex.AddDate(0, 0, -1))[e.symbol])
		if shares <= 0 {
			continue
		}
		amount := portfolio.CashAmount(portfolio.SideReinvest, shares, e.perShare, 0)
		if amount < 0.01 {
			continue
		}

		var logged bool
		err := s.db.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM transactions
			                WHERE account_id = $1 AND kind = $2 AND amount = $3 AND date >= $4
			                  AND source <> $5)
			     OR EXISTS (SELECT 1 FROM trades
			                WHERE account_id = $1 AND symbol = $6 AND side = $7 AND trade_date >= $4)`,
			accountID, models.KindIncome, amount, dateStr(e.ex), models.SourceInterest,
			e.symbol, portfolio.SideReinvest).Scan(&logged)
		if err != nil {
			return raised, fmt.Errorf("checking whether %s was logged: %w", e.symbol, err)
		}
		status := DividendPending
		var resolved any
		if logged {
			status, resolved = DividendDismissed, time.Now()
		}
		res, err := s.db.ExecContext(ctx,
			`INSERT INTO dividends
			   (household_id, account_id, symbol, ex_date, per_share, shares, amount, status, resolved_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			 ON CONFLICT (account_id, symbol, ex_date) DO NOTHING`,
			householdID, accountID, e.symbol, dateStr(e.ex), e.perShare, shares, amount, status, resolved)
		if err != nil {
			return raised, fmt.Errorf("raising %s dividend: %w", e.symbol, err)
		}
		if n, _ := res.RowsAffected(); n > 0 && !logged {
			raised++
		}
	}
	return raised, nil
}

// ListDividends returns the household's pending dividends, oldest ex-date
// first — one account's when accountID is given.
func (s *Service) ListDividends(ctx context.Context, householdID int, accountID *int) ([]Dividend, error) {
	q := `SELECT d.id, d.account_id, a.name, d.symbol, s.name, s.quote_type, d.ex_date, sd.pay_date,
	             d.per_share, d.shares, d.amount, s.last_price, d.status
	      FROM dividends d
	      JOIN accounts a ON a.id = d.account_id
	      JOIN securities s ON s.symbol = d.symbol
	      LEFT JOIN security_dividends sd ON sd.symbol = d.symbol AND sd.ex_date = d.ex_date
	      WHERE d.household_id = $1 AND d.status = 'pending' AND a.archived_at IS NULL`
	args := []any{householdID}
	if accountID != nil {
		q += ` AND d.account_id = $2`
		args = append(args, *accountID)
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY d.ex_date, d.symbol, d.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("listing dividends: %w", err)
	}
	defer rows.Close()
	out := []Dividend{}
	for rows.Next() {
		var (
			d        Dividend
			name, qt sql.NullString
			ex       time.Time
			pay      sql.NullTime
		)
		if err := rows.Scan(&d.ID, &d.AccountID, &d.AccountName, &d.Symbol, &name, &qt, &ex, &pay,
			&d.PerShare, &d.Shares, &d.Amount, &d.Price, &d.Status); err != nil {
			return nil, fmt.Errorf("scanning dividend: %w", err)
		}
		d.Name, d.QuoteType, d.ExDate = strPtr(name), strPtr(qt), dateStr(ex)
		if pay.Valid {
			d.PayDate = ptr(dateStr(pay.Time))
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DividendConfirmation is what confirming one may say. Left empty it is the one
// tap: the estimated amount, as cash, dated its pay date — or today when the
// provider didn't publish one.
type DividendConfirmation struct {
	// Amount is what was actually paid, when the statement differs by a cent
	// or tax was withheld.
	Amount *float64 `json:"amount"`
	// Date is the day it landed, when that isn't the published pay date.
	Date string `json:"date"`
	// Reinvested also buys shares with it. Shares is then how many, from the
	// statement.
	Reinvested bool     `json:"reinvested"`
	Shares     *float64 `json:"shares"`
}

// ConfirmDividend posts a pending dividend as an income transaction under
// Dividends. A reinvested one also buys shares with it the same day, so it is
// counted as income like any other dividend and the cash nets to nothing —
// unlike a hand-logged `reinvest` trade, which only adds shares. The posts and
// the status flip share one database transaction, guarded by the status, so two
// taps (or two phones) can't post it twice.
func (s *Service) ConfirmDividend(ctx context.Context, householdID, id int, in DividendConfirmation) error {
	household, err := s.GetHousehold(ctx, householdID)
	if err != nil {
		return err
	}
	today := household.Today()
	var date time.Time
	if in.Date != "" {
		if date, err = models.ParseDate(in.Date); err != nil {
			return invalid("date must be YYYY-MM-DD")
		}
		if date.After(today) {
			return invalid("a dividend can't be confirmed for a day that hasn't come yet")
		}
	}
	if in.Amount != nil {
		amt := math.Round(*in.Amount*100) / 100
		if amt < 0.01 || amt >= maxMagnitude {
			return invalid("amount must be greater than zero")
		}
		in.Amount = &amt
	}
	if in.Reinvested && (in.Shares == nil || portfolio.RoundShares(*in.Shares) <= 0) {
		return invalid("enter how many shares the dividend bought")
	}
	if !in.Reinvested && in.Shares != nil {
		return invalid("shares only apply to a reinvested dividend")
	}

	return s.inTx(func(tx *sql.Tx) error {
		var (
			accountID int
			symbol    string
			estimate  float64
			ex        time.Time
			pay       sql.NullTime
		)
		err := tx.QueryRowContext(ctx,
			`UPDATE dividends SET status = 'confirmed', resolved_at = NOW()
			 WHERE id = $1 AND household_id = $2 AND status = 'pending'
			 RETURNING account_id, symbol, amount, ex_date,
			   (SELECT pay_date FROM security_dividends sd
			    WHERE sd.symbol = dividends.symbol AND sd.ex_date = dividends.ex_date)`, id, householdID).
			Scan(&accountID, &symbol, &estimate, &ex, &pay)
		if err == sql.ErrNoRows {
			return s.dividendGone(ctx, tx, householdID, id)
		}
		if err != nil {
			return fmt.Errorf("claiming dividend: %w", err)
		}
		if in.Date == "" {
			// The published pay date, once it has come; today without one.
			date = today
			if pay.Valid {
				if pay.Time.After(today) {
					return invalid("%s doesn't pay this dividend until %s", symbol, dateStr(pay.Time))
				}
				date = pay.Time
			}
			in.Date = dateStr(date)
		}
		if date.Before(ex) {
			return invalid("%s went ex-dividend on %s; it can't have been paid before that", symbol, dateStr(ex))
		}
		amount := estimate
		if in.Amount != nil {
			amount = *in.Amount
		}

		categoryID, err := incomeCategory(ctx, tx, householdID, "Dividends")
		if err != nil {
			return err
		}
		var txnID int
		err = tx.QueryRowContext(ctx,
			`INSERT INTO transactions
			   (household_id, account_id, date, amount, kind, description, category_id, source)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			 RETURNING id`,
			householdID, accountID, in.Date, amount, models.KindIncome, symbol+" dividend",
			categoryID, models.SourceDividend).Scan(&txnID)
		if err != nil {
			return fmt.Errorf("posting dividend: %w", err)
		}
		var tradeID sql.NullInt64
		if in.Reinvested {
			shares := portfolio.RoundShares(*in.Shares)
			note := "Reinvested dividend"
			trade := TradeInput{
				Symbol: symbol, Side: portfolio.SideBuy, TradeDate: in.Date,
				Shares: shares, Price: math.Round(amount/shares*1e6) / 1e6, Amount: &amount, Notes: &note,
			}
			if err := trade.normalize(); err != nil {
				return err
			}
			bought, err := insertTrade(ctx, tx, householdID, accountID, trade)
			if err != nil {
				return err
			}
			tradeID = sql.NullInt64{Int64: int64(bought), Valid: true}
		}
		_, err = tx.ExecContext(ctx,
			`UPDATE dividends SET transaction_id = $1, trade_id = $2 WHERE id = $3`, txnID, tradeID, id)
		return err
	})
}

// DismissDividend drops a pending dividend without posting anything — it was
// logged by hand, or never paid. It isn't raised again.
func (s *Service) DismissDividend(ctx context.Context, householdID, id int) error {
	return s.inTx(func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE dividends SET status = 'dismissed', resolved_at = NOW()
			 WHERE id = $1 AND household_id = $2 AND status = 'pending'`, id, householdID)
		if err != nil {
			return fmt.Errorf("dismissing dividend: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return s.dividendGone(ctx, tx, householdID, id)
		}
		return nil
	})
}

// dividendGone explains why a pending dividend couldn't be claimed.
func (s *Service) dividendGone(ctx context.Context, tx *sql.Tx, householdID, id int) error {
	var status string
	err := tx.QueryRowContext(ctx,
		`SELECT status FROM dividends WHERE id = $1 AND household_id = $2`, id, householdID).Scan(&status)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("loading dividend: %w", err)
	}
	return invalid("that dividend was already %s", status)
}
