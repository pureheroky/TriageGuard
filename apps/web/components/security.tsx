import { ShieldCheck, Lock, Eye, BrainCircuit } from "lucide-react";

const checks = [
  {
    icon: Lock,
    title: "Tokens stored server-side",
    description: "All OAuth tokens are stored server-side. Never exposed to the client.",
  },
  {
    icon: ShieldCheck,
    title: "Signed Slack requests",
    description: "Events and actions are verified using Slack signing secret before processing.",
  },
  {
    icon: Eye,
    title: "OAuth state protection",
    description: "Slack and Linear OAuth callbacks are validated with signed state and nonce checks.",
  },
  {
    icon: BrainCircuit,
    title: "No LLM required",
    description: "Core workflows are deterministic and rule-based. No AI inference required.",
  },
];

export function Security() {
  return (
    <section id="security" className="border-b border-border/40 bg-muted/30 py-24 lg:py-32">
      <div className="mx-auto max-w-7xl px-6">
        <div className="mx-auto mb-16 max-w-2xl text-center">
          <p className="mb-3 text-sm font-semibold uppercase tracking-widest text-primary">Security & privacy</p>
          <h2 className="text-balance text-3xl font-bold tracking-tight text-foreground sm:text-4xl">
            Built with security-first principles
          </h2>
        </div>

        <div className="grid gap-px overflow-hidden rounded-2xl border border-border/60 bg-border/60 sm:grid-cols-2">
          {checks.map(({ icon: Icon, title, description }) => (
            <div key={title} className="flex items-start gap-5 bg-card p-7">
              <div className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-primary/8">
                <Icon className="size-5 text-primary" />
              </div>
              <div className="flex flex-col gap-1.5">
                <h3 className="text-sm font-semibold text-foreground">{title}</h3>
                <p className="text-sm leading-relaxed text-muted-foreground">{description}</p>
              </div>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
