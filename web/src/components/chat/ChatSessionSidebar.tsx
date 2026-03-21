import { memo, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Archive, ChevronDown, ChevronRight, GitMerge, Loader2, Plus, Search, ShieldAlert } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import type { SessionRecord, SessionGroup, ChatMessageView } from "./chatTypes";
import { badgeLabelForStatus } from "./chatUtils";

interface SessionItemProps {
  session: SessionRecord;
  isActive: boolean;
  preview: string;
  turnCount: number;
  hasPermission: boolean;
  onSelect: (sessionId: string) => void;
  onArchive?: (sessionId: string) => void;
}

const SessionItem = memo(function SessionItem({ session, isActive, preview, turnCount, hasPermission, onSelect, onArchive }: SessionItemProps) {
  const { t } = useTranslation();
  const canArchive = onArchive && session.status !== "running";
  return (
    <div
      className={cn(
        "group/session relative w-full border-b text-left transition-colors",
        isActive ? "bg-accent" : "hover:bg-muted/50",
      )}
    >
      <button
        onClick={() => onSelect(session.session_id)}
        className="w-full px-4 py-3 pl-7 text-left"
      >
        <div className="flex items-center justify-between gap-2">
          <span className={cn(
            "truncate text-sm",
            isActive ? "font-semibold" : "font-medium",
          )}>{session.title ?? t("chat.newSession")}</span>
          <span className="shrink-0 text-[11px] text-muted-foreground">
            {new Date(session.created_at).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })}
          </span>
        </div>
        <p className="mt-1.5 truncate text-xs text-muted-foreground">{preview}</p>
        <div className="mt-2 flex flex-wrap items-center gap-1.5">
          <span
            className={cn(
              "inline-flex items-center rounded-full px-1.5 py-px text-[10px] font-medium",
              session.status === "running"
                ? "bg-blue-50 text-blue-500"
                : session.status === "alive"
                  ? "bg-emerald-50 text-emerald-600"
                  : "bg-muted text-muted-foreground",
            )}
          >
            {badgeLabelForStatus(session.status, t)}
          </span>
          {turnCount > 0 && (
            <span className="inline-flex items-center rounded-full bg-secondary px-1.5 py-px text-[10px] font-medium text-muted-foreground">
              {turnCount} {t("chat.turns")}
            </span>
          )}
          {session.git && (session.git.additions > 0 || session.git.deletions > 0) && (
            <span className="inline-flex items-center gap-1 rounded-full bg-secondary px-1.5 py-px text-[10px] font-medium">
              <span className="text-green-600">+{session.git.additions}</span>
              <span className="text-red-500">-{session.git.deletions}</span>
            </span>
          )}
          {session.git?.merged && (
            <span className="inline-flex items-center gap-0.5 rounded-full bg-purple-50 px-1.5 py-px text-[10px] font-medium text-purple-600">
              <GitMerge className="h-3 w-3" />
              {t("chat.merged", { defaultValue: "已合并" })}
            </span>
          )}
          {hasPermission && (
            <span className="inline-flex items-center gap-0.5 rounded-full bg-amber-100 px-1.5 py-px text-[10px] font-semibold text-amber-700 animate-pulse">
              <ShieldAlert className="h-3 w-3" />
              {t("chat.needsPermission", { defaultValue: "待授权" })}
            </span>
          )}
        </div>
      </button>
      {canArchive && (
        <button
          type="button"
          title={t("chat.archive", { defaultValue: "归档" })}
          className="absolute right-2 top-2 hidden rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground group-hover/session:block"
          onClick={(e) => {
            e.stopPropagation();
            onArchive(session.session_id);
          }}
        >
          <Archive className="h-3.5 w-3.5" />
        </button>
      )}
    </div>
  );
});

interface ChatSessionSidebarProps {
  groupedSessions: SessionGroup[];
  activeSession: string | null;
  sessionSearch: string;
  loadingSessions: boolean;
  creatingSession: boolean;
  messagesBySession: Record<string, ChatMessageView[]>;
  collapsedGroups: Record<string, boolean>;
  pendingPermissionSessionIds: ReadonlySet<string>;
  onSearchChange: (value: string) => void;
  onSessionSelect: (sessionId: string) => void;
  onGroupToggle: (key: string) => void;
  onCreateSession: () => void;
  onArchiveSession?: (sessionId: string) => void;
  drawerOpen?: boolean;
  onClose?: () => void;
}

