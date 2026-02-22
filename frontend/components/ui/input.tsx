import * as React from "react";

import { cn } from "@/lib/utils";

export const Input = React.forwardRef<
  HTMLInputElement,
  React.InputHTMLAttributes<HTMLInputElement>
>(({ className, ...props }, ref) => {
  return (
    <input
      ref={ref}
      className={cn(
        "h-10 w-full rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] px-3 text-sm text-[var(--c-text)] outline-none transition-all placeholder:text-[var(--c-text-muted)] focus:border-[var(--c-accent)] focus:ring-2 focus:ring-[color-mix(in_oklab,var(--c-accent),white_55%)]",
        className,
      )}
      {...props}
    />
  );
});

Input.displayName = "Input";
