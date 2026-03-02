export type PipelineStatus =
  | "created"
  | "running"
  | "waiting_human"
  | "paused"
  | "done"
  | "failed"
  | "aborted";

export type TaskPlanStatus =
  | "draft"
  | "reviewing"
  | "approved"
  | "waiting_human"
  | "executing"
  | "partially_done"
  | "done"
  | "failed"
  | "abandoned";

export type TaskItemStatus =
  | "pending"
  | "ready"
  | "running"
  | "done"
  | "failed"
  | "skipped"
  | "blocked_by_failure";

export type GitHubConnectionStatus = "connected" | "degraded" | "disconnected";

export interface GitHubRef {
  connection_status?: GitHubConnectionStatus;
  issue_number?: number;
  issue_url?: string;
  pr_number?: number;
  pr_url?: string;
}

export interface Project {
  id: string;
  name: string;
  repo_path: string;
  github_owner?: string;
  github_repo?: string;
  created_at: string;
  updated_at: string;
}

export interface Pipeline {
  id: string;
  project_id: string;
  name: string;
  description: string;
  template: string;
  status: PipelineStatus;
  current_stage: string;
  artifacts: Record<string, string>;
  config: Record<string, unknown>;
  branch_name: string;
  worktree_path: string;
  error_message?: string;
  max_total_retries: number;
  total_retries: number;
  run_count?: number;
  last_error_type?: string;
  queued_at?: string;
  last_heartbeat_at?: string;
  started_at: string;
  finished_at: string;
  created_at: string;
  updated_at: string;
  github?: GitHubRef;
}

export interface ChatMessage {
  role: "user" | "assistant";
  content: string;
  time: string;
}

export interface ChatSession {
  id: string;
  project_id: string;
  messages: ChatMessage[];
  created_at: string;
  updated_at: string;
}

export interface TaskItem {
  id: string;
  plan_id: string;
  title: string;
  description: string;
  labels: string[];
  depends_on: string[];
  template: string;
  pipeline_id: string;
  external_id: string;
  status: TaskItemStatus;
  created_at: string;
  updated_at: string;
  github?: GitHubRef;
}

export interface TaskPlan {
  id: string;
  project_id: string;
  session_id: string;
  name: string;
  status: TaskPlanStatus;
  wait_reason: string;
  tasks: TaskItem[];
  source_files?: string[];
  file_contents?: Record<string, string>;
  fail_policy: "block" | "skip" | "human";
  review_round: number;
  created_at: string;
  updated_at: string;
}
