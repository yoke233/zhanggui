import { startTransition, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  AlertTriangle,
  Bot,
  Brain,
  Check,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  ClipboardCopy,
  Gauge,
  ListTodo,
  Loader2,
  MoreHorizontal,
  Paperclip,
  Plus,
  Search,
  Send,
  User,
  Workflow,
  X,
  Wrench,
} from "lucide-react";
import type {
  AgentDriver,
  AgentProfile,
  ChatMessage,
  ChatSessionDetail,
  ChatSessionSummary,
  Event as ApiEvent,
} from "@/types/apiV2";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { cn } from "@/lib/utils";
import { getErrorMessage } from "@/lib/v2Workbench";

type SessionRecord = ChatSessionSummary;

interface ChatMessageView {
  id: string;
  role: "user" | "assistant";
  content: string;
  time: string;
  at: string;
}

interface RealtimeChatOutputPayload {
  session_id?: string;
  type?: string;
  content?: string;
  tool_call_id?: string;
  stderr?: string;
  exit_code?: number;
  usage_size?: number;
  usage_used?: number;
}

interface RealtimeChatAckPayload {
  request_id?: string;
  session_id?: string;
  ws_path?: string;
  status?: string;
}

interface RealtimeChatErrorPayload {
  request_id?: string;
  session_id?: string;
  error?: string;
  code?: string;
}

interface ChatActivityView {
  id: string;
  type: "agent_thought" | "tool_call" | "usage_update";
  title: string;
  detail?: string;
  time: string;
  at: string;
  status?: "running" | "completed" | "failed";
  toolCallId?: string;
  usageSize?: number;
  usageUsed?: number;
}

interface ChatEventListItem {
  id: string;
  at: string;
  time: string;
  label: string;
  rawType: string;
  summary?: string;
  detail?: string;
  raw?: string;
  tone: "default" | "success" | "warning" | "danger";
}

interface SessionGroup {
  key: string;
  label: string;
  updatedAt: string;
  sessions: SessionRecord[];
}

interface LeadDriverOption {
  key: string;
  label: string;
  driverId: string;
}

const UNKNOWN_PROJECT_GROUP = "project:unknown";
const EMPTY_PROFILE_VALUE = "__empty_profile__";

const formatMessageTime = (value: string): string => {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleTimeString("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
  });
};

const formatActivityTime = (value: string): string => {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleTimeString("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
};

const toMessageView = (sessionId: string, message: ChatMessage, index: number): ChatMessageView => ({
  id: `${sessionId}-${message.role}-${index}-${message.time}`,
  role: message.role === "assistant" ? "assistant" : "user",
  content: message.content,
  time: formatMessageTime(message.time),
  at: message.time,
});

const toSummaryRecord = (session: ChatSessionSummary): SessionRecord => ({
  ...session,
  title: session.title?.trim() || "新会话",
});

const toDetailRecord = (session: ChatSessionDetail): SessionRecord => ({
  ...session,
  title: session.title?.trim() || "新会话",
});

const fallbackLabel = (value: string | null | undefined, fallback: string): string => {
  const trimmed = value?.trim();
  return trimmed ? trimmed : fallback;
};

const toProjectGroupKey = (projectId?: number | null): string => (
  projectId == null ? UNKNOWN_PROJECT_GROUP : `project:${projectId}`
);

const normalizeDriverKey = (driverId?: string): string => {
  const normalized = driverId?.trim().toLowerCase() ?? "";
  if (!normalized) {
    return "";
  }
  if (normalized.includes("codex")) {
    return "codex";
  }
  if (normalized.includes("claude")) {
    return "claude";
  }
  return normalized;
};

const driverLabelForId = (driverId?: string): string => {
  switch (normalizeDriverKey(driverId)) {
    case "codex":
      return "Codex";
    case "claude":
      return "Claude";
    default:
      return fallbackLabel(driverId, "未指定 Driver");
  }
};

const badgeVariantForStatus = (status?: string): "success" | "warning" | "secondary" => {
  switch (status) {
    case "running":
      return "success";
    case "alive":
      return "warning";
    default:
      return "secondary";
  }
};

const badgeLabelForStatus = (status?: string): string => {
  switch (status) {
    case "running":
      return "活跃";
    case "alive":
      return "空闲";
    default:
      return "已关闭";
  }
};

const toStringValue = (value: unknown): string => (
  typeof value === "string" ? value : ""
);

const toNumberValue = (value: unknown): number | undefined => {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === "string") {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) {
      return parsed;
    }
  }
  return undefined;
};

const isRecord = (value: unknown): value is Record<string, unknown> => (
  typeof value === "object" && value !== null && !Array.isArray(value)
);

const compactText = (value: string, maxLength = 160): string => {
  const normalized = value.replace(/\s+/g, " ").trim();
  if (!normalized) {
    return "";
  }
  if (normalized.length <= maxLength) {
    return normalized;
  }
  return `${normalized.slice(0, maxLength)}...`;
};

const stringifyJSON = (value: unknown): string | undefined => {
  if (value == null) {
    return undefined;
  }
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return undefined;
  }
};

const extractTextPreview = (value: unknown): string => {
  if (typeof value === "string") {
    return value.trim();
  }
  if (Array.isArray(value)) {
    return value
      .map((item) => extractTextPreview(item))
      .filter(Boolean)
      .join("\n")
      .trim();
  }
  if (!isRecord(value)) {
    return "";
  }
  return [
    toStringValue(value.text),
    toStringValue(value.content),
    toStringValue(value.newText),
    toStringValue(value.path),
    toStringValue(value.name),
    toStringValue(value.title),
  ]
    .filter((item) => item.trim())
    .join("\n")
    .trim();
};

const formatUsageValue = (value?: number): string => {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return "--";
  }
  return value.toLocaleString("zh-CN");
};

const formatUsagePercent = (used?: number, size?: number): number | null => {
  if (
    typeof used !== "number" ||
    typeof size !== "number" ||
    !Number.isFinite(used) ||
    !Number.isFinite(size) ||
    size <= 0
  ) {
    return null;
  }
  return Math.max(0, Math.min(100, (used / size) * 100));
};

const eventToneForType = (rawType: string): ChatEventListItem["tone"] => {
  switch (rawType) {
    case "error":
      return "danger";
    case "tool_call":
    case "tool_call_update":
      return "warning";
    case "tool_call_completed":
    case "done":
      return "success";
    default:
      return "default";
  }
};

const labelForEventType = (rawType: string): string => {
  switch (rawType) {
    case "agent_message_chunk":
      return "增量输出";
    case "agent_message":
      return "回复完成";
    case "agent_thought":
      return "思考";
    case "tool_call":
      return "工具调用";
    case "tool_call_update":
      return "工具更新";
    case "tool_call_completed":
      return "工具完成";
    case "usage_update":
      return "上下文用量";
    case "available_commands_update":
      return "命令列表更新";
    case "config_option_update":
    case "config_options_update":
      return "会话配置更新";
    case "done":
      return "会话完成";
    case "error":
      return "会话错误";
    default:
      return rawType || "事件";
  }
};

