package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cwnelson/fangorn/internal/models"
	"github.com/cwnelson/fangorn/internal/portfolio"
)

// A trade is a trades row plus, for a buy or sell, one kind = 'trade' transaction
// on the same account carrying the cash side. That is the investment counterpart
// of a transfer's two legs: cash stays starting_balance + SUM(amount) with no
// special-casing, buys and sells show up in the account's register for free, and
// the only extra work is keeping the pair in step — so every mutation below runs
// in one transaction over both rows.
//
// Share counts are never stored. Every write replays the account's whole trade
// log through portfolio.Replay before committing, so a sell can never exceed the
// shares held on its date, including after an earlier buy is edited or deleted.

const tradeSelect = `
	SELECT t.id, t.account_id, t.symbol, s.name, t.side, t.trade_date, t.shares,
	       t.price, t.fees, t.amount, t.notes, t.created_at
	FROM trades t
	JOIN securities s ON s.symbol = t.symbol`

func scanTrade(rows interface{ Scan(...any) error }) (models.Trade, error) {
	var tr models.Trade
	var name, notes sql.NullString
	var date, createdAt time.Time
	err := rows.Scan(&tr.ID, &tr.AccountID, &tr.Symbol, &name, &tr.Side, &date,
		&tr.Shares, &tr.Price, &tr.Fees, &tr.Amount, &notes, &createdAt)
	if err != nil {
		return tr, err
	}
	tr.TradeDate = dateStr(date)
	tr.CreatedAt = createdAt.Format(time.RFC3339)
	tr.SecurityName = strPtr(name)
	tr.Notes = strPtr(notes)
	return tr, nil
}

// ListTrades returns an account's trade log, newest first.
func (s *Service) ListTrades(ctx context.Context, householdID, accountID int) ([]models.Trade, error) {
	if _, err := s.GetAccount(ctx, householdID, accountID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		tradeSelect+` WHERE t.household_id = $1 AND t.account_id = $2
		 ORDER BY t.trade_date DESC, t.id DESC`,
		householdID, accountID)
	if err != nil {
		return nil, fmt.Errorf("listing trades: %w", err)
	}
	defer rows.Close()

	out := []models.Trade{}
	for rows.Next() {
		tr, err := scanTrade(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning trade: %w", err)
		}
		out = append(out, tr)
	}
	return out, rows.Err()
}

func (s *Service) GetTrade(ctx context.Context, householdID, id int) (models.Trade, error) {
	tr, err := scanTrade(s.db.QueryRowContext(ctx,
		tradeSelect+` WHERE t.household_id = $1 AND t.id = $2`, householdID, id))
	if err == sql.ErrNoRows {
		return tr, ErrNotFound
	}
	if err != nil {
		return tr, fmt.Errorf("fetching trade: %w", err)
	}
	return tr, nil
}

// TradeInput is what the trade form submits. There is deliberately no account
// field: a trade is created under an account and cannot be moved to another.
//
// Amount is optional. Left out, it defaults to shares × price ± fees; given, it
// is the real dollar figure from the brokerage confirmation.
type TradeInput struct {
	Symbol    string   `json:"symbol"`
	Side      string   `json:"side"`
	TradeDate string   `json:"trade_date"`
	Shares    float64  `json:"shares"`
	Price     float64  `json:"price"`
	Fees      float64  `json:"fees"`
	Amount    *float64 `json:"amount"`
	Notes     *string  `json:"notes"`
}

