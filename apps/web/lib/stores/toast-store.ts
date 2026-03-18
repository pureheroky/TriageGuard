"use client";

import { create } from "zustand";

export type ToastVariant = "success" | "error" | "info";

export type ToastItem = {
  id: string;
  title: string;
  description?: string;
  variant: ToastVariant;
  durationMs: number;
};

type ToastInput = {
  title: string;
  description?: string;
  variant?: ToastVariant;
  durationMs?: number;
};

type ToastState = {
  toasts: ToastItem[];
  push: (input: ToastInput) => string;
  dismiss: (id: string) => void;
  clearAll: () => void;
};

const activeTimers = new Map<string, ReturnType<typeof setTimeout>>();

function buildID(): string {
  return `${Date.now()}-${Math.random().toString(16).slice(2, 10)}`;
}

const defaultDurationByVariant: Record<ToastVariant, number> = {
  success: 3200,
  info: 3800,
  error: 5200,
};

export const useToastStore = create<ToastState>((set) => ({
  toasts: [],
  push: ({ title, description, variant = "info", durationMs }) => {
    const id = buildID();
    const resolvedDuration = durationMs ?? defaultDurationByVariant[variant];
    const toast: ToastItem = {
      id,
      title,
      description,
      variant,
      durationMs: resolvedDuration,
    };

    set((state) => ({
      toasts: [...state.toasts, toast],
    }));

    const timer = setTimeout(() => {
      useToastStore.getState().dismiss(id);
    }, resolvedDuration);
    activeTimers.set(id, timer);

    return id;
  },
  dismiss: (id) => {
    const timer = activeTimers.get(id);
    if (timer) {
      clearTimeout(timer);
      activeTimers.delete(id);
    }
    set((state) => ({
      toasts: state.toasts.filter((toast) => toast.id !== id),
    }));
  },
  clearAll: () => {
    for (const timer of activeTimers.values()) {
      clearTimeout(timer);
    }
    activeTimers.clear();
    set({ toasts: [] });
  },
}));

export function toastSuccess(title: string, description?: string) {
  useToastStore.getState().push({ title, description, variant: "success" });
}

export function toastError(title: string, description?: string) {
  useToastStore.getState().push({ title, description, variant: "error" });
}

export function toastInfo(title: string, description?: string) {
  useToastStore.getState().push({ title, description, variant: "info" });
}
