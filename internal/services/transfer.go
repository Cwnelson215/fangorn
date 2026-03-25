package services

import (
	"context"
	"database/sql"
	"fmt"

	plaid "github.com/plaid/plaid-go/v29/plaid"

	"github.com/cwnelson/fangorn/internal/crypto"
	"github.com/cwnelson/fangorn/internal/models"
)

type TransferService struct {
	db    *sql.DB
	plaid *PlaidService
	key   string
}

func NewTransferService(db *sql.DB, plaid *PlaidService, encryptionKey string) *TransferService {
	return &TransferService{db: db, plaid: plaid, key: encryptionKey}
}

type accountInfo struct {
	plaidAccountID string
	accessToken    string
}

func (s *TransferService) getAccountInfo(ctx context.Context, accountID int) (*accountInfo, error) {
	var plaidAccountID, encryptedToken string
	err := s.db.QueryRowContext(ctx,
		`SELECT a.plaid_account_id, pi.encrypted_access_token
		 FROM accounts a
		 JOIN plaid_items pi ON a.plaid_item_id = pi.id
		 WHERE a.id = $1`, accountID,
	).Scan(&plaidAccountID, &encryptedToken)
	if err != nil {
		return nil, fmt.Errorf("looking up account %d: %w", accountID, err)
	}

	accessToken, err := crypto.Decrypt(encryptedToken, s.key)
	if err != nil {
		return nil, fmt.Errorf("decrypting access token: %w", err)
	}

	return &accountInfo{plaidAccountID: plaidAccountID, accessToken: accessToken}, nil
}

