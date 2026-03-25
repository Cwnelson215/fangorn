package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"github.com/cwnelson/fangorn/internal/middleware"
)

type AuthHandler struct {
	appPassword string
}

func NewAuthHandler(appPassword string) *AuthHandler {
	return &AuthHandler{appPassword: appPassword}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if subtle.ConstantTimeCompare([]byte(req.Password), []byte(h.appPassword)) != 1 {
		writeError(w, http.StatusUnauthorized, "invalid password")
		return
	}

	middleware.SetSessionCookie(w, h.appPassword)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AuthHandler) Status(w http.ResponseWriter, r *http.Request) {
	if h.appPassword == "" {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "required": false})
		return
	}

	cookie, err := r.Cookie("fangorn_session")
	authenticated := err == nil && middleware.ValidSession(cookie.Value, h.appPassword)
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": authenticated, "required": true})
}
