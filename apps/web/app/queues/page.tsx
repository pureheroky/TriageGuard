"use client";

import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";
import { AdminShell } from "@/components/admin-shell";
import { RequireAuth } from "@/app/components/require-auth";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { toastError, toastSuccess } from "@/lib/stores/toast-store";

type SlackUser = {
  id: string;
  display_name?: string;
  real_name?: string;
  email?: string;
};

type QueueMember = {
  user_id: string;
  role: string;
};

type QueuePolicy = {
  ack_sla_minutes: number;
  assign_sla_minutes: number;
  stale_hours: number;
  digest_time: string;
  digest_weekday?: number | null;
  assign_starts_from: string;
  stale_starts_from: string;
  timezone: string;
  business_hours_enabled: boolean;
  business_hours_start?: string;
  business_hours_end?: string;
  business_days_mask: number;
};

type QueueItem = {
  queue: {
    id: string;
    name: string;
    description?: string;
    default_priority: string;
    default_request_type: string;
    escalation_channel_id?: string;
    is_default: boolean;
  };
  policy: QueuePolicy;
  members: QueueMember[];
};

type QueueDraft = {
  id?: string;
  name: string;
  description: string;
  default_priority: string;
  default_request_type: string;
  escalation_channel_id: string;
  is_default: boolean;
  ack_sla_minutes: string;
  assign_sla_minutes: string;
  stale_hours: string;
  digest_time: string;
  assign_starts_from: string;
  stale_starts_from: string;
  timezone: string;
  business_hours_enabled: boolean;
  business_hours_start: string;
  business_hours_end: string;
  business_days_mask: string;
  triager_ids: string;
  manager_ids: string;
};

function toDraft(item: QueueItem): QueueDraft {
  const triagers = item.members.filter((member) => member.role === "triager").map((member) => member.user_id).join(", ");
  const managers = item.members.filter((member) => member.role === "manager").map((member) => member.user_id).join(", ");
  return {
    id: item.queue.id,
    name: item.queue.name,
    description: item.queue.description || "",
    default_priority: item.queue.default_priority,
    default_request_type: item.queue.default_request_type,
    escalation_channel_id: item.queue.escalation_channel_id || "",
    is_default: item.queue.is_default,
    ack_sla_minutes: String(item.policy.ack_sla_minutes),
    assign_sla_minutes: String(item.policy.assign_sla_minutes),
    stale_hours: String(item.policy.stale_hours),
    digest_time: item.policy.digest_time || "09:00:00",
    assign_starts_from: item.policy.assign_starts_from || "created_at",
    stale_starts_from: item.policy.stale_starts_from || "last_human_activity_at",
    timezone: item.policy.timezone || "UTC",
    business_hours_enabled: item.policy.business_hours_enabled,
    business_hours_start: item.policy.business_hours_start || "09:00:00",
    business_hours_end: item.policy.business_hours_end || "18:00:00",
    business_days_mask: String(item.policy.business_days_mask || 62),
    triager_ids: triagers,
    manager_ids: managers,
  };
}

function emptyDraft(): QueueDraft {
  return {
    name: "",
    description: "",
    default_priority: "P2",
    default_request_type: "other",
    escalation_channel_id: "",
    is_default: false,
    ack_sla_minutes: "15",
    assign_sla_minutes: "30",
    stale_hours: "24",
    digest_time: "09:00:00",
    assign_starts_from: "created_at",
    stale_starts_from: "last_human_activity_at",
    timezone: "UTC",
    business_hours_enabled: false,
    business_hours_start: "09:00:00",
    business_hours_end: "18:00:00",
    business_days_mask: "62",
    triager_ids: "",
    manager_ids: "",
  };
}

function userLabel(user: SlackUser): string {
  return user.display_name || user.real_name || user.email || user.id;
}

