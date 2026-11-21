package goalfill

import (
	"testing"
	"time"
)

func d(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func in(day string, dollars int64) Movement {
	return Movement{Date: d(day), Amount: dollars * 100, Saving: true}
}

func other(day string, dollars int64) Movement {
	return Movement{Date: d(day), Amount: dollars * 100}
}

func longTerm(id, priority int, target, monthly int64) Goal {
	return Goal{ID: id, Priority: priority, Start: d("2026-01-01"), Target: target * 100,
		Plans: []Plan{{From: d("2026-01-01"), Amount: monthly * 100, Set: monthly > 0}}}
}

func check(t *testing.T, what string, got, wantDollars int64) {
	t.Helper()
	if got != wantDollars*100 {
		t.Errorf("%s = %.2f, want %d", what, float64(got)/100, wantDollars)
	}
}

// Money added fills every goal's share of the month in priority order first,
// and only then fills goals toward their targets, in the same order.
func TestSharesBeforeTargets(t *testing.T) {
	goals := []Goal{longTerm(2, 2, 5000, 300), longTerm(1, 1, 5000, 500)}
	got := Replay(Account{}, goals, []Movement{in("2026-01-15", 600)})
	check(t, "first goal", got[1].Saved, 500)
	check(t, "second goal", got[2].Saved, 100)

	// The second goal's share is finished before the first takes more.
	got = Replay(Account{}, goals, []Movement{
		in("2026-01-15", 600),
		in("2026-01-31", 3000),
	})
	check(t, "first goal", got[1].Saved, 3300)
	check(t, "second goal", got[2].Saved, 300)
	check(t, "first goal's month", got[1].Moved[d("2026-01-01")], 3300)
}

// A goal never takes past its target, and a goal with no plan takes only what
// is left past the shares.
func TestTargetsAndNoPlan(t *testing.T) {
	goals := []Goal{longTerm(1, 1, 200, 500), longTerm(2, 2, 5000, 0), longTerm(3, 3, 5000, 100)}
	got := Replay(Account{}, goals, []Movement{in("2026-01-15", 3000)})
	check(t, "capped at target", got[1].Saved, 200)
	check(t, "no plan", got[2].Saved, 2700)
	check(t, "next in line", got[3].Saved, 100)
}

// Spending comes out of free money first, then off the goals from the lowest
// priority up.
func TestSpendingDrainsInReverseOrder(t *testing.T) {
	goals := []Goal{longTerm(1, 1, 500, 500), longTerm(2, 2, 300, 300)}
	got := Replay(Account{Opening: 100 * 100}, goals, []Movement{
		in("2026-01-15", 1000), // 500 + 300 to goals, 200 free (+100 opening)
		other("2026-01-20", -500),
	})
	check(t, "first goal untouched", got[1].Saved, 500)
	check(t, "second goal drained", got[2].Saved, 100)
	check(t, "second goal's month", got[2].Moved[d("2026-01-01")], 100)
	check(t, "second goal drained this month", got[2].Drained[d("2026-01-01")], 200)

	// A deeper hole reaches the first goal, and past both goes overdrawn.
	got = Replay(Account{}, goals, []Movement{
		in("2026-01-15", 800),
		other("2026-01-20", -1000),
	})
	check(t, "first goal", got[1].Saved, 0)
	check(t, "second goal", got[2].Saved, 0)
}

// Same-day money in is counted before money out.
func TestSameDayBillDoesNotDrain(t *testing.T) {
	goals := []Goal{longTerm(1, 1, 500, 500)}
	got := Replay(Account{}, goals, []Movement{
		other("2026-01-15", -400),
		in("2026-01-15", 1000),
	})
	check(t, "goal", got[1].Saved, 500)
}

// A drained goal gets its share back before goals fill toward their targets.
func TestRefillAfterDrain(t *testing.T) {
	goals := []Goal{longTerm(1, 2, 5000, 500), longTerm(2, 1, 5000, 0)}
	got := Replay(Account{}, goals, []Movement{
		in("2026-01-01", 500),
		other("2026-01-05", -200),
		in("2026-01-15", 1000),
	})
	check(t, "refilled to its share", got[1].Saved, 500)
	check(t, "month", got[1].Moved[d("2026-01-01")], 500)
	check(t, "the rest toward the other's target", got[2].Saved, 800)
}

// Money moved in past the monthly shares fills the goals up to their targets,
// in order.
func TestSavingsAccountOverflow(t *testing.T) {
	goals := []Goal{longTerm(1, 1, 1000, 200), longTerm(2, 2, 5000, 100)}
	got := Replay(Account{}, goals, []Movement{in("2026-01-15", 1500)})
	check(t, "first goal to its target", got[1].Saved, 1000)
	check(t, "second goal", got[2].Saved, 500)
}

// Interest and other money in is free: it cushions withdrawals but fills
// nothing. Trades never reach the replay.
func TestOtherMoneyIsFree(t *testing.T) {
	goals := []Goal{longTerm(1, 1, 5000, 0)}
	got := Replay(Account{}, goals, []Movement{
		in("2026-01-15", 300),
		other("2026-01-31", 20),
		other("2026-02-03", -50),
	})
	check(t, "interest took the first $20", got[1].Saved, 270)
}

// Money moved in before a goal starts isn't its.
func TestBeforeStart(t *testing.T) {
	g := longTerm(1, 1, 5000, 0)
	g.Start = d("2026-02-01")
	got := Replay(Account{}, []Goal{g}, []Movement{in("2026-01-15", 300), in("2026-02-15", 100)})
	check(t, "goal", got[1].Saved, 100)
}

// A monthly goal takes up to its target inside its month, keeps what it held
// when the month closed, and its money is then free.
func TestMonthlyGoal(t *testing.T) {
	monthly := Goal{ID: 1, Priority: 1, Start: d("2026-02-01"), End: d("2026-03-01"), Target: 400 * 100}
	lt := longTerm(2, 2, 5000, 300)
	got := Replay(Account{}, []Goal{monthly, lt}, []Movement{
		in("2026-01-15", 1000), // before its month: only the long-term goal
		in("2026-02-15", 1000), // 400 to the monthly goal, 600 to the long-term
		other("2026-03-10", -1000), // after its month: its 400, now free, then the long-term goal
	})
	check(t, "monthly goal saved", got[1].Saved, 400)
	check(t, "long-term goal", got[2].Saved, 1000)

	// Inside its month, spending that reaches it lowers it.
	got = Replay(Account{}, []Goal{monthly, lt}, []Movement{
		in("2026-02-15", 700),
		other("2026-02-20", -500),
	})
	check(t, "long-term drained first", got[2].Saved, 0)
	check(t, "monthly goal drained next", got[1].Saved, 200)
}

// A reached goal takes no more but keeps what it has.
func TestReachedGoal(t *testing.T) {
	g := longTerm(1, 1, 5000, 500)
	g.Reached = d("2026-02-01")
	got := Replay(Account{}, []Goal{g}, []Movement{
		in("2026-01-15", 1000),
		in("2026-02-15", 1000),
	})
	check(t, "goal", got[1].Saved, 1000)
}

// A plan change applies from its month; a stopped plan takes no share. (A goal
// ahead of it with no plan takes the rest, so only the shares reach it.)
func TestPlanByMonth(t *testing.T) {
	g := longTerm(1, 2, 5000, 500)
	g.Plans = append(g.Plans, Plan{From: d("2026-02-01"), Amount: 100 * 100, Set: true},
		Plan{From: d("2026-03-01")})
	ahead := longTerm(2, 1, 100000, 0)
	got := Replay(Account{}, []Goal{g, ahead}, []Movement{
		in("2026-01-15", 1000), in("2026-02-15", 1000), in("2026-03-15", 1000),
	})
	check(t, "goal", got[1].Saved, 600)
}

// Each goal's entries name the movements that filled and drained it, with only
// its own part of each, and add up to its month.
func TestEntriesNameTheMovements(t *testing.T) {
	goals := []Goal{longTerm(1, 1, 5000, 500), longTerm(2, 2, 5000, 300)}
	paycheck, groceries := in("2026-01-15", 600), other("2026-01-20", -100)
	paycheck.ID, groceries.ID = 7, 8
	got := Replay(Account{}, goals, []Movement{paycheck, groceries})

	if e := got[1].Entries; len(e) != 1 || e[0].ID != 7 || e[0].Amount != 500*100 {
		t.Errorf("first goal's entries = %+v, want $500 of movement 7", e)
	}
	e := got[2].Entries
	if len(e) != 2 || e[0].ID != 7 || e[0].Amount != 100*100 || e[1].ID != 8 || e[1].Amount != -100*100 {
		t.Errorf("second goal's entries = %+v, want $100 of 7 then -$100 of 8", e)
	}

	// One movement can reach a goal twice — its share,
	// then the overflow toward its target — and still reads as one entry.
	transfer := in("2026-01-15", 2000)
	transfer.ID = 9
	got = Replay(Account{}, []Goal{longTerm(1, 1, 5000, 500)}, []Movement{transfer})
	if e := got[1].Entries; len(e) != 1 || e[0].Amount != 2000*100 {
		t.Errorf("entries = %+v, want one of $2,000", e)
	}
}
