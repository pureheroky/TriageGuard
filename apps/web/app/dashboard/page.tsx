"use client";

import Link from "next/link";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Clock3, Download, FileText, Inbox, PauseCircle, ShieldAlert, UserX } from "lucide-react";
import { apiFetch } from "@/lib/api";
import { AdminShell } from "@/components/admin-shell";
import { RequireAuth } from "@/app/components/require-auth";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";

type DashboardSummary = {
  open_count: number;
  unacked_count: number;
  unassigned_count: number;
  waiting_count: number;
  breached_count: number;
  at_risk_count: number;
  waiting_debt_count: number;
  no_human_activity_count: number;
  median_ack_minutes?: number;
  median_assign_minutes?: number;
  median_close_minutes?: number;
};

type QueueSummaryRow = {
  queue: {
    id: string;
    name: string;
    is_default: boolean;
  };
  open_count: number;
  unacked_count: number;
  unassigned_count: number;
  waiting_count: number;
  breached_count: number;
  breached_stale_count: number;
  at_risk_count: number;
  waiting_debt_count: number;
  no_human_activity_count: number;
  median_ack_minutes?: number;
  median_assign_minutes?: number;
  median_close_minutes?: number;
};

type QueueAgingRow = {
  queue_id: string;
  queue_name: string;
  open_count: number;
  avg_open_age_hours: number;
  max_open_age_hours: number;
};

type TypeSummaryRow = {
  request_type: string;
  open_count: number;
  waiting_count: number;
  unassigned_count: number;
  breached_count: number;
  no_human_activity_count: number;
  median_ack_minutes?: number;
  median_assign_minutes?: number;
  median_close_minutes?: number;
};

type DashboardResponse = {
  summary: DashboardSummary;
  queues: QueueSummaryRow[];
  top_breached_queues: QueueSummaryRow[];
  top_stale_queues: QueueSummaryRow[];
  aging_by_queue: QueueAgingRow[];
  aging_by_type: TypeSummaryRow[];
  waiting_debt: QueueSummaryRow[];
  unassigned_debt: QueueSummaryRow[];
  no_human_activity: QueueSummaryRow[];
};

type MeResponse = {
  billing?: {
    effective_plan?: string;
    status?: string;
    entitlements?: {
      exports?: boolean;
    };
  };
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

function DashboardSkeleton() {
  return (
    <div className="mx-auto max-w-7xl space-y-6">
      <div className="space-y-2">
        <Skeleton className="h-8 w-44" />
        <Skeleton className="h-4 w-72" />
      </div>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 6 }).map((_, idx) => (
          <Card key={idx} className="border-border/60 shadow-sm">
            <CardContent className="p-5">
              <Skeleton className="h-16 w-full" />
            </CardContent>
          </Card>
        ))}
      </div>
      <Card className="border-border/60 shadow-sm">
        <CardContent className="p-5">
          <Skeleton className="h-64 w-full" />
        </CardContent>
      </Card>
    </div>
  );
}

