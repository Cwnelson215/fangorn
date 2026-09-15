package handlers

import (
	"net/http"

	"github.com/cwnelson/fangorn/internal/ledger"
)

// LedgerHandler serves every ledger endpoint. It is one type rather than eight
// because they all need exactly the same two things — the service and the
// household to scope to — and splitting them would only spread that plumbing out.
//
// householdID is resolved once at boot. Phase 2 replaces it with a per-request
// value read from the session; every method already takes it as a parameter, so
// that change stops at this struct.
type LedgerHandler struct {
	svc         *ledger.Service
	householdID int
}

func NewLedgerHandler(svc *ledger.Service, householdID int) *LedgerHandler {
	return &LedgerHandler{svc: svc, householdID: householdID}
}

// Register wires every ledger route onto the mux.
func (h *LedgerHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/dashboard", h.Dashboard)

	mux.HandleFunc("GET /api/accounts", h.ListAccounts)
	mux.HandleFunc("POST /api/accounts", h.CreateAccount)
	mux.HandleFunc("GET /api/accounts/{id}", h.GetAccount)
	mux.HandleFunc("PATCH /api/accounts/{id}", h.UpdateAccount)
	mux.HandleFunc("DELETE /api/accounts/{id}", h.DeleteAccount)
	mux.HandleFunc("POST /api/accounts/{id}/archive", h.ArchiveAccount)
	mux.HandleFunc("POST /api/accounts/{id}/unarchive", h.UnarchiveAccount)

	mux.HandleFunc("GET /api/categories", h.ListCategories)
	mux.HandleFunc("POST /api/categories", h.CreateCategory)
	mux.HandleFunc("PATCH /api/categories/{id}", h.UpdateCategory)
	mux.HandleFunc("DELETE /api/categories/{id}", h.DeleteCategory)
	mux.HandleFunc("POST /api/categories/{id}/unarchive", h.UnarchiveCategory)

	mux.HandleFunc("GET /api/transactions", h.ListTransactions)
	mux.HandleFunc("POST /api/transactions", h.CreateTransaction)
	mux.HandleFunc("PATCH /api/transactions/{id}", h.UpdateTransaction)
	mux.HandleFunc("DELETE /api/transactions/{id}", h.DeleteTransaction)

	mux.HandleFunc("GET /api/transfers", h.ListTransfers)
	mux.HandleFunc("POST /api/transfers", h.CreateTransfer)
	mux.HandleFunc("PATCH /api/transfers/{groupId}", h.UpdateTransfer)
	mux.HandleFunc("DELETE /api/transfers/{groupId}", h.DeleteTransfer)

	mux.HandleFunc("GET /api/recurring", h.ListRules)
	mux.HandleFunc("POST /api/recurring", h.CreateRule)
	mux.HandleFunc("GET /api/recurring/upcoming", h.Upcoming)
	mux.HandleFunc("PATCH /api/recurring/{id}", h.UpdateRule)
	mux.HandleFunc("DELETE /api/recurring/{id}", h.DeleteRule)
	mux.HandleFunc("POST /api/recurring/{id}/pause", h.PauseRule)
	mux.HandleFunc("POST /api/recurring/{id}/resume", h.ResumeRule)
	mux.HandleFunc("POST /api/recurring/{id}/skip", h.SkipRule)
	mux.HandleFunc("POST /api/recurring/{id}/post-now", h.PostRuleNow)

	mux.HandleFunc("GET /api/budgets", h.ListBudgets)
	mux.HandleFunc("POST /api/budgets", h.SetBudget)
	mux.HandleFunc("DELETE /api/budgets/{id}", h.StopBudget)

	mux.HandleFunc("GET /api/goals", h.ListGoals)
	mux.HandleFunc("POST /api/goals", h.CreateGoal)
	mux.HandleFunc("PATCH /api/goals/{id}", h.UpdateGoal)
	mux.HandleFunc("DELETE /api/goals/{id}", h.DeleteGoal)
	mux.HandleFunc("POST /api/goals/{id}/contribute", h.ContributeToGoal)
	mux.HandleFunc("POST /api/goals/{id}/achieve", h.AchieveGoal)
	mux.HandleFunc("POST /api/goals/{id}/reopen", h.ReopenGoal)
}

// ---------------------------------------------------------------------------
// dashboard
// ---------------------------------------------------------------------------

