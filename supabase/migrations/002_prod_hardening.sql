create table if not exists workspace_members (
  workspace_id uuid not null references workspaces(id) on delete cascade,
  user_id uuid not null,
  role text not null default 'admin',
  created_at timestamptz not null default now(),
  primary key (workspace_id, user_id),
  constraint workspace_members_role_check check (role in ('owner', 'admin'))
);

create index if not exists idx_workspace_members_user on workspace_members(user_id);

create table if not exists slack_action_dedup (
  id text primary key,
  created_at timestamptz not null default now()
);

create index if not exists idx_slack_action_dedup_created_at on slack_action_dedup(created_at);
