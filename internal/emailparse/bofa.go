package emailparse

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// BofAParser parses Bank of America transaction alert emails.
// BofA sends from "ealerts@bankofamerica.com" with subjects like:
// "Debit card transaction" or "Credit card transaction"
type BofAParser struct{}

var (
	bofaAmountRe  = regexp.MustCompile(`\$([0-9,]+\.?\d*)`)
	bofaMerchant  = regexp.MustCompile(`(?i)(?:at|from|to)\s+([A-Z0-9][A-Z0-9\s&'*#.-]{2,30})`)
	bofaAccountRe = regexp.MustCompile(`(?i)(?:card|account)\s+(?:ending in\s+)?(\d{4})`)
)

func (p *BofAParser) CanParse(from, subject string) bool {
	return strings.Contains(strings.ToLower(from), "bankofamerica.com")
}

func (p *BofAParser) Parse(subject, body string) (*ParsedEmailTransaction, error) {
	combined := subject + " " + body

	// Extract amount
	amountMatch := bofaAmountRe.FindStringSubmatch(combined)
	if len(amountMatch) < 2 {
		return nil, fmt.Errorf("no amount found in BofA email")
	}
	amountStr := strings.ReplaceAll(amountMatch[1], ",", "")
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing amount: %w", err)
	}

	txn := &ParsedEmailTransaction{
		Amount: amount,
		Date:   time.Now().Format("2006-01-02"),
	}

	// Extract merchant
	merchantMatch := bofaMerchant.FindStringSubmatch(body)
	if len(merchantMatch) >= 2 {
		txn.MerchantName = strings.TrimSpace(merchantMatch[1])
	}

	// Extract account hint
	accountMatch := bofaAccountRe.FindStringSubmatch(combined)
	if len(accountMatch) >= 2 {
		txn.AccountHint = accountMatch[1]
	}

	return txn, nil
}
