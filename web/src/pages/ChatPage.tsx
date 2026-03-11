import { useEffect, useMemo, useRef, useState } from "react";
import {
  Search,
  Send,
  Plus,
  Bot,
  User,
  MoreHorizontal,
  FolderOpen,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { useV2Workbench } from "@/contexts/V2WorkbenchContext";
import { cn } from "@/lib/utils";
import { getErrorMessage } from "@/lib/v2Workbench";

interface ChatMessage {
  id: string;
  role: "user" | "agent";
  content: string;
  time: string;
}

interface SessionRecord {
  id: string;
  title: string;
  status: "active" | "idle" | "closed";
  updatedAt: string;
  messages: ChatMessage[];
}

const CHAT_STORAGE_KEY = "ai-workflow-live-chat-sessions";

const readSessionsFromStorage = (): SessionRecord[] => {
  if (typeof window === "undefined") {
    return [];
  }
  try {
    const raw = window.localStorage.getItem(CHAT_STORAGE_KEY);
    if (!raw) {
      return [];
    }
    const parsed = JSON.parse(raw) as SessionRecord[];
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
};

export function ChatPage() {
  const { apiClient, selectedProject, selectedProjectId } = useV2Workbench();
  const [sessions, setSessions] = useState<SessionRecord[]>(() => readSessionsFromStorage());
  const [activeSession, setActiveSession] = useState<string | null>(() => readSessionsFromStorage()[0]?.id ?? null);
  const [sessionSearch, setSessionSearch] = useState("");
  const [messageInput, setMessageInput] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }
    window.localStorage.setItem(CHAT_STORAGE_KEY, JSON.stringify(sessions));
  }, [sessions]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [sessions, activeSession]);

  const currentSession = useMemo(
    () => sessions.find((session) => session.id === activeSession) ?? null,
    [activeSession, sessions],
  );

  const filteredSessions = useMemo(
    () =>
      sessions.filter((session) =>
        session.title.toLowerCase().includes(sessionSearch.toLowerCase()),
      ),
    [sessionSearch, sessions],
  );

  const createSession = (): string => {
    const createdAt = new Date().toISOString();
    const id = `local-${Date.now()}`;
    const nextSession: SessionRecord = {
      id,
      title: "新会话",
      status: "idle",
      updatedAt: createdAt,
      messages: [],
    };
    setSessions((current) => [nextSession, ...current]);
    setActiveSession(id);
    setMessageInput("");
    return id;
  };

  const appendMessage = (sessionId: string, message: ChatMessage, status: SessionRecord["status"]) => {
    setSessions((current) =>
      current.map((session) =>
        session.id === sessionId
          ? {
              ...session,
              status,
              updatedAt: new Date().toISOString(),
              title: session.title === "新会话" && message.role === "user"
                ? message.content.slice(0, 24)
                : session.title,
              messages: [...session.messages, message],
            }
          : session,
      ),
    );
  };

  const sendMessage = async () => {
    const content = messageInput.trim();
    if (!content) {
      return;
    }

    let sessionId = activeSession;
    if (!sessionId) {
      sessionId = createSession();
    }

    const currentSessionId = sessionId || activeSession;
    if (!currentSessionId) {
      return;
    }

    const userMessage: ChatMessage = {
      id: `${currentSessionId}-user-${Date.now()}`,
      role: "user",
      content,
      time: new Date().toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" }),
    };
    appendMessage(currentSessionId, userMessage, "active");
    setMessageInput("");
    setSubmitting(true);
    setError(null);

    try {
      const response = await apiClient.chat({
        session_id: currentSessionId.startsWith("local-") ? undefined : currentSessionId,
        message: content,
      });

      setSessions((current) => {
        const exists = current.some((session) => session.id === currentSessionId);
        const targetId = response.session_id;
        const next = exists
          ? current.map((session) =>
              session.id === currentSessionId
                ? {
                    ...session,
                    id: targetId,
                    status: "idle" as const,
                    updatedAt: new Date().toISOString(),
                    title: session.title === "新会话" ? content.slice(0, 24) : session.title,
                  }
                : session,
            )
          : current;
        return next;
      });
      setActiveSession(response.session_id);

      appendMessage(response.session_id, {
        id: `${response.session_id}-agent-${Date.now()}`,
        role: "agent",
        content: response.reply,
        time: new Date().toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" }),
      }, "idle");
    } catch (sendError) {
      setError(getErrorMessage(sendError));
      setSessions((current) =>
        current.map((session) =>
          session.id === currentSessionId
            ? { ...session, status: "idle" }
            : session,
        ),
      );
    } finally {
      setSubmitting(false);
    }
  };

  const closeSession = async () => {
    if (!currentSession || currentSession.id.startsWith("local-")) {
      return;
    }
    try {
      await apiClient.closeChat(currentSession.id);
      setSessions((current) =>
        current.map((session) =>
          session.id === currentSession.id ? { ...session, status: "closed" } : session,
        ),
      );
    } catch (closeError) {
      setError(getErrorMessage(closeError));
    }
  };

  const currentMessages = currentSession?.messages ?? [];

  return (
    <div className="flex h-full overflow-hidden">
      <div className="flex w-72 flex-col border-r bg-sidebar">
        <div className="border-b p-3">
          <div className="mb-3 flex items-center justify-between">
            <h2 className="text-sm font-semibold">会话列表</h2>
            <Button variant="ghost" size="icon" className="h-7 w-7" onClick={createSession}>
              <Plus className="h-4 w-4" />
            </Button>
          </div>
          <div className="relative">
            <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder="搜索会话..."
              className="h-8 pl-8 text-xs"
              value={sessionSearch}
              onChange={(event) => setSessionSearch(event.target.value)}
            />
          </div>
        </div>

        <div className="flex-1 overflow-y-auto">
          {filteredSessions.map((session) => {
            const preview = session.messages[session.messages.length - 1]?.content ?? "暂无消息";
            return (
              <button
                key={session.id}
                onClick={() => setActiveSession(session.id)}
                className={cn(
                  "w-full border-b px-3 py-3 text-left transition-colors",
                  activeSession === session.id ? "bg-accent" : "hover:bg-muted/50",
                )}
              >
                <div className="flex items-center justify-between">
                  <span className="truncate text-sm font-medium">{session.title}</span>
                  <span className="ml-2 shrink-0 text-[10px] text-muted-foreground">
                    {new Date(session.updatedAt).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })}
                  </span>
                </div>
                <div className="mt-1 flex items-center gap-1.5">
                  <div
                    className={cn(
                      "h-1.5 w-1.5 shrink-0 rounded-full",
                      session.status === "active"
                        ? "bg-emerald-500"
                        : session.status === "idle"
                          ? "bg-amber-500"
                          : "bg-zinc-300",
                    )}
                  />
                  <p className="truncate text-xs text-muted-foreground">{preview}</p>
                </div>
              </button>
            );
          })}
        </div>
      </div>

      <div className="flex flex-1 flex-col">
        <div className="flex items-center justify-between border-b px-5 py-3">
          <div className="flex items-center gap-3">
            <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary text-primary-foreground">
              <Bot className="h-4 w-4" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <span className="text-sm font-semibold">{currentSession?.title ?? "Lead Agent"}</span>
                <Badge
                  variant={
                    currentSession?.status === "active"
                      ? "success"
                      : currentSession?.status === "idle"
                        ? "warning"
                        : "secondary"
                  }
                  className="text-[10px]"
                >
                  {currentSession?.status === "active"
                    ? "活跃"
                    : currentSession?.status === "idle"
                      ? "空闲"
                      : "已关闭"}
                </Badge>
              </div>
              <p className="text-xs text-muted-foreground">
                当前项目：{selectedProject?.name ?? "未指定"}{selectedProjectId ? ` · project_id=${selectedProjectId}` : ""}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => void closeSession()}>
              <MoreHorizontal className="h-4 w-4" />
            </Button>
          </div>
        </div>

        {error ? <p className="mx-5 mt-4 rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p> : null}

        <div className="flex-1 space-y-4 overflow-y-auto px-5 py-4">
          {currentMessages.length === 0 ? (
            <div className="rounded-lg border border-dashed p-6 text-sm text-muted-foreground">
              还没有消息。当前页已接入真实 `/api/v2/chat`，但后端没有“会话列表”接口，所以左侧列表是浏览器本地保存的最近会话。
            </div>
          ) : (
            currentMessages.map((message) => (
              <div
                key={message.id}
                className={cn(
                  "flex max-w-[720px] gap-3",
                  message.role === "user" ? "ml-auto flex-row-reverse" : "",
                )}
              >
                <div
                  className={cn(
                    "flex h-8 w-8 shrink-0 items-center justify-center rounded-full",
                    message.role === "user" ? "bg-zinc-200" : "bg-primary text-primary-foreground",
                  )}
                >
                  {message.role === "user" ? <User className="h-4 w-4" /> : <Bot className="h-4 w-4" />}
                </div>
                <div className={cn("space-y-2", message.role === "user" ? "text-right" : "")}>
                  <div
                    className={cn(
                      "rounded-lg px-4 py-3 text-sm leading-relaxed",
                      message.role === "user" ? "bg-primary text-primary-foreground" : "bg-muted",
                    )}
                  >
                    {message.content.split("\n").map((line, index) => (
                      <span key={`${message.id}-${index}`} className="block">{line}</span>
                    ))}
                  </div>
                  <span className="text-[10px] text-muted-foreground">{message.time}</span>
                </div>
              </div>
            ))
          )}
          <div ref={messagesEndRef} />
        </div>

        <div className="border-t p-4">
          <div className="flex items-end gap-3">
            <div className="relative flex-1">
              <Input
                placeholder="输入消息，与 Lead Agent 对话..."
                className="pr-10"
                value={messageInput}
                onChange={(event) => setMessageInput(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter" && !event.shiftKey) {
                    event.preventDefault();
                    void sendMessage();
                  }
                }}
              />
            </div>
            <Button size="icon" className="h-10 w-10 shrink-0" disabled={submitting} onClick={() => void sendMessage()}>
              <Send className="h-4 w-4" />
            </Button>
          </div>
          <div className="mt-2 flex items-center gap-4 text-[10px] text-muted-foreground">
            <span>Enter 发送 · Shift+Enter 换行</span>
            <span className="flex items-center gap-1">
              <FolderOpen className="h-3 w-3" />
              后端当前只提供 chat 会话接口，不提供会话列表，左侧记录存储在浏览器本地
            </span>
          </div>
        </div>
      </div>
    </div>
  );
}
