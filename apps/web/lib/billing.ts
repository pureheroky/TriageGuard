export type BillingPlanKey = "team" | "enterprise";
export type BillingInterval = "monthly" | "yearly";

export type BillingPlanDefinition = {
  key: BillingPlanKey;
  name: string;
  monthlyPrice: number;
  yearlyPrice: number;
  priceLabel: string;
  periodLabel: string;
  bestFor: string;
  popular?: boolean;
  yearlyUnavailable?: boolean;
  features: string[];
};

const TEAM_MONTHLY = 89;
const TEAM_YEARLY = 854;
const ENTERPRISE_MONTHLY = 349;
const ENTERPRISE_YEARLY = 3350;

function formatEuro(value: number): string {
  return `€${value}`;
}

export function getBillingPlanDefinitions(interval: BillingInterval): BillingPlanDefinition[] {
  const isYearly = interval === "yearly";
  return [
    {
      key: "team",
      name: "Team",
      monthlyPrice: TEAM_MONTHLY,
      yearlyPrice: TEAM_YEARLY,
      priceLabel: formatEuro(isYearly ? TEAM_YEARLY : TEAM_MONTHLY),
      periodLabel: isYearly ? "/year" : "/mo",
      bestFor: "For engineering teams scaling Slack triage with SLA ownership.",
      popular: true,
      yearlyUnavailable: isYearly,
      features: [
        "Up to 10 active channels",
        "Workspace-level SLA policies",
        "Escalation channel + daily digest",
        "Linear sync and conversion",
      ],
    },
    {
      key: "enterprise",
      name: "Enterprise",
      monthlyPrice: ENTERPRISE_MONTHLY,
      yearlyPrice: ENTERPRISE_YEARLY,
      priceLabel: formatEuro(isYearly ? ENTERPRISE_YEARLY : ENTERPRISE_MONTHLY),
      periodLabel: isYearly ? "/year" : "/mo",
      bestFor: "For larger orgs needing deep controls and reporting.",
      yearlyUnavailable: isYearly,
      features: [
        "Unlimited active channels",
        "Custom SLA policy per channel",
        "CSV exports: request lifecycle + analytics",
        "Priority support and rollout help",
      ],
    },
  ];
}
