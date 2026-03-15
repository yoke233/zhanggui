import type { RefObject, ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Bot, CheckCircle2, Loader2, User, XCircle } from "lucide-react";
import { cn } from "@/lib/utils";
import type { AgentProfile, ThreadMessage } from "@/types/apiV2";

interface ThreadMessageListProps {
  messages: ThreadMessage[];
  profileByID: Map<string, AgentProfile>;
  thinkingAgentIDs: Set<string>;
  sending: boolean;
  messagesEndRef: RefObject<HTMLDivElement>;
  renderMessageContent: (msg: ThreadMessage) => ReactNode;
  focusAgentProfile: (profileID: string) => void;
  readTargetAgentID: (metadata: Record<string, unknown> | undefined) => string | null;
  readAutoRoutedTo: (metadata: Record<string, unknown> | undefined) => string[];
  readTaskGroupID: (metadata: Record<string, unknown> | undefined) => number | null;
  readMetadataType: (metadata: Record<string, unknown> | undefined) => string | null;
  formatRelativeTime: (value: string) => string;
}

export function ThreadMessageList({
  messages,
  profileByID,
  thinkingAgentIDs,
  sending,
  messagesEndRef,
  renderMessageContent,
  focusAgentProfile,
  readTargetAgentID,
  readAutoRoutedTo,
  readTaskGroupID,
  readMetadataType,
  formatRelativeTime,
}: ThreadMessageListProps) {
  const { t } = useTranslation();

  if (messages.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 text-muted-foreground">
        <Bot className="h-10 w-10 text-muted-foreground/30" />
        <p className="text-sm">{t("threads.noMessages", "No messages yet. Start the conversation.")}</p>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-3xl space-y-4">
      {messages.map((msg) => {
        const isAgent = msg.role === "agent";
        const isSystem = msg.role === "system";
        const targetAgent = readTargetAgentID(msg.metadata);
        const autoRoutedTo = readAutoRoutedTo(msg.metadata);
        const taskGroupID = readTaskGroupID(msg.metadata);
        const metaType = readMetadataType(msg.metadata);
        const profile = isAgent ? profileByID.get(msg.sender_id) : undefined;

        // Render task group progress card
        if (isSystem && metaType === "task_group_progress") {
          return <TaskGroupProgressCard key={msg.id} msg={msg} />;
        }

        // Render task group completed card
        if (isSystem && metaType === "task_group_completed") {
          const finalStatus = (msg.metadata?.final_status as string) ?? "done";
          const isFailed = finalStatus === "failed";
          return (
            <div key={msg.id} className="flex justify-center">
              <div className={cn(
                "flex items-center gap-2 rounded-full border px-4 py-1.5 text-xs",
                isFailed
                  ? "border-rose-200 bg-rose-50 text-rose-700"
                  : "border-emerald-200 bg-emerald-50 text-emerald-700",
              )}>
                {isFailed ? <XCircle className="h-3 w-3" /> : <CheckCircle2 className="h-3 w-3" />}
                <span>{msg.content || `Task Group #${taskGroupID} ${isFailed ? "failed" : "completed"}`}</span>
              </div>
            </div>
          );
        }

        // Render task output / review cards
        if (isAgent && (metaType === "task_output" || metaType === "task_review_approved" || metaType === "task_review_rejected")) {
          const outputFile = (msg.metadata?.output_file as string) ?? "";
          const isReject = metaType === "task_review_rejected";
          const isApproved = metaType === "task_review_approved";
          return (
            <div key={msg.id} className="flex gap-3">
              <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-emerald-100 text-emerald-700">
                <Bot className="h-4 w-4" />
              </div>
              <div className="max-w-[75%] min-w-0">
                <div className="mb-1 flex items-center gap-1.5 text-[11px] text-muted-foreground">
                  <span className="font-medium text-foreground/70">{profile?.name ?? msg.sender_id}</span>
                  {taskGroupID ? (
                    <span className="rounded bg-purple-50 px-1 py-px text-[10px] text-purple-700">
                      Group #{taskGroupID}
                    </span>
                  ) : null}
                  <span>{formatRelativeTime(msg.created_at)}</span>
                </div>
                <div className={cn(
                  "rounded-2xl rounded-tl-md px-4 py-2.5 text-sm leading-relaxed",
                  isReject ? "border border-rose-200 bg-rose-50/50 text-foreground" :
                  isApproved ? "border border-emerald-200 bg-emerald-50/50 text-foreground" :
                  "bg-muted/80 text-foreground",
                )}>
                  <p className="whitespace-pre-wrap break-words">{msg.content}</p>
                  {outputFile && (
                    <p className="mt-1.5 text-xs text-muted-foreground">
                      📄 {outputFile}
                    </p>
                  )}
                </div>
              </div>
            </div>
          );
        }

        if (isSystem) {
          return (
            <div key={msg.id} className="flex justify-center">
              <div className="flex items-center gap-2 rounded-full border border-border/40 bg-muted/40 px-4 py-1.5 text-xs text-muted-foreground">
                {taskGroupID ? (
                  <span className="rounded-full bg-background px-2 py-0.5 text-[10px] font-medium text-foreground/70">
                    Group #{taskGroupID}
                  </span>
                ) : null}
                <Bot className="h-3 w-3" />
                <span>{msg.content}</span>
              </div>
            </div>
          );
        }

        return (
          <div key={msg.id} className={cn("flex gap-3", !isAgent && "flex-row-reverse")}>
            <div
              className={cn(
                "flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-xs font-bold",
                isAgent ? "bg-emerald-100 text-emerald-700" : "bg-blue-100 text-blue-700",
              )}
            >
              {isAgent ? <Bot className="h-4 w-4" /> : <User className="h-4 w-4" />}
            </div>
            <div className="group/msg max-w-[75%] min-w-0">
              <div
                className={cn(
                  "mb-1 flex items-center gap-1.5 text-[11px] text-muted-foreground",
                  !isAgent && "flex-row-reverse",
                )}
              >
                <span className="font-medium text-foreground/70">
                  {isAgent ? (profile?.name ?? msg.sender_id) : (msg.sender_id || "You")}
                </span>
                {targetAgent ? (
                  <span className="rounded bg-blue-50 px-1 py-px text-[10px] text-blue-600">
                    @{targetAgent}
                  </span>
                ) : null}
                {taskGroupID ? (
                  <span className="rounded bg-purple-50 px-1 py-px text-[10px] text-purple-700">
                    Group #{taskGroupID}
                  </span>
                ) : null}
                <span>{formatRelativeTime(msg.created_at)}</span>
              </div>
              <div
                className={cn(
                  "rounded-2xl px-4 py-2.5 text-sm leading-relaxed",
                  isAgent ? "rounded-tl-md bg-muted/80 text-foreground" : "rounded-tr-md bg-blue-600 text-white",
                )}
              >
                <p className="whitespace-pre-wrap break-words">{renderMessageContent(msg)}</p>
              </div>
              {!isAgent && autoRoutedTo.length > 0 && (
                <div className="mt-1 flex flex-wrap items-center justify-end gap-1 text-[10px]">
                  <span className="text-muted-foreground/60">Auto</span>
                  <span className="text-muted-foreground/40">→</span>
                  {autoRoutedTo.map((agentID) => {
                    const agentProfile = profileByID.get(agentID);
                    return (
                      <button
                        key={agentID}
                        type="button"
                        className="inline-flex items-center gap-1 rounded-full border border-emerald-200 bg-emerald-50 px-1.5 py-0.5 font-medium text-emerald-700 transition-colors hover:bg-emerald-100"
                        onClick={() => focusAgentProfile(agentID)}
                      >
                        <Bot className="h-2.5 w-2.5" />
                        {agentProfile?.name ?? agentID}
                      </button>
                    );
                  })}
                </div>
              )}
            </div>
          </div>
        );
      })}

      {thinkingAgentIDs.size > 0 && (
        <div className="flex flex-col gap-2">
          {[...thinkingAgentIDs].map((agentID) => {
            const profile = profileByID.get(agentID);
            return (
              <div key={agentID} className="flex items-center gap-3">
                <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-emerald-100 text-emerald-700">
                  <Bot className="h-4 w-4" />
                </div>
                <div className="flex items-center gap-2 rounded-2xl rounded-tl-md bg-muted/60 px-4 py-2.5">
                  <span className="text-xs font-medium text-muted-foreground">
                    {profile?.name ?? agentID}
                  </span>
                  <span className="inline-flex items-center gap-0.5">
                    <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-muted-foreground/50" style={{ animationDelay: "0ms" }} />
                    <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-muted-foreground/50" style={{ animationDelay: "150ms" }} />
                    <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-muted-foreground/50" style={{ animationDelay: "300ms" }} />
                  </span>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {sending && thinkingAgentIDs.size === 0 && (
        <div className="flex items-center gap-2 px-11 text-xs text-muted-foreground">
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
          <span>{t("threads.sending", "Sending")}...</span>
        </div>
      )}

      <div ref={messagesEndRef} />
    </div>
  );
}

/* ── Task Group Progress Card (inline DAG visualization) ── */

function TaskGroupProgressCard({ msg }: { msg: ThreadMessage }) {
  const tasks = (msg.metadata?.tasks as Array<Record<string, unknown>>) ?? [];
  const groupStatus = (msg.metadata?.group_status as string) ?? "pending";
  const groupId = msg.metadata?.task_group_id as number;

  const statusIcon = (status: string) => {
    switch (status) {
      case "done": return "✅";
      case "running": return "🔄";
      case "ready": return "⏳";
      case "failed": return "❌";
      case "rejected": return "↩️";
      default: return "○";
    }
  };

  const doneCount = tasks.filter((t) => t.status === "done").length;

  return (
    <div className="flex justify-center">
      <div className="w-full max-w-md rounded-lg border border-border/60 bg-muted/20 px-4 py-3">
        <div className="mb-2 flex items-center justify-between">
          <span className="text-xs font-semibold text-foreground/80">
            Task Group #{groupId}
          </span>
          <span className={cn(
            "rounded-full px-2 py-0.5 text-[10px] font-medium",
            groupStatus === "done" ? "bg-emerald-100 text-emerald-700" :
            groupStatus === "running" ? "bg-blue-100 text-blue-700" :
            groupStatus === "failed" ? "bg-rose-100 text-rose-700" :
            "bg-muted text-muted-foreground",
          )}>
            {groupStatus} · {doneCount}/{tasks.length}
          </span>
        </div>
        <div className="space-y-1">
          {tasks.map((task) => (
            <div key={task.id as number} className="flex items-center gap-2 text-xs">
              <span>{statusIcon(task.status as string)}</span>
              <span className="font-medium text-foreground/70">{task.assignee as string}</span>
              <span className="truncate text-muted-foreground">{task.instruction as string}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
