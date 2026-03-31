alter table if exists slack_channels
  add column if not exists default_queue_id uuid null;

alter table if exists requests
  add column if not exists state text null,
  add column if not exists acknowledged_at timestamptz null,
  add column if not exists acknowledged_by text null,
  add column if not exists owner_user_id text null,
  add column if not exists request_type text null,
  add column if not exists queue_id uuid null,
  add column if not exists waiting_on text null,
  add column if not exists snoozed_until timestamptz null,
  add column if not exists closed_at timestamptz null,
  add column if not exists closed_by text null,
  add column if not exists closed_reason text null,
  add column if not exists last_human_activity_at timestamptz null,
  add column if not exists last_state_change_at timestamptz null;

create table if not exists queues (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  name text not null,
  description text null,
  default_priority text not null default 'P2',
  default_request_type text not null default 'other',
  escalation_channel_id text null,
  is_default boolean not null default false,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (workspace_id, name)
);

create index if not exists idx_queues_workspace on queues(workspace_id);

create table if not exists queue_channels (
  queue_id uuid not null references queues(id) on delete cascade,
  channel_id text not null,
  is_default boolean not null default false,
  created_at timestamptz not null default now(),
  primary key (queue_id, channel_id)
);

create index if not exists idx_queue_channels_channel on queue_channels(channel_id);

create table if not exists queue_members (
  queue_id uuid not null references queues(id) on delete cascade,
  user_id text not null,
  role text not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  primary key (queue_id, user_id, role)
);

