import type { TextareaHTMLAttributes } from "react";
import { cn } from "@/lib/utils";

export const Textarea = ({
  className,
  ...props
}: TextareaHTMLAttributes<HTMLTextAreaElement>) => (
  <textarea
    className={cn(
      "flex min-h-[120px] w-full rounded-[10px] border border-slate-200 bg-white px-3 py-2 text-sm text-slate-900",
      "placeholder:text-slate-400 focus:border-blue-400 focus:outline-none focus:ring-2 focus:ring-blue-500/20",
      "disabled:cursor-not-allowed disabled:bg-slate-50 disabled:text-slate-400",
      className,
    )}
    {...props}
  />
);
