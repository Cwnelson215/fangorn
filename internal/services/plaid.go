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
	req.SetProducts([]plaid.Products{plaid.PRODUCTS_TRANSACTIONS, plaid.PRODUCTS_TRANSFER})

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

func (s *PlaidService) AuthorizeTransfer(ctx context.Context, accessToken, accountID string, transferType plaid.TransferType, amount, legalName string) (string, error) {
	user := *plaid.NewTransferAuthorizationUserInRequest(legalName)
	req := plaid.NewTransferAuthorizationCreateRequest(
		accessToken,
		accountID,
		transferType,
		plaid.TRANSFERNETWORK_ACH,
		amount,
		user,
	)
	req.SetAchClass(plaid.ACHCLASS_PPD)

	resp, _, err := s.client.PlaidApi.TransferAuthorizationCreate(ctx).TransferAuthorizationCreateRequest(*req).Execute()
	if err != nil {
		return "", fmt.Errorf("authorizing transfer: %w", err)
	}

	auth := resp.GetAuthorization()
	decision := auth.GetDecision()
	if decision != "approved" {
		rationale := auth.GetDecisionRationale()
		return "", fmt.Errorf("transfer authorization %s: %s", decision, rationale.GetDescription())
	}

	return auth.GetId(), nil
}

func (s *PlaidService) CreateTransfer(ctx context.Context, accessToken, accountID, authorizationID, amount, description string) (string, error) {
	if len(description) > 15 {
		description = description[:15]
	}
	req := plaid.NewTransferCreateRequest(accessToken, accountID, authorizationID, description)
	req.SetAmount(amount)

	resp, _, err := s.client.PlaidApi.TransferCreate(ctx).TransferCreateRequest(*req).Execute()
	if err != nil {
		return "", fmt.Errorf("creating transfer: %w", err)
	}

	transfer := resp.GetTransfer()
	return transfer.GetId(), nil
}

func (s *PlaidService) GetTransferStatus(ctx context.Context, transferID string) (string, error) {
	req := plaid.NewTransferGetRequest()
	req.SetTransferId(transferID)

	resp, _, err := s.client.PlaidApi.TransferGet(ctx).TransferGetRequest(*req).Execute()
	if err != nil {
		return "", fmt.Errorf("getting transfer: %w", err)
	}

	transfer := resp.GetTransfer()
	return string(transfer.GetStatus()), nil
}

func (s *PlaidService) SimulateTransfer(ctx context.Context, transferID, eventType string) error {
	req := plaid.NewSandboxTransferSimulateRequest(transferID, eventType)

	_, _, err := s.client.PlaidApi.SandboxTransferSimulate(ctx).SandboxTransferSimulateRequest(*req).Execute()
	if err != nil {
		return fmt.Errorf("simulating transfer: %w", err)
	}

	return nil
}
