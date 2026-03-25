package handlers

import (
	"database/sql"
	"net/http"
	"time"
)

type DashboardHandler struct {
	db *sql.DB
}

func NewDashboardHandler(db *sql.DB) *DashboardHandler {
	return &DashboardHandler{db: db}
}

func (h *DashboardHandler) Get(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from := q.Get("from")
	to := q.Get("to")

	if from == "" {
		from = time.Now().AddDate(0, -1, 0).Format("2006-01-02")
	}
	if to == "" {
		to = time.Now().Format("2006-01-02")
	}

	// Income (negative amounts in Plaid = money in)
	var income, expenses float64
	err := h.db.QueryRowContext(r.Context(),
		`SELECT COALESCE(SUM(CASE WHEN amount < 0 THEN ABS(amount) ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN amount > 0 THEN amount ELSE 0 END), 0)
		 FROM transactions WHERE date >= $1 AND date <= $2 AND pending = false`,
		from, to,
	).Scan(&income, &expenses)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to calculate totals")
		return
	}

	// Category breakdown (expenses only)
	catRows, err := h.db.QueryContext(r.Context(),
		`SELECT COALESCE(COALESCE(category, plaid_category), 'Uncategorized'), SUM(amount)
		 FROM transactions
		 WHERE date >= $1 AND date <= $2 AND amount > 0 AND pending = false
		 GROUP BY COALESCE(COALESCE(category, plaid_category), 'Uncategorized')
		 ORDER BY SUM(amount) DESC`,
		from, to,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch categories")
		return
	}
	defer catRows.Close()

	type categoryBreakdown struct {
		Category string  `json:"category"`
		Amount   float64 `json:"amount"`
	}
	var categories []categoryBreakdown
	for catRows.Next() {
		var c categoryBreakdown
		if err := catRows.Scan(&c.Category, &c.Amount); err != nil {
			continue
		}
		categories = append(categories, c)
	}
	if categories == nil {
		categories = []categoryBreakdown{}
	}

	// Net worth (latest snapshot)
	var netWorth *float64
	var totalAssets, totalLiabilities float64
	err = h.db.QueryRowContext(r.Context(),
		`SELECT total_assets, total_liabilities, net_worth FROM net_worth_snapshots
		 ORDER BY snapshot_date DESC LIMIT 1`,
	).Scan(&totalAssets, &totalLiabilities, &netWorth)
	if err == sql.ErrNoRows {
		netWorth = nil
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch net worth")
		return
	}

	// Net worth history
	nwRows, err := h.db.QueryContext(r.Context(),
		`SELECT snapshot_date, net_worth FROM net_worth_snapshots
		 ORDER BY snapshot_date ASC LIMIT 365`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch net worth history")
		return
	}
	defer nwRows.Close()

	type nwPoint struct {
		Date     string  `json:"date"`
		NetWorth float64 `json:"net_worth"`
	}
	var netWorthHistory []nwPoint
	for nwRows.Next() {
		var p nwPoint
		if err := nwRows.Scan(&p.Date, &p.NetWorth); err != nil {
			continue
		}
		netWorthHistory = append(netWorthHistory, p)
	}
	if netWorthHistory == nil {
		netWorthHistory = []nwPoint{}
	}

	resp := map[string]any{
		"from":              from,
		"to":                to,
		"income":            income,
		"expenses":          expenses,
		"net":               income - expenses,
		"categories":        categories,
		"net_worth":         netWorth,
		"total_assets":      totalAssets,
		"total_liabilities": totalLiabilities,
		"net_worth_history": netWorthHistory,
	}

	writeJSON(w, http.StatusOK, resp)
}
