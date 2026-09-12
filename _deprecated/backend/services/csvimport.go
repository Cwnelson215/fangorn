package services

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"log"

	"github.com/cwnelson/fangorn/internal/csvimport"
)

type CSVImportService struct {
	db *sql.DB
}

func NewCSVImportService(db *sql.DB) *CSVImportService {
	return &CSVImportService{db: db}
}

type ImportResult struct {
	Imported  int `json:"imported"`
	Skipped   int `json:"skipped"`
	AccountID int `json:"account_id"`
}

type DetectResult struct {
	Headers     []string   `json:"headers"`
	PreviewRows [][]string `json:"preview_rows"`
}

func (s *CSVImportService) Import(ctx context.Context, bankName string, accountID *int, file io.Reader, fileName string) (*ImportResult, error) {
	parser, err := csvimport.GetParser(bankName)
	if err != nil {
		return nil, err
	}

	txns, err := parser.Parse(file)
	if err != nil {
		return nil, fmt.Errorf("parsing CSV: %w", err)
	}

	// Resolve or create account
	acctID, err := s.resolveAccount(ctx, accountID, bankName)
	if err != nil {
		return nil, fmt.Errorf("resolving account: %w", err)
	}

	imported, skipped := 0, 0
	for _, txn := range txns {
		externalID := hashTransaction(acctID, txn.Date, txn.Amount, txn.Name)

		var merchantName, category *string
		if txn.MerchantName != "" {
			merchantName = &txn.MerchantName
		}
		if txn.Category != "" {
			category = &txn.Category
		}

		result, err := s.db.ExecContext(ctx,
			`INSERT INTO transactions (external_id, account_id, amount, iso_currency_code, date, name, merchant_name, category, pending, source)
			 VALUES ($1, $2, $3, 'USD', $4, $5, $6, $7, false, 'csv')
			 ON CONFLICT (external_id) WHERE external_id IS NOT NULL DO NOTHING`,
			externalID, acctID, txn.Amount, txn.Date, txn.Name, merchantName, category,
		)
		if err != nil {
			log.Printf("Error inserting CSV transaction: %v", err)
			skipped++
			continue
		}

		rows, _ := result.RowsAffected()
		if rows > 0 {
			imported++
		} else {
			skipped++
		}
	}

	// Record the import
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO csv_imports (bank_name, file_name, rows_imported, rows_skipped, account_id)
		 VALUES ($1, $2, $3, $4, $5)`,
		bankName, fileName, imported, skipped, acctID,
	)
	if err != nil {
		log.Printf("Error recording CSV import: %v", err)
	}

	// Snapshot net worth
	if err := SnapshotNetWorth(ctx, s.db); err != nil {
		log.Printf("Error snapshotting net worth after CSV import: %v", err)
	}

	return &ImportResult{Imported: imported, Skipped: skipped, AccountID: acctID}, nil
}

// DetectHeaders reads the first few rows of a CSV and returns headers + preview data.
func (s *CSVImportService) DetectHeaders(file io.Reader) (*DetectResult, error) {
	r := csv.NewReader(file)

	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("reading CSV header: %w", err)
	}

	var preview [][]string
	for i := 0; i < 5; i++ {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		preview = append(preview, record)
	}

	return &DetectResult{Headers: header, PreviewRows: preview}, nil
}

// SaveBankFormat stores a column mapping in the database and registers the parser.
func (s *CSVImportService) SaveBankFormat(ctx context.Context, mapping csvimport.ColumnMapping) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO bank_csv_formats (bank_name, date_column, amount_column, description_column, category_column, negate_amounts)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (bank_name) DO UPDATE SET
		   date_column = EXCLUDED.date_column,
		   amount_column = EXCLUDED.amount_column,
		   description_column = EXCLUDED.description_column,
		   category_column = EXCLUDED.category_column,
		   negate_amounts = EXCLUDED.negate_amounts`,
		mapping.BankName, mapping.DateCol, mapping.AmountCol, mapping.DescCol, mapping.CategoryCol, mapping.NegateAmts,
	)
	if err != nil {
		return fmt.Errorf("saving bank format: %w", err)
	}

	csvimport.RegisterParser(mapping.BankName, csvimport.NewGenericParser(mapping))
	return nil
}

// LoadBankFormats reads all saved formats from the database and registers parsers.
func (s *CSVImportService) LoadBankFormats(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT bank_name, date_column, amount_column, description_column, category_column, negate_amounts
		 FROM bank_csv_formats`)
	if err != nil {
		return fmt.Errorf("loading bank formats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var m csvimport.ColumnMapping
		var catCol sql.NullString
		if err := rows.Scan(&m.BankName, &m.DateCol, &m.AmountCol, &m.DescCol, &catCol, &m.NegateAmts); err != nil {
			return fmt.Errorf("scanning bank format: %w", err)
		}
		if catCol.Valid {
			m.CategoryCol = catCol.String
		}
		csvimport.RegisterParser(m.BankName, csvimport.NewGenericParser(m))
	}

	return rows.Err()
}

// resolveAccount returns the account ID to use. If accountID is provided, it validates it exists.
// Otherwise, it creates a new account under a "CSV Import" institution.
func (s *CSVImportService) resolveAccount(ctx context.Context, accountID *int, bankName string) (int, error) {
	if accountID != nil {
		var id int
		err := s.db.QueryRowContext(ctx, `SELECT id FROM accounts WHERE id = $1`, *accountID).Scan(&id)
		if err != nil {
			return 0, fmt.Errorf("account %d not found", *accountID)
		}
		return id, nil
	}

	// Get or create a "CSV Import" institution
	var instID int
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM linked_institutions WHERE institution_name = 'CSV Import' LIMIT 1`,
	).Scan(&instID)
	if err == sql.ErrNoRows {
		err = s.db.QueryRowContext(ctx,
			`INSERT INTO linked_institutions (institution_name, encrypted_access_token)
			 VALUES ('CSV Import', '') RETURNING id`,
		).Scan(&instID)
	}
	if err != nil {
		return 0, fmt.Errorf("getting CSV Import institution: %w", err)
	}

	// Create a new account for this bank format
	var acctID int
	acctName := bankName + " (CSV)"
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO accounts (linked_institution_id, name, type, iso_currency_code, source)
		 VALUES ($1, $2, 'depository', 'USD', 'csv') RETURNING id`,
		instID, acctName,
	).Scan(&acctID)
	if err != nil {
		return 0, fmt.Errorf("creating CSV account: %w", err)
	}

	return acctID, nil
}

func hashTransaction(accountID int, date string, amount float64, name string) string {
	data := fmt.Sprintf("%d|%s|%.2f|%s", accountID, date, amount, name)
	h := sha256.Sum256([]byte(data))
	return fmt.Sprintf("csv_%x", h[:16])
}
