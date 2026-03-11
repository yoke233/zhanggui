export type FlowStatus =
  | "pending"
  | "queued"
  | "running"
  | "blocked"
  | "failed"
  | "done"
  | "cancelled"
  | string;

export interface Flow {
  id: number;
  project_id?: number | null;
  name: string;
  status: FlowStatus;
  parent_step_id?: number | null;
  metadata?: Record<string, string>;
  archived_at?: string | null;
  created_at: string;
  updated_at: string;
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

export type StepType = "exec" | "gate" | "composite" | string;

export type StepStatus =
  | "pending"
  | "ready"
  | "running"
  | "waiting_gate"
  | "blocked"
  | "failed"
  | "done"
  | "cancelled"
  | string;

export interface Step {
  id: number;
  flow_id: number;
  name: string;
  description?: string;
  type: StepType;
  status: StepStatus;
  depends_on?: number[];
  sub_flow_id?: number | null;
  agent_role?: string;
  required_capabilities?: string[];
  acceptance_criteria?: string[];
  timeout?: number;
  max_retries: number;
  retry_count: number;
  config?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export type ExecutionStatus =
  | "created"
  | "running"
  | "succeeded"
  | "failed"
  | "cancelled"
  | string;

export type ExecutionErrorKind =
  | "transient"
  | "permanent"
  | "need_help"
  | string;

export interface Execution {
  id: number;
  step_id: number;
  flow_id: number;
  status: ExecutionStatus;
  agent_id?: string;
  agent_context_id?: number | null;
  briefing_snapshot?: string;
  artifact_id?: number | null;
  input?: Record<string, unknown>;
  output?: Record<string, unknown>;
  error_message?: string;
  error_kind?: ExecutionErrorKind;
  attempt: number;
  started_at?: string | null;
  finished_at?: string | null;
  created_at: string;
}

export type EventType =
  | "flow.queued"
  | "flow.started"
  | "flow.completed"
  | "flow.failed"
  | "flow.cancelled"
  | "step.ready"
  | "step.started"
  | "step.completed"
  | "step.failed"
  | "step.blocked"
  | "exec.created"
  | "exec.started"
  | "exec.succeeded"
  | "exec.failed"
  | "exec.agent_output"
  | "gate.passed"
  | "gate.rejected"
  | "chat.output"
  | string;

export interface Event {
  id: number;
  type: EventType;
  flow_id?: number;
  step_id?: number;
  exec_id?: number;
  data?: Record<string, unknown>;
  timestamp: string;
}

export interface RunFlowResponse {
  flow_id: number;
  status: "accepted" | string;
  message?: string;
}

export interface CancelFlowResponse {
  flow_id: number;
  status: "cancelled" | string;
}

export interface BootstrapPRFlowRequest {
  base_branch?: string;
  title?: string;
  body?: string;
}

export interface BootstrapPRFlowResponse {
  flow_id: number;
  implement_step_id: number;
  commit_push_step_id: number;
  open_pr_step_id: number;
  gate_step_id: number;
}

export interface CreateFlowRequest {
  project_id?: number;
  name: string;
  metadata?: Record<string, string>;
}

export type IssueStatus = "open" | "accepted" | "in_progress" | "done" | "closed";
export type IssuePriority = "low" | "medium" | "high" | "urgent";

export interface Issue {
  id: number;
  project_id?: number;
  title: string;
  body: string;
  status: IssueStatus;
  priority: IssuePriority;
  labels?: string[];
  flow_id?: number;
  metadata?: Record<string, string>;
  created_at: string;
  updated_at: string;
}

export interface CreateIssueRequest {
  project_id?: number;
  title: string;
  body?: string;
  priority?: IssuePriority;
  labels?: string[];
  metadata?: Record<string, string>;
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

export interface ResourceBinding {
  id: number;
  project_id: number;
  kind: string;
  uri: string;
  config?: Record<string, unknown>;
  label?: string;
  created_at: string;
}

export interface CreateResourceBindingRequest {
  kind: string;
  uri: string;
  config?: Record<string, unknown>;
  label?: string;
}

export interface CreateStepRequest {
  name: string;
  type: "exec" | "gate" | "composite";
  depends_on?: number[];
  agent_role?: string;
  required_capabilities?: string[];
  acceptance_criteria?: string[];
  timeout?: string;
  max_retries?: number;
  config?: Record<string, unknown>;
}

export interface GenerateStepsRequest {
  description: string;
}

export interface UpdateStepRequest {
  name?: string;
  type?: "exec" | "gate" | "composite";
  depends_on?: number[];
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
  assign_when: string;
  version: number;
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

export interface ChatRequest {
  session_id?: string;
  message: string;
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
}

export interface ChatStatusResponse {
  session_id: string;
  status: "not_found" | "alive" | "running" | string;
}

export interface ArtifactAsset {
  name: string;
  uri: string;
  media_type?: string;
}

export interface Artifact {
  id: number;
  execution_id: number;
  step_id: number;
  flow_id: number;
  result_markdown: string;
  metadata?: Record<string, unknown>;
  assets?: ArtifactAsset[];
  created_at: string;
}

export type ContextRefType =
  | "flow_summary"
  | "project_brief"
  | "upstream_artifact"
  | "agent_memory"
  | string;

export interface ContextRef {
  type: ContextRefType;
  ref_id: number;
  label?: string;
  inline?: string;
}

export interface Briefing {
  id: number;
  step_id: number;
  objective: string;
  context_refs?: ContextRef[];
  constraints?: string[];
  created_at: string;
}

export interface StatsResponse {
  total_flows: number;
  active_flows: number;
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
  total_flows: number;
  failed_flows: number;
  failure_rate: number;
  failed_execs: number;
}

export interface StepBottleneck {
  step_id: number;
  step_name: string;
  flow_id: number;
  flow_name: string;
  project_id?: number | null;
  avg_duration_s: number;
  max_duration_s: number;
  exec_count: number;
  fail_count: number;
  retry_count: number;
  fail_rate: number;
}

export interface FlowDurationStat {
  flow_id: number;
  flow_name: string;
  project_id?: number | null;
  exec_count: number;
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
  exec_id: number;
  step_id: number;
  step_name: string;
  flow_id: number;
  flow_name: string;
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
  bottlenecks: StepBottleneck[];
  duration_stats: FlowDurationStat[];
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

// Cron types

export interface CronStatus {
  flow_id: number;
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

export interface DAGTemplateStep {
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
  steps: DAGTemplateStep[];
  created_at: string;
  updated_at: string;
}

export interface CreateDAGTemplateRequest {
  name: string;
  description?: string;
  project_id?: number;
  tags?: string[];
  metadata?: Record<string, string>;
  steps: DAGTemplateStep[];
}

export interface UpdateDAGTemplateRequest {
  name?: string;
  description?: string;
  project_id?: number;
  tags?: string[];
  metadata?: Record<string, string>;
  steps?: DAGTemplateStep[];
}

export interface SaveFlowAsTemplateRequest {
  name?: string;
  description?: string;
  tags?: string[];
  metadata?: Record<string, string>;
}

export interface CreateFlowFromTemplateRequest {
  name?: string;
  project_id?: number;
  metadata?: Record<string, string>;
}

export interface CreateFlowFromTemplateResponse {
  flow: Flow;
  steps: Step[];
}
