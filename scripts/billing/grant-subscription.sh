#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

usage() {
  cat <<'USAGE'
Usage:
  scripts/billing/grant-subscription.sh (--workspace-id <uuid> | --user-email <email>) [options]

Options:
  --plan <team|enterprise>          Default: team
  --provider <stripe|paypal>        Default: paypal
  --status <value>                  Default: active
  --months <N>                      Default: 1 (sets current_period_end = now + N months)
  --payer-email <email>             Optional (stored for paypal grants)
  --help                            Show this help

Examples:
  scripts/billing/grant-subscription.sh --workspace-id aac1020f-99d5-47a6-ac81-dac58eb268d4 --plan team --provider paypal --months 1
  scripts/billing/grant-subscription.sh --user-email owner@company.com --provider stripe --months 1
USAGE
}

WORKSPACE_ID=""
USER_EMAIL=""
PLAN_KEY="team"
PROVIDER="paypal"
STATUS="active"
MONTHS="1"
PAYER_EMAIL=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --workspace-id)
      WORKSPACE_ID="${2:-}"
      shift 2
      ;;
    --user-email)
      USER_EMAIL="${2:-}"
      shift 2
      ;;
    --plan)
      PLAN_KEY="${2:-}"
      shift 2
      ;;
    --provider)
      PROVIDER="${2:-}"
      shift 2
      ;;
    --status)
      STATUS="${2:-}"
      shift 2
      ;;
    --months)
      MONTHS="${2:-}"
      shift 2
      ;;
    --payer-email)
      PAYER_EMAIL="${2:-}"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ -z "${SUPABASE_DB_URL:-}" && -f "${ROOT_DIR}/apps/api/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "${ROOT_DIR}/apps/api/.env"
  set +a
fi

if [[ -z "${SUPABASE_DB_URL:-}" ]]; then
  echo "SUPABASE_DB_URL is required (export it or set it in apps/api/.env)." >&2
  exit 1
fi

if [[ -z "$WORKSPACE_ID" && -z "$USER_EMAIL" ]]; then
  echo "Provide --workspace-id or --user-email." >&2
  exit 1
fi

if [[ -n "$WORKSPACE_ID" && -n "$USER_EMAIL" ]]; then
  echo "Use either --workspace-id or --user-email, not both." >&2
  exit 1
fi

case "${PLAN_KEY,,}" in
  team|enterprise) PLAN_KEY="${PLAN_KEY,,}" ;;
  *)
    echo "--plan must be team|enterprise" >&2
    exit 1
    ;;
esac

case "${PROVIDER,,}" in
  stripe|paypal) PROVIDER="${PROVIDER,,}" ;;
  *)
    echo "--provider must be stripe|paypal" >&2
    exit 1
    ;;
esac

if ! [[ "$MONTHS" =~ ^[0-9]+$ ]] || [[ "$MONTHS" -lt 1 ]]; then
  echo "--months must be a positive integer" >&2
  exit 1
fi

if [[ -n "$USER_EMAIL" ]]; then
  WORKSPACE_ID="$(
    psql "$SUPABASE_DB_URL" -v ON_ERROR_STOP=1 -v user_email="$USER_EMAIL" -At <<'SQL'
select wm.workspace_id::text
from workspace_members wm
join auth.users u on u.id = wm.user_id
where lower(u.email) = lower(:'user_email')
order by case when wm.role = 'owner' then 0 else 1 end, wm.created_at asc
limit 1;
SQL
  )"
  if [[ -z "$WORKSPACE_ID" ]]; then
    echo "No workspace found for user email: $USER_EMAIL" >&2
    exit 1
  fi
fi

if [[ -z "$PAYER_EMAIL" && "$PROVIDER" == "paypal" && -n "$USER_EMAIL" ]]; then
  PAYER_EMAIL="$USER_EMAIL"
fi

echo "Granting subscription..."
echo "  workspace_id: $WORKSPACE_ID"
echo "  plan:         $PLAN_KEY"
echo "  provider:     $PROVIDER"
echo "  status:       $STATUS"
echo "  months:       $MONTHS"
if [[ -n "$PAYER_EMAIL" ]]; then
  echo "  payer_email:  $PAYER_EMAIL"
fi

psql "$SUPABASE_DB_URL" -v ON_ERROR_STOP=1 -P pager=off \
  -v workspace_id="$WORKSPACE_ID" \
  -v plan_key="$PLAN_KEY" \
  -v status="$STATUS" \
  -v provider="$PROVIDER" \
  -v months="$MONTHS" \
  -v payer_email="$PAYER_EMAIL" <<'SQL'
insert into workspace_subscriptions (
  workspace_id,
  plan_key,
  status,
  billing_provider,
  stripe_customer_id,
  stripe_subscription_id,
  stripe_checkout_session_id,
  paypal_payer_email,
  paypal_last_payment_at,
  cancel_at_period_end,
  current_period_end,
  created_at,
  updated_at
)
values (
  :'workspace_id'::uuid,
  :'plan_key',
  :'status',
  :'provider',
  null,
  null,
  null,
  nullif(:'payer_email', ''),
  case when :'provider' = 'paypal' then now() else null end,
  false,
  now() + (:'months' || ' months')::interval,
  now(),
  now()
)
on conflict (workspace_id) do update
set
  plan_key = excluded.plan_key,
  status = excluded.status,
  billing_provider = excluded.billing_provider,
  paypal_payer_email = coalesce(excluded.paypal_payer_email, workspace_subscriptions.paypal_payer_email),
  paypal_last_payment_at = case
    when excluded.billing_provider = 'paypal' then now()
    else workspace_subscriptions.paypal_last_payment_at
  end,
  cancel_at_period_end = false,
  current_period_end = excluded.current_period_end,
  updated_at = now();

select
  workspace_id,
  plan_key,
  status,
  billing_provider,
  current_period_end,
  paypal_payer_email,
  paypal_last_payment_at,
  updated_at
from workspace_subscriptions
where workspace_id = :'workspace_id'::uuid;
SQL

echo "Done."
