import { startTransition, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useNavigate } from "react-router-dom";
import type {
  DriverConfig,
  AgentProfile,
  ConfigOption,
  Event as ApiEvent,
  SessionModeState,
  SlashCommand,
} from "@/types/apiV2";
import type { LLMConfigItem } from "@/types/system";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { getErrorMessage } from "@/lib/v2Workbench";
import type {
  SessionRecord,
  ChatMessageView,
  ChatActivityView,
  RealtimeChatOutputPayload,
  RealtimeChatAckPayload,
  RealtimeChatErrorPayload,
  LeadDriverOption,
  SessionGroup,
  PendingMessageView,
} from "@/components/chat/chatTypes";
import {
  toSummaryRecord,
  toDetailRecord,
  toMessageView,
  toProjectGroupKey,
  normalizeDriverKey,
  driverLabelForId,
  fallbackLabel,
  formatUsagePercent,
  touchSessionList,
  applyActivityPayload,
  buildRealtimeEvent,
  buildActivityHistory,
} from "@/components/chat/chatUtils";
import { ChatSessionSidebar } from "@/components/chat/ChatSessionSidebar";
import { ChatHeader } from "@/components/chat/ChatHeader";
import { ChatMainPanel } from "@/components/chat/ChatMainPanel";
import { ChatInputBar } from "@/components/chat/ChatInputBar";
import { PermissionBar, type PermissionRequest } from "@/components/chat/PermissionBar";
import { useChatFeed } from "@/components/chat/useChatFeed";
import type { RuntimeConfigReloadedPayload } from "@/types/ws";
import { useIsMobile } from "@/hooks/useIsMobile";
import { MessagesSquare, X } from "lucide-react";

const FEED_PAGE_SIZE = 100;
function isSessionAlreadyRunningError(message: string | null | undefined): boolean {
  return (message ?? "").toLowerCase().includes("session is already running");
}

