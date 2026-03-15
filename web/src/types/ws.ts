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

// System-wide events (preflight, restart countdown, etc.)
export type SystemEventName =
  | "preflight_start"
  | "preflight_step"
  | "preflight_pass"
  | "preflight_fail"
  | "restart_countdown"
  | "restart";

export interface RuntimeConfigReloadedPayload {
  version?: number;
  loaded_at?: string;
  driver_count?: number;
  profile_count?: number;
}

export interface SystemEventPayload {
  event: SystemEventName;
  timestamp: string;
  data: {
    message?: string;
    step?: number;
    total?: number;
    name?: string;
    status?: string;
    duration?: string;
    seconds?: number;
    success?: boolean;
    commit_sha?: string;
    [key: string]: unknown;
  };
}

export interface SystemEventEnvelope extends WsEnvelope<SystemEventPayload> {
  type: "system_event";
}

// Notification events (real-time push from server)
export type NotificationEventType =
  | "notification.created"
  | "notification.read"
  | "notification.all_read";

export interface NotificationEventPayload {
  notification?: {
    id: number;
    level: string;
    title: string;
    body?: string;
    category?: string;
    action_url?: string;
    project_id?: number | null;
    issue_id?: number | null;
    exec_id?: number | null;
    channels?: string[];
    read: boolean;
    created_at: string;
    [key: string]: unknown;
  };
  notification_id?: number;
  [key: string]: unknown;
}

export interface NotificationEventEnvelope extends WsEnvelope<NotificationEventPayload> {
  type: NotificationEventType;
}

export interface WsClientMessage {
  type:
    | "chat.send"
    | "thread.send"
    | "subscribe_thread"
    | "unsubscribe_thread"
    | "subscribe_run"
    | "unsubscribe_run"
    | "subscribe_issue"
    | "unsubscribe_issue"
    | "subscribe_chat_session"
    | "unsubscribe_chat_session";
  request_id?: string;
  run_id?: string;
  issue_id?: string;
  session_id?: string;
  thread_id?: number;
  message?: string;
  sender_id?: string;
  target_agent_id?: string;
  reply_to_msg_id?: number;
  attachments?: ChatAttachment[];
  work_dir?: string;
  project_id?: number;
  project_name?: string;
  profile_id?: string;
  driver_id?: string;
}

export interface ChatAttachment {
  name: string;
  mime_type: string;
  /** Base64-encoded content. */
  data: string;
}

// Thread WebSocket event types
export type ThreadEventType =
  | "thread.message"
  | "thread.agent_joined"
  | "thread.agent_left"
  | "thread.agent_output"
  | "thread.agent_booted"
  | "thread.agent_failed"
  | "thread.agent_thinking"
  | "thread.track.created"
  | "thread.track.updated"
  | "thread.track.state_changed"
  | "thread.track.review_approved"
  | "thread.track.review_rejected"
  | "thread.track.materialized"
  | "thread.track.execution_confirmed";

export interface ThreadEventPayload {
  thread_id?: number;
  message_id?: number;
  message?: string;
  content?: string;
  sender_id?: string;
  profile_id?: string;
  target_agent_id?: string;
  reply_to_msg_id?: number;
  track_id?: number;
  work_item_id?: number;
  status?: string;
  title?: string;
  objective?: string;
  track?: Record<string, unknown>;
  role?: string;
  error?: string;
  timestamp?: string;
  [key: string]: unknown;
}

export interface ThreadEventEnvelope extends WsEnvelope<ThreadEventPayload> {
  type: ThreadEventType;
}

// Thread WebSocket response types (server → client)
export type ThreadResponseType =
  | "thread.ack"
  | "thread.error"
  | "thread.subscribed"
  | "thread.unsubscribed";

export interface ThreadAckPayload {
  request_id?: string;
  thread_id: number;
  status: string;
}

export interface ThreadSubscriptionPayload {
  thread_id: number;
  status: string;
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
