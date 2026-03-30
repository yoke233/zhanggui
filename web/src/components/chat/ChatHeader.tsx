import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Bot, ExternalLink, GitMerge, GitPullRequest, Loader2, MessagesSquare, Pencil, RefreshCw, User } from "lucide-react";
import { cn } from "@/lib/utils";
import type { SessionRecord, ChatActivityView } from "./chatTypes";
import { badgeLabelForStatus, formatUsageValue } from "./chatUtils";

interface ChatHeaderProps {
  session: SessionRecord | null;
  driverLabel: string;
  profileLabel: string;
  messageCount: number;
  submitting: boolean;
  usage: ChatActivityView | undefined;
  usagePercent: number | null;
  detailView: "chat" | "events";
  lastUserMessage?: string;
  onDetailViewChange: (view: "chat" | "events") => void;
  onCloseSession: () => void;
  onRenameSession?: (title: string) => void;
  onSubmitCode?: () => void;
  onCreatePR?: () => void;
  onRefreshPR?: () => void;
  submitLoading?: boolean;
  prLoading?: boolean;
  onOpenSessions?: () => void;
}

export function ChatHeader(props: ChatHeaderProps) {
  const {
    session,
    driverLabel,
    profileLabel,
    messageCount,
    submitting,
    usage,
    usagePercent,
    detailView,
    lastUserMessage,
    onDetailViewChange,
    onCloseSession,
    onRenameSession,
    onSubmitCode,
    onCreatePR,
    onRefreshPR,
    submitLoading = false,
    prLoading = false,
    onOpenSessions,
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
  const hasSubmittedCode = Boolean(git?.head_sha);
  const hasWorkingTreeChanges = Boolean((git?.files_changed ?? 0) > 0 || (git?.additions ?? 0) > 0 || (git?.deletions ?? 0) > 0);
  const hasPR = Boolean(git?.pr_url);
  const condensedUsage = usagePercent != null
    ? `${Math.round(usagePercent)}%`
    : usage
      ? formatUsageValue(usage.usageUsed)
      : null;

  return (
    <div className="group/header flex flex-col border-b bg-background">
      <div className="flex flex-col gap-2 px-3 py-2 md:h-14 md:flex-row md:items-center md:justify-between md:gap-3 md:px-6 md:py-0">
        <div className="flex min-w-0 items-start gap-2 md:gap-3">
          <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary text-primary-foreground md:h-9 md:w-9">
            <Bot className="h-4 w-4 md:h-[18px] md:w-[18px]" />
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-1.5">
              {editing ? (
                <input
                  ref={inputRef}
                  className="h-7 w-full min-w-0 rounded border bg-background px-2 text-[15px] font-semibold outline-none focus:ring-1 focus:ring-primary md:max-w-[320px]"
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
                  <span className="truncate text-[15px] font-semibold">{session?.title ?? profileLabel}</span>
                  {session && onRenameSession && (
                    <button
                      type="button"
                      className="shrink-0 rounded p-0.5 text-muted-foreground transition-colors hover:text-foreground md:opacity-0 md:group-hover/header:opacity-100 [div:hover>&]:opacity-100"
                      onClick={startEditing}
                      title={t("chat.renameSession", { defaultValue: "重命名" })}
                    >
                      <Pencil className="h-3 w-3" />
                    </button>
                  )}
                </>
              )}
            </div>
            <p className="mt-0.5 flex flex-wrap items-center gap-x-1 gap-y-0.5 text-[11px] text-muted-foreground md:text-xs">
              <span>{profileLabel}</span>
              <span>·</span>
              <span>{driverLabel}</span>
              <span className="hidden md:inline">·</span>
              <span className="hidden md:inline">{messageCount} {t("chat.turns")}</span>
              {submitting ? <Loader2 className="h-3 w-3 animate-spin" /> : null}
            </p>
          </div>
        </div>

        <div className="flex w-full flex-wrap items-center gap-2 pb-0.5 md:w-auto md:justify-end md:pb-0">
          {onOpenSessions ? (
            <button
              type="button"
              className="inline-flex h-8 shrink-0 items-center gap-1 rounded-md border px-2.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
              onClick={onOpenSessions}
              title={t("chat.sessionList")}
            >
              <MessagesSquare className="h-3.5 w-3.5" />
              {t("chat.sessionList")}
            </button>
          ) : null}

          <div className="hidden shrink-0 items-center rounded-md border bg-muted/30 p-0.5 text-xs md:flex">
            <button
              type="button"
              className={cn(
                "rounded px-2 py-1 transition-colors md:px-2.5",
                detailView === "chat" ? "bg-background font-medium shadow-sm" : "text-muted-foreground hover:text-foreground",
              )}
              onClick={() => onDetailViewChange("chat")}
            >
              {t("chat.chat")}
            </button>
            <button
              type="button"
              className={cn(
                "rounded px-2 py-1 transition-colors md:px-2.5",
                detailView === "events" ? "bg-background font-medium shadow-sm" : "text-muted-foreground hover:text-foreground",
              )}
              onClick={() => onDetailViewChange("events")}
            >
              {t("chat.events")}
            </button>
          </div>

          <span
            className={cn(
              "inline-flex shrink-0 items-center gap-1 rounded-full px-2 py-1 text-xs font-medium md:px-2.5",
              session?.status === "running"
                ? "bg-blue-50 text-blue-500"
                : session?.status === "alive"
                  ? "bg-emerald-50 text-emerald-600"
                  : "bg-muted text-muted-foreground",
            )}
          >
            <span
              className={cn(
                "h-1.5 w-1.5 rounded-full",
                session?.status === "running"
                  ? "bg-blue-500"
                  : session?.status === "alive"
                    ? "bg-emerald-500"
                    : "bg-muted-foreground",
              )}
            />
            {badgeLabelForStatus(session?.status, t)}
          </span>

          {hasBranch && git && (git.files_changed > 0 || git.additions > 0 || git.deletions > 0) && (
            <span className="inline-flex shrink-0 items-center gap-1 rounded-full border bg-background px-2 py-1 text-xs text-muted-foreground md:px-2.5">
              <span>{git.files_changed} files</span>
              <span>·</span>
              <span className="text-emerald-600">+{git.additions}</span>
              <span>/</span>
              <span className="text-rose-600">-{git.deletions}</span>
            </span>
          )}

          {hasBranch && !hasPR && hasWorkingTreeChanges && onSubmitCode && (
            <button
              type="button"
              className="inline-flex h-8 shrink-0 items-center gap-1 rounded-md border px-2.5 text-xs font-medium transition-colors hover:bg-muted disabled:cursor-not-allowed disabled:opacity-60 md:gap-1.5 md:px-3 md:text-[13px]"
              disabled={submitLoading}
              onClick={onSubmitCode}
            >
              {submitLoading ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <GitMerge className="h-3.5 w-3.5" />
              )}
              <span>{t("chat.submitCode", { defaultValue: "提交代码" })}</span>
            </button>
          )}

          {hasBranch && !hasPR && hasSubmittedCode && onCreatePR && (
            <button
              type="button"
              className="inline-flex h-8 shrink-0 items-center gap-1 rounded-md border px-2.5 text-xs font-medium transition-colors hover:bg-muted disabled:cursor-not-allowed disabled:opacity-60 md:gap-1.5 md:px-3 md:text-[13px]"
              disabled={prLoading}
              onClick={onCreatePR}
            >
              {prLoading ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <GitPullRequest className="h-3.5 w-3.5" />
              )}
              <span className="md:hidden">PR</span>
              <span className="hidden md:inline">{t("chat.createPR", { defaultValue: "Create PR" })}</span>
            </button>
          )}

          {hasPR && (
            <div className="inline-flex max-w-full shrink-0 items-center gap-1.5">
              <a
                href={git!.pr_url}
                target="_blank"
                rel="noopener noreferrer"
                className={cn(
                  "inline-flex max-w-full items-center gap-1 rounded-full px-2 py-1 text-xs font-medium transition-colors hover:opacity-80 md:px-2.5",
                  git!.pr_state === "merged"
                    ? "bg-purple-50 text-purple-600"
                    : git!.pr_state === "closed"
                      ? "bg-red-50 text-red-600"
                      : "bg-emerald-50 text-emerald-600",
                )}
              >
                {git!.pr_state === "merged" ? (
                  <GitMerge className="h-3 w-3 shrink-0" />
                ) : (
                  <GitPullRequest className="h-3 w-3 shrink-0" />
                )}
                <span className="truncate">PR #{git!.pr_number}</span>
                <ExternalLink className="h-3 w-3 shrink-0" />
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
            <>
              <div className="inline-flex shrink-0 items-center gap-1 rounded-full border bg-background px-2 py-1 text-xs text-muted-foreground md:hidden">
                <span>{t("chat.context")}</span>
                <span>{condensedUsage}</span>
              </div>
              <div className="hidden items-center gap-1.5 rounded-full border bg-background px-2.5 py-1 text-[11px] text-muted-foreground md:flex">
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
            </>
          ) : null}

          <button
            type="button"
            className="h-8 shrink-0 rounded-md border px-2.5 text-xs font-medium transition-colors hover:bg-muted md:px-3 md:text-[13px]"
            onClick={onCloseSession}
          >
            <span className="md:hidden">结束</span>
            <span className="hidden md:inline">{t("chat.endSession")}</span>
          </button>
        </div>
      </div>
      {lastUserMessage ? (
        <div className="hidden items-center gap-2 border-t bg-muted/30 px-3 py-1.5 md:flex md:px-6">
          <User className="h-3 w-3 shrink-0 text-muted-foreground" />
          <p className="truncate text-xs text-muted-foreground">
            {lastUserMessage.length > 120 ? `${lastUserMessage.slice(0, 120)}...` : lastUserMessage}
          </p>
        </div>
      ) : null}
    </div>
  );
}
