package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cwnelson/fangorn/internal/assistant"
	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/middleware"
	"github.com/cwnelson/fangorn/internal/models"
	"github.com/cwnelson/fangorn/internal/testdb"
)

func sseEvents(events ...string) string {
	var b strings.Builder
	for _, e := range events {
		fmt.Fprintf(&b, "data: %s\n\n", e)
	}
	return b.String()
}

// TestChatSendStreamsAndSaves runs a whole turn through the real middleware
// against a fake Messages API: a lookup that reads the ledger, then an answer
// that arrives after the server's WriteTimeout has passed.
func TestChatSendStreamsAndSaves(t *testing.T) {
	db := testdb.Open(t)
	svc := ledger.New(db)
	hh := testdb.Household(t, db, "America/Denver")
	ctx := context.Background()
	if _, err := svc.CreateAccount(ctx, hh, ledger.AccountInput{
		Name: "Checking", Type: models.AccountChecking, StartingBalance: 1000, StartingBalanceDate: "2026-01-01",
	}); err != nil {
		t.Fatal(err)
	}

	calls := 0
	var secondRequest map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		calls++
		w.Header().Set("content-type", "text/event-stream")
		switch calls {
		case 1:
			io.WriteString(w, sseEvents(
				`{"type":"message_start","message":{"model":"claude-opus-5"}}`,
				`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"list_accounts","input":{}}}`,
				`{"type":"content_block_stop","index":0}`,
				`{"type":"message_delta","delta":{"stop_reason":"tool_use"}}`,
				`{"type":"message_stop"}`,
			))
		case 2:
			json.Unmarshal(body, &secondRequest)
			// Longer than the server's WriteTimeout below.
			time.Sleep(1500 * time.Millisecond)
			io.WriteString(w, sseEvents(
				`{"type":"message_start","message":{"model":"claude-opus-5"}}`,
				`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"You have "}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"$1,000.00."}}`,
				`{"type":"content_block_stop","index":0}`,
				`{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
				`{"type":"message_stop"}`,
			))
		default:
			t.Errorf("unexpected call %d", calls)
		}
	}))
	defer api.Close()

	a := assistant.New(svc, "key", "", "claude-opus-5").WithBaseURL(api.URL)
	mux := http.NewServeMux()
	NewChatHandler(svc, a, hh).Register(mux)
	srv := httptest.NewUnstartedServer(middleware.Logging(mux))
	srv.Config.WriteTimeout = time.Second
	srv.Start()
	defer srv.Close()

	chat, err := svc.CreateChat(ctx, hh)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(fmt.Sprintf("%s/api/chats/%d/messages", srv.URL, chat.ID),
		"application/json", strings.NewReader(`{"text":"How much is in checking?"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}

	var events []string
	var text strings.Builder
	sc := bufio.NewScanner(resp.Body)
	var name string
	for sc.Scan() {
		line := sc.Text()
		if v, ok := strings.CutPrefix(line, "event: "); ok {
			name = v
			events = append(events, v)
		}
		if v, ok := strings.CutPrefix(line, "data: "); ok && name == "text" {
			var d struct{ Text string }
			json.Unmarshal([]byte(v), &d)
			text.WriteString(d.Text)
		}
	}
	if got := strings.Join(events, ","); got != "tool,text,text,done" {
		t.Fatalf("events = %s", got)
	}
	if text.String() != "You have $1,000.00." {
		t.Errorf("text = %q", text.String())
	}

	// The lookup really read the ledger.
	msgs := secondRequest["messages"].([]any)
	result := msgs[2].(map[string]any)["content"].([]any)[0].(map[string]any)
	if c, _ := result["content"].(string); !strings.Contains(c, `"Checking"`) || result["is_error"] != nil {
		t.Errorf("tool result = %v", result)
	}

	saved, transcript, err := svc.ChatTranscript(ctx, hh, chat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(transcript) != 4 || saved.Title != "How much is in checking?" {
		t.Fatalf("saved %d messages titled %q", len(transcript), saved.Title)
	}
	shown := assistant.Display(transcript)
	if len(shown) != 2 || shown[1].Text != "You have $1,000.00." || len(shown[1].Tools) != 1 {
		t.Errorf("display = %+v", shown)
	}
}
