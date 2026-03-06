import type {
  ApiIssue,
  ApiRun,
  ApiWorkflowProfile,
  ApiStatsResponse,
  CancelChatResponse,
  ChatEventGroupResponse,
  ChatEventsPageQuery,
  ChatSessionStatus,
  CreateChatResponse,
  CreateChatRequest,
  CreateIssueFromFilesRequest,
  CreateIssueRequest,
  CreateIssueResponse,
  CreateProjectCreateRequest,
  CreateProjectCreateRequestResponse,
  CreateProjectRequest,
  GetProjectCreateRequestResponse,
  GetChatResponse,
  IssueActionRequest,
  IssueActionResponse,
  IssueChangeRecord,
  IssueDagResponse,
  IssueReviewRecord,
  ListChatRunEventsResponse,
  ListRunEventsResponse,
  ListAdminAuditLogResponse,
  ListChatsResponse,
  IssueTimelineEntry,
  ListIssueTimelineQuery,
  ListIssueTimelineResponse,
  ListIssuesResponse,
  ListRunCheckpointsResponse,
  ListRunsResponse,
  StageSessionStatus,
  WakeStageSessionResponse,
  ListWorkflowProfilesResponse,
  ListProjectsResponse,
  RepoDiffResponse,
  RepoStatusResponse,
  RepoTreeResponse,
  SetIssueAutoMergeRequest,
  SetIssueAutoMergeResponse,
  SubmitIssueReviewResponse,
} from "../types/api";
import type { Project } from "../types/workflow";

type Primitive = string | number | boolean;
type PaginationParams = {
  limit?: number;
  offset?: number;
};

