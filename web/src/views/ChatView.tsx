import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type { KeyboardEvent } from "react";
import { ApiError, type ApiClient } from "../lib/apiClient";
import type { ChatMessage } from "../types/workflow";
import type {
  ApiIssue,
  ChatRunEvent,
  ChatSessionStatus,
  RunCheckpoint,
} from "../types/api";
import type { WsClient } from "../lib/wsClient";
import type {
  ACPSessionUpdate,
  AvailableCommand,
  ChatEventPayload,
  ChatEventType,
  ConfigOption,
  WsEnvelope,
} from "../types/ws";
import FileTree from "../components/FileTree";
import GitStatusPanel from "../components/GitStatusPanel";
import CommandPalette from "../components/CommandPalette";
import ConfigSelector from "../components/ConfigSelector";
import { TuiMessage } from "../components/TuiMessage";
import { TuiActivityBlock } from "../components/TuiActivityBlock";
import { TuiMarkdown } from "../components/TuiMarkdown";
import { ScrollNavBar } from "../components/ScrollNavBar";
import type { ScrollMarker } from "../components/ScrollNavBar";
import { useChatStore } from "../stores/chatStore";

interface ChatViewProps {
  apiClient: ApiClient;
  wsClient: WsClient;
  projectId: string;
}

interface ChatSessionSummary {
  id: string;
  updatedAt: string;
  preview: string;
}

interface ChatSessionLike {
  id?: unknown;
  updated_at?: unknown;
  created_at?: unknown;
  messages?: unknown;
}

interface RunEventItem {
  id: string;
  sessionId: string;
  type: string;
  detail: string;
  time: string;
  groupKey?: string;
  groupId?: string;
}

type ChatTimelineItem =
  | {
      id: string;
      kind: "message";
      time: string;
      role: ChatMessage["role"];
      content: string;
    }
  | {
      id: string;
      kind: "activity";
      time: string;
      activityType: string;
      detail: string;
      groupKey?: string;
      groupId?: string;
    };

interface ToolCallGroupState {
  loading: boolean;
  error?: string;
  items: RunEventItem[];
}

const MAX_RUN_EVENTS = 60;
const CROSS_TERMINAL_RUN_NOTICE =
  "当前会话正在其他终端运行，界面已进入同步监听。";
const CHAT_SESSION_BUSY_NOTICE = "该会话正在其他终端运行，已自动同步最新状态。";
const CHAT_TIMELINE_ACTIVITY_TYPES = new Set([
  "agent_thought",
  "tool_call",
  "tool_call_group",
  "plan",
]);

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
  agent_message_chunk: (acp) => toChunkValue(acp.content?.text),
  assistant_message_chunk: (acp) => toChunkValue(acp.content?.text),
  message_chunk: (acp) => toChunkValue(acp.content?.text),
};

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error && error.message.trim().length > 0) {
    return error.message;
  }
  return "请求失败，请稍后重试";
};


const toStringValue = (value: unknown): string => {
  if (typeof value !== "string") {
    return "";
  }
  return value.trim();
};

const toChunkValue = (value: unknown): string => {
  if (typeof value !== "string") {
    return "";
  }
  return value;
};

const sanitizeAgentThoughtDetail = (value: string): string => {
  let next = value;
  const prefixes = ["agent_thought_chunk", "agent_thought"];
  let changed = true;
  while (changed) {
    changed = false;
    const trimmed = next.trimStart();
    for (const prefix of prefixes) {
      if (!trimmed.startsWith(prefix)) {
        continue;
      }
      const suffix = trimmed.slice(prefix.length);
      if (suffix.length === 0 || /^[\s:：-]|[A-Z\u4e00-\u9fff]/.test(suffix)) {
        next = suffix.trimStart();
        changed = true;
        break;
      }
    }
  }
  return next;
};

const toRecord = (value: unknown): Record<string, unknown> | null => {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return null;
  }
  return value as Record<string, unknown>;
};

const toRecordList = (value: unknown): Record<string, unknown>[] => {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map((item) => toRecord(item))
    .filter((item): item is Record<string, unknown> => item !== null);
};

const normalizeAvailableCommands = (value: unknown): AvailableCommand[] => {
  const commands: AvailableCommand[] = [];
  toRecordList(value).forEach((item) => {
    const name = toStringValue(item.name);
    if (!name) {
      return;
    }
    const input = toRecord(item.input);
    commands.push({
      name,
      description: toStringValue(item.description),
      input: input
        ? {
            hint: toStringValue(input.hint),
          }
        : undefined,
    });
  });
  return commands;
};

const normalizeConfigOptionValues = (
  value: unknown,
): ConfigOption["options"] => {
  if (!Array.isArray(value)) {
    return [];
  }
  const directValues: ConfigOption["options"] = [];
  value.forEach((item) => {
    const record = toRecord(item);
    const optionValue = toStringValue(record?.value);
    const name = toStringValue(record?.name);
    if (!optionValue || !name) {
      return;
    }
    directValues.push({
      value: optionValue,
      name,
      description: toStringValue(record?.description),
    });
  });
  if (directValues.length > 0) {
    return directValues;
  }
  return value.flatMap((group) => {
    const record = toRecord(group);
    return normalizeConfigOptionValues(record?.options);
  });
};

const normalizeConfigOptions = (value: unknown): ConfigOption[] => {
  const options: ConfigOption[] = [];
  toRecordList(value).forEach((item) => {
    const id = toStringValue(item.id);
    const name = toStringValue(item.name);
    const currentValue = toStringValue(item.currentValue);
    const type = toStringValue(item.type) || "select";
    if (!id || !name || !currentValue || type !== "select") {
      return;
    }
    options.push({
      id,
      name,
      description: toStringValue(item.description),
      category: toStringValue(item.category),
      type: "select",
      currentValue,
      options: normalizeConfigOptionValues(item.options),
    });
  });
  return options;
};

const getLatestMessagePreview = (rawMessages: unknown): string => {
  if (!Array.isArray(rawMessages)) {
    return "";
  }
  for (let index = rawMessages.length - 1; index >= 0; index -= 1) {
    const message = toRecord(rawMessages[index]);
    const content = toStringValue(message?.content);
    if (content) {
      return content.length > 80 ? `${content.slice(0, 80)}...` : content;
    }
  }
  return "";
};

const extractACPContentBlocks = (rawContent: unknown): string[] => {
  if (Array.isArray(rawContent)) {
    return toRecordList(rawContent)
      .map((item) => {
        const nested = toRecord(item.content);
        return (
          toChunkValue(item.text) ||
          toChunkValue(nested?.text) ||
          toChunkValue(item.content)
        );
      })
      .filter((text) => text.length > 0);
  }
  const nested = toRecord(rawContent);
  const text = toChunkValue(nested?.text) || toChunkValue(rawContent);
  return text ? [text] : [];
};

const toChatSessionSummary = (raw: unknown): ChatSessionSummary | null => {
  const session = toRecord(raw) as ChatSessionLike | null;
  const id = toStringValue(session?.id);
  if (!id) {
    return null;
  }
  const updatedAt =
    toStringValue(session?.updated_at) ||
    toStringValue(session?.created_at) ||
    nowIso();
  return {
    id,
    updatedAt,
    preview: getLatestMessagePreview(session?.messages),
  };
};

const extractChatSessions = (raw: unknown): ChatSessionSummary[] => {
  const listSource = Array.isArray(raw)
    ? raw
    : Array.isArray((raw as { items?: unknown })?.items)
      ? ((raw as { items: unknown[] }).items ?? [])
      : [];
  return listSource
    .map((item) => toChatSessionSummary(item))
    .filter((item): item is ChatSessionSummary => item !== null)
    .sort(
      (a, b) =>
        new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime(),
    );
};

