"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { cn } from "@/lib/utils";
import type { LucideIcon } from "lucide-react";
import { LayoutDashboard, Hash, Timer, ArrowRightLeft, CreditCard, Activity, Menu, LogOut, ChevronLeft, Sparkles, ListChecks, ShieldCheck, ServerCog } from "lucide-react";
import { Button } from "@/components/ui/button";
import { TriageGuardLogo } from "@/components/triageguard-logo";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Separator } from "@/components/ui/separator";
import { Sheet, SheetContent, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { useUIStore } from "@/lib/stores/ui-store";
import { apiFetch } from "@/lib/api";

const fullNavigation = [
  { name: "Dashboard", href: "/dashboard", icon: LayoutDashboard },
  { name: "Onboarding", href: "/onboarding", icon: Sparkles },
  { name: "Channels", href: "/channels", icon: Hash },
  { name: "Queues", href: "/queues", icon: ListChecks },
  { name: "SLA Policies", href: "/policies", icon: Timer },
  { name: "Ops", href: "/ops", icon: ShieldCheck },
  { name: "Linear", href: "/linear", icon: ArrowRightLeft },
  { name: "Billing", href: "/billing", icon: CreditCard },
  { name: "Activity", href: "/activity", icon: Activity },
];

const billingOnlyNavigation = [{ name: "Billing", href: "/billing", icon: CreditCard }];

function isPaidActive(status: string): boolean {
  return ["active", "trialing", "past_due", "unpaid", "incomplete"].includes(status);
}

function SidebarNav({
  onNavigate,
  navigation,
}: {
  onNavigate?: () => void;
  navigation: { name: string; href: string; icon: LucideIcon }[];
}) {
  const pathname = usePathname();

  return (
    <nav className="flex flex-col gap-1 px-3">
      {navigation.map((item) => {
        const isActive = pathname === item.href;
        return (
          <Link
            key={item.name}
            href={item.href}
            onClick={onNavigate}
            className={cn(
              "flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors",
              isActive ? "bg-primary/10 text-primary" : "text-muted-foreground hover:bg-accent hover:text-foreground",
            )}
          >
            <item.icon className="size-4" />
            {item.name}
          </Link>
        );
      })}
    </nav>
  );
}

function SidebarContent({ onNavigate }: { onNavigate?: () => void }) {
  const router = useRouter();
  const authQuery = useQuery({
    queryKey: ["auth-me"],
    queryFn: async () => {
      const response = await fetch("/api/auth/me", { cache: "no-store" });
      if (!response.ok) {
        return null;
      }
      return (await response.json()) as {
        user?: {
          email?: string;
          user_metadata?: {
            full_name?: string;
            name?: string;
          };
        };
      };
    },
    retry: false,
  });

  const userEmail = authQuery.data?.user?.email?.trim() || "";
  const metadata = authQuery.data?.user?.user_metadata;
  const userName = (metadata?.full_name || metadata?.name || "").trim();
  const userLabel = userName || (userEmail ? userEmail.split("@")[0] : "Workspace Admin");
  const userSecondary = userEmail || "triageguard";
  const initials = userLabel
    .split(" ")
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase() || "")
    .join("") || "TG";
  const meQuery = useQuery({
    queryKey: ["me"],
    queryFn: () =>
      apiFetch<{
        integrations?: { slack_connected?: boolean };
        billing?: { effective_plan?: string; status?: string };
        internal_operator?: boolean;
      }>("/api/me"),
  });
  const billing = meQuery.data?.billing;
  const plan = billing?.effective_plan || "";
  const hasPaidSubscription = (plan === "team" || plan === "enterprise") && isPaidActive(billing?.status || "");
  const navigation = hasPaidSubscription
    ? meQuery.data?.internal_operator
      ? [...fullNavigation, { name: "Internal Ops", href: "/internal-ops", icon: ServerCog }]
      : fullNavigation
    : billingOnlyNavigation;

  async function logout() {
    await fetch("/api/auth/logout", {
      method: "POST",
    });
    router.push("/login");
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex h-16 items-center gap-2.5 border-b border-border/40 px-5">
        <div className="flex size-8 items-center justify-center rounded-lg bg-primary">
          <TriageGuardLogo size={18} className="text-primary-foreground" />
        </div>
        <div className="flex flex-col">
          <span className="text-sm font-semibold text-foreground">TriageGuard</span>
          <span className="text-[10px] text-muted-foreground">Admin Console</span>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto py-4">
        <SidebarNav onNavigate={onNavigate} navigation={navigation} />
      </div>

      <div className="border-t border-border/40 p-3">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button className="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left transition-colors hover:bg-accent">
              <Avatar className="size-8">
                <AvatarFallback className="bg-primary/10 text-xs font-semibold text-primary">{initials}</AvatarFallback>
              </Avatar>
              <div className="flex flex-col overflow-hidden">
                <span className="truncate text-sm font-medium text-foreground">{userLabel}</span>
                <span className="truncate text-[11px] text-muted-foreground">{userSecondary}</span>
              </div>
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-56">
            <DropdownMenuItem onClick={logout}>
              <LogOut className="size-4" />
              Sign out
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  );
}