func (h *LedgerHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	data, err := h.svc.Dashboard(r.Context(), h.householdID, q.Get("from"), q.Get("to"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// ---------------------------------------------------------------------------
// accounts
// ---------------------------------------------------------------------------

func (h *LedgerHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.svc.ListAccounts(r.Context(), h.householdID, queryBool(r, "include_archived"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, accounts)
}

// GetAccount returns the account together with its register, so the detail page
// loads in one request.
func (h *LedgerHandler) GetAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	account, err := h.svc.GetAccount(r.Context(), h.householdID, id)
	if err != nil {
		fail(w, err)
		return
	}
	register, err := h.svc.Register(r.Context(), h.householdID, id, queryInt(r, "limit"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"account":      account,
		"transactions": register,
	})
}

func (h *LedgerHandler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var in ledger.AccountInput
	if !decode(w, r, &in) {
		return
	}
	account, err := h.svc.CreateAccount(r.Context(), h.householdID, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, account)
}

func (h *LedgerHandler) UpdateAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	var in ledger.AccountInput
	if !decode(w, r, &in) {
		return
	}
	account, err := h.svc.UpdateAccount(r.Context(), h.householdID, id, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, account)
}

func (h *LedgerHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteAccount(r.Context(), h.householdID, id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *LedgerHandler) ArchiveAccount(w http.ResponseWriter, r *http.Request) {
	h.setAccountArchived(w, r, true)
}

func (h *LedgerHandler) UnarchiveAccount(w http.ResponseWriter, r *http.Request) {
	h.setAccountArchived(w, r, false)
}

func (h *LedgerHandler) setAccountArchived(w http.ResponseWriter, r *http.Request, archived bool) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.SetAccountArchived(r.Context(), h.householdID, id, archived); err != nil {
		fail(w, err)
		return
	}
	account, err := h.svc.GetAccount(r.Context(), h.householdID, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, account)
}

// ---------------------------------------------------------------------------
// categories
// ---------------------------------------------------------------------------

func (h *LedgerHandler) ListCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := h.svc.ListCategories(r.Context(), h.householdID, queryBool(r, "include_archived"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, categories)
}

func (h *LedgerHandler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var in ledger.CategoryInput
	if !decode(w, r, &in) {
		return
	}
	category, err := h.svc.CreateCategory(r.Context(), h.householdID, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, category)
}

func (h *LedgerHandler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	var in ledger.CategoryInput
	if !decode(w, r, &in) {
		return
	}
	category, err := h.svc.UpdateCategory(r.Context(), h.householdID, id, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, category)
}

func (h *LedgerHandler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteCategory(r.Context(), h.householdID, id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *LedgerHandler) UnarchiveCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	category, err := h.svc.UnarchiveCategory(r.Context(), h.householdID, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, category)
}

// ---------------------------------------------------------------------------
// transactions
// ---------------------------------------------------------------------------

func (h *LedgerHandler) ListTransactions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := ledger.TransactionFilter{
		AccountID:  queryInt(r, "account_id"),
		CategoryID: queryInt(r, "category_id"),
		Kind:       q.Get("kind"),
		From:       q.Get("from"),
		To:         q.Get("to"),
		Search:     q.Get("search"),
		Limit:      queryInt(r, "limit"),
	}
	transactions, err := h.svc.ListTransactions(r.Context(), h.householdID, filter)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, transactions)
}

func (h *LedgerHandler) CreateTransaction(w http.ResponseWriter, r *http.Request) {
	var in ledger.TransactionInput
	if !decode(w, r, &in) {
		return
	}
	txn, err := h.svc.CreateTransaction(r.Context(), h.householdID, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, txn)
}

func (h *LedgerHandler) UpdateTransaction(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	var in ledger.TransactionInput
	if !decode(w, r, &in) {
		return
	}
	txn, err := h.svc.UpdateTransaction(r.Context(), h.householdID, id, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, txn)
}

