import type { ReactNode } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

interface ActionItem {
  label: string;
  onClick: () => void;
  variant?: "default" | "secondary" | "ghost" | "outline" | "destructive";
}

interface StatItem {
  label: string;
  value: string;
  helper: string;
}

interface PageScaffoldProps {
  eyebrow: string;
  title: string;
  description: string;
  contextTitle?: string;
  contextMeta?: string;
  actions?: ActionItem[];
  stats?: StatItem[];
  children: ReactNode;
}

export const PageScaffold = ({
  eyebrow,
  title,
  description,
  contextTitle,
  contextMeta,
  actions,
  stats,
  children,
}: PageScaffoldProps) => {
  return (
    <section className="flex flex-col gap-4">
      <Card className="rounded-2xl border-slate-200 shadow-none">
        <CardHeader className="gap-4 p-5">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div className="max-w-3xl">
              <Badge variant="secondary" className="bg-indigo-50 text-indigo-600">
                {eyebrow}
              </Badge>
              <CardTitle className="mt-3 text-[24px] font-semibold tracking-[-0.02em] text-slate-950">
                {title}
              </CardTitle>
              <CardDescription className="mt-1 text-sm leading-6 text-slate-500">{description}</CardDescription>
            </div>
            {contextTitle || contextMeta ? (
              <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
                {contextTitle ? (
                  <p className="text-sm font-semibold text-slate-950">{contextTitle}</p>
                ) : null}
                {contextMeta ? (
                  <p className="mt-1 text-xs leading-5 text-slate-500">{contextMeta}</p>
                ) : null}
              </div>
            ) : null}
          </div>

          {actions && actions.length > 0 ? (
            <div className="flex flex-wrap gap-3">
              {actions.map((action) => (
                <Button
                  key={action.label}
                  variant={action.variant ?? "outline"}
                  onClick={action.onClick}
                >
                  {action.label}
                </Button>
              ))}
            </div>
          ) : null}
        </CardHeader>

        {stats && stats.length > 0 ? (
          <CardContent className="grid gap-3 px-5 pb-5 md:grid-cols-2 xl:grid-cols-4">
            {stats.map((stat) => (
              <div key={stat.label} className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
                <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
                  {stat.label}
                </p>
                <p className="mt-2 text-2xl font-semibold tracking-[-0.02em] text-slate-950">
                  {stat.value}
                </p>
                <p className="mt-2 text-xs leading-5 text-slate-500">{stat.helper}</p>
              </div>
            ))}
          </CardContent>
        ) : null}
      </Card>

      {children}
    </section>
  );
};
