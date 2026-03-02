import type {
  ApiTaskItem,
  ApiTaskPlan,
  ApiPipeline,
  ApiStatsResponse,
  CreateChatResponse,
  CreateChatRequest,
  CreatePlanFromFilesRequest,
  CreatePlanResponse,
  CreatePipelineRequest,
  CreateProjectCreateRequest,
  CreateProjectCreateRequestResponse,
  CreatePlanRequest,
  CreateProjectRequest,
  GetProjectCreateRequestResponse,
  GetPipelineCheckpointsResponse,
  GetChatResponse,
  ListPipelinesResponse,
  ListPlansResponse,
  ListProjectsResponse,
  PipelineActionRequest,
  PipelineActionResponse,
  PlanActionRequest,
  PlanActionResponse,
  PlanDagResponse,
  RepoDiffResponse,
  RepoStatusResponse,
  RepoTreeResponse,
  SubmitPlanReviewResponse,
  TaskActionRequest,
  TaskActionResponse,
} from "../types/api";
import type { Project } from "../types/workflow";

type Primitive = string | number | boolean;
type PaginationParams = {
  limit?: number;
  offset?: number;
};

export interface RequestOptions<TBody = unknown> {
  path: string;
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  query?: Record<string, Primitive | null | undefined>;
  body?: TBody;
  headers?: HeadersInit;
  signal?: AbortSignal;
}

export interface ApiClientOptions {
  baseUrl: string;
  getToken?: () => string | null | undefined;
  fetchImpl?: typeof fetch;
  defaultHeaders?: HeadersInit;
}

export class ApiError extends Error {
  status: number;
  data: unknown;

  constructor(status: number, message: string, data: unknown) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.data = data;
  }
}

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

const buildUrl = (
  baseUrl: string,
  path: string,
  query?: Record<string, Primitive | null | undefined>,
): string => {
  if (/^https?:\/\//.test(path)) {
    const absolute = new URL(path);
    if (query) {
      Object.entries(query).forEach(([key, value]) => {
        if (value !== undefined && value !== null) {
          absolute.searchParams.set(key, String(value));
        }
      });
    }
    return absolute.toString();
  }

  const normalizedPath = path.startsWith("/") ? path : `/${path}`;
  const url = new URL(`${baseUrl}${normalizedPath}`);

  if (query) {
    Object.entries(query).forEach(([key, value]) => {
      if (value !== undefined && value !== null) {
        url.searchParams.set(key, String(value));
      }
    });
  }

  return url.toString();
};

const canSendDirectly = (body: unknown): body is BodyInit =>
  typeof body === "string" ||
  body instanceof Blob ||
  body instanceof FormData ||
  body instanceof URLSearchParams ||
  body instanceof ArrayBuffer ||
  ArrayBuffer.isView(body);

const readResponseData = async (response: Response): Promise<unknown> => {
  const text = await response.text();
  if (!text) {
    return undefined;
  }

  const contentType = response.headers.get("content-type") ?? "";
  if (contentType.toLowerCase().includes("application/json")) {
    try {
      return JSON.parse(text);
    } catch {
      return text;
    }
  }
  return text;
};

const extractErrorMessage = (status: number, data: unknown): string => {
  if (data && typeof data === "object") {
    const maybeMessage = (data as { message?: unknown }).message;
    if (typeof maybeMessage === "string" && maybeMessage.trim().length > 0) {
      return maybeMessage;
    }
    const maybeError = (data as { error?: unknown }).error;
    if (typeof maybeError === "string" && maybeError.trim().length > 0) {
      return maybeError;
    }
  }
  return `Request failed with status ${status}`;
};

const toSafeNumber = (raw: unknown): number | undefined => {
  if (typeof raw === "number" && Number.isFinite(raw)) {
    return Math.trunc(raw);
  }
  if (typeof raw === "string") {
    const parsed = Number.parseInt(raw.trim(), 10);
    if (Number.isFinite(parsed)) {
      return parsed;
    }
  }
  return undefined;
};

const toSafeString = (raw: unknown): string | undefined => {
  if (typeof raw !== "string") {
    return undefined;
  }
  const trimmed = raw.trim();
  return trimmed.length > 0 ? trimmed : undefined;
};

