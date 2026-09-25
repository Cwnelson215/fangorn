package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/cwnelson/fangorn/internal/interest"
	"github.com/cwnelson/fangorn/internal/models"
)

// A high-yield savings account keeps a rate history, and once a month is over
// the scheduler posts that month's interest as an income transaction. An
// investment or retirement account can keep one too, for the yield of its
// uninvested cash (a money market core position like SPAXX): the same posting,
// on the cash balance only, filed as a dividend. The arithmetic is in
// internal/interest; this file loads what it needs and makes posting safe to
// repeat.

func validAPY(apy float64) error {
	if math.IsNaN(apy) || apy < 0 || apy >= 100 {
		return invalid("the APY must be a percentage from 0 to 100, like 4.35")
	}
	return nil
}

// SavingsRateInput sets an APY from a date. Setting a second rate on the same
// date replaces the first.
type SavingsRateInput struct {
	APY           float64 `json:"apy"`
	EffectiveFrom string  `json:"effective_from"`
}

// assertEarnsOnCash checks the account exists in the household and is a type
// whose cash can earn a rate, returning its type and the date it starts
// earning from.
func (s *Service) assertEarnsOnCash(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, householdID, accountID int) (string, time.Time, error) {
	var typ string
	var start time.Time
	err := q.QueryRowContext(ctx,
		`SELECT type, starting_balance_date FROM accounts WHERE household_id = $1 AND id = $2`,
		householdID, accountID).Scan(&typ, &start)
	if err == sql.ErrNoRows {
		return typ, start, ErrNotFound
	}
	if err != nil {
		return typ, start, fmt.Errorf("fetching account: %w", err)
	}
	if !models.EarnsOnCash(typ) {
		return typ, start, invalid("only high-yield savings, investment and retirement accounts earn a rate on their cash")
	}
	return typ, start, nil
}

func (s *Service) ListSavingsRates(ctx context.Context, householdID, accountID int) ([]models.SavingsRate, error) {
	if _, _, err := s.assertEarnsOnCash(ctx, s.db, householdID, accountID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, account_id, apy, effective_from FROM savings_rates
		 WHERE household_id = $1 AND account_id = $2
		 ORDER BY effective_from DESC`, householdID, accountID)
	if err != nil {
		return nil, fmt.Errorf("listing rates: %w", err)
	}
	defer rows.Close()
	rates := []models.SavingsRate{}
	for rows.Next() {
		var r models.SavingsRate
		var from time.Time
		if err := rows.Scan(&r.ID, &r.AccountID, &r.APY, &from); err != nil {
			return nil, fmt.Errorf("scanning rate: %w", err)
		}
		r.EffectiveFrom = dateStr(from)
		rates = append(rates, r)
	}
	return rates, rows.Err()
}

func (s *Service) AddSavingsRate(ctx context.Context, householdID, accountID int, in SavingsRateInput) (models.SavingsRate, error) {
	if err := validAPY(in.APY); err != nil {
		return models.SavingsRate{}, err
	}
	if _, err := models.ParseDate(in.EffectiveFrom); err != nil {
		return models.SavingsRate{}, invalid("effective_from must be YYYY-MM-DD")
	}
	if _, _, err := s.assertEarnsOnCash(ctx, s.db, householdID, accountID); err != nil {
		return models.SavingsRate{}, err
	}
	var fund sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT cash_fund FROM accounts WHERE id = $1`, accountID).Scan(&fund); err != nil {
		return models.SavingsRate{}, fmt.Errorf("checking the cash fund: %w", err)
	}
	if fund.Valid {
		return models.SavingsRate{}, invalid("this account's yield is looked up from %s; unlink it to enter one by hand", fund.String)
	}
	r := models.SavingsRate{AccountID: accountID, APY: in.APY, EffectiveFrom: in.EffectiveFrom}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO savings_rates (household_id, account_id, apy, effective_from)
		 VALUES ($1,$2,$3,$4)
		 ON CONFLICT (account_id, effective_from) DO UPDATE SET apy = EXCLUDED.apy
		 RETURNING id`,
		householdID, accountID, in.APY, in.EffectiveFrom).Scan(&r.ID)
	if err != nil {
		return models.SavingsRate{}, fmt.Errorf("saving rate: %w", err)
	}
	return r, nil
}

// DeleteSavingsRate removes a mistaken entry. A high-yield savings account's
// last rate can't go: it would silently stop earning, which is never what
// deleting a typo means. On an investment account the yield is optional, and
// removing the last one just turns the cash dividend off.
func (s *Service) DeleteSavingsRate(ctx context.Context, householdID, accountID, rateID int) error {
	typ, _, err := s.assertEarnsOnCash(ctx, s.db, householdID, accountID)
	if err != nil {
		return err
	}
	return s.inTx(func(tx *sql.Tx) error {
		var n int
		err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM savings_rates WHERE household_id = $1 AND account_id = $2`,
			householdID, accountID).Scan(&n)
		if err != nil {
			return fmt.Errorf("counting rates: %w", err)
		}
		res, err := tx.ExecContext(ctx,
			`DELETE FROM savings_rates WHERE household_id = $1 AND account_id = $2 AND id = $3`,
			householdID, accountID, rateID)
		if err != nil {
			return fmt.Errorf("deleting rate: %w", err)
		}
		if k, _ := res.RowsAffected(); k == 0 {
			return ErrNotFound
		}
		if n <= 1 && typ == models.AccountHighYieldSavings {
			return invalid("a high-yield savings account needs at least one rate; add the new rate before removing this one")
		}
		return nil
	})
}

