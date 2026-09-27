package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sse renders events the way the Messages API streams them.
func sse(events ...string) string {
	var b strings.Builder
	for _, e := range events {
		var typ struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(e), &typ); err != nil {
			panic(fmt.Sprintf("bad test event %s: %v", e, err))
		}
		fmt.Fprintf(&b, "event: %s\ndata: %s\n\n", typ.Type, e)
	}
	return b.String()
}

const (
	msgStart = `{"type":"message_start","message":{"id":"msg_1","model":"claude-opus-5","content":[]}}`
	msgStop  = `{"type":"message_stop"}`
)

func stopWith(reason string) string {
	return `{"type":"message_delta","delta":{"stop_reason":"` + reason + `"},"usage":{"output_tokens":10}}`
}

// fakeAPI answers each request with the next canned stream and records what
// was sent.
type fakeAPI struct {
	t        *testing.T
	streams  []string
	requests []map[string]any
	headers  []http.Header
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		f.t.Errorf("request is not JSON: %v", err)
	}
	f.requests = append(f.requests, req)
	f.headers = append(f.headers, r.Header.Clone())
	if len(f.requests) > len(f.streams) {
		f.t.Errorf("unexpected request %d", len(f.requests))
		http.Error(w, "no more", http.StatusInternalServerError)
		return
	}
	w.Header().Set("content-type", "text/event-stream")
	io.WriteString(w, f.streams[len(f.requests)-1])
}

func newTestAssistant(t *testing.T, api http.Handler) *Assistant {
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	return New(nil, "test-key", "wrkspc_test", "claude-opus-5").WithBaseURL(srv.URL)
}

func TestReplyRunsToolsUntilAnswered(t *testing.T) {
	api := &fakeAPI{t: t, streams: []string{
		sse(
			msgStart,
			`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-abc"}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`,
			`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Let me "}}`,
			`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"check."}}`,
			`{"type":"content_block_stop","index":1}`,
			`{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"toolu_1","name":"list_goals","input":{}}}`,
			// An argument the schema doesn't allow: rejected before the
			// ledger is touched, and the error goes back to the model.
			`{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"bogus\":"}}`,
			`{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":" 1}"}}`,
			`{"type":"content_block_stop","index":2}`,
			`{"type":"content_block_start","index":3,"content_block":{"type":"tool_use","id":"toolu_2","name":"list_goals","input":{}}}`,
			`{"type":"content_block_delta","index":3,"delta":{"type":"input_json_delta","partial_json":"{\"trunc"}}`,
			`{"type":"content_block_stop","index":3}`,
			stopWith("tool_use"),
			`{"type":"ping"}`,
			msgStop,
		),
		sse(
			msgStart,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"You have no goals."}}`,
			`{"type":"content_block_stop","index":0}`,
			stopWith("end_turn"),
			msgStop,
		),
	}}
	a := newTestAssistant(t, api)

	prior := []json.RawMessage{
		json.RawMessage(`{"role":"user","content":[{"type":"text","text":"Today is x."},{"type":"text","text":"earlier"}]}`),
		json.RawMessage(`{"role":"assistant","content":[{"type":"text","text":"earlier answer"}]}`),
	}
	var text strings.Builder
	var labels []string
	turn, err := a.Reply(context.Background(), 1, prior, "How are my goals?", "2026-09-26", func(e Event) {
		switch e.Kind {
		case "text":
			text.WriteString(e.Text)
		case "tool":
			labels = append(labels, e.Label)
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := text.String(); got != "Let me check.You have no goals." {
		t.Errorf("streamed text = %q", got)
	}
	// The malformed-JSON call never runs, so it announces nothing.
	if len(labels) != 1 || labels[0] != "Checking goals" {
		t.Errorf("tool labels = %v", labels)
	}
	if len(turn) != 4 {
		t.Fatalf("turn has %d messages, want question, reply, results, answer", len(turn))
	}

	if len(api.requests) != 2 {
		t.Fatalf("made %d requests, want 2", len(api.requests))
	}
	first := api.requests[0]
	if first["model"] != "claude-opus-5" || first["stream"] != true || first["fallbacks"] != "default" {
		t.Errorf("request settings = model %v stream %v fallbacks %v", first["model"], first["stream"], first["fallbacks"])
	}
	if cc, _ := first["cache_control"].(map[string]any); cc["type"] != "ephemeral" {
		t.Errorf("cache_control = %v", first["cache_control"])
	}
	h := api.headers[0]
	if h.Get("x-api-key") != "test-key" || h.Get("anthropic-beta") != fallbackBeta || h.Get("anthropic-workspace-id") != "wrkspc_test" {
		t.Errorf("headers = %v", h)
	}

	// The second request replays the history, then the first reply verbatim —
	// thinking signature included — then both results in one message.
	msgs := api.requests[1]["messages"].([]any)
	if len(msgs) != 5 {
		t.Fatalf("second request has %d messages, want 5", len(msgs))
	}
	question := msgs[2].(map[string]any)["content"].([]any)
	if !strings.Contains(question[0].(map[string]any)["text"].(string), "Saturday, September 26, 2026") {
		t.Errorf("date line = %v", question[0])
	}
	reply := msgs[3].(map[string]any)["content"].([]any)
	if sig := reply[0].(map[string]any)["signature"]; sig != "sig-abc" {
		t.Errorf("thinking signature replayed as %v", sig)
	}
	if in := reply[3].(map[string]any)["input"]; len(in.(map[string]any)) != 0 {
		t.Errorf("unparseable input replayed as %v, want {}", in)
	}
	results := msgs[4].(map[string]any)["content"].([]any)
	if len(results) != 2 {
		t.Fatalf("got %d tool results in one message, want 2", len(results))
	}
	for i, id := range []string{"toolu_1", "toolu_2"} {
		r := results[i].(map[string]any)
		if r["tool_use_id"] != id || r["is_error"] != true {
			t.Errorf("result %d = %v", i, r)
		}
	}
	if c := results[0].(map[string]any)["content"].(string); !strings.Contains(c, "bogus") {
		t.Errorf("bad-argument error doesn't name the field: %q", c)
	}
}

func TestReplyRefusal(t *testing.T) {
	api := &fakeAPI{t: t, streams: []string{sse(
		msgStart,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Partial"}}`,
		`{"type":"content_block_stop","index":0}`,
		stopWith("refusal"),
		msgStop,
	)}}
	a := newTestAssistant(t, api)
	_, err := a.Reply(context.Background(), 1, nil, "q", "2026-09-26", func(Event) {})
	if !errors.Is(err, ErrDeclined) {
		t.Fatalf("err = %v, want ErrDeclined", err)
	}
}

