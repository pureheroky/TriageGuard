create extension if not exists pgcrypto;

create table if not exists workspaces (
  id uuid primary key default gen_random_uuid(),
  name text not null,
  created_at timestamptz not null default now()
);

create table if not exists slack_installations (
  workspace_id uuid primary key references workspaces(id) on delete cascade,
  team_id text not null unique,
  team_name text null,
  team_domain text null,
  bot_user_id text not null,
  bot_token text not null,
  installed_at timestamptz not null default now()
);

create table if not exists slack_channels (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  channel_id text not null,
  channel_name text not null,
  enabled boolean not null default false,
  created_at timestamptz not null default now(),
  unique(workspace_id, channel_id)
);

create table if not exists policies (
  workspace_id uuid primary key references workspaces(id) on delete cascade,
  ack_sla_minutes int not null default 15,
  assign_sla_minutes int not null default 30,
  stale_hours int not null default 24,
  daily_digest_time time not null default '09:00',
  timezone text not null default 'Europe/Madrid',
  escalation_channel_id text null,
  linear_team_id text null,
  last_digest_sent_at timestamptz null
);

create table if not exists requests (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,

  -- idempotency key: team:channel:thread_ts
  source_key text not null unique,

  channel_id text not null,
  thread_ts text not null,
  message_ts text not null,
  author_slack_id text not null,

  body_text text not null,
  title text null,

  status text not null default 'NEW',
  priority text not null default 'P2',

  owner_slack_id text null,
  due_at timestamptz null,

  acked_at timestamptz null,
  assigned_at timestamptz null,
  resolved_at timestamptz null,

  last_activity_at timestamptz not null default now(),

  ack_overdue_sent_at timestamptz null,
  assign_overdue_sent_at timestamptz null,
  stale_sent_at timestamptz null,

  triage_message_ts text null,

  created_at timestamptz not null default now()
);

create index if not exists idx_requests_workspace_status on requests(workspace_id, status);
create index if not exists idx_requests_due on requests(workspace_id, due_at);
create index if not exists idx_requests_last_activity on requests(workspace_id, last_activity_at);

create table if not exists linear_installations (
  workspace_id uuid primary key references workspaces(id) on delete cascade,
  access_token text not null,
  created_at timestamptz not null default now()
);

create table if not exists linear_issues (
  id uuid primary key default gen_random_uuid(),
  request_id uuid not null unique references requests(id) on delete cascade,
  issue_id text not null,
  issue_url text not null,
  created_at timestamptz not null default now()
);

create table if not exists request_actions (
  id uuid primary key default gen_random_uuid(),
  request_id uuid not null references requests(id) on delete cascade,
  actor_slack_id text not null,
  action_type text not null,
  payload jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now()
);

create index if not exists idx_request_actions_request on request_actions(request_id, created_at);