// symbolPattern admits what tickers actually look like: BRK.B, BTC-USD, and
// plain letters and digits. It keeps free text out of the securities table.
var symbolPattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9.\-]{0,19}$`)

// maxMagnitude keeps values inside the NUMERIC columns, so an absurd entry is a
// readable 400 rather than a numeric-overflow 500.
const maxMagnitude = 1e11

// NormalizeSymbol upper-cases and trims a ticker the way it is stored.
func NormalizeSymbol(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

func (in *TradeInput) normalize() error {
	in.Symbol = NormalizeSymbol(in.Symbol)
	if in.Symbol == "" {
		return invalid("symbol is required")
	}
	if !symbolPattern.MatchString(in.Symbol) {
		return invalid("%q doesn't look like a ticker symbol", in.Symbol)
	}
	if !portfolio.ValidSide(in.Side) {
		return invalid("side must be buy, sell, reinvest or opening")
	}
	if _, err := models.ParseDate(in.TradeDate); err != nil {
		return invalid("trade_date must be YYYY-MM-DD")
	}
	in.Shares = portfolio.RoundShares(in.Shares)
	if in.Shares <= 0 {
		return invalid("shares must be greater than zero")
	}
	if in.Price < 0 || in.Fees < 0 {
		return invalid("price and fees can't be negative")
	}
	if in.Shares >= maxMagnitude || in.Price >= maxMagnitude || in.Fees >= maxMagnitude {
		return invalid("that number is too large")
	}
	in.Fees = math.Round(in.Fees*100) / 100

	if in.Amount == nil {
		amt := portfolio.CashAmount(in.Side, in.Shares, in.Price, in.Fees)
		in.Amount = &amt
	} else {
		amt := math.Round(*in.Amount*100) / 100
		in.Amount = &amt
	}
	if *in.Amount < 0 || *in.Amount >= maxMagnitude {
		return invalid("amount must be zero or more")
	}
	if portfolio.MovesCash(in.Side) && *in.Amount < 0.01 {
		return invalid("a %s has to move some cash; check the shares, price and fees", in.Side)
	}
	return nil
}

func (in *TradeInput) candidate(seq int) portfolio.Trade {
	date, _ := models.ParseDate(in.TradeDate)
	return portfolio.Trade{
		Seq: seq, Symbol: in.Symbol, Side: in.Side, Date: date,
		Shares: in.Shares, Price: in.Price, Amount: *in.Amount,
	}
}

func (s *Service) CreateTrade(ctx context.Context, householdID, accountID int, in TradeInput) (models.Trade, error) {
	if err := in.normalize(); err != nil {
		return models.Trade{}, err
	}

	var id int
	err := s.inTx(func(tx *sql.Tx) error {
		if err := lockInvestmentAccount(ctx, tx, householdID, accountID); err != nil {
			return err
		}
		if err := seedSecurity(ctx, tx, in.Symbol, in.Price); err != nil {
			return err
		}

		trades, err := loadTrades(ctx, tx, accountID)
		if err != nil {
			return err
		}
		if err := validateLog(append(trades, in.candidate(math.MaxInt))); err != nil {
			return err
		}

		err = tx.QueryRowContext(ctx,
			`INSERT INTO trades
			   (household_id, account_id, symbol, side, trade_date, shares, price, fees, amount, notes)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			 RETURNING id`,
			householdID, accountID, in.Symbol, in.Side, in.TradeDate, in.Shares, in.Price,
			in.Fees, *in.Amount, nullStr(in.Notes),
		).Scan(&id)
		if err != nil {
			return fmt.Errorf("inserting trade: %w", err)
		}
		return insertTradeLeg(ctx, tx, householdID, accountID, id, in)
	})
	if err != nil {
		return models.Trade{}, err
	}
	return s.GetTrade(ctx, householdID, id)
}

func (s *Service) UpdateTrade(ctx context.Context, householdID, id int, in TradeInput) (models.Trade, error) {
	if err := in.normalize(); err != nil {
		return models.Trade{}, err
	}

	err := s.inTx(func(tx *sql.Tx) error {
		accountID, err := tradeAccount(ctx, tx, householdID, id)
		if err != nil {
			return err
		}
		if err := lockInvestmentAccount(ctx, tx, householdID, accountID); err != nil {
			return err
		}
		if err := seedSecurity(ctx, tx, in.Symbol, in.Price); err != nil {
			return err
		}

		trades, err := loadTrades(ctx, tx, accountID)
		if err != nil {
			return err
		}
		for i := range trades {
			if trades[i].Seq == id {
				trades[i] = in.candidate(id)
			}
		}
		if err := validateLog(trades); err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx,
			`UPDATE trades SET
			   symbol = $1, side = $2, trade_date = $3, shares = $4, price = $5,
			   fees = $6, amount = $7, notes = $8, updated_at = NOW()
			 WHERE household_id = $9 AND id = $10`,
			in.Symbol, in.Side, in.TradeDate, in.Shares, in.Price, in.Fees, *in.Amount,
			nullStr(in.Notes), householdID, id,
		)
		if err != nil {
			return fmt.Errorf("updating trade: %w", err)
		}

		// The leg is rewritten rather than patched: a buy edited into a reinvest
		// loses its leg, and a reinvest edited into a sell gains one.
		if _, err := tx.ExecContext(ctx, `DELETE FROM transactions WHERE trade_id = $1`, id); err != nil {
			return fmt.Errorf("removing trade leg: %w", err)
		}
		return insertTradeLeg(ctx, tx, householdID, accountID, id, in)
	})
	if err != nil {
		return models.Trade{}, err
	}
	return s.GetTrade(ctx, householdID, id)
}

// DeleteTrade removes a trade and, by cascade, its cash leg. It is refused if a
// later sell depended on the shares it bought.
func (s *Service) DeleteTrade(ctx context.Context, householdID, id int) error {
	return s.inTx(func(tx *sql.Tx) error {
		accountID, err := tradeAccount(ctx, tx, householdID, id)
		if err != nil {
			return err
		}
		if err := lockInvestmentAccount(ctx, tx, householdID, accountID); err != nil {
			return err
		}

		trades, err := loadTrades(ctx, tx, accountID)
		if err != nil {
			return err
		}
		remaining := trades[:0]
		for _, t := range trades {
			if t.Seq != id {
				remaining = append(remaining, t)
			}
		}
		if err := validateLog(remaining); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx,
			`DELETE FROM trades WHERE household_id = $1 AND id = $2`, householdID, id); err != nil {
			return fmt.Errorf("deleting trade: %w", err)
		}
		return nil
	})
}

// HouseholdSymbols returns every symbol the household currently holds shares of,
// across all its accounts — what the scheduler keeps priced.
func (s *Service) HouseholdSymbols(ctx context.Context, householdID int) ([]string, error) {
	return s.heldSymbols(ctx,
		`SELECT symbol FROM trades WHERE household_id = $1
		 GROUP BY symbol
		 HAVING SUM(CASE WHEN side = 'sell' THEN -shares ELSE shares END) > 0
		 ORDER BY symbol`, householdID)
}

// AccountSymbols returns the symbols one account currently holds shares of.
func (s *Service) AccountSymbols(ctx context.Context, householdID, accountID int) ([]string, error) {
	return s.heldSymbols(ctx,
		`SELECT symbol FROM trades WHERE household_id = $1 AND account_id = $2
		 GROUP BY symbol
		 HAVING SUM(CASE WHEN side = 'sell' THEN -shares ELSE shares END) > 0
		 ORDER BY symbol`, householdID, accountID)
}

func (s *Service) heldSymbols(ctx context.Context, q string, args ...any) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listing held symbols: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var sym string
		if err := rows.Scan(&sym); err != nil {
			return nil, err
		}
		out = append(out, sym)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// helpers shared by the mutations
// ---------------------------------------------------------------------------

// lockInvestmentAccount checks the account belongs to the household and is an
// investment account, and takes a row lock on it for the rest of the transaction.
//
// The lock is what makes the oversell check sound. Two concurrent sells of the
// last 10 shares would each replay a log that doesn't contain the other; locking
// the account row serializes every trade write on that account, which locking
// trade rows cannot do (the new one does not exist yet).
func lockInvestmentAccount(ctx context.Context, tx *sql.Tx, householdID, accountID int) error {
	var typ string
	err := tx.QueryRowContext(ctx,
		`SELECT type FROM accounts WHERE id = $1 AND household_id = $2 FOR UPDATE`,
		accountID, householdID).Scan(&typ)
	if err == sql.ErrNoRows {
		return invalid("account %d does not exist", accountID)
	}
	if err != nil {
		return fmt.Errorf("locking account: %w", err)
	}
	if typ != models.AccountInvestment {
		return invalid("trades can only be logged on investment accounts")
	}
	return nil
}

func tradeAccount(ctx context.Context, tx *sql.Tx, householdID, id int) (int, error) {
	var accountID int
	err := tx.QueryRowContext(ctx,
		`SELECT account_id FROM trades WHERE household_id = $1 AND id = $2`,
		householdID, id).Scan(&accountID)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("fetching trade: %w", err)
	}
	return accountID, nil
}

// seedSecurity makes sure the symbol has a securities row, using the trade's own
// price as a stand-in until a real quote arrives. Handlers try to fetch a quote
// before this runs; seeding is what keeps logging a trade possible when the price
// provider is down.
func seedSecurity(ctx context.Context, tx *sql.Tx, symbol string, price float64) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO securities (symbol, last_price) VALUES ($1, $2)
		 ON CONFLICT (symbol) DO NOTHING`, symbol, price)
	if err != nil {
		return fmt.Errorf("seeding security: %w", err)
	}
	return nil
}

