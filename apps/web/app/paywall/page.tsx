"use client";

import { Suspense, useEffect } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { ArrowRight, CreditCard, LockKeyhole } from "lucide-react";
import { AdminShell } from "@/components/admin-shell";
import { RequireAuth } from "@/app/components/require-auth";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { apiFetch } from "@/lib/api";

type MeResponse = {
  billing?: {
    effective_plan?: string;
    status?: string;
  };
};

function isPaidActive(status: string): boolean {
  return ["active", "trialing", "past_due", "unpaid", "incomplete"].includes(status);
}

function PaywallPageContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const requestedPath = searchParams.get("next") || "";
  const safeRequestedPath = requestedPath.startsWith("/") ? requestedPath : "";

  const meQuery = useQuery({
    queryKey: ["me"],
    queryFn: () => apiFetch<MeResponse>("/api/me"),
    refetchInterval: (query) => {
      const data = query.state.data as MeResponse | undefined;
      const currentPlan = data?.billing?.effective_plan || "";
      const currentStatus = data?.billing?.status || "inactive";
      const paid = (currentPlan === "team" || currentPlan === "enterprise") && isPaidActive(currentStatus);
      return paid ? false : 10_000;
    },
    refetchIntervalInBackground: true,
    refetchOnWindowFocus: true,
  });

  const plan = meQuery.data?.billing?.effective_plan || "";
  const status = meQuery.data?.billing?.status || "inactive";
  const hasPaidSubscription = (plan === "team" || plan === "enterprise") && isPaidActive(status);

  useEffect(() => {
    if (!hasPaidSubscription) {
      return;
    }
    if (safeRequestedPath && safeRequestedPath !== "/paywall") {
      router.replace(safeRequestedPath);
      return;
    }
    router.replace("/dashboard");
  }, [hasPaidSubscription, router, safeRequestedPath]);

  return (
    <RequireAuth>
      <AdminShell>
        <div className="mx-auto max-w-3xl space-y-6">
          <div>
            <h1 className="text-2xl font-bold tracking-tight text-foreground">Upgrade Required</h1>
            <p className="mt-1 text-sm text-muted-foreground">Activate Team or Enterprise subscription to unlock Slack setup, triage, SLA, and Linear flows.</p>
          </div>

          <Card className="border-border/60 shadow-sm">
            <CardHeader>
              <div className="flex items-center gap-3">
                <div className="flex size-10 items-center justify-center rounded-xl bg-primary/10">
                  <LockKeyhole className="size-5 text-primary" />
                </div>
                <div>
                  <CardTitle className="text-base font-semibold">Your workspace is currently locked</CardTitle>
                  <CardDescription className="text-xs">
                    Billing status: <span className="font-medium uppercase text-foreground">{status}</span>
                  </CardDescription>
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex flex-wrap gap-2">
                <Badge className="rounded-md">Paid plan required</Badge>
                {safeRequestedPath ? <Badge variant="secondary">Blocked: {safeRequestedPath}</Badge> : null}
              </div>
              <p className="text-sm text-muted-foreground">
                Once payment is confirmed, this workspace unlocks automatically and you can continue onboarding and operations.
              </p>
              <p className="text-xs text-muted-foreground">
                Auto-checking subscription every 10 seconds{meQuery.isFetching ? "..." : "."}
              </p>
              <div className="flex flex-wrap gap-2">
                <Button asChild className="gap-1.5">
                  <Link href="/billing">
                    <CreditCard className="size-4" />
                    Go to Billing
                    <ArrowRight className="size-4" />
                  </Link>
                </Button>
                <Button variant="outline" onClick={() => meQuery.refetch()} disabled={meQuery.isFetching}>
                  {meQuery.isFetching ? "Checking..." : "I already paid, recheck"}
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>
      </AdminShell>
    </RequireAuth>
  );
}

export default function PaywallPage() {
  return (
    <Suspense
      fallback={
        <RequireAuth>
          <AdminShell>
            <div className="mx-auto max-w-3xl">
              <Card className="border-border/60 shadow-sm">
                <CardContent className="p-6 text-sm text-muted-foreground">Loading paywall...</CardContent>
              </Card>
            </div>
          </AdminShell>
        </RequireAuth>
      }
    >
      <PaywallPageContent />
    </Suspense>
  );
}
