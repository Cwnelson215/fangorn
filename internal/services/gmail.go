package services

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/cwnelson/fangorn/internal/config"
	"github.com/cwnelson/fangorn/internal/emailparse"
)

type GmailService struct {
	db            *sql.DB
	gmailSvc      *gmail.Service
	senderFilters []string
	pollInterval  time.Duration
}

func NewGmailService(db *sql.DB, cfg *config.Config) (*GmailService, error) {
	interval, err := time.ParseDuration(cfg.GmailPollInterval)
	if err != nil {
		interval = 5 * time.Minute
	}

	oauthCfg := &oauth2.Config{
		ClientID:     cfg.GmailClientID,
		ClientSecret: cfg.GmailClientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{gmail.GmailReadonlyScope},
	}

	token := &oauth2.Token{RefreshToken: cfg.GmailRefreshToken}
	client := oauthCfg.Client(context.Background(), token)

	gmailSvc, err := gmail.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("creating Gmail service: %w", err)
	}

	return &GmailService{
		db:            db,
		gmailSvc:      gmailSvc,
		senderFilters: cfg.GmailSenderFilters,
		pollInterval:  interval,
	}, nil
}

// Start runs the Gmail polling loop until the context is cancelled.
func (s *GmailService) Start(ctx context.Context) {
	log.Printf("Gmail watcher started (polling every %s, watching %d senders)", s.pollInterval, len(s.senderFilters))

	// Initial poll
	s.poll(ctx)

	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Gmail watcher stopped")
			return
		case <-ticker.C:
			s.poll(ctx)
		}
	}
}

