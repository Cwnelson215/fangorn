// Package scheduler posts recurring transactions when they come due and keeps
// the net worth history current.
//
// The design goal is that the scheduler is never the reason a number is wrong.
// It achieves that by being fully idempotent rather than by running reliably:
//
//   - Occurrences are materialized into recurring_occurrences, which has a
//     UNIQUE (rule_id, due_date). Re-materializing is a no-op.
//   - Posting flips an occurrence from 'scheduled' to 'posted' in the same
//     transaction that writes the ledger rows, guarded by the current status.
//   - Net worth snapshots upsert on (household_id, snapshot_date).
//
// Together those mean a tick can run twice, overlap with another process, or not
// run for a week, and the ledger still ends up in exactly the right state. That
// last case is the one that matters most in practice: the AWS deployment scales
// to zero overnight, so a rule due at 2 AM has to be picked up when the container
// next wakes rather than being silently missed.
package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
)

// advisoryLockKey is an arbitrary constant identifying this app's scheduler lock.
// Postgres advisory locks are global to the database, so the value only has to be
// stable and unlikely to collide with another app sharing the instance — which
// matters, since the k3s cluster runs one Postgres for everything.
const advisoryLockKey int64 = 0x66616e676f726e1 // "fangorn" + 1

type Scheduler struct {
	svc         *ledger.Service
	interval    time.Duration
	horizonDays int
}

func New(svc *ledger.Service, interval time.Duration, horizonDays int) *Scheduler {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	if horizonDays <= 0 {
		horizonDays = 60
	}
	return &Scheduler{svc: svc, interval: interval, horizonDays: horizonDays}
}

// Start runs a tick immediately, then on every interval until ctx is cancelled.
// The immediate run is what performs catch-up after downtime.
func (s *Scheduler) Start(ctx context.Context) {
	log.Printf("Scheduler starting (every %s, %d day horizon)", s.interval, s.horizonDays)
	s.Tick(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Scheduler stopped")
			return
		case <-ticker.C:
			s.Tick(ctx)
		}
	}
}

// Tick runs one full pass. It is exported so a manual trigger or a k8s CronJob
// can drive it too.
func (s *Scheduler) Tick(ctx context.Context) {
	conn, err := s.svc.DB().Conn(ctx)
	if err != nil {
		log.Printf("Scheduler: cannot acquire connection: %v", err)
		return
	}
	defer conn.Close()

	// Advisory locks are held per-connection, so the lock and the work must share
	// one connection — hence taking a *sql.Conn rather than using the pool.
	var locked bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, advisoryLockKey).Scan(&locked); err != nil {
		log.Printf("Scheduler: cannot take advisory lock: %v", err)
		return
	}
	if !locked {
		// Another process is mid-tick. Skipping is correct: the work is not
		// time-sensitive to the minute and the next tick will pick it up.
		return
	}
	defer func() {
		if _, err := conn.ExecContext(context.WithoutCancel(ctx),
			`SELECT pg_advisory_unlock($1)`, advisoryLockKey); err != nil {
			log.Printf("Scheduler: failed to release advisory lock: %v", err)
		}
	}()

	households, err := s.svc.Households(ctx)
	if err != nil {
		log.Printf("Scheduler: cannot list households: %v", err)
		return
	}

	for _, household := range households {
		if ctx.Err() != nil {
			return
		}
		s.runHousehold(ctx, household)
	}
}

func (s *Scheduler) runHousehold(ctx context.Context, household ledger.Household) {
	rules, err := s.svc.ListRules(ctx, household.ID)
	if err != nil {
		log.Printf("Scheduler: cannot list rules for household %d: %v", household.ID, err)
		return
	}

	// One "today" for the whole pass, in the household's timezone, so a tick that
	// straddles midnight can't process some rules as today and others as tomorrow.
	today := household.Today()

	posted := 0
	for _, rule := range rules {
		if rule.Paused {
			continue
		}
		n, err := s.processRule(ctx, rule, today)
		if err != nil {
			// One broken rule must not stop the others — log it and carry on.
			log.Printf("Scheduler: rule %d (%s): %v", rule.ID, rule.Name, err)
			continue
		}
		posted += n
	}

	if posted > 0 {
		log.Printf("Scheduler: posted %d recurring transaction(s) for household %d", posted, household.ID)
	}

	if err := s.svc.SnapshotNetWorth(ctx, household.ID, today); err != nil {
		log.Printf("Scheduler: net worth snapshot for household %d: %v", household.ID, err)
	}
}

