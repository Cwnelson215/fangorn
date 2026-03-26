package emailparse

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ChaseParser parses Chase transaction alert emails.
// Chase sends from "no.reply.alerts@chase.com" with subjects like:
// "Your $42.50 transaction with AMAZON"
// "You made a $15.00 debit card transaction"
type ChaseParser struct{}

var (
	chaseAmountRe  = regexp.MustCompile(`\$([0-9,]+\.?\d*)`)
	chaseMerchant  = regexp.MustCompile(`(?i)transaction with (.+?)(?:\s+on|\s+has|\s*$)`)
	chaseAccountRe = regexp.MustCompile(`(?i)(?:card|account)\s+(?:ending in\s+)?(\d{4})`)
)

func (p *ChaseParser) CanParse(from, subject string) bool {
	return strings.Contains(strings.ToLower(from), "chase.com")
}

func (p *ChaseParser) Parse(subject, body string) (*ParsedEmailTransaction, error) {
	combined := subject + " " + body

	// Extract amount
	amountMatch := chaseAmountRe.FindStringSubmatch(combined)
	if len(amountMatch) < 2 {
		return nil, fmt.Errorf("no amount found in Chase email")
	}
	amountStr := strings.ReplaceAll(amountMatch[1], ",", "")
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing amount: %w", err)
	}

	// Chase alerts are for charges (expenses) -> positive in our convention
	txn := &ParsedEmailTransaction{
		Amount: amount,
		Date:   time.Now().Format("2006-01-02"),
	}

	// Extract merchant
	merchantMatch := chaseMerchant.FindStringSubmatch(combined)
	if len(merchantMatch) >= 2 {
		txn.MerchantName = strings.TrimSpace(merchantMatch[1])
	}

	// Extract account hint
	accountMatch := chaseAccountRe.FindStringSubmatch(combined)
	if len(accountMatch) >= 2 {
		txn.AccountHint = accountMatch[1]
	}

	return txn, nil
}
