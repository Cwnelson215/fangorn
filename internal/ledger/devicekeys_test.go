package ledger_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
)

func TestDeviceKeyLifecycle(t *testing.T) {
	f := newFixture(t)
	card := f.account("Visa", models.AccountCreditCard, 0)

	key, token, err := f.svc.CreateDeviceKey(f.ctx, f.hh, "  Carter's iPhone ", card.ID)
	if err != nil {
		t.Fatalf("CreateDeviceKey: %v", err)
	}
	if !strings.HasPrefix(token, ledger.DeviceKeyPrefix) || len(token) < 40 {
		t.Fatalf("token %q doesn't look like a key", token)
	}
	if key.Name != "Carter's iPhone" || key.AccountName != "Visa" || key.LastUsedAt != nil {
		t.Fatalf("unexpected key %+v", key)
	}

	got, err := f.svc.DeviceKeyByToken(f.ctx, token)
	if err != nil {
		t.Fatalf("DeviceKeyByToken: %v", err)
	}
	if got.ID != key.ID || got.HouseholdID != f.hh || got.LastUsedAt == nil {
		t.Fatalf("resolved %+v, want key %d in household %d with last_used_at set", got, key.ID, f.hh)
	}

	for name, bad := range map[string]string{
		"unknown":   ledger.DeviceKeyPrefix + "nope",
		"no prefix": strings.TrimPrefix(token, ledger.DeviceKeyPrefix),
		"empty":     "",
	} {
		if _, err := f.svc.DeviceKeyByToken(f.ctx, bad); !errors.Is(err, ledger.ErrNotFound) {
			t.Errorf("%s token: err = %v, want ErrNotFound", name, err)
		}
	}

	if err := f.svc.DeleteDeviceKey(f.ctx, f.hh, key.ID); err != nil {
		t.Fatalf("DeleteDeviceKey: %v", err)
	}
	if _, err := f.svc.DeviceKeyByToken(f.ctx, token); !errors.Is(err, ledger.ErrNotFound) {
		t.Fatalf("revoked key still resolves: %v", err)
	}
}

func TestDeviceKeyIsScopedToItsHousehold(t *testing.T) {
	f := newFixture(t)
	other := newFixture(t)
	card := f.account("Visa", models.AccountCreditCard, 0)
	key, _, err := f.svc.CreateDeviceKey(f.ctx, f.hh, "Phone", card.ID)
	if err != nil {
		t.Fatal(err)
	}

	if err := other.svc.DeleteDeviceKey(other.ctx, other.hh, key.ID); !errors.Is(err, ledger.ErrNotFound) {
		t.Errorf("another household deleted the key: %v", err)
	}
	if keys, _ := other.svc.ListDeviceKeys(other.ctx, other.hh); len(keys) != 0 {
		t.Errorf("another household lists %d keys", len(keys))
	}
	// A key can't be pointed at someone else's account.
	if _, _, err := other.svc.CreateDeviceKey(other.ctx, other.hh, "Phone", card.ID); !isInvalid(err) {
		t.Errorf("key on a foreign account: err = %v, want ErrInvalid", err)
	}
}

func TestLogFromShortcut(t *testing.T) {
	f := newFixture(t)
	card := f.account("Visa", models.AccountCreditCard, 0)
	groceries := f.category("Groceries", models.KindExpense)
	f.category("Paycheck", models.KindIncome)
	key, _, err := f.svc.CreateDeviceKey(f.ctx, f.hh, "Phone", card.ID)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("expense by category name, any case", func(t *testing.T) {
		tx, err := f.svc.LogFromShortcut(f.ctx, key, ledger.ShortcutEntry{Amount: 12.5, Category: "groceries"})
		if err != nil {
			t.Fatal(err)
		}
		if tx.Kind != models.KindExpense || tx.Amount != -12.5 || tx.AccountID != card.ID ||
			tx.CategoryID == nil || *tx.CategoryID != groceries.ID || tx.Description != "Groceries" {
			t.Fatalf("unexpected transaction %+v", tx)
		}
	})

	t.Run("income follows the category", func(t *testing.T) {
		tx, err := f.svc.LogFromShortcut(f.ctx, key, ledger.ShortcutEntry{Amount: 100, Category: "Paycheck", Note: "Bonus"})
		if err != nil {
			t.Fatal(err)
		}
		if tx.Kind != models.KindIncome || tx.Amount != 100 || tx.Description != "Bonus" {
			t.Fatalf("unexpected transaction %+v", tx)
		}
	})

	t.Run("no category is an uncategorized expense", func(t *testing.T) {
		tx, err := f.svc.LogFromShortcut(f.ctx, key, ledger.ShortcutEntry{Amount: 3})
		if err != nil {
			t.Fatal(err)
		}
		if tx.Kind != models.KindExpense || tx.CategoryID != nil || tx.Description != "Expense" {
			t.Fatalf("unexpected transaction %+v", tx)
		}
	})

	t.Run("refund", func(t *testing.T) {
		tx, err := f.svc.LogFromShortcut(f.ctx, key, ledger.ShortcutEntry{Amount: 4, Category: "Groceries", Refund: true})
		if err != nil {
			t.Fatal(err)
		}
		if tx.Kind != models.KindRefund || tx.Amount != 4 {
			t.Fatalf("unexpected transaction %+v", tx)
		}
	})

	for name, in := range map[string]ledger.ShortcutEntry{
		"unknown category":        {Amount: 1, Category: "Yachts"},
		"refund of income":        {Amount: 1, Category: "Paycheck", Refund: true},
		"refund with no category": {Amount: 1, Refund: true},
		"zero amount":             {Amount: 0, Category: "Groceries"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := f.svc.LogFromShortcut(f.ctx, key, in); !isInvalid(err) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}

	// Everything lands on the key's account: 12.50 out, 100 in, 3 out, 4 back.
	if got := f.balance(card.ID); math.Abs(got-88.5) > 0.001 {
		t.Errorf("card balance = %.2f, want 88.50", got)
	}
}

func TestShortcutCategoriesPutRecentUseFirst(t *testing.T) {
	f := newFixture(t)
	cash := f.account("Cash", models.AccountCash, 100)
	f.category("Aardvarks", models.KindExpense)
	dining := f.category("Dining", models.KindExpense)
	f.category("Paycheck", models.KindIncome)
	f.txn(cash.ID, models.KindExpense, f.today(), 5, &dining.ID)

	names, err := f.svc.ShortcutCategories(f.ctx, f.hh, models.KindExpense)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) < 2 || names[0] != "Dining" {
		t.Fatalf("names = %v, want Dining first", names)
	}
	for _, n := range names {
		if n == "Paycheck" {
			t.Fatalf("an income category was offered for spending: %v", names)
		}
	}
	if _, err := f.svc.ShortcutCategories(f.ctx, f.hh, "transfer"); !isInvalid(err) {
		t.Errorf("kind=transfer: err = %v, want ErrInvalid", err)
	}
}

func isInvalid(err error) bool {
	var inv ledger.ErrInvalid
	return errors.As(err, &inv)
}

// today is the household's today, which is what LogFromShortcut dates entries.
func (f *fixture) today() string {
	f.t.Helper()
	h, err := f.svc.GetHousehold(f.ctx, f.hh)
	if err != nil {
		f.t.Fatal(err)
	}
	return h.Today().Format(models.DateOnly)
}
