package ledger_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
)

func TestSpendingBreakdown(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Checking", models.AccountChecking, 1000)
	savings := f.account("Savings", models.AccountSavings, 0)
	food := f.category("Groceries", models.KindExpense)
	fun := f.category("Fun", models.KindExpense)
	pay := f.category("Paycheck", models.KindIncome)

	f.txn(checking.ID, models.KindExpense, "2026-03-02", 100, &food.ID)
	f.txn(checking.ID, models.KindExpense, "2026-03-20", 50, &food.ID)
	f.txn(checking.ID, models.KindRefund, "2026-03-21", 20, &food.ID)
	f.txn(checking.ID, models.KindExpense, "2026-04-05", 40, &fun.ID)
	f.txn(checking.ID, models.KindIncome, "2026-03-15", 2000, &pay.ID)
	f.transfer(checking.ID, savings.ID, 500, "2026-03-16")

	byCategory, err := f.svc.SpendingBreakdown(f.ctx, f.hh, ledger.BreakdownFilter{
		Side: "expense", GroupBy: "category", From: "2026-03-01", To: "2026-04-30",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Refunds net out; the transfer and the paycheck are not spending.
	want := []ledger.BreakdownRow{{Key: "Groceries", Amount: 130, Count: 3}, {Key: "Fun", Amount: 40, Count: 1}}
	if len(byCategory) != len(want) {
		t.Fatalf("got %+v, want %+v", byCategory, want)
	}
	for i := range want {
		if byCategory[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, byCategory[i], want[i])
		}
	}

	byMonth, err := f.svc.SpendingBreakdown(f.ctx, f.hh, ledger.BreakdownFilter{Side: "expense", GroupBy: "month"})
	if err != nil {
		t.Fatal(err)
	}
	if len(byMonth) != 2 || byMonth[0].Key != "2026-03" || byMonth[0].Amount != 130 || byMonth[1].Key != "2026-04" {
		t.Errorf("by month = %+v", byMonth)
	}

	income, err := f.svc.SpendingBreakdown(f.ctx, f.hh, ledger.BreakdownFilter{Side: "income", GroupBy: "account"})
	if err != nil {
		t.Fatal(err)
	}
	if len(income) != 1 || income[0].Amount != 2000 {
		t.Errorf("income = %+v", income)
	}

	_, err = f.svc.SpendingBreakdown(f.ctx, f.hh, ledger.BreakdownFilter{Side: "expense", GroupBy: "t.amount; DROP TABLE x"})
	wantInvalid(t, err)
}

func TestChatTranscript(t *testing.T) {
	f := newFixture(t)
	chat, err := f.svc.CreateChat(f.ctx, f.hh)
	if err != nil {
		t.Fatal(err)
	}
	// Nothing asked yet (or the first question failed): not listed.
	if list, _ := f.svc.ListChats(f.ctx, f.hh); len(list) != 0 {
		t.Errorf("empty chat listed: %+v", list)
	}

	turn := []json.RawMessage{
		json.RawMessage(`{"role":"user","content":[{"type":"text","text":"hi"}]}`),
		json.RawMessage(`{"role":"assistant","content":[{"type":"text","text":"hello"}]}`),
	}
	if err := f.svc.AppendChat(f.ctx, f.hh, chat.ID, 0, turn, "First"); err != nil {
		t.Fatal(err)
	}
	// Built on a transcript that has since grown: refused, not interleaved.
	if err := f.svc.AppendChat(f.ctx, f.hh, chat.ID, 0, turn, "Second"); !errors.Is(err, ledger.ErrChatChanged) {
		t.Fatalf("stale append = %v, want ErrChatChanged", err)
	}
	if err := f.svc.AppendChat(f.ctx, f.hh, chat.ID, 2, turn, "Second"); err != nil {
		t.Fatal(err)
	}

	got, msgs, err := f.svc.ChatTranscript(f.ctx, f.hh, chat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if list, _ := f.svc.ListChats(f.ctx, f.hh); len(list) != 1 {
		t.Errorf("chats = %+v, want the one", list)
	}
	if got.Title != "First" {
		t.Errorf("title = %q, want the first one kept", got.Title)
	}
	if len(msgs) != 4 {
		t.Fatalf("transcript has %d messages, want 4", len(msgs))
	}

	other := newFixture(t)
	if _, _, err := other.svc.ChatTranscript(other.ctx, other.hh, chat.ID); !errors.Is(err, ledger.ErrNotFound) {
		t.Errorf("another household read the chat: %v", err)
	}
	if err := other.svc.AppendChat(other.ctx, other.hh, chat.ID, 4, turn, ""); !errors.Is(err, ledger.ErrNotFound) {
		t.Errorf("another household appended: %v", err)
	}

	if err := f.svc.DeleteChat(f.ctx, f.hh, chat.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := f.svc.ListChats(f.ctx, f.hh); len(list) != 0 {
		t.Errorf("chats after delete = %+v", list)
	}
}
