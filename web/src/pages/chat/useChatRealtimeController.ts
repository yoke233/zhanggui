import {
  startTransition,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type UIEvent,
} from "react";
import type { TFunction } from "i18next";
import type { ConfigOption, Event as ApiEvent, SessionModeState } from "@/types/apiV2";
import type {
  ChatActivityView,
  ChatMessageView,
  PendingMessageView,
  RealtimeChatAckPayload,
  RealtimeChatErrorPayload,
  RealtimeChatOutputPayload,
  SessionRecord,
} from "@/components/chat/chatTypes";
import { applyActivityPayload, buildRealtimeEvent, touchSessionList } from "@/components/chat/chatUtils";
import type { WsClient } from "@/lib/wsClient";
import type { RuntimeConfigReloadedPayload } from "@/types/ws";
import type { PermissionRequest } from "@/components/chat/PermissionBar";
import type { PendingDraftInfo, PendingSendDraft } from "./useChatComposerController";

interface UseChatRealtimeControllerOptions {
  wsClient: WsClient;
  t: TFunction;
  activeSession: string | null;
  detailView: "chat" | "events";
  currentMessages: ChatMessageView[];
  currentEvents: ApiEvent[];
  currentActivities: ChatActivityView[];
  loadAgentCatalog: () => Promise<void>;
  pendingRequestIdRef: React.MutableRefObject<string | null>;
  pendingSendDraftRef: React.MutableRefObject<PendingSendDraft | null>;
  pendingDraftInfoRef: React.MutableRefObject<PendingDraftInfo | null>;
  setSubmitting: React.Dispatch<React.SetStateAction<boolean>>;
  setSessions: React.Dispatch<React.SetStateAction<SessionRecord[]>>;
  setMessagesBySession: React.Dispatch<React.SetStateAction<Record<string, ChatMessageView[]>>>;
  setEventsBySession: React.Dispatch<React.SetStateAction<Record<string, ApiEvent[]>>>;
  setActivitiesBySession: React.Dispatch<React.SetStateAction<Record<string, ChatActivityView[]>>>;
  setDraftMessages: React.Dispatch<React.SetStateAction<ChatMessageView[]>>;
  setLoadedSessions: React.Dispatch<React.SetStateAction<Record<string, boolean>>>;
  setConfigOptionsBySession: React.Dispatch<React.SetStateAction<Record<string, ConfigOption[]>>>;
  setModesBySession: React.Dispatch<React.SetStateAction<Record<string, SessionModeState | null>>>;
  setActiveSession: React.Dispatch<React.SetStateAction<string | null>>;
  onError: (message: string | null) => void;
}

