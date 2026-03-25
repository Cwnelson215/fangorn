package services

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/cwnelson/fangorn/internal/crypto"
)

type SyncService struct {
	db     *sql.DB
	teller *TellerService
	key    string
}

func NewSyncService(db *sql.DB, teller *TellerService, encryptionKey string) *SyncService {
	return &SyncService{db: db, teller: teller, key: encryptionKey}
}

// LinkInstitution stores a new linked institution and fetches its accounts.
func (s *SyncService) LinkInstitution(ctx context.Context, accessToken string, institutionID, institutionName string) error {
	encrypted, err := crypto.Encrypt(accessToken, s.key)
	if err != nil {
		return fmt.Errorf("encrypting access token: %w", err)
	}

	var instID int
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO linked_institutions (institution_id, institution_name, encrypted_access_token)
		 VALUES ($1, $2, $3) RETURNING id`,
		institutionID, institutionName, encrypted,
	).Scan(&instID)
	if err != nil {
		return fmt.Errorf("inserting linked institution: %w", err)
	}

	// Fetch and store accounts
	accounts, err := s.teller.GetAccounts(ctx, accessToken)
	if err != nil {
		return fmt.Errorf("fetching accounts: %w", err)
	}

	for _, acct := range accounts {
		// Fetch balances for each account
		var currentBal, availBal *float64
		balance, err := s.teller.GetAccountBalances(ctx, accessToken, acct.ID)
		if err != nil {
			log.Printf("Warning: could not fetch balances for account %s: %v", acct.ID, err)
		} else {
			if balance.Ledger != nil {
				if f, err := strconv.ParseFloat(*balance.Ledger, 64); err == nil {
					currentBal = &f
				}
			}
			if balance.Available != nil {
				if f, err := strconv.ParseFloat(*balance.Available, 64); err == nil {
					availBal = &f
				}
			}
		}

		currency := acct.Currency
		if currency == "" {
			currency = "USD"
		}

		_, err = s.db.ExecContext(ctx,
			`INSERT INTO accounts (linked_institution_id, teller_account_id, name, official_name, type, subtype, mask, current_balance, available_balance, iso_currency_code)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			 ON CONFLICT (teller_account_id) DO UPDATE SET
			   name = EXCLUDED.name, current_balance = EXCLUDED.current_balance,
			   available_balance = EXCLUDED.available_balance, updated_at = NOW()`,
			instID, acct.ID, acct.Name,
			nullStr(acct.Name), acct.Type,
			nullStr(acct.Subtype), nullStr(acct.LastFour),
			currentBal, availBal, currency,
		)
		if err != nil {
			return fmt.Errorf("upserting account %s: %w", acct.ID, err)
		}
	}

	log.Printf("Linked institution with %d accounts", len(accounts))
	return nil
}

// SyncAll syncs transactions for all linked institutions.
func (s *SyncService) SyncAll(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id, encrypted_access_token, last_synced_at FROM linked_institutions`)
	if err != nil {
		return fmt.Errorf("querying institutions: %w", err)
	}
	defer rows.Close()

	type inst struct {
		id           int
		token        string
		lastSyncedAt *time.Time
	}
	var institutions []inst
	for rows.Next() {
		var it inst
		if err := rows.Scan(&it.id, &it.token, &it.lastSyncedAt); err != nil {
			return fmt.Errorf("scanning institution: %w", err)
		}
		institutions = append(institutions, it)
	}

	for _, it := range institutions {
		if err := s.syncInstitution(ctx, it.id, it.token, it.lastSyncedAt); err != nil {
			log.Printf("Error syncing institution %d: %v", it.id, err)
			continue
		}
	}

	return nil
}