const buildEventSummary = (event: ApiEvent): { rawType: string; summary?: string; detail?: string } => {
  const data = isRecord(event.data) ? event.data : {};
  const payload = toRealtimePayload(event);
  const acp = isRecord(data.acp) ? data.acp : {};
  const rawType = toStringValue(acp.sessionUpdate) || payload.type?.trim() || event.type;
  const status = toStringValue(data.status) || toStringValue(acp.status);
  const title = toStringValue(acp.title);
  const text = toStringValue(data.text);
  const contentPreview = extractTextPreview(acp.content);
  const content = payload.content?.trim() || text.trim() || contentPreview || title.trim();
  const details: string[] = [];

  if (status) {
    details.push(`status: ${status}`);
  }
  if (payload.tool_call_id?.trim()) {
    details.push(`tool_call_id: ${payload.tool_call_id.trim()}`);
  }
  if (rawType === "usage_update") {
    const usageSize = toNumberValue(data.usage_size) ?? payload.usage_size;
    const usageUsed = toNumberValue(data.usage_used) ?? payload.usage_used;
    return {
      rawType,
      summary: `已用 ${formatUsageValue(usageUsed)} / ${formatUsageValue(usageSize)}`,
      detail: details.length > 0 ? details.join("\n") : undefined,
    };
  }
  if (rawType === "available_commands_update") {
    const commands = Array.isArray(acp.availableCommands) ? acp.availableCommands : [];
    const names = commands
      .map((item) => (isRecord(item) ? toStringValue(item.name).trim() : ""))
      .filter(Boolean)
      .slice(0, 6);
    return {
      rawType,
      summary: names.length > 0 ? `${commands.length} 个命令: ${names.join(", ")}` : `${commands.length} 个命令`,
      detail: details.length > 0 ? details.join("\n") : undefined,
    };
  }
  if (rawType === "config_option_update" || rawType === "config_options_update") {
    const options = Array.isArray(acp.configOptions) ? acp.configOptions : [];
    const names = options
      .map((item) => {
        if (!isRecord(item)) {
          return "";
        }
        const id = toStringValue(item.id).trim();
        const currentValue = toStringValue(item.currentValue).trim();
        return [id, currentValue].filter(Boolean).join("=");
      })
      .filter(Boolean)
      .slice(0, 6);
    return {
      rawType,
      summary: names.length > 0 ? `${options.length} 项: ${names.join(", ")}` : `${options.length} 项配置`,
      detail: details.length > 0 ? details.join("\n") : undefined,
    };
  }

  const summary = compactText(content || title || status || rawType, rawType === "agent_message_chunk" ? 120 : 180);
  if (title.trim() && title.trim() !== summary) {
    details.unshift(`title: ${title.trim()}`);
  }
  const keepFullContentInDetail = (
    rawType === "tool_call"
    || rawType === "tool_call_update"
    || rawType === "tool_call_completed"
  );
  if (keepFullContentInDetail && content.trim() && content.trim() !== summary) {
    details.push(content.trim());
  }
  return {
    rawType,
    summary: summary || undefined,
    detail: details.length > 0 ? details.join("\n\n") : undefined,
  };
};

const toEventListItem = (event: ApiEvent): ChatEventListItem => {
  const { rawType, summary, detail } = buildEventSummary(event);
  const data = isRecord(event.data) ? event.data : {};
  const shouldShowRaw = (
    isRecord(data.acp)
    || !!toStringValue(data.tool_call_id).trim()
    || typeof data.exit_code === "number"
    || !!toStringValue(data.stderr).trim()
    || rawType === "tool_call"
    || rawType === "tool_call_update"
    || rawType === "tool_call_completed"
    || rawType === "available_commands_update"
    || rawType === "config_option_update"
    || rawType === "config_options_update"
  );
  return {
    id: `event-${event.id}`,
    at: event.timestamp,
    time: formatActivityTime(event.timestamp),
    label: labelForEventType(rawType),
    rawType,
    summary,
    detail,
    raw: shouldShowRaw ? stringifyJSON(event.data) : undefined,
    tone: eventToneForType(rawType),
  };
};

const buildToolResultDetail = (payload: RealtimeChatOutputPayload): string => {
  const parts: string[] = [];
  if (typeof payload.exit_code === "number") {
    parts.push(`退出码：${payload.exit_code}`);
  }
  if (payload.content?.trim()) {
    parts.push(payload.content.trim());
  }
  if (payload.stderr?.trim()) {
    parts.push(`stderr\n${payload.stderr.trim()}`);
  }
  return parts.join("\n\n");
};

const touchSessionList = (
  sessions: SessionRecord[],
  sessionId: string,
  status: "running" | "alive" | "closed",
  at: string,
): SessionRecord[] => (
  sessions.map((session) =>
    session.session_id === sessionId
      ? {
          ...session,
          status,
          updated_at: at,
        }
      : session,
  )
);

const applyActivityPayload = (
  current: ChatActivityView[],
  sessionId: string,
  payload: RealtimeChatOutputPayload,
  at: string,
): ChatActivityView[] => {
  const updateType = payload.type?.trim();
  if (!updateType) {
    return current;
  }

  const next = [...current];
  const time = formatActivityTime(at);

  if (updateType === "agent_thought") {
    const detail = payload.content?.trim();
    if (!detail) {
      return current;
    }
    const previous = next.at(-1);
    if (previous?.type === "agent_thought") {
      next[next.length - 1] = {
        ...previous,
        detail: previous.detail ? `${previous.detail}\n${detail}` : detail,
        time,
        at,
      };
      return next;
    }
    next.push({
      id: `${sessionId}-thought-${Date.parse(at)}-${next.length}`,
      type: "agent_thought",
      title: "思考中",
      detail,
      time,
      at,
    });
    return next;
  }

  if (updateType === "tool_call") {
    const toolCallId = payload.tool_call_id?.trim();
    const existingIndex = toolCallId
      ? next.findIndex((activity) => activity.toolCallId === toolCallId)
      : -1;
    const previous = existingIndex >= 0 ? next[existingIndex] : null;
    const activity: ChatActivityView = {
      id: previous?.id ?? `${sessionId}-tool-${toolCallId ?? `${Date.parse(at)}-${next.length}`}`,
      type: "tool_call",
      title: payload.content?.trim() || previous?.title || "工具调用",
      detail: previous?.detail || "执行中...",
      time,
      at,
      status: "running",
      toolCallId,
    };
    if (existingIndex >= 0) {
      next[existingIndex] = activity;
      return next;
    }
    next.push(activity);
    return next;
  }

  if (updateType === "tool_call_completed") {
    const toolCallId = payload.tool_call_id?.trim();
    const existingIndex = toolCallId
      ? next.findIndex((activity) => activity.toolCallId === toolCallId)
      : -1;
    const previous = existingIndex >= 0 ? next[existingIndex] : null;
    const status = payload.exit_code && payload.exit_code !== 0 ? "failed" : "completed";
    const detail = buildToolResultDetail(payload) || previous?.detail || "已完成";
    const activity: ChatActivityView = {
      id: previous?.id ?? `${sessionId}-tool-${toolCallId ?? `${Date.parse(at)}-${next.length}`}`,
      type: "tool_call",
      title: previous?.title || payload.content?.trim() || "工具调用",
      detail,
      time,
      at,
      status,
      toolCallId,
    };
    if (existingIndex >= 0) {
      next[existingIndex] = activity;
      return next;
    }
    next.push(activity);
    return next;
  }

  if (updateType === "usage_update") {
    const usageSize = payload.usage_size;
    const usageUsed = payload.usage_used;
    if (typeof usageSize !== "number" || typeof usageUsed !== "number") {
      return current;
    }
    const activity: ChatActivityView = {
      id: `${sessionId}-usage`,
      type: "usage_update",
      title: "上下文用量",
      detail: `已用 ${formatUsageValue(usageUsed)} / ${formatUsageValue(usageSize)}`,
      time,
      at,
      usageSize,
      usageUsed,
    };
    const existingIndex = next.findIndex((item) => item.id === activity.id);
    if (existingIndex >= 0) {
      next[existingIndex] = activity;
      return next;
    }
    next.push(activity);
    return next;
  }

  return current;
};

