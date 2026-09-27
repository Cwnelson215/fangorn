package assistant

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// The assistant talks to the Messages API over plain HTTP, like the receipt
// reader in internal/vision and for the same reason: the repo keeps its
// dependencies to the database driver and the migration runner. Unlike the
// receipt reader it streams, so an answer appears as it is written instead of
// after a long wait, and so a turn that thinks and calls several tools never
// sits on one idle connection.

const (
	anthropicBaseURL = "https://api.anthropic.com"
	anthropicVersion = "2023-06-01"
	fallbackBeta     = "server-side-fallback-2026-07-01"
)

// ErrUnavailable is a failure worth retrying later: the API is down, rate
// limited or overloaded, or the network dropped. ErrDeclined is the model (and
// its fallback) declining to answer.
var (
	ErrUnavailable = errors.New("assistant unavailable")
	ErrDeclined    = errors.New("assistant declined to answer")
	// ErrRejectedKey is a 401 or 403: the API key (or its workspace) is wrong.
	ErrRejectedKey = errors.New("assistant API key rejected")
)

// client is one configured connection to the Messages API.
type client struct {
	http        *http.Client
	baseURL     string
	apiKey      string
	workspaceID string
}

type request struct {
	Model        string            `json:"model"`
	MaxTokens    int               `json:"max_tokens"`
	System       string            `json:"system"`
	Tools        []toolDef         `json:"tools"`
	Messages     []json.RawMessage `json:"messages"`
	Stream       bool              `json:"stream"`
	Fallbacks    string            `json:"fallbacks"`
	CacheControl cacheControl      `json:"cache_control"`
	Thinking     thinking          `json:"thinking"`
	OutputConfig outputConfig      `json:"output_config"`
}

type cacheControl struct {
	Type string `json:"type"`
}

type thinking struct {
	Type string `json:"type"`
}

type outputConfig struct {
	Effort string `json:"effort"`
}

// toolDef is a client tool as the API declares it.
type toolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
	// EagerInputStreaming streams a tool's input as it is written rather than
	// buffered and validated server-side, so the input has to be checked here —
	// runTool decodes it strictly and answers a malformed one with an error.
	EagerInputStreaming bool `json:"eager_input_streaming"`
}

// response is one finished model turn, rebuilt from the stream.
type response struct {
	Model      string
	StopReason string
	// Content holds each block as the API sent it, deltas applied, so it can be
	// replayed verbatim — thinking blocks are only accepted back unchanged.
	Content []map[string]any
	// badInput marks tool_use blocks whose streamed input was not valid JSON.
	badInput map[int]bool
}

