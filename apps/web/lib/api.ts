"use client";

import { toFriendlyApiMessage } from "./error-messages";

const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080";

function redirectToLogin(): never {
  if (typeof window !== "undefined") {
    const nextPath = `${window.location.pathname}${window.location.search}`;
    const safeNext = nextPath.startsWith("/") ? nextPath : "/billing";
    window.location.replace(`/login?next=${encodeURIComponent(safeNext)}`);
  }
  throw new Error("Unauthorized");
}

async function parseResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    let rawMessage = "";
    const contentType = response.headers.get("content-type") || "";

    if (contentType.includes("application/json")) {
      try {
        const body = (await response.json()) as Record<string, unknown>;
        if (typeof body.error === "string") {
          rawMessage = body.error;
        } else if (typeof body.message === "string") {
          rawMessage = body.message;
        } else {
          rawMessage = JSON.stringify(body);
        }
      } catch {
        rawMessage = "";
      }
    } else {
      rawMessage = (await response.text()).trim();
    }

    throw new Error(toFriendlyApiMessage(response.status, rawMessage));
  }
  return (await response.json()) as T;
}

export async function apiFetch<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers || {});
  if (!headers.has("Content-Type") && init.body) {
    headers.set("Content-Type", "application/json");
  }

  const response = await fetch(`/api/backend${path}`, {
    ...init,
    headers,
    cache: "no-store",
  });

  if (response.status === 401) {
    return redirectToLogin();
  }

  return parseResponse<T>(response);
}

export function apiBaseURL() {
  return API_BASE_URL;
}
