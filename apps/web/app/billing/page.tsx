"use client";

import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { CheckCircle2, CreditCard, ExternalLink, Loader2 } from "lucide-react";
import { AdminShell } from "@/components/admin-shell";
import { RequireAuth } from "@/app/components/require-auth";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { getBillingPlanDefinitions, type BillingInterval, type BillingPlanKey } from "@/lib/billing";
import { apiFetch } from "@/lib/api";
import { toastError } from "@/lib/stores/toast-store";
import { toFriendlyErrorMessage } from "@/lib/error-messages";

type BillingState = {
  enabled: boolean;
  provider?: "stripe" | "paypal";
  subscription_provider?: "stripe" | "paypal";
  provider_mismatch?: boolean;
  plan_key: BillingPlanKey;
  status: string;
  effective_plan: BillingPlanKey;
  cancel_at_period_end: boolean;
  current_period_end?: string | null;
  paypal_payer_email?: string | null;
  paypal_last_payment_at?: string | null;
  channel_limit?: number | null;
  channels_enabled: number;
  portal_available: boolean;
  checkout: {
    team_available: boolean;
    enterprise_available: boolean;
  };
  config?: {
    billing_provider?: "stripe" | "paypal";
    stripe_secret_key_set?: boolean;
    stripe_webhook_secret_set?: boolean;
    stripe_team_link_set?: boolean;
    stripe_enterprise_link_set?: boolean;
    stripe_portal_return_set?: boolean;
    paypal_team_link_set?: boolean;
    paypal_enterprise_link_set?: boolean;
    paypal_email_set?: boolean;
  };
};

function isPaidActive(status: string): boolean {
  return ["active", "trialing", "past_due", "unpaid", "incomplete"].includes(status);
}

