import type { ChatSessionSummary } from "@/types/apiV2";

export type SessionRecord = ChatSessionSummary;

export interface ChatAttachmentView {
  name: string;
  mime_type: string;
  data: string;
}

export interface ChatMessageView {
  id: string;
  role: "user" | "assistant";
  content: string;
  time: string;
  at: string;
  attachments?: ChatAttachmentView[];
}

export type ChatFeedItem =
  | { kind: "message"; data: ChatMessageView }
  | { kind: "thought"; data: ChatActivityView }
  | { kind: "tool_call"; data: ChatActivityView };

export type ChatFeedEntry =
  | { type: "message"; item: ChatFeedItem & { kind: "message" } }
  | { type: "thought"; item: ChatFeedItem & { kind: "thought" } }
  | { type: "tool_group"; id: string; items: (ChatFeedItem & { kind: "tool_call" })[] };

export interface RealtimeChatOutputPayload {
  session_id?: string;
  type?: string;
  content?: string;
  title?: string;
  tool_call_id?: string;
  stderr?: string;
  exit_code?: number;
  usage_size?: number;
  usage_used?: number;
}

export interface RealtimeChatAckPayload {
  request_id?: string;
  session_id?: string;
  ws_path?: string;
  status?: string;
}

export interface RealtimeChatErrorPayload {
  request_id?: string;
  session_id?: string;
  error?: string;
  code?: string;
}

export interface ChatActivityView {
  id: string;
  type: "agent_thought" | "tool_call" | "usage_update" | "agent_message";
  title: string;
  detail?: string;
  time: string;
  at: string;
  status?: "running" | "completed" | "failed";
  toolCallId?: string;
  usageSize?: number;
  usageUsed?: number;
}

export interface ChatEventListItem {
  id: string;
  at: string;
  time: string;
  label: string;
  rawType: string;
  summary?: string;
  detail?: string;
  raw?: string;
  tone: "default" | "success" | "warning" | "danger";
}

export interface SessionGroup {
  key: string;
  label: string;
  updatedAt: string;
  sessions: SessionRecord[];
}

export interface LeadDriverOption {
  key: string;
  label: string;
  driverId: string;
}

export interface PendingMessageView {
  sessionId: string;
  content: string;
}

export const UNKNOWN_PROJECT_GROUP = "project:unknown";
export const EMPTY_PROFILE_VALUE = "__empty_profile__";