export default function DashboardPage() {
  const [windowDays, setWindowDays] = useState<7 | 30 | 90>(30);
  const queryClient = useQueryClient();
  const meQuery = useQuery({
    queryKey: ["me"],
    queryFn: () => apiFetch<MeResponse>("/api/me"),
  });
  const sampleSeedMutation = useMutation({
    mutationFn: () => apiFetch<{ ok: boolean }>("/api/onboarding/sample-data", { method: "POST" }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["queue-dashboard"] });
      queryClient.invalidateQueries({ queryKey: ["requests-summary"] });
      queryClient.invalidateQueries({ queryKey: ["queues"] });
    },
  });
  const query = useQuery({
    queryKey: ["queue-dashboard"],
    queryFn: () => apiFetch<DashboardResponse>("/api/requests/summary"),
    refetchInterval: 10_000,
    refetchIntervalInBackground: true,
  });

  if (query.isLoading || meQuery.isLoading) {
    return (
      <RequireAuth>
        <AdminShell>
          <DashboardSkeleton />
        </AdminShell>
      </RequireAuth>
    );
  }

  const summary = query.data?.summary;
  const queues = query.data?.queues ?? [];
  const topBreachedQueues = query.data?.top_breached_queues ?? [];
  const topStaleQueues = query.data?.top_stale_queues ?? [];
  const agingByQueue = query.data?.aging_by_queue ?? [];
  const agingByType = query.data?.aging_by_type ?? [];
  const waitingDebt = query.data?.waiting_debt ?? [];
  const unassignedDebt = query.data?.unassigned_debt ?? [];
  const noHumanActivity = query.data?.no_human_activity ?? [];
  const canExport = Boolean(meQuery.data?.billing?.entitlements?.exports);

  return (
    <RequireAuth>
      <AdminShell>
        <div className="mx-auto max-w-7xl space-y-6">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h1 className="text-2xl font-bold tracking-tight text-foreground">Queue Health</h1>
              <p className="mt-1 text-sm text-muted-foreground">
                See what is unacked, unassigned, at risk, waiting, or simply forgotten before it turns into process debt.
              </p>
            </div>
            <div className="flex items-center gap-2">
              {(summary?.open_count ?? 0) === 0 ? (
                <Button size="sm" variant="outline" className="h-8 px-3 text-xs" onClick={() => sampleSeedMutation.mutate()} disabled={sampleSeedMutation.isPending}>
                  {sampleSeedMutation.isPending ? "Loading sample..." : "Load sample data"}
                </Button>
              ) : null}
              {canExport ? (
                <>
                  <Button
                    size="sm"
                    variant="outline"
                    className="h-8 gap-1.5 px-3 text-xs"
                    onClick={() => window.open(`/api/backend/api/reports/requests.csv?days=${windowDays}`, "_blank", "noopener,noreferrer")}
                  >
                    <Download className="size-3.5" />
                    Requests CSV
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    className="h-8 gap-1.5 px-3 text-xs"
                    onClick={() => window.open(`/api/backend/api/reports/requests.pdf?days=${windowDays}`, "_blank", "noopener,noreferrer")}
                  >
                    <FileText className="size-3.5" />
                    Requests PDF
                  </Button>
                </>
              ) : null}
              <div className="inline-flex items-center gap-1 rounded-lg border border-border/60 bg-card p-1">
                {[7, 30, 90].map((days) => (
                  <Button
                    key={days}
                    type="button"
                    size="sm"
                    variant={windowDays === days ? "default" : "ghost"}
                    className="h-8 px-3 text-xs"
                    onClick={() => setWindowDays(days as 7 | 30 | 90)}
                  >
                    {days}d
                  </Button>
                ))}
              </div>
            </div>
          </div>

          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <MetricCard title="Open requests" value={summary?.open_count ?? 0} icon={Inbox} />
            <MetricCard title="Unacked" value={summary?.unacked_count ?? 0} icon={ShieldAlert} />
            <MetricCard title="Unassigned" value={summary?.unassigned_count ?? 0} icon={UserX} />
            <MetricCard title="Waiting / snoozed" value={summary?.waiting_count ?? 0} icon={PauseCircle} />
            <MetricCard title="Breached" value={summary?.breached_count ?? 0} icon={AlertTriangle} />
            <MetricCard title="At risk" value={summary?.at_risk_count ?? 0} icon={Clock3} />
          </div>

          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <MetricCard title="Waiting debt" value={summary?.waiting_debt_count ?? 0} icon={PauseCircle} />
            <MetricCard title="No human activity" value={summary?.no_human_activity_count ?? 0} icon={Clock3} />
            <MetricCard title="Median ack" value={formatMinutes(summary?.median_ack_minutes)} icon={Clock3} />
          </div>

          <div className="grid gap-4 lg:grid-cols-2">
            <MetricCard title="Median assign" value={formatMinutes(summary?.median_assign_minutes)} icon={Clock3} />
            <MetricCard title="Median close" value={formatMinutes(summary?.median_close_minutes)} icon={Clock3} />
          </div>

          <Card className="border-border/60 shadow-sm">
            <CardHeader>
              <CardTitle className="text-base font-semibold">Queues</CardTitle>
              <CardDescription className="text-xs">Ownership, SLA risk, waiting debt, and inactivity in one list.</CardDescription>
            </CardHeader>
            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow className="border-border/40 hover:bg-transparent">
                    <TableHead className="pl-6 text-xs">Queue</TableHead>
                    <TableHead className="text-xs">Open</TableHead>
                    <TableHead className="text-xs">Unacked</TableHead>
                    <TableHead className="text-xs">Unassigned</TableHead>
                    <TableHead className="text-xs">Waiting debt</TableHead>
                    <TableHead className="text-xs">No activity</TableHead>
                    <TableHead className="text-xs">Breached</TableHead>
                    <TableHead className="text-xs">At risk</TableHead>
                    <TableHead className="pr-6 text-xs">Median ack</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {queues.map((row) => (
                    <TableRow key={row.queue.id} className="border-border/40">
                      <TableCell className="pl-6 text-sm font-medium text-foreground">
                        <Link href={`/queues/${row.queue.id}`} className="transition-colors hover:text-primary">
                          {row.queue.name}
                        </Link>
                        {row.queue.is_default ? <span className="ml-2 text-xs text-muted-foreground">Default</span> : null}
                      </TableCell>
                      <TableCell className="text-xs tabular-nums">{row.open_count}</TableCell>
                      <TableCell className="text-xs tabular-nums">{row.unacked_count}</TableCell>
                      <TableCell className="text-xs tabular-nums">{row.unassigned_count}</TableCell>
                      <TableCell className="text-xs tabular-nums">{row.waiting_debt_count}</TableCell>
                      <TableCell className="text-xs tabular-nums">{row.no_human_activity_count}</TableCell>
                      <TableCell className="text-xs tabular-nums">{row.breached_count}</TableCell>
                      <TableCell className="text-xs tabular-nums">{row.at_risk_count}</TableCell>
                      <TableCell className="pr-6 text-xs">{formatMinutes(row.median_ack_minutes)}</TableCell>
                    </TableRow>
                  ))}
                  {queues.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={9} className="py-10 text-center text-sm text-muted-foreground">
                        No queues yet. Use onboarding or sample data to see queue health in action.
                      </TableCell>
                    </TableRow>
                  ) : null}
                </TableBody>
              </Table>
            </CardContent>
          </Card>

          <div className="grid gap-6 lg:grid-cols-2">
            <Card className="border-border/60 shadow-sm">
              <CardHeader>
                <CardTitle className="text-base font-semibold">Aging by Queue</CardTitle>
                <CardDescription className="text-xs">Queues with the oldest average open work bubble to the top.</CardDescription>
              </CardHeader>
              <CardContent className="space-y-3">
                {agingByQueue.slice(0, 5).map((row) => (
                  <div key={row.queue_id} className="flex items-center justify-between rounded-lg border border-border/50 px-3 py-2">
                    <div>
                      <p className="text-sm font-medium text-foreground">{row.queue_name}</p>
                      <p className="text-xs text-muted-foreground">{row.open_count} open</p>
                    </div>
                    <div className="text-right">
                      <p className="text-sm font-semibold text-foreground">{row.avg_open_age_hours.toFixed(1)}h avg</p>
                      <p className="text-xs text-muted-foreground">{row.max_open_age_hours.toFixed(1)}h max</p>
                    </div>
                  </div>
                ))}
                {agingByQueue.length === 0 ? <p className="text-sm text-muted-foreground">No aging data yet.</p> : null}
              </CardContent>
            </Card>

            <Card className="border-border/60 shadow-sm">
              <CardHeader>
                <CardTitle className="text-base font-semibold">Aging by Type</CardTitle>
                <CardDescription className="text-xs">Compare request types by load, debt, and response speed.</CardDescription>
              </CardHeader>
              <CardContent className="space-y-3">
                {agingByType.slice(0, 6).map((row) => (
                  <div key={row.request_type} className="grid grid-cols-2 gap-3 rounded-lg border border-border/50 px-3 py-2 text-sm">
                    <div>
                      <p className="font-medium capitalize text-foreground">{row.request_type}</p>
                      <p className="text-xs text-muted-foreground">
                        Open {row.open_count} · Waiting {row.waiting_count} · Unassigned {row.unassigned_count}
                      </p>
                    </div>
                    <div className="text-right">
                      <p className="font-semibold text-foreground">Ack {formatMinutes(row.median_ack_minutes)}</p>
                      <p className="text-xs text-muted-foreground">Assign {formatMinutes(row.median_assign_minutes)}</p>
                    </div>
                  </div>
                ))}
                {agingByType.length === 0 ? <p className="text-sm text-muted-foreground">No type-level analytics yet.</p> : null}
              </CardContent>
            </Card>
          </div>

          <div className="grid gap-6 lg:grid-cols-3">
            <DebtCard title="Waiting Debt" rows={waitingDebt} valueKey="waiting_debt_count" />
            <DebtCard title="Unassigned Debt" rows={unassignedDebt} valueKey="unassigned_count" />
            <DebtCard title="No Human Activity" rows={noHumanActivity} valueKey="no_human_activity_count" />
          </div>

          <div className="grid gap-6 lg:grid-cols-2">
            <CompactQueueCard title="Top breached queues" rows={topBreachedQueues} valueKey="breached_count" />
            <CompactQueueCard title="Top stale queues" rows={topStaleQueues} valueKey="breached_stale_count" />
          </div>
        </div>
      </AdminShell>
    </RequireAuth>
  );
}

