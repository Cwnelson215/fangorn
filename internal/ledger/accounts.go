package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/cwnelson/fangorn/internal/models"
)

// accountSelect is the shared projection. Balance is always computed rather than
// cached, from accountBalances: cash (starting_balance plus every transaction)
// plus the market value of any holdings. Because amounts are signed relative to
// the account, this is correct for liabilities too — a credit card's purchases
// are negative, so its balance goes further into the red.
const accountSelect = `
	SELECT a.id, a.household_id, a.name, a.institution_name, a.type, a.class, a.mask,
	       a.starting_balance, a.starting_balance_date, a.currency, a.color, a.notes,
	       a.tax_treatment,
	       (SELECT r.apy FROM savings_rates r
	         WHERE r.account_id = a.id AND r.effective_from <= CURRENT_DATE
	         ORDER BY r.effective_from DESC LIMIT 1) AS apy,
	       a.archived_at IS NOT NULL AS archived,
	       b.cash_balance, b.holdings_value, b.cash_balance + b.holdings_value AS balance
	FROM accounts a
	JOIN (` + accountBalances + `) b ON b.account_id = a.id`

func scanAccount(rows interface{ Scan(...any) error }) (models.Account, error) {
	var a models.Account
	var institution, mask, color, notes, tax sql.NullString
	var apy sql.NullFloat64
	var startDate time.Time
	err := rows.Scan(
		&a.ID, &a.HouseholdID, &a.Name, &institution, &a.Type, &a.Class, &mask,
		&a.StartingBalance, &startDate, &a.Currency, &color, &notes,
		&tax, &apy, &a.Archived, &a.CashBalance, &a.HoldingsValue, &a.Balance,
	)
	if err != nil {
		return a, err
	}
	a.StartingBalanceDate = dateStr(startDate)
	a.InstitutionName = strPtr(institution)
	a.Mask = strPtr(mask)
	a.Color = strPtr(color)
	a.Notes = strPtr(notes)
	a.TaxTreatment = strPtr(tax)
	if apy.Valid {
		a.APY = &apy.Float64
	}
	return a, nil
}

// ListAccounts returns the household's accounts, assets first then liabilities,
// alphabetical within each. Archived accounts are excluded unless asked for.
func (s *Service) ListAccounts(ctx context.Context, householdID int, includeArchived bool) ([]models.Account, error) {
	q := accountSelect + ` WHERE a.household_id = $1`
	if !includeArchived {
		q += ` AND a.archived_at IS NULL`
	}
	q += ` ORDER BY a.class, a.name`

	rows, err := s.db.QueryContext(ctx, q, householdID)
	if err != nil {
		return nil, fmt.Errorf("listing accounts: %w", err)
	}
	defer rows.Close()

	accounts := []models.Account{}
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning account: %w", err)
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

func (s *Service) GetAccount(ctx context.Context, householdID, id int) (models.Account, error) {
	row := s.db.QueryRowContext(ctx,
		accountSelect+` WHERE a.household_id = $1 AND a.id = $2`, householdID, id)

	a, err := scanAccount(row)
	if err == sql.ErrNoRows {
		return a, ErrNotFound
	}
	if err != nil {
		return a, fmt.Errorf("fetching account: %w", err)
	}
	return a, nil
}

// AccountInput is the writable surface of an account. Class is not included —
// it is derived from Type, so the two can never disagree.
type AccountInput struct {
	Name                string  `json:"name"`
	InstitutionName     *string `json:"institution_name"`
	Type                string  `json:"type"`
	Mask                *string `json:"mask"`
	StartingBalance     float64 `json:"starting_balance"`
	StartingBalanceDate string  `json:"starting_balance_date"`
	Currency            string  `json:"currency"`
	Color               *string `json:"color"`
	Notes               *string `json:"notes"`
	// TaxTreatment is required on a retirement account and refused on any other.
	TaxTreatment *string `json:"tax_treatment"`
	// APY is the opening rate of a new high-yield savings account, in percent,
	// in effect from StartingBalanceDate. It is only read on create: later
	// changes go into the rate history (AddSavingsRate) so past months keep the
	// rate they were earned at.
	APY *float64 `json:"apy"`
}

func trimmedOrNil(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}

func (in *AccountInput) normalize() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return invalid("name is required")
	}
	// Accounts are grouped by institution, so "Gesa" and "Gesa " must be one.
	in.InstitutionName = trimmedOrNil(in.InstitutionName)
	in.Mask = trimmedOrNil(in.Mask)
	if !models.ValidAccountType(in.Type) {
		return invalid("unknown account type %q", in.Type)
	}
	in.TaxTreatment = trimmedOrNil(in.TaxTreatment)
	if in.Type == models.AccountRetirement {
		if in.TaxTreatment == nil ||
			(*in.TaxTreatment != models.TaxRoth && *in.TaxTreatment != models.TaxTraditional) {
			return invalid("a retirement account must be Roth or traditional")
		}
	} else if in.TaxTreatment != nil {
		return invalid("only retirement accounts have a tax treatment")
	}
	if in.APY != nil {
		if in.Type != models.AccountHighYieldSavings {
			return invalid("only high-yield savings accounts have an interest rate")
		}
		if err := validAPY(*in.APY); err != nil {
			return err
		}
	}
	if in.StartingBalanceDate == "" {
		return invalid("starting_balance_date is required")
	}
	if _, err := models.ParseDate(in.StartingBalanceDate); err != nil {
		return invalid("starting_balance_date must be YYYY-MM-DD")
	}
	if in.Currency == "" {
		in.Currency = "USD"
	}
	return nil
}