func (s *TransferService) InitiateTransfer(ctx context.Context, sourceAccountID, destAccountID int, amount float64, description string) (int, error) {
	if sourceAccountID == destAccountID {
		return 0, fmt.Errorf("source and destination accounts must be different")
	}
	if amount <= 0 {
		return 0, fmt.Errorf("amount must be positive")
	}

	amountStr := fmt.Sprintf("%.2f", amount)
	legalName := "Account Holder"

	source, err := s.getAccountInfo(ctx, sourceAccountID)
	if err != nil {
		return 0, fmt.Errorf("source account: %w", err)
	}

	dest, err := s.getAccountInfo(ctx, destAccountID)
	if err != nil {
		return 0, fmt.Errorf("destination account: %w", err)
	}

	// Authorize debit from source
	debitAuthID, err := s.plaid.AuthorizeTransfer(ctx, source.accessToken, source.plaidAccountID, plaid.TRANSFERTYPE_DEBIT, amountStr, legalName)
	if err != nil {
		return 0, fmt.Errorf("debit authorization: %w", err)
	}

	// Authorize credit to destination
	creditAuthID, err := s.plaid.AuthorizeTransfer(ctx, dest.accessToken, dest.plaidAccountID, plaid.TRANSFERTYPE_CREDIT, amountStr, legalName)
	if err != nil {
		return 0, fmt.Errorf("credit authorization: %w", err)
	}

	desc := "Transfer"
	if description != "" {
		desc = description
	}

	// Create debit transfer
	debitTransferID, err := s.plaid.CreateTransfer(ctx, source.accessToken, source.plaidAccountID, debitAuthID, amountStr, desc)
	if err != nil {
		return 0, fmt.Errorf("creating debit transfer: %w", err)
	}

	// Create credit transfer
	creditTransferID, err := s.plaid.CreateTransfer(ctx, dest.accessToken, dest.plaidAccountID, creditAuthID, amountStr, desc)
	if err != nil {
		return 0, fmt.Errorf("creating credit transfer: %w", err)
	}

	// Save to database
	var transferID int
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO transfers (source_account_id, destination_account_id, amount, description, status,
		 debit_plaid_transfer_id, credit_plaid_transfer_id, debit_authorization_id, credit_authorization_id,
		 debit_status, credit_status)
		 VALUES ($1, $2, $3, $4, 'processing', $5, $6, $7, $8, 'pending', 'pending')
		 RETURNING id`,
		sourceAccountID, destAccountID, amount, description,
		debitTransferID, creditTransferID, debitAuthID, creditAuthID,
	).Scan(&transferID)
	if err != nil {
		return 0, fmt.Errorf("saving transfer: %w", err)
	}

	return transferID, nil
}

func (s *TransferService) GetTransfer(ctx context.Context, id int) (*models.Transfer, error) {
	var t models.Transfer
	err := s.db.QueryRowContext(ctx,
		`SELECT t.id, t.source_account_id, t.destination_account_id, t.amount, t.description,
		 t.status, t.debit_status, t.credit_status, t.failure_reason, t.created_at, t.updated_at,
		 sa.name, da.name
		 FROM transfers t
		 JOIN accounts sa ON t.source_account_id = sa.id
		 JOIN accounts da ON t.destination_account_id = da.id
		 WHERE t.id = $1`, id,
	).Scan(&t.ID, &t.SourceAccountID, &t.DestinationAccountID, &t.Amount, &t.Description,
		&t.Status, &t.DebitStatus, &t.CreditStatus, &t.FailureReason, &t.CreatedAt, &t.UpdatedAt,
		&t.SourceAccountName, &t.DestinationAccountName)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("transfer not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying transfer: %w", err)
	}
	return &t, nil
}

func (s *TransferService) ListTransfers(ctx context.Context, limit, offset int) ([]models.Transfer, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT t.id, t.source_account_id, t.destination_account_id, t.amount, t.description,
		 t.status, t.debit_status, t.credit_status, t.failure_reason, t.created_at, t.updated_at,
		 sa.name, da.name
		 FROM transfers t
		 JOIN accounts sa ON t.source_account_id = sa.id
		 JOIN accounts da ON t.destination_account_id = da.id
		 ORDER BY t.created_at DESC
		 LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing transfers: %w", err)
	}
	defer rows.Close()

	var transfers []models.Transfer
	for rows.Next() {
		var t models.Transfer
		if err := rows.Scan(&t.ID, &t.SourceAccountID, &t.DestinationAccountID, &t.Amount, &t.Description,
			&t.Status, &t.DebitStatus, &t.CreditStatus, &t.FailureReason, &t.CreatedAt, &t.UpdatedAt,
			&t.SourceAccountName, &t.DestinationAccountName); err != nil {
			return nil, fmt.Errorf("scanning transfer: %w", err)
		}
		transfers = append(transfers, t)
	}
	return transfers, nil
}

func (s *TransferService) RefreshTransferStatus(ctx context.Context, id int) (*models.Transfer, error) {
	var debitPlaidID, creditPlaidID *string
	err := s.db.QueryRowContext(ctx,
		`SELECT debit_plaid_transfer_id, credit_plaid_transfer_id FROM transfers WHERE id = $1`, id,
	).Scan(&debitPlaidID, &creditPlaidID)
	if err != nil {
		return nil, fmt.Errorf("looking up transfer: %w", err)
	}

	var debitStatus, creditStatus string

	if debitPlaidID != nil {
		debitStatus, err = s.plaid.GetTransferStatus(ctx, *debitPlaidID)
		if err != nil {
			return nil, fmt.Errorf("getting debit status: %w", err)
		}
	}

	if creditPlaidID != nil {
		creditStatus, err = s.plaid.GetTransferStatus(ctx, *creditPlaidID)
		if err != nil {
			return nil, fmt.Errorf("getting credit status: %w", err)
		}
	}

	// Determine overall status
	overallStatus := deriveOverallStatus(debitStatus, creditStatus)

	_, err = s.db.ExecContext(ctx,
		`UPDATE transfers SET debit_status = $1, credit_status = $2, status = $3, updated_at = NOW()
		 WHERE id = $4`,
		debitStatus, creditStatus, overallStatus, id)
	if err != nil {
		return nil, fmt.Errorf("updating transfer status: %w", err)
	}

	return s.GetTransfer(ctx, id)
}

func deriveOverallStatus(debitStatus, creditStatus string) string {
	if debitStatus == "failed" || debitStatus == "returned" ||
		creditStatus == "failed" || creditStatus == "returned" {
		return "failed"
	}
	if debitStatus == "cancelled" || creditStatus == "cancelled" {
		return "cancelled"
	}
	if debitStatus == "settled" && creditStatus == "settled" {
		return "completed"
	}
	return "processing"
}

func (s *TransferService) CancelTransfer(ctx context.Context, id int) error {
	t, err := s.GetTransfer(ctx, id)
	if err != nil {
		return err
	}
	if t.Status == "completed" || t.Status == "cancelled" || t.Status == "failed" {
		return fmt.Errorf("transfer cannot be cancelled in status: %s", t.Status)
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE transfers SET status = 'cancelled', updated_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("cancelling transfer: %w", err)
	}
	return nil
}
