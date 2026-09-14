# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Repo Is

A **family finance app kept on paper**. You enter each account's starting balance, and everyone in
the household logs transactions by hand from there — nothing connects to a bank. It tracks accounts
and running balances, categorized income and spending, transfers between your own accounts,
recurring subscriptions and scheduled transfers that post themselves, monthly category budgets,
savings goals, net worth over time, and investment accounts whose holdings (stocks, ETFs, mutual
funds) are priced automatically. Go backend, SvelteKit frontend with D3 visualizations.

It **used** to sync real accounts through the Teller API, import CSV statements, and scrape bank
notification emails out of Gmail. That code is not deleted — it lives in `_deprecated/`, which the
Go toolchain ignores because the directory name starts with `_`. See `_deprecated/README.md`.

## Tech Stack

- **Backend:** Go (stdlib `net/http`, `database/sql` + `lib/pq`, `golang-migrate`) — no ORM, no
  router library, no DI framework. Raw SQL, written out.
- **Frontend:** SvelteKit (Svelte 5 runes), TypeScript, D3. Static adapter — a pure SPA that the Go
  binary embeds and serves. No Tailwind, no component library.
- **Database:** PostgreSQL
- **Infrastructure:** Pulumi (TypeScript), AWS ECS Fargate — see "Migration status" below

## Commands

```bash
docker compose up -d          # Postgres for local dev
source .env && go run ./cmd/server   # backend on :3000
go test ./...                 # DB-backed ledger/scheduler tests skip unless FANGORN_TEST_DSN is set
cd frontend && npm run dev    # Vite on :5173, proxies /api and /health to :3000
cd frontend && npm run check  # svelte-check — keep this clean
cd frontend && npm run build  # required before `go build`; the binary embeds frontend/build
```

`internal/ledger` and `internal/scheduler` tests run against real Postgres. Use a **disposable**
instance, never the dev volume — migration `006` drops tables:

```bash
docker run -d --rm --name fangorn-test-pg -p 55432:5432 \
  -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=fangorn_test postgres:16-alpine
export FANGORN_TEST_DSN="host=localhost port=55432 user=postgres password=postgres dbname=fangorn_test sslmode=disable"
go test ./...
```

Tests isolate by giving each one its own household (`internal/testdb`), not by truncating, so they
can share the database concurrently. Scheduler tests call `runHousehold` rather than `Tick`, which
would sweep every test's household.

## The Conventions That Matter

**1. Amounts are signed relative to the account.** Positive = money in, negative = money out.
Liability accounts (`credit_card`, `loan`) therefore carry **negative** balances. This makes both of
these correct for every account type with no special-casing:

```
balance   = starting_balance + SUM(amount)
net worth = SUM(balance)
```

The API accepts a **positive magnitude** and applies the sign from `kind` — nobody logging groceries
should have to type a minus sign. Same for account starting balances: the form asks a credit card
for "amount owed" as a positive number and negates it.

**2. A transfer is two transaction rows, not a table.** Both share a `transfer_group_id`: a negative
leg on the source, a positive leg on the destination. Each side appears in its own account's
register for free, and income/expense totals everywhere exclude `kind = 'transfer'` — moving your
own money is neither earning nor spending it. Mutations go through `ledger.CreateTransfer` /
`UpdateTransfer` / `DeleteTransfer`, which operate on the whole group in one transaction. Editing a
single leg through `/api/transactions` is rejected; deleting one leg deletes both.

