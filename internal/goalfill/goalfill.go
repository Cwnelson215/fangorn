// Package goalfill decides how much of an account's money belongs to each of
// the savings goals linked to it. It is pure: no database, no clock. The ledger
// supplies the account's money movements in date order and its goals; this
// replays them and says what each goal holds.
//
// Several goals can share one account, so money moved in can't simply count
// toward all of them. Instead:
//
//   - Money added (a transfer in, or income other than interest) fills the
//     goals in priority order, each up to what is left of its share for that
//     month: a monthly goal's whole target, or a long-term goal's plan in force.
//     On an account that isn't where income lands, whatever is left then fills
//     the goals up to their targets, in the same order — money moved into a
//     savings account is being saved. On the income account it stays free: that
//     money is there to pay the bills.
//   - Any other money in (interest, refunds, the starting balance) is free.
//   - Money going out comes from free money first. Only once that runs out does
//     it come off the goals, lowest priority first.
//
// Trades don't count either way: buying a fund moves cash into shares, and the
// money is still in the account. A monthly goal closes with its month; what it
// held then is what it saved, and its money becomes free.
//
// Amounts are in cents so a long replay can't drift.
package goalfill

import (
	"sort"
	"time"
)

// Goal is one goal linked to the account.
type Goal struct {
	ID       int
	Priority int
	// Start is the first day money can fill it: a long-term goal's start date,
	// a monthly goal's month.
	Start time.Time
	// End, on a monthly goal, is the first day after its month; zero on a
	// long-term goal.
	End time.Time
	// Reached, when set, is the day the goal was marked reached: it takes no
	// more money from then on, but keeps what it holds.
	Reached time.Time
	Target  int64
	// Plans is a long-term goal's monthly share, versioned by month; a plan
	// with no amount stops the share. Unused on a monthly goal.
	Plans []Plan
}

type Plan struct {
	From   time.Time // first of a month
	Amount int64
	Set    bool
}

// Movement is one transaction on the account. Saving marks money added toward
// goals; it only matters when Amount is positive. ID is the caller's own name
// for it, handed back in Result.Entries.
type Movement struct {
	ID     int
	Date   time.Time
	Amount int64
	Saving bool
}

// Account is what the replay needs about the account itself.
type Account struct {
	// Opening is the money already there before the first movement: free.
	Opening int64
	// Income is set on the account income lands in: money added past the
	// goals' monthly shares stays free rather than filling their targets.
	Income bool
}

// Result is what one goal ended up with.
type Result struct {
	// Saved is what the goal holds now — for a monthly goal whose month is
	// over, what it held when the month closed.
	Saved int64
	// Moved is the change in what it held, by month (keyed by the first of
	// the month): money filled less money drained.
	Moved map[time.Time]int64
	// Drained is the part of each month's change that was money taken back
	// out by spending or withdrawals.
	Drained map[time.Time]int64
	// Entries is what each movement did to the goal, in replay order: positive
	// where it filled it, negative where it drained it. A month's entries sum
	// to its Moved.
	Entries []Entry
}

// Entry is one movement's effect on one goal.
type Entry struct {
	ID     int // the Movement's ID
	Date   time.Time
	Amount int64
}

// MonthStart returns the first day of t's month.
func MonthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

type state struct {
	goal   Goal
	held   int64
	closed bool
	res    *Result
}

