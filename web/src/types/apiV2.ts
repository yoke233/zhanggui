export type FlowStatus =
  | "pending"
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
  | "gate.passed"
  | "gate.rejected"
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

export interface CreateFlowRequest {
  project_id?: number;
  name: string;
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

export interface ChatRequest {
  session_id?: string;
  message: string;
  work_dir?: string;
}

export interface ChatResponse {
  session_id: string;
  reply: string;
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