export function AdminShell({ children }: { children: React.ReactNode }) {
  const { mobileSidebarOpen, setMobileSidebarOpen } = useUIStore();
  const meQuery = useQuery({
    queryKey: ["me"],
    queryFn: () =>
      apiFetch<{
        integrations?: { slack_connected?: boolean };
        billing?: { effective_plan?: string; status?: string };
      }>("/api/me"),
  });
  const slackConnected = meQuery.data?.integrations?.slack_connected ?? false;
  const billing = meQuery.data?.billing;
  const plan = billing?.effective_plan || "";
  const hasPaidSubscription = (plan === "team" || plan === "enterprise") && isPaidActive(billing?.status || "");
  const opsQuery = useQuery({
    queryKey: ["ops-summary-banner"],
    queryFn: () =>
      apiFetch<{
        health_banner: string;
      }>("/api/ops/summary"),
    enabled: hasPaidSubscription,
    refetchInterval: 30_000,
    refetchIntervalInBackground: true,
  });
  const slackStatusLabel = hasPaidSubscription
    ? slackConnected
      ? "Slack workspace connected"
      : "Slack not connected"
    : "Activate Team or Enterprise to unlock setup";

  return (
    <div className="flex h-dvh bg-background">
      <aside className="hidden w-64 shrink-0 border-r border-border/40 bg-card lg:block">
        <SidebarContent />
      </aside>

      <div className="flex flex-1 flex-col overflow-hidden">
        <header className="flex h-14 shrink-0 items-center justify-between border-b border-border/40 bg-card px-4 lg:px-6">
          <div className="flex items-center gap-3">
            <Sheet open={mobileSidebarOpen} onOpenChange={setMobileSidebarOpen}>
              <SheetTrigger asChild>
                <Button variant="ghost" size="icon" className="lg:hidden">
                  <Menu className="size-5" />
                  <span className="sr-only">Open menu</span>
                </Button>
              </SheetTrigger>
              <SheetContent side="left" className="w-64 p-0">
                <SheetTitle className="sr-only">Navigation</SheetTitle>
                <SidebarContent onNavigate={() => setMobileSidebarOpen(false)} />
              </SheetContent>
            </Sheet>

            <div className="hidden items-center gap-2 text-sm lg:flex">
              <Link href="/" className="flex items-center gap-1 text-muted-foreground transition-colors hover:text-foreground">
                <ChevronLeft className="size-3.5" />
                Back to site
              </Link>
            </div>
          </div>

          <div className="flex items-center gap-2">
            <span className="hidden text-xs text-muted-foreground sm:block">{slackStatusLabel}</span>
            <Separator orientation="vertical" className="hidden h-5 sm:block" />
            {hasPaidSubscription ? (
              <Button size="sm" className="gap-1.5 rounded-lg text-xs font-medium shadow-sm" asChild>
                <Link href="/onboarding">
                  <Sparkles className="size-3" />
                  {slackConnected ? "Manage Setup" : "Complete Setup"}
                </Link>
              </Button>
            ) : null}
          </div>
        </header>

        {hasPaidSubscription && opsQuery.data?.health_banner ? (
          <div className="border-b border-border/40 bg-amber-50 px-4 py-2 text-xs text-amber-950 lg:px-6">
            {opsQuery.data.health_banner}
          </div>
        ) : null}

        <main className="flex-1 overflow-y-auto bg-muted/30 p-4 lg:p-6">{children}</main>
      </div>
    </div>
  );
}
