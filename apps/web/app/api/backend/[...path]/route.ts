import { NextRequest, NextResponse } from "next/server";
import {
  clearSessionCookies,
  ensureAccessToken,
  forceRefreshAccessToken,
  mergeResponseCookies,
} from "@/lib/server/supabase-auth";
import { ensureSameOriginRequest } from "@/lib/server/request-security";
import { sendSentryServerEvent } from "@/lib/server/sentry-lite";

const API_BASE_URL = (process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080").replace(/\/$/, "");

type RouteContext = {
  params: {
    path: string[];
  };
};

function buildUpstreamURL(request: NextRequest, path: string[]): string {
  const joined = path.join("/");
  const upstream = new URL(`${API_BASE_URL}/${joined}`);
  upstream.search = request.nextUrl.search;
  return upstream.toString();
}

function copyResponseHeaders(from: Response, to: NextResponse) {
  const allow = ["content-type", "cache-control", "location", "etag", "content-disposition"];
  for (const [key, value] of from.headers.entries()) {
    if (allow.includes(key.toLowerCase())) {
      to.headers.set(key, value);
    }
  }
  const getSetCookie = (from.headers as unknown as { getSetCookie?: () => string[] }).getSetCookie;
  if (typeof getSetCookie === "function") {
    const cookies = getSetCookie.call(from.headers) || [];
    for (const cookie of cookies) {
      to.headers.append("set-cookie", cookie);
    }
    return;
  }
  const setCookie = from.headers.get("set-cookie");
  if (setCookie) {
    to.headers.append("set-cookie", setCookie);
  }
}

async function proxy(request: NextRequest, context: RouteContext) {
  if (!context.params.path?.length) {
    return NextResponse.json({ error: "Invalid path" }, { status: 404 });
  }

  const originError = ensureSameOriginRequest(request);
  if (originError) {
    return originError;
  }

  const cookieCarrier = new NextResponse(null);
  const accessToken = await ensureAccessToken(request, cookieCarrier);
  if (!accessToken) {
    const unauthorized = NextResponse.json({ error: "Unauthorized" }, { status: 401 });
    mergeResponseCookies(cookieCarrier, unauthorized);
    return unauthorized;
  }

  const upstreamHeaders = new Headers(request.headers);
  upstreamHeaders.delete("host");
  upstreamHeaders.delete("cookie");
  upstreamHeaders.delete("authorization");
  upstreamHeaders.delete("content-length");
  // Prevent client-controlled proxy headers from affecting backend IP/trust logic.
  upstreamHeaders.delete("x-forwarded-for");
  upstreamHeaders.delete("x-real-ip");
  upstreamHeaders.delete("x-forwarded-host");
  upstreamHeaders.delete("x-forwarded-proto");
  upstreamHeaders.delete("forwarded");
  upstreamHeaders.delete("via");
  upstreamHeaders.set("Authorization", `Bearer ${accessToken}`);

  let body: string | undefined;
  if (request.method !== "GET" && request.method !== "HEAD") {
    body = await request.text();
  }

  const upstreamURL = buildUpstreamURL(request, context.params.path);
  const fetchUpstream = async (): Promise<Response> => {
    const abortController = new AbortController();
    const timeout = setTimeout(() => abortController.abort(), 15_000);
    try {
      return await fetch(upstreamURL, {
        method: request.method,
        headers: upstreamHeaders,
        body,
        cache: "no-store",
        redirect: "manual",
        signal: abortController.signal,
      });
    } finally {
      clearTimeout(timeout);
    }
  };

  let upstreamResponse: Response;
  try {
    upstreamResponse = await fetchUpstream();
  } catch {
    void sendSentryServerEvent("backend proxy unreachable", "error", {
      upstream: upstreamURL,
      method: request.method,
    });
    const response = NextResponse.json(
      { error: "Backend is temporarily unavailable. Please retry." },
      { status: 502 },
    );
    mergeResponseCookies(cookieCarrier, response);
    return response;
  }

  if (upstreamResponse.status === 401) {
    const refreshedAccessToken = await forceRefreshAccessToken(request, cookieCarrier);
    if (refreshedAccessToken) {
      upstreamHeaders.set("Authorization", `Bearer ${refreshedAccessToken}`);
      try {
        upstreamResponse = await fetchUpstream();
      } catch {
        void sendSentryServerEvent("backend proxy retry unreachable", "error", {
          upstream: upstreamURL,
          method: request.method,
        });
        const response = NextResponse.json(
          { error: "Backend is temporarily unavailable. Please retry." },
          { status: 502 },
        );
        mergeResponseCookies(cookieCarrier, response);
        return response;
      }
    }
  }

  const redirectLocation = upstreamResponse.headers.get("location");
  if (redirectLocation && upstreamResponse.status >= 300 && upstreamResponse.status < 400) {
    const redirectResponse = new NextResponse(null, { status: upstreamResponse.status });
    copyResponseHeaders(upstreamResponse, redirectResponse);
    mergeResponseCookies(cookieCarrier, redirectResponse);
    return redirectResponse;
  }

  const upstreamBody = await upstreamResponse.arrayBuffer();
  const response = new NextResponse(upstreamBody, {
    status: upstreamResponse.status,
  });
  if (upstreamResponse.status >= 500) {
    void sendSentryServerEvent("backend proxy 5xx", "error", {
      upstream: buildUpstreamURL(request, context.params.path),
      method: request.method,
      status: upstreamResponse.status,
    });
  }
  copyResponseHeaders(upstreamResponse, response);
  mergeResponseCookies(cookieCarrier, response);

  if (upstreamResponse.status === 401) {
    clearSessionCookies(response);
  }

  return response;
}

export async function GET(request: NextRequest, context: RouteContext) {
  return proxy(request, context);
}

export async function POST(request: NextRequest, context: RouteContext) {
  return proxy(request, context);
}

export async function PUT(request: NextRequest, context: RouteContext) {
  return proxy(request, context);
}

export async function PATCH(request: NextRequest, context: RouteContext) {
  return proxy(request, context);
}

export async function DELETE(request: NextRequest, context: RouteContext) {
  return proxy(request, context);
}
