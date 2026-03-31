"use client";

import { useQuery } from "@tanstack/react-query";
import { AdminShell } from "@/components/admin-shell";
import { RequireAuth } from "@/app/components/require-auth";
import { apiFetch } from "@/lib/api";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

type JobRuntimeStatus = {
  job_name: string;
  last_started_at?: string;
  last_finished_at?: string;
  last_success_at?: string;
  last_error?: string;
  last_duration_ms?: number;
};

type OpsSummary = {
  dead_letter_count: number;
  failed_slack_refreshes: number;
  failed_escalations: number;
  failed_tracker_syncs: number;
  retry_backlog_count: number;
  oldest_pending_age_minutes?: number;
  slack_connected: boolean;
  linear_connected: boolean;
  sync_lag_minutes?: number;
  health_banner: string;
  runners: JobRuntimeStatus[];
};

type DeadLetter = {
  id: string;
  source: string;
  operation: string;
  error_message: string;
  created_at: string;
};

type OutboxItem = {
  id: string;
  kind: string;
  status: string;
  retry_count: number;
  last_error?: string;
  updated_at: string;
};

type OpsEvents = {
  dead_letters: DeadLetter[];
  outbox: OutboxItem[];
};

function fmt(value?: string): string {
  if (!value) return "—";
  return new Date(value).toLocaleString();
}

export default function OpsPage() {
  const summaryQuery = useQuery({
    queryKey: ["workspace-ops-summary"],
    queryFn: () => apiFetch<OpsSummary>("/api/ops/summary"),
    refetchInterval: 15_000,
    refetchIntervalInBackground: true,
  });
  const eventsQuery = useQuery({
    queryKey: ["workspace-ops-events"],
    queryFn: () => apiFetch<OpsEvents>("/api/ops/events?limit=25"),
    refetchInterval: 15_000,
    refetchIntervalInBackground: true,
  });

  const summary = summaryQuery.data;
  const events = eventsQuery.data;

  return (
    <RequireAuth>
      <AdminShell>
        <div className="mx-auto max-w-7xl space-y-6">
          <div>
            <h1 className="text-2xl font-bold tracking-tight text-foreground">Ops Visibility</h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Failed refreshes, failed escalations, tracker sync issues, retries, and job health for this workspace.
            </p>
          </div>

          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <MetricCard title="Dead letters (7d)" value={summary?.dead_letter_count ?? 0} />
            <MetricCard title="Retry backlog" value={summary?.retry_backlog_count ?? 0} />
            <MetricCard title="Failed escalations" value={summary?.failed_escalations ?? 0} />
            <MetricCard title="Failed tracker syncs" value={summary?.failed_tracker_syncs ?? 0} />
          </div>

          <div className="grid gap-6 lg:grid-cols-2">
            <Card className="border-border/60 shadow-sm">
              <CardHeader>
                <CardTitle className="text-base font-semibold">Integration Health</CardTitle>
                <CardDescription className="text-xs">{summary?.health_banner || "Loading workspace health..."}</CardDescription>
              </CardHeader>
              <CardContent className="space-y-2 text-sm text-muted-foreground">
                <p>Slack: {summary?.slack_connected ? "Connected" : "Not connected"}</p>
                <p>Linear: {summary?.linear_connected ? "Connected" : "Not connected"}</p>
                <p>Oldest pending side effect: {summary?.oldest_pending_age_minutes != null ? `${summary.oldest_pending_age_minutes}m` : "—"}</p>
                <p>Sync lag: {summary?.sync_lag_minutes != null ? `${summary.sync_lag_minutes}m` : "—"}</p>
              </CardContent>
            </Card>

            <Card className="border-border/60 shadow-sm">
              <CardHeader>
                <CardTitle className="text-base font-semibold">Runner Health</CardTitle>
                <CardDescription className="text-xs">SLA runner and notification dispatcher runtime state.</CardDescription>
              </CardHeader>
              <CardContent className="space-y-3">
                {(summary?.runners || []).map((runner) => (
                  <div key={runner.job_name} className="rounded-lg border border-border/50 px-3 py-2">
                    <p className="text-sm font-medium text-foreground">{runner.job_name}</p>
                    <p className="text-xs text-muted-foreground">Last success: {fmt(runner.last_success_at)}</p>
                    <p className="text-xs text-muted-foreground">Last duration: {runner.last_duration_ms != null ? `${runner.last_duration_ms}ms` : "—"}</p>
                    <p className="text-xs text-muted-foreground">Last error: {runner.last_error || "—"}</p>
                  </div>
                ))}
              </CardContent>
            </Card>
          </div>

          <div className="grid gap-6 lg:grid-cols-2">
            <Card className="border-border/60 shadow-sm">
              <CardHeader>
                <CardTitle className="text-base font-semibold">Dead Letters</CardTitle>
              </CardHeader>
              <CardContent className="p-0">
                <Table>
                  <TableHeader>
                    <TableRow className="border-border/40 hover:bg-transparent">
                      <TableHead className="pl-6 text-xs">Source</TableHead>
                      <TableHead className="text-xs">Operation</TableHead>
                      <TableHead className="pr-6 text-xs">Created</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {(events?.dead_letters || []).map((item) => (
                      <TableRow key={item.id} className="border-border/40">
                        <TableCell className="pl-6 text-xs">{item.source}</TableCell>
                        <TableCell className="text-xs">{item.operation}</TableCell>
                        <TableCell className="pr-6 text-xs">{fmt(item.created_at)}</TableCell>
                      </TableRow>
                    ))}
                    {(events?.dead_letters || []).length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={3} className="py-8 text-center text-sm text-muted-foreground">
                          No dead letters.
                        </TableCell>
                      </TableRow>
                    ) : null}
                  </TableBody>
                </Table>
              </CardContent>
            </Card>

            <Card className="border-border/60 shadow-sm">
              <CardHeader>
                <CardTitle className="text-base font-semibold">Outbox Backlog</CardTitle>
              </CardHeader>
              <CardContent className="p-0">
                <Table>
                  <TableHeader>
                    <TableRow className="border-border/40 hover:bg-transparent">
                      <TableHead className="pl-6 text-xs">Kind</TableHead>
                      <TableHead className="text-xs">Status</TableHead>
                      <TableHead className="text-xs">Retries</TableHead>
                      <TableHead className="pr-6 text-xs">Updated</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {(events?.outbox || []).map((item) => (
                      <TableRow key={item.id} className="border-border/40">
                        <TableCell className="pl-6 text-xs">{item.kind}</TableCell>
                        <TableCell className="text-xs">{item.status}</TableCell>
                        <TableCell className="text-xs">{item.retry_count}</TableCell>
                        <TableCell className="pr-6 text-xs">{fmt(item.updated_at)}</TableCell>
                      </TableRow>
                    ))}
                    {(events?.outbox || []).length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={4} className="py-8 text-center text-sm text-muted-foreground">
                          No backlog items.
                        </TableCell>
                      </TableRow>
                    ) : null}
                  </TableBody>
                </Table>
              </CardContent>
            </Card>
          </div>
        </div>
      </AdminShell>
    </RequireAuth>
  );
}

function MetricCard({ title, value }: { title: string; value: string | number }) {
  return (
    <Card className="border-border/60 shadow-sm">
      <CardContent className="p-5">
        <p className="text-xs text-muted-foreground">{title}</p>
        <p className="mt-1 text-2xl font-bold tabular-nums text-foreground">{value}</p>
      </CardContent>
    </Card>
  );
}
