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
      bestFor: "For teams that need queue control, manager visibility, and the first 10 minutes of value fast.",
      popular: true,
      yearlyUnavailable: isYearly,
      features: [
        "Up to 10 active channels",
        "Up to 3 triagers",
        "Shared SLA preset across queues",
        "Manager dashboard + queue health digest",
        "Linear sync and tracker linking",
      ],
    },
    {
      key: "enterprise",
      name: "Enterprise",
      monthlyPrice: ENTERPRISE_MONTHLY,
      yearlyPrice: ENTERPRISE_YEARLY,
      priceLabel: formatEuro(isYearly ? ENTERPRISE_YEARLY : ENTERPRISE_MONTHLY),
      periodLabel: isYearly ? "/year" : "/mo",
      bestFor: "For orgs that need deeper queue controls, analytics, audit depth, and rollout support.",
      yearlyUnavailable: isYearly,
      features: [
        "Unlimited active channels",
        "Per-queue policies and custom escalations",
        "Advanced analytics and exports",
        "Audit depth, rollout help, and future Jira path",
        "Priority support",
      ],
    },
  ];
}
