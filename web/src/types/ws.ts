export interface WsEnvelope<TPayload = unknown> {
  type: string;
  pipeline_id?: string;
  project_id?: string;
  plan_id?: string;
  data?: TPayload;
  payload?: TPayload;
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
    | "subscribe_plan"
    | "unsubscribe_plan"
    | "subscribe_pipeline"
    | "unsubscribe_pipeline";
  plan_id?: string;
  pipeline_id?: string;
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
