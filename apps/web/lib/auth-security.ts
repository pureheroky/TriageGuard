"use client";

import { validatePasswordPolicy } from "./password-policy";

const LOGIN_ATTEMPTS_KEY = "tg_login_attempts_v1";
const WINDOW_MS = 10 * 60 * 1000;
const MAX_ATTEMPTS_BEFORE_LOCK = 5;
const BASE_LOCK_SECONDS = 30;
const MAX_LOCK_SECONDS = 15 * 60;

type AttemptState = {
  count: number;
  firstFailureAt: number;
  lockUntil: number;
};

function loadState(): AttemptState {
  if (typeof window === "undefined") {
    return { count: 0, firstFailureAt: 0, lockUntil: 0 };
  }

  const raw = window.localStorage.getItem(LOGIN_ATTEMPTS_KEY);
  if (!raw) {
    return { count: 0, firstFailureAt: 0, lockUntil: 0 };
  }

  try {
    const parsed = JSON.parse(raw) as Partial<AttemptState>;
    return {
      count: Number(parsed.count) || 0,
      firstFailureAt: Number(parsed.firstFailureAt) || 0,
      lockUntil: Number(parsed.lockUntil) || 0,
    };
  } catch {
    return { count: 0, firstFailureAt: 0, lockUntil: 0 };
  }
}

function saveState(state: AttemptState) {
  if (typeof window === "undefined") {
    return;
  }
  window.localStorage.setItem(LOGIN_ATTEMPTS_KEY, JSON.stringify(state));
}

function clearState() {
  if (typeof window === "undefined") {
    return;
  }
  window.localStorage.removeItem(LOGIN_ATTEMPTS_KEY);
}

export function getLoginLockRemainingSeconds(now = Date.now()): number {
  const state = loadState();
  if (state.lockUntil <= now) {
    return 0;
  }
  return Math.ceil((state.lockUntil - now) / 1000);
}

export function registerLoginFailure(now = Date.now()): number {
  const state = loadState();

  let count = state.count;
  let firstFailureAt = state.firstFailureAt;
  let lockUntil = state.lockUntil;

  if (lockUntil > now) {
    return Math.ceil((lockUntil - now) / 1000);
  }

  if (firstFailureAt === 0 || now - firstFailureAt > WINDOW_MS) {
    count = 0;
    firstFailureAt = now;
  }

  count += 1;

  if (count >= MAX_ATTEMPTS_BEFORE_LOCK) {
    const extraFailures = count - MAX_ATTEMPTS_BEFORE_LOCK;
    const lockSeconds = Math.min(BASE_LOCK_SECONDS * Math.pow(2, extraFailures), MAX_LOCK_SECONDS);
    lockUntil = now + lockSeconds * 1000;
  } else {
    lockUntil = 0;
  }

  saveState({ count, firstFailureAt, lockUntil });
  return lockUntil > now ? Math.ceil((lockUntil - now) / 1000) : 0;
}

export function clearLoginFailures() {
  clearState();
}

export { validatePasswordPolicy };
