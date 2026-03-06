import { create } from "zustand";
import type { ChatMessage, ChatSession } from "../types/workflow";
import type { AvailableCommand, ConfigOption } from "../types/ws";

const upsertSession = (
  sessions: ChatSession[],
  incoming: ChatSession,
): ChatSession[] => {
  const index = sessions.findIndex((session) => session.id === incoming.id);
  if (index < 0) {
    return [...sessions, incoming];
  }
  const next = sessions.slice();
  next[index] = { ...next[index], ...incoming };
  return next;
};

interface ChatState {
  sessionsByProjectId: Record<string, ChatSession[]>;
  commandsBySessionId: Record<string, AvailableCommand[]>;
  configOptionsBySessionId: Record<string, ConfigOption[]>;
  activeSessionId: string | null;
  loading: boolean;
  error: string | null;
  setSessions: (projectId: string, sessions: ChatSession[]) => void;
  upsertSession: (projectId: string, session: ChatSession) => void;
  appendMessage: (projectId: string, sessionId: string, message: ChatMessage) => void;
  selectSession: (sessionId: string | null) => void;
  clearSession: (projectId: string, sessionId: string) => void;
  setCommands: (sessionId: string, commands: AvailableCommand[]) => void;
  setConfigOptions: (sessionId: string, options: ConfigOption[]) => void;
  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;
  reset: () => void;
}

const initialState = {
  sessionsByProjectId: {} as Record<string, ChatSession[]>,
  commandsBySessionId: {} as Record<string, AvailableCommand[]>,
  configOptionsBySessionId: {} as Record<string, ConfigOption[]>,
  activeSessionId: null as string | null,
  loading: false,
  error: null as string | null,
};

export const useChatStore = create<ChatState>((set) => ({
  ...initialState,
  setSessions: (projectId, sessions) =>
    set((state) => ({
      sessionsByProjectId: {
        ...state.sessionsByProjectId,
        [projectId]: sessions,
      },
    })),
  upsertSession: (projectId, session) =>
    set((state) => ({
      sessionsByProjectId: {
        ...state.sessionsByProjectId,
        [projectId]: upsertSession(state.sessionsByProjectId[projectId] ?? [], session),
      },
    })),
  appendMessage: (projectId, sessionId, message) =>
    set((state) => ({
      sessionsByProjectId: {
        ...state.sessionsByProjectId,
        [projectId]: (state.sessionsByProjectId[projectId] ?? []).map((session) =>
          session.id === sessionId
            ? { ...session, messages: [...session.messages, message] }
            : session,
        ),
      },
    })),
  selectSession: (sessionId) => set({ activeSessionId: sessionId }),
  clearSession: (projectId, sessionId) =>
    set((state) => ({
      sessionsByProjectId: {
        ...state.sessionsByProjectId,
        [projectId]: (state.sessionsByProjectId[projectId] ?? []).filter(
          (session) => session.id !== sessionId,
        ),
      },
      commandsBySessionId: Object.fromEntries(
        Object.entries(state.commandsBySessionId).filter(
          ([key]) => key !== sessionId,
        ),
      ),
      configOptionsBySessionId: Object.fromEntries(
        Object.entries(state.configOptionsBySessionId).filter(
          ([key]) => key !== sessionId,
        ),
      ),
      activeSessionId:
        state.activeSessionId === sessionId ? null : state.activeSessionId,
    })),
  setCommands: (sessionId, commands) =>
    set((state) => ({
      commandsBySessionId: {
        ...state.commandsBySessionId,
        [sessionId]: commands,
      },
    })),
  setConfigOptions: (sessionId, options) =>
    set((state) => ({
      configOptionsBySessionId: {
        ...state.configOptionsBySessionId,
        [sessionId]: options,
      },
    })),
  setLoading: (loading) => set({ loading }),
  setError: (error) => set({ error }),
  reset: () => set({ ...initialState }),
}));
