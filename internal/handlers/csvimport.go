package handlers

import (
	"net/http"
	"strconv"

	"github.com/cwnelson/fangorn/internal/csvimport"
	"github.com/cwnelson/fangorn/internal/services"
)

type CSVImportHandler struct {
	svc *services.CSVImportService
}

func NewCSVImportHandler(svc *services.CSVImportService) *CSVImportHandler {
	return &CSVImportHandler{svc: svc}
}

// Upload handles POST /api/import/csv (multipart form: file, bank_name, optional account_id)
func (h *CSVImportHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB max
		writeError(w, http.StatusBadRequest, "File too large or invalid form data")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Missing file")
		return
	}
	defer file.Close()

	bankName := r.FormValue("bank_name")
	if bankName == "" {
		writeError(w, http.StatusBadRequest, "Missing bank_name")
		return
	}

	var accountID *int
	if v := r.FormValue("account_id"); v != "" {
		id, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid account_id")
			return
		}
		accountID = &id
	}

	result, err := h.svc.Import(r.Context(), bankName, accountID, file, header.Filename)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// SupportedBanks handles GET /api/import/banks
func (h *CSVImportHandler) SupportedBanks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, csvimport.SupportedBanks())
}
