package models

import (
	"database/sql"
	"time"
)

type LinkedInstitution struct {
	ID                   int        `json:"id"`
	InstitutionID        *string    `json:"institution_id"`
	InstitutionName      *string    `json:"institution_name"`
	EncryptedAccessToken string     `json:"-"`
	LastSyncedAt         *time.Time `json:"last_synced_at"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type Account struct {
	ID                  int              `json:"id"`
	LinkedInstitutionID int              `json:"linked_institution_id"`
	TellerAccountID     string           `json:"teller_account_id"`
	Name                string           `json:"name"`
	OfficialName        *string          `json:"official_name"`
	Type                string           `json:"type"`
	Subtype             *string          `json:"subtype"`
	Mask                *string          `json:"mask"`
	CurrentBalance      *sql.NullFloat64 `json:"current_balance"`
	AvailableBalance    *sql.NullFloat64 `json:"available_balance"`
	IsoCurrencyCode     string           `json:"iso_currency_code"`
	UpdatedAt           time.Time        `json:"updated_at"`
}

type Transaction struct {
	ID                   int       `json:"id"`
	TellerTransactionID  string    `json:"teller_transaction_id"`
	AccountID            int       `json:"account_id"`
	Amount               float64   `json:"amount"`
	IsoCurrencyCode      string    `json:"iso_currency_code"`
	Date                 string    `json:"date"`
	Name                 string    `json:"name"`
	MerchantName         *string   `json:"merchant_name"`
	Category             *string   `json:"category"`
	Pending              bool      `json:"pending"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type Transfer struct {
	ID                     int       `json:"id"`
	SourceAccountID        int       `json:"source_account_id"`
	DestinationAccountID   int       `json:"destination_account_id"`
	Amount                 float64   `json:"amount"`
	Description            *string   `json:"description"`
	Status                 string    `json:"status"`
	TellerTransferID       *string   `json:"-"`
	FailureReason          *string   `json:"failure_reason"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
	SourceAccountName      string    `json:"source_account_name,omitempty"`
	DestinationAccountName string    `json:"destination_account_name,omitempty"`
}

type NetWorthSnapshot struct {
	ID               int       `json:"id"`
	TotalAssets      float64   `json:"total_assets"`
	TotalLiabilities float64   `json:"total_liabilities"`
	NetWorth         float64   `json:"net_worth"`
	SnapshotDate     string    `json:"snapshot_date"`
	CreatedAt        time.Time `json:"created_at"`
}
