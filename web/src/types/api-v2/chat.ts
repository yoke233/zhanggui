import type { ConfigOption, SessionModeState, SlashCommand } from "./agent-admin";

export interface ChatAttachment {
  name: string;
  mime_type: string;
  /** Base64-encoded content. */
  data: string;
}

export interface ChatRequest {
  session_id?: string;
  message: string;
  attachments?: ChatAttachment[];
  work_dir?: string;
  project_id?: number;
  project_name?: string;
  profile_id?: string;
  driver_id?: string;
}

export interface ChatResponse {
  session_id: string;
  reply: string;
  ws_path?: string;
}

export interface ChatMessage {
  role: "user" | "assistant" | string;
  content: string;
  time: string;
}

export interface GitStats {
  additions: number;
  deletions: number;
  files_changed: number;
  merged?: boolean;
  head_sha?: string;
  pr_url?: string;
  pr_number?: number;
  pr_state?: string;
}

export interface ChatSessionSummary {
  session_id: string;
  title?: string;
  work_dir?: string;
  branch?: string;
  ws_path?: string;
  project_id?: number;
  project_name?: string;
  profile_id?: string;
  profile_name?: string;
  driver_id?: string;
  created_at: string;
  updated_at: string;
  status: "running" | "alive" | "closed" | string;
  archived?: boolean;
  message_count: number;
  git?: GitStats;
}

export interface ChatSessionDetail extends ChatSessionSummary {
  messages: ChatMessage[];
  available_commands?: SlashCommand[];
  config_options?: ConfigOption[];
  modes?: SessionModeState;
}

export interface ChatStatusResponse {
  session_id: string;
  status: "not_found" | "alive" | "running" | string;
}

export interface StatsResponse {
  total_work_items: number;
  active_work_items: number;
  success_rate: number;
  avg_duration: string;
}

export interface AdminSystemEventRequest {
  event: string;
  data?: Record<string, unknown>;
}

export interface AdminSystemEventResponse {
  status: string;
}

// Analytics types
