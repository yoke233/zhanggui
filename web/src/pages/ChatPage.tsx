import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useNavigate } from "react-router-dom";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { getErrorMessage } from "@/lib/v2Workbench";
import { ChatSessionSidebar } from "@/components/chat/ChatSessionSidebar";
import { ChatHeader } from "@/components/chat/ChatHeader";
import { ChatMainPanel } from "@/components/chat/ChatMainPanel";
import { ChatInputBar } from "@/components/chat/ChatInputBar";
import { ChatPageShell } from "@/components/chat/ChatPageShell";
import { ChatErrorBanner } from "@/components/chat/ChatErrorBanner";
import { PermissionBar } from "@/components/chat/PermissionBar";
import { defaultDraftProfileID } from "@/components/chat/chatUtils";
import { useChatFeed } from "@/components/chat/useChatFeed";
import { useIsMobile } from "@/hooks/useIsMobile";
import { useChatSessionController } from "./chat/useChatSessionController";
import { useChatComposerController } from "./chat/useChatComposerController";
import { useChatRealtimeController } from "./chat/useChatRealtimeController";

const FEED_PAGE_SIZE = 100;

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
  const [detailView, setDetailView] = useState<"chat" | "events">("chat");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [collapsedActivityGroups, setCollapsedActivityGroups] = useState<Record<string, boolean>>({});
  const [prLoading, setPrLoading] = useState(false);
  const [submitLoading, setSubmitLoading] = useState(false);
  const [feedVisibleCount, setFeedVisibleCount] = useState(FEED_PAGE_SIZE);
  const [loadingMore, setLoadingMore] = useState(false);

  const {
    setSessions,
    activeSession,
    setActiveSession,
    messagesBySession,
    setMessagesBySession,
    setEventsBySession,
    setActivitiesBySession,
    setDraftMessages,
    setLoadedSessions,
    setConfigOptionsBySession,
    setModesBySession,
    sessionSearch,
    setSessionSearch,
    drivers,
    leadProfiles,
    draftProjectId,
    setDraftProjectId,
    draftProfileId,
    setDraftProfileId,
    draftDriverId,
    setDraftDriverId,
    draftLLMConfigId,
    setDraftLLMConfigId,
    llmConfigs,
    draftUseWorktree,
    setDraftUseWorktree,
    collapsedGroups,
    loadingSessions,
    currentSession,
    projectNameMap,
    leadDriverOptions,
    currentProjectLabel,
    currentProfileLabel,
    draftSessionReady,
    currentDriverLabel,
    groupedSessions,
    currentMessages,
    currentEvents,
    currentActivities,
    availableCommands,
    configOptions,
    sessionModes,
    isDraftSessionView,
    currentUsage,
    currentUsagePercent,
    lastActivityText,
    lastUserMessage,
    loadAgentCatalog,
    handleGroupToggle,
    archiveSession,
  } = useChatSessionController({
    apiClient,
    projects,
    selectedProjectId,
    onError: setError,
    t,
  });

  const {
    messageInput,
    setMessageInput,
    pendingFiles,
    showCommandPalette,
    setShowCommandPalette,
    commandFilter,
    fileInputRef,
    pendingRequestIdRef,
    pendingSendDraftRef,
    pendingDraftInfoRef,
    clearComposer,
    handlePaste,
    handleFileSelect,
    removePendingFile,
    sendMessage,
    handleInputChange,
    handleInputKeyDown,
    handleCommandSelect,
  } = useChatComposerController({
    wsClient,
    t,
    activeSession,
    currentSession,
    draftProjectId,
    draftProfileId,
    draftDriverId,
    draftLLMConfigId,
    draftUseWorktree,
    projectNameMap,
    setSessions,
    setMessagesBySession,
    setDraftMessages,
    setSubmitting,
    onError: setError,
  });

  const {
    visiblePendingPermissions,
    pendingPermissionSessionIds,
    pendingMessage,
    setPendingMessage,
    dismissPendingPermission,
    messageContainerRef,
    messagesEndRef,
    handleChatScroll,
  } = useChatRealtimeController({
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
    onError: setError,
  });

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
      if (state.pendingMessage) {
        const now = new Date();
        setDraftMessages([{
          id: "draft-home-msg",
          role: "user",
          content: state.pendingMessage,
          time: now.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" }),
          at: now.toISOString(),
        }]);
      }
      navigate(location.pathname, { replace: true, state: null });
    }
  }, [location.pathname, location.state, navigate, pendingDraftInfoRef, pendingRequestIdRef, setDraftMessages]);

  const { chatFeedEntries, visibleFeedEntries, hasMoreFeedEntries } = useChatFeed(
    currentMessages,
    currentActivities,
    feedVisibleCount,
  );
  const firstVisibleFeedIndex = Math.max(
    chatFeedEntries.length - visibleFeedEntries.length,
    0,
  );

  useEffect(() => {
    setFeedVisibleCount(FEED_PAGE_SIZE);
  }, [activeSession]);

  const handleFeedStartReached = useCallback(() => {
    if (!hasMoreFeedEntries || loadingMore) {
      return;
    }
    setLoadingMore(true);
    setFeedVisibleCount((current) => current + FEED_PAGE_SIZE);
  }, [hasMoreFeedEntries, loadingMore]);

  useEffect(() => {
    if (!loadingMore) {
      return;
    }
    requestAnimationFrame(() => {
      setLoadingMore(false);
    });
  }, [loadingMore, visibleFeedEntries.length]);

  const createSession = useCallback(() => {
    const selectedProfileID = defaultDraftProfileID(leadProfiles);
    setDraftProjectId(selectedProjectId);
    setDraftProfileId((current) => defaultDraftProfileID(leadProfiles, current || selectedProfileID));
    setDraftDriverId((current) => {
      if (current && drivers.some((driver) => driver.id === current)) {
        return current;
      }
      const selectedProfile = leadProfiles.find((profile) => profile.id === selectedProfileID);
      if (selectedProfile?.driver_id && drivers.some((driver) => driver.id === selectedProfile.driver_id)) {
        return selectedProfile.driver_id;
      }
      return leadDriverOptions[0]?.driverId ?? drivers[0]?.id ?? "";
    });
    setDraftUseWorktree(true);
    setActiveSession(null);
    setDraftMessages([]);
    clearComposer();
    setError(null);
  }, [
    clearComposer,
    drivers,
    leadDriverOptions,
    leadProfiles,
    selectedProjectId,
    setActiveSession,
    setDraftDriverId,
    setDraftMessages,
    setDraftProfileId,
    setDraftProjectId,
    setDraftUseWorktree,
  ]);

  const sessionRunning = currentSession?.status === "running";

  const cancelPendingMessage = useCallback(() => {
    if (!pendingMessage) return;
    wsClient.send({
      type: "chat.cancel_pending",
      data: { session_id: pendingMessage.sessionId },
    });
    setPendingMessage(null);
  }, [pendingMessage, setPendingMessage, wsClient]);

  const cancelSession = useCallback(async () => {
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
  }, [apiClient, currentSession, setSessions]);

  const closeSession = useCallback(async () => {
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
  }, [apiClient, currentSession, setSessions]);

  const renameSession = useCallback(async (title: string) => {
    if (!currentSession) return;
    try {
      await apiClient.renameChatSession(currentSession.session_id, title);
      setSessions((current) =>
        current.map((session) =>
          session.session_id === currentSession.session_id
            ? { ...session, title }
            : session,
        ),
      );
    } catch (renameError) {
      setError(getErrorMessage(renameError));
    }
  }, [apiClient, currentSession, setSessions]);

  const createPR = useCallback(async () => {
    if (!currentSession) return;
    setPrLoading(true);
    try {
      const stats = await apiClient.createChatPR(currentSession.session_id, currentSession.title);
      setSessions((current) =>
        current.map((session) =>
          session.session_id === currentSession.session_id
            ? { ...session, git: stats }
            : session,
        ),
      );
    } catch (prError) {
      setError(getErrorMessage(prError));
    } finally {
      setPrLoading(false);
    }
  }, [apiClient, currentSession, setSessions]);

  const submitCode = useCallback(async () => {
    if (!currentSession) return;
    setSubmitLoading(true);
    try {
      const stats = await apiClient.submitChatCode(currentSession.session_id, currentSession.title);
      setSessions((current) =>
        current.map((session) =>
          session.session_id === currentSession.session_id
            ? { ...session, git: stats }
            : session,
        ),
      );
    } catch (submitError) {
      setError(getErrorMessage(submitError));
    } finally {
      setSubmitLoading(false);
    }
  }, [apiClient, currentSession, setSessions]);

  const refreshPR = useCallback(async () => {
    if (!currentSession) return;
    setPrLoading(true);
    try {
      const stats = await apiClient.refreshChatPR(currentSession.session_id);
      setSessions((current) =>
        current.map((session) =>
          session.session_id === currentSession.session_id
            ? { ...session, git: stats }
            : session,
        ),
      );
    } catch (prError) {
      setError(getErrorMessage(prError));
    } finally {
      setPrLoading(false);
    }
  }, [apiClient, currentSession, setSessions]);

  const handleCopyMessage = useCallback(async (messageId: string, content: string) => {
    try {
      await navigator.clipboard.writeText(content);
      setCopiedMessageId(messageId);
      setTimeout(() => setCopiedMessageId((current) => (current === messageId ? null : current)), 2000);
    } catch {
      const textarea = document.createElement("textarea");
      textarea.value = content;
      document.body.appendChild(textarea);
      textarea.select();
      document.execCommand("copy");
      document.body.removeChild(textarea);
      setCopiedMessageId(messageId);
      setTimeout(() => setCopiedMessageId((current) => (current === messageId ? null : current)), 2000);
    }
  }, []);

  const handleCreateWorkItem = useCallback((_: string, content: string) => {
    const params = new URLSearchParams();
    params.set("body", content);
    if (selectedProjectId) {
      params.set("project_id", String(selectedProjectId));
    }
    navigate(`/work-items/new?${params.toString()}`);
  }, [navigate, selectedProjectId]);

  const handleSetMode = useCallback((modeId: string) => {
    if (!activeSession) return;
    wsClient.send({
      type: "chat.set_mode",
      data: {
        session_id: activeSession,
        mode_id: modeId,
      },
    });
  }, [activeSession, wsClient]);

  const handleSetConfigOption = useCallback((configId: string, value: string) => {
    if (!activeSession) return;
    wsClient.send({
      type: "chat.set_config",
      data: {
        session_id: activeSession,
        config_id: configId,
        value,
      },
    });
  }, [activeSession, wsClient]);

  const handlePermissionResponse = useCallback((permissionId: string, optionId: string, cancel: boolean) => {
    wsClient.send({
      type: "chat.permission_response",
      data: {
        permission_id: permissionId,
        option_id: optionId,
        cancel,
      },
    });
    dismissPendingPermission(permissionId);
  }, [dismissPendingPermission, wsClient]);

  return (
    <ChatPageShell
      sidebar={
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
      }
      mobileHeader={null}
      header={
        !isDraftSessionView ? (
          <ChatHeader
            session={currentSession}
            driverLabel={currentDriverLabel}
            profileLabel={currentProfileLabel}
            messageCount={currentMessages.length}
            submitting={submitting}
            usage={currentUsage}
            usagePercent={currentUsagePercent}
            detailView={detailView}
            lastUserMessage={lastUserMessage}
            onDetailViewChange={setDetailView}
            onCloseSession={() => void closeSession()}
            onRenameSession={(title) => void renameSession(title)}
            onSubmitCode={() => void submitCode()}
            onCreatePR={() => void createPR()}
            onRefreshPR={() => void refreshPR()}
            submitLoading={submitLoading}
            prLoading={prLoading}
            onOpenSessions={isMobile ? () => setChatSidebarOpen(true) : undefined}
          />
        ) : null
      }
      errorBanner={error ? <ChatErrorBanner error={error} onClose={() => setError(null)} /> : null}
      mainPanel={
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
          currentProfileLabel={currentProfileLabel}
          fileInputRef={fileInputRef}
          onProjectChange={(id) => {
            setDraftProjectId(id);
            setSelectedProjectId(id);
          }}
          onProfileChange={(profileId) => {
            setDraftProfileId(profileId);
            const profile = leadProfiles.find((item) => item.id === profileId);
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
          firstVisibleFeedIndex={firstVisibleFeedIndex}
          activeSession={activeSession}
          sessionRunning={sessionRunning ?? false}
          messageContainerRef={messageContainerRef}
          messagesEndRef={messagesEndRef}
          onMessageListScroll={handleChatScroll}
          onFeedStartReached={handleFeedStartReached}
          onCopyMessage={(id, content) => void handleCopyMessage(id, content)}
          onCreateWorkItem={handleCreateWorkItem}
          lastActivityText={lastActivityText}
          onActivityGroupToggle={(id) =>
            setCollapsedActivityGroups((current) => ({
              ...current,
              [id]: !current[id],
            }))
          }
        />
      }
      permissionBar={
        <PermissionBar
          permissions={visiblePendingPermissions}
          onResponse={handlePermissionResponse}
        />
      }
      inputBar={
        !isDraftSessionView ? (
          <ChatInputBar
            messageInput={messageInput}
            pendingFiles={pendingFiles}
            currentSession={currentSession}
            submitting={submitting}
            draftSessionReady={draftSessionReady}
            currentDriverLabel={currentDriverLabel}
            currentProjectLabel={currentProjectLabel}
            currentProfileLabel={currentProfileLabel}
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
        ) : null
      }
      hiddenFileInput={
        <input
          ref={fileInputRef}
          type="file"
          multiple
          accept="image/*,.txt,.md,.json,.csv,.pdf,.yaml,.yml,.toml,.xml,.html,.css,.js,.ts,.tsx,.jsx,.go,.py,.rs,.java,.c,.cpp,.h,.hpp,.sh,.bat,.ps1,.sql,.log"
          className="hidden"
          onChange={handleFileSelect}
        />
      }
    />
  );
}
