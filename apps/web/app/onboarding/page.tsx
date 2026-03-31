"use client";

import { Suspense, useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSearchParams } from "next/navigation";
import { apiFetch } from "@/lib/api";
import { AdminShell } from "@/components/admin-shell";
import { RequireAuth } from "@/app/components/require-auth";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { toastError, toastInfo, toastSuccess } from "@/lib/stores/toast-store";
import { toFriendlyErrorMessage } from "@/lib/error-messages";

type Channel = {
  channel_id: string;
  channel_name: string;
  enabled: boolean;
  default_queue_id?: string | null;
};

type QueueItem = {
  queue: {
    id: string;
    name: string;
    default_request_type: string;
  };
  members: Array<{
    user_id: string;
    role: string;
  }>;
};

type Policy = {
  ack_sla_minutes: number;
  assign_sla_minutes: number;
  stale_hours: number;
  daily_digest_time: string;
  timezone: string;
  escalation_channel_id?: string;
  linear_team_id?: string;
};

type UserOption = {
  id: string;
  real_name?: string;
  profile?: {
    display_name?: string;
    real_name?: string;
  };
};

type Summary = {
  summary: {
    open_count: number;
    unacked_count: number;
    unassigned_count: number;
    at_risk_count: number;
  };
};

type MeResponse = {
  integrations: {
    slack_connected: boolean;
    linear_connected: boolean;
  };
  policy: Policy;
  billing?: {
    effective_plan?: string;
    status?: string;
    entitlements?: {
      triager_limit?: number;
    };
  };
};

