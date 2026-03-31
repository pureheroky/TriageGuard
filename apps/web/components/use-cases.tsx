import { Badge } from "@/components/ui/badge";
import { CheckCircle2, User } from "lucide-react";

const useCases = [
  {
    title: "Support & customer requests",
    description: "Keep customer-facing queues healthy with explicit ownership, ack visibility, and queue-level manager control.",
    bullets: [
      "See unacked and unassigned support work immediately",
      "Set owner, type, priority, and due date directly in thread",
      "Separate waiting work from forgotten work",
      "Keep managers aligned with queue health digests",
    ],
    mockSender: "Alex T.",
    mockInitials: "AT",
    mockMessage: "Acme Inc can't access their dashboard after the migration.",
    mockStatus: "ACKED",
    mockPriority: "P2",
    mockOwner: "Jordan K.",
  },
  {
    title: "Bugs & ops incidents",
    description: "Surface incidents from Slack and keep the queue under control before response debt piles up.",
    bullets: [
      "Highlight at-risk and breached work before it gets buried",
      "Assign owner and priority directly in the thread",
      "Link tracker work only when execution needs it",
      "Use stale and inactivity signals to keep unresolved work visible",
    ],
    mockSender: "PagerBot",
    mockInitials: "PB",
    mockMessage: "ALERT: Database connection pool at 95% capacity on prod-db-03.",
    mockStatus: "NEW",
    mockPriority: "P1",
    mockOwner: "Unassigned",
  },
];

function MiniSlackMock({
  sender,
  initials,
  message,
  status,
  priority,
  owner,
}: {
  sender: string;
  initials: string;
  message: string;
  status: string;
  priority: string;
  owner: string;
}) {
  const isNew = status === "NEW";
  return (
    <div className="overflow-hidden rounded-xl border border-border/60 bg-muted/30">
      <div className="flex items-center gap-1.5 border-b border-border/40 bg-muted/50 px-3 py-1.5">
        <div className="size-1.5 rounded-full bg-border" />
        <div className="size-1.5 rounded-full bg-border" />
        <div className="size-1.5 rounded-full bg-border" />
      </div>
      <div className="p-4">
        <div className="flex items-start gap-2.5">
          <div className="flex size-7 shrink-0 items-center justify-center rounded-md bg-muted text-[10px] font-bold text-muted-foreground">
            {initials}
          </div>
          <div className="flex flex-col gap-1">
            <span className="text-xs font-semibold text-foreground">{sender}</span>
            <p className="text-xs leading-relaxed text-foreground/75">{message}</p>
          </div>
        </div>
        <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-border/40 pt-3">
          <Badge
            className={`rounded-md px-2 py-0.5 text-[9px] font-bold uppercase tracking-wide ${
              isNew ? "bg-primary/10 text-primary hover:bg-primary/10" : "bg-success/10 text-success hover:bg-success/10"
            }`}
          >
            {status}
          </Badge>
          <Badge className="rounded-md bg-destructive/10 px-2 py-0.5 text-[9px] font-bold uppercase tracking-wide text-destructive hover:bg-destructive/10">
            {priority}
          </Badge>
          <div className="h-3 w-px bg-border" />
          <span className="flex items-center gap-1 text-[10px] text-muted-foreground">
            <User className="size-2.5" />
            {owner}
          </span>
        </div>
      </div>
    </div>
  );
}

export function UseCases() {
  return (
    <section className="border-b border-border/40 bg-muted/30 py-24 lg:py-32">
      <div className="mx-auto max-w-7xl px-6">
        <div className="mx-auto mb-16 max-w-2xl text-center">
          <p className="mb-3 text-sm font-semibold uppercase tracking-widest text-primary">Use cases</p>
          <h2 className="text-balance text-3xl font-bold tracking-tight text-foreground sm:text-4xl">Built for the work your team already runs in Slack</h2>
        </div>

        <div className="grid gap-6 lg:grid-cols-2">
          {useCases.map((uc) => (
            <div key={uc.title} className="flex flex-col gap-6 rounded-2xl border border-border/60 bg-card p-7 shadow-sm lg:p-8">
              <div className="flex flex-col gap-3">
                <h3 className="text-xl font-semibold text-foreground">{uc.title}</h3>
                <p className="text-sm leading-relaxed text-muted-foreground">{uc.description}</p>
                <ul className="mt-2 flex flex-col gap-2.5">
                  {uc.bullets.map((bullet) => (
                    <li key={bullet} className="flex items-start gap-2.5 text-sm text-muted-foreground">
                      <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-success" />
                      {bullet}
                    </li>
                  ))}
                </ul>
              </div>
              <MiniSlackMock
                sender={uc.mockSender}
                initials={uc.mockInitials}
                message={uc.mockMessage}
                status={uc.mockStatus}
                priority={uc.mockPriority}
                owner={uc.mockOwner}
              />
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
