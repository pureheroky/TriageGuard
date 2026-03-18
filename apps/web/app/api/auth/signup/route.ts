import { NextRequest, NextResponse } from "next/server";
import { setSessionCookies, signUpWithPassword } from "@/lib/server/supabase-auth";
import { validatePasswordPolicy } from "@/lib/password-policy";
import { consumeRateLimit } from "@/lib/server/rate-limit";
import { ensureSameOriginRequest } from "@/lib/server/request-security";

type SignupBody = {
  email?: string;
  password?: string;
  nextPath?: string;
};

function safeNextPath(nextPath?: string): string {
  if (!nextPath || !nextPath.startsWith("/")) {
    return "/billing";
  }
  return nextPath;
}

export async function POST(request: NextRequest) {
  const originError = ensureSameOriginRequest(request);
  if (originError) {
    return originError;
  }

  const rateLimit = consumeRateLimit(request, "auth_signup", 5, 30 * 60 * 1000);
  if (!rateLimit.allowed) {
    const response = NextResponse.json(
      { error: "Too many signup attempts. Try again later." },
      { status: 429 },
    );
    response.headers.set("Retry-After", String(rateLimit.retryAfterSeconds));
    return response;
  }

  let body: SignupBody;
  try {
    body = (await request.json()) as SignupBody;
  } catch {
    return NextResponse.json({ error: "Invalid request body." }, { status: 400 });
  }

  const email = (body.email || "").trim();
  const password = body.password || "";
  if (!email || !password) {
    return NextResponse.json({ error: "Email and password are required." }, { status: 400 });
  }
  const policyError = validatePasswordPolicy(password);
  if (policyError) {
    return NextResponse.json({ error: policyError }, { status: 400 });
  }

  const nextPath = safeNextPath(body.nextPath);
  const emailRedirectTo = `${request.nextUrl.origin}/auth/callback?next=${encodeURIComponent(nextPath)}`;
  const result = await signUpWithPassword(email, password, emailRedirectTo);
  if (!result.ok) {
    return NextResponse.json({ error: result.error }, { status: result.status });
  }

  const response = NextResponse.json({
    user: result.data.user,
    requiresEmailVerification: !result.data.tokens,
  });
  if (result.data.tokens) {
    setSessionCookies(response, result.data.tokens);
  }
  return response;
}
