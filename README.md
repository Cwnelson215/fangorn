# Fangorn

A family finance app kept by hand. Enter each account's balance once, and everyone in the household
logs what comes in and goes out from their phone — nothing connects to a bank. It tracks balances,
categorized spending, budgets, savings goals, recurring bills, net worth over time and investment
accounts, and it reads photographed receipts with Claude so most purchases log themselves.

It's built phone-first: installed to the home screen it opens straight to a one-screen quick log, a
receipt is one tap from any page, and on iPhone two Shortcuts log an expense or snap a receipt from
a widget, the Action button or Siri without opening the app at all.

## Features

- **Accounts and balances** — checking, savings, cash, credit cards, loans and investment accounts,
  each starting from a balance you enter. Transfers between your own accounts move money without
  counting as income or spending.
- **Quick log** — amount, a category chip (most-used first), Save. The account you used last is
  remembered on each phone, and Undo is right there.
- **Receipts** — take a photo and Claude reads the total, date, merchant, card and category. A
  receipt posts itself only when nothing about it is doubtful — the card's last four digits match an
  account, the category matches one of yours, the totals add up, no likely duplicate — and waits for
  review otherwise.
- **Budgets and goals** — monthly limits per category with pace warnings, refunds that come back off
  the budget instead of counting as income, and savings goals tracked against an account or by hand.
- **Recurring items** — subscriptions, paychecks and scheduled transfers that post themselves on
  their due date, including month-end rules that fall back to Feb 28 and return to the 31st.
- **Investments** — log buys and sells; stocks, ETFs and mutual funds are priced automatically and
  valued over time.
- **Dashboard** — net worth and its history, money in and out, spending by category, upcoming bills.
- **iPhone Shortcuts** — per-phone keys, made and revoked in the app, let a Shortcut log an expense
  or send a receipt photo. A key can't read anything back.

## Tech stack

- **Backend:** Go — standard library `net/http`, `database/sql` with `lib/pq`, `golang-migrate`.
  No ORM, router or framework; the SQL is written out.
- **Frontend:** SvelteKit (Svelte 5) as a static SPA, TypeScript, D3 for charts. The Go binary
  embeds and serves it, so the app ships as one binary.
- **Database:** PostgreSQL.
- **AI:** the Claude API (Messages API with structured output) for reading receipts.
- **Prices:** Yahoo Finance.
- **Hosting:** a self-hosted k3s cluster — GitHub Actions builds the image to GHCR and applies
  kustomize manifests over Tailscale; Postgres runs under CloudNativePG; TLS from Let's Encrypt.

## Running it locally

Prerequisites: Go (the version in `go.mod`), Node 22, and Docker for Postgres.

```bash
docker compose up -d                 # Postgres on :5432
cp .env.example .env                 # then adjust; see below
cd frontend && npm ci && npm run build && cd ..
source .env && go run ./cmd/server   # http://localhost:3000
```

Migrations run automatically at startup; the first run creates the household and a starter set of
categories. For frontend work, `cd frontend && npm run dev` serves the UI with hot reload on
http://localhost:5173 and proxies `/api` to the Go server.

The frontend must be built before `go build` or `go run` — the binary embeds `frontend/build`, so a
missing or stale build means the wrong UI.

### Configuration

| Variable | Default | |
|---|---|---|
| `PORT` | `3000` | |
| `DB_HOST` / `DB_PORT` / `DB_NAME` / `DB_USER` / `DB_PASSWORD` | `localhost` / `5432` / `fangorn` / `postgres` / — | |
| `DB_SSLMODE` | `require` | Use `disable` for local Postgres |
| `APP_PASSWORD` | — | The shared household login. **Unset turns login off entirely** — fine locally, never in production |
| `RECEIPTS_PROVIDER` | `none` | `anthropic` to read receipts; with `none`, photos are stored for you to enter by hand |
| `ANTHROPIC_API_KEY` | — | Required when `RECEIPTS_PROVIDER=anthropic` |
| `RECEIPTS_MODEL` | `claude-opus-5` | |
| `QUOTES_PROVIDER` | `yahoo` | `none` to turn off price lookups |
| `QUOTES_MARKET_TTL` | `1m` | How stale a stock price may get during market hours |
| `SCHEDULER_INTERVAL` | `5m` | How often recurring items post, prices refresh and net worth is snapshotted |
| `SCHEDULER_HORIZON_DAYS` | `60` | How far ahead upcoming recurring items are shown |

## Tests

```bash
go test ./...                        # backend
cd frontend && npm run check         # svelte-check / TypeScript
cd frontend && npm test              # vitest
```

The ledger, scheduler and receipt tests need a real Postgres and are skipped unless
`FANGORN_TEST_DSN` is set. Point it at a **disposable** instance — never your dev database; the
migrations drop tables:

```bash
docker run -d --rm --name fangorn-test-pg -p 55432:5432 \
  -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=fangorn_test postgres:16-alpine
export FANGORN_TEST_DSN="host=localhost port=55432 user=postgres password=postgres dbname=fangorn_test sslmode=disable"
go test ./...
```

Each test gets its own household rather than a truncated database, so they run concurrently.

## Project layout

```
cmd/server/          startup: config, database, migrations, routes, scheduler
internal/ledger/     every read and write against the ledger — all the SQL lives here
internal/recurring/  the pure date engine for recurring items (no database, no clock)
internal/portfolio/  pure trade-log math: replay, cost basis, value history
internal/receipts/   deciding whether a read receipt posts itself or waits for review
internal/vision/     the Claude API client that reads receipt photos
internal/scheduler/  posts due items, refreshes prices, finishes receipts, snapshots net worth
internal/handlers/   the HTTP API
frontend/            the SvelteKit app
k8s/                 kustomize manifests for the cluster
_deprecated/         the old bank-sync code (Teller, CSV import, Gmail), kept but not compiled
```

A few rules the code is built around:

- **Amounts are signed per account** — positive is money in, negative is money out — so a credit
  card carries a negative balance and net worth is a plain sum.
- **A transfer is two linked rows**, one per account, so each side shows in its own register and
  totals leave transfers out.
- **The scheduler is idempotent rather than reliable.** It can run twice, overlap itself or miss a
  week and the ledger still comes out right — a home server does go down.
- **Model output is untrusted.** Claude is given only the photo, today's date and category names;
  every value it returns is cleaned and matched against the household's own accounts and categories.

## Deployment

Every push to `main` runs the full test suite in GitHub Actions — including the database tests
against a Postgres service container — then builds the image to
`ghcr.io/cwnelson215/fangorn`, joins the tailnet and applies `k8s/overlays/prod` to the cluster.
Pull requests run the tests only.

The workflow needs four repository secrets: `KUBECONFIG`, `TS_AUTHKEY`, `APP_PASSWORD` and
`ANTHROPIC_API_KEY`. It refuses to deploy without the last two. The namespace, database and its
credentials are provisioned once on the cluster and aren't touched by CI.

The image is public, so `.dockerignore` keeps `.env*`, keys and any receipt photos out of the build.
