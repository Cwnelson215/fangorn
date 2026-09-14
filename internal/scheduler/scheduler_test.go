package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
	"github.com/cwnelson/fangorn/internal/testdb"
)

// These drive runHousehold rather than Tick: Tick walks every household in the
// database, and tests share one database with a household apiece.
//
// The scheduler reads the real clock, so rule dates are placed relative to the
// household's today instead of being hardcoded.

type fixture struct {
	t         *testing.T
	ctx       context.Context
	svc       *ledger.Service
	sched     *Scheduler
	household ledger.Household
	checking  models.Account
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := testdb.Open(t)
	svc := ledger.New(db)
	household := ledger.Household{ID: testdb.Household(t, db, "America/Denver"), Timezone: "America/Denver"}

	checking, err := svc.CreateAccount(context.Background(), household.ID, ledger.AccountInput{
		Name: "Checking", Type: models.AccountChecking, StartingBalance: 1000, StartingBalanceDate: "2025-01-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{
		t: t, ctx: context.Background(), svc: svc, sched: New(svc, time.Minute, 60),
		household: household, checking: checking,
	}
}

func (f *fixture) today() time.Time { return f.household.Today() }

func (f *fixture) rule(in ledger.RuleInput) models.RecurringRule {
	f.t.Helper()
	if in.Name == "" {
		in.Name = "Streaming"
	}
	in.Kind = models.KindExpense
	in.AccountID = f.checking.ID
	if in.Amount == 0 {
		in.Amount = 15.99
	}
	rule, err := f.svc.CreateRule(f.ctx, f.household.ID, in)
	if err != nil {
		f.t.Fatalf("CreateRule: %v", err)
	}
	return rule
}

func (f *fixture) run() {
	f.t.Helper()
	f.sched.runHousehold(f.ctx, f.household)
}

// postedDates returns the dates of every transaction a rule has written, oldest first.
func (f *fixture) postedDates(ruleID int) []string {
	f.t.Helper()
	rows, err := f.svc.DB().Query(
		`SELECT date FROM transactions WHERE recurring_rule_id = $1 ORDER BY date`, ruleID)
	if err != nil {
		f.t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d); err != nil {
			f.t.Fatal(err)
		}
		out = append(out, d.Format(models.DateOnly))
	}
	return out
}

func (f *fixture) scheduledCount(ruleID int) int {
	f.t.Helper()
	var n int
	if err := f.svc.DB().QueryRow(
		`SELECT COUNT(*) FROM recurring_occurrences WHERE rule_id = $1 AND status = 'scheduled'`,
		ruleID).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func TestBackfillsMissedOccurrencesExactlyOnce(t *testing.T) {
	f := newFixture(t)
	start := f.today().AddDate(0, -3, 0)
	rule := f.rule(ledger.RuleInput{Frequency: "monthly", StartDate: start.Format(models.DateOnly)})

	spec, err := ledger.RuleSpec(rule)
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, d := range spec.Occurrences(start, f.today(), 0) {
		want = append(want, d.Format(models.DateOnly))
	}
	if len(want) < 3 {
		t.Fatalf("test setup: expected at least 3 past occurrences, got %v", want)
	}

	// Boot, then boot again: the second pass must find nothing new to post.
	f.run()
	f.run()

	got := f.postedDates(rule.ID)
	if len(got) != len(want) {
		t.Fatalf("posted %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("posting %d on %s, want %s", i, got[i], want[i])
		}
	}
	if f.scheduledCount(rule.ID) == 0 {
		t.Error("no future occurrences were projected inside the horizon")
	}

	reloaded, err := f.svc.GetRule(f.ctx, f.household.ID, rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.NextDueDate == nil || *reloaded.NextDueDate <= f.today().Format(models.DateOnly) {
		t.Errorf("next_due_date = %v, want a date after today", reloaded.NextDueDate)
	}
}

// Rescheduling a rule must not go back and post the new schedule's past dates:
// moving a monthly charge from the 1st to the 15th is not permission to bill
// every 15th since the rule started.
func TestEditingScheduleDoesNotBackPost(t *testing.T) {
	f := newFixture(t)
	first := time.Date(f.today().Year(), f.today().Month(), 1, 0, 0, 0, 0, time.UTC)
	start := first.AddDate(0, -4, 0).Format(models.DateOnly)
	rule := f.rule(ledger.RuleInput{Frequency: "monthly", StartDate: start})

	f.run()
	before := f.postedDates(rule.ID)
	if len(before) != 5 {
		t.Fatalf("test setup: posted %v before the edit, want 5 monthly dates", before)
	}
	lastPosted := before[len(before)-1]

	fifteenth := 15
	if _, err := f.svc.UpdateRule(f.ctx, f.household.ID, rule.ID, ledger.RuleInput{
		Name: rule.Name, Kind: rule.Kind, AccountID: rule.AccountID, Amount: rule.Amount,
		Frequency: "monthly", DayOfMonth: &fifteenth, StartDate: start,
	}); err != nil {
		t.Fatal(err)
	}
	f.run()

	existing := map[string]bool{}
	for _, d := range before {
		existing[d] = true
	}
	var added []string
	for _, d := range f.postedDates(rule.ID) {
		if !existing[d] {
			added = append(added, d)
		}
	}
	for _, d := range added {
		if d <= lastPosted {
			t.Errorf("edit back-posted %s, on or before the last posted date %s", d, lastPosted)
		}
	}
	// The only new posting allowed is this month's 15th, and only once it has arrived.
	wantNew := 0
	if f.today().Day() >= 15 {
		wantNew = 1
	}
	if len(added) != wantNew {
		t.Errorf("edit posted %v, want %d new transaction(s)", added, wantNew)
	}
}

func TestPausedAndManualRulesDoNotPost(t *testing.T) {
	f := newFixture(t)
	start := f.today().AddDate(0, -2, 0).Format(models.DateOnly)

	paused := f.rule(ledger.RuleInput{Name: "Paused", Frequency: "weekly", StartDate: start})
	if err := f.svc.SetRulePaused(f.ctx, f.household.ID, paused.ID, true); err != nil {
		t.Fatal(err)
	}
	off := false
	manual := f.rule(ledger.RuleInput{Name: "Manual", Frequency: "weekly", StartDate: start, AutoPost: &off})

	f.run()

	if got := f.postedDates(paused.ID); len(got) != 0 {
		t.Errorf("paused rule posted %v", got)
	}
	if got := f.postedDates(manual.ID); len(got) != 0 {
		t.Errorf("auto_post=false rule posted %v", got)
	}
	if f.scheduledCount(manual.ID) == 0 {
		t.Error("auto_post=false rule should still materialize occurrences for the upcoming list")
	}
}

func TestNetWorthSnapshotIsOneRowPerDay(t *testing.T) {
	f := newFixture(t)
	f.run()
	if _, err := f.svc.CreateTransaction(f.ctx, f.household.ID, ledger.TransactionInput{
		AccountID: f.checking.ID, Kind: models.KindExpense, Date: f.today().Format(models.DateOnly),
		Amount: 250, Description: "Groceries",
	}); err != nil {
		t.Fatal(err)
	}
	f.run()

	history, err := f.svc.NetWorthHistory(f.ctx, f.household.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("got %d snapshots, want 1: %+v", len(history), history)
	}
	if history[0].NetWorth != 750 {
		t.Errorf("snapshot net worth = %.2f, want the latest figure 750.00", history[0].NetWorth)
	}
}
