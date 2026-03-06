import type {
  ChatMessage,
  ChatSession,
  FailurePolicy,
  GitHubConnectionStatus,
  Issue,
  IssueStatus,
  Project,
  WorkflowProfile,
  WorkflowProfileType,
  WorkflowRun,
} from "./workflow";

export interface CreateProjectRequest {
  name: string;
  repo_path: string;
  default_branch?: string;
  github?: {
    owner?: string;
    repo?: string;
  };
}

export type ProjectSourceType = "local_path" | "local_new" | "github_clone";

export interface CreateProjectCreateRequest {
  name: string;
  source_type: ProjectSourceType;
  repo_path?: string;
  remote_url?: string;
  ref?: string;
}

export interface CreateProjectCreateRequestResponse {
  request_id: string;
}

export interface GetProjectCreateRequestResponse {
  request_id: string;
  status: "pending" | "running" | "succeeded" | "failed" | string;
  source_type?: ProjectSourceType;
  project_id?: string;
  progress?: number;
  message?: string;
  error?: string;
}

export interface CreateChatRequest {
  message: string;
  session_id?: string;
}

export interface CreateChatResponse {
  session_id: string;
  status: "accepted" | "running" | "queued" | string;
}

export interface CancelChatResponse {
  session_id: string;
  status: "cancelling" | "cancelled" | string;
}

export interface ChatRunEvent {
  id: number;
  session_id: string;
  project_id: string;
  event_type: string;
  update_type: string;
  payload: Record<string, unknown>;
  created_at: string;
}

export interface ChatEventsPageQuery {
  cursor?: string;
  limit?: number;
}

export interface ChatEventsPage {
  session_id: string;
  project_id: string;
  updated_at: string;
  messages: ChatMessage[];
  events: ChatRunEvent[];
  next_cursor?: string;
}

export interface ChatEventGroupResponse {
  session_id: string;
  project_id: string;
  group_id: string;
  events: ChatRunEvent[];
}

export interface CreateIssueRequest {
  session_id: string;
  name?: string;
  fail_policy?: FailurePolicy;
  auto_merge?: boolean;
}

export interface CreateIssueFromFilesRequest {
  session_id: string;
  name?: string;
  fail_policy?: FailurePolicy;
  auto_merge?: boolean;
  file_paths: string[];
}

export interface FileEntry {
  path: string;
  name: string;
  type: "file" | "dir";
  git_status: string;
}

export interface RepoTreeResponse {
  dir: string;
  items: FileEntry[];
}

export interface RepoStatusResponse {
  items: FileEntry[];
}

export interface RepoDiffResponse {
  file_path: string;
  diff: string;
}

export interface SubmitIssueReviewResponse {
  status: IssueStatus | string;
}

export type IssueRejectFeedbackCategory =
  | "cycle"
  | "missing_node"
  | "bad_granularity"
  | "coverage_gap"
  | "other";

export interface IssueRejectFeedback {
  category: IssueRejectFeedbackCategory;
  detail: string;
  expected_direction?: string;
}

export interface IssueActionRequest {
  action: "approve" | "reject" | "abort" | "abandon";
  feedback?: IssueRejectFeedback;
}

export interface IssueActionResponse {
  status: IssueStatus | string;
}

export interface SetIssueAutoMergeRequest {
  auto_merge: boolean;
}

export interface SetIssueAutoMergeResponse {
  status: IssueStatus | string;
  auto_merge: boolean;
}

export type ListProjectsResponse = Project[] | null;

export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  offset: number;
}

export type ApiRun = WorkflowRun & {
  conclusion?: string;
  github?: {
    connection_status?: GitHubConnectionStatus;
    issue_number?: number;
    issue_url?: string;
    pr_number?: number;
    pr_url?: string;
  };
};

export interface ApiIssue extends Issue {
  github?: {
    issue_number?: number;
    issue_url?: string;
  };
}

export interface ApiWorkflowProfile extends WorkflowProfile {
  type: WorkflowProfileType;
}

