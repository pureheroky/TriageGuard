"use client";

import { Suspense, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter, useSearchParams } from "next/navigation";
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
  is_private?: boolean;
  enabled: boolean;
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

type MeResponse = {
  integrations: {
    slack_connected: boolean;
    linear_connected: boolean;
  };
  policy: Policy;
};

type LinearState = {
  installed: boolean;
  connected: boolean;
  requires_reconnect?: boolean;
  connection_error?: string;
  linear_team?: string;
  oauth_url: string;
};

function OnboardingPageContent() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const searchParams = useSearchParams();
  const [policyForm, setPolicyForm] = useState<Policy | null>(null);
  const connectedParam = (searchParams.get("connected") || "").toLowerCase();

  const meQuery = useQuery({
    queryKey: ["me"],
    queryFn: () => apiFetch<MeResponse>("/api/me"),
  });
  const channelsQuery = useQuery({
    queryKey: ["channels"],
    queryFn: () => apiFetch<{ channels: Channel[] }>("/api/channels"),
  });
  const linearQuery = useQuery({
    queryKey: ["linear"],
    queryFn: () => apiFetch<LinearState>("/api/linear"),
  });

  useEffect(() => {
    if (meQuery.data?.policy) {
      setPolicyForm(meQuery.data.policy);
    }
  }, [meQuery.data]);

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
      // Ignore storage issues in restricted environments.
    }

    if (connectedParam === "slack") {
      toastSuccess("Slack connected", "Workspace connected. Continue channel and SLA setup.");
      return;
    }
    toastSuccess("Linear connected", "Integration connected. Configure team ID for issue creation.");
  }, [connectedParam]);

  const syncChannelsMutation = useMutation({
    mutationFn: () => apiFetch<{ channels: Channel[] }>("/api/channels/sync", { method: "POST" }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["channels"] });
      toastSuccess("Channels synced", "Latest Slack channels were loaded.");
    },
  });

  const disconnectSlackMutation = useMutation({
    mutationFn: () => apiFetch<{ ok: boolean; disconnected: boolean }>("/api/slack/disconnect", { method: "POST" }),
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ["me"] });
      queryClient.invalidateQueries({ queryKey: ["channels"] });
      queryClient.invalidateQueries({ queryKey: ["requests-summary"] });
      if (data.disconnected) {
        toastSuccess("Slack disconnected", "Slack integration removed for this workspace.");
        return;
      }
      toastInfo("Already disconnected", "Slack was not connected for this workspace.");
    },
  });
  const disconnectLinearMutation = useMutation({
    mutationFn: () => apiFetch<{ ok: boolean; disconnected: boolean }>("/api/linear/disconnect", { method: "POST" }),
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ["me"] });
      queryClient.invalidateQueries({ queryKey: ["linear"] });
      queryClient.invalidateQueries({ queryKey: ["linear", "teams"] });
      queryClient.invalidateQueries({ queryKey: ["policies"] });
      if (data.disconnected) {
        toastSuccess("Linear disconnected", "Linear integration removed for this workspace.");
        return;
      }
      toastInfo("Already disconnected", "Linear was not connected for this workspace.");
    },
  });

  const savePoliciesMutation = useMutation({
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
      setPolicyForm(updated);
      queryClient.invalidateQueries({ queryKey: ["me"] });
      queryClient.invalidateQueries({ queryKey: ["policies"] });
      toastSuccess("Policies saved", "SLA policy settings were updated.");
    },
  });

  const deleteWorkspaceMutation = useMutation({
    mutationFn: () =>
      apiFetch<{ ok: boolean; deleted: boolean }>("/api/workspace/delete", {
        method: "POST",
        body: JSON.stringify({ confirm: "DELETE" }),
      }),
    onSuccess: async () => {
      queryClient.clear();
      toastSuccess("Workspace deleted", "All workspace data and integrations were removed.");
      try {
        await fetch("/api/auth/logout", { method: "POST" });
      } catch {
        // Ignore logout API failures; redirect still ensures session reset UX.
      }
      router.replace("/login?deleted=1");
    },
    onError: (error) => {
      toastError("Delete failed", toFriendlyErrorMessage(error));
    },
  });

  return (
    <RequireAuth>
      <AdminShell>
        <div className="mx-auto max-w-5xl space-y-6">
          <div>
            <h1 className="text-2xl font-bold tracking-tight text-foreground">Onboarding Wizard</h1>
            <p className="mt-1 text-sm text-muted-foreground">Configure Slack intake, SLA policies, and Linear issue routing.</p>
          </div>

          <div className="grid gap-6">
            <Card className="border-border/60 shadow-sm">
              <CardHeader>
                <CardTitle className="text-base font-semibold">1. Connect Slack</CardTitle>
                <CardDescription className="text-xs">
                  Status: {meQuery.data?.integrations.slack_connected ? "Connected" : "Not connected"}
                </CardDescription>
              </CardHeader>
              <CardContent className="flex flex-wrap items-center gap-2">
                {meQuery.data?.integrations.slack_connected ? (
                  <Button disabled>Connect Slack</Button>
                ) : (
                  <a href="/api/backend/api/slack/install">
                    <Button disabled={disconnectSlackMutation.isPending}>Connect Slack</Button>
                  </a>
                )}
                <Button
                  type="button"
                  variant="outline"
                  disabled={!meQuery.data?.integrations.slack_connected || disconnectSlackMutation.isPending}
                  onClick={() => {
                    if (!window.confirm("Disconnect Slack from this workspace? Monitoring channels and escalation channel settings will be cleared.")) {
                      return;
                    }
                    disconnectSlackMutation.mutate();
                  }}
                >
                  {disconnectSlackMutation.isPending ? "Disconnecting..." : "Disconnect Slack"}
                </Button>
              </CardContent>
            </Card>

            <Card className="border-border/60 shadow-sm">
              <CardHeader>
                <CardTitle className="text-base font-semibold">2. Select Channels</CardTitle>
                <CardDescription className="text-xs">Sync channels first, then enable them in Channels page.</CardDescription>
              </CardHeader>
              <CardContent>
                <Button variant="outline" onClick={() => syncChannelsMutation.mutate()} disabled={syncChannelsMutation.isPending}>
                  {syncChannelsMutation.isPending ? "Syncing..." : "Sync channels"}
                </Button>
              </CardContent>
            </Card>

            <Card className="border-border/60 shadow-sm">
              <CardHeader>
                <CardTitle className="text-base font-semibold">3-4. SLA + Escalation</CardTitle>
                <CardDescription className="text-xs">Set ack/assign/stale thresholds and escalation destination.</CardDescription>
              </CardHeader>
              <CardContent>
                {policyForm && (
                  <div className="space-y-4">
                    <div className="grid gap-4 sm:grid-cols-2">
                      <div className="space-y-2">
                        <Label>Ack SLA minutes</Label>
                        <Input
                          type="number"
                          value={policyForm.ack_sla_minutes}
                          onChange={(e) => setPolicyForm({ ...policyForm, ack_sla_minutes: Number(e.target.value) })}
                        />
                      </div>
                      <div className="space-y-2">
                        <Label>Assign SLA minutes</Label>
                        <Input
                          type="number"
                          value={policyForm.assign_sla_minutes}
                          onChange={(e) => setPolicyForm({ ...policyForm, assign_sla_minutes: Number(e.target.value) })}
                        />
                      </div>
                    </div>

                    <div className="grid gap-4 sm:grid-cols-2">
                      <div className="space-y-2">
                        <Label>Stale hours</Label>
                        <Input
                          type="number"
                          value={policyForm.stale_hours}
                          onChange={(e) => setPolicyForm({ ...policyForm, stale_hours: Number(e.target.value) })}
                        />
                      </div>
                      <div className="space-y-2">
                        <Label>Daily digest time</Label>
                        <Input
                          value={policyForm.daily_digest_time}
                          onChange={(e) => setPolicyForm({ ...policyForm, daily_digest_time: e.target.value })}
                        />
                      </div>
                    </div>

                    <div className="grid gap-4 sm:grid-cols-2">
                      <div className="space-y-2">
                        <Label>Timezone</Label>
                        <Input value={policyForm.timezone} onChange={(e) => setPolicyForm({ ...policyForm, timezone: e.target.value })} />
                      </div>
                      <div className="space-y-2">
                        <Label>Escalation channel</Label>
                        <Select
                          value={policyForm.escalation_channel_id || "__none__"}
                          onValueChange={(value) =>
                            setPolicyForm({
                              ...policyForm,
                              escalation_channel_id: value === "__none__" ? undefined : value,
                            })
                          }
                        >
                          <SelectTrigger className="w-full">
                            <SelectValue placeholder="No escalation channel" />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="__none__">No escalation channel</SelectItem>
                            {(channelsQuery.data?.channels || []).map((channel) => (
                              <SelectItem key={channel.channel_id} value={channel.channel_id}>
                                #{channel.channel_name} {channel.is_private ? "[private]" : "[public]"} ({channel.channel_id})
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </div>
                    </div>

                    <Button onClick={() => savePoliciesMutation.mutate(policyForm)} disabled={savePoliciesMutation.isPending}>
                      {savePoliciesMutation.isPending ? "Saving..." : "Save policy"}
                    </Button>
                  </div>
                )}
              </CardContent>
            </Card>

            <Card className="border-border/60 shadow-sm">
              <CardHeader>
                <CardTitle className="text-base font-semibold">5. Connect Linear (optional)</CardTitle>
                <CardDescription className="text-xs">
                  Status:{" "}
                  {linearQuery.data?.connected
                    ? "Connected"
                    : linearQuery.data?.requires_reconnect
                      ? "Reconnect required"
                      : "Not connected"}
                </CardDescription>
              </CardHeader>
              <CardContent className="flex flex-wrap items-center gap-2">
                {linearQuery.data?.connected ? (
                  <Button variant="outline" disabled>
                    Connected
                  </Button>
                ) : (
                  <a href="/api/backend/api/linear/install">
                    <Button variant="outline" disabled={disconnectLinearMutation.isPending}>
                      {linearQuery.data?.requires_reconnect ? "Reconnect Linear" : "Connect Linear"}
                    </Button>
                  </a>
                )}
                <Button
                  type="button"
                  variant="outline"
                  disabled={!linearQuery.data?.installed || disconnectLinearMutation.isPending}
                  onClick={() => {
                    if (!window.confirm("Disconnect Linear from this workspace? Default team settings will be cleared.")) {
                      return;
                    }
                    disconnectLinearMutation.mutate();
                  }}
                >
                  {disconnectLinearMutation.isPending ? "Disconnecting..." : "Disconnect Linear"}
                </Button>
                {linearQuery.data?.connection_error && (
                  <p className="w-full text-xs text-warning-foreground">{linearQuery.data.connection_error}</p>
                )}
              </CardContent>
            </Card>

            <Card className="border-destructive/30 shadow-sm">
              <CardHeader>
                <CardTitle className="text-base font-semibold">Danger zone</CardTitle>
                <CardDescription className="text-xs">
                  Permanently delete this workspace and all stored requests, actions, channels, and integration tokens.
                </CardDescription>
              </CardHeader>
              <CardContent className="flex flex-wrap items-center gap-2">
                <Button
                  type="button"
                  variant="destructive"
                  disabled={deleteWorkspaceMutation.isPending}
                  onClick={() => {
                    const confirmation = window.prompt("Type DELETE to permanently remove this workspace.");
                    if (confirmation !== "DELETE") {
                      toastInfo("Deletion canceled", "Workspace was not deleted.");
                      return;
                    }
                    deleteWorkspaceMutation.mutate();
                  }}
                >
                  {deleteWorkspaceMutation.isPending ? "Deleting..." : "Delete workspace"}
                </Button>
              </CardContent>
            </Card>
          </div>

          {(meQuery.isLoading || channelsQuery.isLoading || linearQuery.isLoading) && (
            <p className="text-sm text-muted-foreground">Loading onboarding data...</p>
          )}
        </div>
      </AdminShell>
    </RequireAuth>
  );
}

export default function OnboardingPage() {
  return (
    <Suspense
      fallback={
        <div className="flex min-h-screen items-center justify-center text-sm text-muted-foreground">
          Loading onboarding...
        </div>
      }
    >
      <OnboardingPageContent />
    </Suspense>
  );
}