function parseIDs(raw: string): string[] {
  return raw
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

function OnboardingPageContent() {
  const queryClient = useQueryClient();
  const searchParams = useSearchParams();
  const connectedParam = (searchParams.get("connected") || "").toLowerCase();
  const [selectedChannels, setSelectedChannels] = useState<string[]>([]);
  const [queueName, setQueueName] = useState("Platform requests");
  const [requestType, setRequestType] = useState("other");
  const [triagerIDs, setTriagerIDs] = useState("UDEMO_TRIAGER");
  const [managerIDs, setManagerIDs] = useState("UDEMO_MANAGER");
  const [policyForm, setPolicyForm] = useState<Policy | null>(null);
  const [queueID, setQueueID] = useState<string>("");

  const meQuery = useQuery({
    queryKey: ["me"],
    queryFn: () => apiFetch<MeResponse>("/api/me"),
  });
  const channelsQuery = useQuery({
    queryKey: ["channels"],
    queryFn: () => apiFetch<{ channels: Channel[] }>("/api/channels"),
    enabled: meQuery.data?.integrations.slack_connected,
  });
  const queuesQuery = useQuery({
    queryKey: ["queues"],
    queryFn: () => apiFetch<{ queues: QueueItem[] }>("/api/queues"),
    enabled: meQuery.data?.integrations.slack_connected,
  });
  const usersQuery = useQuery({
    queryKey: ["slack-users"],
    queryFn: () => apiFetch<{ users: UserOption[] }>("/api/slack/users"),
    enabled: meQuery.data?.integrations.slack_connected,
  });
  const summaryQuery = useQuery({
    queryKey: ["requests-summary"],
    queryFn: () => apiFetch<Summary>("/api/requests/summary"),
    enabled: meQuery.data?.integrations.slack_connected,
  });

  useEffect(() => {
    if (meQuery.data?.policy) {
      setPolicyForm(meQuery.data.policy);
    }
  }, [meQuery.data]);

  useEffect(() => {
    const channels = channelsQuery.data?.channels ?? [];
    if (channels.length > 0 && selectedChannels.length === 0) {
      setSelectedChannels(channels.filter((channel) => channel.enabled).map((channel) => channel.channel_id));
    }
  }, [channelsQuery.data, selectedChannels.length]);

  useEffect(() => {
    const firstQueue = queuesQuery.data?.queues?.[0];
    if (!firstQueue) return;
    setQueueID(firstQueue.queue.id);
    setQueueName(firstQueue.queue.name);
    setRequestType(firstQueue.queue.default_request_type);
    setTriagerIDs(firstQueue.members.filter((member) => member.role === "triager").map((member) => member.user_id).join(", "));
    setManagerIDs(firstQueue.members.filter((member) => member.role === "manager").map((member) => member.user_id).join(", "));
  }, [queuesQuery.data]);

  useEffect(() => {
    if (connectedParam !== "slack" && connectedParam !== "linear") {
      return;
    }
    const storageKey = `tg.toast.integration.connected.${connectedParam}`;
    try {
      if (window.sessionStorage.getItem(storageKey) === "1") {
        return;
      }
      window.sessionStorage.setItem(storageKey, "1");
    } catch {
      // ignore storage issues
    }
    toastSuccess(
      connectedParam === "slack" ? "Slack connected" : "Linear connected",
      connectedParam === "slack" ? "Continue queue setup and turn on your first SLA preset." : "Linear is now available for tracker linking."
    );
  }, [connectedParam]);

  const syncChannelsMutation = useMutation({
    mutationFn: () => apiFetch<{ channels: Channel[] }>("/api/channels/sync", { method: "POST" }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["channels"] });
      toastSuccess("Channels synced", "Latest Slack channels loaded.");
    },
    onError: (error) => toastError("Channel sync failed", toFriendlyErrorMessage(error)),
  });

  const sampleSeedMutation = useMutation({
    mutationFn: () => apiFetch<{ ok: boolean }>("/api/onboarding/sample-data", { method: "POST" }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["requests-summary"] });
      queryClient.invalidateQueries({ queryKey: ["queue-dashboard"] });
      queryClient.invalidateQueries({ queryKey: ["queues"] });
      toastSuccess("Sample workspace loaded", "Demo queues, requests, and SLA risk signals are ready.");
    },
    onError: (error) => toastError("Sample data failed", toFriendlyErrorMessage(error)),
  });

  const sampleResetMutation = useMutation({
    mutationFn: () => apiFetch<{ ok: boolean }>("/api/onboarding/sample-data", { method: "DELETE" }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["requests-summary"] });
      queryClient.invalidateQueries({ queryKey: ["queue-dashboard"] });
      queryClient.invalidateQueries({ queryKey: ["queues"] });
      toastInfo("Sample data reset", "Seeded demo requests and queues were removed.");
    },
    onError: (error) => toastError("Reset failed", toFriendlyErrorMessage(error)),
  });

  const finishSetupMutation = useMutation({
    mutationFn: async () => {
      if (!policyForm) {
        throw new Error("Policy settings are not ready yet.");
      }

      const queuePayload = {
        queues: [
          {
            id: queueID,
            name: queueName.trim() || "Platform requests",
            description: "Primary queue configured during onboarding",
            default_priority: "P2",
            default_request_type: requestType,
            escalation_channel_id: "",
            is_default: true,
            policy: {
              ack_sla_minutes: policyForm.ack_sla_minutes,
              assign_sla_minutes: policyForm.assign_sla_minutes,
              stale_hours: policyForm.stale_hours,
              digest_time: policyForm.daily_digest_time?.slice(0, 8) || "09:00:00",
              digest_weekday: 1,
              assign_starts_from: "created_at",
              stale_starts_from: "last_human_activity_at",
              timezone: policyForm.timezone,
              business_hours_enabled: false,
              business_hours_start: "",
              business_hours_end: "",
              business_days_mask: 62,
            },
            members: [
              ...parseIDs(triagerIDs).map((userID) => ({ user_id: userID, role: "triager" })),
              ...parseIDs(managerIDs).map((userID) => ({ user_id: userID, role: "manager" })),
            ],
          },
        ],
      };
      const queueResponse = await apiFetch<{ queues: QueueItem[] }>("/api/queues", {
        method: "PUT",
        body: JSON.stringify(queuePayload),
      });
      const savedQueue = queueResponse.queues[0]?.queue;
      const savedQueueID = savedQueue?.id || queueID;
      if (!savedQueueID) {
        throw new Error("Queue could not be saved.");
      }
      setQueueID(savedQueueID);

      const channels = channelsQuery.data?.channels ?? [];
      await apiFetch<{ channels: Channel[] }>("/api/channels", {
        method: "PUT",
        body: JSON.stringify({
          channels: channels.map((channel) => ({
            channel_id: channel.channel_id,
            enabled: selectedChannels.includes(channel.channel_id),
            default_queue_id: selectedChannels.includes(channel.channel_id) ? savedQueueID : "",
          })),
        }),
      });

      await apiFetch<Policy>("/api/policies", {
        method: "PUT",
        body: JSON.stringify({
          ack_sla_minutes: policyForm.ack_sla_minutes,
          assign_sla_minutes: policyForm.assign_sla_minutes,
          stale_hours: policyForm.stale_hours,
          daily_digest_time: policyForm.daily_digest_time?.slice(0, 8) || "09:00:00",
          timezone: policyForm.timezone,
          escalation_channel_id: policyForm.escalation_channel_id || "",
          linear_team_id: policyForm.linear_team_id || "",
        }),
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["me"] });
      queryClient.invalidateQueries({ queryKey: ["queues"] });
      queryClient.invalidateQueries({ queryKey: ["channels"] });
      queryClient.invalidateQueries({ queryKey: ["requests-summary"] });
      queryClient.invalidateQueries({ queryKey: ["queue-dashboard"] });
      toastSuccess("Onboarding complete", "Your first queue, triagers, channels, and SLA preset are now live.");
    },
    onError: (error) => toastError("Setup failed", toFriendlyErrorMessage(error)),
  });

  const availableUsers = useMemo(() => usersQuery.data?.users ?? [], [usersQuery.data]);
  const triagerLimit = meQuery.data?.billing?.entitlements?.triager_limit;

  return (
    <RequireAuth>
      <AdminShell>
        <div className="mx-auto max-w-7xl space-y-6">
          <div className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
            <div>
              <h1 className="text-2xl font-bold tracking-tight text-foreground">10-Minute Onboarding</h1>
              <p className="mt-1 text-sm text-muted-foreground">
                Connect Slack, create the first queue, assign triagers, enable an SLA preset, and immediately see what is open,
                unacked, unassigned, or at risk.
              </p>
            </div>
            <div className="flex items-center gap-2">
              <Button variant="outline" onClick={() => sampleSeedMutation.mutate()} disabled={sampleSeedMutation.isPending}>
                {sampleSeedMutation.isPending ? "Loading..." : "Load sample workspace"}
              </Button>
              <Button variant="outline" onClick={() => sampleResetMutation.mutate()} disabled={sampleResetMutation.isPending}>
                {sampleResetMutation.isPending ? "Resetting..." : "Reset sample data"}
              </Button>
            </div>
          </div>

          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <MetricCard title="Open" value={summaryQuery.data?.summary.open_count ?? 0} />
            <MetricCard title="Unacked" value={summaryQuery.data?.summary.unacked_count ?? 0} />
            <MetricCard title="Unassigned" value={summaryQuery.data?.summary.unassigned_count ?? 0} />
            <MetricCard title="At risk" value={summaryQuery.data?.summary.at_risk_count ?? 0} />
          </div>

          <div className="grid gap-6 xl:grid-cols-2">
            <StepCard
              step="1"
              title="Connect Slack"
              description={meQuery.data?.integrations.slack_connected ? "Slack workspace connected." : "Authorize Slack first so TriageGuard can sync channels and render queue-control cards."}
            >
              {meQuery.data?.integrations.slack_connected ? (
                <Button disabled>Slack connected</Button>
              ) : (
                <a href="/api/backend/api/slack/install">
                  <Button>Connect Slack</Button>
                </a>
              )}
            </StepCard>

            <StepCard
              step="2"
              title="Choose channels"
              description="Pick the channels that should feed the first queue. New root messages in enabled channels become queue-controlled requests."
            >
              <div className="space-y-3">
                <Button variant="outline" onClick={() => syncChannelsMutation.mutate()} disabled={syncChannelsMutation.isPending || !meQuery.data?.integrations.slack_connected}>
                  {syncChannelsMutation.isPending ? "Syncing..." : "Sync channels"}
                </Button>
                <div className="space-y-2">
                  {(channelsQuery.data?.channels ?? []).map((channel) => (
                    <label key={channel.channel_id} className="flex items-center gap-3 rounded-lg border border-border/50 px-3 py-2 text-sm">
                      <input
                        type="checkbox"
                        className="size-4 rounded border border-border"
                        checked={selectedChannels.includes(channel.channel_id)}
                        onChange={(e) => {
                          setSelectedChannels((current) =>
                            e.target.checked ? [...current, channel.channel_id] : current.filter((item) => item !== channel.channel_id),
                          );
                        }}
                      />
                      <span>{channel.channel_name}</span>
                    </label>
                  ))}
                  {(channelsQuery.data?.channels ?? []).length === 0 ? (
                    <p className="text-sm text-muted-foreground">No channels synced yet.</p>
                  ) : null}
                </div>
              </div>
            </StepCard>

            <StepCard
              step="3"
              title="Create the first queue"
              description="This queue becomes the default home for enabled channels and the main buyer-facing health surface."
            >
              <div className="space-y-3">
                <div className="space-y-2">
                  <Label>Queue name</Label>
                  <Input value={queueName} onChange={(e) => setQueueName(e.target.value)} />
                </div>
                <div className="space-y-2">
                  <Label>Default request type</Label>
                  <Select value={requestType} onValueChange={setRequestType}>
                    <SelectTrigger>
                      <SelectValue placeholder="Choose type" />
                    </SelectTrigger>
                    <SelectContent>
                      {["bug", "incident", "access", "infra", "question", "task", "other"].map((value) => (
                        <SelectItem key={value} value={value}>
                          {value}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </div>
            </StepCard>

            <StepCard
              step="4"
              title="Assign triagers and managers"
              description={`Set who owns the queue itself. ${triagerLimit ? `Team includes up to ${triagerLimit} triagers.` : ""}`}
            >
              <div className="space-y-3">
                <div className="space-y-2">
                  <Label>Triager Slack IDs</Label>
                  <Input value={triagerIDs} onChange={(e) => setTriagerIDs(e.target.value)} />
                </div>
                <div className="space-y-2">
                  <Label>Manager Slack IDs</Label>
                  <Input value={managerIDs} onChange={(e) => setManagerIDs(e.target.value)} />
                </div>
                <div className="rounded-lg border border-border/50 bg-muted/20 px-3 py-3 text-xs text-muted-foreground">
                  Available users:{" "}
                  {availableUsers
                    .slice(0, 8)
                    .map((user) => user.profile?.display_name || user.real_name || user.id)
                    .join(", ") || "Sync Slack users after connecting Slack."}
                </div>
              </div>
            </StepCard>

            <StepCard
              step="5"
              title="Enable the first SLA preset"
              description="Use the shared Team preset or your current workspace default. This becomes the first control loop for ack, assign, and stale risk."
            >
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label>Ack SLA minutes</Label>
                  <Input
                    type="number"
                    value={policyForm?.ack_sla_minutes ?? 15}
                    onChange={(e) => setPolicyForm((current) => ({ ...(current || defaultPolicy()), ack_sla_minutes: Number(e.target.value) }))}
                  />
                </div>
                <div className="space-y-2">
                  <Label>Assign SLA minutes</Label>
                  <Input
                    type="number"
                    value={policyForm?.assign_sla_minutes ?? 30}
                    onChange={(e) => setPolicyForm((current) => ({ ...(current || defaultPolicy()), assign_sla_minutes: Number(e.target.value) }))}
                  />
                </div>
                <div className="space-y-2">
                  <Label>Stale hours</Label>
                  <Input
                    type="number"
                    value={policyForm?.stale_hours ?? 24}
                    onChange={(e) => setPolicyForm((current) => ({ ...(current || defaultPolicy()), stale_hours: Number(e.target.value) }))}
                  />
                </div>
                <div className="space-y-2">
                  <Label>Timezone</Label>
                  <Input
                    value={policyForm?.timezone ?? "UTC"}
                    onChange={(e) => setPolicyForm((current) => ({ ...(current || defaultPolicy()), timezone: e.target.value }))}
                  />
                </div>
              </div>
            </StepCard>
          </div>

          <Card className="border-border/60 shadow-sm">
            <CardHeader>
              <CardTitle className="text-base font-semibold">Finish setup</CardTitle>
              <CardDescription className="text-xs">
                One save applies the queue, members, channel routing, and shared SLA preset.
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-wrap items-center gap-3">
              <Button onClick={() => finishSetupMutation.mutate()} disabled={finishSetupMutation.isPending || !meQuery.data?.integrations.slack_connected}>
                {finishSetupMutation.isPending ? "Saving..." : "Save onboarding setup"}
              </Button>
              <p className="text-xs text-muted-foreground">
                First value appears on the dashboard immediately after save or after loading sample data.
              </p>
            </CardContent>
          </Card>
        </div>
      </AdminShell>
    </RequireAuth>
  );
}

function defaultPolicy(): Policy {
  return {
    ack_sla_minutes: 15,
    assign_sla_minutes: 30,
    stale_hours: 24,
    daily_digest_time: "09:00:00",
    timezone: "UTC",
  };
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

function StepCard({
  step,
  title,
  description,
  children,
}: {
  step: string;
  title: string;
  description: string;
  children: React.ReactNode;
}) {
  return (
    <Card className="border-border/60 shadow-sm">
      <CardHeader>
        <CardTitle className="text-base font-semibold">
          {step}. {title}
        </CardTitle>
        <CardDescription className="text-xs">{description}</CardDescription>
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  );
}

export default function OnboardingPage() {
  return (
    <Suspense fallback={<div className="p-6 text-sm text-muted-foreground">Loading onboarding...</div>}>
      <OnboardingPageContent />
    </Suspense>
  );
}
