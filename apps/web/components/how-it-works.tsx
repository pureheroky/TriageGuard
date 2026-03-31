import { CreditCard, Hash, Zap, ArrowRight } from "lucide-react";

const steps = [
  {
    step: "01",
    icon: CreditCard,
    title: "Start subscription",
    description: "Sign in, activate billing, then connect your Slack workspace in onboarding.",
  },
  {
    step: "02",
    icon: Hash,
    title: "Pick channels and queue",
    description: "Choose the Slack channels that feed your first queue so intake lands in a controlled backlog immediately.",
  },
  {
    step: "03",
    icon: Zap,
    title: "Triage in one screen",
    description: "Use the Slack card to ack, assign owner, set type, due date, waiting or snooze state, and resolve from the thread.",
  },
  {
    step: "04",
    icon: ArrowRight,
    title: "Watch queue health",
    description: "Track unacked, unassigned, at-risk, waiting, and forgotten work with SLA clocks, digests, and optional tracker sync.",
  },
];

export function HowItWorks() {
  return (
    <section id="how-it-works" className="border-b border-border/40 bg-muted/30 py-24 lg:py-32">
      <div className="mx-auto max-w-7xl px-6">
        <div className="mx-auto mb-16 max-w-2xl text-center">
          <p className="mb-3 text-sm font-semibold uppercase tracking-widest text-primary">How it works</p>
          <h2 className="text-balance text-3xl font-bold tracking-tight text-foreground sm:text-4xl">
            First queue health signal in about 10 minutes
          </h2>
        </div>

        <div className="grid gap-px overflow-hidden rounded-2xl border border-border/60 bg-border/60 sm:grid-cols-2 lg:grid-cols-4">
          {steps.map(({ step, icon: Icon, title, description }, i) => (
            <div key={step} className="relative flex flex-col gap-5 bg-card p-7">
              <div className="flex items-center gap-4">
                <div className="flex size-11 items-center justify-center rounded-xl bg-primary/10">
                  <Icon className="size-5 text-primary" />
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-xs font-bold tabular-nums text-muted-foreground/60">{step}</span>
                  {i < steps.length - 1 && <div className="hidden h-px w-8 bg-border lg:block" />}
                </div>
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
