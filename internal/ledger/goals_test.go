package ledger_test

import (
	"testing"
	"time"

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

// A long-term goal's monthly share applies from the month it's set for until
// changed, and a change leaves earlier months as they were.
func TestLongTermGoalPlanByMonth(t *testing.T) {
	f := newFixture(t)
	savings := f.account("Savings", models.AccountSavings, 0)
	today := ledger.Household{Timezone: "America/Denver"}.Today()
	thisMonth := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	ym := func(n int) string { return thisMonth.AddDate(0, n, 0).Format("2006-01") }
	lines := func(n int) map[string]float64 {
		t.Helper()
		bm, err := f.svc.BudgetMonth(f.ctx, f.hh, ym(n))
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]float64{}
		for _, l := range bm.Savings {
			out[l.Name] = l.Monthly
		}
		return out
	}
	planRows := func(goalID int) int {
		t.Helper()
		var n int
		if err := f.svc.DB().QueryRow(`SELECT COUNT(*) FROM goal_plans WHERE goal_id = $1`, goalID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	input := func(amount *float64, from string) ledger.GoalInput {
		return ledger.GoalInput{Name: "House", TargetAmount: 20000, AccountID: &savings.ID,
			MonthlyAmount: amount, MonthlyFrom: &from}
	}
	amt := func(v float64) *float64 { return &v }

	g, err := f.svc.CreateGoal(f.ctx, f.hh, input(amt(300), ym(1)))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lines(0)["House"]; ok {
		t.Error("a plan starting next month shows this month")
	}
	money(t, "next month", lines(1)["House"], 300)

	if _, err := f.svc.UpdateGoal(f.ctx, f.hh, g.ID, input(amt(400), ym(2))); err != nil {
		t.Fatal(err)
	}
	money(t, "next month keeps 300", lines(1)["House"], 300)
	money(t, "the month after is 400", lines(2)["House"], 400)

	// Saving it again with what's already in force writes nothing.
	if _, err := f.svc.UpdateGoal(f.ctx, f.hh, g.ID, input(amt(300), ym(1))); err != nil {
		t.Fatal(err)
	}
	if n := planRows(g.ID); n != 2 {
		t.Errorf("plan rows after a no-op save = %d, want 2", n)
	}
	money(t, "later change survives a no-op save", lines(2)["House"], 400)

	// Clearing it stops the share from that month on.
	if _, err := f.svc.UpdateGoal(f.ctx, f.hh, g.ID, input(nil, ym(3))); err != nil {
		t.Fatal(err)
	}
	if _, ok := lines(3)["House"]; ok {
		t.Error("a stopped plan still shows")
	}
	money(t, "before the stop", lines(2)["House"], 400)

	got, err := f.svc.GetGoal(f.ctx, f.hh, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.MonthlyAmount != nil {
		t.Errorf("stopped goal still reports a monthly amount: %v", *got.MonthlyAmount)
	}
}

// A monthly goal belongs to its month: it shows only there, counts only money
// added inside it, and isn't one of the long-term goals.
func TestMonthlyGoal(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Checking", models.AccountChecking, 5000)
	savings := f.account("Savings", models.AccountSavings, 0)
	today := ledger.Household{Timezone: "America/Denver"}.Today()
	thisMonth := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	next := thisMonth.AddDate(0, 1, 0)
	nextYM := next.Format("2006-01")

	g, err := f.svc.CreateGoal(f.ctx, f.hh, ledger.GoalInput{
		Name: "Refill", TargetAmount: 1000, AccountID: &savings.ID, Month: &nextYM,
	})
	if err != nil {
		t.Fatal(err)
	}
	if g.Month == nil || *g.Month != next.Format(models.DateOnly) {
		t.Fatalf("month = %v, want %s", g.Month, next.Format(models.DateOnly))
	}

	f.transfer(checking.ID, savings.ID, 200, today.Format(models.DateOnly)) // before its month
	f.transfer(checking.ID, savings.ID, 600, next.Format(models.DateOnly))
	f.transfer(checking.ID, savings.ID, 50, next.AddDate(0, 1, 0).Format(models.DateOnly)) // after it

	g, err = f.svc.GetGoal(f.ctx, f.hh, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	money(t, "counts only inside its month", g.Saved, 600)

	for n, want := range map[int]bool{0: false, 1: true, 2: false} {
		bm, err := f.svc.BudgetMonth(f.ctx, f.hh, thisMonth.AddDate(0, n, 0).Format("2006-01"))
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, l := range bm.Savings {
			if l.GoalID == g.ID {
				found = true
				money(t, "line amount is the target", l.Monthly, 1000)
				money(t, "moved in its month", l.Moved, 600)
				if l.GoalMonth == nil {
					t.Error("a monthly goal's line carries no month")
				}
			}
		}
		if found != want {
			t.Errorf("month +%d: line present = %v, want %v", n, found, want)
		}
	}

	goals, err := f.svc.ListGoals(f.ctx, f.hh)
	if err != nil {
		t.Fatal(err)
	}
	if len(goals) != 0 {
		t.Errorf("long-term list includes the monthly goal: %+v", goals)
	}

	// Its kind is fixed, and it can't also take a monthly share.
	_, err = f.svc.UpdateGoal(f.ctx, f.hh, g.ID, ledger.GoalInput{Name: "Refill", TargetAmount: 1000})
	wantInvalid(t, err)
	amount := 100.0
	_, err = f.svc.CreateGoal(f.ctx, f.hh, ledger.GoalInput{
		Name: "Both", TargetAmount: 1000, Month: &nextYM, MonthlyAmount: &amount,
	})
	wantInvalid(t, err)
}
