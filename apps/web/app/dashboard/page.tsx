"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, Briefcase, CheckCircle2, Clock3, Download, FileText, Inbox, UserX } from "lucide-react";
import { apiFetch } from "@/lib/api";
import { AdminShell } from "@/components/admin-shell";
import { RequireAuth } from "@/app/components/require-auth";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

type Summary = {
  new_count: number;
  unassigned_count: number;
  overdue_count: number;
  assigned_count?: number;
  open_count?: number;
};

type OverdueRow = {
  id: string;
  status: string;
  priority: string;
  title?: string;
  channel_id: string;
  thread_ts: string;
  thread_url?: string;
};

type DurationMetric = {
  avg_minutes: number;
  sample_size: number;
};

type TrendPoint = {
  date: string;
  ack_avg_minutes?: number;
  ack_sample_size: number;
  resolve_avg_minutes?: number;
  resolve_sample_size: number;
};

type Analytics = {
  window_days: number;
  ack: DurationMetric;
  resolve: DurationMetric;
  trend: TrendPoint[];
};

type MeResponse = {
  billing?: {
    effective_plan?: string;
    status?: string;
  };
};

function isPaidActive(status?: string): boolean {
  return ["active", "trialing", "past_due", "unpaid", "incomplete"].includes((status || "").toLowerCase());
}

function formatAvgMinutes(metric: DurationMetric): string {
  if (metric.sample_size === 0) {
    return "—";
  }
  if (metric.avg_minutes >= 60) {
    return `${(metric.avg_minutes / 60).toFixed(1)}h`;
  }
  return `${metric.avg_minutes.toFixed(1)}m`;
}

function formatTrendMinutes(value?: number): string {
  if (value == null) {
    return "—";
  }
  if (value >= 60) {
    return `${(value / 60).toFixed(1)}h`;
  }
  return `${value.toFixed(1)}m`;
}

function niceChartMax(value: number): number {
  if (value <= 1) {
    return 1;
  }

  const magnitude = Math.pow(10, Math.floor(Math.log10(value)));
  const normalized = value / magnitude;

  if (normalized <= 1) {
    return magnitude;
  }
  if (normalized <= 2) {
    return 2 * magnitude;
  }
  if (normalized <= 5) {
    return 5 * magnitude;
  }
  return 10 * magnitude;
}