type AdminAuditLogQuery = PaginationParams & {
  projectId?: string;
  action?: string;
  user?: string;
  since?: string;
  until?: string;
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
    return new URL(trimmed, window.location.origin)
      .toString()
      .replace(/\/+$/, "");
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

  if (path.startsWith("/api/")) {
    const base = new URL(baseUrl);
    const normalizedPath = path.startsWith("/") ? path : `/${path}`;
    const normalizedBasePath = base.pathname.replace(/\/+$/, "");
    const apiPrefixIndex = normalizedBasePath.toLowerCase().indexOf("/api/");
    const basePathPrefix =
      apiPrefixIndex >= 0
        ? normalizedBasePath.slice(0, apiPrefixIndex)
        : normalizedBasePath;
    const resolvedPath = `${basePathPrefix}${normalizedPath}`.replace(
      /\/{2,}/g,
      "/",
    );
    const absolute = new URL(resolvedPath, `${base.protocol}//${base.host}`);
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

const normalizeIssueNumberFromExternalId = (
  externalId: string,
): number | undefined => {
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

const normalizeApiRun = (run: ApiRun): ApiRun => {
  if (!run.github) {
    return run;
  }
  return {
    ...run,
    github: {
      connection_status: run.github.connection_status,
      issue_number: run.github.issue_number,
      issue_url: run.github.issue_url,
      pr_number: run.github.pr_number,
      pr_url: run.github.pr_url,
    },
  };
};

const normalizeApiIssue = (issue: ApiIssue): ApiIssue => {
  const issueNumber =
    normalizeIssueNumberFromExternalId(issue.external_id ?? "") ??
    toSafeNumber(
      (issue as { github?: { issue_number?: unknown } }).github?.issue_number,
    );
  const issueUrl =
    toSafeString(
      (issue as { github?: { issue_url?: unknown } }).github?.issue_url,
    ) ??
    (toSafeString(issue.external_id)?.startsWith("http")
      ? toSafeString(issue.external_id)
      : undefined);

  return {
    ...issue,
    labels: Array.isArray(issue.labels) ? issue.labels : [],
    attachments: Array.isArray(issue.attachments) ? issue.attachments : [],
    depends_on: Array.isArray(issue.depends_on) ? issue.depends_on : [],
    blocks: Array.isArray(issue.blocks) ? issue.blocks : [],
    github: {
      issue_number: issueNumber,
      issue_url: issueUrl,
    },
  };
};

const normalizeIssueTimelineEntry = (
  rawEntry: unknown,
  index: number,
): IssueTimelineEntry => {
  const entry =
    rawEntry && typeof rawEntry === "object"
      ? (rawEntry as Record<string, unknown>)
      : {};
  const refs =
    entry.refs && typeof entry.refs === "object"
      ? (entry.refs as Record<string, unknown>)
      : {};
  const meta =
    entry.meta && typeof entry.meta === "object"
      ? (entry.meta as Record<string, unknown>)
      : {};

  const eventID = toSafeString(entry.event_id);
  const createdAt = toSafeString(entry.created_at);
  const issueID = toSafeString(refs.issue_id);

  if (!eventID || !createdAt || !issueID) {
    throw new Error(
      "issue timeline 响应结构不兼容：缺少 event_id/created_at/refs.issue_id，请重启后端到最新版本。",
    );
  }

  const actorName = toSafeString(entry.actor_name) ?? "system";
  const actorAvatarSeed = toSafeString(entry.actor_avatar_seed) ?? actorName;
  const kind = toSafeString(entry.kind) ?? "event";

  return {
    event_id: eventID,
    kind,
    created_at: createdAt,
    actor_type: toSafeString(entry.actor_type) ?? "system",
    actor_name: actorName,
    actor_avatar_seed: actorAvatarSeed,
    title: toSafeString(entry.title) ?? `${kind} #${index + 1}`,
    body: toSafeString(entry.body) ?? "",
    status: toSafeString(entry.status) ?? "info",
    refs: {
      issue_id: issueID,
      run_id: toSafeString(refs.run_id),
      stage: toSafeString(refs.stage),
    },
    meta,
  };
};

export interface ApiClient {
  request<TResponse, TBody = unknown>(
    options: RequestOptions<TBody>,
  ): Promise<TResponse>;
  get<TResponse>(
    path: string,
    options?: Omit<RequestOptions<never>, "path" | "method">,
  ): Promise<TResponse>;
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
  del<TResponse>(
    path: string,
    options?: Omit<RequestOptions<never>, "path" | "method">,
  ): Promise<TResponse>;

  getStats(): Promise<ApiStatsResponse>;
  listProjects(): Promise<ListProjectsResponse>;
  createProject(body: CreateProjectRequest): Promise<Project>;
  createProjectCreateRequest(
    body: CreateProjectCreateRequest,
  ): Promise<CreateProjectCreateRequestResponse>;
  getProjectCreateRequest(
    requestId: string,
  ): Promise<GetProjectCreateRequestResponse>;
  listIssues(
    projectId: string,
    pagination?: PaginationParams,
  ): Promise<ListIssuesResponse>;
  getIssue(issueId: string): Promise<ApiIssue>;
  listWorkflowProfiles(): Promise<ListWorkflowProfilesResponse>;
  getWorkflowProfile(profileType: string): Promise<ApiWorkflowProfile>;
  listRuns(
    projectId: string,
    pagination?: PaginationParams,
  ): Promise<ListRunsResponse>;
  getRun(runId: string): Promise<ApiRun>;
  getRunCheckpoints(runId: string): Promise<ListRunCheckpointsResponse>;
  getStageSessionStatus(
    runId: string,
    stage: string,
  ): Promise<StageSessionStatus>;
  wakeStageSession(
    runId: string,
    stage: string,
  ): Promise<WakeStageSessionResponse>;
  promptStageSession(
    runId: string,
    stage: string,
    message: string,
  ): Promise<void>;
  createIssue(
    projectId: string,
    body: CreateIssueRequest,
  ): Promise<CreateIssueResponse>;
  createIssueFromFiles(
    projectId: string,
    body: CreateIssueFromFilesRequest,
  ): Promise<CreateIssueResponse>;
  submitIssueReview(
    projectId: string,
    issueId: string,
  ): Promise<SubmitIssueReviewResponse>;
  applyIssueAction(
    projectId: string,
    issueId: string,
    body: IssueActionRequest,
  ): Promise<IssueActionResponse>;
  getIssueDag(projectId: string, issueId: string): Promise<IssueDagResponse>;
  listIssueReviews?(
    projectId: string,
    issueId: string,
  ): Promise<IssueReviewRecord[]>;
  listIssueChanges?(
    projectId: string,
    issueId: string,
  ): Promise<IssueChangeRecord[]>;
  listChats(projectId: string): Promise<ListChatsResponse>;
  listChatRunEvents(
    projectId: string,
    sessionId: string,
    query?: ChatEventsPageQuery,
  ): Promise<ListChatRunEventsResponse>;
  getChatEventGroup(
    projectId: string,
    sessionId: string,
    groupId: string,
  ): Promise<ChatEventGroupResponse>;
  createChat(
    projectId: string,
    body: CreateChatRequest,
  ): Promise<CreateChatResponse>;
  cancelChat(projectId: string, sessionId: string): Promise<CancelChatResponse>;
  getChatSessionStatus(
    projectId: string,
    sessionId: string,
  ): Promise<ChatSessionStatus>;
  getChat(projectId: string, sessionId: string): Promise<GetChatResponse>;
  setIssueAutoMerge(
    projectId: string,
    issueId: string,
    body: SetIssueAutoMergeRequest,
  ): Promise<SetIssueAutoMergeResponse>;
  applyTaskAction?(
    projectId: string,
    issueId: string,
    _taskId: string,
    body: IssueActionRequest,
  ): Promise<IssueActionResponse>;
  listIssueTimeline(
    projectId: string,
    issueId: string,
    query?: ListIssueTimelineQuery,
  ): Promise<ListIssueTimelineResponse>;
  listAdminAuditLog?(
    query?: AdminAuditLogQuery,
  ): Promise<ListAdminAuditLogResponse>;
  getRepoTree(projectId: string, dir?: string): Promise<RepoTreeResponse>;
  getRepoStatus(projectId: string): Promise<RepoStatusResponse>;
  getRepoDiff(projectId: string, filePath: string): Promise<RepoDiffResponse>;
  listRunEvents(runId: string): Promise<ListRunEventsResponse>;
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
      throw new ApiError(
        response.status,
        extractErrorMessage(response.status, data),
        data,
      );
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
    getStats: () => request<ApiStatsResponse>({ path: "/api/v1/stats" }),
    listProjects: () =>
      request<ListProjectsResponse>({ path: "/api/v1/projects" }),
    createProject: (body) =>
      request<Project, CreateProjectRequest>({
        path: "/api/v1/projects",
        method: "POST",
        body,
      }),
    createProjectCreateRequest: (body) =>
      request<CreateProjectCreateRequestResponse, CreateProjectCreateRequest>({
        path: "/api/v1/projects/create-requests",
        method: "POST",
        body,
      }),
    getProjectCreateRequest: (requestId) =>
      request<GetProjectCreateRequestResponse>({
        path: `/api/v1/projects/create-requests/${requestId}`,
      }),
    listIssues: async (projectId, pagination) => {
      const response = await request<ListIssuesResponse | ApiIssue[]>({
        path: "/api/v1/issues",
        query: {
          project_id: projectId,
          limit: pagination?.limit,
          offset: pagination?.offset,
        },
      });
      if (Array.isArray(response)) {
        return {
          items: response.map(normalizeApiIssue),
          total: response.length,
          offset: pagination?.offset ?? 0,
        };
      }
      const items = Array.isArray(response.items) ? response.items : [];
      return {
        items: items.map(normalizeApiIssue),
        total:
          typeof response.total === "number" ? response.total : items.length,
        offset:
          typeof response.offset === "number"
            ? response.offset
            : (pagination?.offset ?? 0),
      };
    },
    getIssue: async (issueId) => {
      const response = await request<ApiIssue>({
        path: `/api/v1/issues/${issueId}`,
      });
      return normalizeApiIssue(response);
    },
    listWorkflowProfiles: async () => {
      const response = await request<{ items?: ApiWorkflowProfile[] }>({
        path: "/api/v1/workflow-profiles",
      });
      const items = Array.isArray(response.items) ? response.items : [];
      return {
        items,
        total: items.length,
        offset: 0,
      };
    },
    getWorkflowProfile: (profileType) =>
      request<ApiWorkflowProfile>({
        path: `/api/v1/workflow-profiles/${profileType}`,
      }),
    listRuns: async (projectId, pagination) => {
      const response = await request<ListRunsResponse | ApiRun[]>({
        path: "/api/v1/runs",
        query: {
          project_id: projectId,
          limit: pagination?.limit,
          offset: pagination?.offset,
        },
      });
      if (Array.isArray(response)) {
        return {
          items: response.map(normalizeApiRun),
          total: response.length,
          offset: pagination?.offset ?? 0,
        };
      }
      const items = Array.isArray(response.items) ? response.items : [];
      return {
        items: items.map(normalizeApiRun),
        total:
          typeof response.total === "number" ? response.total : items.length,
        offset:
          typeof response.offset === "number"
            ? response.offset
            : (pagination?.offset ?? 0),
      };
    },
    getRun: async (runId) => {
      const response = await request<ApiRun>({
        path: `/api/v1/runs/${runId}`,
      });
      return normalizeApiRun(response);
    },
    getRunCheckpoints: (runId) =>
      request<ListRunCheckpointsResponse>({
        path: `/api/v1/runs/${runId}/checkpoints`,
      }),
    getStageSessionStatus: (runId, stage) =>
      request<StageSessionStatus>({
        path: `/api/v1/runs/${runId}/stages/${stage}/session`,
      }),
    wakeStageSession: (runId, stage) =>
      request<WakeStageSessionResponse>({
        path: `/api/v1/runs/${runId}/stages/${stage}/session/wake`,
        method: "POST",
      }),
    promptStageSession: (runId, stage, message) =>
      request<void>({
        path: `/api/v1/runs/${runId}/stages/${stage}/session/prompt`,
        method: "POST",
        body: { message },
      }),
    createIssue: async (projectId, body) => {
      const response = await request<CreateIssueResponse, CreateIssueRequest>({
        path: `/api/v1/projects/${projectId}/issues`,
        method: "POST",
        body,
      });
      return normalizeApiIssue(response);
    },
    createIssueFromFiles: async (projectId, body) => {
      const response = await request<
        CreateIssueResponse,
        CreateIssueFromFilesRequest
      >({
        path: `/api/v1/projects/${projectId}/issues/from-files`,
        method: "POST",
        body,
      });
      return normalizeApiIssue(response);
    },
    submitIssueReview: (projectId, issueId) =>
      request<SubmitIssueReviewResponse>({
        path: `/api/v1/projects/${projectId}/issues/${issueId}/review`,
        method: "POST",
      }),
    applyIssueAction: (projectId, issueId, body) =>
      request<IssueActionResponse, IssueActionRequest>({
        path: `/api/v1/projects/${projectId}/issues/${issueId}/action`,
        method: "POST",
        body,
      }),
    getIssueDag: (projectId, issueId) =>
      request<IssueDagResponse>({
        path: `/api/v1/projects/${projectId}/issues/${issueId}/dag`,
      }),
    listIssueReviews: (projectId, issueId) =>
      request<IssueReviewRecord[]>({
        path: `/api/v1/projects/${projectId}/issues/${issueId}/reviews`,
      }),
    listIssueChanges: (projectId, issueId) =>
      request<IssueChangeRecord[]>({
        path: `/api/v1/projects/${projectId}/issues/${issueId}/changes`,
      }),
    listChats: (projectId) =>
      request<ListChatsResponse>({
        path: `/api/v1/projects/${projectId}/chat`,
      }),
    listChatRunEvents: (projectId, sessionId, query) =>
      request<ListChatRunEventsResponse>({
        path: `/api/v1/projects/${projectId}/chat/${sessionId}/events`,
        query: query
          ? {
              cursor: query.cursor,
              limit: query.limit,
            }
          : undefined,
      }),
    getChatEventGroup: (projectId, sessionId, groupId) =>
      request<ChatEventGroupResponse>({
        path: `/api/v1/projects/${projectId}/chat/${sessionId}/event-groups/${groupId}`,
      }),
    createChat: (projectId, body) =>
      request<CreateChatResponse, CreateChatRequest>({
        path: `/api/v1/projects/${projectId}/chat`,
        method: "POST",
        body,
      }),
    cancelChat: (projectId, sessionId) =>
      request<CancelChatResponse>({
        path: `/api/v1/projects/${projectId}/chat/${sessionId}/cancel`,
        method: "POST",
      }),
    getChatSessionStatus: (projectId, sessionId) =>
      request<ChatSessionStatus>({
        path: `/api/v1/projects/${projectId}/chat/${sessionId}/status`,
      }),
    getChat: (projectId, sessionId) =>
      request<GetChatResponse>({
        path: `/api/v1/projects/${projectId}/chat/${sessionId}`,
      }),
    setIssueAutoMerge: (projectId, issueId, body) =>
      request<SetIssueAutoMergeResponse, SetIssueAutoMergeRequest>({
        path: `/api/v1/projects/${projectId}/issues/${issueId}/auto-merge`,
        method: "POST",
        body,
      }),
    applyTaskAction: (projectId, issueId, _taskId, body) =>
      request<IssueActionResponse, IssueActionRequest>({
        path: `/api/v1/projects/${projectId}/issues/${issueId}/action`,
        method: "POST",
        body,
      }),
    listIssueTimeline: async (projectId, issueId, query) => {
      const response = await request<ListIssueTimelineResponse>({
        path: `/api/v1/projects/${projectId}/issues/${issueId}/timeline`,
        query: {
          limit: query?.limit,
          offset: query?.offset,
        },
      });
      const rawItems = Array.isArray(response.items) ? response.items : [];
      return {
        items: rawItems.map((item, index) =>
          normalizeIssueTimelineEntry(item, index),
        ),
        total:
          typeof response.total === "number" ? response.total : rawItems.length,
        offset:
          typeof response.offset === "number"
            ? response.offset
            : (query?.offset ?? 0),
      };
    },
    listAdminAuditLog: (query) =>
      request<ListAdminAuditLogResponse>({
        path: "/api/v1/admin/audit-log",
        query: {
          project_id: query?.projectId?.trim() ? query.projectId : undefined,
          action: query?.action?.trim() ? query.action : undefined,
          user: query?.user?.trim() ? query.user : undefined,
          since: query?.since?.trim() ? query.since : undefined,
          until: query?.until?.trim() ? query.until : undefined,
          limit: query?.limit,
          offset: query?.offset,
        },
      }),
    getRepoTree: (projectId, dir) =>
      request<RepoTreeResponse>({
        path: `/api/v1/projects/${projectId}/repo/tree`,
        query: {
          dir: dir?.trim() ? dir : undefined,
        },
      }),
    getRepoStatus: (projectId) =>
      request<RepoStatusResponse>({
        path: `/api/v1/projects/${projectId}/repo/status`,
      }),
    getRepoDiff: (projectId, filePath) =>
      request<RepoDiffResponse>({
        path: `/api/v1/projects/${projectId}/repo/diff`,
        query: {
          file: filePath,
        },
      }),
    listRunEvents: async (runId) => {
      const response = await request<ListRunEventsResponse>({
        path: `/api/v1/runs/${runId}/events`,
      });
      return {
        items: Array.isArray(response.items) ? response.items : [],
        total: typeof response.total === "number" ? response.total : 0,
      };
    },
  };
};