// interestRates is the rate history the month's earnings are worked out from.
// Usually that is the account's own savings_rates. An account linked to a cash
// fund switches to the fund's published yields on the day it was linked:
// hand-entered rates still cover the time before, the fund everything after.
func (s *Service) interestRates(ctx context.Context, accountID int) ([]interest.Rate, error) {
	var fund sql.NullString
	var since sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT cash_fund, cash_fund_since FROM accounts WHERE id = $1`, accountID).Scan(&fund, &since)
	if err != nil {
		return nil, fmt.Errorf("loading the cash fund: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT effective_from, apy FROM savings_rates WHERE account_id = $1`, accountID)
	if err != nil {
		return nil, fmt.Errorf("loading rates: %w", err)
	}
	var rates []interest.Rate
	for rows.Next() {
		var r interest.Rate
		if err := rows.Scan(&r.From, &r.APY); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scanning rate: %w", err)
		}
		if !fund.Valid || r.From.Before(since.Time) {
			rates = append(rates, r)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil || !fund.Valid {
		return rates, err
	}

	yields, err := s.fundYields(ctx, fund.String)
	if err != nil {
		return nil, err
	}
	// The fund's rate on the day it was linked: the latest yield on or before
	// it, or — if the first lookup came after — the first one there is.
	var atLink *float64
	for _, y := range yields {
		if !y.From.After(since.Time) {
			v := y.APY
			atLink = &v
		}
	}
	if atLink == nil && len(yields) > 0 {
		atLink = &yields[0].APY
	}
	if atLink != nil {
		rates = append(rates, interest.Rate{From: since.Time, APY: *atLink})
	}
	for _, y := range yields {
		if y.From.After(since.Time) {
			rates = append(rates, y)
		}
	}
	return rates, nil
}

