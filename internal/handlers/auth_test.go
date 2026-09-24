package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cwnelson/fangorn/internal/middleware"
)

func sessionCookie(t *testing.T, password string) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	middleware.SetSessionCookie(rec, password)
	return rec.Result().Cookies()[0]
}

func TestStatusRenewsAValidSession(t *testing.T) {
	h := NewAuthHandler("secret")
	req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	req.AddCookie(sessionCookie(t, "secret"))
	rec := httptest.NewRecorder()

	h.Status(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "fangorn_session" {
		t.Fatalf("expected the session cookie to be re-issued, got %v", cookies)
	}
	if want := int(middleware.SessionMaxAge.Seconds()); cookies[0].MaxAge != want {
		t.Errorf("MaxAge = %d, want %d", cookies[0].MaxAge, want)
	}
}

func TestStatusDoesNotIssueASessionWithoutOne(t *testing.T) {
	h := NewAuthHandler("secret")
	for name, cookie := range map[string]*http.Cookie{
		"no cookie":    nil,
		"wrong cookie": {Name: "fangorn_session", Value: "forged"},
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
			if cookie != nil {
				req.AddCookie(cookie)
			}
			rec := httptest.NewRecorder()

			h.Status(rec, req)

			if cookies := rec.Result().Cookies(); len(cookies) != 0 {
				t.Errorf("expected no cookie, got %v", cookies)
			}
		})
	}
}
