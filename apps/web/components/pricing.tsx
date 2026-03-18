"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { getBillingPlanDefinitions, type BillingInterval } from "@/lib/billing";
import { CheckCircle2 } from "lucide-react";

export function Pricing() {
  const [interval, setInterval] = useState<BillingInterval>("monthly");
  const plans = getBillingPlanDefinitions(interval);

  return (
    <section id="pricing" className="border-b border-border/40 py-24 lg:py-32">
      <div className="mx-auto max-w-7xl px-6">
        <div className="mx-auto mb-10 max-w-2xl text-center">
          <p className="mb-3 text-sm font-semibold uppercase tracking-widest text-primary">Pricing</p>
          <h2 className="text-balance text-3xl font-bold tracking-tight text-foreground sm:text-4xl">Simple, transparent pricing</h2>
          <p className="mt-4 text-pretty text-base leading-relaxed text-muted-foreground">
            Choose Team or Enterprise based on channel scale, custom SLA control, and reporting needs.
          </p>
        </div>

        <div className="mb-8 flex justify-center">
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
        </div>

        <div className="grid items-start gap-6 lg:grid-cols-2">
          {plans.map((plan) => (
            <div
              key={plan.name}
              className="relative flex w-full flex-col gap-7 rounded-2xl border border-border/60 bg-card p-7 shadow-lg shadow-primary/5 ring-1 ring-primary/10 lg:p-8"
            >
              {plan.popular ? (
                <Badge className="absolute -top-3 left-7 rounded-full bg-primary px-3.5 py-1 text-[11px] font-semibold text-primary-foreground hover:bg-primary">
                  Most popular
                </Badge>
              ) : null}
              <div className="flex flex-col gap-1.5">
                <h3 className="text-lg font-semibold text-foreground">{plan.name}</h3>
                <p className="text-sm text-muted-foreground">{plan.bestFor}</p>
              </div>
              <div className="flex items-baseline gap-1">
                <span className="text-5xl font-bold tabular-nums text-foreground">{plan.priceLabel}</span>
                <span className="text-sm text-muted-foreground">{plan.periodLabel}</span>
              </div>
              <ul className="flex flex-col gap-3.5">
                {plan.features.map((feature) => (
                  <li key={feature} className="flex items-center gap-2.5 text-sm text-muted-foreground">
                    <CheckCircle2 className="size-4 shrink-0 text-success" />
                    {feature}
                  </li>
                ))}
              </ul>
              <div className="mt-auto pt-2">
                {interval === "yearly" ? (
                  <Button variant="secondary" className="w-full rounded-xl font-medium shadow-sm" size="lg" disabled>
                    Contact us for annual billing
                  </Button>
                ) : (
                  <Button variant="default" className="w-full rounded-xl font-medium shadow-sm" size="lg" asChild>
                    <a href="/login?next=%2Fbilling">Start {plan.name}</a>
                  </Button>
                )}
              </div>
            </div>
          ))}
        </div>
        <p className="mx-auto mt-6 max-w-2xl text-center text-xs text-muted-foreground">
          VAT may apply at checkout depending on customer location.
        </p>
      </div>
    </section>
  );
}
