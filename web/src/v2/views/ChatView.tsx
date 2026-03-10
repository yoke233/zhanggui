import { useEffect, useMemo, useRef, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import type { ApiClientV2 } from "@/lib/apiClientV2";
import { PageScaffold } from "@/v3/components/PageScaffold";

interface ChatViewProps {
  apiClient: ApiClientV2;
  apiBaseUrl: string;
  getToken: () => string | null;
  defaultWorkDir?: string;
}

interface ChatMessageItem {
  id: string;
  role: "user" | "assistant" | "system";
  content: string;
  time: string;
}

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error && error.message.trim().length > 0) {
    return error.message;
  }
  return "请求失败，请稍后重试";
};

const formatTime = (value?: string): string => {
  if (!value) {
    return new Date().toLocaleString("zh-CN", { hour12: false });
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString("zh-CN", { hour12: false });
};

const ChatView = ({ apiClient, apiBaseUrl, getToken, defaultWorkDir }: ChatViewProps) => {
  const [sessionId, setSessionId] = useState("");
  const [workDir, setWorkDir] = useState(defaultWorkDir ?? "");
  const [message, setMessage] = useState("");
  const [status, setStatus] = useState<"unknown" | "not_found" | "alive" | "running">("unknown");
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [history, setHistory] = useState<ChatMessageItem[]>([]);

  const inputRef = useRef<HTMLTextAreaElement | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const streamingAssistantIdRef = useRef<string | null>(null);
  const lastFinalAssistantRef = useRef<string | null>(null);

  const canOperate = useMemo(() => sessionId.trim().length > 0, [sessionId]);

  useEffect(() => {
    if (!defaultWorkDir) {
      return;
    }
    setWorkDir((current) => (current.trim().length > 0 ? current : defaultWorkDir));
  }, [defaultWorkDir]);

  const fetchStatus = async (targetSessionId: string) => {
    const trimmed = targetSessionId.trim();
    if (!trimmed) {
      setStatus("unknown");
      return;
    }
    const resp = await apiClient.getChatStatus(trimmed);
    const normalized = String(resp.status ?? "").trim().toLowerCase();
    if (normalized === "not_found" || normalized === "alive" || normalized === "running") {
      setStatus(normalized as typeof status);
      return;
    }
    setStatus("unknown");
  };

  useEffect(() => {
    let cancelled = false;
    const run = async () => {
      if (!sessionId.trim()) {
        setStatus("unknown");
        return;
      }
      try {
        await fetchStatus(sessionId);
      } catch {
        if (!cancelled) {
          setStatus("unknown");
        }
      }
    };
    void run();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId]);

  const appendItem = (item: Omit<ChatMessageItem, "time">) => {
    setHistory((current) => [
      ...current,
      {
        ...item,
        time: formatTime(),
      },
    ]);
  };

  const buildWsUrl = (baseUrl: string, token: string, types: string[]): string => {
    const url = (() => {
      if (/^wss?:\/\//.test(baseUrl) || /^https?:\/\//.test(baseUrl)) {
        return new URL(baseUrl);
      }
      if (typeof window !== "undefined" && window.location?.origin) {
        return new URL(baseUrl, window.location.origin);
      }
      return new URL(baseUrl, "http://localhost");
    })();

    if (url.protocol === "http:") {
      url.protocol = "ws:";
    } else if (url.protocol === "https:") {
      url.protocol = "wss:";
    }

    const normalizedPath = url.pathname.replace(/\/+$/, "");
    url.pathname = normalizedPath.endsWith("/ws") ? normalizedPath : `${normalizedPath}/ws`;
    url.searchParams.set("token", token);
    url.searchParams.set("types", types.join(","));
    return url.toString();
  };

  const upsertStreamingAssistant = (chunk: string) => {
    setHistory((current) => {
      const trimmed = chunk;
      if (!trimmed) {
        return current;
      }

      const streamingId = streamingAssistantIdRef.current;
      if (!streamingId) {
        const id = `assistant-stream-${Date.now()}`;
        streamingAssistantIdRef.current = id;
        return [
          ...current,
          {
            id,
            role: "assistant",
            content: trimmed,
            time: formatTime(),
          },
        ];
      }

      return current.map((item) => {
        if (item.id !== streamingId) {
          return item;
        }
        return {
          ...item,
          content: item.content + trimmed,
        };
      });
    });
  };

  const finalizeAssistantMessage = (content: string) => {
    const trimmed = String(content ?? "");
    if (trimmed && lastFinalAssistantRef.current === trimmed) {
      return;
    }
    setHistory((current) => {
      const streamingId = streamingAssistantIdRef.current;
      if (!streamingId) {
        lastFinalAssistantRef.current = trimmed;
        return [
          ...current,
          {
            id: `assistant-${Date.now()}`,
            role: "assistant",
            content: trimmed,
            time: formatTime(),
          },
        ];
      }
      streamingAssistantIdRef.current = null;
      if (trimmed) {
        lastFinalAssistantRef.current = trimmed;
      }
      return current.map((item) => {
        if (item.id !== streamingId) {
          return item;
        }
        return {
          ...item,
          content: trimmed || item.content,
        };
      });
    });
  };

  const appendSystemEvent = (content: string) => {
    appendItem({
      id: `system-${Date.now()}`,
      role: "system",
      content,
    });
  };

  useEffect(() => {
    const token = getToken();
    const sid = sessionId.trim();
    if (!token || !apiBaseUrl.trim() || !sid) {
      return;
    }

    const closeCurrent = () => {
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
      if (wsRef.current) {
        wsRef.current.close(1000, "session_changed");
        wsRef.current = null;
      }
    };

    closeCurrent();

    let cancelled = false;
    const connect = () => {
      if (cancelled) {
        return;
      }
      const wsUrl = buildWsUrl(apiBaseUrl, token, ["chat.output"]);
      const socket = new WebSocket(wsUrl);
      wsRef.current = socket;

      socket.onopen = () => {
        appendSystemEvent("WebSocket 已连接（chat.output）。");
      };

      socket.onmessage = (event) => {
        if (typeof event.data !== "string") {
          return;
        }
        let payload: unknown;
        try {
          payload = JSON.parse(event.data);
        } catch {
          return;
        }
        const record = payload && typeof payload === "object" ? (payload as Record<string, unknown>) : null;
        if (!record) {
          return;
        }
        const eventType = String(record.type ?? "");
        if (eventType !== "chat.output") {
          return;
        }
        const data = record.data && typeof record.data === "object" ? (record.data as Record<string, unknown>) : {};
        const targetSession = String(data.session_id ?? "").trim();
        if (!targetSession || targetSession !== sessionId.trim()) {
          return;
        }
        const subType = String(data.type ?? "").trim();
        const content = String(data.content ?? "");

        switch (subType) {
          case "agent_message_chunk":
            upsertStreamingAssistant(content);
            break;
          case "agent_message":
          case "done":
            finalizeAssistantMessage(content);
            break;
          case "tool_call":
            appendSystemEvent(content ? `tool_call: ${content}` : "tool_call");
            break;
          case "tool_call_completed": {
            const exitCode = typeof data.exit_code === "number" ? data.exit_code : undefined;
            const stdout = String(data.content ?? "").trim();
            const stderr = String(data.stderr ?? "").trim();
            const headline = exitCode != null ? `tool_call_completed (exit=${exitCode})` : "tool_call_completed";
            const detail = [stdout && `stdout: ${stdout}`, stderr && `stderr: ${stderr}`]
              .filter(Boolean)
              .join("\n");
            appendSystemEvent(detail ? `${headline}\n${detail}` : headline);
            break;
          }
          default:
            if (subType.endsWith("_chunk")) {
              // Avoid flooding the UI with non-message chunks (thought/usage/etc.)
              return;
            }
            if (subType) {
              appendSystemEvent(content ? `${subType}: ${content}` : subType);
            }
        }
      };

      socket.onclose = () => {
        wsRef.current = null;
        if (cancelled) {
          return;
        }
        reconnectTimerRef.current = setTimeout(connect, 1000);
      };

      socket.onerror = () => {
        // onclose will handle reconnect
      };
    };

    connect();

    return () => {
      cancelled = true;
      closeCurrent();
    };
  }, [apiBaseUrl, getToken, sessionId]);

  const handleSend = async () => {
    const trimmedMessage = message.trim();
    if (!trimmedMessage) {
      setError("消息不能为空。");
      return;
    }

    setSending(true);
    setError(null);

    const sessionCandidate = sessionId.trim();
    appendItem({
      id: `user-${Date.now()}`,
      role: "user",
      content: trimmedMessage,
    });

    try {
      setStatus("running");
      const resp = await apiClient.chat({
        session_id: sessionCandidate || undefined,
        message: trimmedMessage,
        work_dir: workDir.trim() || undefined,
      });
      const nextSession = String(resp.session_id ?? "").trim();
      if (nextSession && nextSession !== sessionCandidate) {
        setSessionId(nextSession);
      }
      finalizeAssistantMessage(String(resp.reply ?? ""));
      setMessage("");
      setStatus("alive");
      requestAnimationFrame(() => {
        inputRef.current?.focus();
      });
    } catch (err) {
      setStatus("unknown");
      setError(getErrorMessage(err));
      appendItem({
        id: `system-${Date.now()}`,
        role: "system",
        content: `发送失败：${getErrorMessage(err)}`,
      });
    } finally {
      setSending(false);
    }
  };

  const handleCancel = async () => {
    const trimmed = sessionId.trim();
    if (!trimmed) {
      return;
    }
    setError(null);
    try {
      await apiClient.cancelChat(trimmed);
      await fetchStatus(trimmed);
      appendItem({
        id: `system-${Date.now()}`,
        role: "system",
        content: "已请求取消当前 prompt。",
      });
    } catch (err) {
      setError(getErrorMessage(err));
    }
  };

  const handleClose = async () => {
    const trimmed = sessionId.trim();
    if (!trimmed) {
      return;
    }
    setError(null);
    try {
      await apiClient.closeChat(trimmed);
      setStatus("not_found");
      appendItem({
        id: `system-${Date.now()}`,
        role: "system",
        content: `已关闭会话 ${trimmed}。`,
      });
    } catch (err) {
      setError(getErrorMessage(err));
    }
  };

  return (
    <PageScaffold
      eyebrow="Lead Chat"
      title="聊天（WebSocket 流式）"
      description="对接 /api/v2/chat（自动创建/复用 session），并通过 /api/v2/ws 订阅 chat.output 实时拼接回复。"
      contextTitle={sessionId.trim() ? `session: ${sessionId.trim()}` : "session: 未创建"}
      contextMeta={workDir.trim() ? `work_dir: ${workDir.trim()}` : "work_dir: 未设置（建议选择项目后自动填充）"}
      actions={[
        {
          label: "查询状态",
          onClick: () => {
            if (!sessionId.trim()) return;
            void fetchStatus(sessionId);
          },
          variant: "outline",
        },
        {
          label: "取消 prompt",
          onClick: () => void handleCancel(),
          variant: "outline",
        },
        {
          label: "关闭会话",
          onClick: () => void handleClose(),
          variant: "outline",
        },
      ]}
    >
      <Card className="rounded-2xl border-slate-200 shadow-none">
        <CardHeader className="p-5">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div>
              <Badge variant="secondary" className="bg-indigo-50 text-indigo-600">
                V2 / Chat
              </Badge>
              <CardTitle className="mt-3 text-[24px] font-semibold tracking-[-0.02em]">
                Lead Chat
              </CardTitle>
              <CardDescription className="mt-2 text-slate-600">
                对接 `/api/v2/chat*`：发送消息会自动创建/复用会话；可取消当前 prompt 或关闭会话。
              </CardDescription>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant="outline" className="bg-slate-50 text-slate-600">
                status {status}
              </Badge>
              <Button
                variant="outline"
                size="sm"
                onClick={() => void fetchStatus(sessionId).catch(() => {})}
                disabled={!canOperate}
              >
                刷新状态
              </Button>
              <Button variant="outline" size="sm" onClick={handleCancel} disabled={!canOperate}>
                取消 prompt
              </Button>
              <Button variant="outline" size="sm" onClick={handleClose} disabled={!canOperate}>
                关闭会话
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-4 px-5 pb-5">
          <div className="grid gap-3 md:grid-cols-2">
            <div className="grid gap-1">
              <label htmlFor="v2-chat-session" className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
                Session ID（可选）
              </label>
              <Input
                id="v2-chat-session"
                value={sessionId}
                onChange={(event) => setSessionId(event.target.value)}
                placeholder="留空则自动创建"
              />
            </div>
            <div className="grid gap-1">
              <label htmlFor="v2-chat-workdir" className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
                Work Dir（可选）
              </label>
              <Input
                id="v2-chat-workdir"
                value={workDir}
                onChange={(event) => setWorkDir(event.target.value)}
                placeholder="例如：D:/project/ai-workflow"
              />
            </div>
          </div>

          <div className="grid gap-1">
            <label htmlFor="v2-chat-message" className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
              Message
            </label>
            <Textarea
              id="v2-chat-message"
              ref={inputRef}
              value={message}
              onChange={(event) => setMessage(event.target.value)}
              placeholder="输入消息，回车换行；点击发送执行 prompt"
              className="min-h-[120px]"
            />
          </div>

          {error ? (
            <p className="rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700">
              {error}
            </p>
          ) : null}

          <div className="flex flex-wrap items-center justify-end gap-2">
            <Button
              onClick={handleSend}
              disabled={sending}
            >
              {sending ? "发送中..." : "发送"}
            </Button>
          </div>

          <div className="grid gap-3">
            {history.length === 0 ? (
              <p className="text-sm text-slate-500">暂无消息。发送第一条消息会自动创建会话。</p>
            ) : null}
            {history.map((item) => (
              <div key={item.id} className="rounded-2xl border border-slate-200 bg-white px-4 py-3">
                <div className="flex items-start justify-between gap-3">
                  <div className="flex items-center gap-2">
                    <Badge
                      variant="outline"
                      className={
                        item.role === "user"
                          ? "bg-blue-50 text-blue-700"
                          : item.role === "assistant"
                            ? "bg-emerald-50 text-emerald-700"
                            : "bg-slate-50 text-slate-600"
                      }
                    >
                      {item.role}
                    </Badge>
                    <p className="text-[11px] text-slate-500">{item.time}</p>
                  </div>
                </div>
                <pre className="mt-2 whitespace-pre-wrap text-sm leading-6 text-slate-900">
                  {item.content}
                </pre>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </PageScaffold>
  );
};

export default ChatView;
