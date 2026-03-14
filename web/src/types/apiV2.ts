export type WorkItemStatus =
  | "open"
  | "accepted"
  | "queued"
  | "running"
  | "blocked"
  | "failed"
  | "done"
  | "cancelled"
  | "closed"
  | string;

export type WorkItemPriority = "low" | "medium" | "high" | "urgent";

export interface WorkItem {
  id: number;
  project_id?: number | null;
  resource_binding_id?: number | null;
  title: string;
  body: string;
  priority: WorkItemPriority;
  labels?: string[];
  depends_on?: number[];
  status: WorkItemStatus;
  metadata?: Record<string, unknown>;
  archived_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface CreateWorkItemRequest {
  project_id?: number;
  resource_binding_id?: number;
  title: string;
  body?: string;
  priority?: WorkItemPriority;
  labels?: string[];
  depends_on?: number[];
  metadata?: Record<string, unknown>;
}

export interface UpdateWorkItemRequest {
  title?: string;
  body?: string;
  status?: WorkItemStatus;
  priority?: WorkItemPriority;
  labels?: string[];
  project_id?: number;
  depends_on?: number[];
}

export interface Project {
  id: number;
  name: string;
  kind: "dev" | "general" | string;
  description?: string;
  metadata?: Record<string, string>;
  created_at: string;
  updated_at: string;
}

export type ActionType = "exec" | "gate" | "composite" | "plan" | string;

export type ActionStatus =
  | "pending"
  | "ready"
  | "running"
  | "waiting_gate"
  | "blocked"
  | "failed"
  | "done"
  | "cancelled"
  | string;

export interface Action {
  id: number;
  work_item_id: number;
  name: string;
  description?: string;
  depends_on?: number[];
  type: ActionType;
  status: ActionStatus;
  position: number;
  agent_role?: string;
  required_capabilities?: string[];
  acceptance_criteria?: string[];
  input?: string;
  timeout?: number;
  max_retries: number;
  retry_count: number;
  config?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export type RunStatus =
  | "created"
  | "running"
  | "succeeded"
  | "failed"
  | "cancelled"
  | string;

export type RunErrorKind =
  | "transient"
  | "permanent"
  | "need_help"
  | string;

export interface Run {
  id: number;
  action_id: number;
  work_item_id: number;
  status: RunStatus;
  agent_id?: string;
  agent_context_id?: number | null;
  briefing_snapshot?: string;
  deliverable_id?: number | null;
  input?: Record<string, unknown>;
  output?: Record<string, unknown>;
  error_message?: string;
  error_kind?: RunErrorKind;
  attempt: number;
  started_at?: string | null;
  finished_at?: string | null;
  created_at: string;
}

export type EventType =
  | "work_item.queued"
  | "work_item.started"
  | "work_item.completed"
  | "work_item.failed"
  | "work_item.cancelled"
  | "action.ready"
  | "action.started"
  | "action.completed"
  | "action.failed"
  | "action.blocked"
  | "run.created"
  | "run.started"
  | "run.succeeded"
  | "run.failed"
  | "run.agent_output"
  | "gate.passed"
  | "gate.rejected"
  | "chat.output"
  | string;

export interface Event {
  id: number;
  type: EventType;
  work_item_id?: number;
  action_id?: number;
  run_id?: number;
  data?: Record<string, unknown>;
  timestamp: string;
}

export interface RunWorkItemResponse {
  issue_id: number;
  status: "accepted" | string;
  message?: string;
}

export interface CancelWorkItemResponse {
  issue_id: number;
  status: "cancelled" | string;
}

export interface BootstrapPRWorkItemRequest {
  base_branch?: string;
  title?: string;
  body?: string;
}

export interface BootstrapPRWorkItemResponse {
  issue_id: number;
  implement_step_id: number;
  commit_push_step_id: number;
  open_pr_step_id: number;
  gate_step_id: number;
}

export interface CreateProjectRequest {
  name: string;
  kind?: "dev" | "general" | string;
  description?: string;
  metadata?: Record<string, string>;
}

export interface UpdateProjectRequest {
  name?: string;
  kind?: "dev" | "general" | string;
  description?: string;
  metadata?: Record<string, string>;
}

export type ResourceBindingKind = "git" | "local_fs" | "s3" | "http" | "webdav" | string;

export interface ResourceBinding {
  id: number;
  project_id: number;
  kind: ResourceBindingKind;
  uri: string;
  config?: Record<string, unknown>;
  label?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateResourceBindingRequest {
  kind: ResourceBindingKind;
  uri: string;
  config?: Record<string, unknown>;
  label?: string;
}

export interface UpdateResourceBindingRequest {
  kind?: ResourceBindingKind;
  uri?: string;
  config?: Record<string, unknown>;
  label?: string;
}

// ---------------------------------------------------------------------------
// Action Resources (per-action input/output resource declarations)
// ---------------------------------------------------------------------------

export type ActionResourceDirection = "input" | "output";

export interface ActionResource {
  id: number;
  action_id: number;
  resource_binding_id: number;
  direction: ActionResourceDirection;
  path: string;
  media_type?: string;
  description?: string;
  required: boolean;
  metadata?: Record<string, unknown>;
  created_at: string;
}

export interface CreateActionResourceRequest {
  resource_binding_id: number;
  direction: ActionResourceDirection;
  path: string;
  media_type?: string;
  description?: string;
  required?: boolean;
  metadata?: Record<string, unknown>;
}

export interface CreateActionRequest {
  name: string;
  type: "exec" | "gate" | "composite" | "plan";
  position?: number;
  agent_role?: string;
  required_capabilities?: string[];
  acceptance_criteria?: string[];
  timeout?: string;
  max_retries?: number;
  config?: Record<string, unknown>;
}

export interface GenerateActionsRequest {
  description: string;
}

export interface UpdateActionRequest {
  name?: string;
  type?: "exec" | "gate" | "composite" | "plan";
  position?: number;
  description?: string;
  agent_role?: string;
  required_capabilities?: string[];
  acceptance_criteria?: string[];
  timeout?: string;
  max_retries?: number;
  config?: Record<string, unknown>;
}

export interface DriverCapabilities {
  fs_read: boolean;
  fs_write: boolean;
  terminal: boolean;
}

export interface AgentDriver {
  id: string;
  launch_command: string;
  launch_args?: string[];
  env?: Record<string, string>;
  capabilities_max: DriverCapabilities;
}

export interface AgentProfileSession {
  reuse?: boolean;
  max_turns?: number;
  idle_ttl?: string;
}

export interface AgentProfileMCP {
  enabled?: boolean;
  tools?: string[];
}

export interface AgentProfile {
  id: string;
  name?: string;
  driver_id: string;
  role: "lead" | "worker" | "gate" | "support" | string;
  capabilities?: string[];
  actions_allowed?: string[];
  prompt_template?: string;
  skills?: string[];
  session?: AgentProfileSession;
  mcp?: AgentProfileMCP;
}

export interface ConfigOptionValue {
  value: string;
  name: string;
  description?: string;
  group_id?: string;
  group_name?: string;
}

export interface ConfigOption {
  id: string;
  name: string;
  description?: string;
  category?: string;
  type: "select" | string;
  current_value: string;
  options: ConfigOptionValue[];
}

export interface SessionMode {
  id: string;
  name: string;
  description?: string;
}

export interface SessionModeState {
  available_modes: SessionMode[];
  current_mode_id: string;
}

export interface SlashCommandInput {
  hint?: string;
}

export interface SlashCommand {
  name: string;
  description?: string;
  input?: SlashCommandInput;
}

export interface SkillMetadata {
  name: string;
  description: string;
}

export interface SkillInfo {
  name: string;
  has_skill_md: boolean;
  valid: boolean;
  metadata?: SkillMetadata;
  validation_errors?: string[];
  profiles_using?: string[];
}

export interface SkillDetail extends SkillInfo {
  skill_md: string;
}

export interface CreateSkillRequest {
  name: string;
  skill_md?: string;
}

export interface ImportGitHubSkillRequest {
  repo_url: string;
  skill_name: string;
}

export interface SchedulerStats {
  enabled: boolean;
  message?: string;
  stats?: Record<string, unknown>;
}

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
  message_count: number;
}

export interface ChatSessionDetail extends ChatSessionSummary {
  messages: ChatMessage[];
  available_commands?: SlashCommand[];
  config_options?: ConfigOption[];
  modes?: SessionModeState;
}

export interface CrystallizeChatSessionThreadRequest {
  thread_title?: string;
  thread_summary?: string;
  work_item_title?: string;
  work_item_body?: string;
  project_id?: number;
  participant_user_ids?: string[];
  create_work_item?: boolean;
  owner_id?: string;
}

export interface CrystallizeChatSessionThreadResponse {
  thread: Thread;
  work_item?: WorkItem;
  participants: ThreadParticipant[];
}

export interface ChatStatusResponse {
  session_id: string;
  status: "not_found" | "alive" | "running" | string;
}

export interface DeliverableAsset {
  name: string;
  uri: string;
  media_type?: string;
}

export interface Deliverable {
  id: number;
  run_id: number;
  action_id: number;
  work_item_id: number;
  result_markdown: string;
  metadata?: Record<string, unknown>;
  assets?: DeliverableAsset[];
  created_at: string;
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

export interface ProjectErrorRank {
  project_id: number;
  project_name: string;
  total_work_items: number;
  failed_work_items: number;
  failure_rate: number;
  failed_runs: number;
}

export interface ActionBottleneck {
  action_id: number;
  action_name: string;
  work_item_id: number;
  work_item_title: string;
  project_id?: number | null;
  avg_duration_s: number;
  max_duration_s: number;
  run_count: number;
  fail_count: number;
  retry_count: number;
  fail_rate: number;
}

export interface WorkItemDurationStat {
  work_item_id: number;
  work_item_title: string;
  project_id?: number | null;
  run_count: number;
  avg_duration_s: number;
  min_duration_s: number;
  max_duration_s: number;
  p50_duration_s: number;
}

export interface ErrorKindCount {
  error_kind: string;
  count: number;
  pct: number;
}

export interface FailureRecord {
  run_id: number;
  action_id: number;
  action_name: string;
  work_item_id: number;
  work_item_title: string;
  project_id?: number | null;
  project_name?: string;
  error_message: string;
  error_kind: string;
  attempt: number;
  duration_s: number;
  failed_at: string;
}

export interface StatusCount {
  status: string;
  count: number;
}

export interface AnalyticsSummary {
  project_errors: ProjectErrorRank[];
  bottlenecks: ActionBottleneck[];
  duration_stats: WorkItemDurationStat[];
  error_breakdown: ErrorKindCount[];
  recent_failures: FailureRecord[];
  status_distribution: StatusCount[];
}

export interface AnalyticsFilter {
  project_id?: number;
  since?: string;
  until?: string;
  limit?: number;
}

// --- Usage / Token Tracking ---

export interface UsageRecord {
  id: number;
  run_id: number;
  work_item_id: number;
  action_id: number;
  project_id?: number | null;
  agent_id: string;
  profile_id?: string;
  model_id?: string;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens?: number;
  cache_write_tokens?: number;
  reasoning_tokens?: number;
  total_tokens: number;
  duration_ms?: number;
  created_at: string;
}

export interface ProjectUsageSummary {
  project_id: number;
  project_name: string;
  run_count: number;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  reasoning_tokens: number;
  total_tokens: number;
}

export interface AgentUsageSummary {
  agent_id: string;
  project_id?: number | null;
  project_name?: string;
  run_count: number;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  reasoning_tokens: number;
  total_tokens: number;
}

export interface ProfileUsageSummary {
  profile_id: string;
  agent_id: string;
  project_id?: number | null;
  project_name?: string;
  run_count: number;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  reasoning_tokens: number;
  total_tokens: number;
}

export interface UsageTotalSummary {
  run_count: number;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  reasoning_tokens: number;
  total_tokens: number;
}

export interface UsageAnalyticsSummary {
  totals: UsageTotalSummary;
  by_project: ProjectUsageSummary[];
  by_agent: AgentUsageSummary[];
  by_profile: ProfileUsageSummary[];
}

// Cron types

export interface CronStatus {
  work_item_id: number;
  enabled: boolean;
  is_template: boolean;
  schedule?: string;
  max_instances?: number;
  last_triggered?: string;
}

export interface SetupCronRequest {
  schedule: string;
  max_instances?: number;
}

// --- DAG Templates ---

export interface DAGTemplateAction {
  name: string;
  description?: string;
  type: "exec" | "gate" | "composite" | string;
  depends_on?: string[];
  agent_role?: string;
  required_capabilities?: string[];
  acceptance_criteria?: string[];
  profile_id?: string;
}

export interface DAGTemplate {
  id: number;
  name: string;
  description?: string;
  project_id?: number | null;
  tags?: string[];
  metadata?: Record<string, string>;
  actions: DAGTemplateAction[];
  created_at: string;
  updated_at: string;
}

export interface CreateDAGTemplateRequest {
  name: string;
  description?: string;
  project_id?: number;
  tags?: string[];
  metadata?: Record<string, string>;
  actions: DAGTemplateAction[];
}

export interface UpdateDAGTemplateRequest {
  name?: string;
  description?: string;
  project_id?: number;
  tags?: string[];
  metadata?: Record<string, string>;
  actions?: DAGTemplateAction[];
}

export interface SaveWorkItemAsTemplateRequest {
  name?: string;
  description?: string;
  tags?: string[];
  metadata?: Record<string, string>;
}

export interface CreateWorkItemFromTemplateRequest {
  title?: string;
  project_id?: number;
  metadata?: Record<string, unknown>;
}

export interface CreateWorkItemFromTemplateResponse {
  issue: WorkItem;
  steps: Action[];
}

// --- Git Tags ---

export interface GitCommitEntry {
  sha: string;
  short: string;
  message: string;
  author: string;
  timestamp: string;
}

export interface GitTagEntry {
  name: string;
  sha: string;
  message?: string;
  timestamp?: string;
}

export interface CreateGitTagRequest {
  name: string;
  ref?: string;
  message?: string;
  push?: boolean;
}

export interface CreateGitTagResponse {
  name: string;
  sha: string;
  pushed: boolean;
  push_error?: string;
}

export interface PushGitTagRequest {
  name: string;
}

export interface PushGitTagResponse {
  name: string;
  pushed: boolean;
}

// ---------------------------------------------------------------------------
// Thread (multi-participant discussion)
// ---------------------------------------------------------------------------

export type ThreadStatus = "active" | "closed" | "archived" | string;

export interface Thread {
  id: number;
  title: string;
  status: ThreadStatus;
  owner_id?: string;
  summary?: string;
  metadata?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface CreateThreadRequest {
  title: string;
  owner_id?: string;
  summary?: string;
  metadata?: Record<string, unknown>;
}

export interface UpdateThreadRequest {
  title?: string;
  status?: string;
  owner_id?: string;
  summary?: string;
  metadata?: Record<string, unknown>;
}

export interface ThreadMessage {
  id: number;
  thread_id: number;
  sender_id: string;
  role: string;
  content: string;
  reply_to_msg_id?: number;
  metadata?: Record<string, unknown>;
  created_at: string;
}

export interface CreateThreadMessageRequest {
  sender_id?: string;
  role?: string;
  content: string;
  reply_to_msg_id?: number;
  target_agent_id?: string;
  metadata?: Record<string, unknown>;
}

export interface ThreadParticipant {
  id: number;
  thread_id: number;
  user_id: string;
  role: string;
  joined_at: string;
}

export interface AddThreadParticipantRequest {
  user_id: string;
  role?: string;
}

// ---------------------------------------------------------------------------
// Thread-WorkItem Links
// ---------------------------------------------------------------------------

export interface ThreadWorkItemLink {
  id: number;
  thread_id: number;
  work_item_id: number;
  relation_type: string;
  is_primary: boolean;
  created_at: string;
}

export interface CreateThreadWorkItemLinkRequest {
  work_item_id: number;
  relation_type?: string;
  is_primary?: boolean;
}

// ---------------------------------------------------------------------------
// Thread Agent Sessions
// ---------------------------------------------------------------------------

export type ThreadAgentSessionStatus =
  | "joining"
  | "booting"
  | "active"
  | "paused"
  | "left"
  | "failed"
  | string;

export interface ThreadAgentSession {
  id: number;
  thread_id: number;
  agent_profile_id: string;
  acp_session_id: string;
  status: ThreadAgentSessionStatus;
  turn_count: number;
  total_input_tokens: number;
  total_output_tokens: number;
  progress_summary?: string;
  metadata?: Record<string, unknown>;
  joined_at: string;
  last_active_at: string;
}

// ---------------------------------------------------------------------------
// Feature Manifest
// ---------------------------------------------------------------------------

export type FeatureStatus = "pending" | "pass" | "fail" | "skipped";

export interface FeatureManifest {
  id: number;
  project_id: number;
  version: number;
  summary?: string;
  metadata?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface FeatureEntry {
  id: number;
  manifest_id: number;
  key: string;
  description: string;
  status: FeatureStatus;
  work_item_id?: number | null;
  action_id?: number | null;
  tags?: string[];
  metadata?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface FeatureManifestSummary {
  manifest_id: number;
  version: number;
  pass: number;
  fail: number;
  pending: number;
  skipped: number;
  total: number;
}

export interface FeatureManifestSnapshot {
  manifest: FeatureManifest;
  entries: FeatureEntry[];
}

// ---------------------------------------------------------------------------
// Work Item Attachments
// ---------------------------------------------------------------------------

export interface WorkItemAttachment {
  id: number;
  issue_id: number;
  file_name: string;
  mime_type: string;
  size: number;
  created_at: string;
}

// ---------------------------------------------------------------------------
// Notifications
// ---------------------------------------------------------------------------

export type NotificationLevel = "info" | "success" | "warning" | "error";

export type NotificationChannel = "browser" | "in_app" | "webhook" | "email";

export interface Notification {
  id: number;
  level: NotificationLevel;
  title: string;
  body?: string;
  category?: string;
  action_url?: string;
  project_id?: number | null;
  work_item_id?: number | null;
  run_id?: number | null;
  channels?: NotificationChannel[];
  read: boolean;
  read_at?: string | null;
  created_at: string;
}

export interface CreateNotificationRequest {
  level?: NotificationLevel;
  title: string;
  body?: string;
  category?: string;
  action_url?: string;
  project_id?: number;
  work_item_id?: number;
  run_id?: number;
  channels?: NotificationChannel[];
}

export interface UnreadCountResponse {
  count: number;
}


// ---------------------------------------------------------------------------
// Backward compatibility aliases
// ---------------------------------------------------------------------------

export type Issue = WorkItem;
export type IssueStatus = WorkItemStatus;
export type IssuePriority = WorkItemPriority;
export type CreateIssueRequest = CreateWorkItemRequest;
export type UpdateIssueRequest = UpdateWorkItemRequest;

export type Step = Action;
export type StepType = ActionType;
export type StepStatus = ActionStatus;
export type CreateStepRequest = CreateActionRequest;
export type UpdateStepRequest = UpdateActionRequest;
export type GenerateStepsRequest = GenerateActionsRequest;

export type Execution = Run;
export type ExecutionStatus = RunStatus;
export type ExecutionErrorKind = RunErrorKind;

export type Artifact = Deliverable;
export type ArtifactAsset = DeliverableAsset;

export type StepBottleneck = ActionBottleneck;
export type IssueDurationStat = WorkItemDurationStat;

export type DAGTemplateStep = DAGTemplateAction;

export type RunIssueResponse = RunWorkItemResponse;
export type CancelIssueResponse = CancelWorkItemResponse;
export type BootstrapPRIssueRequest = BootstrapPRWorkItemRequest;
export type BootstrapPRIssueResponse = BootstrapPRWorkItemResponse;
export type SaveIssueAsTemplateRequest = SaveWorkItemAsTemplateRequest;
export type CreateIssueFromTemplateRequest = CreateWorkItemFromTemplateRequest;
export type CreateIssueFromTemplateResponse = CreateWorkItemFromTemplateResponse;
export type IssueAttachment = WorkItemAttachment;