// Replay runs the account's movements against its goals. Movements must be in
// date order; within a day, money in is counted before money out, so a
// paycheck and a bill on the same day don't dip into savings.
func Replay(account Account, goals []Goal, movements []Movement) map[int]*Result {
	states := make([]*state, len(goals))
	for i, g := range goals {
		states[i] = &state{goal: g, res: &Result{Moved: map[time.Time]int64{}, Drained: map[time.Time]int64{}}}
	}
	sort.SliceStable(states, func(i, j int) bool {
		if states[i].goal.Priority != states[j].goal.Priority {
			return states[i].goal.Priority < states[j].goal.Priority
		}
		return states[i].goal.ID < states[j].goal.ID
	})

	free := account.Opening
	for i := 0; i < len(movements); {
		day := movements[i].Date
		j := i
		for j < len(movements) && movements[j].Date.Equal(day) {
			j++
		}
		// A monthly goal whose month has ended keeps what it saved, and its
		// money goes back to being free.
		for _, s := range states {
			if !s.closed && !s.goal.End.IsZero() && !day.Before(s.goal.End) {
				s.closed = true
				s.res.Saved = s.held
				free += s.held
				s.held = 0
			}
		}
		for _, m := range movements[i:j] {
			if m.Amount <= 0 {
				continue
			}
			if m.Saving {
				free += fill(states, m, !account.Income)
			} else {
				free += m.Amount
			}
		}
		for _, m := range movements[i:j] {
			if m.Amount < 0 {
				free = drain(states, m, free)
			}
		}
		i = j
	}

	out := make(map[int]*Result, len(states))
	for _, s := range states {
		if !s.closed {
			s.res.Saved = s.held
		}
		out[s.goal.ID] = s.res
	}
	return out
}

// fill hands the money m added to the goals in priority order and returns what
// none of them took.
func fill(states []*state, m Movement, overflow bool) int64 {
	day, amount := m.Date, m.Amount
	month := MonthStart(day)
	var open []*state
	for _, s := range states {
		if s.open(day) {
			open = append(open, s)
		}
	}
	for _, s := range open {
		room := min(s.share(month)-s.res.Moved[month], s.goal.Target-s.held)
		amount -= s.take(m, min(room, amount))
	}
	if overflow {
		for _, s := range open {
			amount -= s.take(m, min(s.goal.Target-s.held, amount))
		}
	}
	return amount
}

// drain takes the money m sent out from free money first, then off the goals
// from the lowest priority up. It returns what's free afterwards, which goes
// below zero only when the goals are empty too.
func drain(states []*state, m Movement, free int64) int64 {
	day, amount := m.Date, -m.Amount
	fromFree := min(max(free, 0), amount)
	free -= fromFree
	amount -= fromFree
	month := MonthStart(day)
	for i := len(states) - 1; i >= 0 && amount > 0; i-- {
		s := states[i]
		if s.closed || s.held <= 0 {
			continue
		}
		t := min(s.held, amount)
		s.held -= t
		s.res.Moved[month] -= t
		s.res.Drained[month] += t
		s.res.Entries = append(s.res.Entries, Entry{ID: m.ID, Date: day, Amount: -t})
		amount -= t
	}
	return free - amount
}

func (s *state) open(day time.Time) bool {
	g := s.goal
	return !s.closed && !day.Before(g.Start) &&
		(g.End.IsZero() || day.Before(g.End)) &&
		(g.Reached.IsZero() || day.Before(g.Reached))
}

// share is the goal's part of month's savings: a monthly goal's whole target,
// or a long-term goal's plan in force that month (none without one).
func (s *state) share(month time.Time) int64 {
	if !s.goal.End.IsZero() {
		return s.goal.Target
	}
	var amount int64
	var from time.Time
	for _, p := range s.goal.Plans {
		if !p.From.After(month) && !p.From.Before(from) {
			from = p.From
			amount = 0
			if p.Set {
				amount = p.Amount
			}
		}
	}
	return amount
}

func (s *state) take(m Movement, amount int64) int64 {
	if amount <= 0 {
		return 0
	}
	s.held += amount
	s.res.Moved[MonthStart(m.Date)] += amount
	// One movement can reach a goal twice: its share of the month, then the
	// overflow toward its target.
	if n := len(s.res.Entries); n > 0 && s.res.Entries[n-1].ID == m.ID && s.res.Entries[n-1].Amount > 0 {
		s.res.Entries[n-1].Amount += amount
	} else {
		s.res.Entries = append(s.res.Entries, Entry{ID: m.ID, Date: m.Date, Amount: amount})
	}
	return amount
}
