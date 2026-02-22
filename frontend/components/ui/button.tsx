import * as React from "react";

import { cn } from "@/lib/utils";

type ButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "default" | "secondary" | "outline" | "ghost" | "danger";
  size?: "sm" | "md" | "lg";
};

const variantClasses: Record<NonNullable<ButtonProps["variant"]>, string> = {
  default:
    "bg-[var(--c-accent)] text-[var(--c-ink)] hover:bg-[color-mix(in_oklab,var(--c-accent),black_12%)]",
  secondary:
    "bg-[var(--c-panel-soft)] text-[var(--c-text)] hover:bg-[color-mix(in_oklab,var(--c-panel-soft),black_8%)]",
  outline:
    "border border-[var(--c-border)] bg-transparent text-[var(--c-text)] hover:bg-[var(--c-panel-soft)]",
  ghost: "bg-transparent text-[var(--c-text)] hover:bg-[var(--c-panel-soft)]",
  danger: "bg-[#ef4444] text-white hover:bg-[#dc2626]",
};

const sizeClasses: Record<NonNullable<ButtonProps["size"]>, string> = {
  sm: "h-8 px-3 text-xs",
  md: "h-10 px-4 text-sm",
  lg: "h-12 px-5 text-base",
};

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  (
    {
      className,
      variant = "default",
      size = "md",
      type = "button",
      disabled,
      ...props
    },
    ref,
  ) => {
    return (
      <button
        ref={ref}
        type={type}
        disabled={disabled}
        className={cn(
          "inline-flex items-center justify-center rounded-lg font-semibold transition-colors duration-150 disabled:cursor-not-allowed disabled:opacity-55",
          variantClasses[variant],
          sizeClasses[size],
          className,
        )}
        {...props}
      />
    );
  },
);

Button.displayName = "Button";
