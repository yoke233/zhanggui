import type { HTMLAttributes, PropsWithChildren } from "react";
import { cn } from "@/lib/utils";

type BadgeVariant = "default" | "secondary" | "success" | "warning" | "danger" | "outline";

const VARIANT_CLASS: Record<BadgeVariant, string> = {
  default: "bg-slate-950 text-white",
  secondary: "bg-blue-50 text-blue-700",
  success: "bg-emerald-50 text-emerald-700",
  warning: "bg-amber-50 text-amber-700",
  danger: "bg-rose-50 text-rose-700",
  outline: "border border-slate-200 bg-white text-slate-600",
};

export interface BadgeProps
  extends PropsWithChildren<HTMLAttributes<HTMLSpanElement>> {
  variant?: BadgeVariant;
}

export const Badge = ({
  className,
  variant = "default",
  ...props
}: BadgeProps) => (
  <span
    className={cn(
      "inline-flex items-center rounded-full px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.12em]",
      VARIANT_CLASS[variant],
      className,
    )}
    {...props}
  />
);
