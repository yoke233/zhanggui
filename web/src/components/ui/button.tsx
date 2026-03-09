import type { ButtonHTMLAttributes, PropsWithChildren } from "react";
import { cn } from "@/lib/utils";

type ButtonVariant = "default" | "secondary" | "ghost" | "outline" | "destructive";
type ButtonSize = "default" | "sm" | "lg" | "icon";

export interface ButtonProps
  extends PropsWithChildren<ButtonHTMLAttributes<HTMLButtonElement>> {
  variant?: ButtonVariant;
  size?: ButtonSize;
}

const VARIANT_CLASS: Record<ButtonVariant, string> = {
  default:
    "bg-slate-950 text-white shadow-sm hover:bg-slate-800",
  secondary:
    "bg-blue-600 text-white shadow-sm hover:bg-blue-500",
  ghost:
    "bg-transparent text-slate-300 hover:bg-white/10 hover:text-white",
  outline:
    "border border-slate-300 bg-white text-slate-700 hover:bg-slate-50",
  destructive:
    "bg-rose-600 text-white shadow-sm hover:bg-rose-500",
};

const SIZE_CLASS: Record<ButtonSize, string> = {
  default: "h-10 px-4 py-2",
  sm: "h-8 rounded-[10px] px-3 text-xs",
  lg: "h-11 rounded-xl px-5 text-sm",
  icon: "h-10 w-10 rounded-[10px]",
};

export const Button = ({
  className,
  variant = "default",
  size = "default",
  type = "button",
  ...props
}: ButtonProps) => {
  return (
    <button
      type={type}
      className={cn(
        "inline-flex items-center justify-center rounded-[10px] text-sm font-semibold transition-colors",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/40",
        "disabled:cursor-not-allowed disabled:opacity-50",
        VARIANT_CLASS[variant],
        SIZE_CLASS[size],
        className,
      )}
      {...props}
    />
  );
};
