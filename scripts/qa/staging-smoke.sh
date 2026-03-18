#!/usr/bin/env bash
set -euo pipefail

STAGING_WEB_BASE_URL="${STAGING_WEB_BASE_URL:-}"
STAGING_API_BASE_URL="${STAGING_API_BASE_URL:-}"
STAGING_TEST_EMAIL="${STAGING_TEST_EMAIL:-}"
STAGING_TEST_PASSWORD="${STAGING_TEST_PASSWORD:-}"
STAGING_EXPECT_PAID="${STAGING_EXPECT_PAID:-true}"

if [[ -z "$STAGING_WEB_BASE_URL" || -z "$STAGING_API_BASE_URL" ]]; then
  echo "STAGING_WEB_BASE_URL and STAGING_API_BASE_URL are required."
  exit 1
fi
if [[ -z "$STAGING_TEST_EMAIL" || -z "$STAGING_TEST_PASSWORD" ]]; then
  echo "STAGING_TEST_EMAIL and STAGING_TEST_PASSWORD are required."
  exit 1
fi

web_base="${STAGING_WEB_BASE_URL%/}"
api_base="${STAGING_API_BASE_URL%/}"
tmp_dir="$(mktemp -d)"
cookie_file="$tmp_dir/cookies.txt"

cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

expect_status() {
  local label="$1"
  local got="$2"
  local expected="$3"
  if [[ "$got" != "$expected" ]]; then
    echo "${label}: expected ${expected}, got ${got}"
    exit 1
  fi
}

echo "== Staging smoke =="
echo "WEB: $web_base"
echo "API: $api_base"
echo "Timestamp (UTC): $(date -u +"%Y-%m-%dT%H:%M:%SZ")"

livez_status="$(curl -sS -o "$tmp_dir/livez.json" -w "%{http_code}" "$api_base/livez" || true)"
readyz_status="$(curl -sS -o "$tmp_dir/readyz.json" -w "%{http_code}" "$api_base/readyz" || true)"
expect_status "GET /livez" "$livez_status" "200"
expect_status "GET /readyz" "$readyz_status" "200"

slack_events_status="$(curl -sS -o "$tmp_dir/slack_events.json" -w "%{http_code}" -X POST "$api_base/slack/events" || true)"
expect_status "POST /slack/events without signature" "$slack_events_status" "401"

login_payload="$(printf '{"email":"%s","password":"%s"}' "$STAGING_TEST_EMAIL" "$STAGING_TEST_PASSWORD")"
login_status="$(curl -sS -o "$tmp_dir/login.json" -w "%{http_code}" -c "$cookie_file" -b "$cookie_file" -X POST "$web_base/api/auth/login" -H "Content-Type: application/json" --data "$login_payload" || true)"
expect_status "POST /api/auth/login" "$login_status" "200"

me_status="$(curl -sS -o "$tmp_dir/me.json" -w "%{http_code}" -c "$cookie_file" -b "$cookie_file" "$web_base/api/auth/me" || true)"
expect_status "GET /api/auth/me" "$me_status" "200"

backend_me_status="$(curl -sS -o "$tmp_dir/backend_me.json" -w "%{http_code}" -c "$cookie_file" -b "$cookie_file" "$web_base/api/backend/api/me" || true)"
expect_status "GET /api/backend/api/me" "$backend_me_status" "200"

billing_status="$(curl -sS -o "$tmp_dir/backend_billing.json" -w "%{http_code}" -c "$cookie_file" -b "$cookie_file" "$web_base/api/backend/api/billing" || true)"
expect_status "GET /api/backend/api/billing" "$billing_status" "200"

summary_status="$(curl -sS -o "$tmp_dir/backend_summary.json" -w "%{http_code}" -c "$cookie_file" -b "$cookie_file" "$web_base/api/backend/api/requests/summary?days=30" || true)"
if [[ "$STAGING_EXPECT_PAID" == "true" ]]; then
  expect_status "GET /api/backend/api/requests/summary?days=30 (paid)" "$summary_status" "200"
else
  if [[ "$summary_status" != "200" && "$summary_status" != "402" ]]; then
    echo "GET /api/backend/api/requests/summary expected 200 or 402, got $summary_status"
    exit 1
  fi
fi

logout_status="$(curl -sS -o "$tmp_dir/logout.json" -w "%{http_code}" -c "$cookie_file" -b "$cookie_file" -X POST "$web_base/api/auth/logout" || true)"
expect_status "POST /api/auth/logout" "$logout_status" "200"

post_logout_me_status="$(curl -sS -o "$tmp_dir/post_logout_me.json" -w "%{http_code}" -c "$cookie_file" -b "$cookie_file" "$web_base/api/auth/me" || true)"
expect_status "GET /api/auth/me after logout" "$post_logout_me_status" "401"

echo "Staging smoke passed."