const normalizeIssueNumberFromExternalId = (externalId: string): number | undefined => {
  const trimmed = externalId.trim();
  if (!trimmed) {
    return undefined;
  }
  const hashStripped = trimmed.startsWith("#") ? trimmed.slice(1) : trimmed;
  const direct = Number.parseInt(hashStripped, 10);
  if (Number.isFinite(direct)) {
    return direct;
  }

  const matches = hashStripped.match(/(\d+)(?!.*\d)/);
  if (!matches) {
    return undefined;
  }
  const parsed = Number.parseInt(matches[1] ?? "", 10);
  return Number.isFinite(parsed) ? parsed : undefined;
};

const normalizeApiPipeline = (pipeline: ApiPipeline): ApiPipeline => {
  const config = pipeline.config ?? {};
  const issueNumber =
    toSafeNumber(config.issue_number) ?? toSafeNumber(config.github_issue_number);
  const prNumber = toSafeNumber(config.pr_number) ?? toSafeNumber(config.github_pr_number);
  const issueUrl =
    toSafeString(config.issue_url) ?? toSafeString(config.github_issue_url);
  const prUrl = toSafeString(config.pr_url) ?? toSafeString(config.github_pr_url);
  const rawConnectionStatus =
    toSafeString(config.github_connection_status) ?? toSafeString(config.connection_status);

  const connectionStatus =
    rawConnectionStatus === "connected" ||
    rawConnectionStatus === "degraded" ||
    rawConnectionStatus === "disconnected"
      ? rawConnectionStatus
      : issueNumber || prNumber || issueUrl || prUrl
        ? "connected"
        : "disconnected";

  return {
    ...pipeline,
    github: {
      connection_status: connectionStatus,
      issue_number: issueNumber,
      issue_url: issueUrl,
      pr_number: prNumber,
      pr_url: prUrl,
    },
  };
};

const normalizeApiTaskItem = (task: ApiTaskItem): ApiTaskItem => {
  const issueNumber =
    normalizeIssueNumberFromExternalId(task.external_id ?? "") ??
    toSafeNumber((task as { github?: { issue_number?: unknown } }).github?.issue_number);
  const issueUrl =
    toSafeString((task as { github?: { issue_url?: unknown } }).github?.issue_url) ??
    (toSafeString(task.external_id)?.startsWith("http")
      ? toSafeString(task.external_id)
      : undefined);

  return {
    ...task,
    inputs: Array.isArray(task.inputs) ? task.inputs : [],
    outputs: Array.isArray(task.outputs) ? task.outputs : [],
    acceptance: Array.isArray(task.acceptance) ? task.acceptance : [],
    constraints: Array.isArray(task.constraints) ? task.constraints : [],
    github: {
      issue_number: issueNumber,
      issue_url: issueUrl,
    },
  };
};

const normalizeApiTaskPlan = (plan: ApiTaskPlan): ApiTaskPlan => {
  return {
    ...plan,
    tasks: Array.isArray(plan.tasks) ? plan.tasks.map(normalizeApiTaskItem) : [],
  };
};

export interface ApiClient {
  request<TResponse, TBody = unknown>(options: RequestOptions<TBody>): Promise<TResponse>;
  get<TResponse>(path: string, options?: Omit<RequestOptions<never>, "path" | "method">): Promise<TResponse>;
  post<TResponse, TBody = unknown>(
    path: string,
    body?: TBody,
    options?: Omit<RequestOptions<TBody>, "path" | "method" | "body">,
  ): Promise<TResponse>;
  put<TResponse, TBody = unknown>(
    path: string,
    body?: TBody,
    options?: Omit<RequestOptions<TBody>, "path" | "method" | "body">,
  ): Promise<TResponse>;
  del<TResponse>(path: string, options?: Omit<RequestOptions<never>, "path" | "method">): Promise<TResponse>;

