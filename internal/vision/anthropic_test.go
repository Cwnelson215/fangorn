package vision

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The fixtures in testdata are hand-written in the Messages API's response
// shape. No real receipt photo is committed anywhere: the image the tests send
// is a few fake bytes, and the published container image must never carry a
// family's receipts.

var fakeImage = []byte("\xff\xd8\xff\xe0 not really a jpeg")

// serve answers every request with the fixture (or "status:NNN:file") and checks
// the request on the way in.
func serve(t *testing.T, fixture string) *Anthropic {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checkRequest(t, r)
		status := http.StatusOK
		if strings.HasPrefix(fixture, "status:") {
			parts := strings.SplitN(fixture, ":", 3)
			switch parts[1] {
			case "400":
				status = http.StatusBadRequest
			case "401":
				status = http.StatusUnauthorized
			case "429":
				status = http.StatusTooManyRequests
			case "529":
				status = 529
			}
			fixture = parts[2]
		}
		body, err := os.ReadFile(filepath.Join("testdata", fixture))
		if err != nil {
			// Not t.Fatal: this runs on the server's goroutine, and a handler
			// that exits without answering leaves the client waiting.
			t.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return newAnthropicAt(srv.URL, "test-key", "claude-opus-5")
}

func checkRequest(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Method != http.MethodPost || r.URL.Path != "/v1/messages" {
		t.Errorf("request = %s %s, want POST /v1/messages", r.Method, r.URL.Path)
	}
	for header, want := range map[string]string{
		"x-api-key":         "test-key",
		"anthropic-version": "2023-06-01",
		"anthropic-beta":    "server-side-fallback-2026-07-01",
		"content-type":      "application/json",
	} {
		if got := r.Header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}

	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Error(err)
		return
	}
	var req struct {
		Model        string `json:"model"`
		MaxTokens    int    `json:"max_tokens"`
		Fallbacks    string `json:"fallbacks"`
		OutputConfig struct {
			Effort string `json:"effort"`
			Format struct {
				Type   string          `json:"type"`
				Schema json.RawMessage `json:"schema"`
			} `json:"format"`
		} `json:"output_config"`
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Type   string `json:"type"`
				Text   string `json:"text"`
				Source struct {
					Type      string `json:"type"`
					MediaType string `json:"media_type"`
					Data      string `json:"data"`
				} `json:"source"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Errorf("request body is not JSON: %v", err)
		return
	}
	if req.Model != "claude-opus-5" || req.MaxTokens <= 0 || req.Fallbacks != "default" {
		t.Errorf("model/max_tokens/fallbacks = %q/%d/%q", req.Model, req.MaxTokens, req.Fallbacks)
	}
	if req.OutputConfig.Effort != "low" || req.OutputConfig.Format.Type != "json_schema" || len(req.OutputConfig.Format.Schema) == 0 {
		t.Errorf("output_config = %+v", req.OutputConfig)
	}
	if len(req.Messages) != 1 || len(req.Messages[0].Content) != 2 {
		t.Errorf("want one user message with two blocks, got %+v", req.Messages)
		return
	}
	img, text := req.Messages[0].Content[0], req.Messages[0].Content[1]
	if img.Type != "image" || text.Type != "text" {
		t.Errorf("block order = %s, %s; the image must come before the text", img.Type, text.Type)
	}
	if img.Source.Type != "base64" || img.Source.MediaType != "image/jpeg" {
		t.Errorf("image source = %+v", img.Source)
	}
	if strings.ContainsAny(img.Source.Data, "\r\n") {
		t.Error("base64 image data contains newlines")
	}
	if got, _ := base64.StdEncoding.DecodeString(img.Source.Data); string(got) != string(fakeImage) {
		t.Error("image data did not round-trip")
	}
	if !strings.Contains(text.Text, "2026-09-21") || !strings.Contains(text.Text, "- Groceries") {
		t.Errorf("prompt is missing today or the category list: %q", text.Text)
	}
}

func extract(a *Anthropic) (Extraction, Meta, error) {
	return a.Extract(context.Background(), fakeImage, "image/jpeg",
		Hints{Today: "2026-09-21", Categories: []string{"Groceries", "Dining Out"}})
}

func TestSchemaIsValidJSON(t *testing.T) {
	var v map[string]any
	if err := json.Unmarshal([]byte(receiptSchema), &v); err != nil {
		t.Fatalf("receiptSchema: %v", err)
	}
}

func TestExtract(t *testing.T) {
	x, meta, err := extract(serve(t, "messages_ok.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !x.IsReceipt || x.Merchant == nil || *x.Merchant != "King Soopers" {
		t.Errorf("merchant = %v", x.Merchant)
	}
	if x.Total == nil || *x.Total != 84.2 || x.Tip != nil {
		t.Errorf("total/tip = %v/%v", x.Total, x.Tip)
	}
	if x.CardLast4 == nil || *x.CardLast4 != "4821" || x.Tender != "card" {
		t.Errorf("card = %v %q", x.CardLast4, x.Tender)
	}
	if len(x.LineItems) != 2 || x.LineItems[1].Quantity != nil {
		t.Errorf("line items = %+v", x.LineItems)
	}
	if meta.Model != "claude-opus-5" || meta.InputTokens != 4912 || meta.OutputTokens != 311 {
		t.Errorf("meta = %+v", meta)
	}
}

// A refusal and a truncated answer both arrive as 200s. Neither will go better
// on a retry, so both go to a person.
func TestStopReasonsAreUnreadable(t *testing.T) {
	for _, fixture := range []string{"messages_refusal.json", "messages_max_tokens.json"} {
		_, _, err := extract(serve(t, fixture))
		if !errors.Is(err, ErrUnreadable) {
			t.Errorf("%s: want ErrUnreadable, got %v", fixture, err)
		}
	}
}

// Overload and rate limits clear on their own; a rejected request does not. The
// distinction decides whether a receipt is retried or handed to a person.
func TestStatusClassification(t *testing.T) {
	cases := map[string]error{
		"status:529:error_overloaded.json": ErrUnavailable,
		"status:429:error_overloaded.json": ErrUnavailable,
		"status:401:error_invalid.json":    ErrUnavailable,
		"status:400:error_invalid.json":    ErrUnreadable,
	}
	for fixture, want := range cases {
		_, _, err := extract(serve(t, fixture))
		if !errors.Is(err, want) {
			t.Errorf("%s: want %v, got %v", fixture, want, err)
		}
	}
}

func TestTimeoutIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The server only notices the client hanging up once the request body
		// has been consumed, so read it before waiting.
		io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _, err := newAnthropicAt(srv.URL, "k", "m").Extract(ctx, fakeImage, "image/jpeg", Hints{})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("Extract did not honour the context deadline")
	}
}
