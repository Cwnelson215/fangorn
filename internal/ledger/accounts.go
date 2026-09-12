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
// cached: starting_balance plus every transaction on the account. Because amounts
// are signed relative to the account, this is correct for liabilities too — a
// credit card's purchases are negative, so its balance goes further into the red.
const accountSelect = `
	SELECT a.id, a.household_id, a.name, a.institution_name, a.type, a.class, a.mask,
	       a.starting_balance, a.starting_balance_date, a.currency, a.color, a.notes,
	       a.archived_at IS NOT NULL AS archived,
	       a.starting_balance + COALESCE(t.total, 0) AS balance
	FROM accounts a
	LEFT JOIN (
		SELECT account_id, SUM(amount) AS total FROM transactions GROUP BY account_id
	) t ON t.account_id = a.id`

func scanAccount(rows interface{ Scan(...any) error }) (models.Account, error) {
	var a models.Account
	var institution, mask, color, notes sql.NullString
	var startDate time.Time
	err := rows.Scan(
		&a.ID, &a.HouseholdID, &a.Name, &institution, &a.Type, &a.Class, &mask,
		&a.StartingBalance, &startDate, &a.Currency, &color, &notes,
		&a.Archived, &a.Balance,
	)
	if err != nil {
		return a, err
	}
	a.StartingBalanceDate = dateStr(startDate)
	a.InstitutionName = strPtr(institution)
	a.Mask = strPtr(mask)
	a.Color = strPtr(color)
	a.Notes = strPtr(notes)
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
}

func (in *AccountInput) normalize() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return invalid("name is required")
	}
	if !models.ValidAccountType(in.Type) {
		return invalid("unknown account type %q", in.Type)
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

	var id int
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO accounts
		   (household_id, name, institution_name, type, class, mask,
		    starting_balance, starting_balance_date, currency, color, notes)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		 RETURNING id`,
		householdID, in.Name, nullStr(in.InstitutionName), in.Type,
		models.ClassForType(in.Type), nullStr(in.Mask),
		in.StartingBalance, in.StartingBalanceDate, in.Currency,
		nullStr(in.Color), nullStr(in.Notes),
	).Scan(&id)
	if err != nil {
		return models.Account{}, fmt.Errorf("creating account: %w", err)
	}
	return s.GetAccount(ctx, householdID, id)
}

func (s *Service) UpdateAccount(ctx context.Context, householdID, id int, in AccountInput) (models.Account, error) {
	if err := in.normalize(); err != nil {
		return models.Account{}, err
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE accounts SET
		   name = $1, institution_name = $2, type = $3, class = $4, mask = $5,
		   starting_balance = $6, starting_balance_date = $7, currency = $8,
		   color = $9, notes = $10, updated_at = NOW()
		 WHERE household_id = $11 AND id = $12`,
		in.Name, nullStr(in.InstitutionName), in.Type, models.ClassForType(in.Type),
		nullStr(in.Mask), in.StartingBalance, in.StartingBalanceDate, in.Currency,
		nullStr(in.Color), nullStr(in.Notes), householdID, id,
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
// running balance as of that transaction. The window function sums oldest-first
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
		        recurring_rule_id, source, created_at, running_balance
		 FROM (
		   SELECT t.id, t.account_id, t.date, t.amount, t.kind, t.description, t.merchant,
		          t.category_id, c.name AS category_name, t.notes, t.transfer_group_id,
		          t.recurring_rule_id, t.source, t.created_at,
		          a.starting_balance + SUM(t.amount) OVER (
		              ORDER BY t.date, t.id ROWS UNBOUNDED PRECEDING
		          ) AS running_balance
		   FROM transactions t
		   JOIN accounts a ON a.id = t.account_id
		   LEFT JOIN categories c ON c.id = t.category_id
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
		var categoryID, ruleID sql.NullInt64
		var running sql.NullFloat64
		var date, createdAt time.Time
		if err := rows.Scan(
			&t.ID, &t.AccountID, &date, &t.Amount, &t.Kind, &t.Description, &merchant,
			&categoryID, &categoryName, &notes, &groupID, &ruleID, &t.Source,
			&createdAt, &running,
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
		t.RunningBalance = floatPtr(running)
		out = append(out, t)
	}
	return out, rows.Err()
}
