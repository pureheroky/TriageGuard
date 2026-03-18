"use client";

import { useEffect } from "react";

type GlobalErrorProps = {
  error: Error & { digest?: string };
  reset: () => void;
};

function reportGlobalError(error: Error & { digest?: string }) {
  const payload = {
    source: "global-error",
    message: error.message || "Unknown client error",
    stack: `${error.stack || ""}\n\ndigest=${error.digest || ""}`.trim(),
    pathname: typeof window !== "undefined" ? window.location.pathname : "",
  };

  fetch("/api/telemetry/error", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(payload),
    keepalive: true,
  }).catch(() => {
    // Avoid recursive failures in global error boundary.
  });
}

export default function GlobalError({ error, reset }: GlobalErrorProps) {
  useEffect(() => {
    reportGlobalError(error);
  }, [error]);

  return (
    <html lang="en">
      <body className="min-h-screen bg-background text-foreground">
        <main className="mx-auto flex min-h-screen w-full max-w-xl flex-col items-center justify-center gap-4 px-6 text-center">
          <h1 className="text-2xl font-semibold">Something went wrong</h1>
          <p className="text-sm text-muted-foreground">
            The incident was recorded. Try reloading this page.
          </p>
          <button
            type="button"
            className="rounded-md border px-4 py-2 text-sm font-medium hover:bg-accent"
            onClick={reset}
          >
            Reload
          </button>
        </main>
      </body>
    </html>
  );
}
