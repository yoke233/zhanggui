import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { A2AClient } from "../lib/a2aClient";
import type { WsClient } from "../lib/wsClient";
import type { A2ATask } from "../types/a2a";
import type {
  ACPSessionUpdate,
  ChatEventPayload,
  ChatEventType,
  WsEnvelope,
} from "../types/ws";

interface A2AChatViewProps {
  a2aClient: A2AClient;
  wsClient: WsClient;
  projectId: string;
}

interface LocalMessage {
  id: string;
  role: "user" | "assistant";
  content: string;
  time: string;
}

interface TaskSummary {
  id: string;
  contextId: string;
  state: string;
  updatedAt: string;
  preview: string;
}

interface RunEventItem {
  id: string;
  type: string;
  detail: string;
  time: string;
}

const MAX_RUN_EVENTS = 40;

const CHAT_RUN_EVENT_TYPES = new Set<ChatEventType>([
  "run_started",
  "run_update",
  "run_completed",
  "run_failed",
  "run_cancelled",
  "team_leader_thinking",
  "team_leader_files_changed",
]);

type ChatUpdateParser = (acp: ACPSessionUpdate) => string;

const CHAT_UPDATE_PARSERS: Record<string, ChatUpdateParser> = {
  agent_message_chunk: (acp) => toStringValue(acp.content?.text),
  assistant_message_chunk: (acp) => toStringValue(acp.content?.text),
  message_chunk: (acp) => toStringValue(acp.content?.text),
};

const roleLabel: Record<string, string> = {
  user: "用户",
  assistant: "助手",
};

const roleStyle: Record<string, string> = {
  user: "bg-slate-900 text-white",
  assistant: "border border-slate-200 bg-white text-slate-900",
};

const toStringValue = (value: unknown): string => {
  if (typeof value !== "string") return "";
  return value.trim();
};

const nowIso = (): string => new Date().toISOString();

const formatTime = (time: string): string => {
  const date = new Date(time);
  if (Number.isNaN(date.getTime())) return time;
  return date.toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
};

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error && error.message.trim().length > 0) return error.message;
  return "请求失败，请稍后重试";
};

const isTerminalState = (state: string): boolean => {
  const normalized = state.trim().toLowerCase();
  return normalized === "completed" || normalized === "failed" || normalized === "canceled";
};

const toSafeTaskState = (task: A2ATask): string => {
  const state = task.status?.state;
  if (typeof state !== "string") return "unknown";
  const trimmed = state.trim();
  return trimmed.length > 0 ? trimmed : "unknown";
};

const toEventTimestampMs = (value: unknown): number => {
  const raw = toStringValue(value);
  if (!raw) return 0;
  const parsed = new Date(raw).getTime();
  return Number.isNaN(parsed) ? 0 : parsed;
};

const getStreamingDelta = (payload: ChatEventPayload): string => {
  const acp = payload.acp;
  if (!acp || typeof acp !== "object") return "";
  const updateType = toStringValue(acp.sessionUpdate);
  if (!updateType) return "";
  const parser = CHAT_UPDATE_PARSERS[updateType];
  return parser ? parser(acp) : "";
};

const buildRunEventDetail = (data: ChatEventPayload): string => {
  const updateType = toStringValue(data.acp?.sessionUpdate);
  if (!updateType) return "收到增量更新";
  const fragments = [updateType];
  const title = toStringValue(data.acp?.title);
  if (title) fragments.push(`title=${title}`);
  const kind = toStringValue(data.acp?.kind);
  if (kind) fragments.push(`kind=${kind}`);
  const status = toStringValue(data.acp?.status);
  if (status) fragments.push(`status=${status}`);
  return fragments.join(" · ");
};

