#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MODE="${1:-local}"

cd "$ROOT_DIR"

echo "== TriageGuard Production Audit =="
echo "Mode: $MODE"
echo "Timestamp (UTC): $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
echo

if [[ "$MODE" != "local" && "$MODE" != "staging" ]]; then
  echo "Usage: ./scripts/qa/run-audit.sh [local|staging]"
  exit 1
fi

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1"
    exit 1
  fi
}

require_cmd pnpm
require_cmd awk
require_cmd curl
require_cmd openssl

echo "== Step 1: Static quality gates =="
pnpm lint
pnpm build
echo

echo "== Step 2: Environment key presence (no secret values printed) =="
if [[ ! -f apps/api/.env ]]; then
  echo "apps/api/.env not found"
  exit 1
fi
if [[ ! -f apps/web/.env.local ]]; then
  echo "apps/web/.env.local not found"
  exit 1
fi

api_keys=(
  API_BASE_URL
  WEB_BASE_URL
  SUPABASE_DB_URL
  SUPABASE_URL
  SUPABASE_JWT_SECRET
  OAUTH_STATE_SECRET
  JOBS_SECRET
  TOKENS_ENCRYPTION_KEY
  SLACK_CLIENT_ID
  SLACK_CLIENT_SECRET
  SLACK_SIGNING_SECRET
  LINEAR_CLIENT_ID
  LINEAR_CLIENT_SECRET
  LINEAR_REDIRECT_URI
  STRIPE_SECRET_KEY
  STRIPE_WEBHOOK_SECRET
  STRIPE_TEAM_PAYMENT_LINK
  STRIPE_ENTERPRISE_PAYMENT_LINK
  PAYPAL_TEAM_PAYMENT_LINK
  PAYPAL_ENTERPRISE_PAYMENT_LINK
)

for key in "${api_keys[@]}"; do
  if awk -F= -v k="$key" '$1==k {found=1} END {exit !found}' apps/api/.env; then
    echo "API env: $key = set"
  else
    echo "API env: $key = missing"
  fi
done

web_keys=(
  NEXT_PUBLIC_SUPABASE_URL
  NEXT_PUBLIC_SUPABASE_ANON_KEY
  NEXT_PUBLIC_API_BASE_URL
)
for key in "${web_keys[@]}"; do
  if awk -F= -v k="$key" '$1==k {found=1} END {exit !found}' apps/web/.env.local; then
    echo "WEB env: $key = set"
  else
    echo "WEB env: $key = missing"
  fi
done
echo

echo "== Step 3: Runtime smoke endpoints =="
# shellcheck disable=SC1091
set -a; source apps/api/.env; set +a

if [[ "$MODE" == "staging" ]]; then
  if [[ -z "${STAGING_API_BASE_URL:-}" ]]; then
    echo "STAGING_API_BASE_URL is required for staging mode."
    exit 1
  fi
  api_base="${STAGING_API_BASE_URL%/}"
else
  api_base="${API_BASE_URL%/}"
fi

echo "Checking API_BASE_URL: ${api_base}"
livez_status="$(curl -sS -o /tmp/tg_livez.json -w "%{http_code}" "${api_base}/livez" || true)"
readyz_status="$(curl -sS -o /tmp/tg_readyz.json -w "%{http_code}" "${api_base}/readyz" || true)"
echo "GET /livez status: ${livez_status}"
echo "GET /readyz status: ${readyz_status}"
if [[ "$readyz_status" == "200" ]]; then
  echo "GET /readyz body:"
  cat /tmp/tg_readyz.json
else
  echo "WARN: /readyz did not return 200 (API may be down or unreachable from current environment)."
fi

private_me_status="$(curl -sS -o /tmp/tg_api_me_unauth.json -w "%{http_code}" "${api_base}/api/me" || true)"
echo "GET /api/me (no auth) status: ${private_me_status} (expected 401)"

slack_events_status="$(curl -sS -o /tmp/tg_slack_events_unauth.json -w "%{http_code}" -X POST "${api_base}/slack/events" || true)"
echo "POST /slack/events (no signature) status: ${slack_events_status} (expected 401)"

slack_actions_status="$(curl -sS -o /tmp/tg_slack_actions_unauth.json -w "%{http_code}" -X POST "${api_base}/slack/actions" || true)"
echo "POST /slack/actions (no signature) status: ${slack_actions_status} (expected 401)"

stripe_invalid_sig_status="$(curl -sS -o /tmp/tg_stripe_invalid_sig.json -w "%{http_code}" -X POST "${api_base}/webhooks/stripe" -H "Stripe-Signature: t=1,v1=invalid" -d '{}' || true)"
echo "POST /webhooks/stripe (invalid signature) status: ${stripe_invalid_sig_status} (expected 401 or 404)"

if [[ -n "${STRIPE_WEBHOOK_SECRET:-}" ]]; then
  stripe_payload='{"id":"evt_test_smoke","type":"ping","data":{"object":{}}}'
  stripe_ts="$(date +%s)"
  stripe_signed_payload="${stripe_ts}.${stripe_payload}"
  stripe_sig="$(printf "%s" "${stripe_signed_payload}" | openssl dgst -sha256 -hmac "${STRIPE_WEBHOOK_SECRET}" -hex | awk '{print $NF}')"
  stripe_valid_status="$(curl -sS -o /tmp/tg_stripe_valid_sig.json -w "%{http_code}" -X POST "${api_base}/webhooks/stripe" -H "Content-Type: application/json" -H "Stripe-Signature: t=${stripe_ts},v1=${stripe_sig}" -d "${stripe_payload}" || true)"
  echo "POST /webhooks/stripe (valid signature ping event) status: ${stripe_valid_status} (expected 200 if runtime uses same STRIPE_WEBHOOK_SECRET)"
fi

slack_install_status="$(curl -sS -o /tmp/tg_slack_install_headers.txt -D /tmp/tg_slack_install_headers.txt -w "%{http_code}" "${api_base}/slack/install" || true)"
echo "GET /slack/install status: ${slack_install_status} (expected 302)"

linear_callback_missing_state_status="$(curl -sS -o /tmp/tg_linear_callback_missing_state.txt -w "%{http_code}" "${api_base}/linear/oauth/callback" || true)"
echo "GET /linear/oauth/callback (missing state/code) status: ${linear_callback_missing_state_status} (expected 400)"

cors_options_status="$(curl -sS -o /tmp/tg_cors_options.txt -D /tmp/tg_cors_options_headers.txt -w "%{http_code}" -X OPTIONS "${api_base}/api/me" -H "Origin: http://localhost:3000" -H "Access-Control-Request-Method: GET" || true)"
echo "OPTIONS /api/me with Origin status: ${cors_options_status} (expected 204)"

unauth_jobs_status="$(curl -sS -o /tmp/tg_jobs_unauth.json -w "%{http_code}" -X POST "${api_base}/jobs/sla" || true)"
echo "POST /jobs/sla (no secret) status: ${unauth_jobs_status} (expected 401 or 404)"

if [[ -n "${JOBS_SECRET:-}" ]]; then
  auth_jobs_status="$(curl -sS -o /tmp/tg_jobs_auth.json -w "%{http_code}" -X POST "${api_base}/jobs/sla" -H "X-Jobs-Secret: ${JOBS_SECRET}" || true)"
  echo "POST /jobs/sla (with secret) status: ${auth_jobs_status} (expected 200)"
fi
echo

echo "== Step 4: Manual integration checklist pointers =="
echo "Run docs checklist: docs/qa/production-audit-checklist.md"
echo "Record findings in: docs/qa/latest-audit-report.md"
echo
echo "Audit runner completed."
