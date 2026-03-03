export type A2ATaskState =
  | "unknown"
  | "submitted"
  | "working"
  | "input-required"
  | "completed"
  | "failed"
  | "canceled"
  | string;

export interface A2ATaskStatus {
  state: A2ATaskState;
  timestamp?: string;
  [key: string]: unknown;
}

export interface A2ATask {
  id: string;
  contextId?: string;
  status: A2ATaskStatus;
  metadata?: Record<string, unknown>;
  [key: string]: unknown;
}

export interface A2ATextPart {
  kind: "text";
  text: string;
}

export type A2AMessagePart = A2ATextPart | Record<string, unknown>;

export interface A2AMessage {
  role: "user" | "assistant" | string;
  parts: A2AMessagePart[];
  contextId?: string;
  [key: string]: unknown;
}

export interface A2AMessageSendConfig {
  blocking?: boolean;
  [key: string]: unknown;
}

export interface A2AMessageSendParams {
  message: A2AMessage;
  metadata?: Record<string, unknown>;
  config?: A2AMessageSendConfig;
  [key: string]: unknown;
}

export interface A2ATaskQueryParams {
  id: string;
  metadata?: Record<string, unknown>;
  [key: string]: unknown;
}

export interface A2ATaskIDParams extends A2ATaskQueryParams {}

export interface A2AStreamEvent {
  event: string;
  data: unknown;
}

export interface A2ARpcErrorPayload {
  code: number;
  message: string;
  data?: unknown;
}

export interface A2ARpcResponse<TResult> {
  jsonrpc: string;
  id?: unknown;
  result?: TResult;
  error?: A2ARpcErrorPayload;
}