func (h *LedgerHandler) DeleteTransaction(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteTransaction(r.Context(), h.householdID, id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// transfers
// ---------------------------------------------------------------------------

func (h *LedgerHandler) ListTransfers(w http.ResponseWriter, r *http.Request) {
	transfers, err := h.svc.ListTransfers(r.Context(), h.householdID, queryInt(r, "limit"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, transfers)
}

func (h *LedgerHandler) CreateTransfer(w http.ResponseWriter, r *http.Request) {
	var in ledger.TransferInput
	if !decode(w, r, &in) {
		return
	}
	transfer, err := h.svc.CreateTransfer(r.Context(), h.householdID, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, transfer)
}

func (h *LedgerHandler) UpdateTransfer(w http.ResponseWriter, r *http.Request) {
	groupID := r.PathValue("groupId")
	if groupID == "" {
		writeError(w, http.StatusBadRequest, "Invalid transfer id")
		return
	}
	var in ledger.TransferInput
	if !decode(w, r, &in) {
		return
	}
	transfer, err := h.svc.UpdateTransfer(r.Context(), h.householdID, groupID, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, transfer)
}

func (h *LedgerHandler) DeleteTransfer(w http.ResponseWriter, r *http.Request) {
	groupID := r.PathValue("groupId")
	if groupID == "" {
		writeError(w, http.StatusBadRequest, "Invalid transfer id")
		return
	}
	if err := h.svc.DeleteTransfer(r.Context(), h.householdID, groupID); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// recurring
// ---------------------------------------------------------------------------

func (h *LedgerHandler) ListRules(w http.ResponseWriter, r *http.Request) {
	rules, err := h.svc.ListRules(r.Context(), h.householdID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

func (h *LedgerHandler) Upcoming(w http.ResponseWriter, r *http.Request) {
	occurrences, err := h.svc.Upcoming(r.Context(), h.householdID, queryInt(r, "days"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, occurrences)
}

func (h *LedgerHandler) CreateRule(w http.ResponseWriter, r *http.Request) {
	var in ledger.RuleInput
	if !decode(w, r, &in) {
		return
	}
	rule, err := h.svc.CreateRule(r.Context(), h.householdID, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, rule)
}

func (h *LedgerHandler) UpdateRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	var in ledger.RuleInput
	if !decode(w, r, &in) {
		return
	}
	rule, err := h.svc.UpdateRule(r.Context(), h.householdID, id, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (h *LedgerHandler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteRule(r.Context(), h.householdID, id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *LedgerHandler) PauseRule(w http.ResponseWriter, r *http.Request)  { h.setRulePaused(w, r, true) }
func (h *LedgerHandler) ResumeRule(w http.ResponseWriter, r *http.Request) { h.setRulePaused(w, r, false) }

func (h *LedgerHandler) setRulePaused(w http.ResponseWriter, r *http.Request, paused bool) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.SetRulePaused(r.Context(), h.householdID, id, paused); err != nil {
		fail(w, err)
		return
	}
	rule, err := h.svc.GetRule(r.Context(), h.householdID, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (h *LedgerHandler) SkipRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.SkipNext(r.Context(), h.householdID, id); err != nil {
		fail(w, err)
		return
	}
	rule, err := h.svc.GetRule(r.Context(), h.householdID, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (h *LedgerHandler) PostRuleNow(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	occurrence, err := h.svc.PostNow(r.Context(), h.householdID, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, occurrence)
}

// ---------------------------------------------------------------------------
// budgets
// ---------------------------------------------------------------------------

func (h *LedgerHandler) ListBudgets(w http.ResponseWriter, r *http.Request) {
	month, err := h.svc.BudgetMonth(r.Context(), h.householdID, r.URL.Query().Get("month"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, month)
}

func (h *LedgerHandler) SetBudget(w http.ResponseWriter, r *http.Request) {
	var in ledger.BudgetInput
	if !decode(w, r, &in) {
		return
	}
	budget, err := h.svc.SetBudget(r.Context(), h.householdID, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, budget)
}

// StopBudget ends a budget from ?month= onward (default: this month). Earlier
// months keep it — see ledger.StopBudget.
func (h *LedgerHandler) StopBudget(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.StopBudget(r.Context(), h.householdID, id, r.URL.Query().Get("month")); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// goals
// ---------------------------------------------------------------------------

func (h *LedgerHandler) ListGoals(w http.ResponseWriter, r *http.Request) {
	goals, err := h.svc.ListGoals(r.Context(), h.householdID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, goals)
}

func (h *LedgerHandler) CreateGoal(w http.ResponseWriter, r *http.Request) {
	var in ledger.GoalInput
	if !decode(w, r, &in) {
		return
	}
	goal, err := h.svc.CreateGoal(r.Context(), h.householdID, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, goal)
}

func (h *LedgerHandler) UpdateGoal(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	var in ledger.GoalInput
	if !decode(w, r, &in) {
		return
	}
	goal, err := h.svc.UpdateGoal(r.Context(), h.householdID, id, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, goal)
}

func (h *LedgerHandler) DeleteGoal(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteGoal(r.Context(), h.householdID, id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *LedgerHandler) ContributeToGoal(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	var in ledger.ContributionInput
	if !decode(w, r, &in) {
		return
	}
	goal, err := h.svc.AddContribution(r.Context(), h.householdID, id, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, goal)
}

func (h *LedgerHandler) AchieveGoal(w http.ResponseWriter, r *http.Request) {
	h.setGoalAchieved(w, r, true)
}

func (h *LedgerHandler) ReopenGoal(w http.ResponseWriter, r *http.Request) {
	h.setGoalAchieved(w, r, false)
}

func (h *LedgerHandler) setGoalAchieved(w http.ResponseWriter, r *http.Request, achieved bool) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.SetGoalAchieved(r.Context(), h.householdID, id, achieved); err != nil {
		fail(w, err)
		return
	}
	goal, err := h.svc.GetGoal(r.Context(), h.householdID, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, goal)
}
