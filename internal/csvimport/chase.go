package csvimport

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// ChaseCreditParser parses Chase credit card CSV exports.
// Format: Transaction Date,Post Date,Description,Category,Type,Amount
// Chase credit: negative amount = charge (expense), positive = payment/credit (income)
type ChaseCreditParser struct{}

func (p *ChaseCreditParser) BankName() string { return "chase_credit" }

func (p *ChaseCreditParser) Parse(reader io.Reader) ([]ParsedTransaction, error) {
	r := csv.NewReader(reader)
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("reading header: %w", err)
	}

	colIdx := mapColumns(header)
	dateCol := colIdx["Transaction Date"]
	descCol := colIdx["Description"]
	catCol := colIdx["Category"]
	amtCol := colIdx["Amount"]

	if dateCol < 0 || descCol < 0 || amtCol < 0 {
		return nil, fmt.Errorf("missing required columns (need Transaction Date, Description, Amount)")
	}

	var txns []ParsedTransaction
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading row: %w", err)
		}

		date, err := parseDate(record[dateCol])
		if err != nil {
			continue
		}

		amount, err := strconv.ParseFloat(strings.TrimSpace(record[amtCol]), 64)
		if err != nil {
			continue
		}

		// Chase credit: negative = charge, positive = payment
		// Our convention: positive = expense, negative = income
		// So negate the Chase amount
		amount = -amount

		category := ""
		if catCol >= 0 && catCol < len(record) {
			category = strings.TrimSpace(record[catCol])
		}

		txns = append(txns, ParsedTransaction{
			Date:     date,
			Amount:   amount,
			Name:     strings.TrimSpace(record[descCol]),
			Category: category,
		})
	}

	return txns, nil
}

// ChaseCheckingParser parses Chase checking account CSV exports.
// Format: Details,Posting Date,Description,Amount,Type,Balance,Check or Slip #
// Chase checking: negative = debit (expense), positive = credit (income)
type ChaseCheckingParser struct{}

func (p *ChaseCheckingParser) BankName() string { return "chase_checking" }

func (p *ChaseCheckingParser) Parse(reader io.Reader) ([]ParsedTransaction, error) {
	r := csv.NewReader(reader)
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("reading header: %w", err)
	}

	colIdx := mapColumns(header)
	dateCol := colIdx["Posting Date"]
	descCol := colIdx["Description"]
	amtCol := colIdx["Amount"]

	if dateCol < 0 || descCol < 0 || amtCol < 0 {
		return nil, fmt.Errorf("missing required columns (need Posting Date, Description, Amount)")
	}

	var txns []ParsedTransaction
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading row: %w", err)
		}

		date, err := parseDate(record[dateCol])
		if err != nil {
			continue
		}

		amount, err := strconv.ParseFloat(strings.TrimSpace(record[amtCol]), 64)
		if err != nil {
			continue
		}

		// Chase checking: negative = debit (expense), positive = credit (income)
		// Our convention: positive = expense, negative = income
		// So negate
		amount = -amount

		txns = append(txns, ParsedTransaction{
			Date: date,
			Amount: amount,
			Name:  strings.TrimSpace(record[descCol]),
		})
	}

	return txns, nil
}

// mapColumns returns a map of header name -> column index. Missing columns return -1.
func mapColumns(header []string) map[string]int {
	m := make(map[string]int)
	for i, h := range header {
		m[strings.TrimSpace(h)] = i
	}
	return m
}

// parseDate tries common date formats and returns YYYY-MM-DD.
func parseDate(s string) (string, error) {
	s = strings.TrimSpace(s)
	formats := []string{
		"01/02/2006",
		"1/2/2006",
		"2006-01-02",
		"01-02-2006",
		"Jan 2, 2006",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.Format("2006-01-02"), nil
		}
	}
	return "", fmt.Errorf("cannot parse date: %s", s)
}
