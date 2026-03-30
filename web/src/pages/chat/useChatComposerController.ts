import { useCallback, useRef, useState } from "react";
import type React from "react";
import type { TFunction } from "i18next";
import { getErrorMessage } from "@/lib/v2Workbench";
import type { SessionRecord, ChatMessageView } from "@/components/chat/chatTypes";
import type { WsClient } from "@/lib/wsClient";

export interface PendingSendDraft {
  messageInput: string;
  pendingFiles: File[];
}

export interface PendingDraftInfo {
  projectId?: number;
  projectName?: string;
  profileId: string;
  driverId: string;
  title: string;
}

interface ChatAttachmentPayload {
  name: string;
  mime_type: string;
  data: string;
}

interface UseChatComposerControllerOptions {
  wsClient: WsClient;
  t: TFunction;
  activeSession: string | null;
  currentSession: SessionRecord | null;
  draftProjectId: number | null;
  draftProfileId: string;
  draftDriverId: string;
  draftLLMConfigId: string;
  draftUseWorktree: boolean;
  projectNameMap: Map<number, string>;
  setSessions: React.Dispatch<React.SetStateAction<SessionRecord[]>>;
  setMessagesBySession: React.Dispatch<React.SetStateAction<Record<string, ChatMessageView[]>>>;
  setDraftMessages: React.Dispatch<React.SetStateAction<ChatMessageView[]>>;
  setSubmitting: React.Dispatch<React.SetStateAction<boolean>>;
  onError: (message: string | null) => void;
}

export function useChatComposerController({
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
  onError,
}: UseChatComposerControllerOptions) {
  const [messageInput, setMessageInput] = useState("");
  const [pendingFiles, setPendingFiles] = useState<File[]>([]);
  const [showCommandPalette, setShowCommandPalette] = useState(false);
  const [commandFilter, setCommandFilter] = useState("");
  const fileInputRef = useRef<HTMLInputElement>(null);
  const pendingRequestIdRef = useRef<string | null>(null);
  const pendingSendDraftRef = useRef<PendingSendDraft | null>(null);
  const pendingDraftInfoRef = useRef<PendingDraftInfo | null>(null);

  const appendMessage = useCallback((
    sessionId: string | null,
    role: "user" | "assistant",
    content: string,
    attachments?: ChatAttachmentPayload[],
  ) => {
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
  }, [setDraftMessages, setMessagesBySession, setSessions, t]);

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
        const width = Math.round(img.width * scale);
        const height = Math.round(img.height * scale);
        const canvas = document.createElement("canvas");
        canvas.width = width;
        canvas.height = height;
        const ctx = canvas.getContext("2d");
        if (!ctx) {
          resolve(file);
          return;
        }
        ctx.drawImage(img, 0, 0, width, height);
        canvas.toBlob((blob) => {
          if (blob && blob.size < file.size) {
            resolve(new File([blob], file.name, { type: "image/jpeg", lastModified: Date.now() }));
            return;
          }
          resolve(file);
        }, "image/jpeg", quality);
      };
      img.onerror = () => {
        URL.revokeObjectURL(url);
        resolve(file);
      };
      img.src = url;
    });
  }, []);

  const compressFiles = useCallback(async (files: File[]) => Promise.all(files.map((file) => compressImage(file))), [compressImage]);

  const handlePaste = useCallback(async (event: React.ClipboardEvent) => {
    const items = event.clipboardData?.items;
    if (!items) return;
    const nextFiles: File[] = [];
    for (const item of items) {
      if (item.kind === "file") {
        const file = item.getAsFile();
        if (file) nextFiles.push(file);
      }
    }
    if (nextFiles.length > 0) {
      const compressed = await compressFiles(nextFiles);
      setPendingFiles((current) => [...current, ...compressed]);
    }
  }, [compressFiles]);

  const handleFileSelect = useCallback(async (event: React.ChangeEvent<HTMLInputElement>) => {
    const files = event.target.files;
    if (files && files.length > 0) {
      const compressed = await compressFiles(Array.from(files));
      setPendingFiles((current) => [...current, ...compressed]);
    }
    event.target.value = "";
  }, [compressFiles]);

  const removePendingFile = useCallback((index: number) => {
    setPendingFiles((current) => current.filter((_, itemIndex) => itemIndex !== index));
  }, []);

  const clearComposer = useCallback(() => {
    setMessageInput("");
    setPendingFiles([]);
    setShowCommandPalette(false);
    setCommandFilter("");
  }, []);

  const sendMessage = useCallback(async () => {
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

    if (!resolvedProfileId || !resolvedDriverId) {
      onError(t("chat.selectDriverFirst"));
      return;
    }

    const attachments: ChatAttachmentPayload[] = [];
    for (const file of pendingFiles) {
      const buf = await file.arrayBuffer();
      const bytes = new Uint8Array(buf);
      let binary = "";
      const chunkSize = 8192;
      for (let index = 0; index < bytes.length; index += chunkSize) {
        binary += String.fromCharCode(...bytes.subarray(index, index + chunkSize));
      }
      attachments.push({
        name: file.name,
        mime_type: file.type || "application/octet-stream",
        data: btoa(binary),
      });
    }

    const displayContent = content + (attachments.length > 0
      ? `\n${t("chat.attachmentLabel", { names: attachments.map((attachment) => attachment.name).join(", ") })}`
      : "");
    appendMessage(workingSessionId, "user", displayContent, attachments);
    pendingSendDraftRef.current = {
      messageInput,
      pendingFiles,
    };
    clearComposer();
    setSubmitting(true);
    onError(null);

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
      pendingSendDraftRef.current = null;
      onError(errorMessage);
      if (workingSessionId) {
        setSessions((current) =>
          current.map((session) =>
            session.session_id === workingSessionId ? { ...session, status: "alive" } : session,
          ),
        );
      }
    } finally {
      if (!pendingRequestIdRef.current) {
        setSubmitting(false);
      }
    }
  }, [
    activeSession,
    appendMessage,
    clearComposer,
    currentSession,
    draftDriverId,
    draftLLMConfigId,
    draftProfileId,
    draftProjectId,
    draftUseWorktree,
    messageInput,
    onError,
    pendingFiles,
    projectNameMap,
    setSessions,
    setSubmitting,
    t,
    wsClient,
  ]);

  const handleInputChange = useCallback((value: string) => {
    setMessageInput(value);
    if (value.startsWith("/")) {
      setShowCommandPalette(true);
      setCommandFilter(value.slice(1).split(" ")[0]);
      return;
    }
    setShowCommandPalette(false);
    setCommandFilter("");
  }, []);

  const handleInputKeyDown = useCallback((event: React.KeyboardEvent) => {
    if (event.key === "Escape" && showCommandPalette) {
      setShowCommandPalette(false);
      return;
    }
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      setShowCommandPalette(false);
      void sendMessage();
    }
  }, [sendMessage, showCommandPalette]);

  const handleCommandSelect = useCallback((name: string) => {
    setMessageInput(`/${name} `);
    setShowCommandPalette(false);
    setCommandFilter("");
  }, []);

  return {
    messageInput,
    setMessageInput,
    pendingFiles,
    setPendingFiles,
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
  };
}