const buildRunEventDetail = (data: ChatEventPayload): string => {
  const acp = toRecord(data.acp);
  const updateType = toStringValue(acp?.sessionUpdate);
  if (!updateType) {
    return "收到增量更新";
  }
  if (isAgentThoughtUpdateType(updateType)) {
    return sanitizeAgentThoughtDetail(getAgentThoughtDelta(data));
  }

  const title = toStringValue(acp?.title);
  const status = toStringValue(acp?.status);
  const kind = toStringValue(acp?.kind);
  const toolCallID = toStringValue(acp?.toolCallId);
  const entries = toRecordList(acp?.entries)
    .map((entry) => {
      const content = toStringValue(entry.content);
      const entryStatus = toStringValue(entry.status);
      const priority = toStringValue(entry.priority);
      const parts = [content];
      if (entryStatus) {
        parts.push(`status=${entryStatus}`);
      }
      if (priority) {
        parts.push(`priority=${priority}`);
      }
      return parts.filter((part) => part.length > 0).join(" · ");
    })
    .filter((entryText) => entryText.length > 0);
  const rawContent = acp?.content;
  const contentBlocks = extractACPContentBlocks(rawContent);
  const locations = toRecordList(acp?.locations)
    .map((item) => toStringValue(item.path) || toStringValue(item.uri))
    .filter((value) => value.length > 0);
  const rawInput = acp?.rawInput ? JSON.stringify(acp.rawInput, null, 2) : "";
  const rawOutput = acp?.rawOutput
    ? JSON.stringify(acp.rawOutput, null, 2)
    : "";

  if (updateType === "plan") {
    if (entries.length > 0) {
      return entries.map((entry) => `- ${entry}`).join("\n");
    }
    return "plan";
  }

  if (updateType === "usage_update") {
    const size = toStringValue(acp?.size);
    const used = toStringValue(acp?.used);
    const parts = ["usage_update"];
    if (size) {
      parts.push(`size=${size}`);
    }
    if (used) {
      parts.push(`used=${used}`);
    }
    return parts.join(" · ");
  }

  if (updateType === "tool_call_group") {
    const preview = toStringValue(data.preview) || toStringValue(acp?.preview);
    const itemCount =
      toStringValue(data.item_count) ||
      toStringValue(acp?.itemCount) ||
      toStringValue(acp?.item_count);
    const parts = [preview || "工具调用组"];
    if (itemCount) {
      parts.push(`${itemCount} 项`);
    }
    return parts.join(" · ");
  }

  if (isToolCallUpdateType(updateType)) {
    const lines: string[] = [];
    const headlineParts = [
      title || updateType,
      kind && `kind=${kind}`,
      status && `status=${status}`,
    ].filter(
      (part): part is string => typeof part === "string" && part.length > 0,
    );
    if (headlineParts.length > 0) {
      lines.push(headlineParts.join(" · "));
    }
    if (toolCallID) {
      lines.push(`toolCallId=${toolCallID}`);
    }
    if (contentBlocks.length > 0) {
      lines.push(...contentBlocks);
    }
    if (locations.length > 0) {
      lines.push(`locations=${locations.join(", ")}`);
    }
    if (rawInput) {
      lines.push(`rawInput:\n\`\`\`json\n${rawInput}\n\`\`\``);
    }
    if (rawOutput) {
      lines.push(`rawOutput:\n\`\`\`json\n${rawOutput}\n\`\`\``);
    }
    return lines.join("\n");
  }

  const fragments = [updateType];
  if (title) {
    fragments.push(`title=${title}`);
  }
  if (kind) {
    fragments.push(`kind=${kind}`);
  }
  if (status) {
    fragments.push(`status=${status}`);
  }
  if (toolCallID) {
    fragments.push(`toolCallId=${toolCallID}`);
  }
  if (entries.length > 0) {
    fragments.push(`entries=${entries.join(" | ")}`);
  }
  if (contentBlocks.length > 0) {
    fragments.push(`content=${contentBlocks.join(" | ")}`);
  }
  return fragments.join(" · ");
};

const isAgentThoughtUpdateType = (updateType: string): boolean => {
  return updateType === "agent_thought_chunk" || updateType === "agent_thought";
};

const isToolCallUpdateType = (updateType: string): boolean => {
  return (
    updateType === "tool_call" ||
    updateType === "tool_call_update" ||
    updateType === "tool_call_completed"
  );
};

const normalizeRunEventType = (
  rawEventType: unknown,
  payload?: ChatEventPayload | null,
  rawUpdateType?: unknown,
): string => {
  const updateType =
    toStringValue(rawUpdateType) ||
    (payload ? toStringValue(payload.acp?.sessionUpdate) : "");
  if (isAgentThoughtUpdateType(updateType)) {
    return "agent_thought";
  }
  if (isToolCallUpdateType(updateType)) {
    return "tool_call";
  }
  if (updateType) {
    return updateType;
  }
  return toStringValue(rawEventType) || "run_update";
};

const resolveRunEventGroupKey = (
  normalizedType: string,
  payload?: ChatEventPayload | null,
): string | undefined => {
  if (normalizedType !== "tool_call" || !payload) {
    return undefined;
  }
  const toolCallID = toStringValue(payload.acp?.toolCallId);
  return toolCallID ? `tool_call:${toolCallID}` : undefined;
};

const resolveEventGroupId = (
  normalizedType: string,
  payload?: ChatEventPayload | null,
): string | undefined => {
  if (normalizedType !== "tool_call_group" || !payload) {
    return undefined;
  }
  return toStringValue(payload.group_id) || toStringValue(payload.acp?.groupId);
};

const getAgentThoughtDelta = (payload: ChatEventPayload): string => {
  const updateType = toStringValue(payload.acp?.sessionUpdate);
  if (!isAgentThoughtUpdateType(updateType)) {
    return "";
  }
  const contentBlocks = extractACPContentBlocks(payload.acp?.content);
  const delta =
    contentBlocks.join("") ||
    toChunkValue(payload.text) ||
    toChunkValue(payload.reply);
  const normalizedDelta = sanitizeAgentThoughtDetail(delta);
  return normalizedDelta.trim().length > 0 ? normalizedDelta : "";
};

const toStoredRunEventItem = (event: ChatRunEvent): RunEventItem => {
  const payload = toRecord(event.payload) as ChatEventPayload | null;
  const normalizedType = normalizeRunEventType(
    event.event_type,
    payload,
    event.update_type,
  );
  const thoughtDelta = payload ? getAgentThoughtDelta(payload) : "";
  let detail = "";
  if (thoughtDelta) {
    detail = thoughtDelta;
  } else if (payload) {
    if (normalizedType === "tool_call_group") {
      const preview = toStringValue(payload.preview);
      const itemCount = toStringValue(payload.item_count);
      detail = [preview || "工具调用组", itemCount ? `${itemCount} 项` : ""]
        .filter((part) => part.length > 0)
        .join(" · ");
    } else {
      detail = buildRunEventDetail(payload);
    }
    if (normalizedType === "agent_thought") {
      detail = sanitizeAgentThoughtDetail(detail);
    }
    if (
      (!detail || detail === "收到增量更新") &&
      normalizedType !== "agent_thought"
    ) {
      detail = toStringValue(payload.text) || toStringValue(payload.error);
    }
  }
  if (!detail && normalizedType !== "agent_thought") {
    detail =
      toStringValue(event.update_type) ||
      toStringValue(event.event_type) ||
      "历史运行事件";
  }
  return {
    id: `stored-${event.id}`,
    sessionId: event.session_id,
    type: normalizedType,
    detail,
    time: toStringValue(event.created_at) || nowIso(),
    groupKey: resolveRunEventGroupKey(normalizedType, payload),
    groupId: resolveEventGroupId(normalizedType, payload),
  };
};

const mergeAdjacentAgentThoughtItems = (
  items: RunEventItem[],
): RunEventItem[] => {
  const merged: RunEventItem[] = [];
  items.forEach((item) => {
    if (item.type === "agent_thought" && item.detail.trim().length === 0) {
      return;
    }
    if (item.groupKey) {
      const existingIndex = merged.findIndex(
        (candidate) =>
          candidate.sessionId === item.sessionId &&
          candidate.groupKey === item.groupKey,
      );
      if (existingIndex >= 0) {
        const existing = merged[existingIndex];
        merged[existingIndex] = {
          ...existing,
          detail:
            existing.detail === item.detail || item.detail.length === 0
              ? existing.detail
              : `${existing.detail}\n${item.detail}`,
          time: item.time || existing.time,
        };
        return;
      }
    }
    if (item.type !== "agent_thought") {
      merged.push(item);
      return;
    }
    const previous = merged[merged.length - 1];
    if (
      previous &&
      previous.type === "agent_thought" &&
      previous.sessionId === item.sessionId
    ) {
      merged[merged.length - 1] = {
        ...previous,
        detail: `${previous.detail}${item.detail}`,
        time: item.time || previous.time,
      };
      return;
    }
    merged.push(item);
  });
  return merged;
};

const toStoredRunEventItems = (events: ChatRunEvent[]): RunEventItem[] => {
  return mergeAdjacentAgentThoughtItems(events.map(toStoredRunEventItem));
};

const areRunEventListsEqual = (
  left: RunEventItem[],
  right: RunEventItem[],
): boolean => {
  if (left.length !== right.length) {
    return false;
  }
  return left.every((item, index) => {
    const candidate = right[index];
    return (
      candidate &&
      item.id === candidate.id &&
      item.sessionId === candidate.sessionId &&
      item.type === candidate.type &&
      item.detail === candidate.detail &&
      item.time === candidate.time &&
      item.groupKey === candidate.groupKey
    );
  });
};

const messageIdentityKey = (message: ChatMessage): string => {
  return `${message.role}|${message.time}|${message.content}`;
};

const mergeChatMessages = (
  current: ChatMessage[],
  incoming: ChatMessage[],
  mode: "replace" | "prepend",
): ChatMessage[] => {
  if (mode === "replace") {
    return incoming;
  }
  const seen = new Set<string>();
  const merged: ChatMessage[] = [];
  [...incoming, ...current].forEach((message) => {
    const key = messageIdentityKey(message);
    if (seen.has(key)) {
      return;
    }
    seen.add(key);
    merged.push(message);
  });
  return merged;
};

