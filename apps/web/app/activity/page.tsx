"use client";

import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";
import { AdminShell } from "@/components/admin-shell";
import { RequireAuth } from "@/app/components/require-auth";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

type ActivityItem = {
  id: string;
  request_id: string;
  action_type: string;
  actor_type: string;
  actor_id?: string;
  created_at: string;
  payload: Record<string, unknown>;
};

export default function ActivityPage() {
  const query = useQuery({
    queryKey: ["activity"],
    queryFn: () => apiFetch<{ activity: ActivityItem[] }>("/api/activity?limit=100"),
  });

  const activity = query.data?.activity ?? [];

  return (
    <RequireAuth>
      <AdminShell>
        <div className="mx-auto max-w-6xl space-y-6">
          <div>
            <h1 className="text-2xl font-bold tracking-tight text-foreground">Activity Log</h1>
            <p className="mt-1 text-sm text-muted-foreground">Recent actions on requests.</p>
          </div>

          <Card className="border-border/60 shadow-sm">
            <CardHeader className="pb-3">
              <CardTitle className="text-base font-semibold">Recent activity</CardTitle>
              <CardDescription className="text-xs">Request event log across Slack users, schedulers, system flows and external providers.</CardDescription>
            </CardHeader>
            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow className="border-border/40 hover:bg-transparent">
                    <TableHead className="pl-6 text-xs">When</TableHead>
                    <TableHead className="text-xs">Event</TableHead>
                    <TableHead className="text-xs">Actor</TableHead>
                    <TableHead className="text-xs">Request</TableHead>
                    <TableHead className="pr-6 text-xs">Payload</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {activity.map((item) => (
                    <TableRow key={item.id} className="border-border/40">
                      <TableCell className="pl-6 text-xs text-muted-foreground">
                        {new Date(item.created_at).toLocaleString()}
                      </TableCell>
                      <TableCell className="text-xs font-semibold">{item.action_type}</TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {item.actor_type}
                        {item.actor_id ? `: ${item.actor_id}` : ""}
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">{item.request_id}</TableCell>
                      <TableCell className="pr-6 text-xs text-muted-foreground">{JSON.stringify(item.payload)}</TableCell>
                    </TableRow>
                  ))}
                  {activity.length === 0 && (
                    <TableRow>
                      <TableCell colSpan={5} className="py-10 text-center text-sm text-muted-foreground">
                        No activity yet.
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </CardContent>
          </Card>

          {query.isLoading && <p className="text-sm text-muted-foreground">Loading activity...</p>}
        </div>
      </AdminShell>
    </RequireAuth>
  );
}
