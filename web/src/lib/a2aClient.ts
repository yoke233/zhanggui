import type {
  A2AMessageSendParams,
  A2ARpcResponse,
  A2AStreamEvent,
  A2ATask,
  A2ATaskIDParams,
  A2ATaskQueryParams,
} from "../types/a2a";

type A2ARpcMethod = "message/send" | "tasks/get" | "tasks/cancel" | "message/stream";

export interface A2AClientOptions {
  baseUrl: string;
  getToken?: () => string | null | undefined;
  fetchImpl?: typeof fetch;
  defaultHeaders?: HeadersInit;
  a2aVersion?: string;
}

export interface A2ARequestOptions {
  signal?: AbortSignal;
  requestId?: string;
}

export interface A2AClient {
  sendMessage(params: A2AMessageSendParams, options?: A2ARequestOptions): Promise<A2ATask>;
  getTask(params: A2ATaskQueryParams, options?: A2ARequestOptions): Promise<A2ATask>;
  cancelTask(params: A2ATaskIDParams, options?: A2ARequestOptions): Promise<A2ATask>;
  streamMessage(
    params: A2AMessageSendParams,
    options?: A2ARequestOptions,
  ): Promise<A2AStreamEvent[]>;
}

export class A2ARpcError extends Error {
  code: number;
  data: unknown;

  constructor(code: number, message: string, data: unknown) {
    super(message);
    this.name = "A2ARpcError";
    this.code = code;
    this.data = data;
  }
}

const a2aJSONRPCVersion = "2.0";
const defaultA2AVersion = "0.3";
let requestSeed = 0;

const normalizeBaseUrl = (baseUrl: string): string => {
  const trimmed = baseUrl.replace(/\/+$/, "");
  if (/^https?:\/\//.test(trimmed)) {
    return trimmed;
  }

  if (typeof window !== "undefined" && window.location?.origin) {
    return new URL(trimmed, window.location.origin).toString().replace(/\/+$/, "");
  }

  return new URL(trimmed, "http://localhost").toString().replace(/\/+$/, "");
};

const toA2AEndpoint = (baseUrl: string): string => {
  const normalized = normalizeBaseUrl(baseUrl);
  if (normalized.endsWith("/a2a")) {
    return normalized;
  }
  return `${normalized}/a2a`;
};

const nextRequestId = (): string => {
  requestSeed += 1;
  return `a2a-${Date.now()}-${requestSeed}`;
};

const parseResponsePayload = async (response: Response): Promise<unknown> => {
  const rawText = await response.text();
  if (!rawText) {
    return undefined;
  }

  try {
    return JSON.parse(rawText) as unknown;
  } catch {
    return rawText;
  }
};

const extractHttpErrorMessage = (status: number, payload: unknown): string => {
  if (payload && typeof payload === "object") {
    const maybeMessage = (payload as { message?: unknown }).message;
    if (typeof maybeMessage === "string" && maybeMessage.trim().length > 0) {
      return maybeMessage;
    }
    const maybeError = (payload as { error?: { message?: unknown } | string }).error;
    if (typeof maybeError === "string" && maybeError.trim().length > 0) {
      return maybeError;
    }
    if (maybeError && typeof maybeError === "object") {
      const nestedMessage = maybeError.message;
      if (typeof nestedMessage === "string" && nestedMessage.trim().length > 0) {
        return nestedMessage;
      }
    }
  }
  return `A2A request failed with status ${status}`;
};

const parseRpcResult = <TResult>(payload: unknown): TResult => {
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    throw new Error("invalid JSON-RPC response");
  }

  const rpcPayload = payload as A2ARpcResponse<TResult>;
  if (rpcPayload.error) {
    const code = Number.isFinite(rpcPayload.error.code) ? rpcPayload.error.code : -32603;
    const message =
      typeof rpcPayload.error.message === "string" && rpcPayload.error.message.trim().length > 0
        ? rpcPayload.error.message
        : "a2a rpc error";
    throw new A2ARpcError(code, message, rpcPayload.error.data);
  }

  if (!("result" in rpcPayload)) {
    throw new Error("missing JSON-RPC result");
  }

  return rpcPayload.result as TResult;
};