function TrendChart({ trend }: { trend: TrendPoint[] }) {
  const [hoveredIndex, setHoveredIndex] = useState<number | null>(null);

  const chart = useMemo(() => {
    const width = 860;
    const height = 300;
    const padding = { top: 18, right: 22, bottom: 40, left: 56 };
    const innerWidth = width - padding.left - padding.right;
    const innerHeight = height - padding.top - padding.bottom;
    const columnWidth = trend.length > 0 ? innerWidth / trend.length : innerWidth;

    const points = trend.map((entry, idx) => ({
      idx,
      x: padding.left + columnWidth * (idx + 0.5),
      ack: entry.ack_avg_minutes,
      ackSample: entry.ack_sample_size,
      resolve: entry.resolve_avg_minutes,
      resolveSample: entry.resolve_sample_size,
      date: entry.date,
    }));

    const maxY = niceChartMax(
      Math.max(
        1,
        ...points.map((point) => point.ack ?? 0),
        ...points.map((point) => point.resolve ?? 0),
      ),
    );

    const yTicks = 5;
    const yTickValues = Array.from({ length: yTicks }, (_, idx) => {
      const step = maxY / (yTicks - 1);
      return maxY - idx * step;
    });

    const toY = (value?: number) => {
      if (value == null) {
        return null;
      }
      return padding.top + (1 - value / maxY) * innerHeight;
    };

    const linePath = (series: "ack" | "resolve") => {
      let started = false;
      let d = "";
      for (const point of points) {
        const y = toY(point[series]);
        if (y == null) {
          continue;
        }
        if (!started) {
          d += `M ${point.x.toFixed(2)} ${y.toFixed(2)} `;
          started = true;
        } else {
          d += `L ${point.x.toFixed(2)} ${y.toFixed(2)} `;
        }
      }
      return d.trim();
    };

    const areaPath = (series: "ack" | "resolve") => {
      const seriesPoints = points
        .map((point) => {
          const y = toY(point[series]);
          if (y == null) {
            return null;
          }
          return { x: point.x, y };
        })
        .filter((point): point is { x: number; y: number } => point != null);

      if (seriesPoints.length < 2) {
        return "";
      }

      const baseline = padding.top + innerHeight;
      let d = `M ${seriesPoints[0].x.toFixed(2)} ${baseline.toFixed(2)} `;
      for (const point of seriesPoints) {
        d += `L ${point.x.toFixed(2)} ${point.y.toFixed(2)} `;
      }
      d += `L ${seriesPoints[seriesPoints.length - 1].x.toFixed(2)} ${baseline.toFixed(2)} Z`;
      return d.trim();
    };

    const hoverBands = points.map((point, idx) => {
      return {
        idx: point.idx,
        xStart: padding.left + columnWidth * idx,
        xEnd: padding.left + columnWidth * (idx + 1),
      };
    });

    let defaultIndex: number | null = null;
    for (let idx = points.length - 1; idx >= 0; idx -= 1) {
      const point = points[idx];
      if (point.ack != null || point.resolve != null) {
        defaultIndex = idx;
        break;
      }
    }

    return {
      width,
      height,
      padding,
      innerHeight,
      points,
      yTickValues,
      toY,
      ackPath: linePath("ack"),
      resolvePath: linePath("resolve"),
      ackAreaPath: areaPath("ack"),
      resolveAreaPath: areaPath("resolve"),
      hoverBands,
      defaultIndex,
    };
  }, [trend]);

  const hasData = trend.some((point) => point.ack_sample_size > 0 || point.resolve_sample_size > 0);
  if (!hasData) {
    return <p className="text-sm text-muted-foreground">No analytics data for selected window yet.</p>;
  }

  const selectedIndex = hoveredIndex ?? chart.defaultIndex;
  const selectedPoint = selectedIndex != null ? chart.points[selectedIndex] : null;
  const xLabelStep = Math.max(1, Math.ceil(chart.points.length / 6));

  return (
    <div className="space-y-3">
      <div className="w-full overflow-x-auto rounded-lg border border-border/50 bg-muted/20 p-3">
        <svg
          viewBox={`0 0 ${chart.width} ${chart.height}`}
          className="h-[280px] w-full min-w-[760px]"
          onMouseLeave={() => setHoveredIndex(null)}
        >
          {chart.yTickValues.map((tickValue, idx) => {
            const y = chart.toY(tickValue);
            if (y == null) {
              return null;
            }
            const isZero = Math.abs(tickValue) < 0.0001;
            return (
              <g key={`y-${idx}`}>
                <line
                  x1={chart.padding.left}
                  y1={y}
                  x2={chart.width - chart.padding.right}
                  y2={y}
                  stroke="hsl(var(--border))"
                  strokeWidth={isZero ? 1.5 : 1}
                  strokeDasharray={isZero ? "0" : "4 4"}
                />
                <text x={10} y={y + 3} className="fill-muted-foreground text-[10px]">
                  {formatTrendMinutes(tickValue).replace(".0", "")}
                </text>
              </g>
            );
          })}

          <line
            x1={chart.padding.left}
            y1={chart.padding.top}
            x2={chart.padding.left}
            y2={chart.height - chart.padding.bottom}
            stroke="hsl(var(--border))"
            strokeWidth="1"
          />

          {chart.hoverBands.map((band) => (
            <rect
              key={`hover-${band.idx}`}
              x={band.xStart}
              y={chart.padding.top}
              width={Math.max(1, band.xEnd - band.xStart)}
              height={chart.innerHeight}
              fill="transparent"
              onMouseEnter={() => setHoveredIndex(band.idx)}
            />
          ))}

          {selectedPoint && (
            (() => {
              const band = chart.hoverBands.find((item) => item.idx === selectedPoint.idx);
              if (!band) {
                return null;
              }
              return (
                <rect
                  x={band.xStart}
                  y={chart.padding.top}
                  width={Math.max(1, band.xEnd - band.xStart)}
                  height={chart.innerHeight}
                  fill="hsl(var(--muted))"
                  opacity="0.35"
                />
              );
            })()
          )}

          {chart.ackAreaPath && (
            <path d={chart.ackAreaPath} fill="var(--color-chart-1)" opacity="0.12" stroke="none" />
          )}
          {chart.resolveAreaPath && (
            <path d={chart.resolveAreaPath} fill="var(--color-chart-2)" opacity="0.1" stroke="none" />
          )}

          {chart.ackPath && (
            <path
              d={chart.ackPath}
              fill="none"
              stroke="var(--color-chart-1)"
              strokeWidth="3.5"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          )}
          {chart.resolvePath && (
            <path
              d={chart.resolvePath}
              fill="none"
              stroke="var(--color-chart-2)"
              strokeWidth="3.5"
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeDasharray="8 6"
            />
          )}

          {chart.points.map((point) => {
            const ackY = chart.toY(point.ack);
            const resolveY = chart.toY(point.resolve);
            const isSelected = selectedPoint?.idx === point.idx;
            return (
              <g key={point.date}>
                {ackY != null && (
                  <circle
                    cx={point.x}
                    cy={ackY}
                    r={isSelected ? 4.5 : 3}
                    fill="var(--color-chart-1)"
                    stroke="hsl(var(--background))"
                    strokeWidth="1.5"
                  />
                )}
                {resolveY != null && (
                  <circle
                    cx={point.x}
                    cy={resolveY}
                    r={isSelected ? 4.5 : 3}
                    fill="var(--color-chart-2)"
                    stroke="hsl(var(--background))"
                    strokeWidth="1.5"
                  />
                )}
              </g>
            );
          })}

          {chart.points.map((point, idx) => {
            if (idx % xLabelStep !== 0 && idx !== chart.points.length - 1) {
              return null;
            }
            return (
              <text
                key={`label-${point.date}`}
                x={point.x}
                y={chart.height - 10}
                textAnchor="middle"
                className="fill-muted-foreground text-[10px]"
              >
                {point.date.slice(5)}
              </text>
            );
          })}
        </svg>
      </div>

      {selectedPoint && (
        <div className="grid gap-2 sm:grid-cols-3">
          <div className="rounded-md border border-border/60 bg-card px-3 py-2">
            <p className="text-[11px] uppercase tracking-wide text-muted-foreground">Date</p>
            <p className="text-sm font-medium text-foreground">{selectedPoint.date}</p>
          </div>
          <div className="rounded-md border border-border/60 bg-card px-3 py-2">
            <p className="text-[11px] uppercase tracking-wide text-muted-foreground">Ack avg</p>
            <p className="text-sm font-medium text-foreground">{formatTrendMinutes(selectedPoint.ack)}</p>
            <p className="text-[11px] text-muted-foreground">{selectedPoint.ackSample} samples</p>
          </div>
          <div className="rounded-md border border-border/60 bg-card px-3 py-2">
            <p className="text-[11px] uppercase tracking-wide text-muted-foreground">Resolve avg</p>
            <p className="text-sm font-medium text-foreground">{formatTrendMinutes(selectedPoint.resolve)}</p>
            <p className="text-[11px] text-muted-foreground">{selectedPoint.resolveSample} samples</p>
          </div>
        </div>
      )}
    </div>
  );
}