export default function QueuesPage() {
  const queryClient = useQueryClient();
  const [drafts, setDrafts] = useState<QueueDraft[]>([]);

  const queuesQuery = useQuery({
    queryKey: ["queues"],
    queryFn: () => apiFetch<{ queues: QueueItem[] }>("/api/queues"),
  });

  const usersQuery = useQuery({
    queryKey: ["slack-users"],
    queryFn: () => apiFetch<{ users: SlackUser[] }>("/api/slack/users"),
  });

  useEffect(() => {
    if (queuesQuery.data) {
      setDrafts(queuesQuery.data.queues.map(toDraft));
    }
  }, [queuesQuery.data]);

  const usersByID = useMemo(() => {
    const out = new Map<string, SlackUser>();
    for (const user of usersQuery.data?.users ?? []) {
      out.set(user.id, user);
    }
    return out;
  }, [usersQuery.data]);

  const saveMutation = useMutation({
    mutationFn: async () =>
      apiFetch("/api/queues", {
        method: "PUT",
        body: JSON.stringify({
          queues: drafts.map((draft) => ({
            id: draft.id,
            name: draft.name,
            description: draft.description,
            default_priority: draft.default_priority,
            default_request_type: draft.default_request_type,
            escalation_channel_id: draft.escalation_channel_id,
            is_default: draft.is_default,
            policy: {
              ack_sla_minutes: Number(draft.ack_sla_minutes),
              assign_sla_minutes: Number(draft.assign_sla_minutes),
              stale_hours: Number(draft.stale_hours),
              digest_time: draft.digest_time,
              assign_starts_from: draft.assign_starts_from,
              stale_starts_from: draft.stale_starts_from,
              timezone: draft.timezone,
              business_hours_enabled: draft.business_hours_enabled,
              business_hours_start: draft.business_hours_enabled ? draft.business_hours_start : "",
              business_hours_end: draft.business_hours_enabled ? draft.business_hours_end : "",
              business_days_mask: Number(draft.business_days_mask),
            },
            members: [
              ...draft.triager_ids
                .split(",")
                .map((value) => value.trim())
                .filter(Boolean)
                .map((userID) => ({ user_id: userID, role: "triager" })),
              ...draft.manager_ids
                .split(",")
                .map((value) => value.trim())
                .filter(Boolean)
                .map((userID) => ({ user_id: userID, role: "manager" })),
            ],
          })),
        }),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["queues"] });
      toastSuccess("Queues saved", "Queue defaults, policies and ownership were updated.");
    },
    onError: (error) => {
      toastError("Save failed", error instanceof Error ? error.message : "Could not save queues.");
    },
  });

  function updateDraft(index: number, patch: Partial<QueueDraft>) {
    setDrafts((current) => current.map((draft, draftIndex) => (draftIndex === index ? { ...draft, ...patch } : draft)));
  }

  return (
    <RequireAuth>
      <AdminShell>
        <div className="mx-auto max-w-6xl space-y-6">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h1 className="text-2xl font-bold tracking-tight text-foreground">Queues</h1>
              <p className="mt-1 text-sm text-muted-foreground">Manage queue defaults, SLA policies, business hours and queue ownership.</p>
            </div>
            <div className="flex items-center gap-2">
              <Button variant="outline" onClick={() => setDrafts((current) => [...current, emptyDraft()])}>
                Add queue
              </Button>
              <Button onClick={() => saveMutation.mutate()} disabled={saveMutation.isPending}>
                {saveMutation.isPending ? "Saving..." : "Save queues"}
              </Button>
            </div>
          </div>

          <div className="space-y-4">
            {drafts.map((draft, index) => (
              <Card key={draft.id || `new-${index}`} className="border-border/60 shadow-sm">
                <CardHeader>
                  <CardTitle className="text-base font-semibold">
                    {draft.name || "New queue"}
                    {draft.is_default ? <span className="ml-2 text-xs text-muted-foreground">Default workspace queue</span> : null}
                  </CardTitle>
                  <CardDescription className="text-xs">Queue-level routing defaults, SLA rules and ownership.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-5">
                  <div className="grid gap-5 sm:grid-cols-2">
                    <div className="space-y-2">
                      <Label>Name</Label>
                      <Input value={draft.name} onChange={(e) => updateDraft(index, { name: e.target.value })} />
                    </div>
                    <div className="space-y-2">
                      <Label>Description</Label>
                      <Input value={draft.description} onChange={(e) => updateDraft(index, { description: e.target.value })} />
                    </div>
                  </div>

                  <div className="grid gap-5 sm:grid-cols-4">
                    <div className="space-y-2">
                      <Label>Default priority</Label>
                      <Input value={draft.default_priority} onChange={(e) => updateDraft(index, { default_priority: e.target.value })} />
                    </div>
                    <div className="space-y-2">
                      <Label>Default request type</Label>
                      <Input value={draft.default_request_type} onChange={(e) => updateDraft(index, { default_request_type: e.target.value })} />
                    </div>
                    <div className="space-y-2">
                      <Label>Escalation channel ID</Label>
                      <Input value={draft.escalation_channel_id} onChange={(e) => updateDraft(index, { escalation_channel_id: e.target.value })} />
                    </div>
                    <div className="flex items-center justify-between rounded-lg border border-border/60 px-3 py-2">
                      <div>
                        <p className="text-sm font-medium text-foreground">Default queue</p>
                        <p className="text-xs text-muted-foreground">Fallback for unrouted requests</p>
                      </div>
                      <Switch checked={draft.is_default} onCheckedChange={(checked) => updateDraft(index, { is_default: checked })} />
                    </div>
                  </div>

                  <div className="grid gap-5 sm:grid-cols-4">
                    <div className="space-y-2">
                      <Label>Ack SLA (minutes)</Label>
                      <Input value={draft.ack_sla_minutes} onChange={(e) => updateDraft(index, { ack_sla_minutes: e.target.value })} />
                    </div>
                    <div className="space-y-2">
                      <Label>Assign SLA (minutes)</Label>
                      <Input value={draft.assign_sla_minutes} onChange={(e) => updateDraft(index, { assign_sla_minutes: e.target.value })} />
                    </div>
                    <div className="space-y-2">
                      <Label>Stale hours</Label>
                      <Input value={draft.stale_hours} onChange={(e) => updateDraft(index, { stale_hours: e.target.value })} />
                    </div>
                    <div className="space-y-2">
                      <Label>Digest time</Label>
                      <Input value={draft.digest_time} onChange={(e) => updateDraft(index, { digest_time: e.target.value })} />
                    </div>
                  </div>

                  <div className="grid gap-5 sm:grid-cols-4">
                    <div className="space-y-2">
                      <Label>Assign starts from</Label>
                      <Input value={draft.assign_starts_from} onChange={(e) => updateDraft(index, { assign_starts_from: e.target.value })} />
                    </div>
                    <div className="space-y-2">
                      <Label>Stale starts from</Label>
                      <Input value={draft.stale_starts_from} onChange={(e) => updateDraft(index, { stale_starts_from: e.target.value })} />
                    </div>
                    <div className="space-y-2">
                      <Label>Timezone</Label>
                      <Input value={draft.timezone} onChange={(e) => updateDraft(index, { timezone: e.target.value })} />
                    </div>
                    <div className="space-y-2">
                      <Label>Business days mask</Label>
                      <Input value={draft.business_days_mask} onChange={(e) => updateDraft(index, { business_days_mask: e.target.value })} />
                    </div>
                  </div>

                  <div className="space-y-3 rounded-lg border border-border/60 p-4">
                    <div className="flex items-center justify-between">
                      <div>
                        <p className="text-sm font-medium text-foreground">Business hours</p>
                        <p className="text-xs text-muted-foreground">If enabled, clocks use queue business windows.</p>
                      </div>
                      <Switch checked={draft.business_hours_enabled} onCheckedChange={(checked) => updateDraft(index, { business_hours_enabled: checked })} />
                    </div>
                    <div className="grid gap-5 sm:grid-cols-2">
                      <div className="space-y-2">
                        <Label>Start</Label>
                        <Input value={draft.business_hours_start} onChange={(e) => updateDraft(index, { business_hours_start: e.target.value })} />
                      </div>
                      <div className="space-y-2">
                        <Label>End</Label>
                        <Input value={draft.business_hours_end} onChange={(e) => updateDraft(index, { business_hours_end: e.target.value })} />
                      </div>
                    </div>
                  </div>

                  <div className="grid gap-5 sm:grid-cols-2">
                    <div className="space-y-2">
                      <Label>Triager Slack user IDs</Label>
                      <Input value={draft.triager_ids} onChange={(e) => updateDraft(index, { triager_ids: e.target.value })} />
                    </div>
                    <div className="space-y-2">
                      <Label>Manager Slack user IDs</Label>
                      <Input value={draft.manager_ids} onChange={(e) => updateDraft(index, { manager_ids: e.target.value })} />
                    </div>
                  </div>

                  <div className="rounded-lg border border-border/60 bg-muted/20 p-4">
                    <p className="text-sm font-medium text-foreground">Slack directory</p>
                    <p className="mt-1 text-xs text-muted-foreground">Use these Slack IDs in triager/manager fields.</p>
                    <div className="mt-3 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
                      {(usersQuery.data?.users ?? []).slice(0, 24).map((user) => (
                        <div key={user.id} className="rounded-md border border-border/50 px-3 py-2">
                          <p className="text-sm font-medium text-foreground">{userLabel(user)}</p>
                          <p className="text-xs text-muted-foreground">{user.id}</p>
                        </div>
                      ))}
                    </div>
                    {(draft.triager_ids || draft.manager_ids) && (
                      <div className="mt-3 text-xs text-muted-foreground">
                        Selected:
                        {" "}
                        {[...draft.triager_ids.split(","), ...draft.manager_ids.split(",")]
                          .map((value) => value.trim())
                          .filter(Boolean)
                          .map((userID) => userLabel(usersByID.get(userID) || { id: userID }))
                          .join(", ")}
                      </div>
                    )}
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        </div>
      </AdminShell>
    </RequireAuth>
  );
}
