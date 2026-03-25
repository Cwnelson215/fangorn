package models

import (
	"database/sql"
	"time"
)

type PlaidItem struct {
	ID                   int       `json:"id"`
	InstitutionID        *string   `json:"institution_id"`
	InstitutionName      *string   `json:"institution_name"`
	EncryptedAccessToken string    `json:"-"`
	Cursor               *string   `json:"-"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type Account struct {
	ID              int              `json:"id"`
	PlaidItemID     int              `json:"plaid_item_id"`
	PlaidAccountID  string           `json:"plaid_account_id"`
	Name            string           `json:"name"`
	OfficialName    *string          `json:"official_name"`
	Type            string           `json:"type"`
	Subtype         *string          `json:"subtype"`
	Mask            *string          `json:"mask"`
	CurrentBalance  *sql.NullFloat64 `json:"current_balance"`
	AvailableBalance *sql.NullFloat64 `json:"available_balance"`
	IsoCurrencyCode string           `json:"iso_currency_code"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

type Transaction struct {
	ID                 int       `json:"id"`
	PlaidTransactionID string    `json:"plaid_transaction_id"`
	AccountID          int       `json:"account_id"`
	Amount             float64   `json:"amount"`
	IsoCurrencyCode    string    `json:"iso_currency_code"`
	Date               string    `json:"date"`
	Name               string    `json:"name"`
	MerchantName       *string   `json:"merchant_name"`
	Category           *string   `json:"category"`
	PlaidCategory      *string   `json:"plaid_category"`
	Pending            bool      `json:"pending"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type NetWorthSnapshot struct {
	ID               int       `json:"id"`
	TotalAssets      float64   `json:"total_assets"`
	TotalLiabilities float64   `json:"total_liabilities"`
	NetWorth         float64   `json:"net_worth"`
	SnapshotDate     string    `json:"snapshot_date"`
	CreatedAt        time.Time `json:"created_at"`
}