// fundYields is a fund's yield history, oldest first, as rates. A money market
// fund quotes its 7-day yield as a simple annual rate that accrues daily and is
// paid monthly, so a month earns yield ÷ 12. The rate engine takes an APY and
// earns (1+APY)^(1/12) − 1 a month; (1 + yield/12)^12 − 1 is the APY that makes
// the two agree.
func (s *Service) fundYields(ctx context.Context, symbol string) ([]interest.Rate, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT as_of, yield FROM security_yields WHERE symbol = $1 ORDER BY as_of`, symbol)
	if err != nil {
		return nil, fmt.Errorf("loading %s yields: %w", symbol, err)
	}
	defer rows.Close()
	var out []interest.Rate
	for rows.Next() {
		var r interest.Rate
		var yield float64
		if err := rows.Scan(&r.From, &yield); err != nil {
			return nil, fmt.Errorf("scanning yield: %w", err)
		}
		r.APY = (math.Pow(1+yield/100/12, 12) - 1) * 100
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetCashFund links an investment or retirement account's cash to the money
// market fund it sits in, so its yield is looked up rather than typed in. An
// empty symbol unlinks it. Relinking the same fund keeps the original date.
// The fund must already be a known security (the handler fetches its first
// quote); today is the household's, and is when the fund takes over.
func (s *Service) SetCashFund(ctx context.Context, householdID, accountID int, symbol string, today time.Time) error {
	var typ string
	var current sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT type, cash_fund FROM accounts WHERE household_id = $1 AND id = $2`,
		householdID, accountID).Scan(&typ, &current)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("fetching account: %w", err)
	}
	if !models.HoldsSecurities(typ) {
		return invalid("only investment and retirement accounts have a cash fund")
	}

	if symbol == "" {
		_, err = s.db.ExecContext(ctx,
			`UPDATE accounts SET cash_fund = NULL, cash_fund_since = NULL, updated_at = NOW()
			 WHERE household_id = $1 AND id = $2`, householdID, accountID)
		if err != nil {
			return fmt.Errorf("unlinking cash fund: %w", err)
		}
		return nil
	}

	symbol = NormalizeSymbol(symbol)
	if current.Valid && current.String == symbol {
		return nil
	}
	if _, err := s.GetSecurity(ctx, symbol); errors.Is(err, ErrNotFound) {
		return invalid("couldn't look up %s right now; try again in a minute", symbol)
	} else if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE accounts SET cash_fund = $1, cash_fund_since = $2, updated_at = NOW()
		 WHERE household_id = $3 AND id = $4`,
		symbol, dateStr(today), householdID, accountID)
	if err != nil {
		return fmt.Errorf("linking cash fund: %w", err)
	}
	return nil
}

// SaveFundYield records a fund's yield for a day; a later lookup the same day
// replaces it.
func (s *Service) SaveFundYield(ctx context.Context, symbol string, asOf time.Time, yield float64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO security_yields (symbol, as_of, yield, fetched_at) VALUES ($1,$2,$3,NOW())
		 ON CONFLICT (symbol, as_of) DO UPDATE SET yield = EXCLUDED.yield, fetched_at = NOW()`,
		NormalizeSymbol(symbol), dateStr(asOf), yield)
	if err != nil {
		return fmt.Errorf("saving %s yield: %w", symbol, err)
	}
	return nil
}

// CashFundsDue lists the cash funds the household's open accounts are linked
// to whose yield hasn't been looked up since staleBefore.
func (s *Service) CashFundsDue(ctx context.Context, householdID int, staleBefore time.Time) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT a.cash_fund FROM accounts a
		 WHERE a.household_id = $1 AND a.archived_at IS NULL AND a.cash_fund IS NOT NULL
		   AND NOT EXISTS (SELECT 1 FROM security_yields y
		                   WHERE y.symbol = a.cash_fund AND y.fetched_at >= $2)`,
		householdID, staleBefore)
	if err != nil {
		return nil, fmt.Errorf("listing cash funds: %w", err)
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

// cashBalanceOn is starting_balance plus every transaction dated on or before d.
func cashBalanceOn(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, accountID int, d time.Time) (float64, error) {
	var balance float64
	err := q.QueryRowContext(ctx,
		`SELECT a.starting_balance + COALESCE(
		   (SELECT SUM(t.amount) FROM transactions t WHERE t.account_id = a.id AND t.date <= $2), 0)
		 FROM accounts a WHERE a.id = $1`, accountID, dateStr(d)).Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("balance on %s: %w", dateStr(d), err)
	}
	return balance, nil
}

// PostInterest posts every finished month's interest that hasn't been worked
// out yet, for each of the household's open accounts with a rate history. Like
// the recurring engine it is idempotent rather than reliable: run it twice, or
// not for a month, and the ledger still lands right. It returns how many
// interest transactions it wrote.
func (s *Service) PostInterest(ctx context.Context, householdID int, today time.Time) (int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT a.id, a.type, a.starting_balance_date, a.cash_fund FROM accounts a
		 WHERE a.household_id = $1 AND a.archived_at IS NULL
		   AND (a.cash_fund IS NOT NULL
		        OR EXISTS (SELECT 1 FROM savings_rates r WHERE r.account_id = a.id))`,
		householdID)
	if err != nil {
		return 0, fmt.Errorf("listing accounts with a rate: %w", err)
	}
	type account struct {
		id    int
		typ   string
		start time.Time
		fund  sql.NullString
	}
	var accounts []account
	for rows.Next() {
		var a account
		if err := rows.Scan(&a.id, &a.typ, &a.start, &a.fund); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scanning account: %w", err)
		}
		accounts = append(accounts, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	posted := 0
	for _, a := range accounts {
		if !models.EarnsOnCash(a.typ) {
			continue // rates left from an older type; the update guard prevents new ones
		}
		description, category := earningLabel(a.typ, a.fund.String)
		n, err := s.postAccountInterest(ctx, householdID, a.id, earning{description, category}, a.start, today)
		posted += n
		if err != nil {
			return posted, fmt.Errorf("account %d: %w", a.id, err)
		}
	}
	return posted, nil
}

