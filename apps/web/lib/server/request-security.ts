import { NextRequest, NextResponse } from "next/server";

const SAFE_METHODS = new Set(["GET", "HEAD", "OPTIONS"]);
const ALLOWED_FETCH_SITES = new Set(["same-origin", "same-site", "none"]);

function badRequest(message: string) {
  return NextResponse.json({ error: message }, { status: 403 });
}

function originOf(urlValue: string): string | null {
  try {
    return new URL(urlValue).origin;
  } catch {
    return null;
  }
}

export function ensureSameOriginRequest(request: NextRequest): NextResponse | null {
  const method = request.method.toUpperCase();
  if (SAFE_METHODS.has(method)) {
    return null;
  }

  const expectedOrigin = request.nextUrl.origin;
  const origin = request.headers.get("origin");
  const referer = request.headers.get("referer");
  const fetchSite = (request.headers.get("sec-fetch-site") || "").toLowerCase();

  if (fetchSite && !ALLOWED_FETCH_SITES.has(fetchSite)) {
    return badRequest("Cross-site request blocked.");
  }

  if (origin) {
    if (origin !== expectedOrigin) {
      return badRequest("Origin mismatch.");
    }
    return null;
  }

  if (referer) {
    const refererOrigin = originOf(referer);
    if (!refererOrigin || refererOrigin !== expectedOrigin) {
      return badRequest("Referer mismatch.");
    }
  }

  return null;
}
