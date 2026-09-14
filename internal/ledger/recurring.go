package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/cwnelson/fangorn/internal/models"
	"github.com/cwnelson/fangorn/internal/recurring"
)

const ruleSelect = `
	SELECT r.id, r.household_id, r.name, r.vendor, r.kind, r.account_id, a.name,
	       r.to_account_id, ta.name, r.category_id, c.name, r.amount, r.frequency,
	       r.interval_count, r.day_of_month, r.second_day_of_month, r.day_of_week,
	       r.month_of_year, r.start_date, r.end_date, r.next_due_date, r.auto_post,
	       r.reminder_lead_days, r.paused_at IS NOT NULL, r.notes
	FROM recurring_rules r
	JOIN accounts a ON a.id = r.account_id
	LEFT JOIN accounts ta ON ta.id = r.to_account_id
	LEFT JOIN categories c ON c.id = r.category_id`

func scanRule(rows interface{ Scan(...any) error }) (models.RecurringRule, error) {
	var r models.RecurringRule
	var vendor, toAccountName, categoryName, notes sql.NullString
	var toAccountID, categoryID sql.NullInt64
	var dayOfMonth, secondDay, dayOfWeek, monthOfYear, leadDays sql.NullInt64
	var startDate time.Time
	var endDate, nextDue sql.NullTime

	err := rows.Scan(
		&r.ID, &r.HouseholdID, &r.Name, &vendor, &r.Kind, &r.AccountID, &r.AccountName,
		&toAccountID, &toAccountName, &categoryID, &categoryName, &r.Amount, &r.Frequency,
		&r.IntervalCount, &dayOfMonth, &secondDay, &dayOfWeek, &monthOfYear,
		&startDate, &endDate, &nextDue, &r.AutoPost, &leadDays, &r.Paused, &notes,
	)
	if err != nil {
		return r, err
	}

	r.Vendor = strPtr(vendor)
	r.ToAccountID = intPtr(toAccountID)
	r.ToAccountName = strPtr(toAccountName)
	r.CategoryID = intPtr(categoryID)
	r.CategoryName = strPtr(categoryName)
	r.DayOfMonth = intPtr(dayOfMonth)
	r.SecondDayOfMonth = intPtr(secondDay)
	r.DayOfWeek = intPtr(dayOfWeek)
	r.MonthOfYear = intPtr(monthOfYear)
	r.ReminderLeadDays = intPtr(leadDays)
	r.Notes = strPtr(notes)
	r.StartDate = dateStr(startDate)
	r.EndDate = dateStrPtr(endDate)
	r.NextDueDate = dateStrPtr(nextDue)
	return r, nil
}

func (s *Service) ListRules(ctx context.Context, householdID int) ([]models.RecurringRule, error) {
	rows, err := s.db.QueryContext(ctx,
		ruleSelect+` WHERE r.household_id = $1
		 ORDER BY r.paused_at IS NOT NULL, r.next_due_date NULLS LAST, r.name`,
		householdID)
	if err != nil {
		return nil, fmt.Errorf("listing recurring rules: %w", err)
	}
	defer rows.Close()

	out := []models.RecurringRule{}
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning recurring rule: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Service) GetRule(ctx context.Context, householdID, id int) (models.RecurringRule, error) {
	row := s.db.QueryRowContext(ctx,
		ruleSelect+` WHERE r.household_id = $1 AND r.id = $2`, householdID, id)

	r, err := scanRule(row)
	if err == sql.ErrNoRows {
		return r, ErrNotFound
	}
	if err != nil {
		return r, fmt.Errorf("fetching recurring rule: %w", err)
	}
	return r, nil
}

type RuleInput struct {
	Name             string  `json:"name"`
	Vendor           *string `json:"vendor"`
	Kind             string  `json:"kind"`
	AccountID        int     `json:"account_id"`
	ToAccountID      *int    `json:"to_account_id"`
	CategoryID       *int    `json:"category_id"`
	Amount           float64 `json:"amount"`
	Frequency        string  `json:"frequency"`
	IntervalCount    int     `json:"interval_count"`
	DayOfMonth       *int    `json:"day_of_month"`
	SecondDayOfMonth *int    `json:"second_day_of_month"`
	DayOfWeek        *int    `json:"day_of_week"`
	MonthOfYear      *int    `json:"month_of_year"`
	StartDate        string  `json:"start_date"`
	EndDate          *string `json:"end_date"`
	AutoPost         *bool   `json:"auto_post"`
	ReminderLeadDays *int    `json:"reminder_lead_days"`
	Notes            *string `json:"notes"`
}

