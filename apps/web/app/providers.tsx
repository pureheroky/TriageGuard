"use client";

import { MutationCache, QueryCache, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { Toaster } from "@/components/ui/toaster";
import { toFriendlyErrorMessage } from "@/lib/error-messages";
import { toastError } from "@/lib/stores/toast-store";

const WORKSPACE_NOT_CONNECTED_MESSAGE = "Workspace is not connected yet. Complete Slack onboarding first.";
const WORKSPACE_NOT_CONNECTED_TOAST_SESSION_KEY = "tg.toast.workspace_not_connected.shown";

function shouldShowErrorToast(message: string): boolean {
  if (message !== WORKSPACE_NOT_CONNECTED_MESSAGE) {
    return true;
  }
  if (typeof window === "undefined") {
    return true;
  }
  try {
    if (window.sessionStorage.getItem(WORKSPACE_NOT_CONNECTED_TOAST_SESSION_KEY) === "1") {
      return false;
    }
    window.sessionStorage.setItem(WORKSPACE_NOT_CONNECTED_TOAST_SESSION_KEY, "1");
    return true;
  } catch {
    // If storage is unavailable, keep default behavior.
    return true;
  }
}

function reportClientError(source: string, message: string, stack = "") {
  const payload = {
    source,
    message,
    stack: stack.slice(0, 8000),
    pathname: typeof window !== "undefined" ? window.location.pathname : "",
  };

  fetch("/api/telemetry/error", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(payload),
    keepalive: true,
  }).catch(() => {
    // No-op by design: error reporting must not affect UI behavior.
  });
}

export function Providers({ children }: { children: React.ReactNode }) {
  const [queryClient] = useState(
    () =>
      new QueryClient({
        queryCache: new QueryCache({
          onError: (error) => {
            const message = toFriendlyErrorMessage(error);
            if (!shouldShowErrorToast(message)) {
              return;
            }
            toastError("Request failed", message);
          },
        }),
        mutationCache: new MutationCache({
          onError: (error) => {
            const message = toFriendlyErrorMessage(error);
            if (!shouldShowErrorToast(message)) {
              return;
            }
            toastError("Action failed", message);
          },
        }),
        defaultOptions: {
          queries: {
            staleTime: 15_000,
            refetchOnWindowFocus: false,
            retry: 1,
          },
        },
      }),
  );

  useEffect(() => {
    const handleError = (event: ErrorEvent) => {
      const message = event.message || "Unhandled error";
      reportClientError("window.error", message, event.error?.stack || "");
    };
    const handleRejection = (event: PromiseRejectionEvent) => {
      const reason = event.reason as { message?: string; stack?: string } | string | undefined;
      const message = typeof reason === "string" ? reason : reason?.message || "Unhandled promise rejection";
      const stack = typeof reason === "string" ? "" : reason?.stack || "";
      reportClientError("window.unhandledrejection", message, stack);
    };

    window.addEventListener("error", handleError);
    window.addEventListener("unhandledrejection", handleRejection);
    return () => {
      window.removeEventListener("error", handleError);
      window.removeEventListener("unhandledrejection", handleRejection);
    };
  }, []);

  return (
    <QueryClientProvider client={queryClient}>
      {children}
      <Toaster />
    </QueryClientProvider>
  );
}
