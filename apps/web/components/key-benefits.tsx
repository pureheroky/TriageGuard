import { UserCheck, Timer, MailCheck, Ban, Route, ArrowRightLeft } from "lucide-react";

const benefits = [
  {
    icon: UserCheck,
    title: "Owner in one click",
    description: "Assign responsibility directly in the Slack thread with audit logging.",
  },
  {
    icon: Timer,
    title: "SLA timers & escalations",
    description: "Ack/Assign SLA reminders plus stale pings and escalation channel notifications.",
  },
  {
    icon: MailCheck,
    title: "Daily digest",
    description: "Daily summary includes overdue ack, unassigned requests, active P0, and top threads.",
  },
  {
    icon: Ban,
    title: "Noise control",
    description: "Mark non-actionable messages as ignored so only real requests stay visible.",
  },
  {
    icon: Route,
    title: "Thread-native triage",
    description: "Acknowledge, assign, prioritize, set due dates, resolve, and reopen without leaving Slack.",
  },
  {
    icon: ArrowRightLeft,
    title: "Linear sync",
    description: "Create a Linear issue with Slack context, then keep linked status, assignee, and due date in sync.",
  },
];

export function KeyBenefits() {
  return (
    <section id="features" className="border-b border-border/40 py-24 lg:py-32">
      <div className="mx-auto max-w-7xl px-6">
        <div className="mx-auto mb-16 max-w-2xl text-center">
          <p className="mb-3 text-sm font-semibold uppercase tracking-widest text-primary">Features</p>
          <h2 className="text-balance text-3xl font-bold tracking-tight text-foreground sm:text-4xl">
            Everything you need to triage at scale
          </h2>
          <p className="mt-4 text-pretty text-base leading-relaxed text-muted-foreground">
            Built for teams that live in Slack and need structure without leaving the conversation.
          </p>
        </div>

        <div className="grid gap-px overflow-hidden rounded-2xl border border-border/60 bg-border/60 sm:grid-cols-2 lg:grid-cols-3">
          {benefits.map(({ icon: Icon, title, description }) => (
            <div key={title} className="group flex flex-col gap-4 bg-card p-7 transition-colors hover:bg-muted/30">
              <div className="flex size-11 items-center justify-center rounded-xl bg-primary/8 transition-colors group-hover:bg-primary/12">
                <Icon className="size-5 text-primary" />
              </div>
              <h3 className="text-base font-semibold text-foreground">{title}</h3>
              <p className="text-sm leading-relaxed text-muted-foreground">{description}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
