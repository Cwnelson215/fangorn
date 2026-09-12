package csvimport

import "io"

// ParsedTransaction represents a normalized transaction parsed from a CSV row.
type ParsedTransaction struct {
	Date         string  // "2006-01-02"
	Amount       float64 // positive = expense, negative = income (app convention)
	Name         string
	MerchantName string
	Category     string
}

// BankParser converts raw CSV data into normalized transactions.
type BankParser interface {
	BankName() string
	Parse(reader io.Reader) ([]ParsedTransaction, error)
}
