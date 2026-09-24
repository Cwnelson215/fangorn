package quotes

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The fixtures in testdata are real Yahoo responses captured on 2026-09-14, a
// Monday: FZROX before its NAV for the day was out, VOO after the close.

func serve(t *testing.T, routes map[string]string) *Yahoo {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ua := r.Header.Get("User-Agent"); ua != yahooUserAgent {
			t.Errorf("User-Agent = %q, want %q", ua, yahooUserAgent)
		}
		fixture, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		status := http.StatusOK
		if strings.HasPrefix(fixture, "status:") {
			// "status:429:body" — an error response with a literal body.
			parts := strings.SplitN(fixture, ":", 3)
			switch parts[1] {
			case "404":
				status = http.StatusNotFound
			case "429":
				status = http.StatusTooManyRequests
			}
			w.WriteHeader(status)
			w.Write([]byte(parts[2]))
			return
		}
		body, err := os.ReadFile(filepath.Join("testdata", fixture))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(fixture, "notfound") {
			status = http.StatusNotFound
		}
		w.WriteHeader(status)
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return newYahooAt(srv.URL)
}

func approx(t *testing.T, what string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

func TestQuoteMutualFund(t *testing.T) {
	y := serve(t, map[string]string{"/v8/finance/chart/FZROX": "chart_fzrox.json"})

	q, err := y.Quote(context.Background(), "fzrox")
	if err != nil {
		t.Fatal(err)
	}
	if q.Symbol != "FZROX" || q.QuoteType != "MUTUALFUND" || q.Currency != "USD" {
		t.Errorf("identity = %+v", q)
	}
	if q.Name != "Fidelity ZERO Total Market Index" {
		t.Errorf("name = %q, want the long name", q.Name)
	}
	approx(t, "price", q.Price, 26.71, 1e-9)
	// Friday's NAV of 26.71 was +0.831% on Thursday's 26.49. The chart's
	// chartPreviousClose (26.96) is the close before the range, and must not be used.
	approx(t, "previous close", q.PreviousClose, 26.49, 0.005)

	// Monday's bar has a null close (no NAV yet) and is dropped.
	if len(q.Closes) != 4 {
		t.Fatalf("closes = %d, want 4", len(q.Closes))
	}
	last := q.Closes[len(q.Closes)-1]
	if got := last.Date.Format("2006-01-02"); got != "2026-09-11" {
		t.Errorf("last close dated %s, want 2026-09-11", got)
	}
}

func TestQuoteETFAfterClose(t *testing.T) {
	y := serve(t, map[string]string{"/v8/finance/chart/VOO": "chart_voo_history.json"})

	q, err := y.Quote(context.Background(), "VOO")
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "price", q.Price, 699.3, 1e-9)
	approx(t, "previous close", q.PreviousClose, 702.56, 0.005)
	if len(q.Closes) != 5 {
		t.Fatalf("closes = %d, want 5", len(q.Closes))
	}
	if got := q.PriceTime; !got.Equal(time.Date(2026, 9, 14, 20, 0, 0, 0, time.UTC)) {
		t.Errorf("price time = %s, want 16:00 Eastern", got)
	}
}

func TestQuoteUnknownSymbol(t *testing.T) {
	y := serve(t, map[string]string{"/v8/finance/chart/NOPE": "chart_notfound.json"})
	if _, err := y.Quote(context.Background(), "NOPE"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

// A rate limit is an outage, not an unknown symbol — reporting it as "no such
// symbol" would stop someone logging a perfectly good trade.
func TestRateLimitIsNotNotFound(t *testing.T) {
	y := serve(t, map[string]string{"/v8/finance/chart/VOO": "status:429:Too Many Requests"})
	_, err := y.Quote(context.Background(), "VOO")
	if !errors.Is(err, ErrUnavailable) || errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
}

func TestTimeoutIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := newYahooAt(srv.URL).Quote(ctx, "VOO")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("Quote did not honour the context deadline")
	}
}

func TestHistory(t *testing.T) {
	y := serve(t, map[string]string{"/v8/finance/chart/VOO": "chart_voo_history.json"})
	closes, err := y.History(context.Background(), "VOO", time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(closes) != 5 || closes[0].Date.Format("2006-01-02") != "2026-09-08" {
		t.Fatalf("closes = %+v", closes)
	}
	approx(t, "first close", closes[0].Price, 704.07, 0.005)
}

func TestSearchKeepsHoldableTypes(t *testing.T) {
	y := serve(t, map[string]string{"/v1/finance/search": "search_fidelity.json"})
	matches, err := y.Search(context.Background(), "fidelity zero")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 || matches[0].Symbol != "FZROX" || matches[0].QuoteType != "MUTUALFUND" {
		t.Fatalf("matches = %+v", matches)
	}
	var sawFZILX bool
	for _, m := range matches {
		if m.Symbol == "FZILX" {
			sawFZILX = true
		}
	}
	if !sawFZILX {
		t.Error("FZILX missing from results")
	}
}

func TestPreviousCloseFallbacks(t *testing.T) {
	prev := 10.0
	approx(t, "from previousClose", previousClose(11, nil, &prev, nil), 10, 1e-9)
	recent := []Close{{Price: 9}, {Price: 11}}
	approx(t, "from closes", previousClose(11, nil, nil, recent), 9, 1e-9)
	approx(t, "nothing", previousClose(11, nil, nil, nil), 11, 1e-9)
}

// Search says MONEY_MARKET where the chart says MONEYMARKET. SPAXX, the core
// cash position in a Fidelity account, used to be filtered out by it.
func TestSearchKeepsMoneyMarketFunds(t *testing.T) {
	y := serve(t, map[string]string{"/v1/finance/search": "search_spaxx.json"})
	matches, err := y.Search(context.Background(), "SPAXX")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].Symbol != "SPAXX" || matches[0].QuoteType != "MONEYMARKET" {
		t.Fatalf("matches = %+v", matches)
	}
}
