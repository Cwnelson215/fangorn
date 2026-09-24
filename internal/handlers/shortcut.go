package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
)

// ShortcutHandler serves the iPhone Shortcut and the page that sets it up.
//
// Two halves with different auth. /api/device-keys is the app managing keys and
// sits behind the session cookie like everything else. /api/shortcut/ is what a
// Shortcut calls: the cookie middleware lets it through, and every request here
// must carry a device key instead — see requireKey.
//
// The Shortcut half answers in plain text, errors included. iOS shows a
// response body as-is in a notification and doesn't treat a 4xx as failure, so
// a sentence is what the person holding the phone should get back.
type ShortcutHandler struct {
	svc         *ledger.Service
	receipts    *ReceiptHandler
	householdID int
}

func NewShortcutHandler(svc *ledger.Service, receipts *ReceiptHandler, householdID int) *ShortcutHandler {
	return &ShortcutHandler{svc: svc, receipts: receipts, householdID: householdID}
}

func (h *ShortcutHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/device-keys", h.ListKeys)
	mux.HandleFunc("POST /api/device-keys", h.CreateKey)
	mux.HandleFunc("DELETE /api/device-keys/{id}", h.DeleteKey)

	mux.HandleFunc("GET /api/shortcut/categories", h.requireKey(h.Categories))
	mux.HandleFunc("POST /api/shortcut/log", h.requireKey(h.Log))
	mux.HandleFunc("POST /api/shortcut/receipt", h.requireKey(h.Receipt))
}

// ---- managing keys (session cookie) -----------------------------------------

func (h *ShortcutHandler) ListKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := h.svc.ListDeviceKeys(r.Context(), h.householdID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

func (h *ShortcutHandler) CreateKey(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name      string `json:"name"`
		AccountID int    `json:"account_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	key, token, err := h.svc.CreateDeviceKey(r.Context(), h.householdID, in.Name, in.AccountID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"key": key, "token": token})
}

func (h *ShortcutHandler) DeleteKey(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteDeviceKey(r.Context(), h.householdID, id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- the Shortcut (device key) ------------------------------------------------

type keyedHandler func(http.ResponseWriter, *http.Request, models.DeviceKey)

// requireKey resolves the device key from "Authorization: Bearer fgn_…". It
// applies even when APP_PASSWORD is unset: these endpoints are reachable
// without a session by design, so the key is the only thing guarding them.
func (h *ShortcutHandler) requireKey(next keyedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !found || strings.TrimSpace(token) == "" {
			writeText(w, http.StatusUnauthorized,
				"This Shortcut has no Fangorn key. Add the Authorization header from Fangorn → iPhone Shortcut.")
			return
		}
		key, err := h.svc.DeviceKeyByToken(r.Context(), strings.TrimSpace(token))
		if errors.Is(err, ledger.ErrNotFound) {
			writeText(w, http.StatusUnauthorized,
				"This Fangorn key isn't valid anymore. Make a new one in Fangorn → iPhone Shortcut.")
			return
		}
		if err != nil {
			log.Printf("shortcut key check failed: %v", err)
			writeText(w, http.StatusInternalServerError, "Fangorn couldn't check this Shortcut's key. Try again.")
			return
		}
		next(w, r, key)
	}
}

// Categories returns the names to choose from, as a JSON array — a Shortcut's
// "Choose from List" takes it as is. ?kind=income lists income categories.
func (h *ShortcutHandler) Categories(w http.ResponseWriter, r *http.Request, key models.DeviceKey) {
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = models.KindExpense
	}
	names, err := h.svc.ShortcutCategories(r.Context(), key.HouseholdID, kind)
	var invalid ledger.ErrInvalid
	if errors.As(err, &invalid) {
		writeText(w, http.StatusBadRequest, invalid.Msg)
		return
	}
	if err != nil {
		log.Printf("shortcut categories failed: %v", err)
		writeText(w, http.StatusInternalServerError, "Fangorn couldn't load your categories. Try again.")
		return
	}
	writeJSON(w, http.StatusOK, names)
}

// shortcutAmount accepts a number or a string. Which one arrives depends on how
// the JSON field was set up in the Shortcut, and people will type "$12.50".
type shortcutAmount float64

func (a *shortcutAmount) UnmarshalJSON(b []byte) error {
	var n float64
	if err := json.Unmarshal(b, &n); err == nil {
		*a = shortcutAmount(n)
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return errors.New("amount must be a number")
	}
	s = strings.NewReplacer("$", "", ",", "", " ", "").Replace(s)
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("%q isn't an amount", s)
	}
	*a = shortcutAmount(n)
	return nil
}

func (h *ShortcutHandler) Log(w http.ResponseWriter, r *http.Request, key models.DeviceKey) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var in struct {
		Amount   shortcutAmount `json:"amount"`
		Category string         `json:"category"`
		Note     string         `json:"note"`
		Refund   bool           `json:"refund"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		writeText(w, http.StatusBadRequest,
			"Fangorn couldn't read that. Check the Shortcut sends JSON with amount and category: "+err.Error())
		return
	}

	tx, err := h.svc.LogFromShortcut(r.Context(), key, ledger.ShortcutEntry{
		Amount:   float64(in.Amount),
		Category: in.Category,
		Note:     in.Note,
		Refund:   in.Refund,
	})
	var invalid ledger.ErrInvalid
	if errors.As(err, &invalid) {
		writeText(w, http.StatusBadRequest, "Not logged: "+invalid.Msg+".")
		return
	}
	if err != nil {
		log.Printf("shortcut log failed: %v", err)
		writeText(w, http.StatusInternalServerError, "Not logged: something went wrong in Fangorn. Try again.")
		return
	}
	writeText(w, http.StatusCreated, loggedMessage(tx, key))
}

