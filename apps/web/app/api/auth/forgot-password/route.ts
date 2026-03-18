import { NextRequest, NextResponse } from "next/server";
import { sendPasswordRecovery } from "@/lib/server/supabase-auth";
import { consumeRateLimit } from "@/lib/server/rate-limit";
import { ensureSameOriginRequest } from "@/lib/server/request-security";

type ForgotPasswordBody = {
  email?: string;
};

export async function POST(request: NextRequest) {
  const originError = ensureSameOriginRequest(request);
  if (originError) {
    return originError;
  }

  const rateLimit = consumeRateLimit(request, "auth_forgot_password", 5, 60 * 60 * 1000);
  if (!rateLimit.allowed) {
    const response = NextResponse.json(
      { error: "Too many reset requests. Try again later." },
      { status: 429 },
    );
    response.headers.set("Retry-After", String(rateLimit.retryAfterSeconds));
    return response;
  }

  let body: ForgotPasswordBody;
  try {
    body = (await request.json()) as ForgotPasswordBody;
  } catch {
    return NextResponse.json({ error: "Invalid request body." }, { status: 400 });
  }

  const email = (body.email || "").trim();
  if (!email) {
    return NextResponse.json({ error: "Email is required." }, { status: 400 });
  }

  const redirectTo = `${request.nextUrl.origin}/auth/reset-password`;
  const result = await sendPasswordRecovery(email, redirectTo);
  if (!result.ok) {
    return NextResponse.json({ error: result.error }, { status: result.status });
  }

  return NextResponse.json({ ok: true });
}