const mergeRunEventPages = (
  current: RunEventItem[],
  incoming: RunEventItem[],
  sessionId: string,
  mode: "replace" | "prepend",
): RunEventItem[] => {
  if (mode === "replace") {
    const otherSessionEvents = current.filter(
      (event) => event.sessionId !== sessionId,
    );
    return [...otherSessionEvents, ...mergeAdjacentAgentThoughtItems(incoming)];
  }

  const otherSessionEvents = current.filter(
    (event) => event.sessionId !== sessionId,
  );
  const currentSessionEvents = current.filter(
    (event) => event.sessionId === sessionId,
  );
  const seen = new Set<string>();
  const mergedSessionEvents: RunEventItem[] = [];
  [...incoming, ...currentSessionEvents].forEach((event) => {
    if (seen.has(event.id)) {
      return;
    }
    seen.add(event.id);
    mergedSessionEvents.push(event);
  });
  return [
    ...otherSessionEvents,
    ...mergeAdjacentAgentThoughtItems(mergedSessionEvents),
  ];
};

const nowIso = (): string => new Date().toISOString();

const toEventTimestampMs = (value: unknown): number => {
  const raw = toStringValue(value);
  if (!raw) {
    return 0;
  }
  const parsed = new Date(raw).getTime();
  if (Number.isNaN(parsed)) {
    return 0;
  }
  return parsed;
};

const getStreamingDelta = (payload: ChatEventPayload): string => {
  const acp = payload.acp;
  if (!acp || typeof acp !== "object") {
    return "";
  }
  const updateType = toStringValue(acp.sessionUpdate);
  if (!updateType) {
    return "";
  }
  const parser = CHAT_UPDATE_PARSERS[updateType];
  if (!parser) {
    return "";
  }
  return parser(acp);
};

