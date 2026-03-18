export function extractErrorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  if (typeof error === "string") {
    return error;
  }
  if (error && typeof error === "object") {
    const candidate = error as Record<string, unknown>;
    if (typeof candidate.message === "string") {
      return candidate.message;
    }
    if (typeof candidate.error === "string") {
      return candidate.error;
    }
  }
  return "Unexpected error.";
}

export function toFriendlyErrorMessage(error: unknown): string {
  const message = extractErrorMessage(error).trim();
  const lower = message.toLowerCase();

  if (!message) {
    return "Unexpected error.";
  }
  if (lower.includes("failed to fetch") || lower.includes("network error")) {
    return "Could not connect to server. Check that API is running and reachable.";
  }
  if (lower.includes("missing bearer token") || lower.includes("invalid token") || lower.includes("unauthorized")) {
    return "Your session expired. Please sign in again.";
  }
  if (lower.includes("workspace not found")) {
    return "Workspace is not connected yet. Complete Slack onboarding first.";
  }
  if (lower.includes("slack is not connected")) {
    return "Slack is not connected. Connect Slack in onboarding first.";
  }
  if (lower.includes("linear auth required") || lower.includes("authorization expired") || lower.includes("reconnect linear")) {
    return "Linear authorization expired. Reconnect Linear in onboarding and retry.";
  }
  if (lower.includes("plan supports up to")) {
    return message;
  }
  if (lower.includes("active paid subscription required") || lower.includes("activate team or enterprise plan")) {
    return "Active Team or Enterprise subscription required. Open Billing to continue.";
  }
  if (lower.includes("billing is not configured")) {
    return "Billing is temporarily unavailable. Please contact support.";
  }
  if (lower.includes("no stripe customer found")) {
    return "No active Stripe customer for this workspace yet. Complete checkout first.";
  }
  if (lower.includes("billing portal is unavailable for paypal")) {
    return "PayPal mode does not support a billing portal yet.";
  }
  if (lower.includes("paypal confirmation is available only in paypal billing mode")) {
    return "PayPal confirmation is disabled because billing provider is not PayPal.";
  }
  if (lower.includes("paypal confirmation supports only team or enterprise plan")) {
    return "PayPal confirmation supports only Team or Enterprise.";
  }
  if (lower.includes("enterprise plan required for per-channel sla overrides")) {
    return "Per-channel SLA overrides are available on Enterprise plan.";
  }
  if (lower.includes("enterprise plan required for csv exports")) {
    return "CSV exports are available on Enterprise plan.";
  }
  if (lower.includes("invalid daily_digest_time")) {
    return "Digest time format is invalid. Use HH:MM or HH:MM:SS.";
  }
  if (lower.includes("no supabase session")) {
    return "Your session expired. Please sign in again.";
  }
  if (lower.includes("confirm must be delete")) {
    return "Type DELETE to confirm workspace deletion.";
  }
  return message;
}

export function toFriendlyApiMessage(status: number, rawMessage: string): string {
  const base = rawMessage.trim() || `Request failed (${status})`;
  const lower = base.toLowerCase();

  if (status === 401 || status === 403) {
    return "Your session expired. Please sign in again.";
  }
  if (status === 402) {
    return toFriendlyErrorMessage(base);
  }
  if (status >= 500) {
    return "Server error. Please try again in a moment.";
  }
  if (lower.includes("workspace not found")) {
    return "Workspace is not connected yet. Complete Slack onboarding first.";
  }
  return toFriendlyErrorMessage(base);
}