**3. A trade is a `trades` row plus a cash leg.** In an `investment` account, a buy or sell also
writes one `kind = 'trade'` transaction on the same account (`transactions.trade_id`, composite FK
so it can't sit on another account). Cash therefore stays `starting_balance + SUM(amount)`, and
income/expense totals list kinds explicitly (`kind IN ('income','expense')`) so trades stay out.
`reinvest` and `opening` trades add shares with no leg. Share counts are never stored: every trade
write locks the account row and replays the whole log through `portfolio.Replay`, rejecting any sell
that exceeds shares held on its date. `trades.amount` is the real dollar figure (a "$500 of FZROX"
order isn't exactly shares × price), and cost basis comes from it.

An account's worth is defined **once**, in `ledger/balances.go` (`accountBalances`): cash plus net
shares × `securities.last_price`. `accountSelect`, `SnapshotNetWorth` and `goalSelect` all join it —
don't recompute a balance anywhere else.

## Architecture

**App contract:** listen on `PORT` (default 3000), expose `GET /health` (dependency-free, 200) and
`GET /ready` (pings the DB, 503 when it is down).

```
cmd/server/main.go        wiring: config -> db -> migrate -> ledger service -> routes -> scheduler
internal/config/          env -> Config
internal/database/        Connect, RunMigrations, embedded migrations/
internal/models/          domain types + the enum constants; ClassForType
internal/recurring/       PURE date engine — no DB, no clock. The best-tested code here.
internal/portfolio/       PURE trade-log math — replay, average cost, exact cent rounding
internal/quotes/          price Provider interface + Yahoo client (network, no DB)
internal/prices/          Refresher: decides when a price is stale, fetches, saves, backs off
internal/ledger/          every read and write against the ledger
internal/scheduler/       posts due recurring items, refreshes prices, snapshots net worth
internal/handlers/        HTTP layer
internal/middleware/      auth, CORS, logging
internal/crypto/          AES-GCM (unused today; kept for future invite tokens)
internal/testdb/          test-only: migrated Postgres + a fresh household per test
frontend/                 SvelteKit SPA
_deprecated/              the bank-sync era, not compiled
```

**`internal/ledger` is where SQL lives.** Handlers hold the service, not `*sql.DB`. That split
exists because balance math, transfer pairing, and recurring posting are needed by both the HTTP
layer and the background scheduler, and duplicating them is how the two drift apart.

**Tenancy:** every table carries `household_id` and every query filters on it — except
`securities` and `security_prices`, which are shared public market data (who holds what lives in
`trades`, which is scoped normally). There is one
household today and `main.go` resolves it once at boot, but the scoping is written in so adding real
users is additive rather than a rewrite.

## The Recurring Engine

`internal/recurring` computes occurrence dates by **index from a fixed anchor**, never by adding an
interval to the previous occurrence. That is what makes month-end rules behave: a rule anchored on
the 31st fires Feb 28, then returns to Mar 31 rather than sticking at the 28th. It also means no
accumulated drift. Supports daily, weekly, biweekly, semimonthly, monthly, quarterly, yearly, each
with an interval multiplier.

`internal/scheduler` runs on a ticker (`SCHEDULER_INTERVAL`, default 5m) with an immediate pass on
boot, under a `pg_try_advisory_lock`. It is **idempotent rather than reliable**:

- `recurring_occurrences` has `UNIQUE (rule_id, due_date)` — re-materializing is a no-op
- posting flips `scheduled` → `posted` in the same transaction that writes the ledger rows, guarded
  by the current status, so a lost race rolls back instead of double-posting
- net worth snapshots upsert on `(household_id, snapshot_date)`
- occurrences are only materialized **after the rule's last posted/skipped date**
  (`ledger.LastHandledOccurrence`). The unique key alone is not enough: editing a rule changes its
  dates, and the new schedule's past dates are not in the table yet. Without this boundary, moving a
  monthly charge from the 1st to the 15th back-posted every 15th since `start_date`.

So a tick can run twice, overlap another process, or not run for a week, and the ledger still lands
correct. That last case is not hypothetical: the AWS deployment scales to zero overnight
(`Pulumi.dev.yaml: enableScheduledScaling`), so anything due at 2 AM has to be picked up when the
container next wakes. "Today" is computed in the **household's** timezone, not the server's.

## Investment Prices

Prices come from Yahoo Finance's unofficial chart/search endpoints (no key) behind
`quotes.Provider`. `quotes.ErrNotFound` means "no such symbol" (a 400 when logging a trade);
`quotes.ErrUnavailable` is everything else, and callers fall back to stored prices. A symbol is seeded
from its first trade's price, so trades still work when Yahoo is down.

`prices.Refresher` is called by the scheduler (per household, before the net worth snapshot) and by
`GET /api/accounts/{id}/holdings`, which the account page polls every 60s. Staleness is judged from
the DB, so they don't duplicate fetches: stocks/ETFs `QUOTES_MARKET_TTL` (1m) during US market hours,
15m otherwise; mutual funds 15m always (one NAV a day); failed symbols back off 5m. HTTP-triggered
refreshes get a 4–5s budget, well inside the 15s `WriteTimeout`.

Yahoo quirks: the full Chrome User-Agent got 429s while `Mozilla/5.0` didn't; day change is derived
from `regularMarketChangePercent` because a fund's latest NAV is often dated the next morning, and
`chartPreviousClose` is the close before the *range*, not before today.

## Migrations

`internal/database/migrations/NNN_snake_case.{up,down}.sql`, embedded and applied at every boot.
`001`–`005` are the bank-sync era, kept as history. **`006_family_ledger` is the base schema** —
it drops everything from before and creates households, accounts, categories, transactions,
recurring_rules, recurring_occurrences, budgets, goals, goal_contributions, net_worth_snapshots.
`007_investments` adds securities, security_prices, trades, and the `trade` transaction kind.

## Conventions

- **Errors:** `ledger.ErrNotFound` → 404, `ledger.ErrInvalid` → 400 with its message passed through
  to the user, anything else → logged in full, 500 with a generic message. All via `handlers.fail`.
- **Ownership:** foreign keys do not know about tenancy, so `assertAccount` / `assertCategory` check
  household scope before any write that references them.
- **JSON:** request decoding uses `DisallowUnknownFields` so a typo'd field name fails loudly.
- **Dates:** `YYYY-MM-DD` everywhere, on the wire and in the DB. `lib/pq` returns `time.Time` for
  DATE columns, so reads go through `ledger.dateStr`.
- **Frontend:** shared formatters in `src/lib/format.ts`, design tokens as CSS custom properties in
  `+layout.svelte`, form primitives in `src/lib/components/{Field,Button,Modal}.svelte`. Use them
  rather than re-declaring `Intl.NumberFormat` or hex colours per page.
- **Health check:** `GET /health` must return 200.
- **Env vars:** `PORT`, `DB_HOST`, `DB_PORT`, `DB_NAME`, `DB_USER`, `DB_PASSWORD`, `DB_SSLMODE`,
  `APP_PASSWORD`, `SCHEDULER_INTERVAL`, `SCHEDULER_HORIZON_DAYS`, `QUOTES_PROVIDER` (`yahoo` default,
  or `none`), `QUOTES_MARKET_TTL`

## Auth

Still a **single shared password** (`APP_PASSWORD`) with an HMAC cookie, unchanged from before the
pivot. Note what it is not: the cookie value is a constant, identical for every login forever, with
no expiry and no revocation. If `APP_PASSWORD` is unset, auth is disabled entirely.

Real accounts — email/password users, email invites, and passkey-or-PIN device unlock — are planned
but not built. The schema is already shaped for it.

## Gotchas

- **The production image has no zoneinfo.** `cmd/server/main.go` imports `time/tzdata`; without it
  `LoadLocation` fails in the alpine container and both the household's "today" and US market hours
  silently fall back to UTC.
- **Tests that touch `securities` must use their own symbols** (e.g. suffix the household id).
  That table is global, and DB tests run concurrently against one database.

- **`frontend/build` must exist before `go build`.** `frontend.go` embeds it with
  `//go:embed all:frontend/build`, so a stale or missing build silently ships the wrong UI.
- **Quote values in the Postgres DSN.** `lib/pq` skips whitespace after `=`, so an unquoted empty
  value swallows the next token — `password= dbname=fangorn` parses as `password="dbname=fangorn"`
  with no dbname, and libpq then connects to a database named after the user. `Config.DSN` quotes
  and escapes every value; `internal/config/config_test.go` guards it.
- **`Dockerfile` must match `go.mod`'s Go version.** They drifted once (1.22 vs 1.26) and the image
  build failed.
- **GHCR images are public.** `.dockerignore` excludes `.env*`, `teller/`, `client_secret*.json`,
  `*.pem`, `*.key` — `Dockerfile` does `COPY . .`, and the GHA cache exports intermediate layers.

## Migration Status

Still on AWS ECS Fargate via Pulumi (`index.ts`), fronted by the shared platform ALB at
`fangorn.cwnel.com`. **Moving to the k3s cluster** (`bulbasaur`) is planned but not started — it
matters here because the ECS service scales to zero 10 PM–6 AM, which the scheduler's catch-up
design already tolerates. Per the rule in `~/Dev/portfolio/CLAUDE.md`, the AWS stack stays
authoritative until a k3s deploy is proven.
