"use client";

import { useEffect, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { motion } from "framer-motion";
import { TriageGuardLogo } from "@/components/triageguard-logo";

function isPaidActive(status: string): boolean {
  return ["active", "trialing", "past_due", "unpaid", "incomplete"].includes(status);
}

export function RequireAuth({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const [ready, setReady] = useState(false);

  useEffect(() => {
    let mounted = true;

    async function checkAuth() {
      try {
        const response = await fetch("/api/auth/me", { cache: "no-store" });
        if (!mounted) {
          return;
        }
        if (response.status === 401) {
          const nextPath = `${window.location.pathname}${window.location.search}`;
          const safeNext = nextPath.startsWith("/") ? nextPath : "/billing";
          router.replace(`/login?next=${encodeURIComponent(safeNext)}`);
          return;
        }
        if (!response.ok) {
          router.replace("/login");
          return;
        }
        const payload = (await response.json()) as {
          workspace?: {
            billing?: {
              effective_plan?: string;
              status?: string;
            };
          };
        };
        const billing = payload.workspace?.billing;
        const plan = billing?.effective_plan || "";
        const hasPaidSubscription = (plan === "team" || plan === "enterprise") && isPaidActive(billing?.status || "");
        if (pathname !== "/billing" && pathname !== "/paywall" && !hasPaidSubscription) {
          const nextPath = `${window.location.pathname}${window.location.search}`;
          const safeNext = nextPath.startsWith("/") ? nextPath : "/dashboard";
          router.replace(`/paywall?next=${encodeURIComponent(safeNext)}`);
          return;
        }
        setReady(true);
      } catch {
        if (!mounted) {
          return;
        }
        router.replace("/login");
      }
    }

    checkAuth();
    return () => {
      mounted = false;
    };
  }, [router, pathname]);

  if (!ready) {
    return (
      <div className="relative flex min-h-screen items-center justify-center overflow-hidden bg-background">
        <motion.div
          className="absolute size-64 rounded-full bg-primary/10 blur-3xl"
          animate={{ scale: [0.92, 1.08, 0.92], opacity: [0.25, 0.55, 0.25] }}
          transition={{ duration: 2.2, ease: "easeInOut", repeat: Number.POSITIVE_INFINITY }}
        />
        <div className="relative flex flex-col items-center gap-4 rounded-2xl border border-border/70 bg-card/70 px-8 py-6 backdrop-blur">
          <motion.div
            className="flex size-14 items-center justify-center rounded-2xl border border-primary/30 bg-primary/10"
            animate={{ rotate: [0, 8, -8, 0] }}
            transition={{ duration: 1.8, ease: "easeInOut", repeat: Number.POSITIVE_INFINITY }}
          >
            <TriageGuardLogo size={30} className="text-primary" />
          </motion.div>
          <div className="text-center">
            <p className="text-sm font-semibold text-foreground">Preparing your workspace</p>
            <p className="mt-1 text-xs text-muted-foreground">Validating session and access policy...</p>
          </div>
          <div className="flex items-center gap-1.5">
            {[0, 1, 2].map((idx) => (
              <motion.span
                key={idx}
                className="size-1.5 rounded-full bg-primary/80"
                animate={{ y: [0, -4, 0], opacity: [0.5, 1, 0.5] }}
                transition={{
                  duration: 0.8,
                  ease: "easeInOut",
                  repeat: Number.POSITIVE_INFINITY,
                  delay: idx * 0.12,
                }}
              />
            ))}
          </div>
        </div>
      </div>
    );
  }

  return <>{children}</>;
}
