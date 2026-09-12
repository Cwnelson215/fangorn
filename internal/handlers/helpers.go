package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/cwnelson/fangorn/internal/ledger"
)

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// fail maps a ledger error onto an HTTP response.
//
// Validation problems carry a message written for the person using the app, so
// they are passed through. Anything else is logged in full and reported
// generically — an internal error message is not something to hand a client.
func fail(w http.ResponseWriter, err error) {
	var invalid ledger.ErrInvalid
	switch {
	case errors.Is(err, ledger.ErrNotFound):
		writeError(w, http.StatusNotFound, "Not found")
	case errors.As(err, &invalid):
		writeError(w, http.StatusBadRequest, invalid.Msg)
	default:
		log.Printf("request failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Something went wrong")
	}
}

// decode reads a JSON body, rejecting unknown fields so a typo in a field name
// fails loudly instead of being silently dropped.
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return false
	}
	return true
}

// pathInt reads an integer path parameter, e.g. the {id} in /api/accounts/{id}.
func pathInt(w http.ResponseWriter, r *http.Request, name string) (int, bool) {
	v, err := strconv.Atoi(r.PathValue(name))
	if err != nil || v <= 0 {
		writeError(w, http.StatusBadRequest, "Invalid "+name)
		return 0, false
	}
	return v, true
}

// queryInt reads an optional integer query parameter, returning 0 when absent
// or unparseable.
func queryInt(r *http.Request, name string) int {
	v, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil {
		return 0
	}
	return v
}

func queryBool(r *http.Request, name string) bool {
	return r.URL.Query().Get(name) == "true"
}