function DashboardSkeleton() {
  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="space-y-2">
          <Skeleton className="h-8 w-44" />
          <Skeleton className="h-4 w-72" />
        </div>
        <Skeleton className="h-10 w-56" />
      </div>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, idx) => (
          <Card key={`kpi-${idx}`} className="border-border/60 shadow-sm">
            <CardContent className="flex items-center gap-4 p-5">
              <Skeleton className="size-12 rounded-xl" />
              <div className="space-y-2">
                <Skeleton className="h-3 w-24" />
                <Skeleton className="h-7 w-12" />
              </div>
            </CardContent>
          </Card>
        ))}
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        {Array.from({ length: 2 }).map((_, idx) => (
          <Card key={`metric-${idx}`} className="border-border/60 shadow-sm">
            <CardContent className="flex items-center gap-4 p-5">
              <Skeleton className="size-12 rounded-xl" />
              <div className="space-y-2">
                <Skeleton className="h-3 w-36" />
                <Skeleton className="h-7 w-20" />
                <Skeleton className="h-3 w-40" />
              </div>
            </CardContent>
          </Card>
        ))}
      </div>

      <Card className="border-border/60 shadow-sm">
        <CardHeader className="space-y-2">
          <Skeleton className="h-5 w-56" />
          <Skeleton className="h-3 w-72" />
        </CardHeader>
        <CardContent>
          <Skeleton className="h-[320px] w-full rounded-xl" />
        </CardContent>
      </Card>

      <Card className="border-border/60 shadow-sm">
        <CardHeader className="space-y-2">
          <Skeleton className="h-5 w-44" />
          <Skeleton className="h-3 w-60" />
        </CardHeader>
        <CardContent className="space-y-3">
          {Array.from({ length: 4 }).map((_, idx) => (
            <Skeleton key={`row-${idx}`} className="h-11 w-full rounded-lg" />
          ))}
        </CardContent>
      </Card>
    </div>
  );
}