const toRealtimePayload = (event: ApiEvent): RealtimeChatOutputPayload => ({
  session_id: toStringValue(event.data?.session_id),
  type: toStringValue(event.data?.type),
  content: toStringValue(event.data?.content),
  tool_call_id: toStringValue(event.data?.tool_call_id),
  stderr: toStringValue(event.data?.stderr),
  exit_code: toNumberValue(event.data?.exit_code),
  usage_size: toNumberValue(event.data?.usage_size),
  usage_used: toNumberValue(event.data?.usage_used),
});

const buildRealtimeEvent = (id: number, at: string, payload: RealtimeChatOutputPayload): ApiEvent => ({
  id,
  type: "chat.output",
  timestamp: at,
  data: {
    session_id: payload.session_id,
    type: payload.type,
    content: payload.content,
    tool_call_id: payload.tool_call_id,
    stderr: payload.stderr,
    exit_code: payload.exit_code,
    usage_size: payload.usage_size,
    usage_used: payload.usage_used,
  },
});

const buildActivityHistory = (
  sessionId: string,
  events: ApiEvent[],
): ChatActivityView[] => {
  const sorted = [...events].sort((left, right) => (
    new Date(left.timestamp).getTime() - new Date(right.timestamp).getTime()
  ));
  return sorted.reduce<ChatActivityView[]>((activities, event) => {
    if (event.type !== "chat.output") {
      return activities;
    }
    const payload = toRealtimePayload(event);
    if (payload.session_id?.trim() !== sessionId) {
      return activities;
    }
    return applyActivityPayload(activities, sessionId, payload, event.timestamp);
  }, []);
};

function EventLogRow({ item }: { item: ChatEventListItem }) {
  const hasExpandedContent = Boolean(item.detail || item.raw);
  const shouldCollapseByDefault = hasExpandedContent && (
    item.rawType.includes("tool")
    || item.rawType === "available_commands_update"
    || item.rawType === "config_option_update"
    || item.rawType === "config_options_update"
    || (item.raw?.length ?? 0) > 320
  );
  const [expanded, setExpanded] = useState(!shouldCollapseByDefault);

  const icon = (() => {
    switch (item.tone) {
      case "danger":
        return <AlertTriangle className="h-4 w-4 text-rose-600" />;
      case "success":
        return <CheckCircle2 className="h-4 w-4 text-emerald-600" />;
      case "warning":
        return <Wrench className="h-4 w-4 text-amber-600" />;
      default:
        if (item.rawType === "agent_thought") {
          return <Brain className="h-4 w-4 text-sky-600" />;
        }
        if (item.rawType === "usage_update") {
          return <Gauge className="h-4 w-4 text-violet-600" />;
        }
        return <Bot className="h-4 w-4 text-slate-500" />;
    }
  })();

  return (
    <div className="rounded-xl border bg-background/90 px-4 py-3 shadow-sm">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 shrink-0">{icon}</div>
        <div className="min-w-0 flex-1 space-y-3">
          <div className="flex items-start gap-3">
            <div className="w-24 shrink-0 font-mono text-[11px] text-muted-foreground">{item.time}</div>
            <div className="min-w-0 flex-1 space-y-1">
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant="outline" className="text-[10px]">
                  {item.label}
                </Badge>
                <span className="font-mono text-[10px] text-muted-foreground">{item.rawType}</span>
              </div>
              {item.summary ? (
                <p className="whitespace-pre-wrap break-words font-mono text-xs leading-6 text-foreground">
                  {item.summary}
                </p>
              ) : (
                <p className="text-xs text-muted-foreground">无摘要</p>
              )}
            </div>
            {hasExpandedContent ? (
              <button
                type="button"
                className="shrink-0 rounded-md px-2 py-1 text-[11px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                onClick={() => setExpanded((current) => !current)}
              >
                {expanded ? "收起" : "展开"}
              </button>
            ) : null}
          </div>

          {expanded && item.detail ? (
            <pre className="overflow-x-auto whitespace-pre-wrap break-words rounded-lg border bg-muted/40 px-3 py-2 font-mono text-[11px] leading-6 text-foreground">
              {item.detail}
            </pre>
          ) : null}

          {expanded && item.raw ? (
            <pre className="max-h-[360px] overflow-auto rounded-lg border bg-slate-950 px-3 py-3 font-mono text-[11px] leading-6 text-slate-100">
              {item.raw}
            </pre>
          ) : null}
        </div>
      </div>
    </div>
  );
}

