package services

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/cwnelson/fangorn/internal/crypto"
)

type SyncService struct {
	db    *sql.DB
	plaid *PlaidService
	key   string
}

func NewSyncService(db *sql.DB, plaid *PlaidService, encryptionKey string) *SyncService {
	return &SyncService{db: db, plaid: plaid, key: encryptionKey}
}

// LinkItem exchanges a public token, encrypts the access token, stores the item, and syncs accounts.
func (s *SyncService) LinkItem(ctx context.Context, publicToken string, institutionID, institutionName string) error {
	accessToken, _, err := s.plaid.ExchangePublicToken(ctx, publicToken)
	if err != nil {
		return fmt.Errorf("exchanging token: %w", err)
	}

	encrypted, err := crypto.Encrypt(accessToken, s.key)
	if err != nil {
		return fmt.Errorf("encrypting access token: %w", err)
	}

	var itemID int
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO plaid_items (institution_id, institution_name, encrypted_access_token)
		 VALUES ($1, $2, $3) RETURNING id`,
		institutionID, institutionName, encrypted,
	).Scan(&itemID)
	if err != nil {
		return fmt.Errorf("inserting plaid item: %w", err)
	}

	// Fetch and store accounts
	accounts, err := s.plaid.GetAccounts(ctx, accessToken)
	if err != nil {
		return fmt.Errorf("fetching accounts: %w", err)
	}

	for _, acct := range accounts {
		balances := acct.GetBalances()
		var currentBal, availBal *float64
		if v, ok := balances.GetCurrentOk(); ok && v != nil {
			f := float64(*v)
			currentBal = &f
		}
		if v, ok := balances.GetAvailableOk(); ok && v != nil {
			f := float64(*v)
			availBal = &f
		}

		_, err := s.db.ExecContext(ctx,
			`INSERT INTO accounts (plaid_item_id, plaid_account_id, name, official_name, type, subtype, mask, current_balance, available_balance, iso_currency_code)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			 ON CONFLICT (plaid_account_id) DO UPDATE SET
			   name = EXCLUDED.name, current_balance = EXCLUDED.current_balance,
			   available_balance = EXCLUDED.available_balance, updated_at = NOW()`,
			itemID, acct.GetAccountId(), acct.GetName(),
			nullStr(acct.GetOfficialName()), string(acct.GetType()),
			nullStr(string(acct.GetSubtype())), nullStr(acct.GetMask()),
			currentBal, availBal,
			orDefault(balances.GetIsoCurrencyCode(), "USD"),
		)
		if err != nil {
			return fmt.Errorf("upserting account %s: %w", acct.GetAccountId(), err)
		}
	}

	log.Printf("Linked item with %d accounts", len(accounts))
	return nil
}

// SyncAll syncs transactions for all linked items.
func (s *SyncService) SyncAll(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id, encrypted_access_token, cursor FROM plaid_items`)
	if err != nil {
		return fmt.Errorf("querying items: %w", err)
	}
	defer rows.Close()

	type item struct {
		id        int
		token     string
		cursor    *string
	}
	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.token, &it.cursor); err != nil {
			return fmt.Errorf("scanning item: %w", err)
		}
		items = append(items, it)
	}

	for _, it := range items {
		if err := s.syncItem(ctx, it.id, it.token, it.cursor); err != nil {
			log.Printf("Error syncing item %d: %v", it.id, err)
			continue
		}
	}

	return nil
}