func (s *Service) CreateAccount(ctx context.Context, householdID int, in AccountInput) (models.Account, error) {
	if err := in.normalize(); err != nil {
		return models.Account{}, err
	}
	if in.Type == models.AccountHighYieldSavings && in.APY == nil {
		return models.Account{}, invalid("a high-yield savings account needs its interest rate (APY)")
	}

	var id int
	err := s.inTx(func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx,
			`INSERT INTO accounts
		   (household_id, name, institution_name, type, class, mask,
		    starting_balance, starting_balance_date, currency, color, notes, tax_treatment)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		 RETURNING id`,
			householdID, in.Name, nullStr(in.InstitutionName), in.Type,
			models.ClassForType(in.Type), nullStr(in.Mask),
			in.StartingBalance, in.StartingBalanceDate, in.Currency,
			nullStr(in.Color), nullStr(in.Notes), nullStr(in.TaxTreatment),
		).Scan(&id)
		if err != nil {
			return fmt.Errorf("creating account: %w", err)
		}
		if in.APY == nil {
			return nil
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO savings_rates (household_id, account_id, apy, effective_from)
			 VALUES ($1,$2,$3,$4)`,
			householdID, id, *in.APY, in.StartingBalanceDate)
		if err != nil {
			return fmt.Errorf("recording the opening rate: %w", err)
		}
		return nil
	})
	if err != nil {
		return models.Account{}, err
	}
	return s.GetAccount(ctx, householdID, id)
}

func (s *Service) UpdateAccount(ctx context.Context, householdID, id int, in AccountInput) (models.Account, error) {
	if err := in.normalize(); err != nil {
		return models.Account{}, err
	}
	if in.APY != nil {
		return models.Account{}, invalid("change the rate from the account's page, so past months keep the rate they earned")
	}

	// A rate history only earns on a type that EarnsOnCash; keep the two together
	// rather than strand the rates on, say, a checking account.
	if !models.EarnsOnCash(in.Type) {
		var hasRates bool
		err := s.db.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM savings_rates WHERE household_id = $1 AND account_id = $2)`,
			householdID, id).Scan(&hasRates)
		if err != nil {
			return models.Account{}, fmt.Errorf("checking for rates: %w", err)
		}
		if hasRates {
			return models.Account{}, invalid("this account has an interest rate history; remove it before changing to a type that doesn't earn on cash")
		}
	}

	// A linked cash fund likewise only belongs on an account that holds securities.
	if !models.HoldsSecurities(in.Type) {
		var linked bool
		err := s.db.QueryRowContext(ctx,
			`SELECT cash_fund IS NOT NULL FROM accounts WHERE household_id = $1 AND id = $2`,
			householdID, id).Scan(&linked)
		if err != nil && err != sql.ErrNoRows {
			return models.Account{}, fmt.Errorf("checking the cash fund: %w", err)
		}
		if linked {
			return models.Account{}, invalid("this account's cash is linked to a money market fund; unlink it before changing to a type that doesn't hold securities")
		}
	}

	// Trades only make sense on an account that holds securities, so one that
	// has any cannot be turned into something else out from under them. Moving
	// between investment and retirement is fine: both keep the trade log.
	if !models.HoldsSecurities(in.Type) {
		var hasTrades bool
		err := s.db.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM trades WHERE household_id = $1 AND account_id = $2)`,
			householdID, id).Scan(&hasTrades)
		if err != nil {
			return models.Account{}, fmt.Errorf("checking for trades: %w", err)
		}
		if hasTrades {
			return models.Account{}, invalid("this account has trades logged, so it has to stay an investment or retirement account")
		}
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE accounts SET
		   name = $1, institution_name = $2, type = $3, class = $4, mask = $5,
		   starting_balance = $6, starting_balance_date = $7, currency = $8,
		   color = $9, notes = $10, tax_treatment = $11, updated_at = NOW()
		 WHERE household_id = $12 AND id = $13`,
		in.Name, nullStr(in.InstitutionName), in.Type, models.ClassForType(in.Type),
		nullStr(in.Mask), in.StartingBalance, in.StartingBalanceDate, in.Currency,
		nullStr(in.Color), nullStr(in.Notes), nullStr(in.TaxTreatment), householdID, id,
	)
	if err != nil {
		return models.Account{}, fmt.Errorf("updating account: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.Account{}, ErrNotFound
	}
	return s.GetAccount(ctx, householdID, id)
}