export function ChatPage() {
  const navigate = useNavigate();
  const {
    apiClient,
    wsClient,
    projects,
    selectedProjectId,
    setSelectedProjectId,
  } = useWorkbench();
  const [copiedMessageId, setCopiedMessageId] = useState<string | null>(null);
  const [sessions, setSessions] = useState<SessionRecord[]>([]);
  const [activeSession, setActiveSession] = useState<string | null>(null);
  const [messagesBySession, setMessagesBySession] = useState<Record<string, ChatMessageView[]>>({});
  const [eventsBySession, setEventsBySession] = useState<Record<string, ApiEvent[]>>({});
  const [activitiesBySession, setActivitiesBySession] = useState<Record<string, ChatActivityView[]>>({});
  const [draftMessages, setDraftMessages] = useState<ChatMessageView[]>([]);
  const [loadedSessions, setLoadedSessions] = useState<Record<string, boolean>>({});
  const [sessionSearch, setSessionSearch] = useState("");
  const [messageInput, setMessageInput] = useState("");
  const [pendingFiles, setPendingFiles] = useState<File[]>([]);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [drivers, setDrivers] = useState<AgentDriver[]>([]);
  const [leadProfiles, setLeadProfiles] = useState<AgentProfile[]>([]);
  const [draftProjectId, setDraftProjectId] = useState<number | null>(selectedProjectId);
  const [draftProfileId, setDraftProfileId] = useState("");
  const [draftDriverId, setDraftDriverId] = useState("");
  const [collapsedGroups, setCollapsedGroups] = useState<Record<string, boolean>>({});
  const [detailView, setDetailView] = useState<"chat" | "events">("chat");
  const [submitting, setSubmitting] = useState(false);
  const [loadingSessions, setLoadingSessions] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const pendingChunkBuffersRef = useRef<Record<string, string>>({});
  const chunkFlushFrameRef = useRef<number | null>(null);
  const pendingRequestIdRef = useRef<string | null>(null);
  const syntheticEventIdRef = useRef(-1);

  const syncSessionDetail = (detail: ChatSessionDetail) => {
    const record = toDetailRecord(detail);
    const views = detail.messages.map((message, index) =>
      toMessageView(detail.session_id, message, index),
    );

    setSessions((current) => {
      const existing = current.filter((item) => item.session_id !== detail.session_id);
      return [record, ...existing].sort((left, right) => (
        new Date(right.updated_at).getTime() - new Date(left.updated_at).getTime()
      ));
    });
    setMessagesBySession((current) => ({
      ...current,
      [detail.session_id]: views,
    }));
    setLoadedSessions((current) => ({
      ...current,
      [detail.session_id]: true,
    }));
  };

  const syncSessionEvents = (sessionId: string, events: ApiEvent[]) => {
    setEventsBySession((current) => {
      const latestLoadedAt = events.reduce((maxTime, event) => (
        Math.max(maxTime, new Date(event.timestamp).getTime())
      ), 0);
      const pendingRealtime = (current[sessionId] ?? []).filter((event) => (
        event.id < 0 && new Date(event.timestamp).getTime() > latestLoadedAt
      ));
      const merged = [...events, ...pendingRealtime].sort((left, right) => (
        new Date(left.timestamp).getTime() - new Date(right.timestamp).getTime()
      ));
      setActivitiesBySession((activityCurrent) => ({
        ...activityCurrent,
        [sessionId]: buildActivityHistory(sessionId, merged),
      }));
      return {
        ...current,
        [sessionId]: merged,
      };
    });
  };

  const loadSessionState = async (sessionId: string) => {
    const [detail, events] = await Promise.all([
      apiClient.getChatSession(sessionId),
      apiClient.listEvents({
        session_id: sessionId,
        types: ["chat.output"],
        limit: 200,
        offset: 0,
      }),
    ]);
    syncSessionDetail(detail);
    syncSessionEvents(sessionId, events);
  };

  const refreshSessions = async (preferredSessionId?: string | null) => {
    setLoadingSessions(true);
    try {
      const list = await apiClient.listChatSessions();
      const next = list.map(toSummaryRecord);
      setSessions(next.sort((left, right) => (
        new Date(right.updated_at).getTime() - new Date(left.updated_at).getTime()
      )));
      setActiveSession((current) => {
        if (preferredSessionId) {
          return preferredSessionId;
        }
        if (current && next.some((item) => item.session_id === current)) {
          return current;
        }
        return next[0]?.session_id ?? null;
      });
    } catch (loadError) {
      setError(getErrorMessage(loadError));
    } finally {
      setLoadingSessions(false);
    }
  };

  useEffect(() => {
    void refreshSessions();
  }, []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [profiles, driverList] = await Promise.all([
          apiClient.listProfiles(),
          apiClient.listDrivers(),
        ]);
        if (cancelled) {
          return;
        }
        const leads = profiles.filter((profile) => profile.role === "lead");
        setDrivers(driverList);
        setLeadProfiles(leads);
        setDraftProfileId((current) => {
          if (current && leads.some((profile) => profile.id === current)) {
            return current;
          }
          return leads[0]?.id ?? "";
        });
        setDraftDriverId((current) => {
          if (current && driverList.some((driver) => driver.id === current)) {
            return current;
          }
          return driverList[0]?.id ?? "";
        });
      } catch (loadError) {
        if (!cancelled) {
          setError(getErrorMessage(loadError));
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [apiClient]);

  const flushBufferedChunks = () => {
    if (chunkFlushFrameRef.current != null) {
      cancelAnimationFrame(chunkFlushFrameRef.current);
      chunkFlushFrameRef.current = null;
    }

    const pending = pendingChunkBuffersRef.current;
    const sessionIds = Object.keys(pending);
    if (sessionIds.length === 0) {
      return;
    }
    pendingChunkBuffersRef.current = {};

    const now = new Date();
    const nowISO = now.toISOString();
    const nowTime = now.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" });

    startTransition(() => {
      setMessagesBySession((current) => {
        const next = { ...current };
        for (const sessionId of sessionIds) {
          const chunk = pending[sessionId];
          if (!chunk) {
            continue;
          }
          const existing = next[sessionId] ?? [];
          const last = existing.at(-1);
          if (last && last.id === `${sessionId}-stream-assistant`) {
            next[sessionId] = [
              ...existing.slice(0, -1),
              {
                ...last,
                content: `${last.content}${chunk}`,
                time: nowTime,
                at: nowISO,
              },
            ];
            continue;
          }
          next[sessionId] = [
            ...existing,
            {
              id: `${sessionId}-stream-assistant`,
              role: "assistant",
              content: chunk,
              time: nowTime,
              at: nowISO,
            },
          ];
        }
        return next;
      });

      setSessions((current) =>
        current.map((session) =>
          pending[session.session_id]
            ? {
                ...session,
                status: "running",
                updated_at: nowISO,
              }
            : session,
        ),
      );
    });
  };

  const scheduleChunkFlush = () => {
    if (chunkFlushFrameRef.current != null) {
      return;
    }
    chunkFlushFrameRef.current = requestAnimationFrame(() => {
      chunkFlushFrameRef.current = null;
      flushBufferedChunks();
    });
  };

  const currentSession = useMemo(
    () => sessions.find((session) => session.session_id === activeSession) ?? null,
    [activeSession, sessions],
  );

  useEffect(() => {
    if (!currentSession) {
      setDraftProjectId(selectedProjectId);
    }
  }, [currentSession, selectedProjectId]);

  const projectNameMap = useMemo(
    () => new Map(projects.map((project) => [project.id, project.name])),
    [projects],
  );
  const leadDriverOptions = useMemo<LeadDriverOption[]>(() => {
    const grouped = new Map<string, LeadDriverOption>();
    for (const driver of drivers) {
      const key = normalizeDriverKey(driver.id);
      if (!key) {
        continue;
      }
      if (!grouped.has(key)) {
        grouped.set(key, {
          key,
          label: driverLabelForId(driver.id),
          driverId: driver.id,
        });
      }
    }
    const rank = (key: string): number => {
      if (key === "codex") {
        return 0;
      }
      if (key === "claude") {
        return 1;
      }
      return 9;
    };
    return Array.from(grouped.values()).sort((left, right) => {
      const rankDiff = rank(left.key) - rank(right.key);
      if (rankDiff !== 0) {
        return rankDiff;
      }
      return left.label.localeCompare(right.label, "zh-CN");
    });
  }, [drivers]);
  const leadDriverMap = useMemo(
    () => new Map(leadDriverOptions.map((option) => [option.driverId, option])),
    [leadDriverOptions],
  );

  const currentProjectId = currentSession?.project_id ?? draftProjectId ?? null;
  const currentProjectLabel = fallbackLabel(
    currentSession?.project_name ?? (currentProjectId != null ? projectNameMap.get(currentProjectId) : undefined),
    "未指定项目",
  );
  const currentDriverId = currentSession?.driver_id ?? draftDriverId;
  const draftSessionReady = Boolean(draftProfileId && draftDriverId);
  const currentDriverLabel = currentDriverId
    ? leadDriverMap.get(currentDriverId)?.label ?? driverLabelForId(currentDriverId)
    : "未指定 Driver";

  const filteredSessions = useMemo(
    () =>
      sessions.filter((session) => {
        const query = sessionSearch.trim().toLowerCase();
        if (!query) {
          return true;
        }
        return [
          session.title,
          session.project_name,
          session.profile_name,
          session.driver_id ? driverLabelForId(session.driver_id).toLowerCase() : "",
        ].some((value) => (value ?? "").toLowerCase().includes(query));
      }),
    [sessionSearch, sessions],
  );

  const groupedSessions = useMemo<SessionGroup[]>(() => {
    const groups = new Map<string, SessionGroup>();
    for (const session of filteredSessions) {
      const key = toProjectGroupKey(session.project_id);
      const existing = groups.get(key);
      if (existing) {
        existing.sessions.push(session);
        if (new Date(session.updated_at).getTime() > new Date(existing.updatedAt).getTime()) {
          existing.updatedAt = session.updated_at;
        }
        continue;
      }
      groups.set(key, {
        key,
        label: fallbackLabel(session.project_name, "未指定项目"),
        updatedAt: session.updated_at,
        sessions: [session],
      });
    }
    return Array.from(groups.values())
      .map((group) => ({
        ...group,
        sessions: [...group.sessions].sort((left, right) => (
          new Date(right.updated_at).getTime() - new Date(left.updated_at).getTime()
        )),
      }))
      .sort((left, right) => {
        const timeDiff = new Date(right.updatedAt).getTime() - new Date(left.updatedAt).getTime();
        if (timeDiff !== 0) {
          return timeDiff;
        }
        return left.label.localeCompare(right.label, "zh-CN");
      });
  }, [filteredSessions]);

  const currentMessages = currentSession ? (messagesBySession[currentSession.session_id] ?? []) : draftMessages;
  const currentEvents = currentSession ? (eventsBySession[currentSession.session_id] ?? []) : [];
  const currentActivities = currentSession ? (activitiesBySession[currentSession.session_id] ?? []) : [];
  const isDraftSessionView = !currentSession && currentMessages.length === 0;
  const currentEventItems = useMemo(
    () =>
      [...currentEvents]
        .sort((left, right) => {
          const timeDiff = new Date(left.timestamp).getTime() - new Date(right.timestamp).getTime();
          if (timeDiff !== 0) {
            return timeDiff;
          }
          return left.id - right.id;
        })
        .map((event) => toEventListItem(event)),
    [currentEvents],
  );

  const currentUsage = useMemo(
    () =>
      [...currentActivities]
        .reverse()
        .find((activity) => activity.type === "usage_update"),
    [currentActivities],
  );

  const currentUsagePercent = formatUsagePercent(currentUsage?.usageUsed, currentUsage?.usageSize);

  useEffect(() => {
    const isStreaming = currentMessages.at(-1)?.id.endsWith("stream-assistant");
    messagesEndRef.current?.scrollIntoView({ behavior: isStreaming ? "auto" : "smooth" });
  }, [currentEventItems, currentMessages, detailView]);

  useEffect(() => {
    const unsubscribeOutput = wsClient.subscribe<RealtimeChatOutputPayload>(
      "chat.output",
      (payload) => {
        const sessionId = payload.session_id?.trim();
        if (!sessionId) {
          return;
        }

        const updateType = payload.type?.trim();
        const now = new Date();
        const nowISO = now.toISOString();
        if (updateType) {
          const event = buildRealtimeEvent(syntheticEventIdRef.current, nowISO, payload);
          syntheticEventIdRef.current -= 1;
          setEventsBySession((current) => ({
            ...current,
            [sessionId]: [...(current[sessionId] ?? []), event],
          }));
        }
        if (updateType === "agent_message_chunk" && payload.content) {
          pendingChunkBuffersRef.current[sessionId] = `${pendingChunkBuffersRef.current[sessionId] ?? ""}${payload.content}`;
          scheduleChunkFlush();
          return;
        }

        if (updateType === "agent_message" && payload.content) {
          flushBufferedChunks();
          setMessagesBySession((current) => {
            const existing = current[sessionId] ?? [];
            const last = existing.at(-1);
            if (last && last.id === `${sessionId}-stream-assistant`) {
              return {
                ...current,
                [sessionId]: [
                  ...existing.slice(0, -1),
                  {
                    ...last,
                    content: payload.content ?? last.content,
                    time: now.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" }),
                    at: nowISO,
                  },
                ],
              };
            }
            return current;
          });
          setSessions((current) => touchSessionList(current, sessionId, "running", nowISO));
          return;
        }

        if (updateType === "done") {
          flushBufferedChunks();
          setSessions((current) => touchSessionList(current, sessionId, "alive", nowISO));
          setSubmitting(false);
          pendingRequestIdRef.current = null;
          return;
        }

        if (updateType === "error") {
          flushBufferedChunks();
          setError(payload.content?.trim() || "会话执行失败");
          setSessions((current) => touchSessionList(current, sessionId, "closed", nowISO));
          setSubmitting(false);
          pendingRequestIdRef.current = null;
          return;
        }

        startTransition(() => {
          setActivitiesBySession((current) => ({
            ...current,
            [sessionId]: applyActivityPayload(
              current[sessionId] ?? [],
              sessionId,
              payload,
              nowISO,
            ),
          }));
          setSessions((current) => touchSessionList(current, sessionId, "running", nowISO));
        });
      },
    );
    const unsubscribeAck = wsClient.subscribe<RealtimeChatAckPayload>(
      "chat.ack",
      (payload) => {
        const requestId = payload.request_id?.trim();
        if (!pendingRequestIdRef.current && requestId) {
          return;
        }
        if (pendingRequestIdRef.current && requestId && pendingRequestIdRef.current !== requestId) {
          return;
        }
        const sessionId = payload.session_id?.trim();
        if (!sessionId) {
          return;
        }
        pendingRequestIdRef.current = null;
        setSubmitting(false);
        setActiveSession(sessionId);
        setDraftMessages([]);
        setLoadedSessions((current) => ({
          ...current,
          [sessionId]: false,
        }));
        void refreshSessions(sessionId);
      },
    );
    const unsubscribeError = wsClient.subscribe<RealtimeChatErrorPayload>(
      "chat.error",
      (payload) => {
        const requestId = payload.request_id?.trim();
        if (!pendingRequestIdRef.current && requestId) {
          return;
        }
        if (pendingRequestIdRef.current && requestId && pendingRequestIdRef.current !== requestId) {
          return;
        }
        pendingRequestIdRef.current = null;
        setSubmitting(false);
        setError(payload.error?.trim() || "发送消息失败");
        const sessionId = payload.session_id?.trim();
        if (sessionId) {
          setSessions((current) => touchSessionList(current, sessionId, "closed", new Date().toISOString()));
        }
      },
    );
    return () => {
      if (chunkFlushFrameRef.current != null) {
        cancelAnimationFrame(chunkFlushFrameRef.current);
        chunkFlushFrameRef.current = null;
      }
      pendingChunkBuffersRef.current = {};
      pendingRequestIdRef.current = null;
      unsubscribeOutput();
      unsubscribeAck();
      unsubscribeError();
    };
  }, [wsClient]);

  useEffect(() => {
    if (!activeSession || loadedSessions[activeSession]) {
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        if (!cancelled) {
          await loadSessionState(activeSession);
        }
      } catch (loadError) {
        if (!cancelled) {
          setError(getErrorMessage(loadError));
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [activeSession, apiClient, loadedSessions]);

  const createSession = () => {
    setDraftProjectId(selectedProjectId);
    setDraftProfileId((current) => {
      if (current && leadProfiles.some((profile) => profile.id === current)) {
        return current;
      }
      return leadProfiles[0]?.id ?? "";
    });
    setDraftDriverId((current) => {
      if (current && drivers.some((driver) => driver.id === current)) {
        return current;
      }
      return leadDriverOptions[0]?.driverId ?? drivers[0]?.id ?? "";
    });
    setActiveSession(null);
    setDraftMessages([]);
    setMessageInput("");
    setError(null);
  };

  const appendMessage = (sessionId: string | null, role: "user" | "assistant", content: string) => {
    const now = new Date();
    const nowISO = now.toISOString();
    const view: ChatMessageView = {
      id: `${sessionId ?? "draft"}-${role}-${now.getTime()}`,
      role,
      content,
      time: now.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" }),
      at: nowISO,
    };

    if (!sessionId) {
      setDraftMessages((current) => [...current, view]);
      return;
    }

    setMessagesBySession((current) => ({
      ...current,
      [sessionId]: [...(current[sessionId] ?? []), view],
    }));
    setSessions((current) =>
      current.map((session) =>
        session.session_id === sessionId
          ? {
              ...session,
              title: session.title === "新会话" && role === "user"
                ? content.slice(0, 24)
                : session.title,
              updated_at: nowISO,
              message_count: session.message_count + 1,
              status: role === "user" ? "running" : "alive",
            }
          : session,
      ),
    );
  };

  const handlePaste = (e: React.ClipboardEvent) => {
    const items = e.clipboardData?.items;
    if (!items) return;
    const newFiles: File[] = [];
    for (const item of items) {
      if (item.kind === "file") {
        const file = item.getAsFile();
        if (file) newFiles.push(file);
      }
    }
    if (newFiles.length > 0) {
      setPendingFiles((prev) => [...prev, ...newFiles]);
    }
  };

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = e.target.files;
    if (files && files.length > 0) {
      setPendingFiles((prev) => [...prev, ...Array.from(files)]);
    }
    e.target.value = "";
  };

  const removePendingFile = (index: number) => {
    setPendingFiles((prev) => prev.filter((_, i) => i !== index));
  };

  const sendMessage = async () => {
    const content = messageInput.trim();
    if ((!content && pendingFiles.length === 0) || currentSession?.status === "running") {
      return;
    }

    const workingSessionId = activeSession;
    const resolvedProjectId = currentSession?.project_id ?? draftProjectId ?? undefined;
    const resolvedProjectName = currentSession?.project_name
      ?? (resolvedProjectId != null ? projectNameMap.get(resolvedProjectId) : undefined);
    const resolvedProfileId = currentSession?.profile_id ?? draftProfileId;
    const resolvedDriverId = currentSession?.driver_id ?? draftDriverId;

    if (!resolvedProfileId) {
      setError("请先选择 Driver 后再开始会话。");
      return;
    }
    if (!resolvedDriverId) {
      setError("请先选择 Driver 后再开始会话。");
      return;
    }

    // Convert pending files to base64 attachments.
    const attachments: { name: string; mime_type: string; data: string }[] = [];
    for (const file of pendingFiles) {
      const buf = await file.arrayBuffer();
      const b64 = btoa(String.fromCharCode(...new Uint8Array(buf)));
      attachments.push({ name: file.name, mime_type: file.type || "application/octet-stream", data: b64 });
    }

    const displayContent = content + (attachments.length > 0
      ? `\n[附件: ${attachments.map((a) => a.name).join(", ")}]`
      : "");
    appendMessage(workingSessionId, "user", displayContent);
    setMessageInput("");
    setPendingFiles([]);
    setSubmitting(true);
    setError(null);

    try {
      const requestId = `chat-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
      pendingRequestIdRef.current = requestId;
      wsClient.send({
        type: "chat.send",
        data: {
          request_id: requestId,
          session_id: workingSessionId ?? undefined,
          message: content || "(附件)",
          attachments: attachments.length > 0 ? attachments : undefined,
          project_id: resolvedProjectId,
          project_name: resolvedProjectName,
          profile_id: resolvedProfileId,
          driver_id: resolvedDriverId,
        },
      });
    } catch (sendError) {
      pendingRequestIdRef.current = null;
      setError(getErrorMessage(sendError));
      if (workingSessionId) {
        setSessions((current) => touchSessionList(current, workingSessionId, "closed", new Date().toISOString()));
      }
    } finally {
      if (!pendingRequestIdRef.current) {
        setSubmitting(false);
      }
    }
  };

  const closeSession = async () => {
    if (!currentSession) {
      return;
    }
    try {
      await apiClient.closeChat(currentSession.session_id);
      setSessions((current) =>
        current.map((session) =>
          session.session_id === currentSession.session_id
            ? { ...session, status: "closed" }
            : session,
        ),
      );
    } catch (closeError) {
      setError(getErrorMessage(closeError));
    }
  };

  const handleCopyMessage = useCallback(async (messageId: string, content: string) => {
    try {
      await navigator.clipboard.writeText(content);
      setCopiedMessageId(messageId);
      setTimeout(() => setCopiedMessageId((prev) => (prev === messageId ? null : prev)), 2000);
    } catch {
      // fallback
      const textarea = document.createElement("textarea");
      textarea.value = content;
      document.body.appendChild(textarea);
      textarea.select();
      document.execCommand("copy");
      document.body.removeChild(textarea);
      setCopiedMessageId(messageId);
      setTimeout(() => setCopiedMessageId((prev) => (prev === messageId ? null : prev)), 2000);
    }
  }, []);

  const handleCreateFlowFromMessage = useCallback(
    async (content: string) => {
      try {
        const title = content.length > 60 ? content.slice(0, 60) + "..." : content;
        const flow = await apiClient.createFlow({
          project_id: selectedProjectId ?? undefined,
          name: title,
          metadata: { source: "chat", original_content: content },
        });
        navigate(`/flows/${flow.id}`);
      } catch (err) {
        setError(getErrorMessage(err));
      }
    },
    [apiClient, selectedProjectId, navigate],
  );

  const [createdIssueMessageId, setCreatedIssueMessageId] = useState<string | null>(null);

  const handleCreateIssueFromMessage = useCallback(
    async (messageId: string, content: string) => {
      try {
        const firstLine = content.split("\n")[0] ?? content;
        const title = firstLine.length > 80 ? firstLine.slice(0, 80) + "..." : firstLine;
        await apiClient.createIssue({
          project_id: selectedProjectId ?? undefined,
          title,
          body: content,
          metadata: { source: "chat" },
        });
        setCreatedIssueMessageId(messageId);
        setTimeout(() => setCreatedIssueMessageId((prev) => (prev === messageId ? null : prev)), 2000);
      } catch (err) {
        setError(getErrorMessage(err));
      }
    },
    [apiClient, selectedProjectId],
  );

  return (
    <div className="flex h-full overflow-hidden">
      <div className="flex w-72 flex-col border-r bg-sidebar">
        <div className="border-b p-3">
          <div className="mb-3 flex items-center justify-between">
            <h2 className="text-sm font-semibold">会话列表</h2>
            <Button variant="outline" size="sm" className="h-8 gap-1.5 px-2.5 text-xs" onClick={createSession}>
              <Plus className="h-3.5 w-3.5" />
              新建
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
          {groupedSessions.map((group) => (
            <div key={group.key} className="border-b">
              <button
                type="button"
                className="flex w-full items-center gap-2 px-3 py-2 text-left transition-colors hover:bg-muted/50"
                onClick={() =>
                  setCollapsedGroups((current) => ({
                    ...current,
                    [group.key]: !current[group.key],
                  }))
                }
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
                const preview = messagesBySession[session.session_id]?.at(-1)?.content ?? "暂无消息";
                return (
                  <button
                    key={session.session_id}
                    onClick={() => setActiveSession(session.session_id)}
                    className={cn(
                      "w-full border-t px-3 py-3 pl-8 text-left transition-colors",
                      activeSession === session.session_id ? "bg-accent" : "hover:bg-muted/50",
                    )}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="truncate text-sm font-medium">{session.title ?? "新会话"}</span>
                      <span className="shrink-0 text-[10px] text-muted-foreground">
                        {new Date(session.updated_at).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })}
                      </span>
                    </div>
                    <div className="mt-1 flex items-center gap-1.5">
                      <div
                        className={cn(
                          "h-1.5 w-1.5 shrink-0 rounded-full",
                          session.status === "running"
                            ? "bg-emerald-500"
                            : session.status === "alive"
                              ? "bg-amber-500"
                              : "bg-zinc-300",
                        )}
                      />
                      <p className="truncate text-xs text-muted-foreground">{preview}</p>
                    </div>
                    <div className="mt-2 flex flex-wrap items-center gap-1.5">
                      {session.driver_id ? (
                        <Badge variant="outline" className="text-[10px]">
                          Lead · {driverLabelForId(session.driver_id)}
                        </Badge>
                      ) : null}
                    </div>
                  </button>
                );
              }) : null}
            </div>
          ))}
          {!loadingSessions && groupedSessions.length === 0 ? (
            <div className="px-3 py-4 text-xs text-muted-foreground">
              暂无会话。
            </div>
          ) : null}
        </div>
      </div>

      <div className="flex flex-1 flex-col">
        {!isDraftSessionView ? (
          <div className="border-b px-5 py-3">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary text-primary-foreground">
                <Bot className="h-4 w-4" />
              </div>
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <span className="truncate text-sm font-semibold">{currentSession?.title ?? "Lead Agent"}</span>
                  <Badge
                    variant={badgeVariantForStatus(currentSession?.status)}
                    className="text-[10px]"
                  >
                    {badgeLabelForStatus(currentSession?.status)}
                  </Badge>
                  {submitting ? <Loader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" /> : null}
                </div>
                <div className="mt-2 flex flex-wrap items-center gap-2">
                  <Badge variant="secondary" className="text-[10px]">
                    项目 · {currentProjectLabel}
                  </Badge>
                  <Badge variant="secondary" className="text-[10px]">
                    Lead · {currentDriverLabel}
                  </Badge>
                </div>
              </div>
            </div>
            <div className="flex items-center gap-2">
              {currentUsage ? (
                <div className="flex items-center gap-2 rounded-full border bg-background px-2.5 py-1 text-[11px] text-muted-foreground">
                  <span className="shrink-0 whitespace-nowrap">上下文</span>
                  <div className="h-1.5 w-20 overflow-hidden rounded-full bg-muted">
                    <div
                      className={cn(
                        "h-full rounded-full transition-[width] duration-300",
                        currentUsagePercent != null && currentUsagePercent >= 85
                          ? "bg-rose-500"
                          : currentUsagePercent != null && currentUsagePercent >= 60
                            ? "bg-amber-500"
                            : "bg-emerald-500",
                      )}
                      style={{ width: `${Math.max(currentUsagePercent ?? 0, currentUsagePercent == null ? 0 : 4)}%` }}
                    />
                  </div>
                  <span className="shrink-0 whitespace-nowrap">
                    {formatUsageValue(currentUsage.usageUsed)} / {formatUsageValue(currentUsage.usageSize)}
                    {currentUsagePercent != null ? ` · ${currentUsagePercent.toFixed(1)}%` : ""}
                  </span>
                </div>
              ) : null}
              <div className="flex items-center rounded-md border bg-background p-0.5">
                <button
                  type="button"
                  className={cn(
                    "rounded px-2.5 py-1 text-[11px] font-medium transition-colors",
                    detailView === "chat" ? "bg-foreground text-background" : "text-muted-foreground hover:text-foreground",
                  )}
                  onClick={() => setDetailView("chat")}
                >
                  对话
                </button>
                <button
                  type="button"
                  className={cn(
                    "rounded px-2.5 py-1 text-[11px] font-medium transition-colors",
                    detailView === "events" ? "bg-foreground text-background" : "text-muted-foreground hover:text-foreground",
                  )}
                  onClick={() => setDetailView("events")}
                >
                  事件
                </button>
              </div>
              <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => void closeSession()}>
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </div>
          </div>
          </div>
        ) : null}

        {error ? <p className="mx-5 mt-4 rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p> : null}

        <div className="flex-1 overflow-y-auto px-5 py-4 [scrollbar-gutter:stable]">
          {detailView === "events" ? (
            currentEventItems.length === 0 ? (
              <div className="mx-auto w-full max-w-[920px] rounded-2xl border border-dashed bg-muted/20 px-5 py-6 text-sm text-muted-foreground">
                这个会话还没有可显示的事件。
              </div>
            ) : (
              <div className="mx-auto w-full max-w-[920px] space-y-3">
                {currentEventItems.map((item) => <EventLogRow key={item.id} item={item} />)}
              </div>
            )
          ) : isDraftSessionView ? (
            <div className="flex min-h-full items-center justify-center">
              <div className="w-full max-w-[860px] rounded-[28px] border bg-gradient-to-br from-white via-slate-50 to-slate-100 p-7 shadow-sm">
                <div className="space-y-6">
                  <div className="space-y-2">
                    <p className="text-2xl font-semibold tracking-tight text-foreground">新会话</p>
                    <p className="text-sm text-muted-foreground">先选择项目和 Driver，然后直接在这里输入第一条消息。</p>
                  </div>
                  <div className="grid gap-4 md:grid-cols-2">
                  <div className="space-y-2">
                    <label className="text-xs font-medium uppercase tracking-[0.18em] text-slate-500">项目</label>
                    <Select
                      value={draftProjectId == null ? "" : String(draftProjectId)}
                      onChange={(event) => {
                        const next = event.target.value;
                        const nextProjectId = next ? Number(next) : null;
                        setDraftProjectId(nextProjectId);
                        setSelectedProjectId(nextProjectId);
                      }}
                    >
                      <option value="">未指定项目</option>
                      {projects.map((project) => (
                        <option key={project.id} value={project.id}>{project.name}</option>
                      ))}
                    </Select>
                  </div>
                  <div className="space-y-2">
                    <label className="text-xs font-medium uppercase tracking-[0.18em] text-slate-500">Driver</label>
                    <Select
                      value={draftDriverId || EMPTY_PROFILE_VALUE}
                      onChange={(event) => {
                        const next = event.target.value;
                        setDraftDriverId(next === EMPTY_PROFILE_VALUE ? "" : next);
                      }}
                    >
                      <option value={EMPTY_PROFILE_VALUE}>请选择 Driver</option>
                      {leadDriverOptions.map((option) => (
                        <option key={option.driverId} value={option.driverId}>
                          {option.label}
                        </option>
                      ))}
                    </Select>
                  </div>
                  </div>
                  <div className="space-y-3">
                    <Textarea
                      placeholder={`输入消息，使用 Lead · ${currentDriverLabel} 在 ${currentProjectLabel} 下开始会话...`}
                      className="min-h-[180px] resize-none bg-white/90"
                      value={messageInput}
                      disabled={submitting || !draftSessionReady}
                      onChange={(event) => setMessageInput(event.target.value)}
                      onPaste={handlePaste}
                      onKeyDown={(event) => {
                        if (event.key === "Enter" && !event.shiftKey) {
                          event.preventDefault();
                          void sendMessage();
                        }
                      }}
                    />
                    {pendingFiles.length > 0 && (
                      <div className="flex flex-wrap gap-2">
                        {pendingFiles.map((file, idx) => (
                          <Badge key={idx} variant="secondary" className="gap-1 text-xs">
                            {file.name}
                            <button type="button" onClick={() => removePendingFile(idx)} className="ml-1 hover:text-red-500">
                              <X className="h-3 w-3" />
                            </button>
                          </Badge>
                        ))}
                      </div>
                    )}
                    <div className="flex items-center justify-between gap-3">
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge variant="secondary" className="text-[10px]">
                          项目 · {currentProjectLabel}
                        </Badge>
                        <Badge variant="secondary" className="text-[10px]">
                          Lead · {currentDriverLabel}
                        </Badge>
                      </div>
                      <div className="flex items-center gap-2">
                        <Button
                          variant="outline"
                          size="icon"
                          className="h-10 w-10 shrink-0"
                          disabled={submitting || !draftSessionReady}
                          onClick={() => fileInputRef.current?.click()}
                          title="上传文件或图片"
                        >
                          <Paperclip className="h-4 w-4" />
                        </Button>
                        <Button
                          className="h-10 gap-2 px-4"
                          disabled={submitting || !draftSessionReady}
                          onClick={() => void sendMessage()}
                        >
                          <Send className="h-4 w-4" />
                          发送
                        </Button>
                      </div>
                    </div>
                    <div className="text-[10px] text-muted-foreground">Enter 发送 · Shift+Enter 换行 · 可粘贴图片</div>
                  </div>
                  {leadProfiles.length === 0 ? (
                    <div className="rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
                      还没有可用的 lead driver，请先到代理页面配置。
                    </div>
                  ) : drivers.length === 0 ? (
                    <div className="rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
                      还没有可用的 Driver，请先到代理页面配置。
                    </div>
                  ) : null}
                </div>
              </div>
            </div>
          ) : currentMessages.length === 0 ? (
            <div className="mx-auto w-full max-w-[920px] rounded-2xl border border-dashed bg-muted/20 px-5 py-6 text-sm text-muted-foreground">
              这个会话还没有消息。
            </div>
          ) : (
            <div className="mx-auto w-full max-w-[920px] space-y-4">
              {currentMessages.map((message) => (
                <div
                  key={message.id}
                  className={cn(
                    "group/msg flex max-w-[720px] gap-3",
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
                  <div className={cn("min-w-0 flex-1 space-y-2", message.role === "user" ? "text-right" : "")}>
                    <div className="relative">
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
                      {/* Sticky copy button for assistant messages */}
                      {message.role === "assistant" && (
                        <button
                          type="button"
                          className={cn(
                            "absolute right-2 top-2 z-10 flex h-7 w-7 items-center justify-center rounded-md transition-all",
                            copiedMessageId === message.id
                              ? "bg-emerald-100 text-emerald-600"
                              : "bg-white/80 text-muted-foreground opacity-0 shadow-sm backdrop-blur-sm hover:bg-white hover:text-foreground group-hover/msg:opacity-100",
                          )}
                          title="复制内容"
                          onClick={() => void handleCopyMessage(message.id, message.content)}
                        >
                          {copiedMessageId === message.id ? (
                            <Check className="h-3.5 w-3.5" />
                          ) : (
                            <ClipboardCopy className="h-3.5 w-3.5" />
                          )}
                        </button>
                      )}
                    </div>
                    {/* Hover action bar for assistant messages */}
                    {message.role === "assistant" && (
                      <div className="flex items-center gap-0.5 opacity-0 transition-opacity group-hover/msg:opacity-100">
                        <button
                          type="button"
                          className={cn(
                            "flex h-6 w-6 items-center justify-center rounded-md transition-colors",
                            copiedMessageId === message.id
                              ? "text-emerald-600"
                              : "text-muted-foreground hover:bg-muted hover:text-foreground",
                          )}
                          title="复制"
                          onClick={() => void handleCopyMessage(message.id, message.content)}
                        >
                          {copiedMessageId === message.id ? (
                            <Check className="h-3.5 w-3.5" />
                          ) : (
                            <ClipboardCopy className="h-3.5 w-3.5" />
                          )}
                        </button>
                        <button
                          type="button"
                          className={cn(
                            "flex h-6 w-6 items-center justify-center rounded-md transition-colors",
                            createdIssueMessageId === message.id
                              ? "text-emerald-600"
                              : "text-muted-foreground hover:bg-amber-50 hover:text-amber-600",
                          )}
                          title="创建 Issue"
                          onClick={() => void handleCreateIssueFromMessage(message.id, message.content)}
                        >
                          {createdIssueMessageId === message.id ? (
                            <Check className="h-3.5 w-3.5" />
                          ) : (
                            <ListTodo className="h-3.5 w-3.5" />
                          )}
                        </button>
                        <button
                          type="button"
                          className="flex h-6 w-6 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-blue-50 hover:text-blue-600"
                          title="创建 Flow"
                          onClick={() => void handleCreateFlowFromMessage(message.content)}
                        >
                          <Workflow className="h-3.5 w-3.5" />
                        </button>
                      </div>
                    )}
                    <span className="text-[10px] text-muted-foreground">{message.time}</span>
                  </div>
                </div>
              ))}
            </div>
          )}
          <div ref={messagesEndRef} />
        </div>

        {!isDraftSessionView ? (
          <div className="border-t p-4">
          {pendingFiles.length > 0 && (
            <div className="mb-2 flex flex-wrap gap-2">
              {pendingFiles.map((file, idx) => (
                <Badge key={idx} variant="secondary" className="gap-1 text-xs">
                  {file.name}
                  <button type="button" onClick={() => removePendingFile(idx)} className="ml-1 hover:text-red-500">
                    <X className="h-3 w-3" />
                  </button>
                </Badge>
              ))}
            </div>
          )}
          <div className="flex items-end gap-3">
            <div className="relative flex-1">
              <Input
                placeholder={
                  currentSession
                    ? "输入消息，与 Lead Agent 对话..."
                    : `输入消息，使用 Lead · ${currentDriverLabel} 在 ${currentProjectLabel} 下开始会话...`
                }
                className="pr-10"
                value={messageInput}
                disabled={submitting || currentSession?.status === "running" || (!currentSession && !draftSessionReady)}
                onChange={(event) => setMessageInput(event.target.value)}
                onPaste={handlePaste}
                onKeyDown={(event) => {
                  if (event.key === "Enter" && !event.shiftKey) {
                    event.preventDefault();
                    void sendMessage();
                  }
                }}
              />
            </div>
            <Button
              variant="outline"
              size="icon"
              className="h-10 w-10 shrink-0"
              disabled={submitting || currentSession?.status === "running" || (!currentSession && !draftSessionReady)}
              onClick={() => fileInputRef.current?.click()}
              title="上传文件或图片"
            >
              <Paperclip className="h-4 w-4" />
            </Button>
            <Button
              size="icon"
              className="h-10 w-10 shrink-0"
              disabled={submitting || currentSession?.status === "running" || (!currentSession && !draftSessionReady)}
              onClick={() => void sendMessage()}
            >
              <Send className="h-4 w-4" />
            </Button>
          </div>
          <div className="mt-2 text-[10px] text-muted-foreground">Enter 发送 · Shift+Enter 换行 · 可粘贴图片</div>
          </div>
        ) : null}
        <input
          ref={fileInputRef}
          type="file"
          multiple
          accept="image/*,.txt,.md,.json,.csv,.pdf,.yaml,.yml,.toml,.xml,.html,.css,.js,.ts,.tsx,.jsx,.go,.py,.rs,.java,.c,.cpp,.h,.hpp,.sh,.bat,.ps1,.sql,.log"
          className="hidden"
          onChange={handleFileSelect}
        />
      </div>
    </div>
  );
}
