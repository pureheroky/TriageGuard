import { NextRequest, NextResponse } from "next/server";
import { buildSessionTokensFromRaw, setSessionCookies } from "@/lib/server/supabase-auth";
import { ensureSameOriginRequest } from "@/lib/server/request-security";

type SetSessionBody = {
  access_token?: string;
  refresh_token?: string;
  expires_at?: number;
  expires_in?: number;
};

export async function POST(request: NextRequest) {
  const originError = ensureSameOriginRequest(request);
  if (originError) {
    return originError;
  }

  let body: SetSessionBody;
  try {
    body = (await request.json()) as SetSessionBody;
  } catch {
    return NextResponse.json({ error: "Invalid request body." }, { status: 400 });
  }

  if (!body.access_token || !body.refresh_token) {
    return NextResponse.json({ error: "Missing session tokens." }, { status: 400 });
  }

  const tokens = buildSessionTokensFromRaw({
    access_token: body.access_token,
    refresh_token: body.refresh_token,
    expires_at: body.expires_at,
    expires_in: body.expires_in,
  });

  const response = NextResponse.json({ ok: true });
  setSessionCookies(response, tokens);
  return response;
}