// SetAccountArchived hides an account without touching its transactions, so
// historical figures stay intact. This is the normal way to retire an account.
func (s *Service) SetAccountArchived(ctx context.Context, householdID, id int, archived bool) error {
	q := `UPDATE accounts SET archived_at = NULL, updated_at = NOW() WHERE household_id = $1 AND id = $2`
	if archived {
		q = `UPDATE accounts SET archived_at = NOW(), updated_at = NOW() WHERE household_id = $1 AND id = $2`
	}

	res, err := s.db.ExecContext(ctx, q, householdID, id)
	if err != nil {
		return fmt.Errorf("archiving account: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteAccount removes the account and cascades to its transactions and
// recurring rules. Archiving is almost always the better choice; this exists for
// cleaning up an account created by mistake.
func (s *Service) DeleteAccount(ctx context.Context, householdID, id int) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM accounts WHERE household_id = $1 AND id = $2`, householdID, id)
	if err != nil {
		return fmt.Errorf("deleting account: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Register returns an account's transactions newest-first, each carrying the
// running balance as of that transaction. For an investment account that running
// balance is cash only — holdings are valued at today's prices, which says
// nothing about what they were worth on the date of an old transaction. The window function sums oldest-first
// so the running total reads as a bank statement would; the outer query then
// flips the order for display.
func (s *Service) Register(ctx context.Context, householdID, accountID, limit int) ([]models.Transaction, error) {
	if _, err := s.GetAccount(ctx, householdID, accountID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 1000 {
		limit = 500
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, account_id, date, amount, kind, description, merchant,
		        category_id, category_name, notes, transfer_group_id,
		        recurring_rule_id, trade_id, source, created_at, receipt_id, running_balance
		 FROM (
		   SELECT t.id, t.account_id, t.date, t.amount, t.kind, t.description, t.merchant,
		          t.category_id, c.name AS category_name, t.notes, t.transfer_group_id,
		          t.recurring_rule_id, t.trade_id, t.source, t.created_at, r.id AS receipt_id,
		          a.starting_balance + SUM(t.amount) OVER (
		              ORDER BY t.date, t.id ROWS UNBOUNDED PRECEDING
		          ) AS running_balance
		   FROM transactions t
		   JOIN accounts a ON a.id = t.account_id
		   LEFT JOIN categories c ON c.id = t.category_id
		   LEFT JOIN receipts r ON r.transaction_id = t.id
		   WHERE t.account_id = $1 AND t.household_id = $2
		 ) reg
		 ORDER BY date DESC, id DESC
		 LIMIT $3`,
		accountID, householdID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("loading register: %w", err)
	}
	defer rows.Close()

	out := []models.Transaction{}
	for rows.Next() {
		var t models.Transaction
		var merchant, categoryName, notes, groupID sql.NullString
		var categoryID, ruleID, tradeID, receiptID sql.NullInt64
		var running sql.NullFloat64
		var date, createdAt time.Time
		if err := rows.Scan(
			&t.ID, &t.AccountID, &date, &t.Amount, &t.Kind, &t.Description, &merchant,
			&categoryID, &categoryName, &notes, &groupID, &ruleID, &tradeID, &t.Source,
			&createdAt, &receiptID, &running,
		); err != nil {
			return nil, fmt.Errorf("scanning register row: %w", err)
		}
		t.Date = dateStr(date)
		t.CreatedAt = createdAt.Format(time.RFC3339)
		t.Merchant = strPtr(merchant)
		t.CategoryID = intPtr(categoryID)
		t.CategoryName = strPtr(categoryName)
		t.Notes = strPtr(notes)
		t.TransferGroupID = strPtr(groupID)
		t.RecurringRuleID = intPtr(ruleID)
		t.TradeID = intPtr(tradeID)
		t.ReceiptID = intPtr(receiptID)
		t.RunningBalance = floatPtr(running)
		out = append(out, t)
	}
	return out, rows.Err()
}
