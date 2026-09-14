package handlers

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/prices"
	"github.com/cwnelson/fangorn/internal/quotes"
)

// Every request here that reaches the price provider gets a budget well inside
// the server's 15s WriteTimeout. When it runs out the handler answers from the
// prices already stored rather than failing — a quote a few minutes old is far
// more useful than an error page.
const (
	refreshBudget = 4 * time.Second
	lookupBudget  = 5 * time.Second
)

// InvestmentHandler serves trades, holdings and security lookups. It is separate
// from LedgerHandler because it also needs the price refresher.
type InvestmentHandler struct {
	svc         *ledger.Service
	prices      *prices.Refresher
	householdID int
}

func NewInvestmentHandler(svc *ledger.Service, refresher *prices.Refresher, householdID int) *InvestmentHandler {
	return &InvestmentHandler{svc: svc, prices: refresher, householdID: householdID}
}

func (h *InvestmentHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/securities/search", h.Search)
	mux.HandleFunc("GET /api/securities/{symbol}", h.Quote)

	mux.HandleFunc("GET /api/accounts/{id}/holdings", h.Holdings)
	mux.HandleFunc("GET /api/accounts/{id}/trades", h.ListTrades)
	mux.HandleFunc("POST /api/accounts/{id}/trades", h.CreateTrade)
	mux.HandleFunc("PATCH /api/trades/{id}", h.UpdateTrade)
	mux.HandleFunc("DELETE /api/trades/{id}", h.DeleteTrade)
}

func (h *InvestmentHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, []quotes.Match{})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), refreshBudget)
	defer cancel()
	matches, err := h.prices.Search(ctx, q)
	if err != nil {
		log.Printf("security search %q: %v", q, err)
		fail(w, quotes.ErrUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, matches)
}

// Quote returns a symbol's current price, used to prefill the trade form.
func (h *InvestmentHandler) Quote(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), lookupBudget)
	defer cancel()
	sec, err := h.prices.Quote(ctx, r.PathValue("symbol"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sec)
}

// Holdings refreshes the account's stale prices (within budget), then values
// its positions. The frontend polls this while the account page is open.
func (h *InvestmentHandler) Holdings(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	symbols, err := h.svc.AccountSymbols(r.Context(), h.householdID, id)
	if err != nil {
		fail(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), refreshBudget)
	if err := h.prices.RefreshStale(ctx, symbols); err != nil {
		log.Printf("holdings for account %d: price refresh: %v", id, err)
	}
	cancel()

	holdings, err := h.svc.Holdings(r.Context(), h.householdID, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, holdings)
}

func (h *InvestmentHandler) ListTrades(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	trades, err := h.svc.ListTrades(r.Context(), h.householdID, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, trades)
}

func (h *InvestmentHandler) CreateTrade(w http.ResponseWriter, r *http.Request) {
	accountID, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	var in ledger.TradeInput
	if !decode(w, r, &in) {
		return
	}
	if !h.ensureSecurity(w, r, in.Symbol) {
		return
	}
	trade, err := h.svc.CreateTrade(r.Context(), h.householdID, accountID, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, trade)
}

func (h *InvestmentHandler) UpdateTrade(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	var in ledger.TradeInput
	if !decode(w, r, &in) {
		return
	}
	if !h.ensureSecurity(w, r, in.Symbol) {
		return
	}
	trade, err := h.svc.UpdateTrade(r.Context(), h.householdID, id, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, trade)
}

func (h *InvestmentHandler) DeleteTrade(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteTrade(r.Context(), h.householdID, id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ensureSecurity looks a new symbol up before a trade is saved against it, so a
// typo is caught with a clear message instead of creating a phantom holding.
func (h *InvestmentHandler) ensureSecurity(w http.ResponseWriter, r *http.Request, symbol string) bool {
	if ledger.NormalizeSymbol(symbol) == "" {
		return true // let the ledger report the missing field
	}
	ctx, cancel := context.WithTimeout(r.Context(), lookupBudget)
	defer cancel()
	if err := h.prices.EnsureSecurity(ctx, symbol); err != nil {
		fail(w, err)
		return false
	}
	return true
}