create table if not exists queue_policies (
  queue_id uuid primary key references queues(id) on delete cascade,
  ack_sla_minutes int not null default 15,
  assign_sla_minutes int not null default 30,
  stale_hours int not null default 24,
  digest_time time not null default '09:00',
  digest_weekday int null,
  assign_starts_from text not null default 'created_at',
  stale_starts_from text not null default 'last_human_activity_at',
  timezone text not null default 'UTC',
  business_hours_enabled boolean not null default false,
  business_hours_start time null,
  business_hours_end time null,
  business_days_mask int not null default 62,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists queue_routing_rules (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  name text not null,
  sort_order int not null default 100,
  is_enabled boolean not null default true,
  conditions_json jsonb not null default '{}'::jsonb,
  queue_id uuid not null references queues(id) on delete cascade,
  request_type text null,
  priority text null,
  stop_processing boolean not null default true,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index if not exists idx_queue_routing_rules_workspace
  on queue_routing_rules(workspace_id, is_enabled, sort_order);

create table if not exists request_sla_clocks (
  id uuid primary key default gen_random_uuid(),
  request_id uuid not null references requests(id) on delete cascade,
  clock_type text not null,
  started_at timestamptz not null,
  target_at timestamptz null,
  paused_at timestamptz null,
  satisfied_at timestamptz null,
  breached_at timestamptz null,
  last_notified_at timestamptz null,
  state text not null default 'running',
  created_at timestamptz not null default now()
);

create index if not exists idx_request_sla_clocks_request
  on request_sla_clocks(request_id, clock_type, created_at desc);

create table if not exists request_events (
  id uuid primary key default gen_random_uuid(),
  request_id uuid not null references requests(id) on delete cascade,
  event_type text not null,
  actor_type text not null default 'system',
  actor_id text null,
  occurred_at timestamptz not null default now(),
  payload_json jsonb not null default '{}'::jsonb
);

create index if not exists idx_request_events_request
  on request_events(request_id, occurred_at desc);

create table if not exists notification_outbox (
  id uuid primary key default gen_random_uuid(),
  request_id uuid null references requests(id) on delete set null,
  event_id uuid null references request_events(id) on delete set null,
  kind text not null,
  destination_type text not null,
  destination_value text null,
  payload_json jsonb not null default '{}'::jsonb,
  dedupe_key text null,
  status text not null default 'pending',
  retry_count int not null default 0,
  available_at timestamptz not null default now(),
  sent_at timestamptz null,
  failed_at timestamptz null,
  last_error text null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create unique index if not exists idx_notification_outbox_dedupe
  on notification_outbox(dedupe_key)
  where dedupe_key is not null;

create index if not exists idx_notification_outbox_status
  on notification_outbox(status, available_at);

create table if not exists external_issues (
  id uuid primary key default gen_random_uuid(),
  request_id uuid not null references requests(id) on delete cascade,
  provider text not null,
  external_id text not null,
  external_key text null,
  external_url text null,
  sync_state text not null default 'linked',
  last_sync_at timestamptz null,
  last_sync_error text null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (provider, external_id),
  unique (request_id, provider)
);

create table if not exists escalation_steps (
  id uuid primary key default gen_random_uuid(),
  queue_id uuid not null references queues(id) on delete cascade,
  clock_type text not null,
  delay_minutes_after_breach int not null default 0,
  target_type text not null,
  target_value text null,
  message_template text null,
  is_enabled boolean not null default true,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index if not exists idx_escalation_steps_queue
  on escalation_steps(queue_id, clock_type, is_enabled, delay_minutes_after_breach);

create table if not exists daily_queue_metrics (
  date date not null,
  workspace_id uuid not null references workspaces(id) on delete cascade,
  queue_id uuid not null references queues(id) on delete cascade,
  created_count int not null default 0,
  closed_count int not null default 0,
  unacked_count int not null default 0,
  unassigned_count int not null default 0,
  breached_ack_count int not null default 0,
  breached_assign_count int not null default 0,
  median_ack_minutes numeric null,
  median_assign_minutes numeric null,
  median_close_minutes numeric null,
  reopen_count int not null default 0,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  primary key (date, workspace_id, queue_id)
);

insert into queues (
  workspace_id,
  name,
  description,
  default_priority,
  default_request_type,
  escalation_channel_id,
  is_default
)
select
  w.id,
  'Default queue',
  'Workspace fallback queue',
  'P2',
  'other',
  p.escalation_channel_id,
  true
from workspaces w
left join policies p on p.workspace_id = w.id
where not exists (
  select 1
  from queues q
  where q.workspace_id = w.id
    and q.is_default = true
);

insert into queues (
  workspace_id,
  name,
  description,
  default_priority,
  default_request_type,
  escalation_channel_id,
  is_default
)
select
  sc.workspace_id,
  sc.channel_name,
  'Seeded from Slack channel ' || sc.channel_id,
  'P2',
  'other',
  p.escalation_channel_id,
  false
from slack_channels sc
left join policies p on p.workspace_id = sc.workspace_id
where not exists (
  select 1
  from queues q
  where q.workspace_id = sc.workspace_id
    and q.description = 'Seeded from Slack channel ' || sc.channel_id
);

insert into queue_channels (queue_id, channel_id, is_default)
select
  q.id,
  sc.channel_id,
  true
from slack_channels sc
join queues q
  on q.workspace_id = sc.workspace_id
 and q.description = 'Seeded from Slack channel ' || sc.channel_id
where not exists (
  select 1
  from queue_channels qc
  where qc.queue_id = q.id
    and qc.channel_id = sc.channel_id
);

insert into queue_policies (
  queue_id,
  ack_sla_minutes,
  assign_sla_minutes,
  stale_hours,
  digest_time,
  assign_starts_from,
  stale_starts_from,
  timezone,
  business_hours_enabled,
  business_hours_start,
  business_hours_end,
  business_days_mask
)
select
  q.id,
  coalesce(cso.ack_sla_minutes, p.ack_sla_minutes, 15),
  coalesce(cso.assign_sla_minutes, p.assign_sla_minutes, 30),
  coalesce(cso.stale_hours, p.stale_hours, 24),
  coalesce(p.daily_digest_time, '09:00'::time),
  'created_at',
  'last_human_activity_at',
  coalesce(p.timezone, 'UTC'),
  false,
  null,
  null,
  62
from queues q
left join slack_channels sc
  on sc.workspace_id = q.workspace_id
 and q.description = 'Seeded from Slack channel ' || sc.channel_id
left join policies p
  on p.workspace_id = q.workspace_id
left join channel_sla_overrides cso
  on cso.workspace_id = sc.workspace_id
 and cso.channel_id = sc.channel_id
where not exists (
  select 1
  from queue_policies qp
  where qp.queue_id = q.id
);

update slack_channels sc
set default_queue_id = q.id
from queues q
where q.workspace_id = sc.workspace_id
  and q.description = 'Seeded from Slack channel ' || sc.channel_id
  and sc.default_queue_id is null;

update requests r
set
  state = case when r.status in ('RESOLVED', 'IGNORED') then 'CLOSED' else 'OPEN' end,
  acknowledged_at = case
    when r.acked_at is not null then r.acked_at
    when r.status in ('ACKED', 'ASSIGNED', 'RESOLVED', 'IGNORED') then coalesce(r.assigned_at, r.resolved_at, r.created_at)
    else null
  end,
  owner_user_id = coalesce(r.owner_user_id, r.owner_slack_id),
  request_type = coalesce(r.request_type, 'other'),
  waiting_on = coalesce(r.waiting_on, 'none'),
  closed_at = case when r.status in ('RESOLVED', 'IGNORED') then coalesce(r.resolved_at, r.created_at) else null end,
  closed_reason = case
    when r.status = 'RESOLVED' then 'resolved'
    when r.status = 'IGNORED' then 'noise'
    else null
  end,
  last_human_activity_at = coalesce(r.last_human_activity_at, r.last_activity_at, r.created_at),
  last_state_change_at = coalesce(r.last_state_change_at, r.resolved_at, r.assigned_at, r.acked_at, r.created_at)
where r.state is null
   or r.request_type is null
   or r.waiting_on is null
   or r.last_human_activity_at is null
   or r.last_state_change_at is null;

update requests r
set
  acknowledged_by = coalesce(r.acknowledged_by, r.author_slack_id),
  closed_by = coalesce(r.closed_by, r.author_slack_id)
where (r.acknowledged_at is not null and r.acknowledged_by is null)
   or (r.closed_at is not null and r.closed_by is null);

update requests r
set queue_id = coalesce(
  sc.default_queue_id,
  (
    select q.id
    from queues q
    where q.workspace_id = r.workspace_id
      and q.is_default = true
    order by q.created_at asc
    limit 1
  )
)
from slack_channels sc
where sc.workspace_id = r.workspace_id
  and sc.channel_id = r.channel_id
  and r.queue_id is null;

update requests
set state = 'OPEN'
where state is null;

update requests
set request_type = 'other'
where request_type is null;

update requests
set waiting_on = 'none'
where waiting_on is null;

update requests
set last_human_activity_at = coalesce(last_activity_at, created_at)
where last_human_activity_at is null;

update requests
set last_state_change_at = coalesce(closed_at, assigned_at, acknowledged_at, created_at)
where last_state_change_at is null;

alter table if exists requests
  alter column state set default 'OPEN',
  alter column state set not null,
  alter column request_type set default 'other',
  alter column request_type set not null,
  alter column waiting_on set default 'none',
  alter column waiting_on set not null,
  alter column last_human_activity_at set default now(),
  alter column last_human_activity_at set not null,
  alter column last_state_change_at set default now(),
  alter column last_state_change_at set not null;

do $$
begin
  alter table requests
    add constraint requests_state_check
    check (state in ('OPEN', 'CLOSED'));
exception
  when duplicate_object then null;
end
$$;

do $$
begin
  alter table requests
    add constraint requests_request_type_check
    check (request_type in ('bug', 'incident', 'access', 'infra', 'question', 'task', 'other'));
exception
  when duplicate_object then null;
end
$$;

do $$
begin
  alter table requests
    add constraint requests_waiting_on_check
    check (waiting_on in ('none', 'requester', 'other_team', 'external_vendor', 'scheduled_work'));
exception
  when duplicate_object then null;
end
$$;

do $$
begin
  alter table requests
    add constraint requests_closed_reason_check
    check (closed_reason is null or closed_reason in ('resolved', 'duplicate', 'not_planned', 'noise', 'invalid'));
exception
  when duplicate_object then null;
end
$$;

insert into external_issues (
  request_id,
  provider,
  external_id,
  external_key,
  external_url,
  sync_state,
  last_sync_at
)
select
  li.request_id,
  'linear',
  li.issue_id,
  li.issue_id,
  li.issue_url,
  'linked',
  li.created_at
from linear_issues li
where not exists (
  select 1
  from external_issues ei
  where ei.request_id = li.request_id
    and ei.provider = 'linear'
);

insert into request_events (
  request_id,
  event_type,
  actor_type,
  actor_id,
  occurred_at,
  payload_json
)
select
  ra.request_id,
  lower(replace(ra.action_type, ' ', '_')),
  case
    when ra.actor_slack_id = 'LINEAR_WEBHOOK' then 'external_provider'
    else 'slack_user'
  end,
  nullif(ra.actor_slack_id, ''),
  ra.created_at,
  coalesce(ra.payload, '{}'::jsonb)
from request_actions ra
where not exists (
  select 1
  from request_events re
  where re.request_id = ra.request_id
    and re.event_type = lower(replace(ra.action_type, ' ', '_'))
    and re.actor_id is not distinct from nullif(ra.actor_slack_id, '')
    and re.occurred_at = ra.created_at
);

insert into request_events (
  request_id,
  event_type,
  actor_type,
  actor_id,
  occurred_at,
  payload_json
)
select
  r.id,
  'request_created',
  'slack_user',
  r.author_slack_id,
  r.created_at,
  jsonb_build_object(
    'channel_id', r.channel_id,
    'thread_ts', r.thread_ts,
    'queue_id', r.queue_id,
    'request_type', r.request_type
  )
from requests r
where not exists (
  select 1
  from request_events re
  where re.request_id = r.id
    and re.event_type = 'request_created'
);

insert into request_sla_clocks (
  request_id,
  clock_type,
  started_at,
  target_at,
  breached_at,
  state
)
select
  r.id,
  'ack',
  r.created_at,
  r.created_at + make_interval(mins => qp.ack_sla_minutes),
  case
    when r.ack_overdue_sent_at is not null then r.ack_overdue_sent_at
    else null
  end,
  case
    when r.acknowledged_at is not null then 'satisfied'
    when r.state = 'CLOSED' then 'stopped'
    when r.ack_overdue_sent_at is not null then 'breached'
    else 'running'
  end
from requests r
join queue_policies qp on qp.queue_id = r.queue_id
where not exists (
  select 1
  from request_sla_clocks c
  where c.request_id = r.id
    and c.clock_type = 'ack'
);

update request_sla_clocks c
set satisfied_at = r.acknowledged_at
from requests r
where c.request_id = r.id
  and c.clock_type = 'ack'
  and r.acknowledged_at is not null
  and c.satisfied_at is null;

insert into request_sla_clocks (
  request_id,
  clock_type,
  started_at,
  target_at,
  breached_at,
  state
)
select
  r.id,
  'assign',
  case
    when qp.assign_starts_from = 'acknowledged_at' and r.acknowledged_at is not null then r.acknowledged_at
    else r.created_at
  end,
  case
    when qp.assign_starts_from = 'acknowledged_at' and r.acknowledged_at is not null then r.acknowledged_at + make_interval(mins => qp.assign_sla_minutes)
    when qp.assign_starts_from = 'acknowledged_at' then null
    else r.created_at + make_interval(mins => qp.assign_sla_minutes)
  end,
  case
    when r.assign_overdue_sent_at is not null then r.assign_overdue_sent_at
    else null
  end,
  case
    when coalesce(r.owner_user_id, r.owner_slack_id) is not null then 'satisfied'
    when r.state = 'CLOSED' then 'stopped'
    when qp.assign_starts_from = 'acknowledged_at' and r.acknowledged_at is null then 'paused'
    when r.assign_overdue_sent_at is not null then 'breached'
    else 'running'
  end
from requests r
join queue_policies qp on qp.queue_id = r.queue_id
where not exists (
  select 1
  from request_sla_clocks c
  where c.request_id = r.id
    and c.clock_type = 'assign'
);

update request_sla_clocks c
set satisfied_at = coalesce(r.assigned_at, r.last_state_change_at)
from requests r
where c.request_id = r.id
  and c.clock_type = 'assign'
  and coalesce(r.owner_user_id, r.owner_slack_id) is not null
  and c.satisfied_at is null;

insert into request_sla_clocks (
  request_id,
  clock_type,
  started_at,
  target_at,
  paused_at,
  breached_at,
  state
)
select
  r.id,
  'stale',
  coalesce(r.last_human_activity_at, r.last_activity_at, r.created_at),
  coalesce(r.last_human_activity_at, r.last_activity_at, r.created_at) + make_interval(hours => qp.stale_hours),
  case
    when r.waiting_on <> 'none' or (r.snoozed_until is not null and r.snoozed_until > now()) then now()
    when r.status not in ('ACKED', 'ASSIGNED') then now()
    else null
  end,
  case
    when r.stale_sent_at is not null then r.stale_sent_at
    else null
  end,
  case
    when r.state = 'CLOSED' then 'stopped'
    when r.waiting_on <> 'none' or (r.snoozed_until is not null and r.snoozed_until > now()) then 'paused'
    when r.status not in ('ACKED', 'ASSIGNED') then 'paused'
    when r.stale_sent_at is not null then 'breached'
    else 'running'
  end
from requests r
join queue_policies qp on qp.queue_id = r.queue_id
where not exists (
  select 1
  from request_sla_clocks c
  where c.request_id = r.id
    and c.clock_type = 'stale'
);

insert into escalation_steps (
  queue_id,
  clock_type,
  delay_minutes_after_breach,
  target_type,
  target_value,
  message_template,
  is_enabled
)
select
  q.id,
  'ack',
  0,
  'thread',
  null,
  'Ack overdue',
  true
from queues q
where not exists (
  select 1
  from escalation_steps es
  where es.queue_id = q.id
    and es.clock_type = 'ack'
    and es.delay_minutes_after_breach = 0
    and es.target_type = 'thread'
);

insert into escalation_steps (
  queue_id,
  clock_type,
  delay_minutes_after_breach,
  target_type,
  target_value,
  message_template,
  is_enabled
)
select
  q.id,
  'assign',
  0,
  'thread',
  null,
  'Assign overdue',
  true
from queues q
where not exists (
  select 1
  from escalation_steps es
  where es.queue_id = q.id
    and es.clock_type = 'assign'
    and es.delay_minutes_after_breach = 0
    and es.target_type = 'thread'
);

insert into escalation_steps (
  queue_id,
  clock_type,
  delay_minutes_after_breach,
  target_type,
  target_value,
  message_template,
  is_enabled
)
select
  q.id,
  'stale',
  0,
  'thread',
  null,
  'Request is stale',
  true
from queues q
where not exists (
  select 1
  from escalation_steps es
  where es.queue_id = q.id
    and es.clock_type = 'stale'
    and es.delay_minutes_after_breach = 0
    and es.target_type = 'thread'
);

do $$
begin
  alter table slack_channels
    add constraint slack_channels_default_queue_fk
    foreign key (default_queue_id)
    references queues(id)
    on delete set null;
exception
  when duplicate_object then null;
end
$$;

do $$
begin
  alter table requests
    add constraint requests_queue_fk
    foreign key (queue_id)
    references queues(id)
    on delete set null;
exception
  when duplicate_object then null;
end
$$;

create index if not exists idx_requests_workspace_state on requests(workspace_id, state);
create index if not exists idx_requests_queue on requests(queue_id);
create index if not exists idx_requests_waiting_on on requests(workspace_id, waiting_on);
create index if not exists idx_requests_snoozed_until on requests(snoozed_until);
create index if not exists idx_requests_last_human_activity on requests(workspace_id, last_human_activity_at);