const ChatView = ({ apiClient, wsClient, projectId }: ChatViewProps) => {
  const commandsBySessionId = useChatStore((state) => state.commandsBySessionId);
  const configOptionsBySessionId = useChatStore(
    (state) => state.configOptionsBySessionId,
  );
  const setCommands = useChatStore((state) => state.setCommands);
  const setConfigOptions = useChatStore((state) => state.setConfigOptions);
  const [draft, setDraft] = useState("");
  const [selectedFiles, setSelectedFiles] = useState<string[]>([]);
  const [leftPanelTab, setLeftPanelTab] = useState<"tree" | "git">("tree");
  const [leftPanelOpen, setLeftPanelOpen] = useState(false);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [streamingText, setStreamingText] = useState("");
  const [isStreaming, setIsStreaming] = useState(false);
  const [chatLoading, setChatLoading] = useState(false);
  const [chatCancelling, setChatCancelling] = useState(false);
  const [issueFromFilesLoading, setIssueFromFilesLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [issueNotice, setIssueNotice] = useState<string | null>(null);
  const [crossTerminalRunNotice, setCrossTerminalRunNotice] = useState<
    string | null
  >(null);
  const [chatSessions, setChatSessions] = useState<ChatSessionSummary[]>([]);
  const [chatsLoading, setChatsLoading] = useState(false);
  const [chatsError, setChatsError] = useState<string | null>(null);
  const [sessionStatuses, setSessionStatuses] = useState<
    Record<string, ChatSessionStatus>
  >({});
  const [runEvents, setRunEvents] = useState<RunEventItem[]>([]);
  const [historyCursor, setHistoryCursor] = useState<string | null>(null);
  const [historyLoadingMore, setHistoryLoadingMore] = useState(false);
  const [toolCallGroupStates, setToolCallGroupStates] = useState<
    Record<string, ToolCallGroupState>
  >({});
  const [issueList, setIssueList] = useState<ApiIssue[]>([]);
  const [issueListLoading, setIssueListLoading] = useState(false);
  const [selectedIssueId, setSelectedIssueId] = useState<string | null>(null);
  const [issueCheckpoints, setIssueCheckpoints] = useState<RunCheckpoint[]>([]);
  const [checkpointsLoading, setCheckpointsLoading] = useState(false);
  const [wakingStage, setWakingStage] = useState<string | null>(null);
  const [selectedCommandIndex, setSelectedCommandIndex] = useState(0);
  const [updatingConfigId, setUpdatingConfigId] = useState<string | null>(null);
  const [agents, setAgents] = useState<Array<{ name: string }>>([]);
  const [selectedAgent, setSelectedAgent] = useState("claude");
  const chatRequestIdRef = useRef(0);
  const issueFromFilesRequestIdRef = useRef(0);
  const chatListRequestIdRef = useRef(0);
  const runEventsRequestIdRef = useRef(0);
  const activeRunStartedAtRef = useRef(0);
  const localRunStartPendingRef = useRef(false);
  const messageInputRef = useRef<HTMLTextAreaElement | null>(null);
  const timelineScrollRef = useRef<HTMLDivElement | null>(null);
  const messagesEndRef = useRef<HTMLDivElement | null>(null);
  const pendingScrollRestoreRef = useRef<{
    scrollTop: number;
    scrollHeight: number;
  } | null>(null);

  useEffect(() => {
    chatRequestIdRef.current += 1;
    issueFromFilesRequestIdRef.current += 1;
    setDraft("");
    setSelectedFiles([]);
    setLeftPanelTab("tree");
    setSessionId(null);
    setMessages([]);
    setStreamingText("");
    setIsStreaming(false);
    setError(null);
    setIssueNotice(null);
    setCrossTerminalRunNotice(null);
    setChatSessions([]);
    setChatsLoading(false);
    setChatsError(null);
    setRunEvents([]);
    setHistoryCursor(null);
    setHistoryLoadingMore(false);
    setToolCallGroupStates({});
    setIssueList([]);
    setSelectedIssueId(null);
    setIssueCheckpoints([]);
    setSelectedCommandIndex(0);
    setUpdatingConfigId(null);
    setChatLoading(false);
    setChatCancelling(false);
    setIssueFromFilesLoading(false);
    chatListRequestIdRef.current += 1;
    runEventsRequestIdRef.current += 1;
    activeRunStartedAtRef.current = 0;
    localRunStartPendingRef.current = false;
  }, [projectId]);

  useEffect(() => {
    void apiClient.listAgents().then((res) => {
      setAgents(res.agents ?? []);
      if (res.agents?.length > 0) {
        setSelectedAgent(res.agents[0].name);
      }
    }).catch(() => {});
  }, [apiClient]);

  const syncingFromOtherTerminal = chatLoading && !!crossTerminalRunNotice;
  const sessionCommands = useMemo(
    () => (sessionId ? commandsBySessionId[sessionId] ?? [] : []),
    [commandsBySessionId, sessionId],
  );
  const sessionConfigOptions = useMemo(
    () => (sessionId ? configOptionsBySessionId[sessionId] ?? [] : []),
    [configOptionsBySessionId, sessionId],
  );
  const commandPaletteQuery = useMemo(() => {
    const trimmed = draft.trimStart();
    if (!trimmed.startsWith("/")) {
      return null;
    }
    const body = trimmed.slice(1);
    if (body.includes(" ") || body.includes("\n")) {
      return null;
    }
    return body.toLowerCase();
  }, [draft]);
  const filteredCommands = useMemo(() => {
    if (commandPaletteQuery === null) {
      return [];
    }
    return sessionCommands.filter((command) => {
      if (!commandPaletteQuery) {
        return true;
      }
      return command.name.toLowerCase().includes(commandPaletteQuery);
    });
  }, [commandPaletteQuery, sessionCommands]);
  const showCommandPalette = filteredCommands.length > 0;
  const canSubmit = chatLoading
    ? !!sessionId && !chatCancelling && !syncingFromOtherTerminal
    : draft.trim().length > 0;
  const filePaths = selectedFiles;
  const canCreateIssueFromFiles =
    !!sessionId &&
    filePaths.length > 0 &&
    !issueFromFilesLoading &&
    !chatLoading;

  useEffect(() => {
    setSelectedCommandIndex(0);
  }, [commandPaletteQuery, sessionId]);

  const sortedMessages = useMemo(
    () =>
      [...messages].sort((a, b) => {
        return new Date(a.time).getTime() - new Date(b.time).getTime();
      }),
    [messages],
  );
  const timelineItems = useMemo<ChatTimelineItem[]>(() => {
    const sessionScopedRunEvents = sessionId
      ? runEvents.filter(
          (event) =>
            event.sessionId === sessionId &&
            CHAT_TIMELINE_ACTIVITY_TYPES.has(event.type),
        )
      : [];

    const messageItems: ChatTimelineItem[] = sortedMessages.map(
      (message, index) => ({
        id: `message-${message.time}-${index}`,
        kind: "message",
        time: message.time,
        role: message.role,
        content: message.content,
      }),
    );
    const activityItems: ChatTimelineItem[] = sessionScopedRunEvents.map(
      (event) => ({
        id: `activity-${event.id}`,
        kind: "activity",
        time: event.time,
        activityType: event.type,
        detail: event.detail,
        groupKey: event.groupKey,
        groupId: event.groupId,
      }),
    );

    return [...messageItems, ...activityItems].sort((left, right) => {
      const timeDiff =
        new Date(left.time).getTime() - new Date(right.time).getTime();
      if (timeDiff !== 0) {
        return timeDiff;
      }
      if (left.kind === right.kind) {
        return left.id.localeCompare(right.id);
      }
      return left.kind === "message" ? -1 : 1;
    });
  }, [runEvents, sessionId, sortedMessages]);
  const hasMessages = timelineItems.length > 0 || isStreaming;

  const userMessageMarkers = useMemo<ScrollMarker[]>(() => {
    const total = timelineItems.length || 1;
    return timelineItems
      .map((item, idx) => ({ item, idx }))
      .filter(({ item }) => item.kind === "message" && item.role === "user")
      .map(({ item, idx }) => ({
        id: item.id,
        label:
          item.kind === "message"
            ? item.content.length > 30
              ? `${item.content.slice(0, 30)}...`
              : item.content
            : "",
        position: idx / total,
      }));
  }, [timelineItems]);

  const handleNavMarkerClick = useCallback((markerId: string) => {
    const el = document.getElementById(markerId);
    if (el) {
      el.scrollIntoView({ behavior: "smooth", block: "start" });
    }
  }, []);

  const listChats = apiClient.listChats;

  const upsertChatSessionSummary = useCallback(
    (session: ChatSessionSummary) => {
      setChatSessions((prev) => {
        const next = prev.filter((item) => item.id !== session.id);
        next.push(session);
        next.sort(
          (a, b) =>
            new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime(),
        );
        return next;
      });
    },
    [],
  );

  const pushRunEvent = useCallback(
    (
      session: string,
      type: string,
      detail: string,
      options?: { time?: string; groupKey?: string },
    ) => {
      setRunEvents((prev) => {
        const normalizedDetail =
          type === "agent_thought"
            ? sanitizeAgentThoughtDetail(detail)
            : detail;
        const nextEvent: RunEventItem = {
          id: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
          sessionId: session,
          type,
          detail: normalizedDetail,
          time: options?.time || nowIso(),
          groupKey: options?.groupKey,
        };
        let next: RunEventItem[];
        if (nextEvent.groupKey) {
          const existingIndex = prev.findIndex(
            (item) =>
              item.sessionId === nextEvent.sessionId &&
              item.groupKey === nextEvent.groupKey,
          );
          if (existingIndex >= 0) {
            const existing = prev[existingIndex];
            const mergedDetail =
              existing.detail === nextEvent.detail ||
              nextEvent.detail.length === 0
                ? existing.detail
                : `${existing.detail}\n${nextEvent.detail}`;
            next = [
              ...prev.slice(0, existingIndex),
              {
                ...existing,
                detail: mergedDetail,
                time: nextEvent.time,
              },
              ...prev.slice(existingIndex + 1),
            ];
          } else {
            next = [...prev, nextEvent];
          }
        } else {
          const previous = prev[prev.length - 1];
          if (
            nextEvent.type === "agent_thought" &&
            previous &&
            previous.type === "agent_thought" &&
            previous.sessionId === nextEvent.sessionId
          ) {
            next = [
              ...prev.slice(0, -1),
              {
                ...previous,
                detail: `${previous.detail}${nextEvent.detail}`,
                time: nextEvent.time,
              },
            ];
          } else {
            next = [...prev, nextEvent];
          }
        }
        if (next.length <= MAX_RUN_EVENTS) {
          return next;
        }
        return next.slice(next.length - MAX_RUN_EVENTS);
      });
    },
    [],
  );

  const refreshChatSessions = useCallback(
    async (targetProjectId: string) => {
      const requestId = chatListRequestIdRef.current + 1;
      chatListRequestIdRef.current = requestId;
      setChatsLoading(true);
      setChatsError(null);
      try {
        const response = await listChats(targetProjectId);
        if (chatListRequestIdRef.current !== requestId) {
          return;
        }
        const sessions = extractChatSessions(response);
        setChatSessions(sessions);
        // Fetch ACP session statuses in parallel.
        const statusEntries = await Promise.all(
          sessions.map(async (s) => {
            try {
              const st = await apiClient.getChatSessionStatus(
                targetProjectId,
                s.id,
              );
              return [s.id, st] as const;
            } catch {
              return [s.id, { alive: false, running: false }] as const;
            }
          }),
        );
        if (chatListRequestIdRef.current === requestId) {
          setSessionStatuses(Object.fromEntries(statusEntries));
        }
      } catch (listError) {
        if (chatListRequestIdRef.current !== requestId) {
          return;
        }
        setChatsError(getErrorMessage(listError));
      } finally {
        if (chatListRequestIdRef.current === requestId) {
          setChatsLoading(false);
        }
      }
    },
    [listChats, apiClient],
  );

  const refreshChatRunEvents = useCallback(
    async (
      targetProjectId: string,
      targetSessionId: string,
      options?: { cursor?: string | null; mode?: "replace" | "prepend" },
    ) => {
      const normalizedSessionID = targetSessionId.trim();
      if (!normalizedSessionID) {
        return;
      }
      const mode = options?.mode ?? "replace";
      const requestId = runEventsRequestIdRef.current + 1;
      runEventsRequestIdRef.current = requestId;
      try {
        const page = await apiClient.listChatRunEvents(
          targetProjectId,
          normalizedSessionID,
          {
            limit: 50,
            cursor: options?.cursor ?? undefined,
          },
        );
        if (runEventsRequestIdRef.current !== requestId) {
          return;
        }
        if (targetSessionIdRef.current !== normalizedSessionID) {
          return;
        }
        const mapped = toStoredRunEventItems(page.events);
        setHistoryCursor(page.next_cursor ?? null);
        setMessages((prev) => {
          const nextMessages = mergeChatMessages(
            prev,
            page.messages ?? [],
            mode,
          );
          if (mode === "replace") {
            const currentMessages = [...prev].sort(
              (a, b) => new Date(a.time).getTime() - new Date(b.time).getTime(),
            );
            const nextSorted = [...nextMessages].sort(
              (a, b) => new Date(a.time).getTime() - new Date(b.time).getTime(),
            );
            const isSameLength = currentMessages.length === nextSorted.length;
            const isSame =
              isSameLength &&
              currentMessages.every(
                (message, index) =>
                  messageIdentityKey(message) ===
                  messageIdentityKey(nextSorted[index]!),
              );
            if (isSame) {
              return prev;
            }
          }
          return nextMessages;
        });
        setRunEvents((prev) => {
          const next = mergeRunEventPages(
            prev,
            mapped,
            normalizedSessionID,
            mode,
          );
          if (mode === "replace") {
            const currentSessionEvents = prev.filter(
              (event) => event.sessionId === normalizedSessionID,
            );
            if (areRunEventListsEqual(currentSessionEvents, mapped)) {
              return prev;
            }
          }
          return next;
        });
        upsertChatSessionSummary({
          id: page.session_id,
          updatedAt: page.updated_at,
          preview: getLatestMessagePreview(page.messages),
        });
      } catch {
        // 历史事件加载失败不影响主流程，保留当前 UI 状态。
      }
    },
    [apiClient, upsertChatSessionSummary],
  );

  const refreshSessionState = useCallback(
    async (targetProjectId: string, targetSessionId: string) => {
      const normalizedSessionID = targetSessionId.trim();
      if (!normalizedSessionID) {
        return;
      }

      const [commands, configOptions] = await Promise.all([
        apiClient
          .getSessionCommands(targetProjectId, normalizedSessionID)
          .catch(() => []),
        apiClient
          .getSessionConfigOptions(targetProjectId, normalizedSessionID)
          .catch(() => []),
      ]);

      if (targetSessionIdRef.current !== normalizedSessionID) {
        return;
      }
      setCommands(normalizedSessionID, commands);
      setConfigOptions(normalizedSessionID, configOptions);
    },
    [apiClient, setCommands, setConfigOptions],
  );

  const loadToolCallGroup = useCallback(
    async (
      targetProjectId: string,
      targetSessionId: string,
      groupId: string,
    ) => {
      const normalizedGroupId = groupId.trim();
      if (!normalizedGroupId) {
        return;
      }
      setToolCallGroupStates((prev) => {
        const existing = prev[normalizedGroupId];
        if (existing?.loading || existing?.items.length) {
          return prev;
        }
        return {
          ...prev,
          [normalizedGroupId]: {
            loading: true,
            items: existing?.items ?? [],
          },
        };
      });
      try {
        const response = await apiClient.getChatEventGroup(
          targetProjectId,
          targetSessionId,
          normalizedGroupId,
        );
        const items = toStoredRunEventItems(response.events);
        setToolCallGroupStates((prev) => ({
          ...prev,
          [normalizedGroupId]: {
            loading: false,
            items,
          },
        }));
      } catch (groupError) {
        setToolCallGroupStates((prev) => ({
          ...prev,
          [normalizedGroupId]: {
            loading: false,
            error: getErrorMessage(groupError),
            items: prev[normalizedGroupId]?.items ?? [],
          },
        }));
      }
    },
    [apiClient],
  );

  const refreshIssueList = useCallback(
    async (targetProjectId: string) => {
      setIssueListLoading(true);
      try {
        const response = await apiClient.listIssues(targetProjectId, {
          limit: 50,
        });
        setIssueList(response.items ?? []);
      } catch {
        setIssueList([]);
      } finally {
        setIssueListLoading(false);
      }
    },
    [apiClient],
  );

  const refreshIssueCheckpoints = useCallback(
    async (runId: string) => {
      if (!runId) {
        setIssueCheckpoints([]);
        return;
      }
      setCheckpointsLoading(true);
      try {
        const response = await apiClient.getRunCheckpoints(runId);
        setIssueCheckpoints(response.items ?? []);
      } catch {
        setIssueCheckpoints([]);
      } finally {
        setCheckpointsLoading(false);
      }
    },
    [apiClient],
  );

  useEffect(() => {
    void refreshIssueList(projectId);
  }, [projectId, refreshIssueList]);

  useEffect(() => {
    if (!sessionId) {
      return;
    }
    void refreshSessionState(projectId, sessionId);
  }, [projectId, refreshSessionState, sessionId]);

  useEffect(() => {
    const issue = issueList.find((i) => i.id === selectedIssueId);
    if (issue?.run_id) {
      void refreshIssueCheckpoints(issue.run_id);
    } else {
      setIssueCheckpoints([]);
    }
  }, [selectedIssueId, issueList, refreshIssueCheckpoints]);

  const loadOlderHistory = useCallback(async () => {
    if (!sessionId || !historyCursor || historyLoadingMore) {
      return;
    }
    const container = timelineScrollRef.current;
    if (container) {
      pendingScrollRestoreRef.current = {
        scrollTop: container.scrollTop,
        scrollHeight: container.scrollHeight,
      };
    }
    setHistoryLoadingMore(true);
    try {
      await refreshChatRunEvents(projectId, sessionId, {
        cursor: historyCursor,
        mode: "prepend",
      });
    } finally {
      setHistoryLoadingMore(false);
    }
  }, [
    historyCursor,
    historyLoadingMore,
    projectId,
    refreshChatRunEvents,
    sessionId,
  ]);

  const handleTimelineScroll = useCallback(() => {
    const container = timelineScrollRef.current;
    if (!container || historyLoadingMore || !historyCursor) {
      return;
    }
    if (container.scrollTop <= 80) {
      void loadOlderHistory();
    }
  }, [historyCursor, historyLoadingMore, loadOlderHistory]);

  const handleSelectCommand = useCallback((command: AvailableCommand) => {
    const hint = command.input?.hint?.trim();
    const nextDraft = hint
      ? `/${command.name} [${hint}]`
      : `/${command.name} `;
    setDraft(nextDraft);
    setSelectedCommandIndex(0);
    requestAnimationFrame(() => {
      const input = messageInputRef.current;
      if (!input) {
        return;
      }
      input.focus();
      if (hint) {
        const start = nextDraft.indexOf("[") + 1;
        const end = nextDraft.lastIndexOf("]");
        input.setSelectionRange(start, end);
        return;
      }
      input.setSelectionRange(nextDraft.length, nextDraft.length);
    });
  }, []);

  const handleDraftKeyDown = useCallback(
    (event: KeyboardEvent<HTMLTextAreaElement>) => {
      if (!showCommandPalette) {
        return;
      }
      if (event.key === "ArrowDown") {
        event.preventDefault();
        setSelectedCommandIndex((prev) =>
          Math.min(prev + 1, filteredCommands.length - 1),
        );
        return;
      }
      if (event.key === "ArrowUp") {
        event.preventDefault();
        setSelectedCommandIndex((prev) => Math.max(prev - 1, 0));
        return;
      }
      if (event.key === "Enter") {
        event.preventDefault();
        const target = filteredCommands[selectedCommandIndex];
        if (target) {
          handleSelectCommand(target);
        }
        return;
      }
      if (event.key === "Escape") {
        event.preventDefault();
        setDraft("");
      }
    },
    [
      filteredCommands,
      handleSelectCommand,
      selectedCommandIndex,
      showCommandPalette,
    ],
  );

  const handleConfigOptionChange = useCallback(
    async (configId: string, value: string) => {
      const targetSessionId = sessionId?.trim();
      if (!targetSessionId) {
        return;
      }
      setUpdatingConfigId(configId);
      setError(null);
      try {
        const nextOptions = await apiClient.setSessionConfigOption(
          projectId,
          targetSessionId,
          configId,
          value,
        );
        if (targetSessionIdRef.current !== targetSessionId) {
          return;
        }
        setConfigOptions(targetSessionId, nextOptions);
      } catch (requestError) {
        if (targetSessionIdRef.current !== targetSessionId) {
          return;
        }
        setError(getErrorMessage(requestError));
      } finally {
        if (targetSessionIdRef.current === targetSessionId) {
          setUpdatingConfigId((current) =>
            current === configId ? null : current,
          );
        }
      }
    },
    [apiClient, projectId, sessionId, setConfigOptions],
  );

  const handleStartChat = async () => {
    if (chatLoading) {
      return;
    }
    const message = draft.trim();
    if (!message) {
      return;
    }

    setChatLoading(true);
    setChatCancelling(false);
    setIsStreaming(false);
    setStreamingText("");
    activeRunStartedAtRef.current = 0;
    localRunStartPendingRef.current = true;
    setError(null);
    setIssueNotice(null);
    setCrossTerminalRunNotice(null);
    const requestId = chatRequestIdRef.current + 1;
    chatRequestIdRef.current = requestId;
    const targetProjectId = projectId;
    const currentSessionId = targetSessionIdRef.current;

    setMessages((prev) => [
      ...prev,
      {
        role: "user",
        content: message,
        time: nowIso(),
      },
    ]);
    setDraft("");

    try {
      const payload = currentSessionId
        ? { message, session_id: currentSessionId }
        : { message, agent_name: selectedAgent };
      const created = await apiClient.createChat(targetProjectId, payload);
      if (chatRequestIdRef.current !== requestId) {
        return;
      }
      targetSessionIdRef.current = created.session_id;
      setSessionId(created.session_id);
      upsertChatSessionSummary({
        id: created.session_id,
        updatedAt: nowIso(),
        preview: message,
      });
    } catch (requestError) {
      if (chatRequestIdRef.current !== requestId) {
        return;
      }
      localRunStartPendingRef.current = false;
      setChatLoading(false);
      setChatCancelling(false);
      setIsStreaming(false);
      setStreamingText("");
      if (requestError instanceof ApiError && requestError.status === 409) {
        setError(CHAT_SESSION_BUSY_NOTICE);
        if (currentSessionId) {
          const normalizedSessionID = currentSessionId.trim();
          if (normalizedSessionID) {
            void refreshChatRunEvents(targetProjectId, normalizedSessionID, {
              mode: "replace",
            });
            void refreshChatSessions(targetProjectId);
          }
        }
        return;
      }
      setError(getErrorMessage(requestError));
    }
  };

  const handleCancelChat = async () => {
    if (!sessionId || !chatLoading || chatCancelling) {
      return;
    }
    const targetProjectId = projectId;
    const targetSessionId = sessionId;
    setChatCancelling(true);
    setError(null);
    try {
      await apiClient.cancelChat(targetProjectId, targetSessionId);
    } catch (requestError) {
      if (targetSessionIdRef.current !== targetSessionId) {
        return;
      }
      setChatCancelling(false);
      setError(getErrorMessage(requestError));
    }
  };

  const handleCreateIssueFromFiles = async () => {
    if (!sessionId || filePaths.length === 0) {
      return;
    }

    setIssueFromFilesLoading(true);
    setError(null);
    setIssueNotice(null);
    const requestId = issueFromFilesRequestIdRef.current + 1;
    issueFromFilesRequestIdRef.current = requestId;
    const targetProjectId = projectId;
    const targetSessionId = sessionId;
    try {
      const createdIssue = await apiClient.createIssueFromFiles(
        targetProjectId,
        {
          session_id: targetSessionId,
          file_paths: filePaths,
        },
      );
      if (issueFromFilesRequestIdRef.current !== requestId) {
        return;
      }
      setIssueNotice(`已从文件创建 issue：${createdIssue.id}`);
    } catch (requestError) {
      if (issueFromFilesRequestIdRef.current !== requestId) {
        return;
      }
      setError(getErrorMessage(requestError));
    } finally {
      if (issueFromFilesRequestIdRef.current === requestId) {
        setIssueFromFilesLoading(false);
      }
    }
  };

  const handleToggleFile = (filePath: string, selected: boolean) => {
    const normalizedPath = filePath.trim();
    if (!normalizedPath) {
      return;
    }

    setSelectedFiles((prev) => {
      const exists = prev.includes(normalizedPath);
      let next = prev;
      if (selected && !exists) {
        next = [...prev, normalizedPath];
      }
      if (!selected && exists) {
        next = prev.filter((item) => item !== normalizedPath);
      }
      return next;
    });
  };

  const handleSwitchSession = useCallback(async (nextSessionId: string) => {
    const normalizedSessionID = nextSessionId.trim();
    if (
      !normalizedSessionID ||
      normalizedSessionID === targetSessionIdRef.current ||
      chatLoading
    ) {
      return;
    }
    targetSessionIdRef.current = normalizedSessionID;
    setSessionId(normalizedSessionID);
    setMessages([]);
    setStreamingText("");
    setIsStreaming(false);
    activeRunStartedAtRef.current = 0;
    setChatLoading(false);
    setChatCancelling(false);
    setIssueNotice(null);
    setCrossTerminalRunNotice(null);
    setError(null);
    setRunEvents([]);
    setHistoryCursor(null);
    setToolCallGroupStates({});
    localRunStartPendingRef.current = false;
    await refreshChatRunEvents(projectId, normalizedSessionID, {
      mode: "replace",
    });
  }, [chatLoading, projectId, refreshChatRunEvents]);

  const handleConnectStageSession = useCallback(
    async (runId: string, stageName: string) => {
      setWakingStage(stageName);
      setError(null);
      try {
        const status = await apiClient.getStageSessionStatus(runId, stageName);
        let targetSessionId = status.session_id;
        if (!status.alive) {
          const wakeResult = await apiClient.wakeStageSession(runId, stageName);
          targetSessionId = wakeResult.session_id;
        }
        if (targetSessionId) {
          await handleSwitchSession(targetSessionId);
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : "唤醒 session 失败");
      } finally {
        setWakingStage(null);
      }
    },
    [apiClient, handleSwitchSession],
  );

  const targetSessionIdRef = useRef<string | null>(sessionId);
  const subscribedSessionIdRef = useRef<string | null>(null);
  useEffect(() => {
    targetSessionIdRef.current = sessionId;
  }, [sessionId]);

  const sendChatSessionSubscription = useCallback(
    (
      type: "subscribe_chat_session" | "unsubscribe_chat_session",
      rawSessionId: string | null,
    ) => {
      const normalizedSessionID = (rawSessionId ?? "").trim();
      if (!normalizedSessionID) {
        return;
      }
      try {
        wsClient.send({
          type,
          session_id: normalizedSessionID,
        });
      } catch {
        // 连接未就绪时忽略，重连后会由 onStatusChange 触发补订阅。
      }
    },
    [wsClient],
  );

  useEffect(() => {
    const nextSessionID = (sessionId ?? "").trim();
    const previousSessionID = subscribedSessionIdRef.current;

    if (previousSessionID && previousSessionID !== nextSessionID) {
      sendChatSessionSubscription(
        "unsubscribe_chat_session",
        previousSessionID,
      );
    }
    if (nextSessionID && previousSessionID !== nextSessionID) {
      sendChatSessionSubscription("subscribe_chat_session", nextSessionID);
    }

    subscribedSessionIdRef.current = nextSessionID || null;
  }, [sessionId, sendChatSessionSubscription]);

  useEffect(() => {
    const unsubscribeStatus = wsClient.onStatusChange((status) => {
      if (status !== "open") {
        return;
      }
      const activeSessionID = targetSessionIdRef.current;
      if (!activeSessionID) {
        return;
      }
      sendChatSessionSubscription("subscribe_chat_session", activeSessionID);
      subscribedSessionIdRef.current = activeSessionID;
    });
    return () => {
      unsubscribeStatus();
    };
  }, [sendChatSessionSubscription, wsClient]);

  useEffect(() => {
    return () => {
      const activeSessionID = subscribedSessionIdRef.current;
      if (!activeSessionID) {
        return;
      }
      sendChatSessionSubscription("unsubscribe_chat_session", activeSessionID);
      subscribedSessionIdRef.current = null;
    };
  }, [sendChatSessionSubscription]);

  useEffect(() => {
    void refreshChatSessions(projectId);
  }, [projectId, refreshChatSessions]);

  useEffect(() => {
    const unsubscribe = wsClient.subscribe<WsEnvelope>("*", (payload) => {
      const envelope = payload as WsEnvelope<ChatEventPayload>;
      if (!CHAT_RUN_EVENT_TYPES.has(envelope.type as ChatEventType)) {
        return;
      }
      if (
        envelope.project_id &&
        envelope.project_id.trim().length > 0 &&
        envelope.project_id !== projectId
      ) {
        return;
      }

      const data = (envelope.data ??
        envelope.payload ??
        {}) as ChatEventPayload;
      const wsSessionID = toStringValue(data.session_id);
      if (!wsSessionID) {
        return;
      }
      const activeSessionID = targetSessionIdRef.current;
      if (!activeSessionID || activeSessionID !== wsSessionID) {
        return;
      }

      switch (envelope.type as ChatEventType) {
        case "run_started": {
          const startedByCurrentTerminal = localRunStartPendingRef.current;
          localRunStartPendingRef.current = false;
          activeRunStartedAtRef.current =
            toEventTimestampMs(data.timestamp) || Date.now();
          setChatLoading(true);
          setChatCancelling(false);
          setIsStreaming(true);
          setStreamingText("");
          setError(null);
          setCrossTerminalRunNotice(
            startedByCurrentTerminal ? null : CROSS_TERMINAL_RUN_NOTICE,
          );
          pushRunEvent(wsSessionID, "run_started", "运行已开始");
          break;
        }
        case "run_update":
        case "team_leader_thinking":
        case "team_leader_files_changed": {
          const acpUpdateType = toStringValue(data.acp?.sessionUpdate);
          const isSessionStateUpdate =
            acpUpdateType === "available_commands_update" ||
            acpUpdateType === "config_option_update";
          if (acpUpdateType === "available_commands_update") {
            setCommands(
              wsSessionID,
              normalizeAvailableCommands(data.acp?.availableCommands),
            );
          }
          if (acpUpdateType === "config_option_update") {
            setConfigOptions(
              wsSessionID,
              normalizeConfigOptions(data.acp?.configOptions),
            );
          }
          if (isSessionStateUpdate) {
            break;
          }
          const eventTimestampMs = toEventTimestampMs(data.timestamp);
          const runStartedAt = activeRunStartedAtRef.current;
          if (runStartedAt === 0) {
            break;
          }
          if (eventTimestampMs > 0 && eventTimestampMs < runStartedAt) {
            break;
          }
          const delta = getStreamingDelta(data);
          const thoughtDelta = getAgentThoughtDelta(data);
          if (delta.length > 0) {
            setStreamingText((prev) => `${prev}${delta}`);
          } else if (thoughtDelta.length > 0) {
            pushRunEvent(wsSessionID, "agent_thought", thoughtDelta, {
              time: toStringValue(data.timestamp) || nowIso(),
            });
          } else {
            const normalizedType = normalizeRunEventType(envelope.type, data);
            pushRunEvent(
              wsSessionID,
              normalizedType,
              buildRunEventDetail(data),
              {
                time: toStringValue(data.timestamp) || nowIso(),
                groupKey: resolveRunEventGroupKey(normalizedType, data),
              },
            );
          }
          break;
        }
        case "run_completed": {
          localRunStartPendingRef.current = false;
          activeRunStartedAtRef.current = 0;
          setChatLoading(false);
          setChatCancelling(false);
          setIsStreaming(false);
          setStreamingText("");
          setCrossTerminalRunNotice(null);
          pushRunEvent(wsSessionID, "run_completed", "运行完成");
          void refreshChatRunEvents(projectId, wsSessionID, {
            mode: "replace",
          });
          void refreshChatSessions(projectId);
          break;
        }
        case "run_cancelled": {
          localRunStartPendingRef.current = false;
          activeRunStartedAtRef.current = 0;
          setChatLoading(false);
          setChatCancelling(false);
          setIsStreaming(false);
          setStreamingText("");
          setCrossTerminalRunNotice(null);
          setError("当前请求已取消");
          pushRunEvent(wsSessionID, "run_cancelled", "运行已取消");
          void refreshChatRunEvents(projectId, wsSessionID, {
            mode: "replace",
          });
          void refreshChatSessions(projectId);
          break;
        }
        case "run_failed": {
          localRunStartPendingRef.current = false;
          activeRunStartedAtRef.current = 0;
          setChatLoading(false);
          setChatCancelling(false);
          setIsStreaming(false);
          setStreamingText("");
          setCrossTerminalRunNotice(null);
          const reason = toStringValue(data.error);
          setError(reason || "chat 执行失败");
          pushRunEvent(wsSessionID, "run_failed", reason || "chat 执行失败");
          void refreshChatRunEvents(projectId, wsSessionID, {
            mode: "replace",
          });
          void refreshChatSessions(projectId);
          break;
        }
        default:
          break;
      }
    });

    return () => {
      unsubscribe();
    };
  }, [
    projectId,
    pushRunEvent,
    refreshChatRunEvents,
    refreshChatSessions,
    setCommands,
    setConfigOptions,
    wsClient,
  ]);

  // Restore scroll position after prepend — update ref so loading-indicator
  // height change can also be corrected, but don't clear yet.
  useLayoutEffect(() => {
    const container = timelineScrollRef.current;
    const pendingRestore = pendingScrollRestoreRef.current;
    if (!container || !pendingRestore) {
      return;
    }
    const heightDelta = container.scrollHeight - pendingRestore.scrollHeight;
    container.scrollTop = pendingRestore.scrollTop + Math.max(heightDelta, 0);
    pendingScrollRestoreRef.current = {
      scrollTop: container.scrollTop,
      scrollHeight: container.scrollHeight,
    };
  }, [timelineItems]);

  // When loading indicator disappears its height changes — correct and clear.
  useLayoutEffect(() => {
    const container = timelineScrollRef.current;
    const pendingRestore = pendingScrollRestoreRef.current;
    if (!container || !pendingRestore || historyLoadingMore) {
      return;
    }
    const heightDelta = container.scrollHeight - pendingRestore.scrollHeight;
    container.scrollTop = pendingRestore.scrollTop + Math.max(heightDelta, 0);
    pendingScrollRestoreRef.current = null;
  }, [historyLoadingMore]);

  useEffect(() => {
    // Skip auto-scroll while a history restore is pending (cleared by layout effect above).
    if (pendingScrollRestoreRef.current) {
      return;
    }
    const endNode = messagesEndRef.current;
    if (!endNode || typeof endNode.scrollIntoView !== "function") {
      return;
    }
    endNode.scrollIntoView({
      block: "end",
    });
  }, [timelineItems, streamingText]);

  const submitButtonLabel = chatLoading
    ? syncingFromOtherTerminal
      ? "同步中"
      : "停止"
    : sessionId
      ? "发送"
      : "发送并创建会话";
  return (
    <section
      className={`grid h-[calc(100vh-4rem)] gap-4 font-mono ${leftPanelOpen ? "lg:grid-cols-[280px_minmax(0,2fr)_320px]" : "lg:grid-cols-[minmax(0,2fr)_320px]"}`}
    >
      {leftPanelOpen && (
        <aside className="hidden overflow-hidden rounded-xl border border-slate-200 bg-white p-4 lg:flex lg:flex-col">
          <div className="flex items-center justify-between">
            <h3 className="text-base font-semibold text-slate-900">仓库视图</h3>
            <button
              type="button"
              className="rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-600"
              onClick={() => {
                setLeftPanelOpen(false);
              }}
              title="收起面板"
            >
              <svg
                xmlns="http://www.w3.org/2000/svg"
                className="h-4 w-4"
                viewBox="0 0 20 20"
                fill="currentColor"
              >
                <path
                  fillRule="evenodd"
                  d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"
                  clipRule="evenodd"
                />
              </svg>
            </button>
          </div>
          <div className="mt-3 grid grid-cols-2 rounded-md bg-slate-100 p-1 text-xs">
            <button
              type="button"
              className={`rounded px-2 py-1 font-medium ${
                leftPanelTab === "tree"
                  ? "bg-white text-slate-900 shadow-sm"
                  : "text-slate-600 hover:text-slate-900"
              }`}
              onClick={() => {
                setLeftPanelTab("tree");
              }}
            >
              文件树
            </button>
            <button
              type="button"
              className={`rounded px-2 py-1 font-medium ${
                leftPanelTab === "git"
                  ? "bg-white text-slate-900 shadow-sm"
                  : "text-slate-600 hover:text-slate-900"
              }`}
              onClick={() => {
                setLeftPanelTab("git");
              }}
            >
              Git Status
            </button>
          </div>
          <div className="mt-3 min-h-0 flex-1 overflow-y-auto">
            {leftPanelTab === "tree" ? (
              <FileTree
                apiClient={apiClient}
                projectId={projectId}
                selectedFiles={selectedFiles}
                onToggleFile={handleToggleFile}
              />
            ) : (
              <GitStatusPanel apiClient={apiClient} projectId={projectId} />
            )}
          </div>
          {leftPanelTab === "tree" && (
            <div className="mt-3 border-t border-slate-200 pt-3">
              {selectedFiles.length > 0 && (
                <p className="mb-2 text-xs text-slate-500">
                  已选 {selectedFiles.length} 个文件
                </p>
              )}
              <button
                type="button"
                className="accent-border accent-text w-full rounded-md border px-3 py-2 text-sm font-semibold disabled:cursor-not-allowed disabled:border-slate-300 disabled:text-slate-400"
                disabled={!canCreateIssueFromFiles}
                onClick={() => { void handleCreateIssueFromFiles(); }}
              >
                {issueFromFilesLoading ? "创建中..." : "从选中文件创建 issue"}
              </button>
              {issueNotice ? (
                <p className="mt-2 rounded-md border border-emerald-200 bg-emerald-50 px-2 py-1.5 text-xs text-emerald-700">
                  {issueNotice}
                </p>
              ) : null}
            </div>
          )}
        </aside>
      )}

      <div className="flex min-w-0 flex-col overflow-hidden rounded-xl border border-slate-200 bg-white p-4">
        <div className="flex items-center gap-3">
          {!leftPanelOpen && (
            <button
              type="button"
              className="hidden rounded-md border border-slate-300 px-2 py-1.5 text-xs text-slate-600 hover:bg-slate-50 lg:inline-flex lg:items-center lg:gap-1"
              onClick={() => {
                setLeftPanelOpen(true);
              }}
              title="展开仓库视图"
            >
              <svg
                xmlns="http://www.w3.org/2000/svg"
                className="h-4 w-4"
                viewBox="0 0 20 20"
                fill="currentColor"
              >
                <path d="M2 4.75A.75.75 0 012.75 4h14.5a.75.75 0 010 1.5H2.75A.75.75 0 012 4.75zM2 10a.75.75 0 01.75-.75h14.5a.75.75 0 010 1.5H2.75A.75.75 0 012 10zm0 5.25a.75.75 0 01.75-.75h14.5a.75.75 0 010 1.5H2.75a.75.75 0 01-.75-.75z" />
              </svg>
              文件
            </button>
          )}
          <div>
            <h2 className="text-xl font-bold">Chat</h2>
            <p className="mt-1 text-sm text-slate-600">
              发送消息先通过 ACK 建立/续用会话，再通过 WS
              流式接收状态与增量内容。
            </p>
          </div>
        </div>
        <ConfigSelector
          options={sessionConfigOptions}
          updatingConfigId={updatingConfigId}
          onChange={(configId, value) => {
            void handleConfigOptionChange(configId, value);
          }}
        />

        {hasMessages ? (
          <div className="mt-4 flex min-h-0 flex-1">
            <div
              ref={timelineScrollRef}
              className="flex-1 overflow-y-auto font-mono text-base"
              onScroll={handleTimelineScroll}
            >
              {historyLoadingMore ? (
                <p className="bg-slate-50 px-4 py-2 text-center text-xs text-slate-500">
                  加载更早记录中...
                </p>
              ) : historyCursor ? (
                <p className="px-4 py-1 text-center text-xs text-slate-400">
                  向上滚动可加载更早记录
                </p>
              ) : null}

              {timelineItems.map((item) => {
                if (item.kind === "message") {
                  return (
                    <TuiMessage
                      key={item.id}
                      id={item.id}
                      role={item.role}
                      content={item.content}
                      time={item.time}
                    />
                  );
                }

                const groupState = item.groupId
                  ? toolCallGroupStates[item.groupId]
                  : undefined;

                return (
                  <TuiActivityBlock
                    key={item.id}
                    activityType={item.activityType}
                    detail={item.detail}
                    time={item.time}
                    groupId={item.groupId}
                    onExpandGroup={(gid) => {
                      if (sessionId) {
                        void loadToolCallGroup(projectId, sessionId, gid);
                      }
                    }}
                    groupChildren={groupState?.items.map((child) => ({
                      id: child.id,
                      type: child.type,
                      detail: child.detail,
                      time: child.time,
                    }))}
                    groupLoading={groupState?.loading}
                    groupError={groupState?.error}
                  />
                );
              })}

              {isStreaming ? (
                <div className="border-b border-slate-200 px-4 py-3">
                  <div className="flex items-start gap-2">
                    <span className="mt-0.5 select-none text-sm font-bold text-slate-400" aria-hidden>•</span>
                    <div className="min-w-0 flex-1 text-sm">
                      <span className="text-xs text-slate-400">输入中...</span>
                      <div className="mt-1 space-y-2">
                        <TuiMarkdown content={streamingText.length > 0 ? streamingText : "..."} />
                      </div>
                    </div>
                  </div>
                </div>
              ) : null}

              <div ref={messagesEndRef} />
            </div>

            <ScrollNavBar markers={userMessageMarkers} onMarkerClick={handleNavMarkerClick} />
          </div>
        ) : (
          <div className="mt-4 flex min-h-0 flex-1 items-center justify-center">
            <p className="text-xs text-slate-400">当前会话暂无消息。</p>
          </div>
        )}

        <div className="mt-4">
          <label
            htmlFor="chat-message"
            className="mb-2 block text-sm font-medium"
          >
            新消息
          </label>
          <textarea
            id="chat-message"
            aria-label="新消息"
            ref={messageInputRef}
            rows={3}
            className="min-h-[5rem] w-full resize-y rounded-md border border-slate-300 px-3 py-2 font-mono text-sm focus:border-slate-500 focus:outline-none"
            placeholder="请输入要拆分为 issue 的需求..."
            value={draft}
            onKeyDown={handleDraftKeyDown}
            onChange={(event) => {
              setDraft(event.target.value);
            }}
          />
          {showCommandPalette ? (
            <CommandPalette
              commands={filteredCommands}
              selectedIndex={selectedCommandIndex}
              onHover={setSelectedCommandIndex}
              onSelect={handleSelectCommand}
            />
          ) : null}
          <div className="mt-3 flex items-center justify-between">
            <div className="flex items-center gap-2">
              {!sessionId ? (
                <select
                  className="rounded-md border border-slate-300 px-2 py-1.5 font-mono text-xs focus:border-slate-500 focus:outline-none"
                  value={selectedAgent}
                  onChange={(e) => setSelectedAgent(e.target.value)}
                  disabled={chatLoading}
                >
                  {agents.map((a) => (
                    <option key={a.name} value={a.name}>{a.name}</option>
                  ))}
                </select>
              ) : (
                <span className="px-2 py-1.5 font-mono text-xs text-slate-500">
                  agent: {selectedAgent}
                </span>
              )}
            </div>
            <button
              type="button"
              className="accent-bg w-36 rounded-md px-4 py-2 text-center text-sm font-medium text-white disabled:cursor-not-allowed disabled:opacity-50"
              disabled={!canSubmit}
              onClick={() => {
                if (chatLoading) {
                  if (syncingFromOtherTerminal) {
                    return;
                  }
                  void handleCancelChat();
                  return;
                }
                void handleStartChat();
              }}
            >
              {submitButtonLabel}
            </button>
          </div>
        </div>
      </div>

      <aside className="flex flex-col overflow-hidden rounded-xl border border-slate-200 bg-white p-4">
        <h3 className="text-lg font-semibold">会话与 Team Leader</h3>
        <p className="mt-2 break-all text-xs text-slate-600">
          Session ID: {sessionId ?? "未创建"}
        </p>

        <div className="mt-3">
          <div className="flex items-center justify-between">
            <h4 className="text-sm font-semibold text-slate-800">会话列表</h4>
            <button
              type="button"
              className="rounded border border-slate-300 px-2 py-1 text-xs text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-50"
              onClick={() => {
                void refreshChatSessions(projectId);
              }}
              disabled={chatsLoading}
            >
              {chatsLoading ? "刷新中..." : "刷新"}
            </button>
          </div>
          {chatSessions.length > 0 ? (
            <div className="mt-2 max-h-44 overflow-y-auto rounded-md border border-slate-200">
              {chatSessions.map((session) => {
                const active = sessionId === session.id;
                const status = sessionStatuses[session.id];
                const isRunning = status?.running || (active && chatLoading);
                const isAlive = status?.alive || isRunning;
                return (
                  <button
                    key={session.id}
                    type="button"
                    aria-label={session.id}
                    className={`w-full border-b border-slate-100 px-3 py-2 text-left last:border-b-0 ${
                      active
                        ? "bg-slate-800 text-white"
                        : "text-slate-700 hover:bg-slate-50"
                    }`}
                    onClick={() => {
                      void handleSwitchSession(session.id);
                    }}
                    disabled={chatLoading}
                  >
                    <div className="flex items-center gap-2">
                      <span
                        className={`inline-block h-2.5 w-2.5 flex-shrink-0 rounded-full ${
                          isRunning
                            ? "animate-pulse bg-blue-500"
                            : isAlive
                              ? "bg-emerald-500"
                              : "bg-slate-300"
                        }`}
                        title={
                          isRunning ? "运行中" : isAlive ? "空闲" : "已停止"
                        }
                      />
                      <p className="truncate text-sm font-medium">
                        {session.id}
                      </p>
                    </div>
                    <p
                      className={`mt-0.5 truncate pl-[18px] text-xs ${active ? "text-slate-100" : "text-slate-500"}`}
                    >
                      {session.preview || "暂无消息预览"}
                    </p>
                  </button>
                );
              })}
            </div>
          ) : (
            <p className="mt-2 text-xs text-slate-500">暂无会话</p>
          )}
          {chatsError ? (
            <p className="mt-2 rounded border border-rose-200 bg-rose-50 px-2 py-1 text-xs text-rose-700">
              加载会话失败：{chatsError}
            </p>
          ) : null}
        </div>

        <div className="mt-4">
          <div className="flex items-center justify-between">
            <h4 className="text-sm font-semibold text-slate-800">
              Issue Sessions
            </h4>
            <button
              type="button"
              className="text-xs text-sky-600 hover:text-sky-800"
              onClick={() => void refreshIssueList(projectId)}
              disabled={issueListLoading}
            >
              {issueListLoading ? "..." : "刷新"}
            </button>
          </div>
          <select
            className="mt-2 w-full rounded-md border border-slate-300 bg-white px-2 py-1.5 text-xs"
            value={selectedIssueId ?? ""}
            onChange={(e) => setSelectedIssueId(e.target.value || null)}
          >
            <option value="">选择 Issue...</option>
            {issueList.map((issue) => (
              <option key={issue.id} value={issue.id}>
                {issue.title || issue.id} [{issue.status}]
              </option>
            ))}
          </select>
          {selectedIssueId && (
            <div className="mt-2 max-h-48 overflow-y-auto rounded-md border border-slate-200">
              {checkpointsLoading ? (
                <p className="px-3 py-2 text-xs text-slate-500">加载中...</p>
              ) : issueCheckpoints.length === 0 ? (
                <p className="px-3 py-2 text-xs text-slate-500">
                  暂无 stage 记录
                </p>
              ) : (
                issueCheckpoints.map((cp, idx) => {
                  const noRoleStages = new Set(["setup", "merge", "cleanup"]);
                  const canConnect = !noRoleStages.has(cp.stage_name);
                  const isWaking = wakingStage === cp.stage_name;
                  const selectedIssue = issueList.find(
                    (i) => i.id === selectedIssueId,
                  );
                  const runId = selectedIssue?.run_id ?? "";
                  const statusConfig: Record<
                    string,
                    { label: string; color: string; dot: string }
                  > = {
                    in_progress: {
                      label: "运行中",
                      color: "text-blue-700",
                      dot: "bg-blue-500 animate-pulse",
                    },
                    success: {
                      label: "完成",
                      color: "text-emerald-700",
                      dot: "bg-emerald-500",
                    },
                    failed: {
                      label: "失败",
                      color: "text-rose-700",
                      dot: "bg-rose-500",
                    },
                    skipped: {
                      label: "跳过",
                      color: "text-slate-500",
                      dot: "bg-slate-400",
                    },
                    invalidated: {
                      label: "已废弃",
                      color: "text-amber-600",
                      dot: "bg-amber-400",
                    },
                  };
                  const cfg = statusConfig[cp.status] ?? {
                    label: cp.status,
                    color: "text-slate-600",
                    dot: "bg-slate-400",
                  };
                  const actionLabel = cp.agent_session_id
                    ? cp.status === "in_progress"
                      ? "对话"
                      : "恢复"
                    : canConnect
                      ? "唤醒"
                      : "";
                  const actionBg = cp.agent_session_id
                    ? "bg-sky-100 text-sky-700"
                    : "bg-amber-100 text-amber-700";
                  return (
                    <button
                      key={`${cp.stage_name}-${idx}`}
                      type="button"
                      className={`flex w-full items-center gap-2 border-b border-slate-100 px-3 py-2 text-left last:border-b-0 ${
                        canConnect
                          ? "hover:bg-sky-50 cursor-pointer"
                          : "cursor-default opacity-70"
                      }`}
                      disabled={!canConnect || chatLoading || isWaking}
                      onClick={() => {
                        if (canConnect && runId) {
                          void handleConnectStageSession(runId, cp.stage_name);
                        }
                      }}
                    >
                      <span
                        className={`inline-block h-2.5 w-2.5 flex-shrink-0 rounded-full ${cfg.dot}`}
                      />
                      <span className="flex-1 min-w-0">
                        <span className="block truncate text-sm font-medium text-slate-800">
                          {cp.stage_name}
                        </span>
                        <span className={`block text-xs ${cfg.color}`}>
                          {cfg.label}
                          {cp.agent_used ? ` · ${cp.agent_used}` : ""}
                        </span>
                      </span>
                      {canConnect && actionLabel && (
                        <span
                          className={`flex-shrink-0 rounded-full px-2 py-0.5 text-xs font-medium ${actionBg}`}
                        >
                          {isWaking ? "唤醒中..." : actionLabel}
                        </span>
                      )}
                    </button>
                  );
                })
              )}
            </div>
          )}
        </div>

        {crossTerminalRunNotice ? (
          <p className="mt-3 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800">
            {crossTerminalRunNotice}
          </p>
        ) : null}
        {error ? (
          <p className="mt-3 rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">
            {error}
          </p>
        ) : null}
      </aside>
    </section>
  );
};

export default ChatView;
