create table if not exists linear_webhook_events (
  event_id text primary key,
  event_type text not null default '',
  received_at timestamptz not null default now()
);

create index if not exists idx_linear_webhook_events_received_at
  on linear_webhook_events(received_at desc);

create index if not exists idx_linear_issues_issue_id
  on linear_issues(issue_id);