export default function DashboardPage() {
  const [windowDays, setWindowDays] = useState<7 | 30 | 90>(30);
  const meQuery = useQuery({
    queryKey: ["me"],
    queryFn: () => apiFetch<MeResponse>("/api/me"),
  });
  const isEnterprise =
    meQuery.data?.billing?.effective_plan === "enterprise" && isPaidActive(meQuery.data?.billing?.status);

  const query = useQuery({
    queryKey: ["requests-summary", windowDays],
    queryFn: () =>
      apiFetch<{ summary: Summary; overdue: OverdueRow[]; analytics: Analytics }>(
        `/api/requests/summary?days=${windowDays}`,
      ),
    refetchInterval: 10_000,
    refetchIntervalInBackground: true,
    refetchOnWindowFocus: true,
  });

  const summary = query.data?.summary;
  const overdue = query.data?.overdue ?? [];
  const analytics = query.data?.analytics;
  const isLoadingInitial = query.isLoading || meQuery.isLoading;

  return (
    <RequireAuth>
      <AdminShell>
        {isLoadingInitial ? <DashboardSkeleton /> : <div className="mx-auto max-w-6xl space-y-6">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h1 className="text-2xl font-bold tracking-tight text-foreground">Dashboard</h1>
              <p className="mt-1 text-sm text-muted-foreground">Overview of your workspace triage health.</p>
            </div>
            <div className="flex items-center gap-2">
              {isEnterprise ? (
                <>
                  <Button
                    size="sm"
                    variant="outline"
                    className="h-8 gap-1.5 px-3 text-xs"
                    onClick={() => {
                      window.open(`/api/backend/api/reports/requests.csv?days=${windowDays}`, "_blank", "noopener,noreferrer");
                    }}
                  >
                    <Download className="size-3.5" />
                    Export requests CSV
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    className="h-8 gap-1.5 px-3 text-xs"
                    onClick={() => {
                      window.open(`/api/backend/api/reports/requests.pdf?days=${windowDays}`, "_blank", "noopener,noreferrer");
                    }}
                  >
                    <FileText className="size-3.5" />
                    Export requests PDF
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    className="h-8 gap-1.5 px-3 text-xs"
                    onClick={() => {
                      window.open(`/api/backend/api/reports/analytics.csv?days=${windowDays}`, "_blank", "noopener,noreferrer");
                    }}
                  >
                    <Download className="size-3.5" />
                    Export analytics CSV
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    className="h-8 gap-1.5 px-3 text-xs"
                    onClick={() => {
                      window.open(`/api/backend/api/reports/analytics.pdf?days=${windowDays}`, "_blank", "noopener,noreferrer");
                    }}
                  >
                    <FileText className="size-3.5" />
                    Export analytics PDF
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

          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <Card className="border-border/60 shadow-sm">
              <CardContent className="flex items-center gap-4 p-5">
                <div className="flex size-12 items-center justify-center rounded-xl bg-primary/10">
                  <Inbox className="size-5 text-primary" />
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">New requests</p>
                  <p className="text-2xl font-bold tabular-nums">{summary?.new_count ?? 0}</p>
                </div>
              </CardContent>
            </Card>
            <Card className="border-border/60 shadow-sm">
              <CardContent className="flex items-center gap-4 p-5">
                <div className="flex size-12 items-center justify-center rounded-xl bg-warning/10">
                  <UserX className="size-5 text-warning-foreground" />
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">Unassigned</p>
                  <p className="text-2xl font-bold tabular-nums">{summary?.unassigned_count ?? 0}</p>
                </div>
              </CardContent>
            </Card>
            <Card className="border-border/60 shadow-sm">
              <CardContent className="flex items-center gap-4 p-5">
                <div className="flex size-12 items-center justify-center rounded-xl bg-success/10">
                  <Briefcase className="size-5 text-success" />
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">Assigned</p>
                  <p className="text-2xl font-bold tabular-nums">{summary?.assigned_count ?? 0}</p>
                </div>
              </CardContent>
            </Card>
            <Card className="border-border/60 shadow-sm">
              <CardContent className="flex items-center gap-4 p-5">
                <div className="flex size-12 items-center justify-center rounded-xl bg-destructive/10">
                  <AlertTriangle className="size-5 text-destructive" />
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">Overdue</p>
                  <p className="text-2xl font-bold tabular-nums">{summary?.overdue_count ?? 0}</p>
                </div>
              </CardContent>
            </Card>
          </div>

          <div className="grid gap-4 lg:grid-cols-2">
            <Card className="border-border/60 shadow-sm">
              <CardContent className="flex items-center gap-4 p-5">
                <div className="flex size-12 items-center justify-center rounded-xl bg-primary/10">
                  <Clock3 className="size-5 text-primary" />
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">Average time to ack</p>
                  <p className="text-2xl font-bold tabular-nums">{analytics ? formatAvgMinutes(analytics.ack) : "—"}</p>
                  <p className="text-xs text-muted-foreground">
                    {analytics?.ack.sample_size ?? 0} samples in {analytics?.window_days ?? windowDays}d
                  </p>
                </div>
              </CardContent>
            </Card>

            <Card className="border-border/60 shadow-sm">
              <CardContent className="flex items-center gap-4 p-5">
                <div className="flex size-12 items-center justify-center rounded-xl bg-success/10">
                  <CheckCircle2 className="size-5 text-success" />
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">Average time to resolve</p>
                  <p className="text-2xl font-bold tabular-nums">
                    {analytics ? formatAvgMinutes(analytics.resolve) : "—"}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {analytics?.resolve.sample_size ?? 0} samples in {analytics?.window_days ?? windowDays}d
                  </p>
                </div>
              </CardContent>
            </Card>
          </div>

          <Card className="border-border/60 shadow-sm">
            <CardHeader className="pb-2">
              <CardTitle className="text-base font-semibold">SLA trend: Ack vs Resolve</CardTitle>
              <CardDescription className="text-xs">
                Daily averages for selected window ({analytics?.window_days ?? windowDays} days).
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="flex items-center gap-4 text-xs text-muted-foreground">
                <div className="flex items-center gap-2">
                  <span className="inline-block h-2 w-2 rounded-full" style={{ backgroundColor: "var(--color-chart-1)" }} />
                  Ack avg
                </div>
                <div className="flex items-center gap-2">
                  <span className="inline-block h-2 w-2 rounded-full" style={{ backgroundColor: "var(--color-chart-2)" }} />
                  Resolve avg
                </div>
              </div>
              <TrendChart trend={analytics?.trend ?? []} />
            </CardContent>
          </Card>

          <Card className="border-border/60 shadow-sm">
            <CardHeader className="pb-3">
              <CardTitle className="text-base font-semibold">Top overdue requests</CardTitle>
              <CardDescription className="text-xs">Requests that have exceeded SLA windows.</CardDescription>
            </CardHeader>
            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow className="border-border/40 hover:bg-transparent">
                    <TableHead className="pl-6 text-xs">Status</TableHead>
                    <TableHead className="text-xs">Priority</TableHead>
                    <TableHead className="text-xs">Title</TableHead>
                    <TableHead className="text-xs">Slack thread</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {overdue.map((row) => (
                    <TableRow key={row.id} className="border-border/40">
                      <TableCell className="pl-6">
                        <Badge variant="secondary" className="rounded-md px-2 py-0.5 text-[10px] font-semibold">
                          {row.status}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <Badge className="rounded-md bg-destructive/10 px-2 py-0.5 text-[10px] font-semibold text-destructive hover:bg-destructive/10">
                          {row.priority}
                        </Badge>
                      </TableCell>
                      <TableCell className="max-w-sm truncate text-xs text-muted-foreground">
                        {row.title || "(no title)"}
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {row.thread_url ? (
                          <a
                            href={row.thread_url}
                            target="_blank"
                            rel="noreferrer"
                            className="underline underline-offset-2 hover:text-foreground"
                          >
                            Open Slack thread
                          </a>
                        ) : (
                          `${row.channel_id} / ${row.thread_ts}`
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                  {overdue.length === 0 && (
                    <TableRow>
                      <TableCell colSpan={4} className="py-10 text-center text-sm text-muted-foreground">
                        No overdue requests.
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </CardContent>
          </Card>

        </div>}
      </AdminShell>
    </RequireAuth>
  );
}
