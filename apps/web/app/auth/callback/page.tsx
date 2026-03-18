"use client";

import { Suspense, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";

function getSafeNextPath(value: string | null): string {
  if (!value || !value.startsWith("/")) {
    return "/billing";
  }
  return value;
}

async function postJSON(url: string, body: Record<string, unknown>): Promise<{ ok: boolean; error?: string }> {
  const response = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body),
  });
  if (response.ok) {
    return { ok: true };
  }
  try {
    const data = (await response.json()) as { error?: string };
    return { ok: false, error: data.error || `Request failed (${response.status})` };
  } catch {
    return { ok: false, error: `Request failed (${response.status})` };
  }
}

function parseHashTokens(): { access_token: string; refresh_token: string; expires_at?: number; expires_in?: number } | null {
  const params = new URLSearchParams(window.location.hash.replace(/^#/, ""));
  const accessToken = params.get("access_token");
  const refreshToken = params.get("refresh_token");
  if (!accessToken || !refreshToken) {
    return null;
  }

  const expiresAtRaw = params.get("expires_at");
  const expiresInRaw = params.get("expires_in");
  const expiresAt = expiresAtRaw ? Number(expiresAtRaw) : undefined;
  const expiresIn = expiresInRaw ? Number(expiresInRaw) : undefined;

  return {
    access_token: accessToken,
    refresh_token: refreshToken,
    expires_at: Number.isFinite(expiresAt) ? expiresAt : undefined,
    expires_in: Number.isFinite(expiresIn) ? expiresIn : undefined,
  };
}

function AuthCallbackPageContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const nextPath = useMemo(() => getSafeNextPath(searchParams.get("next")), [searchParams]);
  const [message, setMessage] = useState("Finalizing authentication...");

  useEffect(() => {
    let mounted = true;

    async function finalizeAuth() {
      const tokens = parseHashTokens();
      if (tokens) {
        const setResult = await postJSON("/api/auth/set-session", tokens);
        if (!mounted) {
          return;
        }
        if (!setResult.ok) {
          setMessage(setResult.error || "Could not establish session. Please sign in.");
          window.setTimeout(() => {
            router.replace(`/login?next=${encodeURIComponent(nextPath)}`);
          }, 1500);
          return;
        }
      }

      const me = await fetch("/api/auth/me", { cache: "no-store" });
      if (!mounted) {
        return;
      }

      if (me.ok) {
        router.replace(nextPath);
        return;
      }

      setMessage("Email confirmed. You can now sign in with your password.");
      window.setTimeout(() => {
        router.replace(`/login?next=${encodeURIComponent(nextPath)}`);
      }, 1200);
    }

    finalizeAuth();
    return () => {
      mounted = false;
    };
  }, [router, nextPath]);

  return (
    <main className="flex min-h-screen items-center justify-center px-4">
      <div className="max-w-md space-y-2 text-center">
        <p className="text-sm text-muted-foreground">{message}</p>
        <p className="text-xs text-muted-foreground">
          <Link href="/login" className="underline underline-offset-2">
            Open login
          </Link>
        </p>
      </div>
    </main>
  );
}

export default function AuthCallbackPage() {
  return (
    <Suspense
      fallback={
        <main className="flex min-h-screen items-center justify-center px-4">
          <p className="text-sm text-muted-foreground">Finalizing authentication...</p>
        </main>
      }
    >
      <AuthCallbackPageContent />
    </Suspense>
  );
}
