-- Billing plans: migrate legacy keys to team/enterprise.
update workspace_subscriptions
set plan_key = 'team'
where plan_key = 'starter';

update workspace_subscriptions
set plan_key = 'enterprise'
where plan_key = 'pro';

alter table if exists workspace_subscriptions
  alter column plan_key set default 'team';

alter table if exists workspace_subscriptions
  drop constraint if exists workspace_subscriptions_plan_check;

do $$
begin
  alter table workspace_subscriptions
    add constraint workspace_subscriptions_plan_check
    check (plan_key in ('team', 'enterprise'));
exception
  when duplicate_object then null;
end
$$;

create table if not exists channel_sla_overrides (
  workspace_id uuid not null,
  channel_id text not null,
  ack_sla_minutes int not null,
  assign_sla_minutes int not null,
  stale_hours int not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  primary key (workspace_id, channel_id),
  constraint channel_sla_overrides_channel_fk
    foreign key (workspace_id, channel_id)
    references slack_channels(workspace_id, channel_id)
    on delete cascade,
  constraint channel_sla_overrides_ack_positive check (ack_sla_minutes > 0),
  constraint channel_sla_overrides_assign_positive check (assign_sla_minutes > 0),
  constraint channel_sla_overrides_stale_positive check (stale_hours > 0)
);

create index if not exists idx_channel_sla_overrides_workspace
  on channel_sla_overrides(workspace_id);