// stream sends one request and rebuilds the response from its events, calling
// onText with each piece of answer text as it arrives.
func (c *client) stream(ctx context.Context, req request, onText func(string)) (response, error) {
	req.Stream = true
	body, err := json.Marshal(req)
	if err != nil {
		return response{}, fmt.Errorf("assistant: encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return response{}, fmt.Errorf("assistant: building request: %w", err)
	}
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	httpReq.Header.Set("anthropic-beta", fallbackBeta)
	httpReq.Header.Set("content-type", "application/json")
	if c.workspaceID != "" {
		httpReq.Header.Set("anthropic-workspace-id", c.workspaceID)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return response{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return response{}, statusError(resp.StatusCode, raw)
	}
	return readStream(resp.Body, onText)
}

// streamEvent is the union of the event payloads readStream looks at.
type streamEvent struct {
	Type    string `json:"type"`
	Index   int    `json:"index"`
	Message struct {
		Model string `json:"model"`
	} `json:"message"`
	ContentBlock json.RawMessage `json:"content_block"`
	Delta        struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Thinking    string `json:"thinking"`
		Signature   string `json:"signature"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// readStream parses a server-sent event stream into a response. Only the data
// lines matter: every payload carries its own type.
func readStream(r io.Reader, onText func(string)) (response, error) {
	out := response{badInput: map[int]bool{}}
	partial := map[int]*strings.Builder{}

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 8<<20)
	stopped := false
	for sc.Scan() {
		line := sc.Text()
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)

		var ev streamEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return out, fmt.Errorf("%w: undecodable stream event: %v", ErrUnavailable, err)
		}

		switch ev.Type {
		case "message_start":
			out.Model = ev.Message.Model

		case "content_block_start":
			block, err := decodeBlock(ev.ContentBlock)
			if err != nil {
				return out, fmt.Errorf("%w: undecodable content block: %v", ErrUnavailable, err)
			}
			for len(out.Content) <= ev.Index {
				out.Content = append(out.Content, nil)
			}
			out.Content[ev.Index] = block

		case "content_block_delta":
			if ev.Index >= len(out.Content) || out.Content[ev.Index] == nil {
				return out, fmt.Errorf("%w: delta for unknown block %d", ErrUnavailable, ev.Index)
			}
			block := out.Content[ev.Index]
			switch ev.Delta.Type {
			case "text_delta":
				block["text"] = str(block["text"]) + ev.Delta.Text
				if onText != nil && ev.Delta.Text != "" {
					onText(ev.Delta.Text)
				}
			case "thinking_delta":
				block["thinking"] = str(block["thinking"]) + ev.Delta.Thinking
			case "signature_delta":
				block["signature"] = str(block["signature"]) + ev.Delta.Signature
			case "input_json_delta":
				b := partial[ev.Index]
				if b == nil {
					b = &strings.Builder{}
					partial[ev.Index] = b
				}
				b.WriteString(ev.Delta.PartialJSON)
			}

		case "content_block_stop":
			if ev.Index >= len(out.Content) || out.Content[ev.Index] == nil {
				continue
			}
			block := out.Content[ev.Index]
			if block["type"] != "tool_use" {
				continue
			}
			// A tool's input arrives as JSON fragments. None at all means no
			// arguments. Input that doesn't parse is kept as an empty object, so
			// the block can still be sent back, and answered with an error.
			input := map[string]any{}
			if b := partial[ev.Index]; b != nil && strings.TrimSpace(b.String()) != "" {
				dec := json.NewDecoder(strings.NewReader(b.String()))
				dec.UseNumber()
				if err := dec.Decode(&input); err != nil {
					input = map[string]any{}
					out.badInput[ev.Index] = true
				}
			}
			block["input"] = input

		case "message_delta":
			if ev.Delta.StopReason != "" {
				out.StopReason = ev.Delta.StopReason
			}

		case "message_stop":
			stopped = true

		case "error":
			return out, fmt.Errorf("%w: stream error: %s: %s", ErrUnavailable, ev.Error.Type, ev.Error.Message)
		}
	}
	if err := sc.Err(); err != nil {
		return out, fmt.Errorf("%w: reading stream: %v", ErrUnavailable, err)
	}
	if !stopped {
		return out, fmt.Errorf("%w: stream ended early", ErrUnavailable)
	}
	// Indices are dense in practice; drop any hole rather than send a null.
	blocks := out.Content[:0]
	bad := map[int]bool{}
	for i, b := range out.Content {
		if b == nil {
			continue
		}
		if out.badInput[i] {
			bad[len(blocks)] = true
		}
		blocks = append(blocks, b)
	}
	out.Content, out.badInput = blocks, bad
	return out, nil
}

// decodeBlock keeps every field of a block, numbers included, as sent.
func decodeBlock(raw json.RawMessage) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var block map[string]any
	if err := dec.Decode(&block); err != nil {
		return nil, err
	}
	if block == nil {
		return nil, errors.New("null block")
	}
	return block, nil
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

type errorResponse struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// statusError describes an HTTP error from the API. Rate limits and server
// errors are ErrUnavailable and auth failures ErrRejectedKey; any other
// rejected request is a bug, logged in full by the caller.
func statusError(status int, body []byte) error {
	detail := strings.TrimSpace(string(body))
	if len(detail) > 300 {
		detail = detail[:300] + "…"
	}
	var e errorResponse
	if json.Unmarshal(body, &e) == nil && e.Error.Message != "" {
		detail = e.Error.Type + ": " + e.Error.Message
	}
	switch {
	case status == http.StatusTooManyRequests || status >= 500:
		return fmt.Errorf("%w: status %d: %s", ErrUnavailable, status, detail)
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return fmt.Errorf("%w: status %d: %s", ErrRejectedKey, status, detail)
	}
	return fmt.Errorf("assistant: status %d: %s", status, detail)
}
