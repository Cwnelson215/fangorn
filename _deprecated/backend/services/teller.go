package services

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/cwnelson/fangorn/internal/config"
)

const (
	tellerBaseURL    = "https://api.teller.io"
	tellerSandboxURL = "https://sandbox.teller.io"
)

type TellerService struct {
	client  *http.Client
	baseURL string
	cfg     *config.Config
}

func NewTellerService(cfg *config.Config) *TellerService {
	baseURL := tellerSandboxURL
	if cfg.TellerEnv == "production" || cfg.TellerEnv == "development" {
		baseURL = tellerBaseURL
	}

	transport := &http.Transport{}

	// Load mTLS client certificate if provided (required for production/development, optional for sandbox)
	if cfg.TellerCertPath != "" && cfg.TellerKeyPath != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TellerCertPath, cfg.TellerKeyPath)
		if err == nil {
			transport.TLSClientConfig = &tls.Config{
				Certificates: []tls.Certificate{cert},
			}
		}
	}

	return &TellerService{
		client: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
		baseURL: baseURL,
		cfg:     cfg,
	}
}

// TellerAccount represents an account from the Teller API.
type TellerAccount struct {
	ID           string            `json:"id"`
	EnrollmentID string            `json:"enrollment_id"`
	Institution  TellerInstitution `json:"institution"`
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	Subtype      string            `json:"subtype"`
	Currency     string            `json:"currency"`
	LastFour     string            `json:"last_four"`
	Status       string            `json:"status"`
}

type TellerInstitution struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TellerBalance represents account balances from the Teller API.
type TellerBalance struct {
	AccountID string  `json:"account_id"`
	Ledger    *string `json:"ledger"`
	Available *string `json:"available"`
}

// TellerTransaction represents a transaction from the Teller API.
type TellerTransaction struct {
	ID          string                 `json:"id"`
	AccountID   string                 `json:"account_id"`
	Amount      string                 `json:"amount"`
	Date        string                 `json:"date"`
	Description string                 `json:"description"`
	Status      string                 `json:"status"`
	Type        string                 `json:"type"`
	Details     TellerTransactionDetails `json:"details"`
}

type TellerTransactionDetails struct {
	Category     string            `json:"category"`
	Counterparty TellerCounterparty `json:"counterparty"`
}

type TellerCounterparty struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func (s *TellerService) doRequest(ctx context.Context, accessToken, method, path string, query url.Values) ([]byte, error) {
	u := s.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Teller uses HTTP Basic Auth: access token as username, empty password
	req.SetBasicAuth(accessToken, "")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("teller API error (status %d): %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// GetAccounts lists all accounts for an enrollment.
func (s *TellerService) GetAccounts(ctx context.Context, accessToken string) ([]TellerAccount, error) {
	body, err := s.doRequest(ctx, accessToken, http.MethodGet, "/accounts", nil)
	if err != nil {
		return nil, fmt.Errorf("getting accounts: %w", err)
	}

	var accounts []TellerAccount
	if err := json.Unmarshal(body, &accounts); err != nil {
		return nil, fmt.Errorf("parsing accounts: %w", err)
	}

	return accounts, nil
}

// GetAccountBalances retrieves balances for a specific account.
func (s *TellerService) GetAccountBalances(ctx context.Context, accessToken, accountID string) (*TellerBalance, error) {
	body, err := s.doRequest(ctx, accessToken, http.MethodGet, "/accounts/"+accountID+"/balances", nil)
	if err != nil {
		return nil, fmt.Errorf("getting balances: %w", err)
	}

	var balance TellerBalance
	if err := json.Unmarshal(body, &balance); err != nil {
		return nil, fmt.Errorf("parsing balances: %w", err)
	}

	return &balance, nil
}

// GetTransactions lists transactions for a specific account.
func (s *TellerService) GetTransactions(ctx context.Context, accessToken, accountID string, fromDate, toDate string, count int) ([]TellerTransaction, error) {
	query := url.Values{}
	if fromDate != "" {
		query.Set("from_date", fromDate)
	}
	if toDate != "" {
		query.Set("to_date", toDate)
	}
	if count > 0 {
		query.Set("count", fmt.Sprintf("%d", count))
	}

	body, err := s.doRequest(ctx, accessToken, http.MethodGet, "/accounts/"+accountID+"/transactions", query)
	if err != nil {
		return nil, fmt.Errorf("getting transactions: %w", err)
	}

	var transactions []TellerTransaction
	if err := json.Unmarshal(body, &transactions); err != nil {
		return nil, fmt.Errorf("parsing transactions: %w", err)
	}

	return transactions, nil
}
