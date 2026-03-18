import { NextRequest, NextResponse } from "next/server";
import { consumeRateLimit } from "@/lib/server/rate-limit";
import { ensureSameOriginRequest } from "@/lib/server/request-security";
import { sendSentryServerEvent } from "@/lib/server/sentry-lite";

type ClientErrorPayload = {
  message?: string;
  stack?: string;
  pathname?: string;
  source?: string;
};

export async function POST(request: NextRequest) {
  const originError = ensureSameOriginRequest(request);
  if (originError) {
    return originError;
  }

  const rateLimit = consumeRateLimit(request, "telemetry_error", 30, 10 * 60 * 1000);
  if (!rateLimit.allowed) {
    const response = NextResponse.json({ error: "Rate limited" }, { status: 429 });
    response.headers.set("Retry-After", String(rateLimit.retryAfterSeconds));
    return response;
  }

  let body: ClientErrorPayload;
  try {
    body = (await request.json()) as ClientErrorPayload;
  } catch {
    return NextResponse.json({ error: "Invalid request body." }, { status: 400 });
  }

  const message = (body.message || "").trim();
  if (!message) {
    return NextResponse.json({ error: "message is required" }, { status: 400 });
  }

  try {
    await sendSentryServerEvent("frontend error", "error", {
      source: (body.source || "web").trim(),
      message,
      stack: (body.stack || "").trim().slice(0, 8000),
      pathname: (body.pathname || "").trim(),
      user_agent: request.headers.get("user-agent") || "",
    });
  } catch (error) {
    console.error("telemetry:error send failed", error);
    return NextResponse.json({ ok: false }, { status: 202 });
  }

  return NextResponse.json({ ok: true }, { status: 202 });
}
