package services

import (
	"context"
	"fmt"

	plaid "github.com/plaid/plaid-go/v29/plaid"

	"github.com/cwnelson/fangorn/internal/config"
)

type PlaidService struct {
	client *plaid.APIClient
	cfg    *config.Config
}

func NewPlaidService(cfg *config.Config) *PlaidService {
	plaidCfg := plaid.NewConfiguration()
	plaidCfg.AddDefaultHeader("PLAID-CLIENT-ID", cfg.PlaidClientID)
	plaidCfg.AddDefaultHeader("PLAID-SECRET", cfg.PlaidSecret)

	switch cfg.PlaidEnv {
	case "production":
		plaidCfg.UseEnvironment(plaid.Production)
	case "development":
		plaidCfg.UseEnvironment(plaid.Environment("https://development.plaid.com"))
	default:
		plaidCfg.UseEnvironment(plaid.Sandbox)
	}

	return &PlaidService{
		client: plaid.NewAPIClient(plaidCfg),
		cfg:    cfg,
	}
}

func (s *PlaidService) CreateLinkToken(ctx context.Context) (string, error) {
	req := plaid.NewLinkTokenCreateRequest(
		"Fangorn",
		"en",
		[]plaid.CountryCode{plaid.COUNTRYCODE_US},
		plaid.LinkTokenCreateRequestUser{
			ClientUserId: "user-1",
		},
	)
	req.SetProducts([]plaid.Products{plaid.PRODUCTS_TRANSACTIONS})

	resp, _, err := s.client.PlaidApi.LinkTokenCreate(ctx).LinkTokenCreateRequest(*req).Execute()
	if err != nil {
		return "", fmt.Errorf("creating link token: %w", err)
	}

	return resp.GetLinkToken(), nil
}

func (s *PlaidService) ExchangePublicToken(ctx context.Context, publicToken string) (accessToken string, itemID string, err error) {
	req := plaid.NewItemPublicTokenExchangeRequest(publicToken)

	resp, _, err := s.client.PlaidApi.ItemPublicTokenExchange(ctx).ItemPublicTokenExchangeRequest(*req).Execute()
	if err != nil {
		return "", "", fmt.Errorf("exchanging public token: %w", err)
	}

	return resp.GetAccessToken(), resp.GetItemId(), nil
}

type SyncResult struct {
	Added    []plaid.Transaction
	Modified []plaid.Transaction
	Removed  []plaid.RemovedTransaction
	Accounts []plaid.AccountBase
	Cursor   string
}

func (s *PlaidService) SyncTransactions(ctx context.Context, accessToken string, cursor string) (*SyncResult, error) {
	result := &SyncResult{}

	for {
		req := plaid.NewTransactionsSyncRequest(accessToken)
		if cursor != "" {
			req.SetCursor(cursor)
		}

		resp, _, err := s.client.PlaidApi.TransactionsSync(ctx).TransactionsSyncRequest(*req).Execute()
		if err != nil {
			return nil, fmt.Errorf("syncing transactions: %w", err)
		}

		result.Added = append(result.Added, resp.GetAdded()...)
		result.Modified = append(result.Modified, resp.GetModified()...)
		result.Removed = append(result.Removed, resp.GetRemoved()...)
		result.Accounts = resp.GetAccounts()
		cursor = resp.GetNextCursor()

		if !resp.GetHasMore() {
			break
		}
	}

	result.Cursor = cursor
	return result, nil
}

func (s *PlaidService) GetAccounts(ctx context.Context, accessToken string) ([]plaid.AccountBase, error) {
	req := plaid.NewAccountsGetRequest(accessToken)

	resp, _, err := s.client.PlaidApi.AccountsGet(ctx).AccountsGetRequest(*req).Execute()
	if err != nil {
		return nil, fmt.Errorf("getting accounts: %w", err)
	}

	return resp.GetAccounts(), nil
}
