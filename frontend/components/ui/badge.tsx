import * as React from "react";

import { cn } from "@/lib/utils";

type BadgeProps = React.HTMLAttributes<HTMLSpanElement> & {
  variant?: "accent" | "muted" | "success" | "warning";
};

const variants: Record<NonNullable<BadgeProps["variant"]>, string> = {
  accent: "bg-[color-mix(in_oklab,var(--c-accent),white_70%)] text-[var(--c-ink)]",
  muted: "bg-[var(--c-panel-soft)] text-[var(--c-text)]",
  success: "bg-[#dcfce7] text-[#166534]",
  warning: "bg-[#fef3c7] text-[#92400e]",
};

export function Badge({ className, variant = "muted", ...props }: BadgeProps) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.08em]",
        variants[variant],
        className,
      )}
      {...props}
    />
  );
}
