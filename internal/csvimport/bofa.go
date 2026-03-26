package csvimport

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// BofAParser parses Bank of America CSV exports.
// Format: Date,Description,Amount,Running Bal.
// BofA: negative = debit (expense), positive = credit (income)
type BofAParser struct{}

func (p *BofAParser) BankName() string { return "bofa" }

func (p *BofAParser) Parse(reader io.Reader) ([]ParsedTransaction, error) {
	r := csv.NewReader(reader)
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("reading header: %w", err)
	}

	colIdx := mapColumns(header)
	dateCol := colIdx["Date"]
	descCol := colIdx["Description"]
	amtCol := colIdx["Amount"]

	if dateCol < 0 || descCol < 0 || amtCol < 0 {
		return nil, fmt.Errorf("missing required columns (need Date, Description, Amount)")
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

		// BofA: negative = debit (expense), positive = credit (income)
		// Our convention: positive = expense, negative = income
		// So negate
		amount = -amount

		txns = append(txns, ParsedTransaction{
			Date:   date,
			Amount: amount,
			Name:   strings.TrimSpace(record[descCol]),
		})
	}

	return txns, nil
}
