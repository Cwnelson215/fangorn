# Deprecated: bank-sync era code

Fangorn started as a bank-sync app: it linked real accounts through the Teller API, imported CSV
statements, and scraped bank notification emails out of Gmail. In August 2026 the scope changed to a
**hand-kept family ledger** — balances start from a number you type in, and every transaction is
logged by hand.

None of the code here is compiled. The directory name starts with `_`, which the Go toolchain
ignores outright, so `go build ./...`, `go vet ./...`, and `go test ./...` skip it. The Svelte routes
are outside `frontend/src/routes`, so SvelteKit does not see them either. It is kept readable in
case any of it is wanted again.

## What's here

| Path | Was |
|---|---|
| `backend/services/teller.go` | Teller API client — mTLS cert loading, HTTP Basic auth, accounts/balances/transactions |
| `backend/services/sync.go` | `LinkInstitution` / `SyncAll` — pulled Teller data into the DB, encrypting access tokens |
| `backend/services/gmail.go` | Per-account Gmail watcher: OAuth2 refresh-token client, polled `from:(…) after:…`, parsed transaction emails |
| `backend/services/csvimport.go` | CSV import service — bank-format registry rehydration, dedupe, account auto-creation |
| `backend/csvimport/` | CSV parsing domain: parser registry, generic column-mapping parser, date/amount helpers |
| `backend/emailparse/` | Email parser interface + registry. **The registry was always empty** — no bank parsers were ever written, so the Gmail pipeline was wired but inert |
| `backend/handlers/{link,sync,config}.go` | HTTP layer for Teller linking, manual sync, and handing `teller_app_id` to the Teller Connect widget |
| `backend/handlers/csvimport.go` | Upload / detect-headers / save-format endpoints |
| `frontend/routes/link/` | Teller Connect page (loaded `cdn.teller.io/connect/connect.js`) |
| `frontend/routes/import/` | CSV upload + "add new bank format" wizard — the most elaborate form in the app |

## What it would take to revive any of it

- **Teller** needs the mTLS client certs at `teller/certificate.pem` and `teller/private_key.pem`.
  Those are `.gitignore`d and exist only on the machine that set them up — they are not in this
  repo and not in the deprecated tree. It also needs `TELLER_APP_ID`, `TELLER_ENV`,
  `TELLER_CERT_PATH`, `TELLER_KEY_PATH`, and `ENCRYPTION_KEY` back in `internal/config`.
- **Gmail** needs a Google OAuth client (`client_secret_*.json`, also gitignored) plus
  `GMAIL_{n}_{CLIENT_ID,CLIENT_SECRET,REFRESH_TOKEN,SENDER_FILTERS}`.
- **All of it** targets the pre-`006` schema: `linked_institutions`, `csv_imports`,
  `gmail_watch_state`, `bank_csv_formats`, and the old shapes of `accounts` / `transactions`.
  Migration `006_family_ledger` drops every one of those. Reviving this code means writing a new
  migration, not reverting one.
- `internal/crypto` (AES-256-GCM) is still in the live tree — `sync.go` was its only caller, but it
  was kept for future use.

## Known bugs, recorded so they aren't rediscovered

1. **`sync.go` would fail at runtime.** Migration `004` replaced the unique *constraints* on
   `accounts.external_account_id` and `transactions.external_id` with *partial* unique indexes
   (`WHERE ... IS NOT NULL`), but `sync.go` still uses bare `ON CONFLICT (external_id)`. Postgres
   requires the predicate to match, so those statements raise *"there is no unique or exclusion
   constraint matching the ON CONFLICT specification"*. `csvimport.go` and `gmail.go` got this
   right. Teller sync was behind a disabled feature flag, which is why it never surfaced.
2. **Gmail watchers raced on one row.** `gmail_watch_state` has no per-account column, but `main.go`
   started one goroutine per configured account. Every watcher did
   `SELECT … ORDER BY id LIMIT 1` and then `UPDATE` the same row, so N accounts shared — and
   clobbered — a single `last_polled_at` cursor.
3. `sync.go` set `official_name` to a copy of `name` rather than the institution's official name.
4. `handlers/link.go` passed the Teller *enrollment* ID into `LinkInstitution`'s `institutionID`
   parameter, so `linked_institutions.institution_id` held the wrong value.
