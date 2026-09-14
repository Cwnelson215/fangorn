// Package testdb gives database-backed tests a migrated Postgres to run against.
//
// Tests that use it are skipped unless FANGORN_TEST_DSN is set, so `go test ./...`
// stays green on a machine with no database. Point it at a disposable instance —
// migration 006 drops tables on first run:
//
//	docker run -d --rm --name fangorn-test-pg -p 55432:5432 \
//	  -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=fangorn_test postgres:16-alpine
//	export FANGORN_TEST_DSN="host=localhost port=55432 user=postgres password=postgres dbname=fangorn_test sslmode=disable"
//
// Isolation comes from tenancy rather than truncation: every test gets its own
// household, and every ledger query is scoped to one. That lets tests (and test
// packages) share a database concurrently, and it exercises the scoping for free.
package testdb

import (
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"

	_ "github.com/lib/pq"

	"github.com/cwnelson/fangorn/internal/database"
)

const envDSN = "FANGORN_TEST_DSN"

var (
	once    sync.Once
	shared  *sql.DB
	openErr error
)

// Open returns the shared, migrated test database, or skips the test when
// FANGORN_TEST_DSN is unset.
func Open(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv(envDSN)
	if dsn == "" {
		t.Skipf("%s not set; skipping database test", envDSN)
	}

	once.Do(func() {
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			openErr = fmt.Errorf("opening test database: %w", err)
			return
		}
		if err := db.Ping(); err != nil {
			openErr = fmt.Errorf("pinging test database: %w", err)
			return
		}
		if err := database.RunMigrations(db); err != nil {
			openErr = err
			return
		}
		shared = db
	})
	if openErr != nil {
		t.Fatal(openErr)
	}
	return shared
}

// Household creates a fresh household for one test and returns its id. It is
// removed when the test ends; ON DELETE CASCADE takes everything under it.
func Household(t *testing.T, db *sql.DB, timezone string) int {
	t.Helper()
	var id int
	err := db.QueryRow(
		`INSERT INTO households (name, timezone) VALUES ($1, $2) RETURNING id`,
		t.Name(), timezone,
	).Scan(&id)
	if err != nil {
		t.Fatalf("creating household: %v", err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(`DELETE FROM households WHERE id = $1`, id); err != nil {
			t.Errorf("removing household %d: %v", id, err)
		}
	})
	return id
}