func TestReplyUnavailable(t *testing.T) {
	a := newTestAssistant(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		w.WriteHeader(529)
		io.WriteString(w, `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`)
	}))
	_, err := a.Reply(context.Background(), 1, nil, "q", "2026-09-26", func(Event) {})
	if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "Overloaded") {
		t.Fatalf("err = %v, want ErrUnavailable naming the cause", err)
	}

	a = newTestAssistant(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
	}))
	_, err = a.Reply(context.Background(), 1, nil, "q", "2026-09-26", func(Event) {})
	if !errors.Is(err, ErrRejectedKey) {
		t.Fatalf("401: err = %v, want ErrRejectedKey", err)
	}

	// A stream that ends without message_stop didn't finish.
	a = newTestAssistant(t, &fakeAPI{t: t, streams: []string{sse(msgStart)}})
	_, err = a.Reply(context.Background(), 1, nil, "q", "2026-09-26", func(Event) {})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("truncated stream: err = %v", err)
	}
}

func TestReplayableAfterFallback(t *testing.T) {
	content := []map[string]any{
		{"type": "thinking", "thinking": "", "signature": "declined-model"},
		{"type": "text", "text": "Partial answer "},
		{"type": "tool_use", "id": "toolu_x", "name": "list_goals", "input": map[string]any{}},
		{"type": "fallback", "from": map[string]any{"model": "a"}, "to": map[string]any{"model": "b"}},
		{"type": "thinking", "thinking": "", "signature": "fallback-model"},
		{"type": "text", "text": "continued."},
		{"type": "tool_use", "id": "toolu_y", "name": "list_goals", "input": map[string]any{}},
	}
	got, bad := replayable(content, map[int]bool{6: true})
	var types []string
	for _, b := range got {
		types = append(types, b["type"].(string))
	}
	if strings.Join(types, ",") != "text,thinking,text,tool_use" {
		t.Fatalf("replayable blocks = %v", types)
	}
	if got[1]["signature"] != "fallback-model" || got[3]["id"] != "toolu_y" {
		t.Errorf("kept the wrong side of the fallback: %v", got)
	}
	if !bad[3] || len(bad) != 1 {
		t.Errorf("bad input re-indexed to %v, want {3}", bad)
	}
}

func TestDisplay(t *testing.T) {
	transcript := []json.RawMessage{
		json.RawMessage(`{"role":"user","content":[{"type":"text","text":"Today is x."},{"type":"text","text":"What did we spend?"}]}`),
		json.RawMessage(`{"role":"assistant","content":[{"type":"thinking","thinking":"","signature":"s"},{"type":"text","text":"Checking. "},{"type":"tool_use","id":"t1","name":"spending_breakdown","input":{}}]}`),
		json.RawMessage(`{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"[]"}]}`),
		json.RawMessage(`{"role":"assistant","content":[{"type":"text","text":"$10."}]}`),
		json.RawMessage(`{"role":"user","content":[{"type":"text","text":"Today is y."},{"type":"text","text":"Thanks"}]}`),
	}
	got := Display(transcript)
	want := []Message{
		{Role: "user", Text: "What did we spend?"},
		{Role: "assistant", Text: "Checking. \n\n$10.", Tools: []string{"Adding up spending"}},
		{Role: "user", Text: "Thanks"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i].Role != want[i].Role || got[i].Text != want[i].Text || strings.Join(got[i].Tools, ",") != strings.Join(want[i].Tools, ",") {
			t.Errorf("message %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestTitle(t *testing.T) {
	if got := Title("  How much\non groceries? "); got != "How much on groceries?" {
		t.Errorf("Title = %q", got)
	}
	long := strings.Repeat("word ", 30)
	if got := Title(long); len([]rune(got)) > 61 || !strings.HasSuffix(got, "…") || strings.Contains(got, " …") {
		t.Errorf("Title(long) = %q", got)
	}
}

func TestToolSchemasAreObjects(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range toolDefs {
		if seen[d.Name] {
			t.Errorf("tool %s declared twice", d.Name)
		}
		seen[d.Name] = true
		var schema struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(d.InputSchema, &schema); err != nil || schema.Type != "object" {
			t.Errorf("tool %s schema: %v %q", d.Name, err, schema.Type)
		}
		if toolsByName[d.Name].label == "" {
			t.Errorf("tool %s has no label", d.Name)
		}
	}
}
