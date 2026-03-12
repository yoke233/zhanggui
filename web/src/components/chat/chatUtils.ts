import type { TFunction } from "i18next";
import type { ChatMessage, ChatSessionSummary, ChatSessionDetail, Event as ApiEvent } from "@/types/apiV2";
import type {
  SessionRecord,
  ChatMessageView,
  ChatActivityView,
  ChatEventListItem,
  RealtimeChatOutputPayload,
} from "./chatTypes";
import { UNKNOWN_PROJECT_GROUP } from "./chatTypes";

export const formatMessageTime = (value: string): string => {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleTimeString("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
  });
};

export const formatActivityTime = (value: string): string => {
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

export const toMessageView = (sessionId: string, message: ChatMessage, index: number): ChatMessageView => ({
  id: `${sessionId}-${message.role}-${index}-${message.time}`,
  role: message.role === "assistant" ? "assistant" : "user",
  content: message.content,
  time: formatMessageTime(message.time),
  at: message.time,
});

export const toSummaryRecord = (session: ChatSessionSummary, t: TFunction): SessionRecord => ({
  ...session,
  title: session.title?.trim() || t("chat.newSession"),
});

export const toDetailRecord = (session: ChatSessionDetail, t: TFunction): SessionRecord => ({
  ...session,
  title: session.title?.trim() || t("chat.newSession"),
});

export const fallbackLabel = (value: string | null | undefined, fallback: string): string => {
  const trimmed = value?.trim();
  return trimmed ? trimmed : fallback;
};

export const toProjectGroupKey = (projectId?: number | null): string => (
  projectId == null ? UNKNOWN_PROJECT_GROUP : `project:${projectId}`
);

export const normalizeDriverKey = (driverId?: string): string => {
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

export const driverLabelForId = (driverId: string | undefined, t: TFunction): string => {
  switch (normalizeDriverKey(driverId)) {
    case "codex":
      return "Codex";
    case "claude":
      return "Claude";
    default:
      return fallbackLabel(driverId, t("chat.noDriver"));
  }
};

export const badgeVariantForStatus = (status?: string): "success" | "warning" | "secondary" => {
  switch (status) {
    case "running":
      return "success";
    case "alive":
      return "warning";
    default:
      return "secondary";
  }
};

export const badgeLabelForStatus = (status: string | undefined, t: TFunction): string => {
  switch (status) {
    case "running":
      return t("chat.active");
    case "alive":
      return t("chat.idle");
    default:
      return t("chat.closed");
  }
};

export const toStringValue = (value: unknown): string => (
  typeof value === "string" ? value : ""
);

export const toNumberValue = (value: unknown): number | undefined => {
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

export const isRecord = (value: unknown): value is Record<string, unknown> => (
  typeof value === "object" && value !== null && !Array.isArray(value)
);

export const compactText = (value: string, maxLength = 160): string => {
  const normalized = value.replace(/\s+/g, " ").trim();
  if (!normalized) {
    return "";
  }
  if (normalized.length <= maxLength) {
    return normalized;
  }
  return `${normalized.slice(0, maxLength)}...`;
};

export const stringifyJSON = (value: unknown): string | undefined => {
  if (value == null) {
    return undefined;
  }
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return undefined;
  }
};

export const extractTextPreview = (value: unknown): string => {
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

export const formatUsageValue = (value?: number): string => {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return "--";
  }
  return value.toLocaleString("zh-CN");
};

export const formatUsagePercent = (used?: number, size?: number): number | null => {
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

/**
 * Event severity levels for the events view filter.
 *  debug  — high-frequency streaming chunks, config sync
 *  info   — completed agent messages, usage stats, done
 *  warning — tool calls (need attention but not errors)
 *  error  — error events
 */
export type EventLevel = "debug" | "info" | "warning" | "error";

export const computeEventLevel = (rawType: string): EventLevel => {
  switch (rawType) {
    case "error":
      return "error";
    case "tool_call":
    case "tool_call_update":
    case "tool_call_completed":
      return "warning";
    case "agent_message":
    case "agent_thought":
    case "usage_update":
    case "done":
      return "info";
    default:
      // agent_message_chunk, config_option_update, available_commands_update, etc.
      return "debug";
  }
};

export const EVENT_LEVEL_ORDER: Record<EventLevel, number> = {
  debug: 0,
  info: 1,
  warning: 2,
  error: 3,
};

export const eventToneForType = (rawType: string): ChatEventListItem["tone"] => {
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

export const labelForEventType = (rawType: string, t: TFunction): string => {
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

export const toRealtimePayload = (event: ApiEvent): RealtimeChatOutputPayload => ({
  session_id: toStringValue(event.data?.session_id),
  type: toStringValue(event.data?.type),
  content: toStringValue(event.data?.content),
  tool_call_id: toStringValue(event.data?.tool_call_id),
  stderr: toStringValue(event.data?.stderr),
  exit_code: toNumberValue(event.data?.exit_code),
  usage_size: toNumberValue(event.data?.usage_size),
  usage_used: toNumberValue(event.data?.usage_used),
});

export const buildEventSummary = (event: ApiEvent, t: TFunction): { rawType: string; summary?: string; detail?: string } => {
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

export const toEventListItem = (event: ApiEvent, t: TFunction): ChatEventListItem => {
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

export const buildToolResultDetail = (payload: RealtimeChatOutputPayload, t: TFunction): string => {
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

export const touchSessionList = (
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

export const applyActivityPayload = (
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

export const buildRealtimeEvent = (id: number, at: string, payload: RealtimeChatOutputPayload): ApiEvent => ({
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

export const buildActivityHistory = (
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
