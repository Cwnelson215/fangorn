package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The /api/shortcut/ exemption is a path prefix, so a request that only starts
// with it must not reach anything else without a session.
func TestShortcutExemptionDoesNotOpenOtherRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/accounts", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("secret balances"))
	})
	mux.HandleFunc("GET /api/shortcut/categories", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("reached"))
	})
	handler := Auth("pw")(mux)

	for _, path := range []string{"/api/shortcut/../accounts", "/api/shortcut/%2e%2e/accounts"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code == http.StatusOK {
			t.Errorf("%s reached %q without a session", path, rec.Body.String())
		}
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/shortcut/categories", nil))
	if rec.Body.String() != "reached" {
		t.Errorf("the shortcut route itself was blocked: %d %q", rec.Code, rec.Body.String())
	}
}
