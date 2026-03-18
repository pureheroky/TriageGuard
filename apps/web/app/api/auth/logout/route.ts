import { NextRequest, NextResponse } from "next/server";
import { clearSessionCookies } from "@/lib/server/supabase-auth";
import { ensureSameOriginRequest } from "@/lib/server/request-security";

export async function POST(request: NextRequest) {
  const originError = ensureSameOriginRequest(request);
  if (originError) {
    return originError;
  }

  const response = NextResponse.json({ ok: true });
  clearSessionCookies(response);
  return response;
}
