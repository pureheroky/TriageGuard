import { NextRequest, NextResponse } from "next/server";

const ACCESS_COOKIE = "tg_at";
const REFRESH_COOKIE = "tg_rt";
const EXPIRES_COOKIE = "tg_exp";
const REFRESH_LEEWAY_SECONDS = 30;
const REFRESH_COOKIE_MAX_AGE_SECONDS = 60 * 60 * 24 * 30;

type SupabaseErrorBody = {
  error?: string;
  error_description?: string;
  msg?: string;
  message?: string;
};

type TokenPayload = {
  access_token?: string;
  refresh_token?: string;
  expires_at?: number;
  expires_in?: number;
  token_type?: string;
  user?: Record<string, unknown>;
};

export type SessionTokens = {
  accessToken: string;
  refreshToken: string;
  expiresAt: number;
};

export type SessionIdentity = {
  sub?: string;
  email?: string;
  role?: string;
  exp?: number;
  [key: string]: unknown;
};

type ApiResult<T> = {
  ok: true;
  status: number;
  data: T;
} | {
  ok: false;
  status: number;
  error: string;
};

function supabaseUrl(): string {
  const value = process.env.NEXT_PUBLIC_SUPABASE_URL;
  if (!value) {
    throw new Error("NEXT_PUBLIC_SUPABASE_URL is required");
  }
  return value.replace(/\/$/, "");
}

function supabaseAnonKey(): string {
  const value = process.env.NEXT_PUBLIC_SUPABASE_ANON_KEY;
  if (!value) {
    throw new Error("NEXT_PUBLIC_SUPABASE_ANON_KEY is required");
  }
  return value;
}

function authEndpoint(path: string): string {
  return `${supabaseUrl()}/auth/v1${path}`;
}

function authHeaders(extra?: HeadersInit): Headers {
  const headers = new Headers(extra || {});
  headers.set("apikey", supabaseAnonKey());
  headers.set("Content-Type", "application/json");
  return headers;
}

function secureCookies(): boolean {
  return process.env.NODE_ENV === "production";
}

function computeExpiresAt(payload: TokenPayload): number {
  if (typeof payload.expires_at === "number" && payload.expires_at > 0) {
    return payload.expires_at;
  }
  if (typeof payload.expires_in === "number" && payload.expires_in > 0) {
    return Math.floor(Date.now() / 1000) + payload.expires_in;
  }
  return Math.floor(Date.now() / 1000) + 3600;
}

function extractTokens(payload: TokenPayload): SessionTokens | null {
  if (!payload.access_token || !payload.refresh_token) {
    return null;
  }
  return {
    accessToken: payload.access_token,
    refreshToken: payload.refresh_token,
    expiresAt: computeExpiresAt(payload),
  };
}

function isExpiringSoon(expiresAt: number): boolean {
  const now = Math.floor(Date.now() / 1000);
  return expiresAt <= now + REFRESH_LEEWAY_SECONDS;
}

async function parseErrorText(response: Response): Promise<string> {
  let fallback = `Supabase auth error (${response.status})`;
  try {
    const body = (await response.json()) as SupabaseErrorBody;
    fallback = body.message || body.error_description || body.msg || body.error || fallback;
  } catch {
    // Ignore parse failures and return fallback.
  }
  return fallback;
}

async function postAuth<T>(path: string, body: Record<string, unknown>, bearer?: string): Promise<ApiResult<T>> {
  const headers = authHeaders();
  if (bearer) {
    headers.set("Authorization", `Bearer ${bearer}`);
  }

  const response = await fetch(authEndpoint(path), {
    method: "POST",
    headers,
    body: JSON.stringify(body),
    cache: "no-store",
  });

  if (!response.ok) {
    return {
      ok: false,
      status: response.status,
      error: await parseErrorText(response),
    };
  }

  return {
    ok: true,
    status: response.status,
    data: (await response.json()) as T,
  };
}

