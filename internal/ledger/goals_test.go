package ledger_test

import (
	"testing"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
)

// A linked goal counts money added to its account from the day it starts —
// transfers and income deposited there — but not the balance already there,
// not interest the account earned, and not money moved in before it began.
func TestGoalCountsMoneyMovedIn(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Checking", models.AccountChecking, 5000)
	savings := f.account("Savings", models.AccountSavings, 2000)
	gifts := f.category("Gifts", models.KindIncome)
	today := ledger.Household{Timezone: "America/Denver"}.Today()
	day := today.Format(models.DateOnly)
	yesterday := today.AddDate(0, 0, -1).Format(models.DateOnly)

	f.transfer(checking.ID, savings.ID, 999, yesterday) // before the goal

	monthly := 200.0
	goal, err := f.svc.CreateGoal(f.ctx, f.hh, ledger.GoalInput{
		Name: "Emergency fund", TargetAmount: 1000, AccountID: &savings.ID, MonthlyAmount: &monthly,
	})
	if err != nil {
		t.Fatal(err)
	}
	if goal.StartedOn != day {
		t.Errorf("started_on = %s, want today %s", goal.StartedOn, day)
	}
	money(t, "nothing moved yet", goal.Saved, 0)

	f.transfer(checking.ID, savings.ID, 300, day)
	f.transfer(savings.ID, checking.ID, 50, day)
	f.txn(savings.ID, models.KindIncome, day, 25, &gifts.ID) // deposited straight in: counts
	interest := f.txn(savings.ID, models.KindIncome, day, 7.5, &gifts.ID)
	if _, err := f.svc.DB().Exec(`UPDATE transactions SET source = 'interest' WHERE id = $1`, interest.ID); err != nil {
		t.Fatal(err)
	}

	goal, err = f.svc.GetGoal(f.ctx, f.hh, goal.ID)
	if err != nil {
		t.Fatal(err)
	}
	money(t, "moved in, less moved out, plus income deposited", goal.Saved, 275)

	bm, err := f.svc.BudgetMonth(f.ctx, f.hh, day)
	if err != nil {
		t.Fatal(err)
	}
	if len(bm.Savings) != 1 {
		t.Fatalf("got %d savings lines, want 1", len(bm.Savings))
	}
	line := bm.Savings[0]
	money(t, "planned", line.Monthly, 200)
	money(t, "moved this month", line.Moved, 275)
	money(t, "saved overall", line.Saved, 275)

	// A goal without a monthly amount isn't in the budget.
	if _, err := f.svc.UpdateGoal(f.ctx, f.hh, goal.ID, ledger.GoalInput{
		Name: "Emergency fund", TargetAmount: 1000, AccountID: &savings.ID,
	}); err != nil {
		t.Fatal(err)
	}
	bm, err = f.svc.BudgetMonth(f.ctx, f.hh, day)
	if err != nil {
		t.Fatal(err)
	}
	if len(bm.Savings) != 0 {
		t.Errorf("savings lines = %+v, want none", bm.Savings)
	}
}

func TestIncomeAccountSetting(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Checking", models.AccountChecking, 0)

	got, err := f.svc.UpdateSettings(f.ctx, f.hh, ledger.Settings{IncomeAccountID: &checking.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got.IncomeAccountID == nil || *got.IncomeAccountID != checking.ID {
		t.Fatalf("income account = %v, want %d", got.IncomeAccountID, checking.ID)
	}

	other := newFixture(t)
	theirs := other.account("Theirs", models.AccountChecking, 0)
	_, err = f.svc.UpdateSettings(f.ctx, f.hh, ledger.Settings{IncomeAccountID: &theirs.ID})
	wantInvalid(t, err)

	// Archiving the account un-chooses it rather than leaving income going nowhere.
	if err := f.svc.SetAccountArchived(f.ctx, f.hh, checking.ID, true); err != nil {
		t.Fatal(err)
	}
	got, err = f.svc.GetSettings(f.ctx, f.hh)
	if err != nil {
		t.Fatal(err)
	}
	if got.IncomeAccountID != nil {
		t.Errorf("archived income account still chosen: %d", *got.IncomeAccountID)
	}
	_, err = f.svc.UpdateSettings(f.ctx, f.hh, ledger.Settings{IncomeAccountID: &checking.ID})
	wantInvalid(t, err)
}

// Money meant for savings that leaves the income account as spending counts
// against the month's savings, split by monthly amount — once, even when it
// was first pulled back out of savings.
func TestSavingsShortfallFromIncomeAccount(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Checking", models.AccountChecking, 0)
	emergency := f.account("Emergency", models.AccountSavings, 0)
	vacation := f.account("Vacation", models.AccountSavings, 0)
	pay := f.category("Pay", models.KindIncome)
	food := f.category("Food", models.KindExpense)
	day := ledger.Household{Timezone: "America/Denver"}.Today().Format(models.DateOnly)

	if _, err := f.svc.UpdateSettings(f.ctx, f.hh, ledger.Settings{IncomeAccountID: &checking.ID}); err != nil {
		t.Fatal(err)
	}
	five, twoFifty := 500.0, 250.0
	for _, g := range []ledger.GoalInput{
		{Name: "Emergency", TargetAmount: 5000, AccountID: &emergency.ID, MonthlyAmount: &five},
		{Name: "Vacation", TargetAmount: 3000, AccountID: &vacation.ID, MonthlyAmount: &twoFifty},
	} {
		if _, err := f.svc.CreateGoal(f.ctx, f.hh, g); err != nil {
			t.Fatal(err)
		}
	}

	// $3,000 in and $750 planned for savings leaves $2,250 to spend.
	f.txn(checking.ID, models.KindIncome, day, 3000, &pay.ID)
	f.transfer(checking.ID, emergency.ID, 500, day)
	f.txn(checking.ID, models.KindExpense, day, 2400, &food.ID)

	type view struct {
		Total float64
		Lines map[string][2]float64 // moved, overspent
	}
	month := func() view {
		t.Helper()
		bm, err := f.svc.BudgetMonth(f.ctx, f.hh, day)
		if err != nil {
			t.Fatal(err)
		}
		out := view{Total: bm.SavingsShortfall, Lines: map[string][2]float64{}}
		for _, l := range bm.Savings {
			out.Lines[l.Name] = [2]float64{l.Moved, l.Overspent}
		}
		return out
	}

	got := month()
	money(t, "shortfall", got.Total, 150)
	money(t, "emergency's share", got.Lines["Emergency"][1], 100)
	money(t, "vacation's share", got.Lines["Vacation"][1], 50)

	// Pulling $100 back out of savings lowers Emergency's month by $100 — and
	// only that: it isn't also counted as spending from the income account.
	f.transfer(emergency.ID, checking.ID, 100, day)
	got = month()
	money(t, "emergency moved after pulling back", got.Lines["Emergency"][0], 400)
	money(t, "shortfall after pulling back", got.Total, 50)

	// Expected income counts for the current month, so a paycheck still to come
	// isn't read as overspending.
	f.setBudget(pay.ID, 6000, day[:7])
	money(t, "shortfall with more income expected", month().Total, 0)
}