// processRule materializes a rule's occurrences out to the horizon, then posts
// everything already due. It returns how many transactions it wrote.
func (s *Scheduler) processRule(ctx context.Context, rule models.RecurringRule, today time.Time) (int, error) {
	spec, err := ledger.RuleSpec(rule)
	if err != nil {
		return 0, err
	}

	horizon := today.AddDate(0, 0, s.horizonDays)

	start, err := models.ParseDate(rule.StartDate)
	if err != nil {
		return 0, err
	}

	// Generate from the rule's own start rather than from "today minus something",
	// so a rule created with a back-dated start backfills its whole history.
	dates := spec.Occurrences(start, horizon, 0)
	if err := s.materialize(ctx, rule.ID, dates); err != nil {
		return 0, err
	}

	if !rule.AutoPost {
		return 0, s.syncNextDue(ctx, rule.ID)
	}
	posted, err := s.postDue(ctx, rule, today)
	if err != nil {
		return posted, err
	}
	return posted, s.syncNextDue(ctx, rule.ID)
}

// materialize inserts occurrence rows, letting the unique constraint discard
// any that already exist.
func (s *Scheduler) materialize(ctx context.Context, ruleID int, dates []time.Time) error {
	for _, d := range dates {
		_, err := s.svc.DB().ExecContext(ctx,
			`INSERT INTO recurring_occurrences (rule_id, due_date)
			 VALUES ($1, $2)
			 ON CONFLICT (rule_id, due_date) DO NOTHING`,
			ruleID, d.Format(models.DateOnly))
		if err != nil {
			return err
		}
	}
	return nil
}

// postDue posts every scheduled occurrence dated on or before today.
func (s *Scheduler) postDue(ctx context.Context, rule models.RecurringRule, today time.Time) (int, error) {
	rows, err := s.svc.DB().QueryContext(ctx,
		`SELECT id, due_date FROM recurring_occurrences
		 WHERE rule_id = $1 AND status = 'scheduled' AND due_date <= $2
		 ORDER BY due_date`,
		rule.ID, today.Format(models.DateOnly))
	if err != nil {
		return 0, err
	}

	type due struct {
		id   int
		date time.Time
	}
	var pending []due
	for rows.Next() {
		var d due
		if err := rows.Scan(&d.id, &d.date); err != nil {
			rows.Close()
			return 0, err
		}
		pending = append(pending, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	posted := 0
	for _, d := range pending {
		if ctx.Err() != nil {
			return posted, ctx.Err()
		}
		err := s.svc.PostOccurrence(ctx, rule, d.id, d.date)
		switch {
		case err == nil:
			posted++
		case ledger.IsAlreadyPosted(err):
			// Lost a race with another worker. Nothing to do.
		case errors.Is(err, sql.ErrNoRows):
			log.Printf("Scheduler: occurrence %d vanished mid-post", d.id)
		default:
			return posted, err
		}
	}
	return posted, nil
}

// syncNextDue keeps recurring_rules.next_due_date in step with the earliest
// still-scheduled occurrence, so the UI can show it without recomputing dates.
func (s *Scheduler) syncNextDue(ctx context.Context, ruleID int) error {
	_, err := s.svc.DB().ExecContext(ctx,
		`UPDATE recurring_rules r
		 SET next_due_date = (
		   SELECT MIN(o.due_date) FROM recurring_occurrences o
		   WHERE o.rule_id = r.id AND o.status = 'scheduled'
		 )
		 WHERE r.id = $1
		   AND next_due_date IS DISTINCT FROM (
		     SELECT MIN(o.due_date) FROM recurring_occurrences o
		     WHERE o.rule_id = r.id AND o.status = 'scheduled'
		   )`, ruleID)
	return err
}
