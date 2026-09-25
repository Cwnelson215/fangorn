package ledger_test

import (
	"testing"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
	"github.com/cwnelson/fangorn/internal/portfolio"
)

func (f *fixture) retirement(name, tax string, starting float64) models.Account {
	f.t.Helper()
	a, err := f.svc.CreateAccount(f.ctx, f.hh, ledger.AccountInput{
		Name: name, Type: models.AccountRetirement, TaxTreatment: &tax,
		StartingBalance: starting, StartingBalanceDate: "2026-01-01",
	})
	if err != nil {
		f.t.Fatalf("CreateAccount(%s): %v", name, err)
	}
	return a
}

// A retirement account is an investment account with rules on top: everything
// that works for a brokerage account has to work for it too.
func TestRetirementAccountHoldsSecurities(t *testing.T) {
	f := newFixture(t)
	f.account("Checking", models.AccountChecking, 2000)
	roth := f.retirement("Roth IRA", models.TaxRoth, 1000)
	if roth.TaxTreatment == nil || *roth.TaxTreatment != models.TaxRoth || roth.Class != models.ClassAsset {
		t.Fatalf("account = %+v", roth)
	}
	fund := f.sym("FXAIX")
	f.trade(roth.ID, portfolio.SideBuy, fund, "2026-02-01", 5, 100)
	f.trade(roth.ID, portfolio.SideOpening, fund, "2026-01-01", 2, 80)
	f.price(fund, 120, 118)

	a := f.accountByID(roth.ID)
	money(t, "cash", a.CashBalance, 500)
	money(t, "holdings", a.HoldingsValue, 840)

	h, err := f.svc.Holdings(f.ctx, f.hh, roth.ID)
	if err != nil || len(h.Positions) != 1 {
		t.Fatalf("holdings = %+v, %v", h, err)
	}

	summary, err := f.svc.InvestmentsSummary(f.ctx, f.hh)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Accounts) != 1 {
		t.Errorf("investments summary accounts = %+v, want the Roth IRA", summary.Accounts)
	}

	if pts := f.valueHistory(nil, 30); len(pts) == 0 {
		t.Error("combined value history left the retirement account out")
	}

	d := f.dashboard("", "")
	if d.Investments == nil {
		t.Error("dashboard investments line missing with only a retirement account")
	}
	money(t, "retirement value", d.RetirementValue, 1340)
	money(t, "net worth", d.NetWorth, 3340)
}

func TestRetirementTaxTreatment(t *testing.T) {
	f := newFixture(t)
	create := func(typ string, tax *string) error {
		_, err := f.svc.CreateAccount(f.ctx, f.hh, ledger.AccountInput{
			Name: "Account", Type: typ, TaxTreatment: tax, StartingBalanceDate: "2026-01-01",
		})
		return err
	}
	str := func(s string) *string { return &s }

	wantInvalid(t, create(models.AccountRetirement, nil))
	wantInvalid(t, create(models.AccountRetirement, str("hsa")))
	wantInvalid(t, create(models.AccountInvestment, str(models.TaxRoth)))
	wantInvalid(t, create(models.AccountChecking, str(models.TaxTraditional)))
	if err := create(models.AccountRetirement, str(models.TaxTraditional)); err != nil {
		t.Errorf("traditional retirement account: %v", err)
	}
}

// An account set up as a plain investment account can become a retirement one
// later without losing its trades — and back.
func TestInvestmentBecomesRetirement(t *testing.T) {
	f := newFixture(t)
	ira := f.account("IRA", models.AccountInvestment, 1000)
	f.trade(ira.ID, portfolio.SideBuy, f.sym("VOO"), "2026-02-01", 1, 500)

	roth := models.TaxRoth
	a, err := f.svc.UpdateAccount(f.ctx, f.hh, ira.ID, ledger.AccountInput{
		Name: "Roth IRA", Type: models.AccountRetirement, TaxTreatment: &roth,
		StartingBalance: 1000, StartingBalanceDate: "2026-01-01",
	})
	if err != nil {
		t.Fatalf("investment -> retirement: %v", err)
	}
	money(t, "holdings kept", a.HoldingsValue, 500)

	_, err = f.svc.UpdateAccount(f.ctx, f.hh, ira.ID, ledger.AccountInput{
		Name: "Roth IRA", Type: models.AccountSavings, StartingBalance: 1000, StartingBalanceDate: "2026-01-01",
	})
	wantInvalid(t, err)
}
