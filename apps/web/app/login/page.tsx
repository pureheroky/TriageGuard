"use client";

import { FormEvent, Suspense, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { CheckCircle2, Circle, Eye, EyeOff } from "lucide-react";
import { clearLoginFailures, getLoginLockRemainingSeconds, registerLoginFailure, validatePasswordPolicy } from "@/lib/auth-security";
import { getPasswordPolicyRules } from "@/lib/password-policy";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { toastError, toastInfo, toastSuccess } from "@/lib/stores/toast-store";

type AuthMode = "signin" | "signup" | "forgot";

function parseMode(value: string | null): AuthMode {
  if (value === "signup" || value === "forgot" || value === "signin") {
    return value;
  }
  return "signin";
}

function getSafeNextPath(value: string | null): string {
  if (!value || !value.startsWith("/")) {
    return "/billing";
  }
  return value;
}

async function postJSON<TResponse>(url: string, body: Record<string, unknown>): Promise<{ ok: true; data: TResponse } | { ok: false; error: string; status: number }> {
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

  return {
    ok: true,
    data: data as TResponse,
  };
}

type BackendReadyResult =
  | { ok: true }
  | { ok: false; status: number; error: string };

async function backendSessionReady(): Promise<BackendReadyResult> {
  try {
    const response = await fetch("/api/auth/me", { cache: "no-store" });
    if (response.ok) {
      return { ok: true };
    }
    try {
      const body = (await response.json()) as { error?: string };
      return {
        ok: false,
        status: response.status,
        error: body.error || `Request failed (${response.status})`,
      };
    } catch {
      return {
        ok: false,
        status: response.status,
        error: `Request failed (${response.status})`,
      };
    }
  } catch {
    return { ok: false, status: 0, error: "Network error while checking session." };
  }
}

function mapAuthError(message: string): string {
  const value = message.toLowerCase();
  if (value.includes("invalid login credentials")) {
    return "Invalid email or password.";
  }
  if (value.includes("email not confirmed")) {
    return "Email confirmation is enabled in Supabase. Disable Confirm email for password-only login.";
  }
  if (value.includes("rate limit")) {
    return "Too many attempts. Wait a bit and try again.";
  }
  return message;
}

function LoginPageContent() {
  const router = useRouter();
  const searchParams = useSearchParams();

  const nextPath = useMemo(() => getSafeNextPath(searchParams.get("next")), [searchParams]);
  const initialMode = parseMode(searchParams.get("mode"));

  const [mode, setMode] = useState<AuthMode>(initialMode);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [showConfirmPassword, setShowConfirmPassword] = useState(false);
  const [loading, setLoading] = useState(false);
  const [lockRemaining, setLockRemaining] = useState(0);
  const passwordRules = useMemo(() => getPasswordPolicyRules(password), [password]);
  const confirmMatches = password.length > 0 && confirmPassword.length > 0 && password === confirmPassword;

  useEffect(() => {
    setMode(initialMode);
  }, [initialMode]);

  useEffect(() => {
    const interval = window.setInterval(() => {
      setLockRemaining(getLoginLockRemainingSeconds());
    }, 1000);
    setLockRemaining(getLoginLockRemainingSeconds());
    return () => window.clearInterval(interval);
  }, []);

  useEffect(() => {
    let mounted = true;

    async function checkSession() {
      const response = await fetch("/api/auth/me", { cache: "no-store" });
      if (!mounted) {
        return;
      }
      if (response.ok) {
        router.replace(nextPath);
      }
    }

    checkSession();
    return () => {
      mounted = false;
    };
  }, [router, nextPath]);

  function resetFeedback() {}

  function handleModeChange(nextMode: AuthMode) {
    setMode(nextMode);
    setPassword("");
    setConfirmPassword("");
    setShowPassword(false);
    setShowConfirmPassword(false);
    resetFeedback();
  }

  async function handleSignIn(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    resetFeedback();

    if (lockRemaining > 0) {
      toastError("Sign in blocked", `Too many failed attempts. Try again in ${lockRemaining}s.`);
      return;
    }

    setLoading(true);
    const result = await postJSON<{ user: Record<string, unknown> }>("/api/auth/login", {
      email: email.trim(),
      password,
    });
    setLoading(false);

    if (!result.ok) {
      const cooldown = registerLoginFailure();
      setLockRemaining(cooldown);
      const text = mapAuthError(result.error);
      toastError("Sign in failed", cooldown > 0 ? `Too many failed attempts. Try again in ${cooldown}s.` : text);
      return;
    }

    const ready = await backendSessionReady();
    if (!ready.ok) {
      if (ready.status === 401 || ready.status === 403) {
        toastError("Session unavailable", "Login succeeded, but we could not start your workspace session. Please try again.");
      } else {
        toastError("Session unavailable", "Login succeeded, but session verification failed. Please try again in a moment.");
      }
      return;
    }

    clearLoginFailures();
    toastSuccess("Signed in", "Session is active.");
    router.replace(nextPath);
  }

  async function handleSignUp(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    resetFeedback();

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
    const result = await postJSON<{ requiresEmailVerification: boolean }>("/api/auth/signup", {
      email: email.trim(),
      password,
      nextPath,
    });
    setLoading(false);

    if (!result.ok) {
      toastError("Sign up failed", mapAuthError(result.error));
      return;
    }

    if (!result.data.requiresEmailVerification) {
      const ready = await backendSessionReady();
      if (!ready.ok) {
        if (ready.status === 401 || ready.status === 403) {
          toastError("Session unavailable", "Account was created, but we could not start your workspace session. Please sign in again.");
        } else {
          toastError("Session unavailable", "Account was created, but session verification failed. Please sign in again.");
        }
        return;
      }
      clearLoginFailures();
      toastSuccess("Account created", "You are now signed in.");
      router.replace(nextPath);
      return;
    }

    toastInfo("Check your inbox", "Please confirm your email address to finish account setup.");
    setMode("signin");
    setPassword("");
    setConfirmPassword("");
  }

  async function handleForgotPassword(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    resetFeedback();

    if (!email.trim()) {
      toastError("Missing email", "Enter your email first.");
      return;
    }

    setLoading(true);
    const result = await postJSON<{ ok: boolean }>("/api/auth/forgot-password", {
      email: email.trim(),
    });
    setLoading(false);

    if (!result.ok) {
      toastError("Reset failed", mapAuthError(result.error));
      return;
    }

    toastInfo("Reset email sent", "Open the email link and set a new password.");
  }

  const submitLabel =
    mode === "signin" ? (loading ? "Signing in..." : "Sign in") :
    mode === "signup" ? (loading ? "Creating account..." : "Create account") :
    loading ? "Sending reset email..." : "Send reset email";

  return (
    <main className="flex min-h-screen items-center justify-center bg-muted/30 px-4">
      <Card className="w-full max-w-md border-border/60 shadow-sm">
        <CardHeader>
          <CardTitle>Admin Access</CardTitle>
          <CardDescription>Sign in to manage your workspace and billing.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-3 gap-2">
            <Button type="button" variant={mode === "signin" ? "default" : "outline"} onClick={() => handleModeChange("signin")}>
              Sign in
            </Button>
            <Button type="button" variant={mode === "signup" ? "default" : "outline"} onClick={() => handleModeChange("signup")}>
              Sign up
            </Button>
            <Button type="button" variant={mode === "forgot" ? "default" : "outline"} onClick={() => handleModeChange("forgot")}>
              Reset
            </Button>
          </div>

          <form
            onSubmit={
              mode === "signin" ? handleSignIn :
              mode === "signup" ? handleSignUp :
              handleForgotPassword
            }
            className="space-y-4"
          >
            <div className="space-y-2">
              <Label htmlFor="email">Email</Label>
              <Input
                id="email"
                type="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="you@company.com"
                autoComplete="email"
              />
            </div>

            {mode !== "forgot" && (
              <div className="space-y-2">
                <Label htmlFor="password">Password</Label>
                <div className="relative">
                  <Input
                    id="password"
                    type={showPassword ? "text" : "password"}
                    required
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    autoComplete={mode === "signin" ? "current-password" : "new-password"}
                    placeholder="Enter password"
                    className="pr-10"
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword((v) => !v)}
                    className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                    aria-label={showPassword ? "Hide password" : "Show password"}
                  >
                    {showPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                  </button>
                </div>
              </div>
            )}

            {mode === "signup" && (
              <>
                <div className="space-y-2">
                  <Label htmlFor="confirmPassword">Confirm password</Label>
                  <div className="relative">
                    <Input
                      id="confirmPassword"
                      type={showConfirmPassword ? "text" : "password"}
                      required
                      value={confirmPassword}
                      onChange={(e) => setConfirmPassword(e.target.value)}
                      autoComplete="new-password"
                      placeholder="Repeat password"
                      className="pr-10"
                    />
                    <button
                      type="button"
                      onClick={() => setShowConfirmPassword((v) => !v)}
                      className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                      aria-label={showConfirmPassword ? "Hide confirmation password" : "Show confirmation password"}
                    >
                      {showConfirmPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                    </button>
                  </div>
                </div>
                <div className="rounded-md border border-border/60 bg-muted/30 p-3">
                  <p className="text-xs font-medium text-foreground">Password requirements</p>
                  <ul className="mt-2 space-y-1.5">
                    {passwordRules.map((rule) => (
                      <li key={rule.id} className={`flex items-center gap-2 text-xs ${rule.ok ? "text-success" : "text-muted-foreground"}`}>
                        {rule.ok ? <CheckCircle2 className="size-3.5" /> : <Circle className="size-3.5" />}
                        <span>{rule.label}</span>
                      </li>
                    ))}
                    <li className={`flex items-center gap-2 text-xs ${confirmMatches ? "text-success" : "text-muted-foreground"}`}>
                      {confirmMatches ? <CheckCircle2 className="size-3.5" /> : <Circle className="size-3.5" />}
                      <span>Passwords match</span>
                    </li>
                  </ul>
                </div>
              </>
            )}

            <Button type="submit" disabled={loading || (mode === "signin" && lockRemaining > 0)} className="w-full">
              {submitLabel}
            </Button>
          </form>

          {mode === "signin" && (
            <button type="button" onClick={() => handleModeChange("forgot")} className="text-xs text-muted-foreground underline underline-offset-2">
              Forgot password?
            </button>
          )}

          <p className="text-xs text-muted-foreground">
            <Link href="/" className="underline underline-offset-2">
              Back to landing
            </Link>
          </p>
        </CardContent>
      </Card>
    </main>
  );
}

export default function LoginPage() {
  return (
    <Suspense
      fallback={
        <main className="flex min-h-screen items-center justify-center bg-muted/30 px-4">
          <p className="text-sm text-muted-foreground">Loading sign in...</p>
        </main>
      }
    >
      <LoginPageContent />
    </Suspense>
  );
}