// earning is how an account's monthly earnings are described and filed.
type earning struct{ description, category string }

func (s *Service) postAccountInterest(ctx context.Context, householdID, accountID int, label earning, start, today time.Time) (int, error) {
	rates, err := s.interestRates(ctx, accountID)
	if err != nil || len(rates) == 0 {
		return 0, err
	}
	earliest := rates[0].From
	for _, r := range rates {
		if r.From.Before(earliest) {
			earliest = r.From
		}
	}
	if earliest.Before(start) {
		earliest = start
	}
	first := interest.MonthStart(earliest)
	// Only months that are completely over: the current one is still earning.
	last := interest.MonthStart(today).AddDate(0, -1, 0)

	done := map[string]bool{}
	rows, err := s.db.QueryContext(ctx,
		`SELECT month FROM interest_postings WHERE account_id = $1 AND month >= $2`,
		accountID, dateStr(first))
	if err != nil {
		return 0, fmt.Errorf("listing postings: %w", err)
	}
	for rows.Next() {
		var m time.Time
		if err := rows.Scan(&m); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scanning posting: %w", err)
		}
		done[dateStr(m)] = true
	}
	rows.Close()

	posted := 0
	// Oldest first, so each month's balance includes the interest before it.
	for m := first; !m.After(last); m = m.AddDate(0, 1, 0) {
		if done[dateStr(m)] {
			continue
		}
		wrote, err := s.postInterestMonth(ctx, householdID, accountID, label, m, rates, start)
		if err != nil {
			return posted, fmt.Errorf("%s: %w", m.Format("2006-01"), err)
		}
		if wrote {
			posted++
		}
	}
	return posted, nil
}

// earningLabel is how a month's earnings are described and filed. Savings pay
// interest; the money market fund an investment account's cash sits in pays a
// dividend, and keeping those apart lets a report tell them apart. A linked
// fund is named, the way a statement names it.
func earningLabel(accountType, fund string) (description, category string) {
	if accountType == models.AccountHighYieldSavings {
		return "Interest", "Interest"
	}
	if fund != "" {
		return fund + " dividend", "Dividends"
	}
	return "Money market dividend", "Dividends"
}

// postInterestMonth works out one month and records it. The postings row and
// the transaction go in together; a row that already exists means another pass
// got there first, and nothing is written. The balance is the cash balance, so
// an investment account's holdings don't earn the cash rate.
func (s *Service) postInterestMonth(ctx context.Context, householdID, accountID int, label earning, month time.Time, rates []interest.Rate, start time.Time) (bool, error) {
	end := interest.MonthEnd(month)
	wrote := false
	err := s.inTx(func(tx *sql.Tx) error {
		balance, err := cashBalanceOn(ctx, tx, accountID, end)
		if err != nil {
			return err
		}
		amount := interest.ForMonth(month, balance, rates, start)

		var postingID int
		err = tx.QueryRowContext(ctx,
			`INSERT INTO interest_postings (household_id, account_id, month, amount)
			 VALUES ($1,$2,$3,$4)
			 ON CONFLICT (account_id, month) DO NOTHING
			 RETURNING id`,
			householdID, accountID, dateStr(month), amount).Scan(&postingID)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return fmt.Errorf("recording posting: %w", err)
		}
		if amount < 0.01 {
			return nil // handled, but nothing earned: no zero-dollar transaction
		}

		categoryID, err := incomeCategory(ctx, tx, householdID, label.category)
		if err != nil {
			return err
		}
		var txnID int
		err = tx.QueryRowContext(ctx,
			`INSERT INTO transactions
			   (household_id, account_id, date, amount, kind, description, category_id, source)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			 RETURNING id`,
			householdID, accountID, dateStr(end), amount, models.KindIncome, label.description,
			categoryID, models.SourceInterest).Scan(&txnID)
		if err != nil {
			return fmt.Errorf("posting interest: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE interest_postings SET transaction_id = $1 WHERE id = $2`, txnID, postingID); err != nil {
			return fmt.Errorf("linking posting: %w", err)
		}
		wrote = true
		return nil
	})
	return wrote, err
}