func (in *RuleInput) normalize() (recurring.Rule, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return recurring.Rule{}, invalid("name is required")
	}
	switch in.Kind {
	case models.KindIncome, models.KindExpense:
		if in.ToAccountID != nil {
			return recurring.Rule{}, invalid("only transfer rules take a destination account")
		}
	case models.KindTransfer:
		if in.ToAccountID == nil {
			return recurring.Rule{}, invalid("a transfer rule needs a destination account")
		}
		if *in.ToAccountID == in.AccountID {
			return recurring.Rule{}, invalid("source and destination must be different accounts")
		}
	default:
		return recurring.Rule{}, invalid("kind must be income, expense, or transfer")
	}
	if in.AccountID <= 0 {
		return recurring.Rule{}, invalid("account_id is required")
	}
	if math.Abs(in.Amount) < 0.005 {
		return recurring.Rule{}, invalid("amount must be greater than zero")
	}
	in.Amount = math.Abs(in.Amount)
	if in.IntervalCount < 1 {
		in.IntervalCount = 1
	}

	start, err := models.ParseDate(in.StartDate)
	if err != nil {
		return recurring.Rule{}, invalid("start_date must be YYYY-MM-DD")
	}

	rule := recurring.Rule{
		Frequency:        in.Frequency,
		IntervalCount:    in.IntervalCount,
		DayOfMonth:       in.DayOfMonth,
		SecondDayOfMonth: in.SecondDayOfMonth,
		DayOfWeek:        in.DayOfWeek,
		MonthOfYear:      in.MonthOfYear,
		StartDate:        start,
	}
	if in.EndDate != nil && *in.EndDate != "" {
		end, err := models.ParseDate(*in.EndDate)
		if err != nil {
			return recurring.Rule{}, invalid("end_date must be YYYY-MM-DD")
		}
		rule.EndDate = &end
	} else {
		in.EndDate = nil
	}

	// Reuse the date engine's own validation so the API rejects exactly what the
	// scheduler would refuse to schedule.
	if err := rule.Validate(); err != nil {
		return recurring.Rule{}, invalid("%s", err.Error())
	}
	return rule, nil
}

