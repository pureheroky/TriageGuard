"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, Clock3, Inbox, PauseCircle, ShieldAlert, UserX } from "lucide-react";
import { apiFetch } from "@/lib/api";
import { AdminShell } from "@/components/admin-shell";
import { RequireAuth } from "@/app/components/require-auth";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";

type QueueDetailResponse = {
  queue: {
    id: string;
    name: string;
    description?: string;
    default_priority: string;
    default_request_type: string;
    escalation_channel_id?: string;
    is_default: boolean;
  };
  policy: {
    ack_sla_minutes: number;
    assign_sla_minutes: number;
    stale_hours: number;
    digest_time: string;
    assign_starts_from: string;
    stale_starts_from: string;
    timezone: string;
    business_hours_enabled: boolean;
    business_hours_start?: string;
    business_hours_end?: string;
    business_days_mask: number;
  };
  created_count: number;
  unacked_count: number;
  unassigned_count: number;
  waiting_count: number;
  breached_count: number;
  at_risk_count: number;
  median_ack_minutes?: number;
  median_assign_minutes?: number;
  median_close_minutes?: number;
  reopen_rate: number;
  close_reasons: Record<string, number>;
};

function formatMinutes(value?: number): string {
  if (value == null) {
    return "—";
  }
  if (value >= 60) {
    return `${(value / 60).toFixed(1)}h`;
  }
  return `${value.toFixed(1)}m`;
}

function MetricCard({
  title,
  value,
  icon: Icon,
}: {
  title: string;
  value: string | number;
  icon: typeof Inbox;
}) {
  return (
    <Card className="border-border/60 shadow-sm">
      <CardContent className="flex items-center gap-4 p-5">
        <div className="flex size-12 items-center justify-center rounded-xl bg-primary/10">
          <Icon className="size-5 text-primary" />
        </div>
        <div>
          <p className="text-xs text-muted-foreground">{title}</p>
          <p className="text-2xl font-bold tabular-nums">{value}</p>
        </div>
      </CardContent>
    </Card>
  );
}