export default function BillingPage() {
  const [interval, setInterval] = useState<BillingInterval>("monthly");
  const plans = getBillingPlanDefinitions(interval);

  const billingQuery = useQuery({
    queryKey: ["billing"],
    queryFn: () => apiFetch<BillingState>("/api/billing"),
  });

  const checkoutMutation = useMutation({
    mutationFn: (planKey: BillingPlanKey) =>
      apiFetch<{ url: string; plan_key: BillingPlanKey }>("/api/billing/checkout", {
        method: "POST",
        body: JSON.stringify({ plan_key: planKey }),
      }),
    onSuccess: (data) => {
      const provider = billingQuery.data?.provider ?? "stripe";
      if (provider === "paypal") {
        window.location.href = data.url;
        return;
      }

      const popup = window.open(data.url, "_blank", "noopener,noreferrer");
      if (!popup) {
        window.location.href = data.url;
      }
    },
    onError: (error) => {
      toastError("Checkout failed", error instanceof Error ? error.message : "Could not start checkout.");
    },
  });

  const portalMutation = useMutation({
    mutationFn: () => apiFetch<{ url: string }>("/api/billing/portal", { method: "POST" }),
    onSuccess: (data) => {
      window.location.href = data.url;
    },
    onError: (error) => {
      toastError("Portal unavailable", error instanceof Error ? error.message : "Could not open billing portal.");
    },
  });

  const billing = billingQuery.data;
  const billingProvider = billing?.provider ?? "stripe";
  const subscriptionProvider = billing?.subscription_provider ?? billingProvider;
  const providerMismatch = Boolean(billing?.provider_mismatch);
  const sameProvider = subscriptionProvider === billingProvider;
  const currentPlan = billing?.effective_plan || "team";
  const currentStatus = billing?.status || "inactive";
  const hasActivePaidSubscription = (currentPlan === "team" || currentPlan === "enterprise") && isPaidActive(currentStatus);
  const currentPlanLabel = hasActivePaidSubscription ? currentPlan.toUpperCase() : "NONE";
  const billingErrorMessage = billingQuery.isError ? toFriendlyErrorMessage(billingQuery.error) : "";
  const hasCurrentPeriodEnd = Boolean(billing?.current_period_end);
  const activeUntilLabel = billing?.current_period_end ? new Date(billing.current_period_end).toLocaleString() : null;

  return (
    <RequireAuth>
      <AdminShell>
        <div className="mx-auto max-w-6xl space-y-6">
          <div>
            <h1 className="text-2xl font-bold tracking-tight text-foreground">Billing</h1>
            <p className="mt-1 text-sm text-muted-foreground">Manage subscription plan and payment details for your workspace.</p>
          </div>

          <Card className="border-border/60 shadow-sm">
            <CardHeader className="pb-4">
              <CardTitle className="text-base font-semibold">Current subscription</CardTitle>
              <CardDescription className="text-xs">
                {billingProvider === "paypal"
                  ? "Checkout is processed by PayPal. Status updates are shown once subscription state is synced."
                  : "Stripe webhooks sync status automatically after successful checkout."}
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-wrap items-center gap-3">
              <Badge className="rounded-md">{currentPlanLabel}</Badge>
              <Badge variant="outline" className="rounded-md">
                {billingProvider.toUpperCase()}
              </Badge>
              {hasActivePaidSubscription ? (
                <Badge variant="outline" className="rounded-md">
                  Active via {subscriptionProvider.toUpperCase()}
                </Badge>
              ) : null}
              <Badge variant="secondary" className="rounded-md">
                {currentStatus}
              </Badge>
              {billing?.cancel_at_period_end ? (
                <Badge variant="outline" className="rounded-md">
                  Cancels at period end
                </Badge>
              ) : null}
              {hasActivePaidSubscription && hasCurrentPeriodEnd && activeUntilLabel ? (
                <span className="text-xs text-muted-foreground">Active until: {activeUntilLabel}</span>
              ) : null}
              {hasActivePaidSubscription && !hasCurrentPeriodEnd ? (
                <span className="text-xs text-muted-foreground">Active until: not provided by billing provider</span>
              ) : null}
              {providerMismatch ? (
                <span className="text-xs text-amber-600">
                  Provider switched to {billingProvider.toUpperCase()}. You can start checkout to migrate billing.
                </span>
              ) : null}
              {billing?.channel_limit != null ? (
                <span className="text-xs text-muted-foreground">
                  Channels: {billing.channels_enabled}/{billing.channel_limit}
                </span>
              ) : (
                <span className="text-xs text-muted-foreground">Channels: {billing?.channels_enabled ?? 0}/unlimited</span>
              )}
              {billingProvider === "stripe" ? (
                <div className="ml-auto">
                  <Button
                    variant="outline"
                    size="sm"
                    className="gap-1.5"
                    disabled={!billing?.portal_available || portalMutation.isPending}
                    onClick={() => portalMutation.mutate()}
                  >
                    {portalMutation.isPending ? <Loader2 className="size-3.5 animate-spin" /> : <CreditCard className="size-3.5" />}
                    Manage billing
                  </Button>
                </div>
              ) : null}
            </CardContent>
          </Card>

          <div className="inline-flex items-center gap-1 rounded-lg border border-border/60 bg-card p-1">
            <Button
              size="sm"
              variant={interval === "monthly" ? "default" : "ghost"}
              className="h-8 px-3 text-xs"
              onClick={() => setInterval("monthly")}
            >
              Monthly
            </Button>
            <Button
              size="sm"
              variant={interval === "yearly" ? "default" : "ghost"}
              className="h-8 px-3 text-xs"
              onClick={() => setInterval("yearly")}
            >
              Yearly (-20%)
            </Button>
          </div>

          <div className="grid gap-4 lg:grid-cols-2">
            {plans.map((plan) => {
              const isCurrent = currentPlan === plan.key && hasActivePaidSubscription && sameProvider;
              const checkoutAvailable =
                plan.key === "team" ? Boolean(billing?.checkout.team_available) : Boolean(billing?.checkout.enterprise_available);
              const lockedByExistingPaid = hasActivePaidSubscription && !isCurrent && !providerMismatch;
              const yearlyUnavailable = interval === "yearly";

              return (
                <Card key={plan.key} className={plan.popular ? "border-border/60 shadow-sm ring-1 ring-primary/20" : "border-border/60 shadow-sm"}>
                  <CardHeader>
                    <div className="flex items-center justify-between">
                      <CardTitle className="text-lg">{plan.name}</CardTitle>
                      {plan.popular ? <Badge>Most popular</Badge> : null}
                    </div>
                    <CardDescription>{plan.bestFor}</CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    <div className="flex items-baseline gap-1">
                      <span className="text-3xl font-bold">{plan.priceLabel}</span>
                      <span className="text-sm text-muted-foreground">{plan.periodLabel}</span>
                    </div>
                    <ul className="space-y-2">
                      {plan.features.map((feature) => (
                        <li key={feature} className="flex items-center gap-2 text-sm text-muted-foreground">
                          <CheckCircle2 className="size-4 text-success" />
                          {feature}
                        </li>
                      ))}
                    </ul>

                    <Button
                      variant={isCurrent ? "secondary" : "default"}
                      className="w-full gap-1.5"
                      disabled={
                        !billing?.enabled ||
                        !checkoutAvailable ||
                        checkoutMutation.isPending ||
                        billingQuery.isLoading ||
                        isCurrent ||
                        lockedByExistingPaid ||
                        yearlyUnavailable
                      }
                      onClick={() => checkoutMutation.mutate(plan.key)}
                    >
                      {checkoutMutation.isPending ? <Loader2 className="size-3.5 animate-spin" /> : null}
                      {isCurrent
                        ? "Current plan"
                        : yearlyUnavailable
                          ? "Annual billing via sales"
                          : lockedByExistingPaid
                            ? "Use billing portal to change"
                            : providerMismatch
                              ? billingProvider === "stripe"
                                ? "Switch billing to Stripe"
                                : "Switch billing to PayPal"
                              : billingProvider === "paypal"
                                ? `Pay with PayPal (${plan.name})`
                                : `Subscribe to ${plan.name}`}
                      {!isCurrent && !lockedByExistingPaid && !yearlyUnavailable ? <ExternalLink className="size-3.5" /> : null}
                    </Button>
                  </CardContent>
                </Card>
              );
            })}
          </div>

          <p className="text-xs text-muted-foreground">VAT may apply at checkout based on your location.</p>

          {billingQuery.isSuccess && !billing?.enabled ? (
            <div className="space-y-1 text-sm text-muted-foreground">
              <p>Billing is temporarily unavailable for this workspace. Please contact support.</p>
            </div>
          ) : null}
          {billingQuery.isError ? (
            <div className="space-y-2">
              <p className="text-sm text-destructive">{billingErrorMessage}</p>
            </div>
          ) : null}
          {billingQuery.isLoading ? <p className="text-sm text-muted-foreground">Loading billing status...</p> : null}
        </div>
      </AdminShell>
    </RequireAuth>
  );
}
