import { NextRequest, NextResponse } from "next/server";
import { resendVerificationEmail } from "@/lib/server/supabase-auth";
import { consumeRateLimit } from "@/lib/server/rate-limit";
import { ensureSameOriginRequest } from "@/lib/server/request-security";

type ResendBody = {
  email?: string;
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

  const rateLimit = consumeRateLimit(request, "auth_resend_verification", 5, 60 * 60 * 1000);
  if (!rateLimit.allowed) {
    const response = NextResponse.json(
      { error: "Too many verification requests. Try again later." },
      { status: 429 },
    );
    response.headers.set("Retry-After", String(rateLimit.retryAfterSeconds));
    return response;
  }

  let body: ResendBody;
  try {
    body = (await request.json()) as ResendBody;
  } catch {
    return NextResponse.json({ error: "Invalid request body." }, { status: 400 });
  }

  const email = (body.email || "").trim();
  if (!email) {
    return NextResponse.json({ error: "Email is required." }, { status: 400 });
  }

  const nextPath = safeNextPath(body.nextPath);
  const emailRedirectTo = `${request.nextUrl.origin}/auth/callback?next=${encodeURIComponent(nextPath)}`;
  const result = await resendVerificationEmail(email, emailRedirectTo);
  if (!result.ok) {
    return NextResponse.json({ error: result.error }, { status: result.status });
  }

  return NextResponse.json({ ok: true });
}
