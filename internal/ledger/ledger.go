// Package ledger holds every read and write against the family ledger.
//
// The repo has no ORM and no generic repository layer — SQL is written out by
// hand, as it was before the pivot. What changed is that the SQL moved out of the
// handlers: balance math, transfer pairing, and recurring posting are all needed
// by the HTTP layer AND the background scheduler, and duplicating them would be
// how the two drift apart.
//
// Every method takes a householdID and every query filters on it. There is only
// one household today (auth is still a single shared password), but writing the
// scope in from the start is what keeps adding real users an additive change.
package ledger

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"github.com/cwnelson/fangorn/internal/models"
)

// ErrNotFound is returned when a row does not exist, or exists but belongs to a
// different household. Those two cases are deliberately indistinguishable to the
// caller so the API cannot be used to probe for other households' row IDs.
var ErrNotFound = errors.New("not found")

// ErrInvalid marks a caller mistake — bad enum value, missing field, an amount of
// zero. Handlers turn this into a 400.
type ErrInvalid struct{ Msg string }

func (e ErrInvalid) Error() string { return e.Msg }

func invalid(format string, args ...any) error {
	return ErrInvalid{Msg: fmt.Sprintf(format, args...)}
}

// isUniqueViolation reports whether err is Postgres error 23505, so a duplicate
// name comes back as a 400 with a useful message instead of a 500.
func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}

type Service struct {
	db *sql.DB
}

func New(db *sql.DB) *Service {
	return &Service{db: db}
}

// DB exposes the underlying handle for the few callers that need their own
// transaction (the scheduler's advisory lock).
func (s *Service) DB() *sql.DB { return s.db }

// inTx runs fn inside a transaction, rolling back on error or panic.
func (s *Service) inTx(fn func(*sql.Tx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// nullStr converts an optional string to a NULL-able SQL value, treating "" as
// absent so the UI can clear a field by submitting a blank input.
func nullStr(s *string) any {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}

func nullInt(i *int) any {
	if i == nil {
		return nil
	}
	return *i
}

func strPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	v := ns.String
	return &v
}

func intPtr(ni sql.NullInt64) *int {
	if !ni.Valid {
		return nil
	}
	v := int(ni.Int64)
	return &v
}

func floatPtr(nf sql.NullFloat64) *float64 {
	if !nf.Valid {
		return nil
	}
	v := nf.Float64
	return &v
}

// dateStr renders a DATE column the way the API and the frontend both expect.
// lib/pq hands back a time.Time for DATE and TIMESTAMPTZ columns, so every read
// goes through here rather than scanning straight into a string.
func dateStr(t time.Time) string { return t.Format(models.DateOnly) }

func dateStrPtr(nt sql.NullTime) *string {
	if !nt.Valid {
		return nil
	}
	s := dateStr(nt.Time)
	return &s
}
