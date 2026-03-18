create table if not exists dead_letters (
  id uuid primary key default gen_random_uuid(),
  source text not null,
  operation text not null,
  workspace_id uuid null references workspaces(id) on delete cascade,
  request_id uuid null references requests(id) on delete set null,
  external_id text null,
  error_message text not null,
  payload jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now()
);

create index if not exists idx_dead_letters_workspace_created
  on dead_letters(workspace_id, created_at desc);

create index if not exists idx_dead_letters_created
  on dead_letters(created_at desc);