async function putAuth<T>(path: string, body: Record<string, unknown>, bearer: string): Promise<ApiResult<T>> {
  const headers = authHeaders();
  headers.set("Authorization", `Bearer ${bearer}`);

  const response = await fetch(authEndpoint(path), {
    method: "PUT",
    headers,
    body: JSON.stringify(body),
    cache: "no-store",
  });

  if (!response.ok) {
    return {
      ok: false,
      status: response.status,
      error: await parseErrorText(response),
    };
  }

  return {
    ok: true,
    status: response.status,
    data: (await response.json()) as T,
  };
}

function readTokens(request: NextRequest): SessionTokens | null {
  const accessToken = request.cookies.get(ACCESS_COOKIE)?.value || "";
  const refreshToken = request.cookies.get(REFRESH_COOKIE)?.value || "";
  const expiresRaw = request.cookies.get(EXPIRES_COOKIE)?.value || "";
  const expiresAt = Number(expiresRaw);

  if (!refreshToken || !Number.isFinite(expiresAt)) {
    return null;
  }

  return { accessToken, refreshToken, expiresAt };
}

export function setSessionCookies(response: NextResponse, tokens: SessionTokens) {
  const now = Math.floor(Date.now() / 1000);
  const accessMaxAge = Math.max(0, tokens.expiresAt - now);

  response.cookies.set({
    name: ACCESS_COOKIE,
    value: tokens.accessToken,
    httpOnly: true,
    secure: secureCookies(),
    sameSite: "lax",
    path: "/",
    maxAge: accessMaxAge,
  });
  response.cookies.set({
    name: REFRESH_COOKIE,
    value: tokens.refreshToken,
    httpOnly: true,
    secure: secureCookies(),
    sameSite: "lax",
    path: "/",
    maxAge: REFRESH_COOKIE_MAX_AGE_SECONDS,
  });
  response.cookies.set({
    name: EXPIRES_COOKIE,
    value: String(tokens.expiresAt),
    httpOnly: true,
    secure: secureCookies(),
    sameSite: "lax",
    path: "/",
    maxAge: REFRESH_COOKIE_MAX_AGE_SECONDS,
  });
}

export function clearSessionCookies(response: NextResponse) {
  for (const name of [ACCESS_COOKIE, REFRESH_COOKIE, EXPIRES_COOKIE]) {
    response.cookies.set({
      name,
      value: "",
      httpOnly: true,
      secure: secureCookies(),
      sameSite: "lax",
      path: "/",
      maxAge: 0,
    });
  }
}

export function mergeResponseCookies(from: NextResponse, to: NextResponse) {
  for (const cookie of from.cookies.getAll()) {
    to.cookies.set(cookie);
  }
}

export async function signInWithPassword(email: string, password: string): Promise<ApiResult<{ user: Record<string, unknown>; tokens: SessionTokens }>> {
  const result = await postAuth<TokenPayload>("/token?grant_type=password", { email, password });
  if (!result.ok) {
    return result;
  }

  const tokens = extractTokens(result.data);
  if (!tokens || !result.data.user) {
    return {
      ok: false,
      status: 401,
      error: "Invalid login response from Supabase.",
    };
  }

  return {
    ok: true,
    status: 200,
    data: {
      user: result.data.user,
      tokens,
    },
  };
}

export async function signUpWithPassword(
  email: string,
  password: string,
  emailRedirectTo: string,
): Promise<ApiResult<{ user: Record<string, unknown> | null; tokens: SessionTokens | null }>> {
  const result = await postAuth<TokenPayload>("/signup", {
    email,
    password,
    email_redirect_to: emailRedirectTo,
  });

  if (!result.ok) {
    return result;
  }

  return {
    ok: true,
    status: result.status,
    data: {
      user: result.data.user || null,
      tokens: extractTokens(result.data),
    },
  };
}

