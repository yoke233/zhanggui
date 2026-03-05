export type RunStatus =
  | "queued"
  | "in_progress"
  | "action_required"
  | "completed";

export type IssueState = "open" | "closed";
export type FailurePolicy = "block" | "skip" | "human";
export type IssueStatus =
  | "draft"
  | "reviewing"
  | "queued"
  | "ready"
  | "executing"
  | "merging"
  | "done"
  | "failed"
  | "superseded"
  | "abandoned";

export type WorkflowProfileType = "normal" | "strict" | "fast_release";
export type WorkflowRunStatus =
  | "queued"
  | "in_progress"
  | "action_required"
  | "completed";

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
  default_branch?: string;
  created_at: string;
  updated_at: string;
}

export interface Run {
  id: string;
  project_id: string;
  name: string;
  description: string;
  template: string;
  status: RunStatus;
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

export interface Issue {
  id: string;
  project_id: string;
  session_id: string;
  title: string;
  body: string;
  labels: string[];
  milestone_id: string;
  attachments: string[];
  depends_on: string[];
  blocks: string[];
  priority: number;
  template: string;
  auto_merge: boolean;
  state: IssueState;
  status: IssueStatus;
  run_id: string;
  version: number;
  superseded_by: string;
  external_id: string;
  fail_policy: FailurePolicy;
  created_at: string;
  updated_at: string;
  closed_at?: string;
  github?: GitHubRef;
}

export interface WorkflowProfile {
  type: WorkflowProfileType;
  sla_minutes: number;
  reviewer_count: number;
  description: string;
}

export interface WorkflowRun {
  id: string;
  project_id: string;
  issue_id?: string;
  profile: WorkflowProfileType;
  status: WorkflowRunStatus;
  error?: string;
  started_at?: string;
  finished_at?: string;
  created_at: string;
  updated_at: string;
}
