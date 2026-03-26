package csvimport

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ColumnMapping defines how to parse a bank's CSV format.
type ColumnMapping struct {
	BankName    string `json:"bank_name"`
	DateCol     string `json:"date_column"`
	AmountCol   string `json:"amount_column"`
	DescCol     string `json:"description_column"`
	CategoryCol string `json:"category_column,omitempty"`
	NegateAmts  bool   `json:"negate_amounts"`
}

// GenericParser parses any CSV using a stored column mapping.
type GenericParser struct {
	mapping ColumnMapping
}

func NewGenericParser(mapping ColumnMapping) *GenericParser {
	return &GenericParser{mapping: mapping}
}

func (p *GenericParser) BankName() string { return p.mapping.BankName }

func (p *GenericParser) Parse(reader io.Reader) ([]ParsedTransaction, error) {
	r := csv.NewReader(reader)
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("reading header: %w", err)
	}

	colIdx := mapColumns(header)

	dateCol, ok := colIdx[p.mapping.DateCol]
	if !ok {
		return nil, fmt.Errorf("date column %q not found in CSV headers", p.mapping.DateCol)
	}
	amtCol, ok := colIdx[p.mapping.AmountCol]
	if !ok {
		return nil, fmt.Errorf("amount column %q not found in CSV headers", p.mapping.AmountCol)
	}
	descCol, ok := colIdx[p.mapping.DescCol]
	if !ok {
		return nil, fmt.Errorf("description column %q not found in CSV headers", p.mapping.DescCol)
	}

	catCol := -1
	if p.mapping.CategoryCol != "" {
		if idx, ok := colIdx[p.mapping.CategoryCol]; ok {
			catCol = idx
		}
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

		if p.mapping.NegateAmts {
			amount = -amount
		}

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
