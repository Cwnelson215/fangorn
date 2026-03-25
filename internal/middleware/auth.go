package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"
)

func Auth(appPassword string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If no password configured, skip auth entirely
			if appPassword == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Exempt paths
			path := r.URL.Path
			if path == "/health" || path == "/api/login" || path == "/api/auth/status" ||
				path == "/login" || strings.HasPrefix(path, "/_app/") {
				next.ServeHTTP(w, r)
				return
			}

			// Check session cookie
			cookie, err := r.Cookie("fangorn_session")
			if err != nil || !ValidSession(cookie.Value, appPassword) {
				if strings.HasPrefix(path, "/api/") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnauthorized)
					w.Write([]byte(`{"error":"unauthorized"}`))
				} else {
					http.Redirect(w, r, "/login", http.StatusFound)
				}
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func SetSessionCookie(w http.ResponseWriter, appPassword string) {
	token := makeSessionToken(appPassword)
	http.SetCookie(w, &http.Cookie{
		Name:     "fangorn_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   7 * 24 * 60 * 60, // 7 days
	})
}

func makeSessionToken(appPassword string) string {
	mac := hmac.New(sha256.New, []byte(appPassword))
	mac.Write([]byte("fangorn-session-v1"))
	return hex.EncodeToString(mac.Sum(nil))
}

func ValidSession(token, appPassword string) bool {
	expected := makeSessionToken(appPassword)
	return hmac.Equal([]byte(token), []byte(expected))
}

func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "fangorn_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}