func (s *SyncService) syncInstitution(ctx context.Context, instID int, encryptedToken string, lastSyncedAt *time.Time) error {
	accessToken, err := crypto.Decrypt(encryptedToken, s.key)
	if err != nil {
		return fmt.Errorf("decrypting access token: %w", err)
	}

	// Fetch accounts for this institution
	accounts, err := s.teller.GetAccounts(ctx, accessToken)
	if err != nil {
		return fmt.Errorf("fetching accounts: %w", err)
	}

	// Determine sync date range: 10 days before last sync to catch pending→posted changes
	fromDate := ""
	if lastSyncedAt != nil {
		fromDate = lastSyncedAt.AddDate(0, 0, -10).Format("2006-01-02")
	}

	totalAdded := 0
	totalUpdated := 0

	for _, acct := range accounts {
		// Update account balances
		balance, err := s.teller.GetAccountBalances(ctx, accessToken, acct.ID)
		if err != nil {
			log.Printf("Warning: could not fetch balances for account %s: %v", acct.ID, err)
		} else {
			var currentBal, availBal *float64
			if balance.Ledger != nil {
				if f, err := strconv.ParseFloat(*balance.Ledger, 64); err == nil {
					currentBal = &f
				}
			}
			if balance.Available != nil {
				if f, err := strconv.ParseFloat(*balance.Available, 64); err == nil {
					availBal = &f
				}
			}
			_, err = s.db.ExecContext(ctx,
				`UPDATE accounts SET current_balance = $1, available_balance = $2, updated_at = NOW()
				 WHERE teller_account_id = $3`,
				currentBal, availBal, acct.ID,
			)
			if err != nil {
				log.Printf("Error updating account %s balance: %v", acct.ID, err)
			}
		}

		// Fetch transactions
		transactions, err := s.teller.GetTransactions(ctx, accessToken, acct.ID, fromDate, "", 500)
		if err != nil {
			log.Printf("Error fetching transactions for account %s: %v", acct.ID, err)
			continue
		}

		// Get internal account ID
		var accountID int
		err = s.db.QueryRowContext(ctx,
			`SELECT id FROM accounts WHERE teller_account_id = $1`, acct.ID,
		).Scan(&accountID)
		if err != nil {
			log.Printf("Account not found for teller account %s: %v", acct.ID, err)
			continue
		}

		for _, txn := range transactions {
			amount, err := strconv.ParseFloat(txn.Amount, 64)
			if err != nil {
				log.Printf("Invalid amount for transaction %s: %v", txn.ID, err)
				continue
			}

			// Teller amounts are negative for debits, positive for credits
			// Our convention: positive = expense (money out), negative = income (money in)
			// Teller: negative = money out, positive = money in
			// So we negate to match our convention
			amount = -amount

			pending := txn.Status == "pending"
			merchantName := txn.Details.Counterparty.Name
			category := txn.Details.Category

			result, err := s.db.ExecContext(ctx,
				`INSERT INTO transactions (teller_transaction_id, account_id, amount, iso_currency_code, date, name, merchant_name, category, pending)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
				 ON CONFLICT (teller_transaction_id) DO UPDATE SET
				   amount = EXCLUDED.amount, name = EXCLUDED.name, merchant_name = EXCLUDED.merchant_name,
				   category = EXCLUDED.category, pending = EXCLUDED.pending, updated_at = NOW()`,
				txn.ID, accountID, amount, "USD",
				txn.Date, txn.Description, nullStr(merchantName),
				nilIfEmpty(category), pending,
			)
			if err != nil {
				log.Printf("Error upserting transaction %s: %v", txn.ID, err)
				continue
			}

			rowsAffected, _ := result.RowsAffected()
			if rowsAffected > 0 {
				totalAdded++
			} else {
				totalUpdated++
			}
		}
	}

	// Update last synced timestamp
	now := time.Now()
	_, err = s.db.ExecContext(ctx,
		`UPDATE linked_institutions SET last_synced_at = $1, updated_at = NOW() WHERE id = $2`,
		now, instID,
	)
	if err != nil {
		return fmt.Errorf("updating last_synced_at: %w", err)
	}

	// Snapshot net worth
	if err := s.snapshotNetWorth(ctx); err != nil {
		log.Printf("Error snapshotting net worth: %v", err)
	}

	log.Printf("Synced institution %d: %d transactions processed", instID, totalAdded+totalUpdated)
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