func (s *SyncService) syncItem(ctx context.Context, itemID int, encryptedToken string, cursor *string) error {
	accessToken, err := crypto.Decrypt(encryptedToken, s.key)
	if err != nil {
		return fmt.Errorf("decrypting access token: %w", err)
	}

	cursorStr := ""
	if cursor != nil {
		cursorStr = *cursor
	}

	result, err := s.plaid.SyncTransactions(ctx, accessToken, cursorStr)
	if err != nil {
		return fmt.Errorf("syncing transactions: %w", err)
	}

	// Update account balances
	for _, acct := range result.Accounts {
		balances := acct.GetBalances()
		var currentBal, availBal *float64
		if v, ok := balances.GetCurrentOk(); ok && v != nil {
			f := float64(*v)
			currentBal = &f
		}
		if v, ok := balances.GetAvailableOk(); ok && v != nil {
			f := float64(*v)
			availBal = &f
		}

		_, err := s.db.ExecContext(ctx,
			`UPDATE accounts SET current_balance = $1, available_balance = $2, updated_at = NOW()
			 WHERE plaid_account_id = $3`,
			currentBal, availBal, acct.GetAccountId(),
		)
		if err != nil {
			log.Printf("Error updating account %s balance: %v", acct.GetAccountId(), err)
		}
	}

	// Upsert added/modified transactions
	for _, txn := range append(result.Added, result.Modified...) {
		var accountID int
		err := s.db.QueryRowContext(ctx,
			`SELECT id FROM accounts WHERE plaid_account_id = $1`, txn.GetAccountId(),
		).Scan(&accountID)
		if err != nil {
			log.Printf("Account not found for transaction %s: %v", txn.GetTransactionId(), err)
			continue
		}

		category := strings.Join(txn.GetCategory(), " > ")

		_, err = s.db.ExecContext(ctx,
			`INSERT INTO transactions (plaid_transaction_id, account_id, amount, iso_currency_code, date, name, merchant_name, plaid_category, pending)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			 ON CONFLICT (plaid_transaction_id) DO UPDATE SET
			   amount = EXCLUDED.amount, name = EXCLUDED.name, merchant_name = EXCLUDED.merchant_name,
			   plaid_category = EXCLUDED.plaid_category, pending = EXCLUDED.pending, updated_at = NOW()`,
			txn.GetTransactionId(), accountID, txn.GetAmount(),
			orDefault(txn.GetIsoCurrencyCode(), "USD"),
			txn.GetDate(), txn.GetName(), nullStr(txn.GetMerchantName()),
			nilIfEmpty(category), txn.GetPending(),
		)
		if err != nil {
			log.Printf("Error upserting transaction %s: %v", txn.GetTransactionId(), err)
		}
	}

	// Remove deleted transactions
	for _, removed := range result.Removed {
		_, err := s.db.ExecContext(ctx,
			`DELETE FROM transactions WHERE plaid_transaction_id = $1`,
			removed.GetTransactionId(),
		)
		if err != nil {
			log.Printf("Error removing transaction %s: %v", removed.GetTransactionId(), err)
		}
	}

	// Update cursor
	_, err = s.db.ExecContext(ctx,
		`UPDATE plaid_items SET cursor = $1, updated_at = NOW() WHERE id = $2`,
		result.Cursor, itemID,
	)
	if err != nil {
		return fmt.Errorf("updating cursor: %w", err)
	}

	// Snapshot net worth
	if err := s.snapshotNetWorth(ctx); err != nil {
		log.Printf("Error snapshotting net worth: %v", err)
	}

	log.Printf("Synced item %d: %d added, %d modified, %d removed",
		itemID, len(result.Added), len(result.Modified), len(result.Removed))
	return nil
}

func (s *SyncService) snapshotNetWorth(ctx context.Context) error {
	var assets, liabilities float64

	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(CASE WHEN type IN ('depository', 'investment') THEN COALESCE(current_balance, 0) ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN type IN ('credit', 'loan') THEN COALESCE(current_balance, 0) ELSE 0 END), 0)
		 FROM accounts`,
	).Scan(&assets, &liabilities)
	if err != nil {
		return err
	}

	today := time.Now().Format("2006-01-02")
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO net_worth_snapshots (total_assets, total_liabilities, net_worth, snapshot_date)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (snapshot_date) DO UPDATE SET
		   total_assets = EXCLUDED.total_assets, total_liabilities = EXCLUDED.total_liabilities,
		   net_worth = EXCLUDED.net_worth`,
		assets, liabilities, assets-liabilities, today,
	)
	return err
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
