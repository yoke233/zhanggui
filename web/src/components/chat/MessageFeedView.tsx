import { useTranslation } from "react-i18next";
import {
  Brain,
  Check,
  ChevronDown,
  ChevronRight,
  ClipboardCopy,
  ListTodo,
  Loader2,
  Wrench,
} from "lucide-react";
import { cn } from "@/lib/utils";
import type { ChatFeedEntry } from "./chatTypes";
import { compactText } from "./chatUtils";

interface MessageFeedViewProps {
  entries: ChatFeedEntry[];
  submitting: boolean;
  copiedMessageId: string | null;
  collapsedActivityGroups: Record<string, boolean>;
  onCopyMessage: (id: string, content: string) => void;
  onCreateWorkItem: (id: string, content: string) => void;
  onActivityGroupToggle: (id: string) => void;
}

export function MessageFeedView(props: MessageFeedViewProps) {
  const {
    entries,
    submitting,
    copiedMessageId,
    collapsedActivityGroups,
    onCopyMessage,
    onCreateWorkItem,
    onActivityGroupToggle,
  } = props;
  const { t } = useTranslation();

  return (
    <>
      {entries.map((entry) => {
        /* ── thought: italic one-liner ── */
        if (entry.type === "thought") {
          const act = entry.item.data;
          return (
            <div key={act.id} className="flex items-start gap-1.5 py-0.5 text-xs text-violet-500">
              <Brain className="mt-px h-3.5 w-3.5 shrink-0" />
              <span className="min-w-0 italic">{compactText(act.detail || act.title, 200)}</span>
            </div>
          );
        }

        /* ── tool_group: collapsible compact block ── */
        if (entry.type === "tool_group") {
          const isCollapsed = collapsedActivityGroups[entry.id] !== false;
          const count = entry.items.length;
          return (
            <div key={entry.id} className="py-0.5">
              <button
                type="button"
                className="flex items-center gap-1 text-xs text-muted-foreground transition-colors hover:text-foreground"
                onClick={() => onActivityGroupToggle(entry.id)}
              >
                {isCollapsed ? <ChevronRight className="h-3 w-3" /> : <ChevronDown className="h-3 w-3" />}
                <Wrench className="h-3 w-3 text-amber-500" />
                <span>{count} {t("chat.toolCalls").toLowerCase()}</span>
              </button>
              {!isCollapsed && (
                <div className="ml-4 mt-0.5 space-y-px border-l border-muted pl-2">
                  {entry.items.map((item) => {
                    const act = item.data;
                    return (
                      <div key={act.id} className="flex items-center gap-1.5 text-xs text-muted-foreground">
                        <Wrench className="h-3 w-3 shrink-0 text-amber-500/70" />
                        <span className="truncate font-medium text-foreground/80">{act.title}</span>
                        {act.detail && <span className="truncate text-muted-foreground/60">— {compactText(act.detail, 60)}</span>}
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          );
        }

        /* ── message ── */
        const message = entry.item.data;
        const isUser = message.role === "user";
        return (
          <div
            key={message.id}
            {...(isUser ? { "data-user-msg": "true" } : {})}
            className={cn(
              "group/msg rounded-sm py-1.5",
              isUser ? "bg-blue-50/60" : "",
            )}
          >
            <div className="flex items-start gap-2">
              <span className={cn(
                "shrink-0 select-none text-xs font-bold tracking-wide",
                isUser ? "text-blue-600" : "text-emerald-600",
              )}>
                {isUser ? "❯ You" : "⦿ Agent"}
              </span>
              <span className="shrink-0 text-[10px] text-muted-foreground/50">{message.time}</span>
              {!isUser && (
                <div className="ml-auto flex shrink-0 items-center gap-1.5 opacity-0 transition-opacity group-hover/msg:opacity-100">
                  <button
                    type="button"
                    className={cn(
                      "flex h-6 w-6 items-center justify-center rounded transition-colors",
                      copiedMessageId === message.id ? "text-emerald-600" : "text-muted-foreground hover:text-foreground",
                    )}
                    title={t("chat.copy")}
                    onClick={() => onCopyMessage(message.id, message.content)}
                  >
                    {copiedMessageId === message.id ? <Check className="h-3.5 w-3.5" /> : <ClipboardCopy className="h-3.5 w-3.5" />}
                  </button>
                  <button
                    type="button"
                    className="flex h-6 w-6 items-center justify-center rounded text-muted-foreground transition-colors hover:text-amber-600"
                    title={t("chat.createWorkItem")}
                    onClick={() => onCreateWorkItem(message.id, message.content)}
                  >
                    <ListTodo className="h-3.5 w-3.5" />
                  </button>
                </div>
              )}
            </div>
            <div className={cn(
              "mt-0.5 whitespace-pre-wrap text-sm leading-relaxed",
              isUser ? "border-l-2 border-blue-300 pl-3 text-foreground" : "border-l-2 border-emerald-200 pl-3 text-foreground/90",
            )}>
              {message.content}
            </div>
          </div>
        );
      })}
      {submitting && (
        <div className="flex items-center gap-1.5 py-1 text-xs text-muted-foreground">
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
          <span>{t("chat.thinking")}...</span>
        </div>
      )}
    </>
  );
}