export const ChatSessionSidebar = memo(function ChatSessionSidebar(props: ChatSessionSidebarProps) {
  const {
    groupedSessions,
    activeSession,
    sessionSearch,
    loadingSessions,
    creatingSession,
    messagesBySession,
    collapsedGroups,
    pendingPermissionSessionIds,
    onSearchChange,
    onGroupToggle,
    onCreateSession,
    onArchiveSession,
  } = props;
  const { t } = useTranslation();

  const isDrawer = props.drawerOpen !== undefined;
  if (isDrawer && !props.drawerOpen) return null;

  const handleSessionSelect = (sessionId: string) => {
    props.onSessionSelect(sessionId);
    if (isDrawer && props.onClose) props.onClose();
  };

  /* Derive a stable preview map: only recalculate when messagesBySession changes */
  const previewMap = useMemo(() => {
    const map: Record<string, { preview: string; turnCount: number }> = {};
    for (const [sessionId, messages] of Object.entries(messagesBySession)) {
      map[sessionId] = {
        preview: [...messages].reverse().find((m) => m.role === "assistant")?.content ?? messages.at(-1)?.content ?? "",
        turnCount: messages.length,
      };
    }
    return map;
  }, [messagesBySession]);

  const content = (
    <div className={cn("flex w-72 flex-col border-r bg-sidebar", isDrawer && "h-screen")}>
      <div className="border-b p-3">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-semibold">{t("chat.sessionList")}</h2>
          <Button variant="outline" size="sm" className="h-8 gap-1.5 px-2.5 text-xs" onClick={onCreateSession}>
            <Plus className="h-3.5 w-3.5" />
            {t("chat.new")}
          </Button>
        </div>
        <div className="relative">
          <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder={t("chat.searchSessions")}
            className="h-8 pl-8 text-xs"
            value={sessionSearch}
            onChange={(event) => onSearchChange(event.target.value)}
          />
        </div>
      </div>

      <div className="flex-1 overflow-y-auto">
        {creatingSession && (
          <div className="flex items-center gap-2.5 border-b bg-blue-50/40 px-4 py-3 pl-7">
            <Loader2 className="h-3.5 w-3.5 animate-spin text-blue-500" />
            <span className="text-sm text-muted-foreground">
              {t("chat.creatingSession", { defaultValue: "正在创建会话..." })}
            </span>
          </div>
        )}
        {groupedSessions.map((group) => (
          <div key={group.key} className="border-b">
            <button
              type="button"
              className="flex w-full items-center gap-2 px-3 py-2 text-left transition-colors hover:bg-muted/50"
              onClick={() => onGroupToggle(group.key)}
            >
              {collapsedGroups[group.key] ? (
                <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
              ) : (
                <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
              )}
              <div className="min-w-0 flex-1">
                <div className="truncate text-[11px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">
                  {group.label}
                </div>
              </div>
              <Badge variant="secondary" className="text-[10px]">
                {group.sessions.length}
              </Badge>
            </button>

            {!collapsedGroups[group.key] ? group.sessions.map((session) => {
              const info = previewMap[session.session_id];
              return (
                <SessionItem
                  key={session.session_id}
                  session={session}
                  isActive={activeSession === session.session_id}
                  preview={info?.preview || t("chat.noMessages")}
                  turnCount={info?.turnCount ?? 0}
                  hasPermission={pendingPermissionSessionIds.has(session.session_id)}
                  onSelect={handleSessionSelect}
                  onArchive={onArchiveSession}
                />
              );
            }) : null}
          </div>
        ))}
        {!loadingSessions && groupedSessions.length === 0 ? (
          <div className="px-3 py-4 text-xs text-muted-foreground">
            {t("chat.noSessions")}
          </div>
        ) : null}
      </div>
    </div>
  );

  if (isDrawer) {
    return (
      <div className="fixed inset-0 z-50 flex" onClick={props.onClose}>
        <div onClick={(e) => e.stopPropagation()}>
          {content}
        </div>
        <div className="flex-1 drawer-overlay" />
      </div>
    );
  }

  return content;
});
