import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ChevronDown, ChevronRight } from "lucide-react";
import { cn } from "@/lib/utils";
import type { ChatEventListItem } from "./chatTypes";

const TONE_DOT: Record<ChatEventListItem["tone"], string> = {
  danger:  "bg-rose-500",
  warning: "bg-amber-400",
  success: "bg-emerald-500",
  default: "bg-slate-300",
};

export function EventLogRow({ item }: { item: ChatEventListItem }) {
  const { t } = useTranslation();
  const hasExpandedContent = Boolean(item.detail || item.raw);
  const [expanded, setExpanded] = useState(false);

  return (
    <>
      <tr className="group border-b border-border/50 hover:bg-muted/30 transition-colors">
        {/* time */}
        <td className="w-[72px] whitespace-nowrap py-1.5 pl-3 pr-2 font-mono text-[11px] text-muted-foreground">
          {item.time}
        </td>

        {/* tone dot */}
        <td className="w-5 py-1.5 pr-2">
          <span className={cn("inline-block h-1.5 w-1.5 rounded-full", TONE_DOT[item.tone])} />
        </td>

        {/* rawType */}
        <td className="w-44 py-1.5 pr-3">
          <span className="font-mono text-[11px] text-muted-foreground">{item.rawType}</span>
        </td>

        {/* summary */}
        <td className="py-1.5 pr-2">
          {item.summary ? (
            <span className="line-clamp-1 text-xs text-foreground">{item.summary}</span>
          ) : (
            <span className="text-[11px] text-muted-foreground/50">{t("chat.noSummary")}</span>
          )}
        </td>

        {/* expand toggle */}
        <td className="w-7 py-1.5 pr-2 text-right">
          {hasExpandedContent && (
            <button
              type="button"
              className="rounded p-0.5 text-muted-foreground transition-colors hover:text-foreground"
              onClick={() => setExpanded((v) => !v)}
              title={expanded ? t("chat.collapse") : t("chat.expand")}
            >
              {expanded
                ? <ChevronDown className="h-3.5 w-3.5" />
                : <ChevronRight className="h-3.5 w-3.5" />}
            </button>
          )}
        </td>
      </tr>

      {/* expanded detail row */}
      {expanded && (item.detail || item.raw) && (
        <tr className="border-b border-border/50 bg-muted/20">
          <td colSpan={5} className="px-3 pb-2 pt-1">
            {item.detail && (
              <pre className="mb-1.5 whitespace-pre-wrap break-words rounded border bg-muted/40 px-2.5 py-1.5 font-mono text-[11px] leading-5 text-foreground">
                {item.detail}
              </pre>
            )}
            {item.raw && (
              <pre className="max-h-[280px] overflow-auto rounded border bg-slate-950 px-2.5 py-1.5 font-mono text-[11px] leading-5 text-slate-100">
                {item.raw}
              </pre>
            )}
          </td>
        </tr>
      )}
    </>
  );
}