  getStats(): Promise<ApiStatsResponse>;
  listProjects(): Promise<ListProjectsResponse>;
  createProject(body: CreateProjectRequest): Promise<Project>;
  createProjectCreateRequest(
    body: CreateProjectCreateRequest,
  ): Promise<CreateProjectCreateRequestResponse>;
  getProjectCreateRequest(requestId: string): Promise<GetProjectCreateRequestResponse>;
  listPipelines(projectId: string, pagination?: PaginationParams): Promise<ListPipelinesResponse>;
  createPipeline(projectId: string, body: CreatePipelineRequest): Promise<ApiPipeline>;
  createChat(projectId: string, body: CreateChatRequest): Promise<CreateChatResponse>;
  getChat(projectId: string, sessionId: string): Promise<GetChatResponse>;
  createPlan(projectId: string, body: CreatePlanRequest): Promise<CreatePlanResponse>;
  createPlanFromFiles(
    projectId: string,
    body: CreatePlanFromFilesRequest,
  ): Promise<CreatePlanResponse>;
  submitPlanReview(projectId: string, planId: string): Promise<SubmitPlanReviewResponse>;
  applyPlanAction(
    projectId: string,
    planId: string,
    body: PlanActionRequest,
  ): Promise<PlanActionResponse>;
  applyTaskAction(
    projectId: string,
    planId: string,
    taskId: string,
    body: TaskActionRequest,
  ): Promise<TaskActionResponse>;
  listPlans(projectId: string, pagination?: PaginationParams): Promise<ListPlansResponse>;
  getPlanDag(projectId: string, planId: string): Promise<PlanDagResponse>;
  getPipeline(projectId: string, pipelineId: string): Promise<ApiPipeline>;
  getPipelineCheckpoints(
    projectId: string,
    pipelineId: string,
  ): Promise<GetPipelineCheckpointsResponse>;
  getRepoTree(projectId: string, dir?: string): Promise<RepoTreeResponse>;
  getRepoStatus(projectId: string): Promise<RepoStatusResponse>;
  getRepoDiff(projectId: string, filePath: string): Promise<RepoDiffResponse>;
  applyPipelineAction(
    projectId: string,
    pipelineId: string,
    body: PipelineActionRequest,
  ): Promise<PipelineActionResponse>;
}

