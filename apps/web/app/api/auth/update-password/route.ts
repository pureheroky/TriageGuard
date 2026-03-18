import { NextRequest, NextResponse } from "next/server";
import { ensureAuthenticatedRequest, mergeResponseCookies, updatePassword } from "@/lib/server/supabase-auth";
import { validatePasswordPolicy } from "@/lib/password-policy";
import { ensureSameOriginRequest } from "@/lib/server/request-security";

type UpdatePasswordBody = {
  password?: string;
};

export async function POST(request: NextRequest) {
  const originError = ensureSameOriginRequest(request);
  if (originError) {
    return originError;
  }

  const cookieCarrier = new NextResponse(null);
  const auth = await ensureAuthenticatedRequest(request, cookieCarrier);
  if (!auth) {
    const response = NextResponse.json({ error: "Unauthorized" }, { status: 401 });
    mergeResponseCookies(cookieCarrier, response);
    return response;
  }

  let body: UpdatePasswordBody;
  try {
    body = (await request.json()) as UpdatePasswordBody;
  } catch {
    const response = NextResponse.json({ error: "Invalid request body." }, { status: 400 });
    mergeResponseCookies(cookieCarrier, response);
    return response;
  }

  const password = body.password || "";
  if (!password) {
    const response = NextResponse.json({ error: "Password is required." }, { status: 400 });
    mergeResponseCookies(cookieCarrier, response);
    return response;
  }
  const policyError = validatePasswordPolicy(password);
  if (policyError) {
    const response = NextResponse.json({ error: policyError }, { status: 400 });
    mergeResponseCookies(cookieCarrier, response);
    return response;
  }

  const updated = await updatePassword(auth.accessToken, password);
  if (!updated.ok) {
    const response = NextResponse.json({ error: updated.error }, { status: updated.status });
    mergeResponseCookies(cookieCarrier, response);
    return response;
  }

  const response = NextResponse.json({ ok: true });
  mergeResponseCookies(cookieCarrier, response);
  return response;
}