func (s *Service) CreateRule(ctx context.Context, householdID int, in RuleInput) (models.RecurringRule, error) {
	spec, err := in.normalize()
	if err != nil {
		return models.RecurringRule{}, err
	}
	if err := s.assertAccount(ctx, householdID, in.AccountID); err != nil {
		return models.RecurringRule{}, err
	}
	if in.ToAccountID != nil {
		if err := s.assertAccount(ctx, householdID, *in.ToAccountID); err != nil {
			return models.RecurringRule{}, err
		}
	}
	if err := s.assertCategory(ctx, householdID, in.CategoryID); err != nil {
		return models.RecurringRule{}, err
	}

	autoPost := true
	if in.AutoPost != nil {
		autoPost = *in.AutoPost
	}

	var nextDue any
	if first, ok := spec.First(); ok {
		nextDue = first.Format(models.DateOnly)
	}

	var id int
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO recurring_rules
		   (household_id, name, vendor, kind, account_id, to_account_id, category_id,
		    amount, frequency, interval_count, day_of_month, second_day_of_month,
		    day_of_week, month_of_year, start_date, end_date, next_due_date,
		    auto_post, reminder_lead_days, notes)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		 RETURNING id`,
		householdID, in.Name, nullStr(in.Vendor), in.Kind, in.AccountID,
		nullInt(in.ToAccountID), nullInt(in.CategoryID), in.Amount, in.Frequency,
		in.IntervalCount, nullInt(in.DayOfMonth), nullInt(in.SecondDayOfMonth),
		nullInt(in.DayOfWeek), nullInt(in.MonthOfYear), in.StartDate,
		nullStr(in.EndDate), nextDue, autoPost, nullInt(in.ReminderLeadDays),
		nullStr(in.Notes),
	).Scan(&id)
	if err != nil {
		return models.RecurringRule{}, fmt.Errorf("creating recurring rule: %w", err)
	}
	return s.GetRule(ctx, householdID, id)
}

func (s *Service) UpdateRule(ctx context.Context, householdID, id int, in RuleInput) (models.RecurringRule, error) {
	spec, err := in.normalize()
	if err != nil {
		return models.RecurringRule{}, err
	}
	if err := s.assertAccount(ctx, householdID, in.AccountID); err != nil {
		return models.RecurringRule{}, err
	}
	if in.ToAccountID != nil {
		if err := s.assertAccount(ctx, householdID, *in.ToAccountID); err != nil {
			return models.RecurringRule{}, err
		}
	}
	if err := s.assertCategory(ctx, householdID, in.CategoryID); err != nil {
		return models.RecurringRule{}, err
	}

	autoPost := true
	if in.AutoPost != nil {
		autoPost = *in.AutoPost
	}

	// The schedule may have moved, so recompute next_due_date from the last date
	// actually posted rather than trusting the stored value.
	lastPosted, err := s.LastHandledOccurrence(ctx, id)
	if err != nil {
		return models.RecurringRule{}, err
	}

	var nextDue any
	if next, ok := nextAfterPosted(spec, lastPosted); ok {
		nextDue = next.Format(models.DateOnly)
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE recurring_rules SET
		   name = $1, vendor = $2, kind = $3, account_id = $4, to_account_id = $5,
		   category_id = $6, amount = $7, frequency = $8, interval_count = $9,
		   day_of_month = $10, second_day_of_month = $11, day_of_week = $12,
		   month_of_year = $13, start_date = $14, end_date = $15, next_due_date = $16,
		   auto_post = $17, reminder_lead_days = $18, notes = $19, updated_at = NOW()
		 WHERE household_id = $20 AND id = $21`,
		in.Name, nullStr(in.Vendor), in.Kind, in.AccountID, nullInt(in.ToAccountID),
		nullInt(in.CategoryID), in.Amount, in.Frequency, in.IntervalCount,
		nullInt(in.DayOfMonth), nullInt(in.SecondDayOfMonth), nullInt(in.DayOfWeek),
		nullInt(in.MonthOfYear), in.StartDate, nullStr(in.EndDate), nextDue,
		autoPost, nullInt(in.ReminderLeadDays), nullStr(in.Notes), householdID, id,
	)
	if err != nil {
		return models.RecurringRule{}, fmt.Errorf("updating recurring rule: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.RecurringRule{}, ErrNotFound
	}

	// Occurrences not yet posted may no longer be on the schedule at all; drop
	// them so the scheduler rebuilds the future from the new definition. Posted
	// and skipped rows are history and stay put.
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM recurring_occurrences WHERE rule_id = $1 AND status = 'scheduled'`,
		id); err != nil {
		return models.RecurringRule{}, fmt.Errorf("clearing future occurrences: %w", err)
	}

	return s.GetRule(ctx, householdID, id)
}

// LastHandledOccurrence returns the due date of a rule's latest posted or
// skipped occurrence — the point before which its schedule is settled history.
// Invalid means nothing has been handled yet.
//
// Both the scheduler and UpdateRule start from here rather than from start_date.
// Once a rule's dates have changed, the new schedule's past dates are not in
// recurring_occurrences, so the unique key cannot stop them from being posted;
// this boundary is what does.
func (s *Service) LastHandledOccurrence(ctx context.Context, ruleID int) (sql.NullTime, error) {
	var last sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT MAX(due_date) FROM recurring_occurrences
		 WHERE rule_id = $1 AND status <> 'scheduled'`, ruleID).Scan(&last)
	if err != nil {
		return last, fmt.Errorf("reading rule history: %w", err)
	}
	return last, nil
}

func nextAfterPosted(spec recurring.Rule, lastPosted sql.NullTime) (time.Time, bool) {
	if lastPosted.Valid {
		return spec.Next(lastPosted.Time)
	}
	return spec.First()
}

