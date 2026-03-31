# TriageGuard MVP v1

TriageGuard converts Slack messages from selected channels into managed Requests with ownership, SLA enforcement, escalations, and optional Linear sync.

## Monorepo structure

- `apps/api`: Go API for Slack webhooks, admin API, OAuth callbacks, SLA jobs
- `apps/web`: Next.js landing + admin console
- `supabase/migrations`: SQL schema migrations
- `turbo.json`: Turborepo task pipeline
- `docs/product-logic.md`: detailed product logic, lifecycle, integrations, and functional behavior
- `docs/security-checklist.md`: production security hardening checklist

## MVP scope implemented

- Monorepo workspace orchestration via Turborepo
- Slack OAuth install + callback
- Slack Events API intake (`message.channels` + `message.groups`, root messages only)
- Triage Card in thread with actions: Ack, Assign, Priority, Due date, Ignore, Convert to Linear
- Linked Linear issues sync status/assignee/due in both directions (Slack ↔ Linear)
- SLA engine: Ack overdue, Assign overdue, stale reminders, daily digest
- Slack lifecycle cleanup (`app_uninstalled`, `tokens_revoked` for bot token revocation)
- Admin API + Next.js pages:
  - `/` Landing
  - `/login`
  - `/auth/callback`
  - `/auth/reset-password`
  - `/onboarding`
  - `/dashboard`
  - `/channels`
  - `/policies`
  - `/linear`
  - `/billing`
  - `/activity`
  - `/privacy`
  - `/support`
- Supabase Postgres schema for workspaces/installations/channels/policies/requests/actions/linear links
- Web stack: Tailwind CSS + shadcn/ui + TanStack Query + Zustand

## Launch-safety hardening

- Stripe webhook idempotency by `event.id` (`stripe_webhook_events` dedupe table)
- Linear OAuth lifecycle supports refresh token rotation and automatic access token refresh
- Workspace data deletion endpoint (`POST /api/workspace/delete` with `{"confirm":"DELETE"}`)
- Public privacy/support pages linked from landing footer
- Public endpoint rate limiting for Slack/webhook/job routes
- Retry/backoff for Slack and Linear outbound API requests (429/5xx/transient network)
- Dead-letter capture for failed Linear sync paths (`dead_letters` table)
- Sentry-ready error capture for API 5xx and web runtime errors (`SENTRY_DSN`)
- Alerting hooks for 5xx spikes and dead-letter spikes (`ALERT_WEBHOOK_URL`)

## Auth architecture (httpOnly, BFF)

- Browser never sends Supabase JWT directly to Go API.
- Next.js route handlers (`/api/auth/*`, `/api/backend/*`) manage auth/session server-side.
- Session tokens are stored in `httpOnly` cookies (`tg_at`, `tg_rt`, `tg_exp`).
- Access token refresh is done server-side when token is near expiry.
- If refresh fails, cookies are cleared and client is redirected to `/login`.
- Browser API calls use same-origin endpoints only:
  - Auth: `/api/auth/*`
  - Backend proxy to Go: `/api/backend/*`
- Workspace access is resolved from Supabase JWT `sub` via `workspace_members` (multi-tenant safe by user membership).

## Slack app setup checklist

