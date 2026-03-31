"use client";

import { useQuery } from "@tanstack/react-query";
import { AdminShell } from "@/components/admin-shell";
import { RequireAuth } from "@/app/components/require-auth";
import { apiFetch } from "@/lib/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

type JobRuntimeStatus = {
  job_name: string;
  last_success_at?: string;
  last_error?: string;
  last_duration_ms?: number;
};

type InternalOpsSummary = {
  dead_letter_count: number;
  failed_outbox_count: number;
  retry_backlog_count: number;
  oldest_pending_age_minutes?: number;
  workspaces_affected: number;
  runners: JobRuntimeStatus[];
};

type DeadLetter = {
  id: string;
  source: string;
  operation: string;
  error_message: string;
  created_at: string;
  workspace_id?: string;
};

type OutboxItem = {
  id: string;
  kind: string;
  status: string;
  retry_count: number;
  updated_at: string;
  last_error?: string;
};

function fmt(value?: string): string {
  if (!value) return "—";
  return new Date(value).toLocaleString();
}

export default function InternalOpsPage() {
  const summaryQuery = useQuery({
    queryKey: ["internal-ops-summary"],
    queryFn: () => apiFetch<InternalOpsSummary>("/api/internal/ops/summary"),
    refetchInterval: 15_000,
    refetchIntervalInBackground: true,
  });
  const deadLettersQuery = useQuery({
    queryKey: ["internal-ops-dead-letters"],
    queryFn: () => apiFetch<{ dead_letters: DeadLetter[] }>("/api/internal/ops/dead-letters?limit=50"),
    refetchInterval: 15_000,
    refetchIntervalInBackground: true,
  });
  const outboxQuery = useQuery({
    queryKey: ["internal-ops-outbox"],
    queryFn: () => apiFetch<{ outbox: OutboxItem[] }>("/api/internal/ops/outbox?limit=50"),
    refetchInterval: 15_000,
    refetchIntervalInBackground: true,
  });

  const summary = summaryQuery.data;
  const deadLetters = deadLettersQuery.data?.dead_letters ?? [];
  const outbox = outboxQuery.data?.outbox ?? [];

  return (
    <RequireAuth>
      <AdminShell>
        <div className="mx-auto max-w-7xl space-y-6">
          <div>
            <h1 className="text-2xl font-bold tracking-tight text-foreground">Internal Ops</h1>
            <p className="mt-1 text-sm text-muted-foreground">Cross-workspace reliability view for dead letters, outbox backlog, and runner health.</p>
          </div>

          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <MetricCard title="Dead letters (7d)" value={summary?.dead_letter_count ?? 0} />
            <MetricCard title="Failed outbox" value={summary?.failed_outbox_count ?? 0} />
            <MetricCard title="Retry backlog" value={summary?.retry_backlog_count ?? 0} />
            <MetricCard title="Workspaces affected" value={summary?.workspaces_affected ?? 0} />
          </div>

          <div className="grid gap-6 lg:grid-cols-2">
            <Card className="border-border/60 shadow-sm">
              <CardHeader>
                <CardTitle className="text-base font-semibold">Runner Health</CardTitle>
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

            <Card className="border-border/60 shadow-sm">
              <CardHeader>
                <CardTitle className="text-base font-semibold">Backlog</CardTitle>
              </CardHeader>
              <CardContent className="space-y-2 text-sm text-muted-foreground">
                <p>Oldest pending side effect: {summary?.oldest_pending_age_minutes != null ? `${summary.oldest_pending_age_minutes}m` : "—"}</p>
                <p>Use this view to spot degraded escalation delivery, Slack refresh failures, or tracker sync backlog before customers notice.</p>
              </CardContent>
            </Card>
          </div>

          <div className="grid gap-6 lg:grid-cols-2">
            <Card className="border-border/60 shadow-sm">
              <CardHeader>
                <CardTitle className="text-base font-semibold">Recent Dead Letters</CardTitle>
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
                    {deadLetters.map((item) => (
                      <TableRow key={item.id} className="border-border/40">
                        <TableCell className="pl-6 text-xs">{item.source}</TableCell>
                        <TableCell className="text-xs">{item.operation}</TableCell>
                        <TableCell className="pr-6 text-xs">{fmt(item.created_at)}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </CardContent>
            </Card>

            <Card className="border-border/60 shadow-sm">
              <CardHeader>
                <CardTitle className="text-base font-semibold">Outbox Failures / Retries</CardTitle>
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
                    {outbox.map((item) => (
                      <TableRow key={item.id} className="border-border/40">
                        <TableCell className="pl-6 text-xs">{item.kind}</TableCell>
                        <TableCell className="text-xs">{item.status}</TableCell>
                        <TableCell className="text-xs">{item.retry_count}</TableCell>
                        <TableCell className="pr-6 text-xs">{fmt(item.updated_at)}</TableCell>
                      </TableRow>
                    ))}
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