export default function QueueDetailPage() {
  const params = useParams<{ id: string }>();
  const queueID = Array.isArray(params?.id) ? params.id[0] : params?.id;

  const query = useQuery({
    queryKey: ["queue-detail", queueID],
    queryFn: () => apiFetch<QueueDetailResponse>(`/api/queues/${queueID}`),
    enabled: Boolean(queueID),
    refetchInterval: 10_000,
    refetchIntervalInBackground: true,
  });

  const detail = query.data;

  return (
    <RequireAuth>
      <AdminShell>
        <div className="mx-auto max-w-6xl space-y-6">
          <div className="space-y-3">
            <Link href="/dashboard" className="inline-flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-foreground">
              <ArrowLeft className="size-4" />
              Back to queue health
            </Link>
            {query.isLoading ? (
              <div className="space-y-2">
                <Skeleton className="h-8 w-60" />
                <Skeleton className="h-4 w-96" />
              </div>
            ) : (
              <div>
                <h1 className="text-2xl font-bold tracking-tight text-foreground">{detail?.queue.name ?? "Queue"}</h1>
                <p className="mt-1 text-sm text-muted-foreground">{detail?.queue.description || "Queue-level health, SLA posture and close-reason mix."}</p>
              </div>
            )}
          </div>

          {query.isLoading ? (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {Array.from({ length: 6 }).map((_, idx) => (
                <Card key={idx} className="border-border/60 shadow-sm">
                  <CardContent className="p-5">
                    <Skeleton className="h-16 w-full" />
                  </CardContent>
                </Card>
              ))}
            </div>
          ) : null}

          {detail ? (
            <>
              <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
                <MetricCard title="Requests in queue" value={detail.created_count} icon={Inbox} />
                <MetricCard title="Unacked" value={detail.unacked_count} icon={ShieldAlert} />
                <MetricCard title="Unassigned" value={detail.unassigned_count} icon={UserX} />
                <MetricCard title="Waiting / snoozed" value={detail.waiting_count} icon={PauseCircle} />
                <MetricCard title="Breached" value={detail.breached_count} icon={Clock3} />
                <MetricCard title="At risk" value={detail.at_risk_count} icon={Clock3} />
              </div>

              <div className="grid gap-4 lg:grid-cols-3">
                <MetricCard title="Median ack" value={formatMinutes(detail.median_ack_minutes)} icon={Clock3} />
                <MetricCard title="Median assign" value={formatMinutes(detail.median_assign_minutes)} icon={Clock3} />
                <MetricCard title="Median close" value={formatMinutes(detail.median_close_minutes)} icon={Clock3} />
              </div>

              <div className="grid gap-4 lg:grid-cols-2">
                <Card className="border-border/60 shadow-sm">
                  <CardHeader>
                    <CardTitle className="text-base font-semibold">Policy</CardTitle>
                    <CardDescription className="text-xs">Runtime queue policy used by clocks, routing defaults and digests.</CardDescription>
                  </CardHeader>
                  <CardContent className="grid gap-3 text-sm sm:grid-cols-2">
                    <div>
                      <p className="text-xs text-muted-foreground">Default priority</p>
                      <p className="font-medium text-foreground">{detail.queue.default_priority}</p>
                    </div>
                    <div>
                      <p className="text-xs text-muted-foreground">Default request type</p>
                      <p className="font-medium text-foreground">{detail.queue.default_request_type}</p>
                    </div>
                    <div>
                      <p className="text-xs text-muted-foreground">Ack SLA</p>
                      <p className="font-medium text-foreground">{detail.policy.ack_sla_minutes} min</p>
                    </div>
                    <div>
                      <p className="text-xs text-muted-foreground">Assign SLA</p>
                      <p className="font-medium text-foreground">{detail.policy.assign_sla_minutes} min</p>
                    </div>
                    <div>
                      <p className="text-xs text-muted-foreground">Stale window</p>
                      <p className="font-medium text-foreground">{detail.policy.stale_hours} h</p>
                    </div>
                    <div>
                      <p className="text-xs text-muted-foreground">Assign starts from</p>
                      <p className="font-medium text-foreground">{detail.policy.assign_starts_from}</p>
                    </div>
                    <div>
                      <p className="text-xs text-muted-foreground">Timezone</p>
                      <p className="font-medium text-foreground">{detail.policy.timezone}</p>
                    </div>
                    <div>
                      <p className="text-xs text-muted-foreground">Digest time</p>
                      <p className="font-medium text-foreground">{detail.policy.digest_time}</p>
                    </div>
                    <div className="sm:col-span-2">
                      <p className="text-xs text-muted-foreground">Business hours</p>
                      <p className="font-medium text-foreground">
                        {detail.policy.business_hours_enabled
                          ? `${detail.policy.business_hours_start || "09:00:00"} - ${detail.policy.business_hours_end || "18:00:00"}`
                          : "Disabled"}
                      </p>
                    </div>
                  </CardContent>
                </Card>

                <Card className="border-border/60 shadow-sm">
                  <CardHeader>
                    <CardTitle className="text-base font-semibold">Lifecycle</CardTitle>
                    <CardDescription className="text-xs">Close reasons and reopen rate for this queue.</CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    <div className="rounded-lg border border-border/50 px-4 py-3">
                      <p className="text-xs text-muted-foreground">Reopen rate</p>
                      <p className="text-2xl font-bold text-foreground">{(detail.reopen_rate * 100).toFixed(1)}%</p>
                    </div>
                    <div className="space-y-2">
                      {Object.entries(detail.close_reasons).map(([reason, count]) => (
                        <div key={reason} className="flex items-center justify-between rounded-lg border border-border/50 px-3 py-2">
                          <span className="text-sm font-medium text-foreground">{reason}</span>
                          <span className="text-sm tabular-nums text-muted-foreground">{count}</span>
                        </div>
                      ))}
                      {Object.keys(detail.close_reasons).length === 0 ? <p className="text-sm text-muted-foreground">No close reasons yet.</p> : null}
                    </div>
                  </CardContent>
                </Card>
              </div>
            </>
          ) : null}
        </div>
      </AdminShell>
    </RequireAuth>
  );
}