func (s *Service) SetRulePaused(ctx context.Context, householdID, id int, paused bool) error {
	q := `UPDATE recurring_rules SET paused_at = NULL, updated_at = NOW() WHERE household_id = $1 AND id = $2`
	if paused {
		q = `UPDATE recurring_rules SET paused_at = NOW(), updated_at = NOW() WHERE household_id = $1 AND id = $2`
	}
	res, err := s.db.ExecContext(ctx, q, householdID, id)
	if err != nil {
		return fmt.Errorf("pausing rule: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) DeleteRule(ctx context.Context, householdID, id int) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM recurring_rules WHERE household_id = $1 AND id = $2`, householdID, id)
	if err != nil {
		return fmt.Errorf("deleting recurring rule: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// occurrences
// ---------------------------------------------------------------------------

// Upcoming lists occurrences due between today and today+days that have not been
// posted yet. This is what the dashboard's "coming up" panel reads.
func (s *Service) Upcoming(ctx context.Context, householdID, days int) ([]models.Occurrence, error) {
	if days <= 0 {
		days = 30
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT o.id, o.rule_id, r.name, r.kind, r.amount, a.name, ta.name,
		        o.due_date, o.status, o.transaction_id
		 FROM recurring_occurrences o
		 JOIN recurring_rules r ON r.id = o.rule_id
		 JOIN accounts a ON a.id = r.account_id
		 LEFT JOIN accounts ta ON ta.id = r.to_account_id
		 WHERE r.household_id = $1
		   AND o.status = 'scheduled'
		   AND o.due_date <= CURRENT_DATE + ($2 || ' days')::interval
		 ORDER BY o.due_date, r.name`,
		householdID, days)
	if err != nil {
		return nil, fmt.Errorf("listing upcoming occurrences: %w", err)
	}
	defer rows.Close()

	out := []models.Occurrence{}
	for rows.Next() {
		var o models.Occurrence
		var toAccount sql.NullString
		var txnID sql.NullInt64
		var due time.Time
		if err := rows.Scan(&o.ID, &o.RuleID, &o.RuleName, &o.Kind, &o.Amount,
			&o.AccountName, &toAccount, &due, &o.Status, &txnID); err != nil {
			return nil, fmt.Errorf("scanning occurrence: %w", err)
		}
		o.DueDate = dateStr(due)
		o.ToAccountName = strPtr(toAccount)
		o.TransactionID = intPtr(txnID)
		out = append(out, o)
	}
	return out, rows.Err()
}

// SkipNext marks the earliest unposted occurrence as skipped, so a one-off
// cancellation does not require pausing or editing the whole rule.
func (s *Service) SkipNext(ctx context.Context, householdID, ruleID int) error {
	if _, err := s.GetRule(ctx, householdID, ruleID); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE recurring_occurrences SET status = 'skipped'
		 WHERE id = (
		   SELECT id FROM recurring_occurrences
		   WHERE rule_id = $1 AND status = 'scheduled'
		   ORDER BY due_date LIMIT 1
		 )`, ruleID)
	if err != nil {
		return fmt.Errorf("skipping occurrence: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return invalid("this rule has no upcoming occurrence to skip")
	}
	return s.refreshNextDue(ctx, ruleID)
}

// PostNow posts the earliest unposted occurrence immediately, regardless of its
// due date — the manual override for "this charge landed early".
func (s *Service) PostNow(ctx context.Context, householdID, ruleID int) (models.Occurrence, error) {
	rule, err := s.GetRule(ctx, householdID, ruleID)
	if err != nil {
		return models.Occurrence{}, err
	}

	var occID int
	var due time.Time
	err = s.db.QueryRowContext(ctx,
		`SELECT id, due_date FROM recurring_occurrences
		 WHERE rule_id = $1 AND status = 'scheduled'
		 ORDER BY due_date LIMIT 1`, ruleID).Scan(&occID, &due)
	if err == sql.ErrNoRows {
		return models.Occurrence{}, invalid("this rule has no upcoming occurrence to post")
	}
	if err != nil {
		return models.Occurrence{}, fmt.Errorf("finding occurrence: %w", err)
	}

	if err := s.PostOccurrence(ctx, rule, occID, due); err != nil {
		return models.Occurrence{}, err
	}
	if err := s.refreshNextDue(ctx, ruleID); err != nil {
		return models.Occurrence{}, err
	}

	return models.Occurrence{
		ID: occID, RuleID: ruleID, RuleName: rule.Name,
		DueDate: dateStr(due), Status: models.OccurrencePosted,
	}, nil
}

// PostOccurrence turns one scheduled occurrence into real transactions.
//
// The whole thing runs in a single transaction, and the final UPDATE is guarded
// by `status = 'scheduled'`. If two schedulers race on the same occurrence, one
// commits and the other finds zero rows affected and rolls back — so a charge
// posts exactly once even without the advisory lock.
func (s *Service) PostOccurrence(ctx context.Context, rule models.RecurringRule, occurrenceID int, due time.Time) error {
	dueStr := due.Format(models.DateOnly)

	return s.inTx(func(tx *sql.Tx) error {
		var txnID sql.NullInt64

		if rule.Kind == models.KindTransfer {
			if rule.ToAccountID == nil {
				return fmt.Errorf("transfer rule %d has no destination account", rule.ID)
			}
			var groupID string
			if err := tx.QueryRowContext(ctx, `SELECT gen_random_uuid()::text`).Scan(&groupID); err != nil {
				return fmt.Errorf("generating transfer id: %w", err)
			}
			in := TransferInput{
				FromAccountID: rule.AccountID,
				ToAccountID:   *rule.ToAccountID,
				Amount:        rule.Amount,
				Date:          dueStr,
				Description:   rule.Name,
			}
			if err := insertTransferLegs(ctx, tx, rule.HouseholdID, groupID, in, &rule.ID); err != nil {
				return err
			}
			// Link the occurrence to the source leg; deleting either leg removes both.
			err := tx.QueryRowContext(ctx,
				`SELECT id FROM transactions
				 WHERE transfer_group_id = $1 AND amount < 0`, groupID).Scan(&txnID)
			if err != nil {
				return fmt.Errorf("locating transfer leg: %w", err)
			}
		} else {
			amount := rule.Amount
			if rule.Kind == models.KindExpense {
				amount = -amount
			}
			err := tx.QueryRowContext(ctx,
				`INSERT INTO transactions
				   (household_id, account_id, date, amount, kind, description, merchant,
				    category_id, recurring_rule_id, source)
				 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'recurring')
				 RETURNING id`,
				rule.HouseholdID, rule.AccountID, dueStr, amount, rule.Kind,
				rule.Name, nullStr(rule.Vendor), nullInt(rule.CategoryID), rule.ID,
			).Scan(&txnID)
			if err != nil {
				return fmt.Errorf("posting recurring transaction: %w", err)
			}
		}

		res, err := tx.ExecContext(ctx,
			`UPDATE recurring_occurrences
			 SET status = 'posted', transaction_id = $1, posted_at = NOW()
			 WHERE id = $2 AND status = 'scheduled'`,
			txnID, occurrenceID)
		if err != nil {
			return fmt.Errorf("marking occurrence posted: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			// Another worker got there first. Roll back the transactions we just
			// wrote rather than double-posting.
			return errAlreadyPosted
		}
		return nil
	})
}

// errAlreadyPosted signals a lost race on an occurrence. It is expected, not
// exceptional — callers roll back and move on.
var errAlreadyPosted = fmt.Errorf("occurrence already posted")

// IsAlreadyPosted reports whether an error came from losing a posting race.
func IsAlreadyPosted(err error) bool { return err == errAlreadyPosted }

// refreshNextDue recomputes a rule's next_due_date from its earliest remaining
// scheduled occurrence, falling back to the date engine when none are
// materialized yet.
func (s *Service) refreshNextDue(ctx context.Context, ruleID int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE recurring_rules r
		 SET next_due_date = (
		   SELECT MIN(o.due_date) FROM recurring_occurrences o
		   WHERE o.rule_id = r.id AND o.status = 'scheduled'
		 ), updated_at = NOW()
		 WHERE r.id = $1`, ruleID)
	if err != nil {
		return fmt.Errorf("refreshing next due date: %w", err)
	}
	return nil
}

// RuleSpec converts a stored rule into the pure date-engine form.
func RuleSpec(r models.RecurringRule) (recurring.Rule, error) {
	start, err := models.ParseDate(r.StartDate)
	if err != nil {
		return recurring.Rule{}, fmt.Errorf("rule %d has an unparseable start date: %w", r.ID, err)
	}
	spec := recurring.Rule{
		Frequency:        r.Frequency,
		IntervalCount:    r.IntervalCount,
		DayOfMonth:       r.DayOfMonth,
		SecondDayOfMonth: r.SecondDayOfMonth,
		DayOfWeek:        r.DayOfWeek,
		MonthOfYear:      r.MonthOfYear,
		StartDate:        start,
	}
	if r.EndDate != nil {
		end, err := models.ParseDate(*r.EndDate)
		if err != nil {
			return recurring.Rule{}, fmt.Errorf("rule %d has an unparseable end date: %w", r.ID, err)
		}
		spec.EndDate = &end
	}
	return spec, nil
}