function DebtCard({
  title,
  rows,
  valueKey,
}: {
  title: string;
  rows: QueueSummaryRow[];
  valueKey: "waiting_debt_count" | "unassigned_count" | "no_human_activity_count";
}) {
  const nonZeroRows = rows.filter((row) => row[valueKey] > 0).slice(0, 5);
  return (
    <Card className="border-border/60 shadow-sm">
      <CardHeader>
        <CardTitle className="text-base font-semibold">{title}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {nonZeroRows.map((row) => (
          <div key={row.queue.id} className="flex items-center justify-between rounded-lg border border-border/50 px-3 py-2">
            <p className="text-sm font-medium text-foreground">{row.queue.name}</p>
            <p className="text-sm font-semibold text-foreground">{row[valueKey]}</p>
          </div>
        ))}
        {nonZeroRows.length === 0 ? <p className="text-sm text-muted-foreground">No current debt.</p> : null}
      </CardContent>
    </Card>
  );
}

function CompactQueueCard({
  title,
  rows,
  valueKey,
}: {
  title: string;
  rows: QueueSummaryRow[];
  valueKey: "breached_count" | "breached_stale_count";
}) {
  return (
    <Card className="border-border/60 shadow-sm">
      <CardHeader>
        <CardTitle className="text-base font-semibold">{title}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {rows.map((row) => (
          <div key={row.queue.id} className="flex items-center justify-between rounded-lg border border-border/50 px-3 py-2">
            <div>
              <Link href={`/queues/${row.queue.id}`} className="text-sm font-medium text-foreground transition-colors hover:text-primary">
                {row.queue.name}
              </Link>
              <p className="text-xs text-muted-foreground">{row.open_count} open</p>
            </div>
            <span className="text-sm font-semibold text-foreground">{row[valueKey]}</span>
          </div>
        ))}
        {rows.length === 0 ? <p className="text-sm text-muted-foreground">No queues in this view.</p> : null}
      </CardContent>
    </Card>
  );
}