const parseSSEEventData = (raw: string): unknown => {
  const trimmed = raw.trim();
  if (!trimmed) {
    return "";
  }
  try {
    return JSON.parse(trimmed) as unknown;
  } catch {
    return trimmed;
  }
};

const parseSSEEvents = (raw: string): A2AStreamEvent[] => {
  if (!raw.trim()) {
    return [];
  }

  const events: A2AStreamEvent[] = [];
  let currentEvent = "message";
  let dataLines: string[] = [];

  const pushEvent = (): void => {
    if (dataLines.length === 0) {
      currentEvent = "message";
      return;
    }
    const data = parseSSEEventData(dataLines.join("\n"));
    events.push({
      event: currentEvent || "message",
      data,
    });
    currentEvent = "message";
    dataLines = [];
  };

  const lines = raw.replace(/\r\n/g, "\n").split("\n");
  lines.forEach((line) => {
    if (!line.trim()) {
      pushEvent();
      return;
    }

    if (line.startsWith(":")) {
      return;
    }

    if (line.startsWith("event:")) {
      currentEvent = line.slice("event:".length).trim() || "message";
      return;
    }

    if (line.startsWith("data:")) {
      dataLines.push(line.slice("data:".length).trimStart());
      return;
    }
  });

  pushEvent();
  return events;
};

export const createA2AClient = (options: A2AClientOptions): A2AClient => {
  const endpoint = toA2AEndpoint(options.baseUrl);
  const fetchImpl = options.fetchImpl ?? fetch;
  const getToken = options.getToken;
  const defaultHeaders = options.defaultHeaders;
  const a2aVersion = options.a2aVersion?.trim() || defaultA2AVersion;

  const buildHeaders = (): Headers => {
    const headers = new Headers(defaultHeaders);
    headers.set("Accept", "application/json");
    if (a2aVersion) {
      headers.set("A2A-Version", a2aVersion);
    }

    const token = getToken?.();
    if (typeof token === "string" && token.trim().length > 0) {
      headers.set("Authorization", `Bearer ${token.trim()}`);
    }
    return headers;
  };

  const postJSONRPC = async <TResult>(
    method: A2ARpcMethod,
    params: unknown,
    requestOptions?: A2ARequestOptions,
  ): Promise<TResult> => {
    const headers = buildHeaders();
    headers.set("Content-Type", "application/json");

    const payload = {
      jsonrpc: a2aJSONRPCVersion,
      id: requestOptions?.requestId ?? nextRequestId(),
      method,
      params,
    };

    const response = await fetchImpl(endpoint, {
      method: "POST",
      headers,
      body: JSON.stringify(payload),
      signal: requestOptions?.signal,
    });

    const responsePayload = await parseResponsePayload(response);
    if (!response.ok) {
      throw new Error(extractHttpErrorMessage(response.status, responsePayload));
    }

    return parseRpcResult<TResult>(responsePayload);
  };

  return {
    sendMessage: (params, requestOptions) => {
      return postJSONRPC<A2ATask>("message/send", params, requestOptions);
    },
    getTask: (params, requestOptions) => {
      return postJSONRPC<A2ATask>("tasks/get", params, requestOptions);
    },
    cancelTask: (params, requestOptions) => {
      return postJSONRPC<A2ATask>("tasks/cancel", params, requestOptions);
    },
    streamMessage: async (params, requestOptions) => {
      const headers = buildHeaders();
      headers.set("Content-Type", "application/json");

      const payload = {
        jsonrpc: a2aJSONRPCVersion,
        id: requestOptions?.requestId ?? nextRequestId(),
        method: "message/stream" as const,
        params,
      };

      const response = await fetchImpl(endpoint, {
        method: "POST",
        headers,
        body: JSON.stringify(payload),
        signal: requestOptions?.signal,
      });

      if (!response.ok) {
        const errorPayload = await parseResponsePayload(response);
        throw new Error(extractHttpErrorMessage(response.status, errorPayload));
      }

      const contentType = (response.headers.get("content-type") ?? "").toLowerCase();
      if (contentType.includes("text/event-stream")) {
        const rawStream = await response.text();
        return parseSSEEvents(rawStream);
      }

      const rpcPayload = await parseResponsePayload(response);
      parseRpcResult<unknown>(rpcPayload);
      return [];
    },
  };
};
