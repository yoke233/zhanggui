import { useTranslation } from "react-i18next";
import { ChevronDown, ChevronRight, Loader2, Plus, Search } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import type { SessionGroup, ChatMessageView } from "./chatTypes";
import { badgeLabelForStatus } from "./chatUtils";

interface ChatSessionSidebarProps {
  groupedSessions: SessionGroup[];
  activeSession: string | null;
  sessionSearch: string;
  loadingSessions: boolean;
  creatingSession: boolean;
  messagesBySession: Record<string, ChatMessageView[]>;
  collapsedGroups: Record<string, boolean>;
  onSearchChange: (value: string) => void;
  onSessionSelect: (sessionId: string) => void;
  onGroupToggle: (key: string) => void;
  onCreateSession: () => void;
}

export function ChatSessionSidebar(props: ChatSessionSidebarProps) {
  const {
    groupedSessions,
    activeSession,
    sessionSearch,
    loadingSessions,
    creatingSession,
    messagesBySession,
    collapsedGroups,
    onSearchChange,
    onSessionSelect,
    onGroupToggle,
    onCreateSession,
  } = props;
  const { t } = useTranslation();

  return (
    <div className="flex w-72 flex-col border-r bg-sidebar">
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
              const preview = messagesBySession[session.session_id]?.at(-1)?.content ?? t("chat.noMessages");
              const turnCount = messagesBySession[session.session_id]?.length ?? 0;
              return (
                <button
                  key={session.session_id}
                  onClick={() => onSessionSelect(session.session_id)}
                  className={cn(
                    "w-full border-b px-4 py-3 pl-7 text-left transition-colors",
                    activeSession === session.session_id ? "bg-accent" : "hover:bg-muted/50",
                  )}
                >
                  <div className="flex items-center justify-between gap-2">
                    <span className={cn(
                      "truncate text-sm",
                      activeSession === session.session_id ? "font-semibold" : "font-medium",
                    )}>{session.title ?? t("chat.newSession")}</span>
                    <span className="shrink-0 text-[11px] text-muted-foreground">
                      {new Date(session.updated_at).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })}
                    </span>
                  </div>
                  <p className="mt-1.5 truncate text-xs text-muted-foreground">{preview}</p>
                  <div className="mt-2 flex items-center gap-1.5">
                    <span
                      className={cn(
                        "inline-flex items-center rounded-full px-1.5 py-px text-[10px] font-medium",
                        session.status === "running"
                          ? "bg-blue-50 text-blue-500"
                          : session.status === "alive"
                            ? "bg-amber-50 text-amber-500"
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
                  </div>
                </button>
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
}
