package prices

import (
	"testing"
	"time"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
	"github.com/cwnelson/fangorn/internal/quotes"
)

func (f *fixture) linkedAccount(fund string) models.Account {
	f.t.Helper()
	f.provider.prices[fund] = 1
	if err := f.r.EnsureSecurity(f.ctx, fund); err != nil {
		f.t.Fatal(err)
	}
	a, err := f.svc.CreateAccount(f.ctx, f.hh, ledger.AccountInput{
		Name: "Brokerage", Type: models.AccountInvestment, StartingBalance: 1000, StartingBalanceDate: "2026-09-01",
		CashFund: &fund,
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return a
}

func TestRefreshYieldsRecordsAndCaches(t *testing.T) {
	f := newFixture(t)
	fund := f.sym("SPAXX")
	a := f.linkedAccount(fund)
	f.provider.yields[fund] = 3.33
	today := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)

	if err := f.r.RefreshYields(f.ctx, f.hh, today); err != nil {
		t.Fatal(err)
	}
	if n := f.provider.yieldCallCount(); n != 1 {
		t.Fatalf("yield lookups = %d, want 1", n)
	}
	out, err := f.svc.SavingsOutlookFor(f.ctx, f.hh, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if out.FundYield == nil || *out.FundYield != 3.33 || *out.FundYieldAsOf != "2026-09-12" {
		t.Errorf("outlook = %+v", out)
	}

	// Fresh: the next tick doesn't ask again.
	if err := f.r.RefreshYields(f.ctx, f.hh, today); err != nil {
		t.Fatal(err)
	}
	if n := f.provider.yieldCallCount(); n != 1 {
		t.Errorf("asked again while fresh (%d lookups)", n)
	}
}

func TestRefreshYieldsBacksOffAfterFailure(t *testing.T) {
	f := newFixture(t)
	fund := f.sym("SPAXX")
	f.linkedAccount(fund)
	f.provider.fail[fund] = quotes.ErrUnavailable
	today := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)

	if err := f.r.RefreshYields(f.ctx, f.hh, today); err == nil {
		t.Error("a failed lookup should be reported for logging")
	}
	f.r.RefreshYields(f.ctx, f.hh, today) // the next tick, moments later
	if n := f.provider.yieldCallCount(); n != 1 {
		t.Errorf("retried immediately (%d lookups)", n)
	}

	f.now = f.now.Add(31 * time.Minute)
	delete(f.provider.fail, fund)
	f.provider.yields[fund] = 3.4
	if err := f.r.RefreshYields(f.ctx, f.hh, today); err != nil {
		t.Fatal(err)
	}
	if n := f.provider.yieldCallCount(); n != 2 {
		t.Errorf("lookups after the retry wait = %d, want 2", n)
	}
}
