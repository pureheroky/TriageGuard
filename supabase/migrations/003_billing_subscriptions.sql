create table if not exists workspace_subscriptions (
  workspace_id uuid primary key references workspaces(id) on delete cascade,
  plan_key text not null default 'starter',
  status text not null default 'inactive',
  stripe_customer_id text null,
  stripe_subscription_id text null,
  stripe_checkout_session_id text null,
  cancel_at_period_end boolean not null default false,
  current_period_end timestamptz null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint workspace_subscriptions_plan_check check (plan_key in ('starter', 'team', 'pro'))
);

create unique index if not exists idx_workspace_subscriptions_customer
  on workspace_subscriptions(stripe_customer_id)
  where stripe_customer_id is not null;

create unique index if not exists idx_workspace_subscriptions_subscription
  on workspace_subscriptions(stripe_subscription_id)
  where stripe_subscription_id is not null;

create index if not exists idx_workspace_subscriptions_status
  on workspace_subscriptions(status);
