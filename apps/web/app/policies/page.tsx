"use client";

import { FormEvent, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";
import { AdminShell } from "@/components/admin-shell";
import { RequireAuth } from "@/app/components/require-auth";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { toastSuccess } from "@/lib/stores/toast-store";

type Policy = {
  ack_sla_minutes: number;
  assign_sla_minutes: number;
  stale_hours: number;
  daily_digest_time: string;
  timezone: string;
  escalation_channel_id?: string;
  linear_team_id?: string;
};

export default function PoliciesPage() {
  const queryClient = useQueryClient();
  const [form, setForm] = useState<Policy | null>(null);

  const query = useQuery({
    queryKey: ["policies"],
    queryFn: () => apiFetch<Policy>("/api/policies"),
  });

  useEffect(() => {
    if (query.data) {
      setForm(query.data);
    }
  }, [query.data]);

  const mutation = useMutation({
    mutationFn: (payload: Policy) =>
      apiFetch<Policy>("/api/policies", {
        method: "PUT",
        body: JSON.stringify({
          ack_sla_minutes: payload.ack_sla_minutes,
          assign_sla_minutes: payload.assign_sla_minutes,
          stale_hours: payload.stale_hours,
          daily_digest_time: payload.daily_digest_time?.slice(0, 8) || "09:00:00",
          timezone: payload.timezone,
          escalation_channel_id: payload.escalation_channel_id || "",
          linear_team_id: payload.linear_team_id || "",
        }),
      }),
    onSuccess: (updated) => {
      setForm(updated);
      queryClient.invalidateQueries({ queryKey: ["policies"] });
      toastSuccess("Policies saved", "SLA policy changes were applied.");
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!form) {
      return;
    }
    mutation.mutate(form);
  }

  return (
    <RequireAuth>
      <AdminShell>
        <div className="mx-auto max-w-4xl space-y-6">
          <div>
            <h1 className="text-2xl font-bold tracking-tight text-foreground">SLA Policies</h1>
            <p className="mt-1 text-sm text-muted-foreground">Configure acknowledgment, assignment and digest policies.</p>
          </div>

          <Card className="border-border/60 shadow-sm">
            <CardHeader>
              <CardTitle className="text-base font-semibold">Policy settings</CardTitle>
              <CardDescription className="text-xs">These values drive escalation logic in Slack.</CardDescription>
            </CardHeader>
            <CardContent>
              {form && (
                <form onSubmit={onSubmit} className="space-y-5">
                  <div className="grid gap-5 sm:grid-cols-2">
                    <div className="space-y-2">
                      <Label>Ack SLA (minutes)</Label>
                      <Input
                        type="number"
                        value={form.ack_sla_minutes}
                        onChange={(e) => setForm({ ...form, ack_sla_minutes: Number(e.target.value) })}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label>Assign SLA (minutes)</Label>
                      <Input
                        type="number"
                        value={form.assign_sla_minutes}
                        onChange={(e) => setForm({ ...form, assign_sla_minutes: Number(e.target.value) })}
                      />
                    </div>
                  </div>

                  <div className="grid gap-5 sm:grid-cols-2">
                    <div className="space-y-2">
                      <Label>Stale hours</Label>
                      <Input
                        type="number"
                        value={form.stale_hours}
                        onChange={(e) => setForm({ ...form, stale_hours: Number(e.target.value) })}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label>Daily digest time (HH:MM or HH:MM:SS)</Label>
                      <Input
                        value={form.daily_digest_time}
                        onChange={(e) => setForm({ ...form, daily_digest_time: e.target.value })}
                      />
                    </div>
                  </div>

                  <div className="grid gap-5 sm:grid-cols-2">
                    <div className="space-y-2">
                      <Label>Timezone</Label>
                      <Input value={form.timezone} onChange={(e) => setForm({ ...form, timezone: e.target.value })} />
                    </div>
                    <div className="space-y-2">
                      <Label>Escalation channel ID</Label>
                      <Input
                        value={form.escalation_channel_id || ""}
                        onChange={(e) => setForm({ ...form, escalation_channel_id: e.target.value })}
                      />
                    </div>
                  </div>

                  <Button type="submit" disabled={mutation.isPending}>
                    {mutation.isPending ? "Saving..." : "Save policies"}
                  </Button>
                </form>
              )}
            </CardContent>
          </Card>

          {query.isLoading && <p className="text-sm text-muted-foreground">Loading policy...</p>}
        </div>
      </AdminShell>
    </RequireAuth>
  );
}
