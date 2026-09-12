package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/cwnelson/fangorn/internal/services"
)

type LinkHandler struct {
	sync *services.SyncService
}

func NewLinkHandler(sync *services.SyncService) *LinkHandler {
	return &LinkHandler{sync: sync}
}

func (h *LinkHandler) LinkAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccessToken     string `json:"access_token"`
		EnrollmentID    string `json:"enrollment_id"`
		InstitutionName string `json:"institution_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.AccessToken == "" {
		writeError(w, http.StatusBadRequest, "access_token is required")
		return
	}

	if err := h.sync.LinkInstitution(r.Context(), req.AccessToken, req.EnrollmentID, req.InstitutionName); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to link account")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "linked"})
}
