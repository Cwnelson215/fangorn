package services

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/cwnelson/fangorn/internal/models"
)

type TransferService struct {
	db  *sql.DB
	key string
}

func NewTransferService(db *sql.DB, encryptionKey string) *TransferService {
	return &TransferService{db: db, key: encryptionKey}
}

func (s *TransferService) InitiateTransfer(ctx context.Context, sourceAccountID, destAccountID int, amount float64, description string) (int, error) {
	if sourceAccountID == destAccountID {
		return 0, fmt.Errorf("source and destination accounts must be different")
	}
	if amount <= 0 {
		return 0, fmt.Errorf("amount must be positive")
	}

	// For now, transfers are recorded locally. Teller transfer API integration
	// can be added when available/needed.
	var transferID int
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO transfers (source_account_id, destination_account_id, amount, description, status)
		 VALUES ($1, $2, $3, $4, 'pending')
		 RETURNING id`,
		sourceAccountID, destAccountID, amount, description,
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
		 t.status, t.failure_reason, t.created_at, t.updated_at,
		 sa.name, da.name
		 FROM transfers t
		 JOIN accounts sa ON t.source_account_id = sa.id
		 JOIN accounts da ON t.destination_account_id = da.id
		 WHERE t.id = $1`, id,
	).Scan(&t.ID, &t.SourceAccountID, &t.DestinationAccountID, &t.Amount, &t.Description,
		&t.Status, &t.FailureReason, &t.CreatedAt, &t.UpdatedAt,
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
		 t.status, t.failure_reason, t.created_at, t.updated_at,
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
			&t.Status, &t.FailureReason, &t.CreatedAt, &t.UpdatedAt,
			&t.SourceAccountName, &t.DestinationAccountName); err != nil {
			return nil, fmt.Errorf("scanning transfer: %w", err)
		}
		transfers = append(transfers, t)
	}
	return transfers, nil
}

func (s *TransferService) RefreshTransferStatus(ctx context.Context, id int) (*models.Transfer, error) {
	// With Teller, transfer status refresh is a no-op for now.
	// When Teller transfer API is integrated, this will poll for status updates.
	return s.GetTransfer(ctx, id)
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