export const createApiClient = (options: ApiClientOptions): ApiClient => {
  const baseUrl = normalizeBaseUrl(options.baseUrl);
  const fetchImpl = options.fetchImpl ?? fetch;
  const getToken = options.getToken;
  const defaultHeaders = options.defaultHeaders;

  const request = async <TResponse, TBody = unknown>(
    requestOptions: RequestOptions<TBody>,
  ): Promise<TResponse> => {
    const headers = new Headers(defaultHeaders);
    if (requestOptions.headers) {
      new Headers(requestOptions.headers).forEach((value, key) => {
        headers.set(key, value);
      });
    }
    headers.set("Accept", "application/json");

    const token = getToken?.();
    if (token) {
      headers.set("Authorization", `Bearer ${token}`);
    }

    let requestBody: BodyInit | undefined;
    if (requestOptions.body !== undefined && requestOptions.body !== null) {
      if (canSendDirectly(requestOptions.body)) {
        requestBody = requestOptions.body;
      } else {
        requestBody = JSON.stringify(requestOptions.body);
        if (!headers.has("Content-Type")) {
          headers.set("Content-Type", "application/json");
        }
      }
    }

    const response = await fetchImpl(
      buildUrl(baseUrl, requestOptions.path, requestOptions.query),
      {
        method: requestOptions.method ?? "GET",
        headers,
        body: requestBody,
        signal: requestOptions.signal,
      },
    );

    const data = await readResponseData(response);
    if (!response.ok) {
      throw new ApiError(response.status, extractErrorMessage(response.status, data), data);
    }
    return data as TResponse;
  };

  return {
    request,
    get: (path, requestOptions) =>
      request({
        ...requestOptions,
        path,
        method: "GET",
      }),
    post: (path, body, requestOptions) =>
      request({
        ...requestOptions,
        path,
        method: "POST",
        body,
      }),
    put: (path, body, requestOptions) =>
      request({
        ...requestOptions,
        path,
        method: "PUT",
        body,
      }),
    del: (path, requestOptions) =>
      request({
        ...requestOptions,
        path,
        method: "DELETE",
      }),
    getStats: () => request<ApiStatsResponse>({ path: "/stats" }),
    listProjects: () => request<ListProjectsResponse>({ path: "/projects" }),
    createProject: (body) =>
      request<Project, CreateProjectRequest>({
        path: "/projects",
        method: "POST",
        body,
      }),
    createProjectCreateRequest: (body) =>
      request<CreateProjectCreateRequestResponse, CreateProjectCreateRequest>({
        path: "/projects/create-requests",
        method: "POST",
        body,
      }),
    getProjectCreateRequest: (requestId) =>
      request<GetProjectCreateRequestResponse>({
        path: `/projects/create-requests/${requestId}`,
      }),
    listPipelines: async (projectId, pagination) => {
      const response = await request<ListPipelinesResponse>({
        path: `/projects/${projectId}/pipelines`,
        query: pagination,
      });
      return {
        ...response,
        items: response.items.map(normalizeApiPipeline),
      };
    },
    createPipeline: async (projectId, body) => {
      const response = await request<ApiPipeline, CreatePipelineRequest>({
        path: `/projects/${projectId}/pipelines`,
        method: "POST",
        body,
      });
      return normalizeApiPipeline(response);
    },
    createChat: (projectId, body) =>
      request<CreateChatResponse, CreateChatRequest>({
        path: `/projects/${projectId}/chat`,
        method: "POST",
        body,
      }),
    getChat: (projectId, sessionId) =>
      request<GetChatResponse>({
        path: `/projects/${projectId}/chat/${sessionId}`,
      }),
    createPlan: async (projectId, body) => {
      const response = await request<CreatePlanResponse, CreatePlanRequest>({
        path: `/projects/${projectId}/plans`,
        method: "POST",
        body,
      });
      return normalizeApiTaskPlan(response);
    },
    createPlanFromFiles: async (projectId, body) => {
      const response = await request<CreatePlanResponse, CreatePlanFromFilesRequest>({
        path: `/projects/${projectId}/plans/from-files`,
        method: "POST",
        body,
      });
      return normalizeApiTaskPlan(response);
    },
    submitPlanReview: (projectId, planId) =>
      request<SubmitPlanReviewResponse>({
        path: `/projects/${projectId}/plans/${planId}/review`,
        method: "POST",
      }),
    applyPlanAction: (projectId, planId, body) =>
      request<PlanActionResponse, PlanActionRequest>({
        path: `/projects/${projectId}/plans/${planId}/action`,
        method: "POST",
        body,
      }),
    applyTaskAction: (projectId, planId, taskId, body) =>
      request<TaskActionResponse, TaskActionRequest>({
        path: `/projects/${projectId}/plans/${planId}/tasks/${taskId}/action`,
        method: "POST",
        body,
      }),
    listPlans: async (projectId, pagination) => {
      const response = await request<ListPlansResponse>({
        path: `/projects/${projectId}/plans`,
        query: pagination,
      });
      return {
        ...response,
        items: response.items.map(normalizeApiTaskPlan),
      };
    },
    getPlanDag: (projectId, planId) =>
      request<PlanDagResponse>({
        path: `/projects/${projectId}/plans/${planId}/dag`,
      }),
    getPipeline: async (projectId, pipelineId) => {
      const response = await request<ApiPipeline>({
        path: `/projects/${projectId}/pipelines/${pipelineId}`,
      });
      return normalizeApiPipeline(response);
    },
    getPipelineCheckpoints: (projectId, pipelineId) =>
      request<GetPipelineCheckpointsResponse>({
        path: `/projects/${projectId}/pipelines/${pipelineId}/checkpoints`,
      }),
    getRepoTree: (projectId, dir) =>
      request<RepoTreeResponse>({
        path: `/projects/${projectId}/repo/tree`,
        query: {
          dir: dir?.trim() ? dir : undefined,
        },
      }),
    getRepoStatus: (projectId) =>
      request<RepoStatusResponse>({
        path: `/projects/${projectId}/repo/status`,
      }),
    getRepoDiff: (projectId, filePath) =>
      request<RepoDiffResponse>({
        path: `/projects/${projectId}/repo/diff`,
        query: {
          file: filePath,
        },
      }),
    applyPipelineAction: (projectId, pipelineId, body) =>
      request<PipelineActionResponse, PipelineActionRequest>({
        path: `/projects/${projectId}/pipelines/${pipelineId}/action`,
        method: "POST",
        body,
      }),
  };
};
