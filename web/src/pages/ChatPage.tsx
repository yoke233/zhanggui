import { startTransition, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import type {
  AgentDriver,
  AgentProfile,
  ConfigOption,
  Event as ApiEvent,
  SessionModeState,
  SlashCommand,
} from "@/types/apiV2";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { getErrorMessage } from "@/lib/v2Workbench";
import type {
  SessionRecord,
  ChatMessageView,
  ChatActivityView,
  ChatFeedItem,
  ChatFeedEntry,
  RealtimeChatOutputPayload,
  RealtimeChatAckPayload,
  RealtimeChatErrorPayload,
  LeadDriverOption,
  SessionGroup,
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
  toEventListItem,
  touchSessionList,
  applyActivityPayload,
  toRealtimePayload,
  buildRealtimeEvent,
  buildActivityHistory,
  computeEventLevel,
  EVENT_LEVEL_ORDER,
  type EventLevel,
} from "@/components/chat/chatUtils";
import { ChatSessionSidebar } from "@/components/chat/ChatSessionSidebar";
import { ChatHeader } from "@/components/chat/ChatHeader";
import { DraftSessionSetup } from "@/components/chat/DraftSessionSetup";
import { MessageFeedView } from "@/components/chat/MessageFeedView";
import { ChatInputBar } from "@/components/chat/ChatInputBar";
import { EventLogRow } from "@/components/chat/EventLogRow";
import { ChatScrollTrack } from "@/components/chat/ChatScrollTrack";
import { Loader2 } from "lucide-react";

const FEED_PAGE_SIZE = 100;

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
  const [sessionModes, setSessionModes] = useState<SessionModeState | null>(null);
  const [showCommandPalette, setShowCommandPalette] = useState(false);
  const [commandFilter, setCommandFilter] = useState("");
  const [collapsedActivityGroups, setCollapsedActivityGroups] = useState<Record<string, boolean>>({});
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const chatContainerRef = useRef<HTMLDivElement>(null);
  const pendingChunkBuffersRef = useRef<Record<string, string>>({});
  const chunkFlushFrameRef = useRef<number | null>(null);
  const pendingRequestIdRef = useRef<string | null>(null);
  const syntheticEventIdRef = useRef(-1);
  const pendingDraftInfoRef = useRef<{
    projectId?: number;
    projectName?: string;
    profileId: string;
    driverId: string;
    title: string;
  } | null>(null);
  const prevScrollHeightRef = useRef<number>(0);

  // Feed pagination: show last FEED_PAGE_SIZE entries, expand on scroll-to-top
  const [feedVisibleCount, setFeedVisibleCount] = useState(FEED_PAGE_SIZE);
  const [loadingMore, setLoadingMore] = useState(false);

  // Event level filter for the events panel
  const [minEventLevel, setMinEventLevel] = useState<EventLevel>("info");

  const syncSessionDetail = (detail: import("@/types/apiV2").ChatSessionDetail) => {
    const record = toDetailRecord(detail, t);
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
    if (detail.modes) {
      setSessionModes(detail.modes);
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

  // Paginated slice of the feed — show last feedVisibleCount entries
  const visibleFeedEntries = useMemo(() => {
    const start = Math.max(0, chatFeedEntries.length - feedVisibleCount);
    return chatFeedEntries.slice(start);
  }, [chatFeedEntries, feedVisibleCount]);

  const hasMoreFeedEntries = feedVisibleCount < chatFeedEntries.length;

  // Event level filter
  const filteredEventItems = useMemo(
    () =>
      currentEventItems.filter(
        (item) => EVENT_LEVEL_ORDER[computeEventLevel(item.rawType)] >= EVENT_LEVEL_ORDER[minEventLevel],
      ),
    [currentEventItems, minEventLevel],
  );

  // Reset feed pagination when switching sessions
  useEffect(() => {
    setFeedVisibleCount(FEED_PAGE_SIZE);
  }, [activeSession]);

  const handleChatScroll = useCallback(
    (e: React.UIEvent<HTMLDivElement>) => {
      const el = e.currentTarget;
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
    const unsubscribeModeUpdate = wsClient.subscribe<{ session_id?: string; modes?: SessionModeState }>(
      "chat.mode_updated",
      (payload) => {
        if (payload.modes) {
          setSessionModes(payload.modes);
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
        onSearchChange={setSessionSearch}
        onSessionSelect={setActiveSession}
        onGroupToggle={(key) =>
          setCollapsedGroups((current) => ({
            ...current,
            [key]: !current[key],
          }))
        }
        onCreateSession={createSession}
      />

      <div className="flex flex-1 flex-col">
        {!isDraftSessionView && (
          <ChatHeader
            session={currentSession}
            driverLabel={currentDriverLabel}
            messageCount={currentMessages.length}
            submitting={submitting}
            usage={currentUsage}
            usagePercent={currentUsagePercent}
            detailView={detailView}
            onDetailViewChange={setDetailView}
            onCloseSession={() => void closeSession()}
          />
        )}

        {error && (
          <p className="mx-5 mt-4 rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
            {error}
          </p>
        )}

        <div className="relative flex-1">
          <div
            ref={chatContainerRef}
            className="absolute inset-0 overflow-y-auto px-5 py-4 pr-6 [scrollbar-gutter:stable]"
            onScroll={handleChatScroll}
          >
          {detailView === "events" ? (
            <>
              {/* Event level filter */}
              <div className="mx-auto mb-3 flex w-full max-w-[1200px] items-center gap-2 text-xs text-muted-foreground">
                <span>{t("chat.showLevel", { defaultValue: "显示级别:" })}</span>
                {(["debug", "info", "warning", "error"] as EventLevel[]).map((level) => (
                  <button
                    key={level}
                    type="button"
                    onClick={() => setMinEventLevel(level)}
                    className={[
                      "rounded px-2 py-0.5 font-mono transition-colors",
                      minEventLevel === level
                        ? "bg-primary text-primary-foreground"
                        : "bg-muted hover:bg-muted/80 text-muted-foreground",
                    ].join(" ")}
                  >
                    {level}
                  </button>
                ))}
                <span className="ml-auto text-[10px]">
                  {filteredEventItems.length} / {currentEventItems.length}
                </span>
              </div>
              {filteredEventItems.length === 0 ? (
                <div className="mx-auto w-full max-w-[1200px] rounded-xl border border-dashed bg-muted/20 px-5 py-6 text-sm text-muted-foreground">
                  {t("chat.noEvents")}
                </div>
              ) : (
                <table className="mx-auto w-full max-w-[1200px] border-collapse text-left">
                  <thead>
                    <tr className="border-b border-border text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                      <th className="py-1.5 pl-3 pr-2">{t("chat.colTime", { defaultValue: "时间" })}</th>
                      <th className="py-1.5 pr-2" />
                      <th className="py-1.5 pr-3">{t("chat.colType", { defaultValue: "类型" })}</th>
                      <th className="py-1.5 pr-2">{t("chat.colSummary", { defaultValue: "摘要" })}</th>
                      <th className="py-1.5 pr-2" />
                    </tr>
                  </thead>
                  <tbody>
                    {filteredEventItems.map((item) => <EventLogRow key={item.id} item={item} />)}
                  </tbody>
                </table>
              )}
            </>
          ) : isDraftSessionView ? (
            <div className="flex min-h-full items-center justify-center">
              <DraftSessionSetup
                projects={projects}
                draftProjectId={draftProjectId}
                draftDriverId={draftDriverId}
                leadDriverOptions={leadDriverOptions}
                leadProfiles={leadProfiles}
                drivers={drivers}
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
                onDriverChange={setDraftDriverId}
                onMessageChange={setMessageInput}
                onSend={() => void sendMessage()}
                onPaste={handlePaste}
                onRemovePendingFile={removePendingFile}
              />
            </div>
          ) : chatFeedEntries.length === 0 ? (
            <div className="mx-auto w-full max-w-[1200px] rounded-2xl border border-dashed bg-muted/20 px-5 py-6 text-sm text-muted-foreground">
              {t("chat.noMessagesInSession")}
            </div>
          ) : (
            <div className="mx-auto w-full max-w-[1200px] space-y-1">
              {/* Load-more indicator at top */}
              {hasMoreFeedEntries && (
                <div className="flex items-center justify-center py-2 text-xs text-muted-foreground">
                  {loadingMore
                    ? t("chat.loadingMore", { defaultValue: "加载中..." })
                    : t("chat.scrollUpForMore", { defaultValue: "向上滚动加载更早消息" })}
                </div>
              )}
              <MessageFeedView
                entries={visibleFeedEntries}
                submitting={submitting}
                copiedMessageId={copiedMessageId}
                collapsedActivityGroups={collapsedActivityGroups}
                onCopyMessage={(id, content) => void handleCopyMessage(id, content)}
                onCreateWorkItem={handleCreateWorkItem}
                onActivityGroupToggle={(id) =>
                  setCollapsedActivityGroups((prev) => ({ ...prev, [id]: !(collapsedActivityGroups[id] !== false) }))
                }
              />
              {submitting && !activeSession && (
                <div className="flex items-center gap-2.5 rounded-xl border border-blue-100 bg-blue-50/60 px-4 py-3 text-sm text-blue-600">
                  <Loader2 className="h-4 w-4 animate-spin" />
                  <span>{t("chat.creatingSession", { defaultValue: "正在创建会话..." })}</span>
                </div>
              )}
            </div>
          )}
          <div ref={messagesEndRef} />
          </div>{/* end scrollable inner */}
          <ChatScrollTrack containerRef={chatContainerRef} />
        </div>{/* end relative wrapper */}

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
            onSend={() => void sendMessage()}
            onCommandSelect={handleCommandSelect}
            onRemovePendingFile={removePendingFile}
            onCommandPaletteClose={() => setShowCommandPalette(false)}
            modes={sessionModes}
            configOptions={configOptions}
            onSetMode={handleSetMode}
            onSetConfigOption={handleSetConfigOption}
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
