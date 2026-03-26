package emailparse

// ParsedEmailTransaction represents financial data extracted from a transaction notification email.
type ParsedEmailTransaction struct {
	Amount       float64 // positive = expense, negative = income (app convention)
	MerchantName string
	Date         string // "2006-01-02", empty if not parseable
	AccountHint  string // e.g. "1234" (last 4 digits)
}

// EmailParser extracts transaction info from bank notification emails.
type EmailParser interface {
	// CanParse returns true if this parser can handle emails from the given sender/subject.
	CanParse(from, subject string) bool
	// Parse extracts transaction data from the email subject and body.
	Parse(subject, body string) (*ParsedEmailTransaction, error)
}

// registry holds all registered email parsers.
var registry []EmailParser

func init() {
	registry = []EmailParser{
		&ChaseParser{},
		&BofAParser{},
	}
}

// FindParser returns the first parser that can handle the given email.
func FindParser(from, subject string) EmailParser {
	for _, p := range registry {
		if p.CanParse(from, subject) {
			return p
		}
	}
	return nil
}
