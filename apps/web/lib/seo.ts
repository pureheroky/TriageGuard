export const siteName = "TriageGuard";
export const siteDescription =
  "Keep internal requests from getting lost with queue health, unacked and unassigned visibility, SLA risk, and waiting vs forgotten control.";

export function getSiteUrl(): string {
  const envUrl = process.env.NEXT_PUBLIC_SITE_URL?.trim();
  if (!envUrl) {
    return "https://triageguard.app";
  }

  return envUrl.replace(/\/+$/, "");
}
