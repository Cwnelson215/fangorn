package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cwnelson/fangorn/internal/models"
)

func TestShortcutAmountAcceptsNumbersAndText(t *testing.T) {
	for in, want := range map[string]float64{
		`12.5`:        12.5,
		`"12.50"`:     12.5,
		`"$1,234.56"`: 1234.56,
		`" 7 "`:       7,
	} {
		var a shortcutAmount
		if err := json.Unmarshal([]byte(in), &a); err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		if float64(a) != want {
			t.Errorf("%s = %v, want %v", in, a, want)
		}
	}
	for _, in := range []string{`"twelve"`, `true`, `{}`} {
		var a shortcutAmount
		if err := json.Unmarshal([]byte(in), &a); err == nil {
			t.Errorf("%s: expected an error, got %v", in, a)
		}
	}
}

func TestFormatUSD(t *testing.T) {
	for in, want := range map[float64]string{
		0: "$0.00", 5: "$5.00", 12.5: "$12.50", 999.999: "$1,000.00", 1234567.8: "$1,234,567.80",
	} {
		if got := formatUSD(in); got != want {
			t.Errorf("formatUSD(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestLoggedMessage(t *testing.T) {
	groceries := "Groceries"
	key := models.DeviceKey{AccountName: "Gesa Visa"}
	for _, c := range []struct {
		tx   models.Transaction
		want string
	}{
		{models.Transaction{Kind: "expense", Amount: -12.5, Description: "Groceries", CategoryName: &groceries},
			"Logged $12.50 for Groceries on Gesa Visa."},
		{models.Transaction{Kind: "expense", Amount: -3, Description: "Milk", CategoryName: &groceries},
			"Logged $3.00 for Milk (Groceries) on Gesa Visa."},
		{models.Transaction{Kind: "refund", Amount: 4, Description: "Groceries", CategoryName: &groceries},
			"Logged refund: $4.00 for Groceries on Gesa Visa."},
	} {
		if got := loggedMessage(c.tx, key); got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
}

func TestShortcutEndpointsNeedAKey(t *testing.T) {
	// No key means the handler never reaches the service, so none is needed here.
	h := &ShortcutHandler{}
	mux := http.NewServeMux()
	h.Register(mux)

	for _, header := range []string{"", "Basic abc", "Bearer ", "Bearer    "} {
		req := httptest.NewRequest(http.MethodPost, "/api/shortcut/log", strings.NewReader(`{"amount":1}`))
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "key") {
			t.Errorf("Authorization %q: %d %q, want 401 explaining the key", header, rec.Code, rec.Body.String())
		}
	}
}

func TestReceiptResult(t *testing.T) {
	h := &ShortcutHandler{} // none of these cases look up an account
	req := httptest.NewRequest(http.MethodPost, "/api/shortcut/receipt", nil)
	total, target := 42.17, "Target"
	for name, c := range map[string]struct {
		res  uploadResponse
		want string
	}{
		"duplicate": {uploadResponse{Duplicate: true}, "Fangorn already had that photo."},
		"posted without an account": {
			uploadResponse{Receipt: models.Receipt{Status: models.ReceiptPosted, Total: &total, Merchant: &target}},
			"Posted $42.17 at Target.",
		},
		"needs review": {
			uploadResponse{Enabled: true, Receipt: models.Receipt{Status: models.ReceiptNeedsReview, Total: &total}},
			"Read $42.17, but it needs a look in Fangorn → Receipts.",
		},
		"reading switched off": {
			uploadResponse{Receipt: models.Receipt{Status: models.ReceiptNeedsReview}},
			"Saved. Enter it in Fangorn → Receipts.",
		},
		"still reading": {
			uploadResponse{Enabled: true, Receipt: models.Receipt{Status: models.ReceiptProcessing}},
			"Got it. Still reading — it'll post on its own, or wait in Fangorn → Receipts if it needs you.",
		},
	} {
		if got := h.receiptResult(req, models.DeviceKey{}, c.res); got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
}
