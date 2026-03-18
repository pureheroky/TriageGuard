#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1"
    exit 1
  fi
}

require_cmd pg_dump
require_cmd pg_restore
require_cmd docker

if [[ -z "${SUPABASE_DB_URL:-}" && -f "apps/api/.env" ]]; then
  # shellcheck disable=SC1091
  set -a; source apps/api/.env; set +a
fi

if [[ -z "${SUPABASE_DB_URL:-}" ]]; then
  echo "SUPABASE_DB_URL is required (env or apps/api/.env)"
  exit 1
fi

tmp_dir="$(mktemp -d)"
dump_file="$tmp_dir/supabase-backup.dump"
container_name="tg-restore-check-$(date +%s)-$RANDOM"

cleanup() {
  docker rm -f "$container_name" >/dev/null 2>&1 || true
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

echo "== Backup/Restore check =="
echo "Timestamp (UTC): $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
echo "Step 1/4: creating logical backup..."
pg_dump "$SUPABASE_DB_URL" \
  --format=custom \
  --no-owner \
  --no-privileges \
  --file="$dump_file"

if [[ ! -s "$dump_file" ]]; then
  echo "Backup file is empty: $dump_file"
  exit 1
fi

echo "Step 2/4: starting ephemeral Postgres for restore validation..."
docker run -d \
  --name "$container_name" \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=restore_check \
  postgres:16-alpine >/dev/null

for attempt in {1..30}; do
  if docker exec "$container_name" pg_isready -U postgres -d restore_check >/dev/null 2>&1; then
    break
  fi
  if [[ "$attempt" -eq 30 ]]; then
    echo "Ephemeral Postgres did not become ready in time."
    exit 1
  fi
  sleep 1
done

echo "Step 3/4: restoring backup into ephemeral DB..."
docker cp "$dump_file" "$container_name":/tmp/backup.dump
if ! docker exec "$container_name" pg_restore -U postgres -d restore_check --no-owner --no-privileges /tmp/backup.dump >/tmp/tg_restore_output.log 2>&1; then
  echo "Restore failed:"
  cat /tmp/tg_restore_output.log
  exit 1
fi

echo "Step 4/4: validating restored schema..."
table_count="$(docker exec "$container_name" psql -U postgres -d restore_check -At -c "select count(*) from information_schema.tables where table_schema='public';" | tr -d '[:space:]')"
if [[ -z "$table_count" || "$table_count" -lt 5 ]]; then
  echo "Unexpected public table count after restore: ${table_count:-empty}"
  exit 1
fi

required_tables=(
  workspaces
  slack_installations
  slack_channels
  policies
  requests
  request_actions
)
for table in "${required_tables[@]}"; do
  exists="$(docker exec "$container_name" psql -U postgres -d restore_check -At -c "select to_regclass('public.${table}') is not null;" | tr -d '[:space:]')"
  if [[ "$exists" != "t" ]]; then
    echo "Missing required table after restore: ${table}"
    exit 1
  fi
done

echo "Backup/restore validation passed."
