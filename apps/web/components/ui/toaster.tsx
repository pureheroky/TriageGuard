"use client";

import { CheckCircle2, Info, TriangleAlert, X } from "lucide-react";
import { useToastStore } from "@/lib/stores/toast-store";
import { cn } from "@/lib/utils";

function ToastIcon({ variant }: { variant: "success" | "error" | "info" }) {
  if (variant === "success") {
    return <CheckCircle2 className="size-4 text-success" />;
  }
  if (variant === "error") {
    return <TriangleAlert className="size-4 text-destructive" />;
  }
  return <Info className="size-4 text-primary" />;
}

export function Toaster() {
  const toasts = useToastStore((state) => state.toasts);
  const dismiss = useToastStore((state) => state.dismiss);

  return (
    <div className="pointer-events-none fixed right-4 top-4 z-[100] flex w-[360px] max-w-[calc(100vw-2rem)] flex-col gap-2">
      {toasts.map((toast) => (
        <div
          key={toast.id}
          className={cn(
            "pointer-events-auto animate-in slide-in-from-right-6 fade-in-0 rounded-lg border bg-card p-3 shadow-lg",
            toast.variant === "error" && "border-destructive/30",
            toast.variant === "success" && "border-success/30",
            toast.variant === "info" && "border-primary/20",
          )}
        >
          <div className="flex items-start gap-2.5">
            <ToastIcon variant={toast.variant} />
            <div className="min-w-0 flex-1 space-y-1">
              <p className="text-sm font-semibold text-foreground">{toast.title}</p>
              {toast.description && <p className="text-xs leading-relaxed text-muted-foreground">{toast.description}</p>}
            </div>
            <button
              type="button"
              onClick={() => dismiss(toast.id)}
              className="rounded p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
              aria-label="Dismiss notification"
            >
              <X className="size-3.5" />
            </button>
          </div>
        </div>
      ))}
    </div>
  );
}