func (s *GmailService) poll(ctx context.Context) {
	// Get last polled time
	var lastPolled *time.Time
	var stateID int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, last_polled_at FROM gmail_watch_state ORDER BY id LIMIT 1`,
	).Scan(&stateID, &lastPolled)
	if err == sql.ErrNoRows {
		// First run: look back 1 day
		t := time.Now().Add(-24 * time.Hour)
		lastPolled = &t
		err = s.db.QueryRowContext(ctx,
			`INSERT INTO gmail_watch_state (last_polled_at, updated_at) VALUES ($1, NOW()) RETURNING id`,
			lastPolled,
		).Scan(&stateID)
		if err != nil {
			log.Printf("Gmail: error creating watch state: %v", err)
			return
		}
	} else if err != nil {
		log.Printf("Gmail: error reading watch state: %v", err)
		return
	}

	// Build search query
	query := s.buildSearchQuery(lastPolled)
	if query == "" {
		return
	}

	// List messages
	resp, err := s.gmailSvc.Users.Messages.List("me").Q(query).MaxResults(50).Context(ctx).Do()
	if err != nil {
		log.Printf("Gmail: error listing messages: %v", err)
		return
	}

	newTxns := 0
	for _, msg := range resp.Messages {
		processed, err := s.processMessage(ctx, msg.Id)
		if err != nil {
			log.Printf("Gmail: error processing message %s: %v", msg.Id, err)
			continue
		}
		if processed {
			newTxns++
		}
	}

	// Update last polled time
	now := time.Now()
	_, err = s.db.ExecContext(ctx,
		`UPDATE gmail_watch_state SET last_polled_at = $1, updated_at = NOW() WHERE id = $2`,
		now, stateID,
	)
	if err != nil {
		log.Printf("Gmail: error updating watch state: %v", err)
	}

	if newTxns > 0 {
		log.Printf("Gmail: processed %d new transactions", newTxns)
		if err := SnapshotNetWorth(ctx, s.db); err != nil {
			log.Printf("Gmail: error snapshotting net worth: %v", err)
		}
	}
}

func (s *GmailService) buildSearchQuery(lastPolled *time.Time) string {
	if len(s.senderFilters) == 0 {
		return ""
	}

	// Build "from:(a OR b OR c)" clause
	fromClause := "from:(" + strings.Join(s.senderFilters, " OR ") + ")"

	// Date filter
	dateFilter := ""
	if lastPolled != nil {
		dateFilter = " after:" + lastPolled.Format("2006/01/02")
	}

	return fromClause + dateFilter
}

func (s *GmailService) processMessage(ctx context.Context, msgID string) (bool, error) {
	// Check if already processed
	externalID := "gmail_" + msgID
	var exists int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM transactions WHERE external_id = $1`, externalID,
	).Scan(&exists)
	if err == nil {
		return false, nil // already processed
	}
	if err != sql.ErrNoRows {
		return false, err
	}

	// Fetch full message
	msg, err := s.gmailSvc.Users.Messages.Get("me", msgID).Format("full").Context(ctx).Do()
	if err != nil {
		return false, fmt.Errorf("fetching message: %w", err)
	}

	// Extract headers
	var from, subject string
	for _, h := range msg.Payload.Headers {
		switch strings.ToLower(h.Name) {
		case "from":
			from = h.Value
		case "subject":
			subject = h.Value
		}
	}

	// Find matching parser
	parser := emailparse.FindParser(from, subject)
	if parser == nil {
		return false, nil // no parser for this email
	}

	// Extract body text
	body := extractBody(msg.Payload)

	// Parse the email
	txn, err := parser.Parse(subject, body)
	if err != nil {
		return false, fmt.Errorf("parsing email: %w", err)
	}

	// Match account by mask (last 4 digits)
	accountID, err := s.matchAccount(ctx, txn.AccountHint)
	if err != nil {
		return false, fmt.Errorf("matching account: %w", err)
	}

	// Insert transaction
	name := txn.MerchantName
	if name == "" {
		name = subject
	}
	var merchantName *string
	if txn.MerchantName != "" {
		merchantName = &txn.MerchantName
	}

	date := txn.Date
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO transactions (external_id, account_id, amount, iso_currency_code, date, name, merchant_name, pending, source)
		 VALUES ($1, $2, $3, 'USD', $4, $5, $6, false, 'gmail')
		 ON CONFLICT (external_id) WHERE external_id IS NOT NULL DO NOTHING`,
		externalID, accountID, txn.Amount, date, name, merchantName,
	)
	if err != nil {
		return false, fmt.Errorf("inserting transaction: %w", err)
	}

	return true, nil
}

// matchAccount finds an account by the last-4-digits hint, or creates a fallback account.
func (s *GmailService) matchAccount(ctx context.Context, hint string) (int, error) {
	if hint != "" {
		var id int
		err := s.db.QueryRowContext(ctx,
			`SELECT id FROM accounts WHERE mask = $1 LIMIT 1`, hint,
		).Scan(&id)
		if err == nil {
			return id, nil
		}
	}

	// Fallback: use or create a "Gmail Transactions" account
	var id int
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM accounts WHERE name = 'Gmail Transactions' AND source = 'gmail' LIMIT 1`,
	).Scan(&id)
	if err == sql.ErrNoRows {
		// Create institution and account
		var instID int
		err = s.db.QueryRowContext(ctx,
			`SELECT id FROM linked_institutions WHERE institution_name = 'Gmail Import' LIMIT 1`,
		).Scan(&instID)
		if err == sql.ErrNoRows {
			err = s.db.QueryRowContext(ctx,
				`INSERT INTO linked_institutions (institution_name, encrypted_access_token)
				 VALUES ('Gmail Import', '') RETURNING id`,
			).Scan(&instID)
		}
		if err != nil {
			return 0, err
		}

		err = s.db.QueryRowContext(ctx,
			`INSERT INTO accounts (linked_institution_id, name, type, iso_currency_code, source)
			 VALUES ($1, 'Gmail Transactions', 'depository', 'USD', 'gmail') RETURNING id`,
			instID,
		).Scan(&id)
		if err != nil {
			return 0, err
		}
	} else if err != nil {
		return 0, err
	}

	return id, nil
}

// extractBody extracts plain text from a Gmail message payload.
func extractBody(payload *gmail.MessagePart) string {
	if payload.MimeType == "text/plain" && payload.Body != nil && payload.Body.Data != "" {
		data, err := base64.URLEncoding.DecodeString(payload.Body.Data)
		if err == nil {
			return string(data)
		}
	}

	// Search parts recursively
	for _, part := range payload.Parts {
		if text := extractBody(part); text != "" {
			return text
		}
	}

	// Fallback to HTML if no plain text
	if payload.MimeType == "text/html" && payload.Body != nil && payload.Body.Data != "" {
		data, err := base64.URLEncoding.DecodeString(payload.Body.Data)
		if err == nil {
			return string(data)
		}
	}

	return ""
}
