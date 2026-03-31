import { UserCheck, Timer, MailCheck, Ban, Route, ArrowRightLeft } from "lucide-react";

const benefits = [
  {
    icon: UserCheck,
    title: "Unacked and unassigned visibility",
    description: "See which internal requests were seen, who owns them, and where the queue is already drifting.",
  },
  {
    icon: Timer,
    title: "SLA risk before breach",
    description: "Highlight at-risk work before it breaches and escalate only when a request is actually slipping.",
  },
  {
    icon: MailCheck,
    title: "Queue health digests",
    description: "Daily queue digests and weekly manager summaries show what worsened and what needs attention this week.",
  },
  {
    icon: Ban,
    title: "Waiting vs forgotten",
    description: "Separate healthy waiting work from truly forgotten requests with waiting states and snooze windows.",
  },
  {
    icon: Route,
    title: "Thread-native queue control",
    description: "Triage, ownership, waiting, snooze, resolve, and reopen all happen inside the Slack conversation.",
  },
  {
    icon: ArrowRightLeft,
    title: "Tracker stays optional",
    description: "Linear linking is available, but queue health, ownership, SLA state, and escalation stay owned by TriageGuard.",
  },
];

export function KeyBenefits() {
  return (
    <section id="features" className="border-b border-border/40 py-24 lg:py-32">
      <div className="mx-auto max-w-7xl px-6">
        <div className="mx-auto mb-16 max-w-2xl text-center">
          <p className="mb-3 text-sm font-semibold uppercase tracking-widest text-primary">Features</p>
          <h2 className="text-balance text-3xl font-bold tracking-tight text-foreground sm:text-4xl">
            Built to keep internal requests under control
          </h2>
          <p className="mt-4 text-pretty text-base leading-relaxed text-muted-foreground">
            Not another intake form. A queue-control layer for teams that already live in Slack.
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
