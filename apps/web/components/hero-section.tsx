import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { SlackThreadMock } from "@/components/slack-thread-mock";
import { ArrowRight } from "lucide-react";

export function HeroSection() {
  return (
    <section className="relative overflow-hidden border-b border-border/40 bg-background">
      <div className="pointer-events-none absolute inset-0 bg-[linear-gradient(to_right,var(--border)_1px,transparent_1px),linear-gradient(to_bottom,var(--border)_1px,transparent_1px)] bg-[size:4rem_4rem] opacity-30" />
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(ellipse_60%_50%_at_50%_-20%,oklch(0.488_0.185_264/0.08),transparent)]" />

      <div className="relative mx-auto flex max-w-7xl flex-col items-center gap-16 px-6 py-24 lg:flex-row lg:gap-20 lg:py-32">
        <div className="flex max-w-2xl flex-col items-start gap-8 lg:flex-1">
          <Badge variant="secondary" className="rounded-full border border-primary/15 bg-primary/5 px-4 py-1.5 text-xs font-medium text-primary">
            Slack-native triage for engineering teams
          </Badge>

          <div className="flex flex-col gap-5">
            <h1 className="text-balance text-4xl font-bold leading-[1.1] tracking-tight text-foreground sm:text-5xl lg:text-[3.5rem]">
              Stop losing requests
              <br />
              <span className="text-primary">in Slack.</span>
            </h1>
            <p className="max-w-lg text-pretty text-lg leading-relaxed text-muted-foreground">
              Convert Slack messages into managed requests with ownership, SLA timers, escalations, and optional Linear bidirectional sync.
            </p>
          </div>

          <div className="flex flex-wrap items-center gap-3">
            <a href="/login?next=%2Fbilling">
              <Button size="lg" className="h-12 rounded-xl px-6 text-sm font-semibold shadow-md">
                Start subscription
                <ArrowRight className="ml-1 size-4" />
              </Button>
            </a>
            <a href="#pricing">
              <Button variant="outline" size="lg" className="h-12 rounded-xl px-6 text-sm font-medium">
                See pricing
              </Button>
            </a>
          </div>

          <div className="flex flex-wrap items-center gap-2 border-t border-border/60 pt-6">
            <Badge variant="secondary" className="rounded-full border border-border/60 px-3 py-1 text-[11px]">
              Team / Enterprise
            </Badge>
            <Badge variant="secondary" className="rounded-full border border-border/60 px-3 py-1 text-[11px]">
              Public + private channels
            </Badge>
            <Badge variant="secondary" className="rounded-full border border-border/60 px-3 py-1 text-[11px]">
              Slack + Linear (optional)
            </Badge>
          </div>
        </div>

        <div className="w-full max-w-lg lg:flex-1">
          <div className="relative">
            <div className="absolute -inset-4 rounded-2xl bg-primary/5 blur-2xl" />
            <div className="relative">
              <SlackThreadMock />
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
