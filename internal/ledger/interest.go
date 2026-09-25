package ledger

import (
	"context"
	"database/sql"
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

func (s *Service) interestRates(ctx context.Context, accountID int) ([]interest.Rate, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT effective_from, apy FROM savings_rates WHERE account_id = $1`, accountID)
	if err != nil {
		return nil, fmt.Errorf("loading rates: %w", err)
	}
	defer rows.Close()
	var rates []interest.Rate
	for rows.Next() {
		var r interest.Rate
		if err := rows.Scan(&r.From, &r.APY); err != nil {
			return nil, fmt.Errorf("scanning rate: %w", err)
		}
		rates = append(rates, r)
	}
	return rates, rows.Err()
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
		`SELECT a.id, a.type, a.starting_balance_date FROM accounts a
		 WHERE a.household_id = $1 AND a.archived_at IS NULL
		   AND EXISTS (SELECT 1 FROM savings_rates r WHERE r.account_id = a.id)`,
		householdID)
	if err != nil {
		return 0, fmt.Errorf("listing accounts with a rate: %w", err)
	}
	type account struct {
		id    int
		typ   string
		start time.Time
	}
	var accounts []account
	for rows.Next() {
		var a account
		if err := rows.Scan(&a.id, &a.typ, &a.start); err != nil {
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
		n, err := s.postAccountInterest(ctx, householdID, a.id, a.typ, a.start, today)
		posted += n
		if err != nil {
			return posted, fmt.Errorf("account %d: %w", a.id, err)
		}
	}
	return posted, nil
}

func (s *Service) postAccountInterest(ctx context.Context, householdID, accountID int, typ string, start, today time.Time) (int, error) {
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
		wrote, err := s.postInterestMonth(ctx, householdID, accountID, typ, m, rates, start)
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
// dividend, and keeping those apart lets a report tell them apart.
func earningLabel(accountType string) (description, category string) {
	if accountType == models.AccountHighYieldSavings {
		return "Interest", "Interest"
	}
	return "Money market dividend", "Dividends"
}

// postInterestMonth works out one month and records it. The postings row and
// the transaction go in together; a row that already exists means another pass
// got there first, and nothing is written. The balance is the cash balance, so
// an investment account's holdings don't earn the cash rate.
func (s *Service) postInterestMonth(ctx context.Context, householdID, accountID int, typ string, month time.Time, rates []interest.Rate, start time.Time) (bool, error) {
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

		description, categoryName := earningLabel(typ)
		categoryID, err := incomeCategory(ctx, tx, householdID, categoryName)
		if err != nil {
			return err
		}
		var txnID int
		err = tx.QueryRowContext(ctx,
			`INSERT INTO transactions
			   (household_id, account_id, date, amount, kind, description, category_id, source)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			 RETURNING id`,
			householdID, accountID, dateStr(end), amount, models.KindIncome, description,
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
	return out, nil
}
