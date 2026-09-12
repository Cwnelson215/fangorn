package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/cwnelson/fangorn/internal/models"
)

// SnapshotNetWorth records the given day's net worth for a household.
//
// Balances are always derivable from the ledger, so a snapshot is not the source
// of truth — it exists purely so the net worth chart has history. It upserts on
// (household_id, snapshot_date), which makes it safe to call on every scheduler
// tick: the day's row is simply rewritten with the latest figure.
//
// The date is passed in rather than using CURRENT_DATE so it comes from the
// household's timezone, matching how the scheduler decides what is due.
//
// Note the sign handling: liability balances are stored negative, so the debt
// total is negated to report as a positive number, and net worth is just the sum
// of every balance.
func (s *Service) SnapshotNetWorth(ctx context.Context, householdID int, day time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`WITH balances AS (
		   SELECT a.class,
		          a.starting_balance + COALESCE((
		            SELECT SUM(t.amount) FROM transactions t WHERE t.account_id = a.id
		          ), 0) AS balance
		   FROM accounts a
		   WHERE a.household_id = $1 AND a.archived_at IS NULL
		 ), totals AS (
		   SELECT COALESCE(SUM(balance) FILTER (WHERE class = 'asset'), 0)      AS assets,
		          COALESCE(-SUM(balance) FILTER (WHERE class = 'liability'), 0) AS liabilities
		   FROM balances
		 )
		 INSERT INTO net_worth_snapshots
		   (household_id, total_assets, total_liabilities, net_worth, snapshot_date)
		 SELECT $1, assets, liabilities, assets - liabilities, $2::date FROM totals
		 ON CONFLICT (household_id, snapshot_date) DO UPDATE SET
		   total_assets = EXCLUDED.total_assets,
		   total_liabilities = EXCLUDED.total_liabilities,
		   net_worth = EXCLUDED.net_worth`,
		householdID, day.Format(models.DateOnly))
	if err != nil {
		return fmt.Errorf("snapshotting net worth: %w", err)
	}
	return nil
}

func (s *Service) NetWorthHistory(ctx context.Context, householdID, limit int) ([]models.NetWorthPoint, error) {
	if limit <= 0 || limit > 3650 {
		limit = 365
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT snapshot_date, total_assets, total_liabilities, net_worth
		 FROM net_worth_snapshots
		 WHERE household_id = $1
		 ORDER BY snapshot_date DESC
		 LIMIT $2`,
		householdID, limit)
	if err != nil {
		return nil, fmt.Errorf("loading net worth history: %w", err)
	}
	defer rows.Close()

	// Selected newest-first so LIMIT keeps the most recent window, then reversed
	// for charting, which wants oldest-first.
	var points []models.NetWorthPoint
	for rows.Next() {
		var p models.NetWorthPoint
		var date time.Time
		if err := rows.Scan(&date, &p.TotalAssets, &p.TotalLiabilities, &p.NetWorth); err != nil {
			return nil, fmt.Errorf("scanning net worth point: %w", err)
		}
		p.Date = dateStr(date)
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]models.NetWorthPoint, 0, len(points))
	for i := len(points) - 1; i >= 0; i-- {
		out = append(out, points[i])
	}
	return out, nil
}

// Household is the minimum the scheduler needs to process one.
type Household struct {
	ID       int
	Timezone string
}

// Today returns the current calendar date in the household's own timezone.
//
// This has to be the household's zone, not the server's and not UTC. A rule due
// on the 25th should post once the 25th starts where the family lives — under
// UTC, an evening tick in Mountain time is already "tomorrow" and would post a
// day early; under a server-local zone the answer changes if the app moves.
func (h Household) Today() time.Time {
	loc, err := time.LoadLocation(h.Timezone)
	if err != nil {
		loc = time.UTC
	}
	y, m, d := time.Now().In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// Households returns every household, so the scheduler can iterate them without
// assuming there is only one.
func (s *Service) Households(ctx context.Context) ([]Household, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, timezone FROM households ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("listing households: %w", err)
	}
	defer rows.Close()

	var out []Household
	for rows.Next() {
		var h Household
		if err := rows.Scan(&h.ID, &h.Timezone); err != nil {
			return nil, fmt.Errorf("scanning household: %w", err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// DefaultHouseholdID returns the household every request is scoped to while auth
// is still a single shared password. Phase 2 replaces this with the household on
// the session; until then it is resolved once at boot.
func (s *Service) DefaultHouseholdID(ctx context.Context) (int, error) {
	var id int
	err := s.db.QueryRowContext(ctx, `SELECT id FROM households ORDER BY id LIMIT 1`).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("resolving default household: %w", err)
	}
	return id, nil
}
