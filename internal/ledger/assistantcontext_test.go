package ledger_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
)

// A chat keeps the instructions and files it started with; a removed file
// stays for the chats using it and is deleted once none do.
func TestAssistantContextSnapshots(t *testing.T) {
	f := newFixture(t)

	if _, err := f.svc.SetAssistantInstructions(f.ctx, f.hh, "  Saving for a house.  "); err != nil {
		t.Fatal(err)
	}
	plan, err := f.svc.AddAssistantFile(f.ctx, f.hh, "plan.pdf", "application/pdf", []byte("%PDF-1.4"))
	if err != nil {
		t.Fatal(err)
	}
	notes, err := f.svc.AddAssistantFile(f.ctx, f.hh, "notes.md", "text/plain", []byte("notes"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.AddAssistantFile(f.ctx, f.hh, "x.docx", "application/msword", []byte("x"))
	wantInvalid(t, err)

	snap, err := f.svc.SnapshotChatContext(f.ctx, f.hh)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Instructions != "Saving for a house." || len(snap.FileIDs) != 2 {
		t.Fatalf("snapshot = %+v", snap)
	}

	chat, err := f.svc.CreateChat(f.ctx, f.hh)
	if err != nil {
		t.Fatal(err)
	}
	turn := []json.RawMessage{
		json.RawMessage(`{"role":"user","content":[{"type":"text","text":"q"}]}`),
		json.RawMessage(`{"role":"assistant","content":[{"type":"text","text":"a"}]}`),
	}
	if err := f.svc.AppendChat(f.ctx, f.hh, chat.ID, 0, turn, "Q", &snap); err != nil {
		t.Fatal(err)
	}
	// A later turn can't replace it.
	other := models.ChatContext{Instructions: "changed", FileIDs: []int{}}
	if err := f.svc.AppendChat(f.ctx, f.hh, chat.ID, 2, turn, "Q", &other); err != nil {
		t.Fatal(err)
	}

	// Edits change what new chats get, not this one.
	if _, err := f.svc.SetAssistantInstructions(f.ctx, f.hh, "Retiring early."); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.RemoveAssistantFile(f.ctx, f.hh, plan.ID); err != nil {
		t.Fatal(err)
	}
	got, _, err := f.svc.ChatTranscript(f.ctx, f.hh, chat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Context == nil || got.Context.Instructions != "Saving for a house." || len(got.Context.FileIDs) != 2 {
		t.Fatalf("chat context = %+v", got.Context)
	}
	files, err := f.svc.ContextFiles(f.ctx, f.hh, got.Context.FileIDs)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Name != "plan.pdf" || string(files[1].Data) != "notes" {
		t.Fatalf("chat still reads %+v, want both files", files)
	}

	cur, err := f.svc.GetAssistantContext(f.ctx, f.hh)
	if err != nil {
		t.Fatal(err)
	}
	if cur.Instructions != "Retiring early." || len(cur.Files) != 1 || cur.Files[0].ID != notes.ID {
		t.Fatalf("current context = %+v", cur)
	}
	if err := f.svc.RemoveAssistantFile(f.ctx, f.hh, plan.ID); !errors.Is(err, ledger.ErrNotFound) {
		t.Fatalf("removing twice: err = %v, want not found", err)
	}

	// Deleting the last chat that used it deletes the removed file.
	if err := f.svc.DeleteChat(f.ctx, f.hh, chat.ID); err != nil {
		t.Fatal(err)
	}
	files, err = f.svc.ContextFiles(f.ctx, f.hh, []int{plan.ID, notes.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].ID != notes.ID {
		t.Fatalf("after deleting the chat, files = %+v, want only notes.md", files)
	}

	// Another household sees none of it.
	g := newFixture(t)
	if files, _ := g.svc.ContextFiles(g.ctx, g.hh, []int{notes.ID}); len(files) != 0 {
		t.Fatal("another household read the file")
	}
}
