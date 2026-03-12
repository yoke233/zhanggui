import { startTransition, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import type { TFunction } from "i18next";
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
  Paperclip,
  Plus,
  Search,
  Send,
  X,
  Wrench,
} from "lucide-react";
import type {
  AgentDriver,
  AgentProfile,
  ChatMessage,
  ChatSessionDetail,
  ChatSessionSummary,
  ConfigOption,
  Event as ApiEvent,
  SlashCommand,
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

type ChatFeedItem =
  | { kind: "message"; data: ChatMessageView }
  | { kind: "thought"; data: ChatActivityView }
  | { kind: "tool_call"; data: ChatActivityView };

type ChatFeedEntry =
  | { type: "message"; item: ChatFeedItem & { kind: "message" } }
  | { type: "thought"; item: ChatFeedItem & { kind: "thought" } }
  | { type: "tool_group"; id: string; items: (ChatFeedItem & { kind: "tool_call" })[] };

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
  type: "agent_thought" | "tool_call" | "usage_update" | "agent_message";
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

const toSummaryRecord = (session: ChatSessionSummary, t: TFunction): SessionRecord => ({
  ...session,
  title: session.title?.trim() || t("chat.newSession"),
});

const toDetailRecord = (session: ChatSessionDetail, t: TFunction): SessionRecord => ({
  ...session,
  title: session.title?.trim() || t("chat.newSession"),
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

const driverLabelForId = (driverId: string | undefined, t: TFunction): string => {
  switch (normalizeDriverKey(driverId)) {
    case "codex":
      return "Codex";
    case "claude":
      return "Claude";
    default:
      return fallbackLabel(driverId, t("chat.noDriver"));
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

const badgeLabelForStatus = (status: string | undefined, t: TFunction): string => {
  switch (status) {
    case "running":
      return t("chat.active");
    case "alive":
      return t("chat.idle");
    default:
      return t("chat.closed");
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

const labelForEventType = (rawType: string, t: TFunction): string => {
  switch (rawType) {
    case "agent_message_chunk":
      return t("chat.deltaOutput");
    case "agent_message":
      return t("chat.replyDone");
    case "agent_thought":
      return t("chat.thinking");
    case "tool_call":
      return t("chat.toolCall");
    case "tool_call_update":
      return t("chat.toolUpdate");
    case "tool_call_completed":
      return t("chat.toolDone");
    case "usage_update":
      return t("chat.contextUsage");
    case "available_commands_update":
      return t("chat.commandListUpdate");
    case "config_option_update":
    case "config_options_update":
      return t("chat.sessionConfigUpdate");
    case "done":
      return t("chat.sessionComplete");
    case "error":
      return t("chat.sessionError");
    default:
      return rawType || t("chat.event");
  }
};

const buildEventSummary = (event: ApiEvent, t: TFunction): { rawType: string; summary?: string; detail?: string } => {
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
      summary: t("chat.usageStats", { used: formatUsageValue(usageUsed), total: formatUsageValue(usageSize) }),
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
      summary: names.length > 0 ? t("chat.nCommandsWithNames", { count: commands.length, names: names.join(", ") }) : t("chat.nCommands", { count: commands.length }),
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
      summary: names.length > 0 ? t("chat.nOptionsWithNames", { count: options.length, names: names.join(", ") }) : t("chat.nOptions", { count: options.length }),
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

const toEventListItem = (event: ApiEvent, t: TFunction): ChatEventListItem => {
  const { rawType, summary, detail } = buildEventSummary(event, t);
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
    label: labelForEventType(rawType, t),
    rawType,
    summary,
    detail,
    raw: shouldShowRaw ? stringifyJSON(event.data) : undefined,
    tone: eventToneForType(rawType),
  };
};

const buildToolResultDetail = (payload: RealtimeChatOutputPayload, t: TFunction): string => {
  const parts: string[] = [];
  if (typeof payload.exit_code === "number") {
    parts.push(t("chat.exitCode", { code: payload.exit_code }));
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
  t: TFunction,
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
      title: t("chat.thinkingState"),
      detail,
      time,
      at,
    });
    return next;
  }

  
  if (updateType === "agent_message") {
    const detail = payload.content?.trim();
    if (!detail) {
      return current;
    }
    const previous = next.at(-1);
    if (previous?.type === "agent_message") {
      next[next.length - 1] = {
        ...previous,
        detail: previous.detail ? `${previous.detail}\n${detail}` : detail,
        time,
        at,
      };
      return next;
    }
    next.push({
      id: `${sessionId}-message-${Date.parse(at)}-${next.length}`,
      type: "agent_message",
      title: t("chat.thinkingState"),
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
      title: payload.content?.trim() || previous?.title || t("chat.toolCall"),
      detail: previous?.detail || t("chat.executing"),
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
    const detail = buildToolResultDetail(payload, t) || previous?.detail || t("chat.completed");
    const activity: ChatActivityView = {
      id: previous?.id ?? `${sessionId}-tool-${toolCallId ?? `${Date.parse(at)}-${next.length}`}`,
      type: "tool_call",
      title: previous?.title || payload.content?.trim() || t("chat.toolCall"),
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
      title: t("chat.contextUsage"),
      detail: t("chat.usageStats", { used: formatUsageValue(usageUsed), total: formatUsageValue(usageSize) }),
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
  t: TFunction,
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
    return applyActivityPayload(activities, sessionId, payload, event.timestamp, t);
  }, []);
};

function EventLogRow({ item }: { item: ChatEventListItem }) {
  const { t } = useTranslation();
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
                <p className="text-xs text-muted-foreground">{t("chat.noSummary")}</p>
              )}
            </div>
            {hasExpandedContent ? (
              <button
                type="button"
                className="shrink-0 rounded-md px-2 py-1 text-[11px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                onClick={() => setExpanded((current) => !current)}
              >
                {expanded ? t("chat.collapse") : t("chat.expand")}
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
  const { t } = useTranslation();
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
  const [initialLoaded, setInitialLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [availableCommands, setAvailableCommands] = useState<SlashCommand[]>([]);
  const [configOptions, setConfigOptions] = useState<ConfigOption[]>([]);
  const [showCommandPalette, setShowCommandPalette] = useState(false);
  const [commandFilter, setCommandFilter] = useState("");
  const [collapsedActivityGroups, setCollapsedActivityGroups] = useState<Record<string, boolean>>({});
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const pendingChunkBuffersRef = useRef<Record<string, string>>({});
  const chunkFlushFrameRef = useRef<number | null>(null);
  const pendingRequestIdRef = useRef<string | null>(null);
  const syntheticEventIdRef = useRef(-1);

  const syncSessionDetail = (detail: ChatSessionDetail) => {
    const record = toDetailRecord(detail, t);
    // Only keep user messages from session detail; assistant messages come from events
    const userViews = detail.messages
      .filter((message) => message.role === "user")
      .map((message, index) => toMessageView(detail.session_id, message, index));

    setSessions((current) => {
      const existing = current.filter((item) => item.session_id !== detail.session_id);
      return [record, ...existing].sort((left, right) => (
        new Date(right.updated_at).getTime() - new Date(left.updated_at).getTime()
      ));
    });
    setMessagesBySession((current) => ({
      ...current,
      [detail.session_id]: userViews,
    }));
    setLoadedSessions((current) => ({
      ...current,
      [detail.session_id]: true,
    }));
    if (detail.available_commands) {
      setAvailableCommands(detail.available_commands);
    }
    if (detail.config_options) {
      setConfigOptions(detail.config_options);
    }
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
        [sessionId]: buildActivityHistory(sessionId, merged, t),
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
      const next = list.map((s) => toSummaryRecord(s, t));
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
      setInitialLoaded(true);
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
          label: driverLabelForId(driver.id, t),
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
    t("chat.noProject"),
  );
  const currentDriverId = currentSession?.driver_id ?? draftDriverId;
  const draftSessionReady = Boolean(draftProfileId && draftDriverId);
  const currentDriverLabel = currentDriverId
    ? leadDriverMap.get(currentDriverId)?.label ?? driverLabelForId(currentDriverId, t)
    : t("chat.noDriver");

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
          session.driver_id ? driverLabelForId(session.driver_id, t).toLowerCase() : "",
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
        label: fallbackLabel(session.project_name, t("chat.noProject")),
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
  const isDraftSessionView = initialLoaded && !currentSession && currentMessages.length === 0;
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
        .map((event) => toEventListItem(event, t)),
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

  const chatFeed = useMemo<ChatFeedItem[]>(() => {
    const items: ChatFeedItem[] = [];
    for (const msg of currentMessages) {
      items.push({ kind: "message", data: msg });
    }
    for (const act of currentActivities) {
      if (act.type === "agent_thought") {
        items.push({ kind: "thought", data: act });
      } else if (act.type === "tool_call") {
        items.push({ kind: "tool_call", data: act });
      } else if (act.type === "agent_message") {
        // Convert agent_message activities into assistant message bubbles
        items.push({
          kind: "message",
          data: {
            id: act.id,
            role: "assistant",
            content: act.detail || act.title,
            time: act.time,
            at: act.at,
          },
        });
      }
    }
    items.sort((a, b) => {
      const aAt = a.kind === "message" ? a.data.at : a.data.at;
      const bAt = b.kind === "message" ? b.data.at : b.data.at;
      return new Date(aAt).getTime() - new Date(bAt).getTime();
    });
    return items;
  }, [currentMessages, currentActivities]);

  const chatFeedEntries = useMemo<ChatFeedEntry[]>(() => {
    const entries: ChatFeedEntry[] = [];
    let toolBuffer: (ChatFeedItem & { kind: "tool_call" })[] = [];
    let groupCounter = 0;

    const flushTools = () => {
      if (toolBuffer.length > 0) {
        entries.push({ type: "tool_group", id: `tg-${groupCounter++}`, items: [...toolBuffer] });
        toolBuffer = [];
      }
    };

    for (const item of chatFeed) {
      if (item.kind === "tool_call") {
        toolBuffer.push(item);
      } else {
        flushTools();
        if (item.kind === "message") {
          entries.push({ type: "message", item });
        } else if (item.kind === "thought") {
          entries.push({ type: "thought", item });
        }
      }
    }
    flushTools();
    return entries;
  }, [chatFeed]);

  useEffect(() => {
    const isStreaming = currentMessages.at(-1)?.id.endsWith("stream-assistant");
    messagesEndRef.current?.scrollIntoView({ behavior: isStreaming ? "auto" : "smooth" });
  }, [currentEventItems, currentMessages, currentActivities, detailView]);

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
          // Remove the streaming bubble — the final message comes from activities
          setMessagesBySession((current) => {
            const existing = current[sessionId] ?? [];
            const last = existing.at(-1);
            if (last && last.id === `${sessionId}-stream-assistant`) {
              return {
                ...current,
                [sessionId]: existing.slice(0, -1),
              };
            }
            return current;
          });
          setSessions((current) => touchSessionList(current, sessionId, "running", nowISO));
          // Don't return — let it fall through to applyActivityPayload below
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
          setError(payload.content?.trim() || t("chat.sessionFailed"));
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
              t,
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
        setError(payload.error?.trim() || t("chat.sendFailed"));
        const sessionId = payload.session_id?.trim();
        if (sessionId) {
          setSessions((current) => touchSessionList(current, sessionId, "closed", new Date().toISOString()));
        }
      },
    );
    const unsubscribeConfigUpdate = wsClient.subscribe<{ session_id?: string; config_options?: ConfigOption[] }>(
      "chat.config_updated",
      (payload) => {
        if (payload.config_options) {
          setConfigOptions(payload.config_options);
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
      unsubscribeConfigUpdate();
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
              title: session.title === t("chat.newSession") && role === "user"
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
      setError(t("chat.selectDriverFirst"));
      return;
    }
    if (!resolvedDriverId) {
      setError(t("chat.selectDriverFirst"));
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
      ? `\n${t("chat.attachmentLabel", { names: attachments.map((a) => a.name).join(", ") })}`
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
          message: content || t("chat.attachment"),
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

  const handleCreateWorkItem = useCallback(
    (_messageId: string, content: string) => {
      const params = new URLSearchParams();
      params.set("body", content);
      if (selectedProjectId) params.set("project_id", String(selectedProjectId));
      navigate(`/work-items/new?${params.toString()}`);
    },
    [navigate, selectedProjectId],
  );

  return (
    <div className="flex h-full overflow-hidden">
      <div className="flex w-72 flex-col border-r bg-sidebar">
        <div className="border-b p-3">
          <div className="mb-3 flex items-center justify-between">
            <h2 className="text-sm font-semibold">{t("chat.sessionList")}</h2>
            <Button variant="outline" size="sm" className="h-8 gap-1.5 px-2.5 text-xs" onClick={createSession}>
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
                const preview = messagesBySession[session.session_id]?.at(-1)?.content ?? t("chat.noMessages");
                const turnCount = messagesBySession[session.session_id]?.length ?? 0;
                return (
                  <button
                    key={session.session_id}
                    onClick={() => setActiveSession(session.session_id)}
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
                              : "bg-emerald-50 text-emerald-500",
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

      <div className="flex flex-1 flex-col">
        {!isDraftSessionView ? (
          <div className="flex h-14 items-center justify-between border-b px-6">
            <div className="flex items-center gap-3">
              <div className="flex h-9 w-9 items-center justify-center rounded-full bg-primary text-primary-foreground">
                <Bot className="h-[18px] w-[18px]" />
              </div>
              <div className="min-w-0">
                <span className="truncate text-[15px] font-semibold">{currentSession?.title ?? "Lead Agent"}</span>
                <p className="text-xs text-muted-foreground">
                  Lead Agent · {currentDriverLabel} · {currentMessages.length} {t("chat.turns")}
                  {submitting ? <Loader2 className="ml-1.5 inline h-3 w-3 animate-spin" /> : null}
                </p>
              </div>
            </div>
            <div className="flex items-center gap-2">
              <span
                className={cn(
                  "inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs font-medium",
                  currentSession?.status === "running"
                    ? "bg-blue-50 text-blue-500"
                    : currentSession?.status === "alive"
                      ? "bg-amber-50 text-amber-500"
                      : "bg-emerald-50 text-emerald-500",
                )}
              >
                <span className={cn(
                  "h-1.5 w-1.5 rounded-full",
                  currentSession?.status === "running"
                    ? "bg-blue-500"
                    : currentSession?.status === "alive"
                      ? "bg-amber-500"
                      : "bg-emerald-500",
                )} />
                {badgeLabelForStatus(currentSession?.status, t)}
              </span>
              {currentUsage ? (
                <div className="flex items-center gap-1.5 rounded-full border bg-background px-2.5 py-1 text-[11px] text-muted-foreground">
                  <span className="shrink-0 whitespace-nowrap">{t("chat.context")}</span>
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
                  </span>
                </div>
              ) : null}
              <button
                type="button"
                className="h-8 rounded-md border px-3 text-[13px] font-medium transition-colors hover:bg-muted"
                onClick={() => void closeSession()}
              >
                {t("chat.endSession")}
              </button>
            </div>
          </div>
        ) : null}

        {error ? <p className="mx-5 mt-4 rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p> : null}

        <div className="flex-1 overflow-y-auto px-5 py-4 [scrollbar-gutter:stable]">
          {detailView === "events" ? (
            currentEventItems.length === 0 ? (
              <div className="mx-auto w-full max-w-[920px] rounded-2xl border border-dashed bg-muted/20 px-5 py-6 text-sm text-muted-foreground">
                {t("chat.noEvents")}
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
                    <p className="text-2xl font-semibold tracking-tight text-foreground">{t("chat.newSession")}</p>
                    <p className="text-sm text-muted-foreground">{t("chat.newSessionHint")}</p>
                  </div>
                  <div className="grid gap-4 md:grid-cols-2">
                  <div className="space-y-2">
                    <label className="text-xs font-medium uppercase tracking-[0.18em] text-slate-500">{t("common.project")}</label>
                    <Select
                      value={draftProjectId == null ? "" : String(draftProjectId)}
                      onChange={(event) => {
                        const next = event.target.value;
                        const nextProjectId = next ? Number(next) : null;
                        setDraftProjectId(nextProjectId);
                        setSelectedProjectId(nextProjectId);
                      }}
                    >
                      <option value="">{t("chat.noProject")}</option>
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
                      <option value={EMPTY_PROFILE_VALUE}>{t("chat.selectDriver")}</option>
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
                      placeholder={t("chat.inputPlaceholderNew", { driver: currentDriverLabel, project: currentProjectLabel })}
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
                          {t("chat.projectDot")}{currentProjectLabel}
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
                          title={t("chat.uploadFile")}
                        >
                          <Paperclip className="h-4 w-4" />
                        </Button>
                        <Button
                          className="h-10 gap-2 px-4"
                          disabled={submitting || !draftSessionReady}
                          onClick={() => void sendMessage()}
                        >
                          <Send className="h-4 w-4" />
                          {t("chat.send")}
                        </Button>
                      </div>
                    </div>
                    <div className="text-[10px] text-muted-foreground">{t("chat.inputHint")}</div>
                  </div>
                  {leadProfiles.length === 0 ? (
                    <div className="rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
                      {t("chat.noLeadDriver")}
                    </div>
                  ) : drivers.length === 0 ? (
                    <div className="rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
                      {t("chat.noDriverAvailable")}
                    </div>
                  ) : null}
                </div>
              </div>
            </div>
          ) : chatFeedEntries.length === 0 ? (
            <div className="mx-auto w-full max-w-[920px] rounded-2xl border border-dashed bg-muted/20 px-5 py-6 text-sm text-muted-foreground">
              {t("chat.noMessagesInSession")}
            </div>
          ) : (
            <div className="mx-auto w-full max-w-[920px] space-y-1">
              {chatFeedEntries.map((entry) => {
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
                        onClick={() => setCollapsedActivityGroups((prev) => ({ ...prev, [entry.id]: !isCollapsed }))}
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
                  <div key={message.id} className={cn(
                    "group/msg rounded-sm py-1.5",
                    isUser ? "bg-blue-50/60" : "",
                  )}>
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
                            onClick={() => void handleCopyMessage(message.id, message.content)}
                          >
                            {copiedMessageId === message.id ? <Check className="h-3.5 w-3.5" /> : <ClipboardCopy className="h-3.5 w-3.5" />}
                          </button>
                          <button
                            type="button"
                            className="flex h-6 w-6 items-center justify-center rounded text-muted-foreground transition-colors hover:text-amber-600"
                            title={t("chat.createWorkItem")}
                            onClick={() => handleCreateWorkItem(message.id, message.content)}
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
            </div>
          )}
          <div ref={messagesEndRef} />
        </div>

        {!isDraftSessionView ? (
          <div className="space-y-2 border-t px-6 py-4">
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
          <div className="relative">
          {showCommandPalette && availableCommands.length > 0 && (
            <div className="absolute bottom-full left-0 z-50 mb-1 w-full max-w-md rounded-lg border bg-background shadow-lg">
              <div className="max-h-48 overflow-y-auto p-1">
                {availableCommands
                  .filter((cmd) => !commandFilter || cmd.name.toLowerCase().includes(commandFilter.toLowerCase()))
                  .map((cmd) => (
                    <button
                      key={cmd.name}
                      type="button"
                      className="flex w-full items-center gap-2 rounded-md px-3 py-2 text-left text-sm transition-colors hover:bg-accent"
                      onClick={() => {
                        setMessageInput(`/${cmd.name} `);
                        setShowCommandPalette(false);
                        setCommandFilter("");
                      }}
                    >
                      <span className="font-mono text-xs font-medium text-foreground">/{cmd.name}</span>
                      {cmd.description && (
                        <span className="truncate text-xs text-muted-foreground">{cmd.description}</span>
                      )}
                    </button>
                  ))}
                {availableCommands.filter((cmd) => !commandFilter || cmd.name.toLowerCase().includes(commandFilter.toLowerCase())).length === 0 && (
                  <div className="px-3 py-2 text-xs text-muted-foreground">{t("chat.noCommandsMatch")}</div>
                )}
              </div>
            </div>
          )}
          <div className="flex items-center gap-2.5 rounded-lg border bg-background px-3.5 py-2.5">
            <Input
              placeholder={
                currentSession
                  ? t("chat.inputPlaceholderActive")
                  : t("chat.inputPlaceholderNew", { driver: currentDriverLabel, project: currentProjectLabel })
              }
              className="h-auto flex-1 border-0 p-0 text-sm shadow-none focus-visible:ring-0"
              value={messageInput}
              disabled={submitting || currentSession?.status === "running" || (!currentSession && !draftSessionReady)}
              onChange={(event) => {
                const val = event.target.value;
                setMessageInput(val);
                if (val.startsWith("/")) {
                  setShowCommandPalette(true);
                  setCommandFilter(val.slice(1).split(" ")[0]);
                } else {
                  setShowCommandPalette(false);
                  setCommandFilter("");
                }
              }}
              onPaste={handlePaste}
              onKeyDown={(event) => {
                if (event.key === "Escape" && showCommandPalette) {
                  setShowCommandPalette(false);
                  return;
                }
                if (event.key === "Enter" && !event.shiftKey) {
                  event.preventDefault();
                  setShowCommandPalette(false);
                  void sendMessage();
                }
              }}
              onBlur={() => {
                setTimeout(() => setShowCommandPalette(false), 150);
              }}
            />
            <div className="flex shrink-0 items-center gap-1.5">
              <button
                type="button"
                className="flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:text-foreground disabled:opacity-40"
                disabled={submitting || currentSession?.status === "running" || (!currentSession && !draftSessionReady)}
                onClick={() => fileInputRef.current?.click()}
                title={t("chat.uploadFile")}
              >
                <Paperclip className="h-[18px] w-[18px]" />
              </button>
              <Button
                size="icon"
                className="h-8 w-8"
                disabled={submitting || currentSession?.status === "running" || (!currentSession && !draftSessionReady)}
                onClick={() => void sendMessage()}
              >
                <Send className="h-4 w-4" />
              </Button>
            </div>
          </div>
          <div className="flex items-center justify-between text-[11px] text-muted-foreground">
            <div className="flex items-center gap-2">
              {currentSession?.project_name && (
                <Badge variant="secondary" className="text-[10px]">
                  {currentSession.project_name}
                </Badge>
              )}
              {currentSession?.branch && (
                <Badge variant="outline" className="font-mono text-[10px]">
                  {currentSession.branch}
                </Badge>
              )}
            </div>
            <span>{t("chat.inputHint")}</span>
          </div>
          </div>
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
