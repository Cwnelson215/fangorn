package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/cwnelson/fangorn/internal/services"
)

type PlaidHandler struct {
	plaid *services.PlaidService
	sync  *services.SyncService
}

func NewPlaidHandler(plaid *services.PlaidService, sync *services.SyncService) *PlaidHandler {
	return &PlaidHandler{plaid: plaid, sync: sync}
}

func (h *PlaidHandler) CreateLinkToken(w http.ResponseWriter, r *http.Request) {
	token, err := h.plaid.CreateLinkToken(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create link token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"link_token": token})
}

func (h *PlaidHandler) ExchangeToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PublicToken     string `json:"public_token"`
		InstitutionID   string `json:"institution_id"`
		InstitutionName string `json:"institution_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.PublicToken == "" {
		writeError(w, http.StatusBadRequest, "public_token is required")
		return
	}

	if err := h.sync.LinkItem(r.Context(), req.PublicToken, req.InstitutionID, req.InstitutionName); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to link account")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "linked"})
}
