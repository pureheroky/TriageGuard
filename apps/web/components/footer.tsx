import { Button } from "@/components/ui/button";
import { ArrowRight } from "lucide-react";
import { TriageGuardLogo } from "@/components/triageguard-logo";

export function Footer() {
  return (
    <footer className="bg-foreground text-background">
      <div className="border-b border-background/10">
        <div className="mx-auto flex max-w-7xl flex-col items-center gap-6 px-6 py-16 text-center sm:flex-row sm:justify-between sm:text-left">
          <div className="flex flex-col gap-2">
            <h3 className="text-xl font-semibold text-background">Ready to stop losing requests?</h3>
            <p className="text-sm text-background/60">Start your subscription, connect Slack, and go live in minutes.</p>
          </div>
          <a href="/login?next=%2Fbilling">
            <Button size="lg" className="h-12 rounded-xl bg-background px-6 text-sm font-semibold text-foreground hover:bg-background/90">
              Start subscription
              <ArrowRight className="ml-1 size-4" />
            </Button>
          </a>
        </div>
      </div>

      <div className="mx-auto max-w-7xl px-6 py-12">
        <div className="grid gap-8 sm:grid-cols-2 lg:grid-cols-4">
          <div className="flex flex-col gap-4">
            <a href="#" className="flex items-center gap-2.5">
              <div className="flex size-8 items-center justify-center rounded-lg bg-background">
                <TriageGuardLogo size={18} className="text-foreground" />
              </div>
              <span className="text-base font-semibold tracking-tight text-background">TriageGuard</span>
            </a>
            <p className="max-w-xs text-sm leading-relaxed text-background/50">
              Turn Slack messages into trackable requests with ownership, priority, and SLA timers.
            </p>
          </div>
          <div className="space-y-3">
            <p className="text-xs font-semibold uppercase tracking-wide text-background/70">Product</p>
            <div className="flex flex-col gap-2 text-sm text-background/60">
              <a href="/login?next=%2Fbilling" className="hover:text-background">
                Get started
              </a>
              <a href="/login?next=%2Fbilling" className="hover:text-background">
                Pricing
              </a>
              <a href="#faq" className="hover:text-background">
                FAQ
              </a>
            </div>
          </div>
          <div className="space-y-3">
            <p className="text-xs font-semibold uppercase tracking-wide text-background/70">Company</p>
            <div className="flex flex-col gap-2 text-sm text-background/60">
              <a href="/privacy" className="hover:text-background">
                Privacy
              </a>
              <a href="/terms" className="hover:text-background">
                Terms
              </a>
              <a href="/support" className="hover:text-background">
                Support
              </a>
            </div>
          </div>
          <div className="space-y-3">
            <p className="text-xs font-semibold uppercase tracking-wide text-background/70">Contact</p>
            <div className="flex flex-col gap-2 text-sm text-background/60">
              <a href="mailto:support@triageguard.app" className="hover:text-background">
                support@triageguard.app
              </a>
            </div>
          </div>
        </div>

        <div className="mt-12 border-t border-background/10 pt-6">
          <p className="text-center text-xs text-background/40">© 2026 TriageGuard. All rights reserved.</p>
        </div>
      </div>
    </footer>
  );
}