export async function resendVerificationEmail(email: string, emailRedirectTo: string): Promise<ApiResult<Record<string, unknown>>> {
  return postAuth<Record<string, unknown>>("/resend", {
    type: "signup",
    email,
    email_redirect_to: emailRedirectTo,
  });
}

export async function sendPasswordRecovery(email: string, redirectTo: string): Promise<ApiResult<Record<string, unknown>>> {
  return postAuth<Record<string, unknown>>("/recover", {
    email,
    redirect_to: redirectTo,
  });
}

export async function refreshSession(refreshToken: string): Promise<ApiResult<SessionTokens>> {
  const result = await postAuth<TokenPayload>("/token?grant_type=refresh_token", {
    refresh_token: refreshToken,
  });
  if (!result.ok) {
    return result;
  }

  const tokens = extractTokens(result.data);
  if (!tokens) {
    return {
      ok: false,
      status: 401,
      error: "Invalid refresh response from Supabase.",
    };
  }

  return {
    ok: true,
    status: 200,
    data: tokens,
  };
}

export async function updatePassword(accessToken: string, password: string): Promise<ApiResult<Record<string, unknown>>> {
  return putAuth<Record<string, unknown>>("/user", { password }, accessToken);
}

function decodeBase64Url(value: string): string | null {
  try {
    const normalized = value.replace(/-/g, "+").replace(/_/g, "/");
    const padded = normalized + "=".repeat((4 - (normalized.length % 4)) % 4);
    return Buffer.from(padded, "base64").toString("utf-8");
  } catch {
    return null;
  }
}

function parseIdentityFromAccessToken(accessToken: string): SessionIdentity {
  const parts = accessToken.split(".");
  if (parts.length < 2) {
    return {};
  }
  const payload = decodeBase64Url(parts[1]);
  if (!payload) {
    return {};
  }
  try {
    return JSON.parse(payload) as SessionIdentity;
  } catch {
    return {};
  }
}

export async function ensureAccessToken(
  request: NextRequest,
  cookieCarrier: NextResponse,
): Promise<string | null> {
  let tokens = readTokens(request);
  if (!tokens) {
    clearSessionCookies(cookieCarrier);
    return null;
  }

  if (!tokens.accessToken || isExpiringSoon(tokens.expiresAt)) {
    const refreshed = await refreshSession(tokens.refreshToken);
    if (!refreshed.ok) {
      clearSessionCookies(cookieCarrier);
      return null;
    }
    tokens = refreshed.data;
    setSessionCookies(cookieCarrier, tokens);
  }

  return tokens.accessToken;
}

export async function forceRefreshAccessToken(
  request: NextRequest,
  cookieCarrier: NextResponse,
): Promise<string | null> {
  const refreshToken = request.cookies.get(REFRESH_COOKIE)?.value || "";
  if (!refreshToken) {
    clearSessionCookies(cookieCarrier);
    return null;
  }

  const refreshed = await refreshSession(refreshToken);
  if (!refreshed.ok) {
    clearSessionCookies(cookieCarrier);
    return null;
  }

  setSessionCookies(cookieCarrier, refreshed.data);
  return refreshed.data.accessToken;
}

export async function ensureAuthenticatedRequest(
  request: NextRequest,
  cookieCarrier: NextResponse,
): Promise<{ accessToken: string; user: SessionIdentity } | null> {
  const accessToken = await ensureAccessToken(request, cookieCarrier);
  if (!accessToken) {
    return null;
  }
  return {
    accessToken,
    user: parseIdentityFromAccessToken(accessToken),
  };
}

export function buildSessionTokensFromRaw(raw: {
  access_token: string;
  refresh_token: string;
  expires_at?: number;
  expires_in?: number;
}): SessionTokens {
  return {
    accessToken: raw.access_token,
    refreshToken: raw.refresh_token,
    expiresAt:
      typeof raw.expires_at === "number" && raw.expires_at > 0
        ? raw.expires_at
        : Math.floor(Date.now() / 1000) + (raw.expires_in || 3600),
  };
}
