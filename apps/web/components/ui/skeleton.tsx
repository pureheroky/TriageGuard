"use client";

import { motion } from "framer-motion";
import type { HTMLMotionProps } from "framer-motion";
import { cn } from "@/lib/utils";

type SkeletonProps = Omit<HTMLMotionProps<"div">, "initial" | "animate" | "transition">;

export function Skeleton({ className, ...props }: SkeletonProps) {
  return (
    <motion.div
      initial={{ opacity: 0.5 }}
      animate={{ opacity: [0.45, 0.9, 0.45] }}
      transition={{ duration: 1.35, ease: "easeInOut", repeat: Number.POSITIVE_INFINITY }}
      className={cn("rounded-md bg-muted/80", className)}
      {...props}
    />
  );
}
