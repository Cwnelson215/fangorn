# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Repo Is

A personal finance dashboard that connects to real bank accounts via the Plaid API, syncs transactions, categorizes spending, tracks income/expenses, calculates net worth, and visualizes financial data. Go backend, SvelteKit frontend with D3.js visualizations. Deployed on the portfolio platform as a containerized service on AWS ECS Fargate.

## Tech Stack

- **Backend:** Go, PostgreSQL (shared platform RDS)
- **Frontend:** SvelteKit, TypeScript, D3.js (data visualization)
- **External APIs:** Plaid (bank account linking, transaction sync)
- **Infrastructure:** Pulumi (TypeScript), AWS ECS Fargate, Secrets Manager

## Commands

```bash
# Application (Go backend)
go run ./cmd/server      # Run locally (http://localhost:3000)
go build ./cmd/server    # Build binary
go test ./...            # Run tests

# Frontend (SvelteKit)
cd frontend && npm run dev    # Dev server
cd frontend && npm run build  # Production build

# Infrastructure (Pulumi)
npm run preview          # Preview infra changes
npm run up               # Deploy infra
npm run destroy          # Tear down infra
```

## Architecture

**App contract:** The container must (1) listen on the configured port (default 3000) and (2) expose `GET /health` returning HTTP 200.

**Infrastructure (`index.ts`):** Defines app-specific AWS resources:
- ECR repository (`portfolio/fangorn`) with lifecycle policy (keep last 10 images)
- Security group allowing traffic from the shared ALB
- ALB target group + host-based listener rule (`fangorn.cwnel.com`)
- ECS Fargate task definition + service (Fargate Spot by default)
- Secrets Manager entries for Plaid API credentials and encryption key
- Scheduled scaling (scale to zero at 10 PM, up at 6 AM Mountain)

All shared resources (VPC, ALB, ECS cluster, Route53, ACM, CloudWatch log group, RDS) come from the platform stack and are imported via `pulumi.StackReference`.

## Key Files

- `cmd/server/` — Go server entry point
- `internal/` — Go application code (handlers, services, models, plaid client)
- `frontend/` — SvelteKit app with D3.js visualizations
- `index.ts` — Pulumi infrastructure definition
- `Pulumi.yaml` — Project metadata
- `Pulumi.dev.yaml` — Environment config
- `Dockerfile` — Multi-stage Go build
- `.github/workflows/deploy.yml` — CI/CD pipeline

## Conventions

- **Naming:** Resources prefixed with `appName`. All tagged with Project, App, ManagedBy.
- **Config:** Environment-specific values in `Pulumi.{stack}.yaml`. Secrets via `pulumi config set --secret`.
- **Logs:** CloudWatch at `/ecs/portfolio-dev/fangorn`, 14-day retention.
- **Platform stack reference:** `cwnelson/portfolio-platform/dev`
- **Health check:** `GET /health` must return HTTP 200.
- **Environment variables injected by infra:** `PORT`, `PLAID_ENV`, `DB_HOST`, `DB_PORT`, `DB_NAME`, `DB_USER`, `DB_PASSWORD`, `PLAID_CLIENT_ID`, `PLAID_SECRET`, `ENCRYPTION_KEY`

## Security Notes

- Plaid access tokens (returned after account linking) must be encrypted at rest using `ENCRYPTION_KEY` before storing in PostgreSQL
- Never log Plaid access tokens or financial data
- PII (account numbers, balances) should be treated as sensitive — no client-side caching in localStorage
