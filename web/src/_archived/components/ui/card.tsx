import type { HTMLAttributes, PropsWithChildren } from "react";
import { cn } from "@/lib/utils";

export const Card = ({
  className,
  ...props
}: PropsWithChildren<HTMLAttributes<HTMLDivElement>>) => (
  <div
    className={cn(
      "rounded-2xl border border-slate-200 bg-white shadow-none",
      className,
    )}
    {...props}
  />
);

export const CardHeader = ({
  className,
  ...props
}: PropsWithChildren<HTMLAttributes<HTMLDivElement>>) => (
  <div className={cn("flex flex-col gap-2 p-5", className)} {...props} />
);

export const CardTitle = ({
  className,
  ...props
}: PropsWithChildren<HTMLAttributes<HTMLHeadingElement>>) => (
  <h3 className={cn("text-lg font-semibold tracking-tight text-slate-950", className)} {...props} />
);

export const CardDescription = ({
  className,
  ...props
}: PropsWithChildren<HTMLAttributes<HTMLParagraphElement>>) => (
  <p className={cn("text-sm leading-6 text-slate-500", className)} {...props} />
);

export const CardContent = ({
  className,
  ...props
}: PropsWithChildren<HTMLAttributes<HTMLDivElement>>) => (
  <div className={cn("px-5 pb-5", className)} {...props} />
);