type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// loadTrades reads an account's whole log in the form portfolio.Replay takes.
func loadTrades(ctx context.Context, q queryer, accountID int) ([]portfolio.Trade, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, symbol, side, trade_date, shares, price, amount
		 FROM trades WHERE account_id = $1`, accountID)
	if err != nil {
		return nil, fmt.Errorf("loading trades: %w", err)
	}
	defer rows.Close()
	var out []portfolio.Trade
	for rows.Next() {
		var t portfolio.Trade
		if err := rows.Scan(&t.Seq, &t.Symbol, &t.Side, &t.Date, &t.Shares, &t.Price, &t.Amount); err != nil {
			return nil, fmt.Errorf("scanning trade: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// validateLog turns an oversell into a message for the person using the app.
func validateLog(trades []portfolio.Trade) error {
	_, err := portfolio.Replay(trades)
	var over *portfolio.OversellError
	if errors.As(err, &over) {
		return invalid("%s", over.Error())
	}
	return err
}

// insertTradeLeg writes the cash side of a buy or sell. Sides that don't move
// cash write nothing.
func insertTradeLeg(ctx context.Context, tx *sql.Tx, householdID, accountID, tradeID int, in TradeInput) error {
	if !portfolio.MovesCash(in.Side) {
		return nil
	}
	verb := "Buy"
	if in.Side == portfolio.SideSell {
		verb = "Sell"
	}
	description := fmt.Sprintf("%s %s %s @ $%s", verb, portfolio.FormatShares(in.Shares), in.Symbol,
		strconv.FormatFloat(in.Price, 'f', -1, 64))

	_, err := tx.ExecContext(ctx,
		`INSERT INTO transactions
		   (household_id, account_id, date, amount, kind, description, trade_id, source)
		 VALUES ($1,$2,$3,$4,'trade',$5,$6,'manual')`,
		householdID, accountID, in.TradeDate, portfolio.SignedCash(in.Side, *in.Amount),
		description, tradeID,
	)
	if err != nil {
		return fmt.Errorf("inserting trade leg: %w", err)
	}
	return nil
}