export function useChatRealtimeController({
  wsClient,
  t,
  activeSession,
  detailView,
  currentMessages,
  currentEvents,
  currentActivities,
  loadAgentCatalog,
  pendingRequestIdRef,
  pendingSendDraftRef,
  pendingDraftInfoRef,
  setSubmitting,
  setSessions,
  setMessagesBySession,
  setEventsBySession,
  setActivitiesBySession,
  setDraftMessages,
  setLoadedSessions,
  setConfigOptionsBySession,
  setModesBySession,
  setActiveSession,
  onError,
}: UseChatRealtimeControllerOptions) {
  const [pendingPermissions, setPendingPermissions] = useState<PermissionRequest[]>([]);
  const [pendingMessage, setPendingMessage] = useState<PendingMessageView | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const messageContainerRef = useRef<HTMLDivElement>(null);
  const pendingChunkBuffersRef = useRef<Record<string, string>>({});
  const chunkFlushFrameRef = useRef<number | null>(null);
  const isNearBottomRef = useRef(true);
  const prevActiveSessionRef = useRef<string | null>(null);
  const syntheticEventIdRef = useRef(-1);
  const justSwitchedSessionRef = useRef(false);

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
  }, [setMessagesBySession, setSessions]);

  const scheduleChunkFlush = useCallback(() => {
    if (chunkFlushFrameRef.current != null) {
      return;
    }
    chunkFlushFrameRef.current = requestAnimationFrame(() => {
      chunkFlushFrameRef.current = null;
      flushBufferedChunks();
    });
  }, [flushBufferedChunks]);

  const handleChatScroll = useCallback((event: UIEvent<HTMLDivElement>) => {
    const element = event.currentTarget;
    isNearBottomRef.current = element.scrollHeight - element.scrollTop - element.clientHeight < 80;
  }, []);

  useEffect(() => {
    const sessionChanged = prevActiveSessionRef.current !== activeSession;
    prevActiveSessionRef.current = activeSession;
    if (sessionChanged) {
      isNearBottomRef.current = true;
      justSwitchedSessionRef.current = true;
      messagesEndRef.current?.scrollIntoView({ behavior: "auto" });
      return;
    }
    if (!isNearBottomRef.current) return;
    if (justSwitchedSessionRef.current) {
      justSwitchedSessionRef.current = false;
      messagesEndRef.current?.scrollIntoView({ behavior: "auto" });
      return;
    }
    const isStreaming = currentMessages.at(-1)?.id.endsWith("stream-assistant");
    messagesEndRef.current?.scrollIntoView({ behavior: isStreaming ? "auto" : "smooth" });
  }, [activeSession, currentActivities, currentEvents, currentMessages, detailView]);

  useEffect(() => {
    const unsubscribeOutput = wsClient.subscribe<RealtimeChatOutputPayload>("chat.output", (payload) => {
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
        onError(payload.content?.trim() || t("chat.sessionFailed"));
        setSessions((current) => touchSessionList(current, sessionId, "alive", nowISO));
        setSubmitting(false);
        pendingRequestIdRef.current = null;
        return;
      }

      startTransition(() => {
        setActivitiesBySession((current) => ({
          ...current,
          [sessionId]: applyActivityPayload(current[sessionId] ?? [], sessionId, payload, nowISO, t),
        }));
        setSessions((current) => touchSessionList(current, sessionId, "running", nowISO));
      });
    });

    const unsubscribeAck = wsClient.subscribe<RealtimeChatAckPayload>("chat.ack", (payload) => {
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

      setDraftMessages((draftCurrent) => {
        if (draftCurrent.length > 0) {
          setMessagesBySession((messageCurrent) => ({
            ...messageCurrent,
            [sessionId]: draftCurrent.map((message) => ({
              ...message,
              id: message.id.replace("draft", sessionId),
            })),
          }));
        }
        return [];
      });

      const draftInfo = pendingDraftInfoRef.current;
      pendingDraftInfoRef.current = null;
      const now = new Date().toISOString();
      setSessions((current) => {
        if (current.some((session) => session.session_id === sessionId)) {
          return current;
        }
        return [{
          session_id: sessionId,
          title: draftInfo?.title || t("chat.newSession"),
          status: "running",
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
    });

    const unsubscribeError = wsClient.subscribe<RealtimeChatErrorPayload>("chat.error", (payload) => {
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
      if (errorMessage.toLowerCase().includes("session is already running")) {
        pendingSendDraftRef.current = null;
        return;
      }

      pendingSendDraftRef.current = null;
      onError(errorMessage);
      const sessionId = payload.session_id?.trim();
      if (sessionId) {
        setSessions((current) => touchSessionList(current, sessionId, "alive", new Date().toISOString()));
      }
    });

    const unsubscribeConfigUpdate = wsClient.subscribe<{ session_id?: string; config_options?: ConfigOption[] }>("chat.config_updated", (payload) => {
      const sessionId = payload.session_id?.trim();
      if (sessionId && payload.config_options) {
        setConfigOptionsBySession((current) => ({ ...current, [sessionId]: payload.config_options ?? [] }));
      }
    });

    const unsubscribeModeUpdate = wsClient.subscribe<{ session_id?: string; modes?: SessionModeState }>("chat.mode_updated", (payload) => {
      const sessionId = payload.session_id?.trim();
      if (sessionId && payload.modes) {
        setModesBySession((current) => ({ ...current, [sessionId]: payload.modes ?? null }));
      }
    });

    const unsubscribePermissionRequest = wsClient.subscribe<PermissionRequest>("chat.permission_request", (payload) => {
      if (payload.permission_id) {
        setPendingPermissions((current) => [...current, payload]);
      }
    });

    const unsubscribePermissionResolved = wsClient.subscribe<{ permission_id?: string }>("chat.permission_resolved", (payload) => {
      if (payload.permission_id) {
        setPendingPermissions((current) => current.filter((permission) => permission.permission_id !== payload.permission_id));
      }
    });

    const unsubscribeRuntimeConfigReloaded = wsClient.subscribe<RuntimeConfigReloadedPayload>("runtime.config_reloaded", () => {
      void loadAgentCatalog();
    });

    const unsubscribeSessionTitle = wsClient.subscribe<{ session_id?: string; title?: string }>("chat.session_title", (payload) => {
      const sessionId = payload.session_id?.trim();
      const title = payload.title?.trim();
      if (sessionId && title) {
        setSessions((current) =>
          current.map((session) => (session.session_id === sessionId ? { ...session, title } : session)),
        );
      }
    });

    const unsubscribePendingCancelled = wsClient.subscribe<{ session_id?: string }>("chat.pending_cancelled", (payload) => {
      if (payload.session_id?.trim()) {
        setPendingMessage(null);
      }
    });

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
  }, [
    flushBufferedChunks,
    loadAgentCatalog,
    onError,
    pendingDraftInfoRef,
    pendingRequestIdRef,
    pendingSendDraftRef,
    scheduleChunkFlush,
    setActiveSession,
    setActivitiesBySession,
    setConfigOptionsBySession,
    setDraftMessages,
    setEventsBySession,
    setLoadedSessions,
    setMessagesBySession,
    setModesBySession,
    setSessions,
    setSubmitting,
    t,
    wsClient,
  ]);

  const visiblePendingPermissions = useMemo(
    () => pendingPermissions.filter((permission) => permission.session_id === activeSession),
    [activeSession, pendingPermissions],
  );

  const pendingPermissionSessionIds = useMemo(
    () => new Set(pendingPermissions.map((permission) => permission.session_id)),
    [pendingPermissions],
  );

  const dismissPendingPermission = useCallback((permissionId: string) => {
    setPendingPermissions((current) =>
      current.filter((permission) => permission.permission_id !== permissionId),
    );
  }, []);

  return {
    pendingPermissions,
    visiblePendingPermissions,
    pendingPermissionSessionIds,
    pendingMessage,
    setPendingMessage,
    dismissPendingPermission,
    messageContainerRef,
    messagesEndRef,
    handleChatScroll,
  };
}
