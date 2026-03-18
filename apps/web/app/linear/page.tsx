"use client";

import { FormEvent, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowRightLeft, Link as LinkIcon } from "lucide-react";
import { apiFetch } from "@/lib/api";
import { AdminShell } from "@/components/admin-shell";
import { RequireAuth } from "@/app/components/require-auth";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { toastInfo, toastSuccess } from "@/lib/stores/toast-store";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

type LinearState = {
  installed: boolean;
  connected: boolean;
  requires_reconnect?: boolean;
  connection_error?: string;
  linear_team?: string;
  oauth_url: string;
};

type LinearTeam = {
  id: string;
  key: string;
  name: string;
};

export default function LinearPage() {
  const queryClient = useQueryClient();
  const [teamID, setTeamID] = useState("");

  const query = useQuery({
    queryKey: ["linear"],
    queryFn: () => apiFetch<LinearState>("/api/linear"),
  });
  const teamsQuery = useQuery({
    queryKey: ["linear", "teams"],
    queryFn: () => apiFetch<{ teams: LinearTeam[] }>("/api/linear/teams"),
    enabled: Boolean(query.data?.connected),
    retry: false,
  });

  useEffect(() => {
    if (query.data?.linear_team) {
      setTeamID(query.data.linear_team);
    }
  }, [query.data]);

  const mutation = useMutation({
    mutationFn: (value: string) =>
      apiFetch("/api/linear/defaults", {
        method: "PUT",
        body: JSON.stringify({ linear_team_id: value }),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["linear"] });
      queryClient.invalidateQueries({ queryKey: ["linear", "teams"] });
      toastSuccess("Linear defaults saved", "Default team ID updated.");
    },
  });
  const disconnectMutation = useMutation({
    mutationFn: () => apiFetch<{ ok: boolean; disconnected: boolean }>("/api/linear/disconnect", { method: "POST" }),
    onSuccess: (data) => {
      setTeamID("");
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

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    mutation.mutate(teamID);
  }

  return (
    <RequireAuth>
      <AdminShell>
        <div className="mx-auto max-w-4xl space-y-6">
          <div>
            <h1 className="text-2xl font-bold tracking-tight text-foreground">Linear Integration</h1>
            <p className="mt-1 text-sm text-muted-foreground">Connect Linear to convert Slack requests into issues.</p>
          </div>

          <Card className="border-border/60 shadow-sm">
            <CardContent className="flex items-center justify-between p-5">
              <div className="flex items-center gap-4">
                <div className="flex size-12 items-center justify-center rounded-xl bg-primary/10">
                  <ArrowRightLeft className="size-5 text-primary" />
                </div>
                <div>
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-semibold">Linear</span>
                    {query.data?.connected ? (
                      <Badge className="rounded-md bg-success/10 text-success hover:bg-success/10">Connected</Badge>
                    ) : query.data?.requires_reconnect ? (
                      <Badge className="rounded-md bg-warning/10 text-warning-foreground hover:bg-warning/10">Reconnect required</Badge>
                    ) : (
                      <Badge variant="secondary" className="rounded-md">
                        Disconnected
                      </Badge>
                    )}
                  </div>
                  <p className="text-xs text-muted-foreground">Use OAuth connection and set default team ID.</p>
                </div>
              </div>
              {query.data?.connected ? (
                <Button size="sm" className="gap-1.5" disabled>
                  <LinkIcon className="size-3.5" />
                  Connected
                </Button>
              ) : (
                <a href="/api/backend/api/linear/install">
                  <Button size="sm" className="gap-1.5" disabled={disconnectMutation.isPending}>
                    <LinkIcon className="size-3.5" />
                    {query.data?.requires_reconnect ? "Reconnect OAuth" : "Connect OAuth"}
                  </Button>
                </a>
              )}
              <Button
                size="sm"
                variant="outline"
                disabled={!query.data?.installed || disconnectMutation.isPending}
                onClick={() => {
                  if (!window.confirm("Disconnect Linear from this workspace? Default team settings will be cleared.")) {
                    return;
                  }
                  disconnectMutation.mutate();
                }}
              >
                {disconnectMutation.isPending ? "Disconnecting..." : "Disconnect"}
              </Button>
            </CardContent>
            {query.data?.connection_error && (
              <CardContent className="pt-0">
                <p className="text-xs text-warning-foreground">{query.data.connection_error}</p>
              </CardContent>
            )}
          </Card>

          <Card className="border-border/60 shadow-sm">
            <CardHeader>
              <CardTitle className="text-base font-semibold">Default team ID</CardTitle>
              <CardDescription className="text-xs">Required for issue creation. Pick a team from your Linear workspace.</CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={onSubmit} className="space-y-4">
                <div className="space-y-2">
                  <Label>Linear team</Label>
                  {teamsQuery.data?.teams?.length ? (
                    <Select value={teamID} onValueChange={setTeamID}>
                      <SelectTrigger className="w-full">
                        <SelectValue placeholder="Select team" />
                      </SelectTrigger>
                      <SelectContent>
                        {teamsQuery.data.teams.map((team) => (
                          <SelectItem key={team.id} value={team.id}>
                            {team.name} ({team.key})
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  ) : (
                    <Input value={teamID} onChange={(e) => setTeamID(e.target.value)} placeholder="team_xxx" />
                  )}
                  <p className="text-xs text-muted-foreground">Selected Team ID: {teamID || "not selected"}</p>
                </div>
                <Button type="submit" disabled={!query.data?.connected || mutation.isPending}>
                  {mutation.isPending ? "Saving..." : "Save team ID"}
                </Button>
              </form>
            </CardContent>
          </Card>

          {(query.isLoading || teamsQuery.isLoading) && <p className="text-sm text-muted-foreground">Loading integration state...</p>}
        </div>
      </AdminShell>
    </RequireAuth>
  );
}
