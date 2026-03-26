package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
)

type TransactionsHandler struct {
	db *sql.DB
}

func NewTransactionsHandler(db *sql.DB) *TransactionsHandler {
	return &TransactionsHandler{db: db}
}

func (h *TransactionsHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset, _ := strconv.Atoi(q.Get("offset"))

	query := `SELECT t.id, t.external_id, t.account_id, t.amount,
	                 t.iso_currency_code, t.date, t.name, t.merchant_name,
	                 t.category, t.pending, t.source, a.name as account_name
	          FROM transactions t
	          JOIN accounts a ON t.account_id = a.id
	          WHERE 1=1`
	args := []any{}
	argIdx := 1

	if v := q.Get("account_id"); v != "" {
		query += " AND t.account_id = $" + strconv.Itoa(argIdx)
		id, _ := strconv.Atoi(v)
		args = append(args, id)
		argIdx++
	}
	if v := q.Get("from"); v != "" {
		query += " AND t.date >= $" + strconv.Itoa(argIdx)
		args = append(args, v)
		argIdx++
	}
	if v := q.Get("to"); v != "" {
		query += " AND t.date <= $" + strconv.Itoa(argIdx)
		args = append(args, v)
		argIdx++
	}
	if v := q.Get("category"); v != "" {
		query += " AND t.category = $" + strconv.Itoa(argIdx)
		args = append(args, v)
		argIdx++
	}
	if v := q.Get("search"); v != "" {
		query += " AND (t.name ILIKE $" + strconv.Itoa(argIdx) + " OR t.merchant_name ILIKE $" + strconv.Itoa(argIdx) + ")"
		args = append(args, "%"+v+"%")
		argIdx++
	}

	query += " ORDER BY t.date DESC, t.id DESC"
	query += " LIMIT $" + strconv.Itoa(argIdx) + " OFFSET $" + strconv.Itoa(argIdx+1)
	args = append(args, limit, offset)

	rows, err := h.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch transactions")
		return
	}
	defer rows.Close()

	type txnResp struct {
		ID              int     `json:"id"`
		ExternalID      *string `json:"external_id"`
		AccountID       int     `json:"account_id"`
		Amount          float64 `json:"amount"`
		IsoCurrencyCode string  `json:"iso_currency_code"`
		Date            string  `json:"date"`
		Name            string  `json:"name"`
		MerchantName    *string `json:"merchant_name"`
		Category        *string `json:"category"`
		Pending         bool    `json:"pending"`
		Source          string  `json:"source"`
		AccountName     string  `json:"account_name"`
	}

	var transactions []txnResp
	for rows.Next() {
		var t txnResp
		if err := rows.Scan(&t.ID, &t.ExternalID, &t.AccountID, &t.Amount,
			&t.IsoCurrencyCode, &t.Date, &t.Name, &t.MerchantName,
			&t.Category, &t.Pending, &t.Source, &t.AccountName); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to scan transaction")
			return
		}
		transactions = append(transactions, t)
	}

	if transactions == nil {
		transactions = []txnResp{}
	}
	writeJSON(w, http.StatusOK, transactions)
}
