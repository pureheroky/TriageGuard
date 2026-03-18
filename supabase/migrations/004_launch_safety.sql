alter table if exists linear_installations
  add column if not exists refresh_token text null,
  add column if not exists token_expires_at timestamptz null,
  add column if not exists last_refreshed_at timestamptz null;

create table if not exists stripe_webhook_events (
  event_id text primary key,
  event_type text not null default '',
  received_at timestamptz not null default now()
);

create index if not exists idx_stripe_webhook_events_received_at
  on stripe_webhook_events(received_at desc);
