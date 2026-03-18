import { NextRequest } from "next/server";

type Bucket = {
  count: number;
  resetAt: number;
};

const buckets = new Map<string, Bucket>();
const MAX_BUCKETS = 20000;

function extractClientIP(request: NextRequest): string {
  const forwardedFor = request.headers.get("x-forwarded-for") || "";
  if (forwardedFor) {
    const first = forwardedFor.split(",")[0]?.trim();
    if (first) {
      return first;
    }
  }
  const realIP = request.headers.get("x-real-ip") || "";
  if (realIP.trim()) {
    return realIP.trim();
  }
  return "unknown";
}

function pruneBuckets(now: number) {
  if (buckets.size < MAX_BUCKETS) {
    return;
  }

  for (const [key, bucket] of buckets) {
    if (bucket.resetAt <= now) {
      buckets.delete(key);
    }
  }

  if (buckets.size <= MAX_BUCKETS) {
    return;
  }

  const removeCount = buckets.size - MAX_BUCKETS;
  let removed = 0;
  for (const key of buckets.keys()) {
    buckets.delete(key);
    removed += 1;
    if (removed >= removeCount) {
      break;
    }
  }
}

export function consumeRateLimit(
  request: NextRequest,
  scope: string,
  limit: number,
  windowMs: number,
): { allowed: true } | { allowed: false; retryAfterSeconds: number } {
  const now = Date.now();
  pruneBuckets(now);

  const key = `${scope}:${extractClientIP(request)}`;
  const bucket = buckets.get(key);

  if (!bucket || bucket.resetAt <= now) {
    buckets.set(key, {
      count: 1,
      resetAt: now + windowMs,
    });
    return { allowed: true };
  }

  if (bucket.count >= limit) {
    return {
      allowed: false,
      retryAfterSeconds: Math.max(1, Math.ceil((bucket.resetAt - now) / 1000)),
    };
  }

  bucket.count += 1;
  buckets.set(key, bucket);
  return { allowed: true };
}
