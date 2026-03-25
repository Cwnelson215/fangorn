package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/cwnelson/fangorn/internal/models"
	"github.com/cwnelson/fangorn/internal/services"
)

type TransferHandler struct {
	svc *services.TransferService
}

func NewTransferHandler(svc *services.TransferService) *TransferHandler {
	return &TransferHandler{svc: svc}
}

func (h *TransferHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SourceAccountID      int     `json:"source_account_id"`
		DestinationAccountID int     `json:"destination_account_id"`
		Amount               float64 `json:"amount"`
		Description          string  `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SourceAccountID == 0 || req.DestinationAccountID == 0 || req.Amount <= 0 {
		writeError(w, http.StatusBadRequest, "source_account_id, destination_account_id, and positive amount are required")
		return
	}

	id, err := h.svc.InitiateTransfer(r.Context(), req.SourceAccountID, req.DestinationAccountID, req.Amount, req.Description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	transfer, err := h.svc.GetTransfer(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, transfer)
}

func (h *TransferHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	transfers, err := h.svc.ListTransfers(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if transfers == nil {
		transfers = []models.Transfer{}
	}

	writeJSON(w, http.StatusOK, transfers)
}

func (h *TransferHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid transfer id")
		return
	}

	transfer, err := h.svc.GetTransfer(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, transfer)
}

func (h *TransferHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid transfer id")
		return
	}

	transfer, err := h.svc.RefreshTransferStatus(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, transfer)
}

func (h *TransferHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid transfer id")
		return
	}

	if err := h.svc.CancelTransfer(r.Context(), id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}