export function ChatPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const location = useLocation();
  const {
    apiClient,
    wsClient,
    projects,
    selectedProjectId,
    setSelectedProjectId,
  } = useWorkbench();
  const isMobile = useIsMobile();
  const [chatSidebarOpen, setChatSidebarOpen] = useState(false);
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
  const [drivers, setDrivers] = useState<DriverConfig[]>([]);
  const [leadProfiles, setLeadProfiles] = useState<AgentProfile[]>([]);
  const [draftProjectId, setDraftProjectId] = useState<number | null>(selectedProjectId);
  const [draftProfileId, setDraftProfileId] = useState("");
  const [draftDriverId, setDraftDriverId] = useState("");
  const [draftLLMConfigId, setDraftLLMConfigId] = useState("system");
  const [llmConfigs, setLLMConfigs] = useState<LLMConfigItem[]>([]);
  const [draftUseWorktree, setDraftUseWorktree] = useState(true);
  const [collapsedGroups, setCollapsedGroups] = useState<Record<string, boolean>>({});
  const [detailView, setDetailView] = useState<"chat" | "events">("chat");
  const [submitting, setSubmitting] = useState(false);
  const [loadingSessions, setLoadingSessions] = useState(false);
  const [initialLoaded, setInitialLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [pendingMessage, setPendingMessage] = useState<PendingMessageView | null>(null);
  const [commandsBySession, setCommandsBySession] = useState<Record<string, SlashCommand[]>>({});
  const [configOptionsBySession, setConfigOptionsBySession] = useState<Record<string, ConfigOption[]>>({});
  const [modesBySession, setModesBySession] = useState<Record<string, SessionModeState | null>>({});
  const [pendingPermissions, setPendingPermissions] = useState<PermissionRequest[]>([]);
  const [showCommandPalette, setShowCommandPalette] = useState(false);
  const [commandFilter, setCommandFilter] = useState("");
  const [collapsedActivityGroups, setCollapsedActivityGroups] = useState<Record<string, boolean>>({});
  const [prLoading, setPrLoading] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const chatContainerRef = useRef<HTMLDivElement>(null);
  const pendingChunkBuffersRef = useRef<Record<string, string>>({});
  const chunkFlushFrameRef = useRef<number | null>(null);
  const pendingRequestIdRef = useRef<string | null>(null);
  const pendingSendDraftRef = useRef<{ messageInput: string; pendingFiles: File[] } | null>(null);
  const activeSessionRef = useRef<string | null>(null);
  const isNearBottomRef = useRef(true);
  const prevActiveSessionRef = useRef<string | null>(null);
  const syntheticEventIdRef = useRef(-1);
  const pendingDraftInfoRef = useRef<{
    projectId?: number;
    projectName?: string;
    profileId: string;
    driverId: string;
    title: string;
  } | null>(null);
  const prevScrollHeightRef = useRef<number>(0);
  const justSwitchedSessionRef = useRef(false);

  // Feed pagination: show last FEED_PAGE_SIZE entries, expand on scroll-to-top
  const [feedVisibleCount, setFeedVisibleCount] = useState(FEED_PAGE_SIZE);
  const [loadingMore, setLoadingMore] = useState(false);

  const syncSessionDetail = useCallback((detail: import("@/types/apiV2").ChatSessionDetail) => {
    const record = toDetailRecord(detail, t);
    const userViews = detail.messages
      .filter((message) => message.role === "user")
      .map((message, index) => toMessageView(detail.session_id, message, index));

    setSessions((current) => {
      const existing = current.filter((item) => item.session_id !== detail.session_id);
      return [record, ...existing].sort((left, right) => (
        new Date(right.created_at).getTime() - new Date(left.created_at).getTime()
      ));
    });
    setMessagesBySession((current) => {
      // Preserve optimistically-inserted messages (e.g. from draft transfer)
      const existing = current[detail.session_id];
      if (existing && existing.length > 0) {
        return current;
      }
      return { ...current, [detail.session_id]: userViews };
    });
    setLoadedSessions((current) => ({
      ...current,
      [detail.session_id]: true,
    }));
    setCommandsBySession((current) => ({ ...current, [detail.session_id]: detail.available_commands ?? [] }));
    setConfigOptionsBySession((current) => ({ ...current, [detail.session_id]: detail.config_options ?? [] }));
    setModesBySession((current) => ({ ...current, [detail.session_id]: detail.modes ?? null }));
  }, [t]);

  const syncSessionEvents = useCallback((sessionId: string, events: ApiEvent[]) => {
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
  }, [t]);

  const loadSessionState = useCallback(async (sessionId: string) => {
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
  }, [apiClient, syncSessionDetail, syncSessionEvents]);

  const loadAgentCatalog = useCallback(async () => {
    try {
      const [profiles, driverList, llmConfigResp] = await Promise.all([
        apiClient.listProfiles(),
        apiClient.listDrivers(),
        apiClient.getLLMConfig(),
      ]);
      setDrivers(driverList);
      setLeadProfiles(profiles);
      setLLMConfigs(llmConfigResp.configs ?? []);
      setDraftProfileId((current) => {
        if (current && profiles.some((profile) => profile.id === current)) {
          return current;
        }
        return profiles[0]?.id ?? "";
      });
      setDraftDriverId((current) => {
        if (current && driverList.some((driver) => driver.id === current)) {
          return current;
        }
        // Auto-select driver from first profile's driver_id, or first driver.
        const firstProfile = profiles[0];
        if (firstProfile?.driver_id && driverList.some((d) => d.id === firstProfile.driver_id)) {
          return firstProfile.driver_id;
        }
        return driverList[0]?.id ?? "";
      });
      // Auto-select LLM config from first profile's llm_config_id
      setDraftLLMConfigId((current) => {
        if (current && current !== "system") {
          return current;
        }
        const firstProfile = profiles[0];
        return firstProfile?.llm_config_id || "system";
      });
    } catch (loadError) {
      setError(getErrorMessage(loadError));
    }
  }, [apiClient]);

  const refreshSessions = useCallback(async (preferredSessionId?: string | null) => {
    setLoadingSessions(true);
    try {
      const list = await apiClient.listChatSessions();
      const next = list.map((s) => toSummaryRecord(s, t));
      setSessions(next.sort((left, right) => (
        new Date(right.created_at).getTime() - new Date(left.created_at).getTime()
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
  }, [apiClient, t]);

  useEffect(() => {
    void refreshSessions();
  }, [refreshSessions]);

  // Pick up pending request from homepage navigation state
  useEffect(() => {
    const state = location.state as {
      pendingRequestId?: string;
      pendingDraftInfo?: {
        projectId?: number;
        projectName?: string;
        profileId: string;
        driverId: string;
        title: string;
      };
      pendingMessage?: string;
    } | null;
    if (state?.pendingRequestId) {
      pendingRequestIdRef.current = state.pendingRequestId;
      setSubmitting(true);
      if (state.pendingDraftInfo) {
        pendingDraftInfoRef.current = state.pendingDraftInfo;
      }
      // Show draft message optimistically
      if (state.pendingMessage) {
        const now = new Date();
        setDraftMessages([{
          id: "draft-home-msg",
          role: "user" as const,
          content: state.pendingMessage,
          time: now.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" }),
          at: now.toISOString(),
        }]);
      }
      // Clear navigation state to prevent re-triggering on remount
      navigate(location.pathname, { replace: true, state: null });
    }
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    activeSessionRef.current = activeSession;
  }, [activeSession]);

  useEffect(() => {
    void loadAgentCatalog();
  }, [loadAgentCatalog]);

  const flushBufferedChunks = useCallback(() => {
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
  }, []);

  const scheduleChunkFlush = useCallback(() => {
    if (chunkFlushFrameRef.current != null) {
      return;
    }
    chunkFlushFrameRef.current = requestAnimationFrame(() => {
      chunkFlushFrameRef.current = null;
      flushBufferedChunks();
    });
  }, [flushBufferedChunks]);

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
  }, [drivers, t]);
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
    [sessionSearch, sessions, t],
  );

  const groupedSessions = useMemo<SessionGroup[]>(() => {
    const groups = new Map<string, SessionGroup>();
    for (const session of filteredSessions) {
      const key = toProjectGroupKey(session.project_id);
      const existing = groups.get(key);
      if (existing) {
        existing.sessions.push(session);
        if (new Date(session.created_at).getTime() > new Date(existing.updatedAt).getTime()) {
          existing.updatedAt = session.created_at;
        }
        continue;
      }
      groups.set(key, {
        key,
        label: fallbackLabel(session.project_name, t("chat.noProject")),
        updatedAt: session.created_at,
        sessions: [session],
      });
    }
    return Array.from(groups.values())
      .map((group) => ({
        ...group,
        sessions: [...group.sessions].sort((left, right) => (
          new Date(right.created_at).getTime() - new Date(left.created_at).getTime()
        )),
      }))
      .sort((left, right) => {
        const timeDiff = new Date(right.updatedAt).getTime() - new Date(left.updatedAt).getTime();
        if (timeDiff !== 0) {
          return timeDiff;
        }
        return left.label.localeCompare(right.label, "zh-CN");
      });
  }, [filteredSessions, t]);

  const currentMessages = useMemo(
    () => (currentSession ? (messagesBySession[currentSession.session_id] ?? []) : draftMessages),
    [currentSession, draftMessages, messagesBySession],
  );
  const currentEvents = useMemo(
    () => (currentSession ? (eventsBySession[currentSession.session_id] ?? []) : []),
    [currentSession, eventsBySession],
  );
  const currentActivities = useMemo(
    () => (currentSession ? (activitiesBySession[currentSession.session_id] ?? []) : []),
    [activitiesBySession, currentSession],
  );
  const availableCommands = useMemo(
    () => (currentSession ? (commandsBySession[currentSession.session_id] ?? []) : []),
    [currentSession, commandsBySession],
  );
  const configOptions = useMemo(
    () => (currentSession ? (configOptionsBySession[currentSession.session_id] ?? []) : []),
    [currentSession, configOptionsBySession],
  );
  const sessionModes = useMemo(
    () => (currentSession ? (modesBySession[currentSession.session_id] ?? null) : null),
    [currentSession, modesBySession],
  );
  const isDraftSessionView = initialLoaded && !currentSession && currentMessages.length === 0;
  const currentUsage = useMemo(
    () =>
      [...currentActivities]
        .reverse()
        .find((activity) => activity.type === "usage_update"),
    [currentActivities],
  );

  const currentUsagePercent = formatUsagePercent(currentUsage?.usageUsed, currentUsage?.usageSize);

  const lastActivityText = useMemo(() => {
    if (!submitting || currentActivities.length === 0) return "";
    const last = [...currentActivities].reverse().find(
      (a) => a.type === "agent_thought" || a.type === "tool_call",
    );
    return last ? (last.detail || last.title) : "";
  }, [submitting, currentActivities]);

  const lastUserMessage = useMemo(() => {
    const last = [...currentMessages].reverse().find((m) => m.role === "user");
    return last?.content.replace(/\s+/g, " ").trim() ?? "";
  }, [currentMessages]);

  const { chatFeedEntries, visibleFeedEntries, hasMoreFeedEntries } = useChatFeed(
    currentMessages, currentActivities, feedVisibleCount,
  );

  const visiblePendingPermissions = useMemo(
    () => pendingPermissions.filter((perm) => perm.session_id === activeSession),
    [pendingPermissions, activeSession],
  );

  const pendingPermissionSessionIds = useMemo(
    () => new Set(pendingPermissions.map((perm) => perm.session_id)),
    [pendingPermissions],
  );

  // Reset feed pagination when switching sessions
  useEffect(() => {
    setFeedVisibleCount(FEED_PAGE_SIZE);
  }, [activeSession]);

  const handleChatScroll = useCallback(
    (e: React.UIEvent<HTMLDivElement>) => {
      const el = e.currentTarget;
      // Track whether user is near the bottom (within 80px)
      isNearBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
      if (el.scrollTop < 80 && hasMoreFeedEntries && !loadingMore) {
        prevScrollHeightRef.current = el.scrollHeight;
        setLoadingMore(true);
        setFeedVisibleCount((prev) => prev + FEED_PAGE_SIZE);
      }
    },
    [hasMoreFeedEntries, loadingMore],
  );

  // After prepending entries, restore scroll position so the view doesn't jump
  useEffect(() => {
    if (!loadingMore) return;
    const el = chatContainerRef.current;
    if (!el || prevScrollHeightRef.current === 0) return;
    // Wait for DOM to repaint with the new content
    requestAnimationFrame(() => {
      el.scrollTop = el.scrollHeight - prevScrollHeightRef.current;
      prevScrollHeightRef.current = 0;
      setLoadingMore(false);
    });
  }, [visibleFeedEntries.length, loadingMore]);

  useEffect(() => {
    const sessionChanged = prevActiveSessionRef.current !== activeSession;
    prevActiveSessionRef.current = activeSession;
    if (sessionChanged) {
      // Switched session — jump instantly and reset scroll tracking
      isNearBottomRef.current = true;
      justSwitchedSessionRef.current = true;
      messagesEndRef.current?.scrollIntoView({ behavior: "auto" });
      return;
    }
    if (!isNearBottomRef.current) return;
    // After switching session, the first data load should also jump instantly (no smooth animation)
    if (justSwitchedSessionRef.current) {
      justSwitchedSessionRef.current = false;
      messagesEndRef.current?.scrollIntoView({ behavior: "auto" });
      return;
    }
    const isStreaming = currentMessages.at(-1)?.id.endsWith("stream-assistant");
    messagesEndRef.current?.scrollIntoView({ behavior: isStreaming ? "auto" : "smooth" });
  }, [activeSession, currentEvents, currentMessages, currentActivities, detailView]);

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
          const finalContent = payload.content.trim();
          const finalTime = now.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" });
          setMessagesBySession((current) => {
            const existing = current[sessionId] ?? [];
            const last = existing.at(-1);
            const finalMsg: ChatMessageView = {
              id: `${sessionId}-assistant-${Date.now()}`,
              role: "assistant",
              content: finalContent,
              time: finalTime,
              at: nowISO,
            };
            if (last && last.id === `${sessionId}-stream-assistant`) {
              return {
                ...current,
                [sessionId]: [...existing.slice(0, -1), finalMsg],
              };
            }
            return {
              ...current,
              [sessionId]: [...existing, finalMsg],
            };
          });
          setSessions((current) => touchSessionList(current, sessionId, "running", nowISO));
        }

        if (updateType === "pending_dispatched") {
          setPendingMessage(null);
          setSubmitting(true);
          setSessions((current) => touchSessionList(current, sessionId, "running", nowISO));
          return;
        }

        if (updateType === "done") {
          flushBufferedChunks();
          setSessions((current) => touchSessionList(current, sessionId, "alive", nowISO));
          setSubmitting(false);
          pendingRequestIdRef.current = null;
          pendingSendDraftRef.current = null;
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

        // Handle queued status — message was queued for later dispatch.
        if (payload.status === "queued") {
          setPendingMessage({
            sessionId,
            content: pendingSendDraftRef.current?.messageInput ?? "",
          });
          setSubmitting(false);
          pendingSendDraftRef.current = null;
          return;
        }

        setSubmitting(false);
        pendingSendDraftRef.current = null;

        // Transfer draft messages to the new session before clearing
        setDraftMessages((draftCurrent) => {
          if (draftCurrent.length > 0) {
            setMessagesBySession((msgCurrent) => ({
              ...msgCurrent,
              [sessionId]: draftCurrent.map((m) => ({
                ...m,
                id: m.id.replace("draft", sessionId),
              })),
            }));
          }
          return [];
        });

        // Optimistic insert — avoid full refreshSessions() to prevent UI jitter
        const draftInfo = pendingDraftInfoRef.current;
        pendingDraftInfoRef.current = null;
        const now = new Date().toISOString();
        setSessions((current) => {
          if (current.some((s) => s.session_id === sessionId)) {
            return current;
          }
          return [{
            session_id: sessionId,
            title: draftInfo?.title || t("chat.newSession"),
            status: "running" as const,
            project_id: draftInfo?.projectId,
            project_name: draftInfo?.projectName,
            profile_id: draftInfo?.profileId,
            driver_id: draftInfo?.driverId,
            created_at: now,
            updated_at: now,
            message_count: 1,
          }, ...current];
        });

        setActiveSession(sessionId);
        setLoadedSessions((current) => ({
          ...current,
          [sessionId]: false,
        }));
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
        const errorMessage = payload.error?.trim() || t("chat.sendFailed");

        // Backend now queues messages for busy sessions. This error should not
        // normally arrive, but as a safety net, silently ignore it.
        if (isSessionAlreadyRunningError(errorMessage)) {
          pendingSendDraftRef.current = null;
          return;
        }

        pendingSendDraftRef.current = null;
        setError(errorMessage);
        const sessionId = payload.session_id?.trim();
        if (sessionId) {
          setSessions((current) => touchSessionList(current, sessionId, "closed", new Date().toISOString()));
        }
      },
    );
    const unsubscribeConfigUpdate = wsClient.subscribe<{ session_id?: string; config_options?: ConfigOption[] }>(
      "chat.config_updated",
      (payload) => {
        const sessionId = payload.session_id?.trim();
        if (sessionId && payload.config_options) {
          setConfigOptionsBySession((current) => ({ ...current, [sessionId]: payload.config_options! }));
        }
      },
    );
    const unsubscribeModeUpdate = wsClient.subscribe<{ session_id?: string; modes?: SessionModeState }>(
      "chat.mode_updated",
      (payload) => {
        const sessionId = payload.session_id?.trim();
        if (sessionId && payload.modes) {
          setModesBySession((current) => ({ ...current, [sessionId]: payload.modes! }));
        }
      },
    );
    const unsubscribePermissionRequest = wsClient.subscribe<PermissionRequest>(
      "chat.permission_request",
      (payload) => {
        if (payload.permission_id) {
          setPendingPermissions((prev) => [...prev, payload]);
        }
      },
    );
    const unsubscribePermissionResolved = wsClient.subscribe<{ permission_id?: string }>(
      "chat.permission_resolved",
      (payload) => {
        if (payload.permission_id) {
          setPendingPermissions((prev) => prev.filter((p) => p.permission_id !== payload.permission_id));
        }
      },
    );
    const unsubscribeRuntimeConfigReloaded = wsClient.subscribe<RuntimeConfigReloadedPayload>(
      "runtime.config_reloaded",
      () => {
        void loadAgentCatalog();
      },
    );
    const unsubscribeSessionTitle = wsClient.subscribe<{ session_id?: string; title?: string }>(
      "chat.session_title",
      (payload) => {
        const sessionId = payload.session_id?.trim();
        const title = payload.title?.trim();
        if (sessionId && title) {
          setSessions((current) =>
            current.map((s) => s.session_id === sessionId ? { ...s, title } : s),
          );
        }
      },
    );
    const unsubscribePendingCancelled = wsClient.subscribe<{ session_id?: string }>(
      "chat.pending_cancelled",
      (payload) => {
        const sessionId = payload.session_id?.trim();
        if (sessionId) {
          setPendingMessage(null);
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
      unsubscribeModeUpdate();
      unsubscribePermissionRequest();
      unsubscribePermissionResolved();
      unsubscribeRuntimeConfigReloaded();
      unsubscribeSessionTitle();
      unsubscribePendingCancelled();
    };
  }, [flushBufferedChunks, loadAgentCatalog, scheduleChunkFlush, t, wsClient]);

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
  }, [activeSession, loadedSessions, loadSessionState]);

  const handleGroupToggle = useCallback((key: string) => {
    setCollapsedGroups((current) => ({ ...current, [key]: !current[key] }));
  }, []);

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
    setDraftUseWorktree(true);
    setActiveSession(null);
    setDraftMessages([]);
    setMessageInput("");
    setError(null);
  };

  const archiveSession = useCallback(async (sessionId: string) => {
    try {
      await apiClient.archiveChatSession(sessionId, true);
      setSessions((current) => current.filter((s) => s.session_id !== sessionId));
      if (activeSession === sessionId) {
        setActiveSession(null);
      }
    } catch {
      // silently ignore — session list will refresh on next poll
    }
  }, [apiClient, activeSession]);

  const appendMessage = (sessionId: string | null, role: "user" | "assistant", content: string, attachments?: { name: string; mime_type: string; data: string }[]) => {
    const now = new Date();
    const nowISO = now.toISOString();
    const view: ChatMessageView = {
      id: `${sessionId ?? "draft"}-${role}-${now.getTime()}`,
      role,
      content,
      time: now.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" }),
      at: nowISO,
      attachments: attachments && attachments.length > 0 ? attachments : undefined,
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

  const compressImage = useCallback((file: File, maxWidth = 1536, quality = 0.8): Promise<File> => {
    return new Promise((resolve) => {
      if (!file.type.startsWith("image/") || file.type === "image/gif") {
        resolve(file);
        return;
      }
      const img = new Image();
      const url = URL.createObjectURL(file);
      img.onload = () => {
        URL.revokeObjectURL(url);
        if (img.width <= maxWidth && file.size <= 512 * 1024) {
          resolve(file);
          return;
        }
        const scale = Math.min(1, maxWidth / img.width);
        const w = Math.round(img.width * scale);
        const h = Math.round(img.height * scale);
        const canvas = document.createElement("canvas");
        canvas.width = w;
        canvas.height = h;
        const ctx = canvas.getContext("2d")!;
        ctx.drawImage(img, 0, 0, w, h);
        canvas.toBlob(
          (blob) => {
            if (blob && blob.size < file.size) {
              resolve(new File([blob], file.name, { type: "image/jpeg", lastModified: Date.now() }));
            } else {
              resolve(file);
            }
          },
          "image/jpeg",
          quality,
        );
      };
      img.onerror = () => {
        URL.revokeObjectURL(url);
        resolve(file);
      };
      img.src = url;
    });
  }, []);

  const compressFiles = useCallback(async (files: File[]): Promise<File[]> => {
    return Promise.all(files.map((f) => compressImage(f)));
  }, [compressImage]);

  const handlePaste = async (e: React.ClipboardEvent) => {
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
      const compressed = await compressFiles(newFiles);
      setPendingFiles((prev) => [...prev, ...compressed]);
    }
  };

  const handleFileSelect = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = e.target.files;
    if (files && files.length > 0) {
      const compressed = await compressFiles(Array.from(files));
      setPendingFiles((prev) => [...prev, ...compressed]);
    }
    e.target.value = "";
  };

  const removePendingFile = (index: number) => {
    setPendingFiles((prev) => prev.filter((_, i) => i !== index));
  };

  const sendMessage = async () => {
    const content = messageInput.trim();
    if (!content && pendingFiles.length === 0) {
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

    const attachments: { name: string; mime_type: string; data: string }[] = [];
    for (const file of pendingFiles) {
      const buf = await file.arrayBuffer();
      const bytes = new Uint8Array(buf);
      let binary = "";
      const chunkSize = 8192;
      for (let i = 0; i < bytes.length; i += chunkSize) {
        binary += String.fromCharCode(...bytes.subarray(i, i + chunkSize));
      }
      const b64 = btoa(binary);
      attachments.push({ name: file.name, mime_type: file.type || "application/octet-stream", data: b64 });
    }

    const displayContent = content + (attachments.length > 0
      ? `\n${t("chat.attachmentLabel", { names: attachments.map((a) => a.name).join(", ") })}`
      : "");
    appendMessage(workingSessionId, "user", displayContent, attachments);
    pendingSendDraftRef.current = {
      messageInput,
      pendingFiles,
    };
    setMessageInput("");
    setPendingFiles([]);
    setSubmitting(true);
    setError(null);

    try {
      const requestId = `chat-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
      pendingRequestIdRef.current = requestId;
      if (!workingSessionId) {
        pendingDraftInfoRef.current = {
          projectId: typeof resolvedProjectId === "number" ? resolvedProjectId : undefined,
          projectName: resolvedProjectName,
          profileId: resolvedProfileId,
          driverId: resolvedDriverId,
          title: content.slice(0, 24) || t("chat.newSession"),
        };
      }
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
          ...(!workingSessionId ? {
            use_worktree: draftUseWorktree,
            llm_config_id: draftLLMConfigId || undefined,
          } : {}),
        },
      });
    } catch (sendError) {
      pendingRequestIdRef.current = null;
      const errorMessage = getErrorMessage(sendError);
      if (isSessionAlreadyRunningError(errorMessage)) {
        pendingSendDraftRef.current = null;
        return;
      }
      pendingSendDraftRef.current = null;
      setError(errorMessage);
      if (workingSessionId) {
        setSessions((current) => touchSessionList(current, workingSessionId, "closed", new Date().toISOString()));
      }
    } finally {
      if (!pendingRequestIdRef.current) {
        setSubmitting(false);
      }
    }
  };

  const sessionRunning = currentSession?.status === "running";

  const cancelPendingMessage = () => {
    if (!pendingMessage) return;
    wsClient.send({
      type: "chat.cancel_pending",
      data: { session_id: pendingMessage.sessionId },
    });
    setPendingMessage(null); // optimistic
  };

  const cancelSession = async () => {
    if (!currentSession) {
      return;
    }
    try {
      await apiClient.cancelChat(currentSession.session_id);
      setSessions((current) =>
        current.map((session) =>
          session.session_id === currentSession.session_id
            ? { ...session, status: "alive" }
            : session,
        ),
      );
      setSubmitting(false);
    } catch (cancelError) {
      setError(getErrorMessage(cancelError));
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

  const renameSession = async (title: string) => {
    if (!currentSession) return;
    try {
      await apiClient.renameChatSession(currentSession.session_id, title);
      setSessions((current) =>
        current.map((s) =>
          s.session_id === currentSession.session_id ? { ...s, title } : s,
        ),
      );
    } catch (renameError) {
      setError(getErrorMessage(renameError));
    }
  };

  const createPR = async () => {
    if (!currentSession) return;
    setPrLoading(true);
    try {
      const stats = await apiClient.createChatPR(currentSession.session_id, currentSession.title);
      setSessions((current) =>
        current.map((s) =>
          s.session_id === currentSession.session_id ? { ...s, git: stats } : s,
        ),
      );
    } catch (prError) {
      setError(getErrorMessage(prError));
    } finally {
      setPrLoading(false);
    }
  };

  const refreshPR = async () => {
    if (!currentSession) return;
    setPrLoading(true);
    try {
      const stats = await apiClient.refreshChatPR(currentSession.session_id);
      setSessions((current) =>
        current.map((s) =>
          s.session_id === currentSession.session_id ? { ...s, git: stats } : s,
        ),
      );
    } catch (prError) {
      setError(getErrorMessage(prError));
    } finally {
      setPrLoading(false);
    }
  };

  const handleCopyMessage = useCallback(async (messageId: string, content: string) => {
    try {
      await navigator.clipboard.writeText(content);
      setCopiedMessageId(messageId);
      setTimeout(() => setCopiedMessageId((prev) => (prev === messageId ? null : prev)), 2000);
    } catch {
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

  const handleSetMode = useCallback(
    (modeId: string) => {
      if (!activeSession) return;
      wsClient.send({
        type: "chat.set_mode",
        data: {
          session_id: activeSession,
          mode_id: modeId,
        },
      });
    },
    [activeSession, wsClient],
  );

  const handleSetConfigOption = useCallback(
    (configId: string, value: string) => {
      if (!activeSession) return;
      wsClient.send({
        type: "chat.set_config",
        data: {
          session_id: activeSession,
          config_id: configId,
          value,
        },
      });
    },
    [activeSession, wsClient],
  );

  const handlePermissionResponse = useCallback(
    (permissionId: string, optionId: string, cancel: boolean) => {
      wsClient.send({
        type: "chat.permission_response",
        data: {
          permission_id: permissionId,
          option_id: optionId,
          cancel,
        },
      });
      setPendingPermissions((prev) => prev.filter((p) => p.permission_id !== permissionId));
    },
    [wsClient],
  );

  const handleInputChange = (val: string) => {
    setMessageInput(val);
    if (val.startsWith("/")) {
      setShowCommandPalette(true);
      setCommandFilter(val.slice(1).split(" ")[0]);
    } else {
      setShowCommandPalette(false);
      setCommandFilter("");
    }
  };

  const handleInputKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === "Escape" && showCommandPalette) {
      setShowCommandPalette(false);
      return;
    }
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      setShowCommandPalette(false);
      void sendMessage();
    }
  };

  const handleCommandSelect = (name: string) => {
    setMessageInput(`/${name} `);
    setShowCommandPalette(false);
    setCommandFilter("");
  };

  return (
    <div className="flex h-full overflow-hidden">
      <ChatSessionSidebar
        groupedSessions={groupedSessions}
        activeSession={activeSession}
        sessionSearch={sessionSearch}
        loadingSessions={loadingSessions}
        creatingSession={submitting && !activeSession}
        messagesBySession={messagesBySession}
        collapsedGroups={collapsedGroups}
        pendingPermissionSessionIds={pendingPermissionSessionIds}
        onSearchChange={setSessionSearch}
        onSessionSelect={setActiveSession}
        onGroupToggle={handleGroupToggle}
        onCreateSession={createSession}
        onArchiveSession={archiveSession}
        {...(isMobile ? { drawerOpen: chatSidebarOpen, onClose: () => setChatSidebarOpen(false) } : {})}
      />

      <div className="flex flex-1 flex-col">
        {isMobile && (
          <div className="flex h-10 shrink-0 items-center border-b px-3">
            <button
              onClick={() => setChatSidebarOpen(true)}
              className="flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground hover:bg-accent"
              title="Sessions"
            >
              <MessagesSquare className="h-4 w-4" />
            </button>
          </div>
        )}
        {!isDraftSessionView && (
          <ChatHeader
            session={currentSession}
            driverLabel={currentDriverLabel}
            messageCount={currentMessages.length}
            submitting={submitting}
            usage={currentUsage}
            usagePercent={currentUsagePercent}
            detailView={detailView}
            lastUserMessage={lastUserMessage}
            onDetailViewChange={setDetailView}
            onCloseSession={() => void closeSession()}
            onRenameSession={(title) => void renameSession(title)}
            onCreatePR={() => void createPR()}
            onRefreshPR={() => void refreshPR()}
            prLoading={prLoading}
          />
        )}

        {error && (
          <div className="mx-5 mt-4 flex items-center gap-2 rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
            <span className="min-w-0 flex-1">{error}</span>
            <button
              type="button"
              className="shrink-0 rounded p-0.5 text-rose-400 transition-colors hover:text-rose-600"
              onClick={() => setError(null)}
            >
              <X className="h-3.5 w-3.5" />
            </button>
          </div>
        )}

        <ChatMainPanel
          detailView={detailView}
          currentEvents={currentEvents}
          isDraftSessionView={isDraftSessionView}
          projects={projects}
          draftProjectId={draftProjectId}
          draftProfileId={draftProfileId}
          draftDriverId={draftDriverId}
          draftLLMConfigId={draftLLMConfigId}
          draftUseWorktree={draftUseWorktree}
          leadDriverOptions={leadDriverOptions}
          leadProfiles={leadProfiles}
          drivers={drivers}
          llmConfigs={llmConfigs}
          messageInput={messageInput}
          pendingFiles={pendingFiles}
          draftSessionReady={draftSessionReady}
          submitting={submitting}
          currentDriverLabel={currentDriverLabel}
          currentProjectLabel={currentProjectLabel}
          fileInputRef={fileInputRef}
          onProjectChange={(id) => {
            setDraftProjectId(id);
            setSelectedProjectId(id);
          }}
          onProfileChange={(profileId) => {
            setDraftProfileId(profileId);
            const profile = leadProfiles.find((p) => p.id === profileId);
            if (profile?.driver_id) {
              setDraftDriverId(profile.driver_id);
            }
            setDraftLLMConfigId(profile?.llm_config_id || "system");
          }}
          onDriverChange={setDraftDriverId}
          onLLMConfigChange={setDraftLLMConfigId}
          onUseWorktreeChange={setDraftUseWorktree}
          onMessageChange={setMessageInput}
          onSend={() => void sendMessage()}
          onPaste={handlePaste}
          onRemovePendingFile={removePendingFile}
          chatFeedEntries={chatFeedEntries}
          hasMoreFeedEntries={hasMoreFeedEntries}
          loadingMore={loadingMore}
          visibleFeedEntries={visibleFeedEntries}
          copiedMessageId={copiedMessageId}
          collapsedActivityGroups={collapsedActivityGroups}
          activeSession={activeSession}
          sessionRunning={sessionRunning ?? false}
          chatContainerRef={chatContainerRef}
          messagesEndRef={messagesEndRef}
          onScroll={handleChatScroll}
          onCopyMessage={(id, content) => void handleCopyMessage(id, content)}
          onCreateWorkItem={handleCreateWorkItem}
          lastActivityText={lastActivityText}
          onActivityGroupToggle={(id) =>
            setCollapsedActivityGroups((prev) => {
              const currentlyCollapsed = prev[id] === true;
              return { ...prev, [id]: !currentlyCollapsed };
            })
          }
        />

        <PermissionBar
          permissions={visiblePendingPermissions}
          onResponse={handlePermissionResponse}
        />

        {!isDraftSessionView && (
          <ChatInputBar
            messageInput={messageInput}
            pendingFiles={pendingFiles}
            currentSession={currentSession}
            submitting={submitting}
            draftSessionReady={draftSessionReady}
            currentDriverLabel={currentDriverLabel}
            currentProjectLabel={currentProjectLabel}
            showCommandPalette={showCommandPalette}
            availableCommands={availableCommands}
            commandFilter={commandFilter}
            fileInputRef={fileInputRef}
            onMessageChange={handleInputChange}
            onPaste={handlePaste}
            onKeyDown={handleInputKeyDown}
            sessionRunning={sessionRunning ?? false}
            onSend={() => void sendMessage()}
            onCancel={() => void cancelSession()}
            onCommandSelect={handleCommandSelect}
            onRemovePendingFile={removePendingFile}
            onCommandPaletteClose={() => setShowCommandPalette(false)}
            modes={sessionModes}
            configOptions={configOptions}
            onSetMode={handleSetMode}
            onSetConfigOption={handleSetConfigOption}
            pendingMessage={pendingMessage}
            onCancelPending={cancelPendingMessage}
          />
        )}

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