// incomeCategory finds the household's income category with this name, any
// case, creating it the first time, so posted earnings are filed rather than
// left uncategorized. An archived one is reused rather than duplicated.
func incomeCategory(ctx context.Context, tx *sql.Tx, householdID int, name string) (int, error) {
	var id int
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM categories
		 WHERE household_id = $1 AND kind = $2 AND lower(btrim(name)) = lower($3)
		 ORDER BY archived_at NULLS FIRST, id LIMIT 1`,
		householdID, models.KindIncome, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("finding the %s category: %w", name, err)
	}
	err = tx.QueryRowContext(ctx,
		`INSERT INTO categories (household_id, name, kind) VALUES ($1, $2, $3) RETURNING id`,
		householdID, name, models.KindIncome).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("creating the %s category: %w", name, err)
	}
	return id, nil
}

// SavingsOutlook is what the account page shows: the rate history and roughly
// what this month will earn if the cash balance stays where it is. Rates is
// empty on an investment account whose cash yield was never set.
type SavingsOutlook struct {
	Rates []models.SavingsRate `json:"rates"`
	// ProjectedDate is the last day of this month, when the interest posts.
	ProjectedDate   string  `json:"projected_date"`
	ProjectedAmount float64 `json:"projected_amount"`

	// CashFund is the money market fund an investment account's yield is looked
	// up from, with its latest published yield (a simple annual rate, percent).
	CashFund      *string  `json:"cash_fund"`
	CashFundSince *string  `json:"cash_fund_since"`
	FundYield     *float64 `json:"fund_yield"`
	FundYieldAsOf *string  `json:"fund_yield_as_of"`
}

func (s *Service) SavingsOutlookFor(ctx context.Context, householdID, accountID int) (SavingsOutlook, error) {
	out := SavingsOutlook{}
	rates, err := s.ListSavingsRates(ctx, householdID, accountID)
	if err != nil {
		return out, err
	}
	out.Rates = rates

	household, err := s.GetHousehold(ctx, householdID)
	if err != nil {
		return out, err
	}
	month := interest.MonthStart(household.Today())
	out.ProjectedDate = dateStr(interest.MonthEnd(month))

	_, start, err := s.assertEarnsOnCash(ctx, s.db, householdID, accountID)
	if err != nil {
		return out, err
	}
	calc, err := s.interestRates(ctx, accountID)
	if err != nil {
		return out, err
	}
	balance, err := cashBalanceOn(ctx, s.db, accountID, interest.MonthEnd(month))
	if err != nil {
		return out, err
	}
	out.ProjectedAmount = interest.ForMonth(month, balance, calc, start)

	var fund sql.NullString
	var since sql.NullTime
	if err := s.db.QueryRowContext(ctx,
		`SELECT cash_fund, cash_fund_since FROM accounts WHERE id = $1`, accountID).Scan(&fund, &since); err != nil {
		return out, fmt.Errorf("loading the cash fund: %w", err)
	}
	if fund.Valid {
		out.CashFund = &fund.String
		d := dateStr(since.Time)
		out.CashFundSince = &d
		var yield float64
		var asOf time.Time
		err := s.db.QueryRowContext(ctx,
			`SELECT yield, as_of FROM security_yields WHERE symbol = $1 ORDER BY as_of DESC LIMIT 1`,
			fund.String).Scan(&yield, &asOf)
		if err == nil {
			a := dateStr(asOf)
			out.FundYield, out.FundYieldAsOf = &yield, &a
		} else if err != sql.ErrNoRows {
			return out, fmt.Errorf("loading %s yield: %w", fund.String, err)
		}
	}
	return out, nil
}