1. Create Slack app at [https://api.slack.com/apps](https://api.slack.com/apps)
2. Enable OAuth & Permissions
3. Add Redirect URL:
   - `${API_BASE_URL}/slack/oauth/callback`
4. Add bot scopes:
- `chat:write`
- `channels:read`
- `channels:history`
- `channels:join`
- `groups:read`
- `groups:history`
- `users:read`
- `users:read.email` (required to map Slack assignee to Linear assignee by email)
5. Enable Event Subscriptions
6. Set Request URL:
   - `${API_BASE_URL}/slack/events`
7. Subscribe to bot event:
   - `message.channels`
   - `message.groups`
   - `app_uninstalled`
   - `tokens_revoked`
8. Enable Interactivity
9. Set Interactivity Request URL:
   - `${API_BASE_URL}/slack/actions`
10. Install app to workspace

## Environment variables

### API (`apps/api/.env`)

Use `apps/api/.env.example` as base:

```bash
API_BASE_URL=http://localhost:8080
WEB_BASE_URL=http://localhost:3000
SENTRY_DSN=
ALERT_WEBHOOK_URL=
ALERT_5XX_THRESHOLD=20
ALERT_5XX_WINDOW_MINUTES=5
ALERT_5XX_COOLDOWN_MINUTES=20
ALERT_DEAD_LETTER_THRESHOLD=3
ALERT_DEAD_LETTER_WINDOW_MINUTES=10
SUPABASE_URL=https://your-project-ref.supabase.co
SUPABASE_DB_URL=postgres://postgres:postgres@localhost:54322/postgres?sslmode=disable
SUPABASE_JWT_SECRET=replace-me
OAUTH_STATE_SECRET=replace-me-with-strong-random-secret
BILLING_SIGNING_SECRET= # optional, defaults to OAUTH_STATE_SECRET
BILLING_PROVIDER=stripe # stripe | paypal
TOKENS_ENCRYPTION_KEY= # optional in dev, recommended in production (base64/hex decoded to 32 bytes)
JOBS_SECRET=replace-me-for-manual-jobs-triggers
RETENTION_ENABLED=true
RETENTION_TERMINAL_DAYS=180
RETENTION_DEDUPE_DAYS=30
RETENTION_BATCH_SIZE=500
RETENTION_INTERVAL_MINUTES=60
STRIPE_SECRET_KEY=
STRIPE_WEBHOOK_SECRET=
STRIPE_TEAM_PAYMENT_LINK=
STRIPE_ENTERPRISE_PAYMENT_LINK=
STRIPE_TEAM_PRICE_ID= # optional but recommended for plan detection in webhooks
STRIPE_ENTERPRISE_PRICE_ID=  # optional but recommended for plan detection in webhooks
STRIPE_PORTAL_RETURN_URL=http://localhost:3000/billing
PAYPAL_TEAM_PAYMENT_LINK=
PAYPAL_ENTERPRISE_PAYMENT_LINK=
PAYPAL_BUSINESS_EMAIL=
SLACK_CLIENT_ID=
SLACK_CLIENT_SECRET=
SLACK_SIGNING_SECRET=
LINEAR_CLIENT_ID=
LINEAR_CLIENT_SECRET=
LINEAR_REDIRECT_URI=http://localhost:8080/linear/oauth/callback
LINEAR_WEBHOOK_SECRET=
PORT=8080
```

Retention defaults:
- terminal requests (`RESOLVED` / `IGNORED`) older than `RETENTION_TERMINAL_DAYS` are deleted in batches
- webhook/action dedupe rows older than `RETENTION_DEDUPE_DAYS` are deleted
- runner executes every `RETENTION_INTERVAL_MINUTES`

### Web (`apps/web/.env.local`)

Use `apps/web/.env.example` as base:

```bash
NEXT_PUBLIC_SUPABASE_URL=http://127.0.0.1:54321
NEXT_PUBLIC_SUPABASE_ANON_KEY=replace-me
NEXT_PUBLIC_API_BASE_URL=http://localhost:8080
NEXT_PUBLIC_SITE_URL=https://triageguard.app
SENTRY_DSN=
```

Billing provider notes:
- `BILLING_PROVIDER=stripe` keeps the current Stripe webhook-driven flow.
- `BILLING_PROVIDER=paypal` switches checkout to PayPal links (`PAYPAL_TEAM_PAYMENT_LINK`, optional `PAYPAL_ENTERPRISE_PAYMENT_LINK`) without removing Stripe code.
- In PayPal mode, billing portal endpoint is disabled.
- You can keep both Stripe and PayPal variables in `.env` and switch provider by changing only `BILLING_PROVIDER` + API restart.
- For manual grants (paid beta), use script:
  - `scripts/billing/grant-subscription.sh --workspace-id <uuid> --plan team --provider paypal --months 1`
  - Or by email: `scripts/billing/grant-subscription.sh --user-email owner@company.com --provider paypal --months 1`

Observability notes:
- Set `SENTRY_DSN` to enable Sentry-compatible error capture in API and web runtime telemetry route.
- Set `ALERT_WEBHOOK_URL` to receive operational alerts (5xx spike, dead-letter spike).
- Alert tuning:
  - `ALERT_5XX_THRESHOLD`
  - `ALERT_5XX_WINDOW_MINUTES`
  - `ALERT_5XX_COOLDOWN_MINUTES`
  - `ALERT_DEAD_LETTER_THRESHOLD`
  - `ALERT_DEAD_LETTER_WINDOW_MINUTES`

Stripe setup notes:
- `STRIPE_TEAM_PAYMENT_LINK` / `STRIPE_ENTERPRISE_PAYMENT_LINK` are used for checkout redirects.
- App appends signed `client_reference_id` to payment links so webhook can map payment to workspace.
- Configure Stripe webhook endpoint:
  - `POST ${API_BASE_URL}/webhooks/stripe`
  - Recommended events:
    - `checkout.session.completed`
    - `customer.subscription.created`
    - `customer.subscription.updated`
    - `customer.subscription.deleted`

Linear webhook setup notes:
- Configure endpoint:
  - `POST ${API_BASE_URL}/webhooks/linear`
- Set webhook signing secret from Linear app settings in:
  - `LINEAR_WEBHOOK_SECRET`
- Subscribe webhook to `Issue` updates (create/update/remove).
- Resulting behavior for linked requests:
  - Linear `completed` -> Slack `RESOLVED`
  - Linear `canceled` -> Slack `IGNORED`
  - Linear active state from closed -> Slack `REOPEN`
  - Assignee/Due changes sync both ways where identity mapping is possible.

## Supabase setup (step-by-step)

### 1) Project API values (for `.env.local` and API `.env`)

Supabase Dashboard -> `Project Settings` -> `API`:

1. Copy `Project URL` -> `NEXT_PUBLIC_SUPABASE_URL`.
2. Copy `anon public key` -> `NEXT_PUBLIC_SUPABASE_ANON_KEY`.
3. Copy `Project URL` -> `SUPABASE_URL` (for Go API JWKS verification).
4. Copy `JWT secret` -> `SUPABASE_JWT_SECRET` (required only if your project uses HS256 tokens).
5. Copy DB connection string -> `SUPABASE_DB_URL`.

Important for hosted Supabase:
- Prefer the `Session pooler` DB URL (`*.pooler.supabase.com`) for `SUPABASE_DB_URL`.
- Using direct host `db.<project-ref>.supabase.co:5432` can fail on networks without IPv6 routing.
- If you see `prepared statement "stmtcache_*" already exists (SQLSTATE 42P05)`, switch to pooler URL and restart API.

### 2) Auth provider settings (email+password)

Supabase Dashboard -> `Authentication` -> `Providers` -> `Email`:

1. Enable Email provider.
2. Enable email/password sign-in.
3. For your current setup (no SMTP), disable `Confirm email`.
4. Configure SMTP later, then re-enable `Confirm email` for production.

### 3) URL configuration (required)

Supabase Dashboard -> `Authentication` -> `URL Configuration`:

1. `Site URL`:
   - Local: `http://localhost:3000`
   - Production: your real domain.
2. `Redirect URLs` add all used paths:
   - `http://localhost:3000/login`
   - `http://localhost:3000/auth/callback`
   - `http://localhost:3000/auth/reset-password`
   - `https://<your-prod-domain>/login`
   - `https://<your-prod-domain>/auth/callback`
   - `https://<your-prod-domain>/auth/reset-password`

### 4) Rate limits and protections

Supabase Dashboard -> `Authentication` -> `Rate Limits` and `Security`:

1. Increase email send limits for real usage or configure custom SMTP.
2. Keep sign-in rate limits enabled.
3. Enable CAPTCHA / bot detection for signup/login/reset if available.
4. Enable leaked password protection / password checks if available on your plan.

### 5) Password reset redirect

Supabase uses redirect URL from request; this app sends:

- `http://localhost:3000/auth/reset-password` (local)
- `https://<your-prod-domain>/auth/reset-password` (production)

Make sure both are in allowed Redirect URLs.

Implemented web auth flows:

- Password sign in (`/login`)
- Password sign up (instant session when `Confirm email` is disabled)
- Optional email verification flow (`/auth/callback`) if you re-enable `Confirm email`
- Forgot password email (`/login`, reset mode)
- Set new password via recovery link (`/auth/reset-password`)
- Server-side session exchange from hash tokens to `httpOnly` cookies (`/api/auth/set-session`)
- Client-side password policy guard (12+ chars, upper/lower/number/symbol)
- Client-side login cooldown after repeated failures

## Database migrations

Schema migration:

- `supabase/migrations/001_init.sql`
- `supabase/migrations/002_prod_hardening.sql`
- `supabase/migrations/003_billing_subscriptions.sql`
- `supabase/migrations/004_launch_safety.sql`
- `supabase/migrations/005_dual_billing_provider.sql`
- `supabase/migrations/006_private_channels.sql`
- `supabase/migrations/007_linear_webhook_sync.sql`
- `supabase/migrations/008_enterprise_plans_and_channel_sla.sql`

Apply with Supabase CLI:

```bash
supabase db push
```

Or apply manually with `psql`:

```bash
psql "$SUPABASE_DB_URL" -f supabase/migrations/001_init.sql
psql "$SUPABASE_DB_URL" -f supabase/migrations/002_prod_hardening.sql
psql "$SUPABASE_DB_URL" -f supabase/migrations/003_billing_subscriptions.sql
psql "$SUPABASE_DB_URL" -f supabase/migrations/004_launch_safety.sql
psql "$SUPABASE_DB_URL" -f supabase/migrations/005_dual_billing_provider.sql
psql "$SUPABASE_DB_URL" -f supabase/migrations/006_private_channels.sql
psql "$SUPABASE_DB_URL" -f supabase/migrations/007_linear_webhook_sync.sql
psql "$SUPABASE_DB_URL" -f supabase/migrations/008_enterprise_plans_and_channel_sla.sql
```

## Local development

### 0) Install monorepo deps

```bash
pnpm install
```

### 1) API

```bash
pnpm --filter @triageguard/api dev
```

### 2) Web

```bash
pnpm --filter @triageguard/web dev
```

### 3) Expose API to Slack (ngrok)

```bash
ngrok http 8080
```

Set `API_BASE_URL` to your ngrok HTTPS URL and update Slack app URLs accordingly.

### Turborepo (run all apps)

```bash
pnpm dev
```

`apps/web` runs with `next dev`.

## API Docker deploy (`apps/api`)

Build image:

```bash
docker build -f apps/api/Dockerfile -t triageguard-api:latest apps/api
```

Run container with API env:

```bash
docker run --rm -p 8080:8080 --env-file apps/api/.env --name triageguard-api triageguard-api:latest
```

Or with Compose:

```bash
docker compose -f apps/api/docker-compose.yml up -d --build
```

Health checks:

```bash
curl -s http://localhost:8080/livez   # process is up
curl -s http://localhost:8080/readyz  # process + DB dependency ready
```

Backward-compatible alias: `GET /healthz` behaves like `/readyz`.

Notes:
- Container listens on `PORT` (default `8080`).
- Keep real secrets only in runtime env/secret manager; do not bake `.env` into image.
- If Slack/Linear callbacks point to public URL, set `API_BASE_URL` to your deployed HTTPS domain.

## SLA behavior and quick test guide

How SLA works now:
- A background runner executes every 60 seconds (`cmd/server` starts it automatically).
- It checks four rules: `Ack overdue`, `Assign overdue`, `Stale`, `Daily digest`.
- Alerts are deduplicated per request using DB flags (`*_sent_at`).
- SLA notifications are sent only for paid workspaces (`team/enterprise` with active-like status).
- Per-channel SLA overrides are Enterprise-only (`/api/channels/overrides`).

Fast local test (recommended):
1. In Admin -> `Policies`, set:
   - `ack_sla_minutes=1`
   - `assign_sla_minutes=2`
   - `stale_hours=1`
   - set `escalation_channel_id` to a real channel where bot is present.
2. Ensure the target channel is enabled in `Channels`.
3. Send a root message in that channel.
4. Trigger SLA immediately (instead of waiting next minute tick):

```bash
curl -X POST http://localhost:8080/jobs/sla \
  -H "X-Jobs-Secret: $JOBS_SECRET"
```

Manual retention run:

```bash
curl -X POST http://localhost:8080/jobs/retention \
  -H "X-Jobs-Secret: $JOBS_SECRET"
```

Expected results:
- No action on request -> `⚠️ Ack overdue` in thread + escalation channel.
- `Acknowledge` but no owner -> after assign window, `⚠️ Assign overdue`.
- Assigned request with no activity -> after stale window, stale ping (`owner` mention if set).
- Digest sends once per day at `daily_digest_time` in policy timezone.

Tip for stale/digest testing without waiting long:
- Keep normal policy values and manually adjust timestamps in DB test data, then call `/jobs/sla`.
- Runner is idempotent-safe (single advisory lock) and won’t duplicate notifications if flags are already set.

## Production audit runbook

Use the built-in QA runner and checklist before release:

```bash
./scripts/qa/run-audit.sh local
```

For staging smoke:

```bash
STAGING_API_BASE_URL=https://api.your-staging-domain ./scripts/qa/run-audit.sh staging
```

Automated backup/restore verification:

```bash
./scripts/qa/check-backup-restore.sh
```

Automated staging smoke:

```bash
STAGING_WEB_BASE_URL=https://staging.triageguard.app \
STAGING_API_BASE_URL=https://staging-api.triageguard.app \
STAGING_TEST_EMAIL=qa@triageguard.app \
STAGING_TEST_PASSWORD='***' \
./scripts/qa/staging-smoke.sh
```

Artifacts:

- `docs/qa/production-audit-checklist.md`
- `docs/qa/latest-audit-report.md`

GitHub automation:

- `.github/workflows/backup-restore-check.yml`
  - requires secret: `SUPABASE_DB_URL`
- `.github/workflows/staging-smoke.yml`
  - requires secrets: `STAGING_WEB_BASE_URL`, `STAGING_API_BASE_URL`, `STAGING_TEST_EMAIL`, `STAGING_TEST_PASSWORD`
  - optional repo variable: `STAGING_EXPECT_PAID` (`true` by default)

## Main API endpoints

Public:

- `GET /livez`
- `GET /readyz` (`/healthz` alias)
- `GET /slack/install`
- `GET /slack/oauth/callback`
- `POST /slack/events`
- `POST /slack/actions`
- `GET /linear/oauth/callback`
- `POST /webhooks/stripe`
- `POST /webhooks/linear`
- `POST /jobs/sla` (manual trigger, requires `X-Jobs-Secret`; endpoint is disabled if `JOBS_SECRET` is empty)
- `POST /jobs/retention` (manual trigger, requires `X-Jobs-Secret`; endpoint is disabled if `JOBS_SECRET` is empty)

Private (Bearer Supabase JWT):

- `GET /api/slack/install`
- `GET /api/linear/install`
- `GET /api/me`
- `GET /api/channels`
- `POST /api/channels/sync`
- `PUT /api/channels`
- `GET /api/channels/overrides`
- `PUT /api/channels/overrides`
- `POST /api/slack/disconnect`
- `GET /api/policies`
- `PUT /api/policies`
- `GET /api/linear`
- `GET /api/linear/teams`
- `PUT /api/linear/defaults`
- `POST /api/linear/disconnect`
- `GET /api/billing`
- `POST /api/billing/checkout`
- `POST /api/billing/portal`
- `POST /api/billing/paypal/confirm`
- `GET /api/requests`
- `GET /api/requests/summary` (`?days=7|30|90`, adds analytics block)
- `GET /api/reports/requests.csv` (`?days=7|30|90`, Enterprise only)
- `GET /api/reports/analytics.csv` (`?days=7|30|90`, Enterprise only)
- `GET /api/activity`
- `GET /api/dead-letters`

Web auth/BFF routes (Next.js):

- `POST /api/auth/login`
- `POST /api/auth/signup`
- `POST /api/auth/resend-verification`
- `POST /api/auth/forgot-password`
- `POST /api/auth/set-session`
- `POST /api/auth/update-password`
- `POST /api/auth/logout`
- `GET /api/auth/me`
- `POST /api/telemetry/error`
- `ALL /api/backend/*` (proxy to Go API with server-side bearer injection)

## Notes

- Slack bot token and Linear token are encrypted-at-rest when `TOKENS_ENCRYPTION_KEY` is configured.
- Existing plaintext tokens remain readable for backward compatibility; new writes are encrypted when key is present.
- Slack request signature verification is enforced for events/actions.
- OAuth callbacks (`Slack`, `Linear`) now require signed `state` + nonce cookie verification.
- Billing checkout links include signed workspace reference and are reconciled by Stripe webhooks.
- Only root messages in enabled channels are converted to Requests (public + private).
- Private channels require explicit bot membership (`/invite @TriageGuard`) to receive events and post triage cards.
- Team plan enforces up to 10 enabled channels; Enterprise is unlimited.
- Daily digest is deduplicated per workspace/day using `last_digest_sent_at`.
- Linear webhook troubleshooting:
  - `Linear session expired/revoked`: reconnect Linear in Admin Console.
  - `missing LINEAR_WEBHOOK_SECRET`: endpoint returns `404`.
  - `assignee not matched`: keep owner unchanged and check Slack/Linear email parity.
