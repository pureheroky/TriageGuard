"use client";

import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Hash, Lock, Search } from "lucide-react";
import { apiFetch } from "@/lib/api";
import { AdminShell } from "@/components/admin-shell";
import { RequireAuth } from "@/app/components/require-auth";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { toastError, toastSuccess } from "@/lib/stores/toast-store";

type Channel = {
  channel_id: string;
  channel_name: string;
  is_private?: boolean;
  enabled: boolean;
};

type ChannelOverride = {
  channel_id: string;
  ack_sla_minutes: number;
  assign_sla_minutes: number;
  stale_hours: number;
};

type ChannelOverrideDraft = {
  ack_sla_minutes: string;
  assign_sla_minutes: string;
  stale_hours: string;
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

function toOverrideDrafts(overrides: ChannelOverride[]): Record<string, ChannelOverrideDraft> {
  const out: Record<string, ChannelOverrideDraft> = {};
  for (const item of overrides) {
    out[item.channel_id] = {
      ack_sla_minutes: String(item.ack_sla_minutes),
      assign_sla_minutes: String(item.assign_sla_minutes),
      stale_hours: String(item.stale_hours),
    };
  }
  return out;
}

export default function ChannelsPage() {
  const queryClient = useQueryClient();
  const [search, setSearch] = useState("");
  const [localChannels, setLocalChannels] = useState<Channel[] | null>(null);
  const [localOverrides, setLocalOverrides] = useState<Record<string, ChannelOverrideDraft> | null>(null);

  const meQuery = useQuery({
    queryKey: ["me"],
    queryFn: () => apiFetch<MeResponse>("/api/me"),
  });
  const isEnterprise =
    meQuery.data?.billing?.effective_plan === "enterprise" && isPaidActive(meQuery.data?.billing?.status || "");

  const channelsQuery = useQuery({
    queryKey: ["channels"],
    queryFn: () => apiFetch<{ channels: Channel[] }>("/api/channels"),
  });

  const overridesQuery = useQuery({
    queryKey: ["channel-overrides"],
    queryFn: () => apiFetch<{ overrides: ChannelOverride[]; can_edit: boolean }>("/api/channels/overrides"),
    enabled: isEnterprise,
  });

  useEffect(() => {
    if (!isEnterprise) {
      if (localOverrides !== null) {
        setLocalOverrides(null);
      }
      return;
    }
    if (localOverrides !== null || overridesQuery.isLoading) {
      return;
    }
    setLocalOverrides(toOverrideDrafts(overridesQuery.data?.overrides ?? []));
  }, [isEnterprise, localOverrides, overridesQuery.isLoading, overridesQuery.data?.overrides]);

  const channels = useMemo(() => localChannels ?? channelsQuery.data?.channels ?? [], [localChannels, channelsQuery.data?.channels]);

  const filtered = useMemo(
    () => channels.filter((channel) => channel.channel_name.toLowerCase().includes(search.toLowerCase())),
    [channels, search],
  );

  const syncMutation = useMutation({
    mutationFn: () => apiFetch<{ channels: Channel[] }>("/api/channels/sync", { method: "POST" }),
    onSuccess: (data) => {
      setLocalChannels(data.channels);
      queryClient.invalidateQueries({ queryKey: ["channels"] });
      toastSuccess("Channels synced", "Latest Slack channels were loaded.");
    },
    onError: (error) => {
      toastError("Sync failed", error instanceof Error ? error.message : "Could not sync channels.");
    },
  });

  const saveChannelsMutation = useMutation({
    mutationFn: (payload: Channel[]) =>
      apiFetch("/api/channels", {
        method: "PUT",
        body: JSON.stringify({
          channels: payload.map((channel) => ({ channel_id: channel.channel_id, enabled: channel.enabled })),
        }),
      }),
  });

  const saveOverridesMutation = useMutation({
    mutationFn: (overrides: ChannelOverride[]) =>
      apiFetch<{ overrides: ChannelOverride[] }>("/api/channels/overrides", {
        method: "PUT",
        body: JSON.stringify({ overrides }),
      }),
  });

  function updateOverrideDraft(channelID: string, field: keyof ChannelOverrideDraft, value: string) {
    setLocalOverrides((prev) => {
      const next = { ...(prev ?? {}) };
      const current = next[channelID] ?? { ack_sla_minutes: "", assign_sla_minutes: "", stale_hours: "" };
      const updated: ChannelOverrideDraft = { ...current, [field]: value };
      if (!updated.ack_sla_minutes && !updated.assign_sla_minutes && !updated.stale_hours) {
        delete next[channelID];
      } else {
        next[channelID] = updated;
      }
      return next;
    });
  }

  function buildOverridePayload(): ChannelOverride[] {
    if (!isEnterprise) {
      return [];
    }
    const drafts = localOverrides ?? {};
    const payload: ChannelOverride[] = [];
    for (const [channelID, draft] of Object.entries(drafts)) {
      if (!draft.ack_sla_minutes && !draft.assign_sla_minutes && !draft.stale_hours) {
        continue;
      }
      if (!draft.ack_sla_minutes || !draft.assign_sla_minutes || !draft.stale_hours) {
        throw new Error(`Fill all SLA fields for channel ${channelID} or clear them to inherit workspace policy.`);
      }
      const ack = Number(draft.ack_sla_minutes);
      const assign = Number(draft.assign_sla_minutes);
      const stale = Number(draft.stale_hours);
      if (!Number.isFinite(ack) || !Number.isFinite(assign) || !Number.isFinite(stale) || ack <= 0 || assign <= 0 || stale <= 0) {
        throw new Error(`SLA override values must be positive numbers for channel ${channelID}.`);
      }
      payload.push({
        channel_id: channelID,
        ack_sla_minutes: ack,
        assign_sla_minutes: assign,
        stale_hours: stale,
      });
    }
    return payload;
  }

  async function handleSave() {
    try {
      if (isEnterprise && overridesQuery.isLoading) {
        throw new Error("Loading channel SLA overrides. Please try save again in a moment.");
      }
      await saveChannelsMutation.mutateAsync(channels);
      if (isEnterprise) {
        const payload = buildOverridePayload();
        await saveOverridesMutation.mutateAsync(payload);
      }
      setLocalChannels(null);
      queryClient.invalidateQueries({ queryKey: ["channels"] });
      queryClient.invalidateQueries({ queryKey: ["channel-overrides"] });
      toastSuccess("Channels saved", isEnterprise ? "Monitoring settings and per-channel SLA overrides were updated." : "Monitoring settings were updated.");
    } catch (error) {
      toastError("Save failed", error instanceof Error ? error.message : "Could not save channel settings.");
    }
  }

  const isSaving = saveChannelsMutation.isPending || saveOverridesMutation.isPending || (isEnterprise && overridesQuery.isLoading);
  const overrideDrafts = localOverrides ?? {};

  return (
    <RequireAuth>
      <AdminShell>
        <div className="mx-auto max-w-6xl space-y-6">
          <div>
            <h1 className="text-2xl font-bold tracking-tight text-foreground">Channels</h1>
            <p className="mt-1 text-sm text-muted-foreground">Select Slack channels monitored by TriageGuard.</p>
          </div>

          <Card className="border-border/60 shadow-sm">
            <CardHeader className="pb-4">
              <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
                <div>
                  <CardTitle className="text-base font-semibold">Monitored channels</CardTitle>
                  <CardDescription className="text-xs">
                    {channels.filter((channel) => channel.enabled).length} of {channels.length} enabled
                  </CardDescription>
                </div>
                <div className="flex items-center gap-2">
                  <Button variant="outline" size="sm" className="text-xs" onClick={() => syncMutation.mutate()}>
                    Sync channels
                  </Button>
                  <Button size="sm" className="text-xs" onClick={handleSave} disabled={isSaving}>
                    {isSaving ? "Saving..." : "Save"}
                  </Button>
                </div>
              </div>
            </CardHeader>
            <CardContent className="p-0">
              <div className="flex items-center gap-3 border-b border-border/40 px-6 pb-4">
                <div className="relative flex-1 max-w-sm">
                  <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    placeholder="Search channels..."
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                    className="h-9 pl-9 text-sm"
                  />
                </div>
                {isEnterprise ? (
                  <Badge variant="outline" className="rounded-md">
                    Enterprise: per-channel SLA enabled
                  </Badge>
                ) : (
                  <Badge variant="secondary" className="rounded-md">
                    Upgrade to Enterprise for per-channel SLA
                  </Badge>
                )}
              </div>

              <Table>
                <TableHeader>
                  <TableRow className="border-border/40 hover:bg-transparent">
                    <TableHead className="w-12 pl-6 text-xs">Active</TableHead>
                    <TableHead className="text-xs">Channel</TableHead>
                    <TableHead className="text-xs">Visibility</TableHead>
                    <TableHead className="text-xs">ID</TableHead>
                    <TableHead className="text-xs">SLA override (Ack/Assign/Stale)</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filtered.map((channel) => {
                    const draft = overrideDrafts[channel.channel_id] ?? {
                      ack_sla_minutes: "",
                      assign_sla_minutes: "",
                      stale_hours: "",
                    };
                    return (
                      <TableRow key={channel.channel_id} className="border-border/40">
                        <TableCell className="pl-6">
                          <Switch
                            checked={channel.enabled}
                            onCheckedChange={(next) =>
                              setLocalChannels(
                                channels.map((item) =>
                                  item.channel_id === channel.channel_id ? { ...item, enabled: next } : item,
                                ),
                              )
                            }
                          />
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            {channel.is_private ? <Lock className="size-3.5 text-muted-foreground" /> : <Hash className="size-3.5 text-muted-foreground" />}
                            <span className="text-sm font-medium text-foreground">#{channel.channel_name}</span>
                            {channel.enabled ? (
                              <Badge variant="secondary" className="rounded-md px-1.5 py-0 text-[9px]">
                                enabled
                              </Badge>
                            ) : null}
                          </div>
                        </TableCell>
                        <TableCell>
                          <Badge variant="secondary" className="rounded-md px-1.5 py-0 text-[9px] uppercase tracking-wide">
                            {channel.is_private ? "private" : "public"}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-xs text-muted-foreground">{channel.channel_id}</TableCell>
                        <TableCell>
                          {isEnterprise ? (
                            <div className="grid grid-cols-3 gap-2">
                              <Input
                                type="number"
                                min={1}
                                value={draft.ack_sla_minutes}
                                onChange={(e) => updateOverrideDraft(channel.channel_id, "ack_sla_minutes", e.target.value)}
                                placeholder="Ack min"
                                className="h-8 text-xs"
                              />
                              <Input
                                type="number"
                                min={1}
                                value={draft.assign_sla_minutes}
                                onChange={(e) => updateOverrideDraft(channel.channel_id, "assign_sla_minutes", e.target.value)}
                                placeholder="Assign min"
                                className="h-8 text-xs"
                              />
                              <Input
                                type="number"
                                min={1}
                                value={draft.stale_hours}
                                onChange={(e) => updateOverrideDraft(channel.channel_id, "stale_hours", e.target.value)}
                                placeholder="Stale h"
                                className="h-8 text-xs"
                              />
                            </div>
                          ) : (
                            <span className="text-xs text-muted-foreground">Enterprise only</span>
                          )}
                        </TableCell>
                      </TableRow>
                    );
                  })}
                  {filtered.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={5} className="py-12 text-center">
                        <p className="text-sm text-muted-foreground">No channels found.</p>
                      </TableCell>
                    </TableRow>
                  ) : null}
                </TableBody>
              </Table>
            </CardContent>
          </Card>

          {channelsQuery.isLoading ? <p className="text-sm text-muted-foreground">Loading channels...</p> : null}
        </div>
      </AdminShell>
    </RequireAuth>
  );
}
