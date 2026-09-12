package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

// Health is the liveness probe: deliberately dependency-free, so a database blip
// never gets the container restarted.
func Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Ready is the readiness probe. It touches the database, so a pod with a broken
// connection is pulled out of service without being killed and restarted.
func Ready(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.PingContext(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "db_unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}
