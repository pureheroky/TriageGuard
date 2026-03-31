create table if not exists demo_seed_runs (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  manifest_json jsonb not null default '{}'::jsonb,
  created_by uuid null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (workspace_id)
);

create table if not exists job_runtime_status (
  job_name text primary key,
  last_started_at timestamptz null,
  last_finished_at timestamptz null,
  last_success_at timestamptz null,
  last_error text null,
  last_duration_ms int null,
  updated_at timestamptz not null default now()
);

create index if not exists idx_dead_letters_workspace_created
  on dead_letters(workspace_id, created_at desc);

create index if not exists idx_notification_outbox_kind_status
  on notification_outbox(kind, status, available_at);
