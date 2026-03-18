export const siteName = "TriageGuard";
export const siteDescription =
  "Turn Slack messages into trackable requests with ownership, priority, due dates, and SLA timers.";

export function getSiteUrl(): string {
  const envUrl = process.env.NEXT_PUBLIC_SITE_URL?.trim();
  if (!envUrl) {
    return "https://triageguard.app";
  }

  return envUrl.replace(/\/+$/, "");
}
