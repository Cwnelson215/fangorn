package ledger_test

import (
	"testing"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
)

func TestAccountCardsAreNormalized(t *testing.T) {
	f := newFixture(t)
	create := func(mask string) (models.Account, error) {
		return f.svc.CreateAccount(f.ctx, f.hh, ledger.AccountInput{
			Name: "Checking " + mask, Type: models.AccountChecking, Mask: &mask, StartingBalanceDate: "2026-01-01",
		})
	}
	a, err := create(" 5678 1234,1234; ")
	if err != nil {
		t.Fatal(err)
	}
	if a.Mask == nil || *a.Mask != "5678, 1234" {
		t.Errorf("mask = %v, want \"5678, 1234\"", a.Mask)
	}
	for _, bad := range []string{"12a4", "123", "1234, 56789"} {
		_, err := create(bad)
		wantInvalid(t, err)
	}
	blank, err := create(" , ")
	if err != nil {
		t.Fatal(err)
	}
	if blank.Mask != nil {
		t.Errorf("separators only should clear the mask, got %q", *blank.Mask)
	}
}
