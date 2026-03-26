package handlers

import (
	"encoding/json"
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

// DetectHeaders handles POST /api/import/csv/detect (multipart form: file)
// Returns the CSV headers and a few preview rows so the frontend can show a column mapping UI.
func (h *CSVImportHandler) DetectHeaders(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "File too large or invalid form data")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Missing file")
		return
	}
	defer file.Close()

	result, err := h.svc.DetectHeaders(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// SaveBankFormat handles POST /api/import/csv/format (JSON body: column mapping)
// Saves a new bank CSV format so it can be reused for future imports.
func (h *CSVImportHandler) SaveBankFormat(w http.ResponseWriter, r *http.Request) {
	var mapping csvimport.ColumnMapping
	if err := json.NewDecoder(r.Body).Decode(&mapping); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	if mapping.BankName == "" || mapping.DateCol == "" || mapping.AmountCol == "" || mapping.DescCol == "" {
		writeError(w, http.StatusBadRequest, "bank_name, date_column, amount_column, and description_column are required")
		return
	}

	if err := h.svc.SaveBankFormat(r.Context(), mapping); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
