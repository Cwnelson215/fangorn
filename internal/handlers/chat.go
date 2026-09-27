package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/cwnelson/fangorn/internal/assistant"
	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
)

const (
	// turnTimeout bounds one answer, every lookup included. The server's 15s
	// WriteTimeout is lifted for the streamed response only.
	turnTimeout = 4 * time.Minute

	// pingEvery keeps the stream from looking idle to the proxies in front of
	// the app while the model thinks, which can send nothing for a while.
	pingEvery = 15 * time.Second

	maxQuestionRunes = 4000
)

// ChatHandler serves conversations with the assistant. assistant is nil when
// no API key is configured, and the app then hides the feature.
type ChatHandler struct {
	svc         *ledger.Service
	assistant   *assistant.Assistant
	householdID int
}

func NewChatHandler(svc *ledger.Service, a *assistant.Assistant, householdID int) *ChatHandler {
	return &ChatHandler{svc: svc, assistant: a, householdID: householdID}
}

func (h *ChatHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/chats", h.List)
	mux.HandleFunc("POST /api/chats", h.Create)
	mux.HandleFunc("GET /api/chats/{id}", h.Get)
	mux.HandleFunc("DELETE /api/chats/{id}", h.Delete)
	mux.HandleFunc("POST /api/chats/{id}/messages", h.Send)
}

func (h *ChatHandler) List(w http.ResponseWriter, r *http.Request) {
	chats, err := h.svc.ListChats(r.Context(), h.householdID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": h.assistant != nil, "chats": chats})
}

func (h *ChatHandler) Create(w http.ResponseWriter, r *http.Request) {
	chat, err := h.svc.CreateChat(r.Context(), h.householdID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, chat)
}

func (h *ChatHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	chat, transcript, err := h.svc.ChatTranscript(r.Context(), h.householdID, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chat": chat, "messages": assistant.Display(transcript)})
}

func (h *ChatHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteChat(r.Context(), h.householdID, id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Send asks a question and streams the answer as server-sent events:
//
//	text  {"text": "..."}          the next piece of the answer
//	tool  {"label": "..."}         something being looked up
//	done  {"chat": {...}}          the turn is saved
//	error {"error": "..."}         the turn failed and nothing was saved
//
// Errors found before the stream starts are ordinary JSON responses.
func (h *ChatHandler) Send(w http.ResponseWriter, r *http.Request) {
	if h.assistant == nil {
		writeError(w, http.StatusServiceUnavailable, "The assistant isn't set up on this server")
		return
	}
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Text string `json:"text"`
	}
	if !decode(w, r, &in) {
		return
	}
	text := strings.TrimSpace(in.Text)
	if text == "" {
		writeError(w, http.StatusBadRequest, "Ask something first")
		return
	}
	if utf8.RuneCountInString(text) > maxQuestionRunes {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Keep it under %d characters", maxQuestionRunes))
		return
	}

	chat, transcript, err := h.svc.ChatTranscript(r.Context(), h.householdID, id)
	if err != nil {
		fail(w, err)
		return
	}
	household, err := h.svc.GetHousehold(r.Context(), h.householdID)
	if err != nil {
		fail(w, err)
		return
	}
	today := household.Today().Format(models.DateOnly)

	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Now().Add(turnTimeout + 30*time.Second)); err != nil {
		log.Printf("chat: cannot extend write deadline: %v", err)
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	// nginx would otherwise buffer the stream and deliver the answer all at once.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Events come from the turn and from the keep-alive ticker; one writer at
	// a time.
	var mu sync.Mutex
	send := func(event string, data any) {
		payload, _ := json.Marshal(data)
		mu.Lock()
		defer mu.Unlock()
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload)
		rc.Flush()
	}
	stopPing := make(chan struct{})
	go func() {
		t := time.NewTicker(pingEvery)
		defer t.Stop()
		for {
			select {
			case <-stopPing:
				return
			case <-t.C:
				mu.Lock()
				fmt.Fprint(w, ": ping\n\n")
				rc.Flush()
				mu.Unlock()
			}
		}
	}()
	defer close(stopPing)

	ctx, cancel := context.WithTimeout(r.Context(), turnTimeout)
	defer cancel()

	turn, err := h.assistant.Reply(ctx, h.householdID, transcript, text, today, func(e assistant.Event) {
		switch e.Kind {
		case "text":
			send("text", map[string]string{"text": e.Text})
		case "tool":
			send("tool", map[string]string{"label": e.Label})
		}
	})
	if err != nil {
		if r.Context().Err() == nil {
			log.Printf("chat %d: %v", id, err)
		}
		send("error", map[string]string{"error": replyError(err)})
		return
	}

	// A finished answer is kept even if the phone has gone away meanwhile.
	saveCtx, cancelSave := context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Second)
	defer cancelSave()
	err = h.svc.AppendChat(saveCtx, h.householdID, id, len(transcript), turn, assistant.Title(text))
	if err != nil {
		if errors.Is(err, ledger.ErrChatChanged) {
			send("error", map[string]string{"error": "This chat was answered somewhere else at the same time. Reload it to see where it stands."})
			return
		}
		log.Printf("chat %d: saving: %v", id, err)
		send("error", map[string]string{"error": "The answer couldn't be saved. Try asking again."})
		return
	}
	if chat.Title == "" {
		chat.Title = assistant.Title(text)
	}
	chat.UpdatedAt = time.Now().Format(time.RFC3339)
	send("done", map[string]any{"chat": chat})
}

// replyError words a failed turn for the person who asked.
func replyError(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "That took too long to answer. Try a narrower question."
	case errors.Is(err, assistant.ErrDeclined):
		return "Claude declined to answer that one. Try rephrasing it."
	case errors.Is(err, assistant.ErrRejectedKey):
		return "The server's Anthropic API key was rejected. Check ANTHROPIC_API_KEY and ANTHROPIC_WORKSPACE_ID."
	case errors.Is(err, assistant.ErrUnavailable):
		return "Claude isn't reachable right now. Try again in a minute."
	default:
		return "Something went wrong answering that. Try again."
	}
}
