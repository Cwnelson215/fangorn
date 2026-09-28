package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/lib/pq"

	"github.com/cwnelson/fangorn/internal/goalfill"
)

// goalFill is what a goal linked to an account holds, from replaying the
// account (goalfill.Replay). Dollars, rounded to the cent.
type goalFill struct {
	Saved   float64
	moved   map[time.Time]int64
	drained map[time.Time]int64
}

// MovedIn is the change in what the goal held during the month starting at
// monthStart ("YYYY-MM-DD").
func (f goalFill) MovedIn(monthStart string) float64 {
	return cents(f.moved[monthKey(monthStart)])
}

// DrainedIn is the part of that month's change that spending or withdrawals
// took back.
func (f goalFill) DrainedIn(monthStart string) float64 {
	return cents(f.drained[monthKey(monthStart)])
}

// goalFills replays every account that has goals linked to it and returns what
// each of those goals holds, by goal id. Goals without an account aren't in it:
// they count logged contributions instead.
func (s *Service) goalFills(ctx context.Context, householdID int) (map[int]goalFill, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT g.id, g.account_id, g.priority, COALESCE(g.month, g.started_on), g.month,
		        (g.achieved_at AT TIME ZONE h.timezone)::date, g.target_amount,
		        a.starting_balance, a.id = h.income_account_id
		 FROM goals g
		 JOIN households h ON h.id = g.household_id
		 JOIN accounts a ON a.id = g.account_id AND a.household_id = g.household_id
		 WHERE g.household_id = $1`, householdID)
	if err != nil {
		return nil, fmt.Errorf("loading goals to fill: %w", err)
	}
	defer rows.Close()

	type account struct {
		goals   []goalfill.Goal
		from    time.Time
		opening float64
		income  bool
	}
	accounts := map[int]*account{}
	var ids []int64
	var order []int
	for rows.Next() {
		var g goalfill.Goal
		var accountID int
		var start time.Time
		var month, reached sql.NullTime
		var target, opening float64
		var income sql.NullBool
		if err := rows.Scan(&g.ID, &accountID, &g.Priority, &start, &month, &reached, &target,
			&opening, &income); err != nil {
			return nil, fmt.Errorf("scanning goal to fill: %w", err)
		}
		g.Start = midnight(start)
		if month.Valid {
			g.End = midnight(month.Time).AddDate(0, 1, 0)
		}
		if reached.Valid {
			g.Reached = midnight(reached.Time)
		}
		g.Target = toCents(target)
		a := accounts[accountID]
		if a == nil {
			a = &account{from: g.Start, opening: opening, income: income.Bool}
			accounts[accountID] = a
			order = append(order, accountID)
		}
		if g.Start.Before(a.from) {
			a.from = g.Start
		}
		a.goals = append(a.goals, g)
		ids = append(ids, int64(g.ID))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := map[int]goalFill{}
	if len(ids) == 0 {
		return out, nil
	}

	plans := map[int][]goalfill.Plan{}
	prows, err := s.db.QueryContext(ctx,
		`SELECT goal_id, effective_from, amount FROM goal_plans WHERE goal_id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("loading goal plans: %w", err)
	}
	defer prows.Close()
	for prows.Next() {
		var id int
		var from time.Time
		var amount sql.NullFloat64
		if err := prows.Scan(&id, &from, &amount); err != nil {
			return nil, fmt.Errorf("scanning goal plan: %w", err)
		}
		plans[id] = append(plans[id], goalfill.Plan{
			From: midnight(from), Amount: toCents(amount.Float64), Set: amount.Valid})
	}
	if err := prows.Err(); err != nil {
		return nil, err
	}

	for _, accountID := range order {
		a := accounts[accountID]
		for i := range a.goals {
			a.goals[i].Plans = plans[a.goals[i].ID]
		}
		// Trades move cash into shares without the money leaving the account,
		// so they count neither way.
		var before float64
		if err := s.db.QueryRowContext(ctx,
			`SELECT COALESCE(SUM(amount), 0) FROM transactions
			 WHERE household_id = $1 AND account_id = $2 AND kind <> 'trade' AND date < $3`,
			householdID, accountID, a.from).Scan(&before); err != nil {
			return nil, fmt.Errorf("totalling before the goals: %w", err)
		}
		mrows, err := s.db.QueryContext(ctx,
			`SELECT t.date, t.amount, `+goalMoney+` FROM transactions t
			 WHERE t.household_id = $1 AND t.account_id = $2 AND t.kind <> 'trade' AND t.date >= $3
			 ORDER BY t.date, t.id`,
			householdID, accountID, a.from)
		if err != nil {
			return nil, fmt.Errorf("loading the goals' account: %w", err)
		}
		var movements []goalfill.Movement
		for mrows.Next() {
			var m goalfill.Movement
			var date time.Time
			var amount float64
			if err := mrows.Scan(&date, &amount, &m.Saving); err != nil {
				mrows.Close()
				return nil, fmt.Errorf("scanning the goals' account: %w", err)
			}
			m.Date = midnight(date)
			m.Amount = toCents(amount)
			movements = append(movements, m)
		}
		mrows.Close()
		if err := mrows.Err(); err != nil {
			return nil, err
		}

		results := goalfill.Replay(goalfill.Account{
			Opening: toCents(a.opening + before), Income: a.income,
		}, a.goals, movements)
		for id, r := range results {
			out[id] = goalFill{Saved: cents(r.Saved), moved: r.Moved, drained: r.Drained}
		}
	}
	return out, nil
}

func toCents(v float64) int64 { return int64(math.Round(v * 100)) }
func cents(c int64) float64   { return float64(c) / 100 }

// midnight drops the clock and zone from a DATE column's value.
func midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// monthKey parses a "YYYY-MM-DD" known to be valid.
func monthKey(s string) time.Time {
	t, _ := time.Parse(time.DateOnly, s)
	return t
}
