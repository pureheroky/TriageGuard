"use client";

import { FormEvent, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { validatePasswordPolicy } from "@/lib/auth-security";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { toastError, toastSuccess } from "@/lib/stores/toast-store";

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

async function postJSON(url: string, body: Record<string, unknown>): Promise<{ ok: true } | { ok: false; error: string; status: number }> {
  const response = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body),
  });

  let data: Record<string, unknown> = {};
  try {
    data = (await response.json()) as Record<string, unknown>;
  } catch {
    data = {};
  }

  if (!response.ok) {
    return {
      ok: false,
      status: response.status,
      error: String(data.error || `Request failed (${response.status})`),
    };
  }

  return { ok: true };
}

export default function ResetPasswordPage() {
  const router = useRouter();

  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [ready, setReady] = useState(false);
  const [validRecovery, setValidRecovery] = useState(false);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    let mounted = true;

    async function initRecoverySession() {
      const tokens = parseHashTokens();
      if (tokens) {
        const setResult = await postJSON("/api/auth/set-session", tokens);
        if (!mounted) {
          return;
        }
        if (!setResult.ok) {
          setValidRecovery(false);
          setReady(true);
          return;
        }
      }

      const me = await fetch("/api/auth/me", { cache: "no-store" });
      if (!mounted) {
        return;
      }
      setValidRecovery(me.ok);
      setReady(true);
    }

    initRecoverySession();
    return () => {
      mounted = false;
    };
  }, []);

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();

    if (password !== confirmPassword) {
      toastError("Invalid form", "Passwords do not match.");
      return;
    }

    const policyError = validatePasswordPolicy(password);
    if (policyError) {
      toastError("Weak password", policyError);
      return;
    }

    setLoading(true);
    const result = await postJSON("/api/auth/update-password", { password });
    setLoading(false);

    if (!result.ok) {
      toastError("Password update failed", result.error);
      return;
    }

    toastSuccess("Password updated", "Redirecting to billing...");
    window.setTimeout(() => {
      router.replace("/billing");
    }, 1000);
  }

  if (!ready) {
    return <main className="flex min-h-screen items-center justify-center text-sm text-muted-foreground">Checking reset link...</main>;
  }

  if (!validRecovery) {
    return (
      <main className="flex min-h-screen items-center justify-center px-4">
        <Card className="w-full max-w-md border-border/60 shadow-sm">
          <CardHeader>
            <CardTitle>Reset link expired</CardTitle>
            <CardDescription>Request a new password reset link from login page.</CardDescription>
          </CardHeader>
          <CardContent>
            <Link href="/login?mode=forgot" className="text-sm underline underline-offset-2">
              Back to reset request
            </Link>
          </CardContent>
        </Card>
      </main>
    );
  }

  return (
    <main className="flex min-h-screen items-center justify-center bg-muted/30 px-4">
      <Card className="w-full max-w-md border-border/60 shadow-sm">
        <CardHeader>
          <CardTitle>Set new password</CardTitle>
          <CardDescription>Use a strong password to secure your workspace.</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={onSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="password">New password</Label>
              <Input
                id="password"
                type="password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="new-password"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="confirmPassword">Confirm password</Label>
              <Input
                id="confirmPassword"
                type="password"
                required
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                autoComplete="new-password"
              />
            </div>
            <p className="text-xs text-muted-foreground">
              Password policy: 12+ chars, uppercase, lowercase, number, symbol.
            </p>
            <Button type="submit" disabled={loading} className="w-full">
              {loading ? "Updating..." : "Update password"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </main>
  );
}
