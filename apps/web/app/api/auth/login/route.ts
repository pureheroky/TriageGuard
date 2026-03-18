import { NextRequest, NextResponse } from "next/server";
import { setSessionCookies, signInWithPassword } from "@/lib/server/supabase-auth";
import { consumeRateLimit } from "@/lib/server/rate-limit";
import { ensureSameOriginRequest } from "@/lib/server/request-security";

type LoginBody = {
  email?: string;
  password?: string;
};

export async function POST(request: NextRequest) {
  const originError = ensureSameOriginRequest(request);
  if (originError) {
    return originError;
  }

  const rateLimit = consumeRateLimit(request, "auth_login", 10, 10 * 60 * 1000);
  if (!rateLimit.allowed) {
    const response = NextResponse.json(
      { error: "Too many login attempts. Try again in a few minutes." },
      { status: 429 },
    );
    response.headers.set("Retry-After", String(rateLimit.retryAfterSeconds));
    return response;
  }

  let body: LoginBody;
  try {
    body = (await request.json()) as LoginBody;
  } catch {
    return NextResponse.json({ error: "Invalid request body." }, { status: 400 });
  }

  const email = (body.email || "").trim();
  const password = body.password || "";
  if (!email || !password) {
    return NextResponse.json({ error: "Email and password are required." }, { status: 400 });
  }

  const result = await signInWithPassword(email, password);
  if (!result.ok) {
    return NextResponse.json({ error: result.error }, { status: result.status });
  }

  const response = NextResponse.json({ user: result.data.user });
  setSessionCookies(response, result.data.tokens);
  return response;
}