const parseInlineMarkdown = (text: string, keyPrefix: string) => {
  const nodes: Array<string | JSX.Element> = [];
  const pattern = /`([^`]+)`|\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)|\*\*([^*]+)\*\*|(\*[^*]+\*)/g;
  let lastIndex = 0;
  let matchIndex = 0;
  let match = pattern.exec(text);
  while (match) {
    if (match.index > lastIndex) nodes.push(text.slice(lastIndex, match.index));
    if (match[1]) {
      nodes.push(
        <code key={`${keyPrefix}-c-${matchIndex}`} className="rounded bg-slate-100 px-1 py-0.5 font-mono text-[0.9em] text-slate-900">
          {match[1]}
        </code>,
      );
    } else if (match[2] && match[3]) {
      nodes.push(
        <a key={`${keyPrefix}-a-${matchIndex}`} href={match[3]} target="_blank" rel="noreferrer" className="text-sky-700 underline">
          {match[2]}
        </a>,
      );
    } else if (match[4]) {
      nodes.push(<strong key={`${keyPrefix}-b-${matchIndex}`} className="font-semibold">{match[4]}</strong>);
    } else if (match[5]) {
      nodes.push(<em key={`${keyPrefix}-e-${matchIndex}`} className="italic">{match[5].slice(1, -1)}</em>);
    }
    lastIndex = match.index + match[0].length;
    matchIndex += 1;
    match = pattern.exec(text);
  }
  if (lastIndex < text.length) nodes.push(text.slice(lastIndex));
  if (nodes.length === 0) nodes.push(text);
  return nodes;
};

const renderBasicMarkdown = (content: string, keyPrefix: string): JSX.Element[] => {
  const lines = content.replace(/\r\n/g, "\n").split("\n");
  const elements: JSX.Element[] = [];
  let index = 0;
  while (index < lines.length) {
    const line = (lines[index] ?? "").trim();
    if (!line) { index += 1; continue; }

    if (line.startsWith("```")) {
      const codeLines: string[] = [];
      index += 1;
      while (index < lines.length && !(lines[index] ?? "").trim().startsWith("```")) {
        codeLines.push(lines[index] ?? "");
        index += 1;
      }
      index += 1;
      elements.push(
        <pre key={`${keyPrefix}-code-${index}`} className="overflow-x-auto rounded-md bg-slate-900 p-2 text-xs text-slate-100">
          <code>{codeLines.join("\n")}</code>
        </pre>,
      );
      continue;
    }

    const headingMatch = line.match(/^(#{1,6})\s+(.+)$/);
    if (headingMatch) {
      const HeadingTag = `h${headingMatch[1].length}` as keyof JSX.IntrinsicElements;
      elements.push(
        <HeadingTag key={`${keyPrefix}-h-${index}`} className="font-semibold leading-snug">
          {parseInlineMarkdown(headingMatch[2], `${keyPrefix}-h-${index}`)}
        </HeadingTag>,
      );
      index += 1;
      continue;
    }

    if (/^[-*]\s+/.test(line)) {
      const items: string[] = [];
      while (index < lines.length) {
        const itemMatch = (lines[index] ?? "").trim().match(/^[-*]\s+(.+)$/);
        if (!itemMatch) break;
        items.push(itemMatch[1]);
        index += 1;
      }
      elements.push(
        <ul key={`${keyPrefix}-ul-${index}`} className="list-disc space-y-1 pl-5">
          {items.map((item, i) => (
            <li key={`${keyPrefix}-li-${index}-${i}`}>{parseInlineMarkdown(item, `${keyPrefix}-li-${index}-${i}`)}</li>
          ))}
        </ul>,
      );
      continue;
    }

    const paragraphLines = [line];
    index += 1;
    while (index < lines.length) {
      const nextLine = (lines[index] ?? "").trim();
      if (!nextLine || /^#{1,6}\s+/.test(nextLine) || /^[-*]\s+/.test(nextLine) || nextLine.startsWith("```")) break;
      paragraphLines.push(nextLine);
      index += 1;
    }
    elements.push(
      <p key={`${keyPrefix}-p-${index}`} className="whitespace-pre-wrap">
        {parseInlineMarkdown(paragraphLines.join(" "), `${keyPrefix}-p-${index}`)}
      </p>,
    );
  }

  if (elements.length === 0) {
    elements.push(<p key={`${keyPrefix}-empty`} className="whitespace-pre-wrap">{content}</p>);
  }
  return elements;
};

const TASK_STATE_LABELS: Record<string, string> = {
  submitted: "已提交",
  working: "执行中",
  "input-required": "待反馈",
  completed: "已完成",
  failed: "失败",
  canceled: "已取消",
  unknown: "未知",
};

const TASK_STATE_COLORS: Record<string, string> = {
  submitted: "text-sky-600",
  working: "text-amber-600",
  "input-required": "text-purple-600",
  completed: "text-emerald-600",
  failed: "text-rose-600",
  canceled: "text-slate-500",
  unknown: "text-slate-400",
};

const A2AChatView = ({ a2aClient, wsClient, projectId }: A2AChatViewProps) => {
  const [draft, setDraft] = useState("");
  const [messages, setMessages] = useState<LocalMessage[]>([]);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [taskId, setTaskId] = useState<string | null>(null);
  const [taskState, setTaskState] = useState<string>("unknown");
  const [loading, setLoading] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [streamingText, setStreamingText] = useState("");
  const [isStreaming, setIsStreaming] = useState(false);
  const [runEvents, setRunEvents] = useState<RunEventItem[]>([]);
  const [taskList, setTaskList] = useState<TaskSummary[]>([]);
  const [tasksLoading, setTasksLoading] = useState(false);
  const [tasksError, setTasksError] = useState<string | null>(null);

  const activeRunStartedAtRef = useRef(0);
  const localRunStartPendingRef = useRef(false);
  const messagesEndRef = useRef<HTMLDivElement | null>(null);
  const sessionIdRef = useRef<string | null>(null);
  const subscribedSessionIdRef = useRef<string | null>(null);
  const requestIdRef = useRef(0);
  const taskListRequestIdRef = useRef(0);

  useEffect(() => { sessionIdRef.current = sessionId; }, [sessionId]);

  // Reset on project change
  useEffect(() => {
    requestIdRef.current += 1;
    taskListRequestIdRef.current += 1;
    setDraft("");
    setMessages([]);
    setSessionId(null);
    setTaskId(null);
    setTaskState("unknown");
    setLoading(false);
    setCancelling(false);
    setError(null);
    setStreamingText("");
    setIsStreaming(false);
    setRunEvents([]);
    setTaskList([]);
    setTasksLoading(false);
    setTasksError(null);
    activeRunStartedAtRef.current = 0;
    localRunStartPendingRef.current = false;
  }, [projectId]);

  const syncingFromOtherTerminal = loading && !localRunStartPendingRef.current && isStreaming;
  const canSubmit = loading
    ? !!taskId && !cancelling && !syncingFromOtherTerminal
    : draft.trim().length > 0;

  const hasMessages = messages.length > 0 || isStreaming;

  const pushRunEvent = useCallback((type: string, detail: string) => {
    setRunEvents((prev) => {
      const next = [...prev, {
        id: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
        type,
        detail,
        time: nowIso(),
      }];
      return next.length <= MAX_RUN_EVENTS ? next : next.slice(next.length - MAX_RUN_EVENTS);
    });
  }, []);

  // Load task list
  const refreshTaskList = useCallback(async (targetProjectId: string) => {
    const reqId = taskListRequestIdRef.current + 1;
    taskListRequestIdRef.current = reqId;
    setTasksLoading(true);
    setTasksError(null);
    try {
      const response = await a2aClient.listTasks({
        metadata: { project_id: targetProjectId },
        pageSize: 50,
      });
      if (taskListRequestIdRef.current !== reqId) return;
      const summaries: TaskSummary[] = (response.tasks ?? []).map((task) => ({
        id: task.id,
        contextId: task.contextId ?? "",
        state: toSafeTaskState(task),
        updatedAt: task.status?.timestamp ?? "",
        preview: task.id.slice(0, 12),
      }));
      summaries.sort((a, b) => {
        const ta = a.updatedAt ? new Date(a.updatedAt).getTime() : 0;
        const tb = b.updatedAt ? new Date(b.updatedAt).getTime() : 0;
        return tb - ta;
      });
      setTaskList(summaries);
    } catch (listError) {
      if (taskListRequestIdRef.current !== reqId) return;
      setTasksError(getErrorMessage(listError));
    } finally {
      if (taskListRequestIdRef.current === reqId) setTasksLoading(false);
    }
  }, [a2aClient]);

  useEffect(() => { void refreshTaskList(projectId); }, [projectId, refreshTaskList]);

  // Upsert task in list
  const upsertTaskInList = useCallback((task: A2ATask) => {
    const summary: TaskSummary = {
      id: task.id,
      contextId: task.contextId ?? "",
      state: toSafeTaskState(task),
      updatedAt: task.status?.timestamp ?? nowIso(),
      preview: task.id.slice(0, 12),
    };
    setTaskList((prev) => {
      const next = prev.filter((t) => t.id !== summary.id);
      next.unshift(summary);
      return next;
    });
  }, []);

  // Refresh task state
  const refreshTask = useCallback(async (tId: string) => {
    try {
      const task = await a2aClient.getTask({ id: tId, metadata: { project_id: projectId } });
      upsertTaskInList(task);
      const state = toSafeTaskState(task);
      if (sessionIdRef.current === (task.contextId ?? "")) {
        setTaskState(state);
      }
    } catch {
      // ignore
    }
  }, [a2aClient, projectId, upsertTaskInList]);

  // WS subscription management
  const sendChatSessionSubscription = useCallback(
    (type: "subscribe_chat_session" | "unsubscribe_chat_session", sid: string | null) => {
      const normalized = (sid ?? "").trim();
      if (!normalized) return;
      try {
        wsClient.send({ type, session_id: normalized });
      } catch {
        // ignore if not connected
      }
    },
    [wsClient],
  );

  useEffect(() => {
    const nextSID = (sessionId ?? "").trim();
    const prevSID = subscribedSessionIdRef.current;
    if (prevSID && prevSID !== nextSID) {
      sendChatSessionSubscription("unsubscribe_chat_session", prevSID);
    }
    if (nextSID && prevSID !== nextSID) {
      sendChatSessionSubscription("subscribe_chat_session", nextSID);
    }
    subscribedSessionIdRef.current = nextSID || null;
  }, [sessionId, sendChatSessionSubscription]);

  // Re-subscribe on WS reconnect
  useEffect(() => {
    const unsubscribeStatus = wsClient.onStatusChange((status) => {
      if (status !== "open") return;
      const activeSID = sessionIdRef.current;
      if (!activeSID) return;
      sendChatSessionSubscription("subscribe_chat_session", activeSID);
      subscribedSessionIdRef.current = activeSID;
    });
    return () => { unsubscribeStatus(); };
  }, [sendChatSessionSubscription, wsClient]);

  // Cleanup WS subscription on unmount
  useEffect(() => {
    return () => {
      const activeSID = subscribedSessionIdRef.current;
      if (activeSID) {
        sendChatSessionSubscription("unsubscribe_chat_session", activeSID);
        subscribedSessionIdRef.current = null;
      }
    };
  }, [sendChatSessionSubscription]);

  // WS event handler
  useEffect(() => {
    const unsubscribe = wsClient.subscribe<WsEnvelope>("*", (payload) => {
      const envelope = payload as WsEnvelope<ChatEventPayload>;
      if (!CHAT_RUN_EVENT_TYPES.has(envelope.type as ChatEventType)) return;
      if (envelope.project_id && envelope.project_id.trim().length > 0 && envelope.project_id !== projectId) return;

      const data = (envelope.data ?? envelope.payload ?? {}) as ChatEventPayload;
      const wsSessionID = toStringValue(data.session_id);
      if (!wsSessionID) return;
      const activeSID = sessionIdRef.current;
      if (!activeSID || activeSID !== wsSessionID) return;

      switch (envelope.type as ChatEventType) {
        case "run_started": {
          const startedByLocal = localRunStartPendingRef.current;
          localRunStartPendingRef.current = false;
          activeRunStartedAtRef.current = toEventTimestampMs(data.timestamp) || Date.now();
          setLoading(true);
          setCancelling(false);
          setIsStreaming(true);
          setStreamingText("");
          setError(null);
          if (!startedByLocal) {
            setError(null);
          }
          pushRunEvent("run_started", "运行已开始");
          break;
        }
        case "run_update":
        case "team_leader_thinking":
        case "team_leader_files_changed": {
          const eventTs = toEventTimestampMs(data.timestamp);
          const runStartedAt = activeRunStartedAtRef.current;
          if (runStartedAt === 0) break;
          if (eventTs > 0 && eventTs < runStartedAt) break;
          const delta = getStreamingDelta(data);
          if (delta.length > 0) {
            setStreamingText((prev) => `${prev}${delta}`);
          } else {
            pushRunEvent(String(envelope.type), buildRunEventDetail(data));
          }
          break;
        }
        case "run_completed": {
          localRunStartPendingRef.current = false;
          activeRunStartedAtRef.current = 0;
          const finalText = streamingTextRef.current;
          if (finalText.length > 0) {
            setMessages((prev) => [...prev, {
              id: `${Date.now()}-assistant`,
              role: "assistant",
              content: finalText,
              time: nowIso(),
            }]);
          }
          setLoading(false);
          setCancelling(false);
          setIsStreaming(false);
          setStreamingText("");
          setTaskState("completed");
          pushRunEvent("run_completed", "运行完成");
          if (taskIdRef.current) void refreshTask(taskIdRef.current);
          void refreshTaskList(projectId);
          break;
        }
        case "run_cancelled": {
          localRunStartPendingRef.current = false;
          activeRunStartedAtRef.current = 0;
          setLoading(false);
          setCancelling(false);
          setIsStreaming(false);
          setStreamingText("");
          setTaskState("canceled");
          setError("当前请求已取消");
          pushRunEvent("run_cancelled", "运行已取消");
          void refreshTaskList(projectId);
          break;
        }
        case "run_failed": {
          localRunStartPendingRef.current = false;
          activeRunStartedAtRef.current = 0;
          setLoading(false);
          setCancelling(false);
          setIsStreaming(false);
          setStreamingText("");
          setTaskState("failed");
          const reason = toStringValue(data.error);
          setError(reason || "执行失败");
          pushRunEvent("run_failed", reason || "执行失败");
          void refreshTaskList(projectId);
          break;
        }
        default:
          break;
      }
    });
    return () => { unsubscribe(); };
  }, [projectId, pushRunEvent, refreshTask, refreshTaskList, wsClient]);

  // Refs for accessing latest values in WS callbacks
  const streamingTextRef = useRef("");
  const taskIdRef = useRef<string | null>(null);
  useEffect(() => { streamingTextRef.current = streamingText; }, [streamingText]);
  useEffect(() => { taskIdRef.current = taskId; }, [taskId]);

  // Auto-scroll
  useEffect(() => {
    const endNode = messagesEndRef.current;
    if (endNode && typeof endNode.scrollIntoView === "function") {
      endNode.scrollIntoView({ block: "end" });
    }
  }, [messages, streamingText]);

  const handleSend = async () => {
    if (loading) return;
    const message = draft.trim();
    if (!message) return;

    setError(null);
    setLoading(true);
    setCancelling(false);
    setDraft("");
    setStreamingText("");
    setIsStreaming(false);
    activeRunStartedAtRef.current = 0;
    localRunStartPendingRef.current = true;
    setRunEvents([]);

    const reqId = requestIdRef.current + 1;
    requestIdRef.current = reqId;

    setMessages((prev) => [...prev, {
      id: `${Date.now()}-user`,
      role: "user",
      content: message,
      time: nowIso(),
    }]);

    try {
      const task = await a2aClient.sendMessage({
        message: {
          role: "user",
          parts: [{ kind: "text", text: message }],
          ...(sessionId ? { contextId: sessionId } : {}),
        },
        metadata: { project_id: projectId },
      });

      if (requestIdRef.current !== reqId) return;

      const nextTaskId = typeof task.id === "string" ? task.id.trim() : "";
      const nextSessionId = typeof task.contextId === "string" ? task.contextId.trim() : "";
      const nextState = toSafeTaskState(task);

      setTaskId(nextTaskId || null);
      setTaskState(nextState);
      if (nextSessionId) {
        setSessionId(nextSessionId);
      }

      upsertTaskInList(task);

      if (isTerminalState(nextState)) {
        setLoading(false);
        setCancelling(false);
        localRunStartPendingRef.current = false;
      }
    } catch (requestError) {
      if (requestIdRef.current !== reqId) return;
      setLoading(false);
      setCancelling(false);
      localRunStartPendingRef.current = false;
      setError(getErrorMessage(requestError));
    }
  };

  const handleCancel = async () => {
    if (!loading || cancelling || !taskId) return;
    setCancelling(true);
    setError(null);
    try {
      const task = await a2aClient.cancelTask({ id: taskId, metadata: { project_id: projectId } });
      const nextState = toSafeTaskState(task);
      setTaskState(nextState);
      if (isTerminalState(nextState)) {
        setLoading(false);
        setCancelling(false);
        if (nextState === "canceled") {
          setError("当前请求已取消");
        }
      }
    } catch (requestError) {
      setCancelling(false);
      setError(getErrorMessage(requestError));
    }
  };

  const handleSwitchTask = (task: TaskSummary) => {
    if (loading) return;
    const nextSessionId = task.contextId || null;
    setSessionId(nextSessionId);
    setTaskId(task.id);
    setTaskState(task.state);
    setMessages([]);
    setStreamingText("");
    setIsStreaming(false);
    setError(null);
    setRunEvents([]);
    activeRunStartedAtRef.current = 0;
    localRunStartPendingRef.current = false;
    // If task is in working state, mark as loading
    if (task.state === "working" || task.state === "submitted") {
      setLoading(true);
    } else {
      setLoading(false);
    }
  };

  const handleNewSession = () => {
    if (loading) return;
    setSessionId(null);
    setTaskId(null);
    setTaskState("unknown");
    setMessages([]);
    setStreamingText("");
    setIsStreaming(false);
    setError(null);
    setRunEvents([]);
    activeRunStartedAtRef.current = 0;
    localRunStartPendingRef.current = false;
  };

  const submitLabel = loading
    ? syncingFromOtherTerminal ? "同步中" : "停止"
    : sessionId ? "发送" : "发送并创建会话";

  const visibleRunEvents = useMemo(() => runEvents.slice(-20), [runEvents]);

  // Group tasks by session for the sidebar
  const sessionGroups = useMemo(() => {
    const groups = new Map<string, TaskSummary[]>();
    for (const task of taskList) {
      const key = task.contextId || task.id;
      const existing = groups.get(key) ?? [];
      existing.push(task);
      groups.set(key, existing);
    }
    return Array.from(groups.entries()).map(([key, tasks]) => ({
      sessionId: key,
      tasks,
      latestState: tasks[0]?.state ?? "unknown",
      latestUpdate: tasks[0]?.updatedAt ?? "",
    }));
  }, [taskList]);

  return (
    <section className="grid gap-4 lg:grid-cols-[260px_minmax(0,2fr)_280px]">
      {/* Left sidebar: Task/Session list */}
      <aside className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm lg:min-h-[680px]">
        <div className="flex items-center justify-between">
          <h3 className="text-base font-semibold text-slate-900">会话列表</h3>
          <div className="flex gap-1">
            <button
              type="button"
              className="rounded border border-slate-300 px-2 py-1 text-xs text-slate-700 hover:bg-slate-50 disabled:opacity-50"
              onClick={handleNewSession}
              disabled={loading}
            >
              新建
            </button>
            <button
              type="button"
              className="rounded border border-slate-300 px-2 py-1 text-xs text-slate-700 hover:bg-slate-50 disabled:opacity-50"
              onClick={() => { void refreshTaskList(projectId); }}
              disabled={tasksLoading}
            >
              {tasksLoading ? "..." : "刷新"}
            </button>
          </div>
        </div>

        {tasksError && (
          <p className="mt-2 rounded border border-rose-200 bg-rose-50 px-2 py-1 text-xs text-rose-700">
            {tasksError}
          </p>
        )}

        <div className="mt-3 max-h-[580px] overflow-y-auto">
          {sessionGroups.length > 0 ? (
            <div className="space-y-1">
              {sessionGroups.map((group) => {
                const isActive = sessionId === group.sessionId ||
                  group.tasks.some((t) => t.id === taskId);
                return (
                  <button
                    key={group.sessionId}
                    type="button"
                    className={`w-full rounded-lg px-3 py-2 text-left transition ${
                      isActive
                        ? "bg-slate-900 text-white"
                        : "bg-slate-50 text-slate-700 hover:bg-slate-100"
                    }`}
                    onClick={() => handleSwitchTask(group.tasks[0])}
                    disabled={loading}
                  >
                    <p className="truncate text-sm font-medium">
                      {group.tasks[0]?.id.slice(0, 16) ?? group.sessionId.slice(0, 16)}
                    </p>
                    <div className="mt-1 flex items-center justify-between text-xs">
                      <span className={isActive ? "text-slate-300" : (TASK_STATE_COLORS[group.latestState] ?? "text-slate-400")}>
                        {TASK_STATE_LABELS[group.latestState] ?? group.latestState}
                      </span>
                      {group.latestUpdate && (
                        <span className={isActive ? "text-slate-400" : "text-slate-500"}>
                          {formatTime(group.latestUpdate)}
                        </span>
                      )}
                    </div>
                  </button>
                );
              })}
            </div>
          ) : (
            <p className="text-xs text-slate-500">暂无会话，发送消息创建新会话。</p>
          )}
        </div>
      </aside>

      {/* Center: Chat area */}
      <div className="min-w-0 rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-xl font-bold">A2A Chat</h2>
            <p className="mt-1 text-sm text-slate-600">
              通过 A2A JSON-RPC 发送任务，WS 流式接收执行进度。
            </p>
          </div>
          <div className="flex items-center gap-2 text-xs text-slate-500">
            <span className={`inline-block h-2 w-2 rounded-full ${
              taskState === "working" ? "bg-amber-400 animate-pulse" :
              taskState === "completed" ? "bg-emerald-400" :
              taskState === "failed" ? "bg-rose-400" :
              "bg-slate-300"
            }`} />
            <span>{TASK_STATE_LABELS[taskState] ?? taskState}</span>
          </div>
        </div>

        <div className="mt-4 h-[30rem] rounded-lg border border-slate-200 bg-slate-50 p-3">
          {hasMessages ? (
            <div className="flex h-full flex-col gap-3 overflow-y-auto pr-1">
              {messages.map((message) => (
                <article
                  key={message.id}
                  className={`max-w-[92%] rounded-lg px-3 py-2 text-sm ${
                    roleStyle[message.role] ?? roleStyle.assistant
                  } ${message.role === "user" ? "self-end" : "self-start"}`}
                >
                  <p className="mb-1 text-xs font-semibold opacity-80">
                    {roleLabel[message.role] ?? message.role} · {formatTime(message.time)}
                  </p>
                  <div className="space-y-2">
                    {renderBasicMarkdown(message.content, message.id)}
                  </div>
                </article>
              ))}
              {isStreaming && (
                <article className={`max-w-[92%] self-start rounded-lg px-3 py-2 text-sm ${roleStyle.assistant}`}>
                  <p className="mb-1 text-xs font-semibold opacity-80">助手 · 输入中...</p>
                  <div className="space-y-2">
                    {renderBasicMarkdown(
                      streamingText.length > 0 ? streamingText : "...",
                      "streaming-temp",
                    )}
                  </div>
                </article>
              )}
              <div ref={messagesEndRef} />
            </div>
          ) : (
            <p className="text-sm text-slate-500">当前会话暂无消息。发送消息开始对话。</p>
          )}
        </div>

        <div className="mt-4">
          <textarea
            id="a2a-chat-message"
            rows={3}
            className="min-h-[5rem] w-full resize-y rounded-lg border border-slate-300 px-3 py-2 text-sm"
            placeholder="请输入要发送给 A2A agent 的内容..."
            value={draft}
            onChange={(event) => { setDraft(event.target.value); }}
            onKeyDown={(event) => {
              if (event.key === "Enter" && (event.metaKey || event.ctrlKey) && !loading && draft.trim()) {
                void handleSend();
              }
            }}
          />
          <div className="mt-2 flex items-center justify-between">
            <span className="text-xs text-slate-400">Ctrl+Enter 发送</span>
            <button
              type="button"
              className="w-36 rounded-md bg-slate-900 px-4 py-2 text-center text-sm font-semibold text-white disabled:cursor-not-allowed disabled:bg-slate-400"
              disabled={!canSubmit}
              onClick={() => {
                if (loading) { void handleCancel(); return; }
                void handleSend();
              }}
            >
              {submitLabel}
            </button>
          </div>
        </div>
      </div>

      {/* Right sidebar: Task info + Run events */}
      <aside className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
        <h3 className="text-base font-semibold text-slate-900">任务详情</h3>

        <div className="mt-3 space-y-2 text-xs text-slate-600">
          <p className="break-all">
            <span className="font-medium text-slate-800">Session:</span>{" "}
            {sessionId ?? "未创建"}
          </p>
          <p className="break-all">
            <span className="font-medium text-slate-800">Task ID:</span>{" "}
            {taskId ?? "未创建"}
          </p>
          <p>
            <span className="font-medium text-slate-800">状态:</span>{" "}
            <span className={TASK_STATE_COLORS[taskState] ?? "text-slate-400"}>
              {TASK_STATE_LABELS[taskState] ?? taskState}
            </span>
          </p>
        </div>

        <div className="mt-4">
          <h4 className="text-sm font-semibold text-slate-800">运行事件</h4>
          <div className="mt-2 max-h-[24rem] overflow-y-auto rounded-md border border-slate-200 bg-slate-50">
            {visibleRunEvents.length > 0 ? (
              <ul className="divide-y divide-slate-200">
                {visibleRunEvents.map((event) => (
                  <li key={event.id} className="px-3 py-2 text-xs text-slate-700">
                    <p className="font-medium text-slate-800">
                      [{event.type}] {formatTime(event.time)}
                    </p>
                    <p className="mt-1 whitespace-pre-wrap break-words">{event.detail}</p>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="px-3 py-2 text-xs text-slate-500">暂无运行事件</p>
            )}
          </div>
        </div>

        {error && (
          <p className="mt-3 rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">
            {error}
          </p>
        )}
      </aside>
    </section>
  );
};

export default A2AChatView;
