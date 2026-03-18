alter table if exists workspace_subscriptions
  add column if not exists billing_provider text not null default 'stripe',
  add column if not exists paypal_payer_email text null,
  add column if not exists paypal_last_payment_at timestamptz null;

do $$
begin
  alter table workspace_subscriptions
    add constraint workspace_subscriptions_provider_check check (billing_provider in ('stripe', 'paypal'));
exception
  when duplicate_object then null;
end
$$;

create index if not exists idx_workspace_subscriptions_provider
  on workspace_subscriptions(billing_provider);
