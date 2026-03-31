import { Badge } from "@/components/ui/badge";
import { User, Clock, AlertTriangle, CheckCircle2, PauseCircle } from "lucide-react";

export function SlackThreadMock() {
  return (
    <div className="w-full overflow-hidden rounded-2xl border border-border/60 bg-card shadow-xl">
      <div className="flex items-center gap-2 border-b border-border/40 bg-muted/50 px-4 py-2.5">
        <div className="flex gap-1.5">
          <div className="size-2.5 rounded-full bg-border" />
          <div className="size-2.5 rounded-full bg-border" />
          <div className="size-2.5 rounded-full bg-border" />
        </div>
        <div className="ml-3 flex items-center gap-1.5">
          <div className="size-2 rounded-full bg-success" />
          <span className="text-xs font-medium text-muted-foreground">#support-requests</span>
        </div>
      </div>

      <div className="border-b border-border/40 px-5 py-4">
        <div className="flex items-start gap-3">
          <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted text-xs font-bold text-muted-foreground">
            SM
          </div>
          <div className="flex flex-col gap-1.5">
            <div className="flex items-center gap-2">
              <span className="text-sm font-semibold text-foreground">Sarah M.</span>
              <span className="text-[11px] text-muted-foreground">11:32 AM</span>
            </div>
            <p className="text-sm leading-relaxed text-foreground/85">
              Can someone help with the billing API? Customer Acme Corp is seeing duplicate charges on their invoice and nobody has acked the request yet.
            </p>
          </div>
        </div>
      </div>

      <div className="px-5 py-4">
        <div className="rounded-xl border border-primary/15 bg-primary/[0.02] p-5">
          <div className="mb-4 flex items-center justify-between">
            <div className="flex items-center gap-2">
              <div className="flex size-5 items-center justify-center rounded-md bg-primary text-primary-foreground">
                <CheckCircle2 className="size-3" />
              </div>
              <span className="text-[11px] font-bold uppercase tracking-widest text-primary">TriageGuard</span>
            </div>
            <span className="text-[10px] font-medium text-muted-foreground">REQ-1247</span>
          </div>

          <div className="mb-4 flex flex-wrap items-center gap-2">
            <Badge className="rounded-md bg-primary/10 px-2.5 py-0.5 text-[10px] font-bold uppercase tracking-wide text-primary hover:bg-primary/10">
              Unacked
            </Badge>
            <Badge className="rounded-md bg-destructive/10 px-2.5 py-0.5 text-[10px] font-bold uppercase tracking-wide text-destructive hover:bg-destructive/10">
              P1
            </Badge>
            <Badge className="rounded-md bg-amber-100 px-2.5 py-0.5 text-[10px] font-bold uppercase tracking-wide text-amber-900 hover:bg-amber-100">
              At risk
            </Badge>
            <div className="h-3.5 w-px bg-border" />
            <span className="flex items-center gap-1 text-xs text-muted-foreground">
              <User className="size-3" />
              Unassigned
            </span>
            <div className="h-3.5 w-px bg-border" />
            <span className="flex items-center gap-1 text-xs text-muted-foreground">
              <Clock className="size-3" />
              SLA: Ack overdue
            </span>
          </div>

          <div className="mb-3 flex items-center gap-2 rounded-lg bg-muted/40 px-3 py-2">
            <PauseCircle className="size-3.5 text-muted-foreground" />
            <span className="text-[11px] font-medium text-muted-foreground">Blocker: none yet. Waiting state and snooze stay visible on the card.</span>
          </div>

          <div className="flex flex-wrap gap-1.5">
            {[
              { label: "Triage", icon: CheckCircle2 },
              { label: "Ack", icon: CheckCircle2 },
              { label: "Resolve", icon: AlertTriangle },
              { label: "Assign to me", icon: User },
              { label: "Waiting on requester", icon: PauseCircle },
              { label: "More", icon: Clock },
            ].map(({ label, icon: Icon }) => (
              <button
                key={label}
                className="inline-flex items-center gap-1.5 rounded-lg border border-border/70 bg-background px-2.5 py-1.5 text-[11px] font-medium text-muted-foreground transition-colors hover:border-border hover:bg-muted hover:text-foreground"
              >
                <Icon className="size-3" />
                {label}
              </button>
            ))}
          </div>
        </div>

        <div className="mt-3 flex items-center gap-2">
          <div className="flex items-center gap-2 rounded-lg bg-warning/10 px-3.5 py-1.5">
            <AlertTriangle className="size-3.5 text-warning-foreground" />
            <span className="text-[11px] font-semibold text-warning-foreground">Queue health risk: unacked + unassigned for 2h 15m</span>
          </div>
        </div>
      </div>
    </div>
  );
}
