type SentryTarget = {
  endpoint: string;
  publicKey: string;
  secretKey: string;
};

function parseSentryDSN(raw: string | undefined): SentryTarget | null {
  const dsn = (raw || "").trim();
  if (!dsn) {
    return null;
  }

  try {
    const url = new URL(dsn);
    const publicKey = (url.username || "").trim();
    const secretKey = (url.password || "").trim();
    const path = url.pathname.replace(/^\/+|\/+$/g, "");
    if (!publicKey || !path) {
      return null;
    }

    const pathParts = path.split("/");
    const projectId = pathParts[pathParts.length - 1];
    const prefix = pathParts.slice(0, -1).join("/");
    const endpointPath = `${prefix ? `/${prefix}` : ""}/api/${projectId}/store/`;

    return {
      endpoint: `${url.protocol}//${url.host}${endpointPath}`,
      publicKey,
      secretKey,
    };
  } catch {
    return null;
  }
}

function randomEventID(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return Array.from(bytes)
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}

function sentryAuthHeader(target: SentryTarget): string {
  if (!target.secretKey) {
    return `Sentry sentry_version=7, sentry_client=triageguard-web/1.0, sentry_key=${target.publicKey}`;
  }
  return `Sentry sentry_version=7, sentry_client=triageguard-web/1.0, sentry_key=${target.publicKey}, sentry_secret=${target.secretKey}`;
}

export async function sendSentryServerEvent(
  message: string,
  level: "error" | "warning" | "info",
  extras?: Record<string, unknown>,
): Promise<void> {
  const target = parseSentryDSN(process.env.SENTRY_DSN);
  if (!target) {
    return;
  }

  const payload = {
    event_id: randomEventID(),
    timestamp: new Date().toISOString(),
    platform: "javascript",
    logger: "triageguard-web",
    level,
    message,
    extra: extras || {},
  };

  const response = await fetch(target.endpoint, {
    method: "POST",
    headers: {
      "content-type": "application/json",
      "x-sentry-auth": sentryAuthHeader(target),
    },
    body: JSON.stringify(payload),
    cache: "no-store",
  });
  if (!response.ok) {
    throw new Error(`Sentry store failed: ${response.status}`);
  }
}