export type ListRunsResponse = PaginatedResponse<ApiRun>;
export type ListIssuesResponse = PaginatedResponse<ApiIssue>;
export type ListWorkflowProfilesResponse =
  PaginatedResponse<ApiWorkflowProfile>;

export interface IssueDagNode {
  id: string;
  title: string;
  status: IssueStatus;
  run_id: string;
}

export interface IssueDagEdge {
  from: string;
  to: string;
}

export interface IssueDagStats {
  total: number;
  pending: number;
  ready: number;
  running: number;
  done: number;
  failed: number;
}

export interface IssueDagResponse {
  nodes: IssueDagNode[];
  edges: IssueDagEdge[];
  stats: IssueDagStats;
}

export interface IssueReviewIssue {
  severity: string;
  issue_id: string;
  description: string;
  suggestion: string;
}

export interface IssueProposedFix {
  issue_id?: string;
  description: string;
  suggestion?: string;
}

export interface IssueReviewRecord {
  id: number;
  issue_id: string;
  round: number;
  reviewer: string;
  verdict: string;
  summary?: string;
  raw_output?: string;
  issues: IssueReviewIssue[];
  fixes: IssueProposedFix[];
  score?: number;
  created_at: string;
}

export interface IssueChangeRecord {
  id: string;
  issue_id: string;
  field: string;
  old_value: string;
  new_value: string;
  reason: string;
  changed_by: string;
  created_at: string;
}

export interface IssueTimelineRefs {
  issue_id: string;
  run_id?: string;
  stage?: string;
}

export interface IssueTimelineEntry {
  event_id: string;
  kind:
    | "review"
    | "change"
    | "action"
    | "checkpoint"
    | "log"
    | "audit"
    | string;
  created_at: string;
  actor_type: "human" | "agent" | "system" | string;
  actor_name: string;
  actor_avatar_seed: string;
  title: string;
  body: string;
  status: "success" | "failed" | "running" | "info" | "warning" | string;
  refs: IssueTimelineRefs;
  meta: Record<string, unknown>;
}

export interface ListIssueTimelineQuery {
  limit?: number;
  offset?: number;
}

export type ListIssueTimelineResponse = PaginatedResponse<IssueTimelineEntry>;

export interface AdminAuditLogItem {
  id: number;
  project_id?: string;
  issue_id?: string;
  run_id: string;
  stage?: string;
  action: string;
  message: string;
  source: string;
  user_id: string;
  created_at: string;
}

export interface ApiStatsResponse {
  total_Runs: number;
  active_Runs: number;
  success_rate: number;
  avg_duration: string;
  tokens_used: {
    claude: number;
    codex: number;
  };
}

export interface RunEvent {
  id: number;
  run_id: string;
  project_id: string;
  issue_id?: string;
  event_type: string;
  stage?: string;
  agent?: string;
  data?: Record<string, string>;
  error?: string;
  created_at: string;
}

export interface ListRunEventsResponse {
  items: RunEvent[];
  total: number;
}

export interface RunCheckpoint {
  run_id: string;
  stage_name: string;
  status:
    | "in_progress"
    | "success"
    | "failed"
    | "skipped"
    | "invalidated"
    | string;
  agent_used: string;
  agent_session_id?: string;
  tokens_used: number;
  retry_count: number;
  error?: string;
  started_at: string;
  finished_at: string;
}

export interface ListRunCheckpointsResponse {
  items: RunCheckpoint[];
  total: number;
}

export interface StageSessionStatus {
  alive: boolean;
  session_id: string;
}

export interface ChatSessionStatus {
  alive: boolean;
  running: boolean;
}

export interface WakeStageSessionResponse {
  session_id: string;
}

export type ListChatsResponse = ChatSession[];
export type ListChatRunEventsResponse = ChatEventsPage;
export type GetChatResponse = ChatSession;
export type CreateIssueResponse = ApiIssue;
export type ListAdminAuditLogResponse = PaginatedResponse<AdminAuditLogItem>;