// Receipt takes a photo straight from the camera as the raw request body — in
// the Shortcut, Request Body "File" — and reads it like an upload from the app.
// The key's account plays no part: which account paid is still worked out from
// the receipt, and a doubtful one waits in the app for review.
func (h *ShortcutHandler) Receipt(w http.ResponseWriter, r *http.Request, key models.DeviceKey) {
	start := time.Now()
	r.Body = http.MaxBytesReader(w, r.Body, maxReceiptBytes)
	img, err := io.ReadAll(r.Body)
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig):
		writeText(w, http.StatusRequestEntityTooLarge,
			"That photo is too big. Add a Resize Image step (width 2000) before sending it.")
		return
	case err != nil:
		writeText(w, http.StatusBadRequest, "Fangorn couldn't read the photo. Try again.")
		return
	case len(img) == 0:
		writeText(w, http.StatusBadRequest, "No photo arrived. Set the Request Body to File and pick the photo.")
		return
	}

	res, err := h.receipts.ingest(r, key.HouseholdID, img, start)
	if errors.Is(err, errNotAnImage) {
		writeText(w, http.StatusBadRequest,
			"That isn't a JPEG. Add a Convert Image step (to JPEG) before sending it.")
		return
	}
	if err != nil {
		log.Printf("shortcut receipt failed: %v", err)
		writeText(w, http.StatusInternalServerError, "Fangorn couldn't save the photo. Try again.")
		return
	}
	writeText(w, http.StatusCreated, h.receiptResult(r, key, res))
}

// receiptResult says what happened to a photo in one line for a notification.
func (h *ShortcutHandler) receiptResult(r *http.Request, key models.DeviceKey, res uploadResponse) string {
	rec := res.Receipt
	if res.Duplicate {
		return "Fangorn already had that photo."
	}
	var read []string
	if rec.Total != nil {
		read = append(read, formatUSD(*rec.Total))
	}
	if rec.Merchant != nil && *rec.Merchant != "" {
		read = append(read, "at "+*rec.Merchant)
	}
	switch rec.Status {
	case models.ReceiptPosted:
		if rec.AccountID != nil {
			if a, err := h.svc.GetAccount(r.Context(), key.HouseholdID, *rec.AccountID); err == nil {
				read = append(read, "on "+a.Name)
			}
		}
		return "Posted " + strings.Join(read, " ") + "."
	case models.ReceiptNeedsReview:
		if !res.Enabled {
			return "Saved. Enter it in Fangorn → Receipts."
		}
		if len(read) > 0 {
			return "Read " + strings.Join(read, " ") + ", but it needs a look in Fangorn → Receipts."
		}
		return "Saved, but it needs a look in Fangorn → Receipts."
	}
	return "Got it. Still reading — it'll post on its own, or wait in Fangorn → Receipts if it needs you."
}

// loggedMessage is the notification the phone shows after logging an entry.
func loggedMessage(tx models.Transaction, key models.DeviceKey) string {
	what := tx.Description
	if tx.CategoryName != nil && *tx.CategoryName != tx.Description {
		what = tx.Description + " (" + *tx.CategoryName + ")"
	}
	verb := "Logged"
	switch tx.Kind {
	case models.KindIncome:
		verb = "Logged income:"
	case models.KindRefund:
		verb = "Logged refund:"
	}
	return fmt.Sprintf("%s %s for %s on %s.", verb, formatUSD(math.Abs(tx.Amount)), what, key.AccountName)
}

// formatUSD renders 1234.5 as "$1,234.50".
func formatUSD(v float64) string {
	cents := int64(math.Round(v * 100))
	whole, frac := cents/100, cents%100
	digits := strconv.FormatInt(whole, 10)
	var b strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	return fmt.Sprintf("$%s.%02d", b.String(), frac)
}

func writeText(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintln(w, message)
}
