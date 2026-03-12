import { useTranslation } from "react-i18next";
import { Bot, Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import type { SessionRecord, ChatActivityView } from "./chatTypes";
import { badgeLabelForStatus, formatUsageValue } from "./chatUtils";

interface ChatHeaderProps {
  session: SessionRecord | null;
  driverLabel: string;
  messageCount: number;
  submitting: boolean;
  usage: ChatActivityView | undefined;
  usagePercent: number | null;
  detailView: "chat" | "events";
  onDetailViewChange: (view: "chat" | "events") => void;
  onCloseSession: () => void;
}

export function ChatHeader(props: ChatHeaderProps) {
  const {
    session,
    driverLabel,
    messageCount,
    submitting,
    usage,
    usagePercent,
    detailView,
    onDetailViewChange,
    onCloseSession,
  } = props;
  const { t } = useTranslation();

  return (
    <div className="flex h-14 items-center justify-between border-b px-6">
      <div className="flex items-center gap-3">
        <div className="flex h-9 w-9 items-center justify-center rounded-full bg-primary text-primary-foreground">
          <Bot className="h-[18px] w-[18px]" />
        </div>
        <div className="min-w-0">
          <span className="truncate text-[15px] font-semibold">{session?.title ?? "Lead Agent"}</span>
          <p className="text-xs text-muted-foreground">
            Lead Agent · {driverLabel} · {messageCount} {t("chat.turns")}
            {submitting ? <Loader2 className="ml-1.5 inline h-3 w-3 animate-spin" /> : null}
          </p>
        </div>
      </div>
      <div className="flex items-center gap-2">
        <div className="flex items-center rounded-md border bg-muted/30 p-0.5 text-xs">
          <button
            type="button"
            className={cn(
              "rounded px-2.5 py-1 transition-colors",
              detailView === "chat" ? "bg-background shadow-sm font-medium" : "text-muted-foreground hover:text-foreground",
            )}
            onClick={() => onDetailViewChange("chat")}
          >
            {t("chat.chat")}
          </button>
          <button
            type="button"
            className={cn(
              "rounded px-2.5 py-1 transition-colors",
              detailView === "events" ? "bg-background shadow-sm font-medium" : "text-muted-foreground hover:text-foreground",
            )}
            onClick={() => onDetailViewChange("events")}
          >
            {t("chat.events")}
          </button>
        </div>
        <span
          className={cn(
            "inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs font-medium",
            session?.status === "running"
              ? "bg-blue-50 text-blue-500"
              : session?.status === "alive"
                ? "bg-amber-50 text-amber-500"
                : "bg-muted text-muted-foreground",
          )}
        >
          <span className={cn(
            "h-1.5 w-1.5 rounded-full",
            session?.status === "running"
              ? "bg-blue-500"
              : session?.status === "alive"
                ? "bg-amber-500"
                : "bg-muted-foreground",
          )} />
          {badgeLabelForStatus(session?.status, t)}
        </span>
        {usage ? (
          <div className="flex items-center gap-1.5 rounded-full border bg-background px-2.5 py-1 text-[11px] text-muted-foreground">
            <span className="shrink-0 whitespace-nowrap">{t("chat.context")}</span>
            <div className="h-1.5 w-20 overflow-hidden rounded-full bg-muted">
              <div
                className={cn(
                  "h-full rounded-full transition-[width] duration-300",
                  usagePercent != null && usagePercent >= 85
                    ? "bg-rose-500"
                    : usagePercent != null && usagePercent >= 60
                      ? "bg-amber-500"
                      : "bg-emerald-500",
                )}
                style={{ width: `${Math.max(usagePercent ?? 0, usagePercent == null ? 0 : 4)}%` }}
              />
            </div>
            <span className="shrink-0 whitespace-nowrap">
              {formatUsageValue(usage.usageUsed)} / {formatUsageValue(usage.usageSize)}
            </span>
          </div>
        ) : null}
        <button
          type="button"
          className="h-8 rounded-md border px-3 text-[13px] font-medium transition-colors hover:bg-muted"
          onClick={onCloseSession}
        >
          {t("chat.endSession")}
        </button>
      </div>
    </div>
  );
}
