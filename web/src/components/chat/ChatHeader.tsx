import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Bot, ExternalLink, GitMerge, GitPullRequest, Loader2, Pencil, RefreshCw, User } from "lucide-react";
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
  lastUserMessage?: string;
  onDetailViewChange: (view: "chat" | "events") => void;
  onCloseSession: () => void;
  onRenameSession?: (title: string) => void;
  onCreatePR?: () => void;
  onRefreshPR?: () => void;
  prLoading?: boolean;
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
    lastUserMessage,
    onDetailViewChange,
    onCloseSession,
    onRenameSession,
    onCreatePR,
    onRefreshPR,
    prLoading = false,
  } = props;
  const { t } = useTranslation();

  const [editing, setEditing] = useState(false);
  const [editValue, setEditValue] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  const startEditing = () => {
    if (!session || !onRenameSession) return;
    setEditValue(session.title ?? "");
    setEditing(true);
    requestAnimationFrame(() => inputRef.current?.select());
  };

  const commitRename = () => {
    const trimmed = editValue.trim();
    setEditing(false);
    if (trimmed && trimmed !== (session?.title ?? "") && onRenameSession) {
      onRenameSession(trimmed);
    }
  };

  const cancelEditing = () => {
    setEditing(false);
  };

  const hasBranch = Boolean(session?.branch);
  const git = session?.git;
  const hasPR = Boolean(git?.pr_url);

  return (
    <div className="flex flex-col border-b">
      <div className="flex h-14 items-center justify-between px-6">
        <div className="flex items-center gap-3">
          <div className="flex h-9 w-9 items-center justify-center rounded-full bg-primary text-primary-foreground">
            <Bot className="h-[18px] w-[18px]" />
          </div>
          <div className="min-w-0">
            <div className="flex items-center gap-1.5">
              {editing ? (
                <input
                  ref={inputRef}
                  className="h-6 min-w-[120px] max-w-[320px] rounded border bg-background px-1.5 text-[15px] font-semibold outline-none focus:ring-1 focus:ring-primary"
                  value={editValue}
                  onChange={(e) => setEditValue(e.target.value)}
                  onBlur={commitRename}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") commitRename();
                    if (e.key === "Escape") cancelEditing();
                  }}
                />
              ) : (
                <>
                  <span className="truncate text-[15px] font-semibold">{session?.title ?? "Lead Agent"}</span>
                  {session && onRenameSession && (
                    <button
                      type="button"
                      className="shrink-0 rounded p-0.5 text-muted-foreground opacity-0 transition-opacity hover:text-foreground group-hover/header:opacity-100 [div:hover>&]:opacity-100"
                      onClick={startEditing}
                      title={t("chat.renameSession", { defaultValue: "重命名" })}
                    >
                      <Pencil className="h-3 w-3" />
                    </button>
                  )}
                </>
              )}
            </div>
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
                ? "bg-emerald-50 text-emerald-600"
                : "bg-muted text-muted-foreground",
          )}
        >
          <span className={cn(
            "h-1.5 w-1.5 rounded-full",
            session?.status === "running"
              ? "bg-blue-500"
              : session?.status === "alive"
                ? "bg-emerald-500"
                : "bg-muted-foreground",
          )} />
          {badgeLabelForStatus(session?.status, t)}
        </span>
        {/* PR section */}
        {hasBranch && !hasPR && onCreatePR && (
          <button
            type="button"
            className="inline-flex h-8 items-center gap-1.5 rounded-md border px-3 text-[13px] font-medium transition-colors hover:bg-muted disabled:cursor-not-allowed disabled:opacity-60"
            disabled={prLoading}
            onClick={onCreatePR}
          >
            {prLoading ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <GitPullRequest className="h-3.5 w-3.5" />
            )}
            {t("chat.createPR", { defaultValue: "Create PR" })}
          </button>
        )}
        {hasPR && (
          <div className="inline-flex items-center gap-1.5">
            <a
              href={git!.pr_url}
              target="_blank"
              rel="noopener noreferrer"
              className={cn(
                "inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs font-medium transition-colors hover:opacity-80",
                git!.pr_state === "merged"
                  ? "bg-purple-50 text-purple-600"
                  : git!.pr_state === "closed"
                    ? "bg-red-50 text-red-600"
                    : "bg-emerald-50 text-emerald-600",
              )}
            >
              {git!.pr_state === "merged" ? (
                <GitMerge className="h-3 w-3" />
              ) : (
                <GitPullRequest className="h-3 w-3" />
              )}
              PR #{git!.pr_number}
              <ExternalLink className="h-3 w-3" />
            </a>
            {git!.pr_state !== "merged" && onRefreshPR && (
              <button
                type="button"
                className="rounded p-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-60"
                disabled={prLoading}
                onClick={onRefreshPR}
                title={t("chat.refreshPR", { defaultValue: "刷新 PR 状态" })}
              >
                {prLoading ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <RefreshCw className="h-3.5 w-3.5" />
                )}
              </button>
            )}
          </div>
        )}
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
      {lastUserMessage ? (
        <div className="flex items-center gap-2 border-t bg-muted/30 px-6 py-1.5">
          <User className="h-3 w-3 shrink-0 text-muted-foreground" />
          <p className="truncate text-xs text-muted-foreground">
            {lastUserMessage.length > 120 ? `${lastUserMessage.slice(0, 120)}...` : lastUserMessage}
          </p>
        </div>
      ) : null}
    </div>
  );
}
