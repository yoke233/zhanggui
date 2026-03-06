export interface WsEnvelope<TPayload = unknown> {
  type: string;
  run_id?: string;
  project_id?: string;
  issue_id?: string;
  session_id?: string;
  data?: TPayload;
  payload?: TPayload;
}

export type ChatEventType =
  | "run_started"
  | "run_update"
  | "run_completed"
  | "run_failed"
  | "run_cancelled"
  | "team_leader_thinking"
  | "team_leader_files_changed";

export interface AvailableCommand {
  name: string;
  description: string;
  input?: {
    hint?: string;
    [key: string]: unknown;
  };
}

export interface ConfigOptionValue {
  value: string;
  name: string;
  description?: string;
}

export interface ConfigOption {
  id: string;
  name: string;
  description?: string;
  category?: string;
  type: "select";
  currentValue: string;
  options: ConfigOptionValue[];
}

export interface ACPSessionUpdate {
  sessionUpdate?: string;
  availableCommands?: AvailableCommand[];
  configOptions?: unknown;
  content?: {
    type?: string;
    text?: string;
    [key: string]: unknown;
  };
  toolCallId?: string;
  title?: string;
  kind?: string;
  status?: string;
  entries?: Array<{
    content?: string;
    priority?: string;
    status?: string;
    [key: string]: unknown;
  }>;
  [key: string]: unknown;
}

export interface ChatEventPayload {
  session_id?: string;
  role?: string;
  agent_session_id?: string;
  reply?: string;
  error?: string;
  acp?: ACPSessionUpdate;
  timestamp?: string;
  [key: string]: unknown;
}

export interface ChatEventEnvelope extends WsEnvelope<ChatEventPayload> {
  type: ChatEventType;
}

export type ProjectCreateEventType =
  | "project_create_started"
  | "project_create_progress"
  | "project_create_succeeded"
  | "project_create_failed";

export interface ProjectCreateEventPayload {
  request_id?: string;
  project_id?: string;
  progress?: number;
  message?: string;
  error?: string;
}

export interface ProjectCreateEventEnvelope
  extends WsEnvelope<ProjectCreateEventPayload> {
  type: ProjectCreateEventType;
}

export interface WsClientMessage {
  type:
    | "subscribe_run"
    | "unsubscribe_run"
    | "subscribe_issue"
    | "unsubscribe_issue"
    | "subscribe_chat_session"
    | "unsubscribe_chat_session";
  run_id?: string;
  issue_id?: string;
  session_id?: string;
}

export type WsEventHandler<TPayload = unknown> = (
  payload: TPayload,
  raw: MessageEvent<string>,
) => void;

export interface WsClientOptions {
  baseUrl: string;
  getToken?: () => string | null | undefined;
  reconnectIntervalMs?: number;
  maxReconnectIntervalMs?: number;
}
