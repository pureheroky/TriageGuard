import { NextRequest, NextResponse } from "next/server";
import {
  clearSessionCookies,
  ensureAuthenticatedRequest,
  forceRefreshAccessToken,
  mergeResponseCookies,
} from "@/lib/server/supabase-auth";

const API_BASE_URL = (process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080").replace(/\/$/, "");

export async function GET(request: NextRequest) {
  const cookieCarrier = new NextResponse(null);
  const auth = await ensureAuthenticatedRequest(request, cookieCarrier);
  if (!auth) {
    const response = NextResponse.json({ error: "Unauthorized" }, { status: 401 });
    mergeResponseCookies(cookieCarrier, response);
    return response;
  }

  // Validate that token is accepted by our Go API as well.
  const fetchApiMe = (accessToken: string) =>
    fetch(`${API_BASE_URL}/api/me`, {
      method: "GET",
      headers: {
        Authorization: `Bearer ${accessToken}`,
      },
      cache: "no-store",
    });

  let upstream = await fetchApiMe(auth.accessToken);

  if (upstream.status === 401 || upstream.status === 403) {
    const refreshedAccessToken = await forceRefreshAccessToken(request, cookieCarrier);
    if (refreshedAccessToken) {
      upstream = await fetchApiMe(refreshedAccessToken);
    }
  }

  if (upstream.status === 401 || upstream.status === 403) {
    const response = NextResponse.json({ error: "Unauthorized" }, { status: 401 });
    clearSessionCookies(response);
    mergeResponseCookies(cookieCarrier, response);
    return response;
  }

  if (upstream.status === 404) {
    const response = NextResponse.json({
      user: auth.user,
      workspace: null,
      onboarding_required: true,
    });
    mergeResponseCookies(cookieCarrier, response);
    return response;
  }

  if (!upstream.ok) {
    const response = NextResponse.json({ error: `Auth check failed (${upstream.status})` }, { status: 502 });
    mergeResponseCookies(cookieCarrier, response);
    return response;
  }

  const apiMe = (await upstream.json()) as Record<string, unknown>;
  const response = NextResponse.json({ user: auth.user, workspace: apiMe });
  mergeResponseCookies(cookieCarrier, response);
  return response;
}
