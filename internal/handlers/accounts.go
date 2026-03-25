package handlers

import (
	"database/sql"
	"net/http"
)

type AccountsHandler struct {
	db *sql.DB
}

func NewAccountsHandler(db *sql.DB) *AccountsHandler {
	return &AccountsHandler{db: db}
}

func (h *AccountsHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(),
		`SELECT a.id, a.plaid_account_id, a.name, a.official_name, a.type, a.subtype,
		        a.mask, a.current_balance, a.available_balance, a.iso_currency_code,
		        pi.institution_name
		 FROM accounts a
		 JOIN plaid_items pi ON a.plaid_item_id = pi.id
		 ORDER BY a.type, a.name`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch accounts")
		return
	}
	defer rows.Close()

	type accountResp struct {
		ID              int      `json:"id"`
		PlaidAccountID  string   `json:"plaid_account_id"`
		Name            string   `json:"name"`
		OfficialName    *string  `json:"official_name"`
		Type            string   `json:"type"`
		Subtype         *string  `json:"subtype"`
		Mask            *string  `json:"mask"`
		CurrentBalance  *float64 `json:"current_balance"`
		AvailableBalance *float64 `json:"available_balance"`
		IsoCurrencyCode string   `json:"iso_currency_code"`
		InstitutionName *string  `json:"institution_name"`
	}

	var accounts []accountResp
	for rows.Next() {
		var a accountResp
		if err := rows.Scan(&a.ID, &a.PlaidAccountID, &a.Name, &a.OfficialName,
			&a.Type, &a.Subtype, &a.Mask, &a.CurrentBalance, &a.AvailableBalance,
			&a.IsoCurrencyCode, &a.InstitutionName); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to scan account")
			return
		}
		accounts = append(accounts, a)
	}

	if accounts == nil {
		accounts = []accountResp{}
	}
	writeJSON(w, http.StatusOK, accounts)
}
