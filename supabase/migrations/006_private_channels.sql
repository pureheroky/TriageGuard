alter table if exists slack_channels
  add column if not exists is_private boolean not null default false;
