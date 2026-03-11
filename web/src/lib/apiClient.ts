import type {
  CancelFlowResponse,
  AgentDriver,
  AgentProfile,
  CreateSkillRequest,
  Artifact,
  Briefing,
  StatsResponse,
  AdminSystemEventRequest,
  AdminSystemEventResponse,
  ChatRequest,
  ChatResponse,
  ChatSessionDetail,
  ChatSessionSummary,
  ChatStatusResponse,
  CreateResourceBindingRequest,
  CreateProjectRequest,
  CreateFlowRequest,
  CreateStepRequest,
  GenerateStepsRequest,
  Event,
  Execution,
  Flow,
  ImportGitHubSkillRequest,
  Project,
  ResourceBinding,
  RunFlowResponse,
  SchedulerStats,
  SkillDetail,
  SkillInfo,
  Step,
  UpdateStepRequest,
  UpdateProjectRequest,
  DAGTemplate,
  CreateDAGTemplateRequest,
  UpdateDAGTemplateRequest,
  SaveFlowAsTemplateRequest,
  CreateFlowFromTemplateRequest,
  CreateFlowFromTemplateResponse,
} from "../types/apiV2";
import type { SandboxSupportResponse } from "../types/system";

type Primitive = string | number | boolean;

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

export interface ApiClient {
  request<TResponse, TBody = unknown>(
    options: RequestOptions<TBody>,
  ): Promise<TResponse>;

  getStats(): Promise<StatsResponse>;
  getSchedulerStats(): Promise<SchedulerStats>;
  getSandboxSupport(): Promise<SandboxSupportResponse>;
  sendSystemEvent(body: AdminSystemEventRequest): Promise<AdminSystemEventResponse>;

  listProjects(params?: { limit?: number; offset?: number }): Promise<Project[]>;
  createProject(body: CreateProjectRequest): Promise<Project>;
  getProject(projectId: number): Promise<Project>;
  updateProject(projectId: number, body: UpdateProjectRequest): Promise<Project>;
  deleteProject(projectId: number): Promise<void>;

  listProjectResources(projectId: number): Promise<ResourceBinding[]>;
  createProjectResource(projectId: number, body: CreateResourceBindingRequest): Promise<ResourceBinding>;
  getResource(resourceId: number): Promise<ResourceBinding>;
  deleteResource(resourceId: number): Promise<void>;

  getArtifact(artifactId: number): Promise<Artifact>;
  getLatestArtifact(stepId: number): Promise<Artifact>;
  listArtifactsByExecution(execId: number): Promise<Artifact[]>;

  getBriefing(briefingId: number): Promise<Briefing>;
  getBriefingByStep(stepId: number): Promise<Briefing>;

  chat(body: ChatRequest): Promise<ChatResponse>;
  listChatSessions(): Promise<ChatSessionSummary[]>;
  getChatSession(sessionId: string): Promise<ChatSessionDetail>;
  cancelChat(sessionId: string): Promise<{ session_id: string; status: string }>;
  closeChat(sessionId: string): Promise<{ session_id: string; status: string }>;
  getChatStatus(sessionId: string): Promise<ChatStatusResponse>;

  listFlows(params?: {
    project_id?: number;
    status?: string;
    archived?: boolean | "all";
    limit?: number;
    offset?: number;
  }): Promise<Flow[]>;
  createFlow(body: CreateFlowRequest): Promise<Flow>;
  getFlow(flowId: number): Promise<Flow>;
  runFlow(flowId: number): Promise<RunFlowResponse>;
  cancelFlow(flowId: number): Promise<CancelFlowResponse>;

  listSteps(flowId: number): Promise<Step[]>;
  createStep(flowId: number, body: CreateStepRequest): Promise<Step>;
  generateSteps(flowId: number, body: GenerateStepsRequest): Promise<Step[]>;
  getStep(stepId: number): Promise<Step>;
  updateStep(stepId: number, body: UpdateStepRequest): Promise<Step>;
  deleteStep(stepId: number): Promise<void>;

  listExecutions(stepId: number): Promise<Execution[]>;
  getExecution(execId: number): Promise<Execution>;

  listEvents(params?: {
    flow_id?: number;
    step_id?: number;
    session_id?: string;
    types?: string[];
    limit?: number;
    offset?: number;
  }): Promise<Event[]>;
  listFlowEvents(
    flowId: number,
    params?: { types?: string[]; limit?: number; offset?: number },
  ): Promise<Event[]>;

  listDrivers(): Promise<AgentDriver[]>;
  createDriver(body: AgentDriver): Promise<AgentDriver>;
  listProfiles(): Promise<AgentProfile[]>;
  createProfile(body: AgentProfile): Promise<AgentProfile>;
  listSkills(): Promise<SkillInfo[]>;
  getSkill(name: string): Promise<SkillDetail>;
  createSkill(body: CreateSkillRequest): Promise<SkillInfo>;
  importGitHubSkill(body: ImportGitHubSkillRequest): Promise<SkillInfo>;

  // DAG Templates
  listDAGTemplates(params?: {
    project_id?: number;
    tag?: string;
    search?: string;
    limit?: number;
    offset?: number;
  }): Promise<DAGTemplate[]>;
  createDAGTemplate(body: CreateDAGTemplateRequest): Promise<DAGTemplate>;
  getDAGTemplate(templateId: number): Promise<DAGTemplate>;
  updateDAGTemplate(templateId: number, body: UpdateDAGTemplateRequest): Promise<DAGTemplate>;
  deleteDAGTemplate(templateId: number): Promise<void>;
  saveFlowAsTemplate(flowId: number, body: SaveFlowAsTemplateRequest): Promise<DAGTemplate>;
  createFlowFromTemplate(templateId: number, body: CreateFlowFromTemplateRequest): Promise<CreateFlowFromTemplateResponse>;
}

export const createApiClient = (opts: ApiClientOptions): ApiClient => {
  const fetchImpl = opts.fetchImpl ?? fetch;
  const baseUrl = normalizeBaseUrl(opts.baseUrl);
  const getToken = opts.getToken;
  const defaultHeaders = opts.defaultHeaders;

  const request = async <TResponse, TBody = unknown>(
    options: RequestOptions<TBody>,
  ): Promise<TResponse> => {
    const url = buildUrl(baseUrl, options.path, options.query);
    const headers: HeadersInit = {
      ...(defaultHeaders ?? {}),
      ...(options.headers ?? {}),
    };

    const token = getToken?.();
    if (token) {
      (headers as Record<string, string>)["Authorization"] = `Bearer ${token}`;
    }

    let body: BodyInit | undefined;
    if (options.body !== undefined) {
      (headers as Record<string, string>)["Content-Type"] =
        (headers as Record<string, string>)["Content-Type"] ??
        "application/json";
      body = JSON.stringify(options.body);
    }

    const response = await fetchImpl(url, {
      method: options.method ?? "GET",
      headers,
      body,
      signal: options.signal,
    });

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
    getStats: () =>
      request<StatsResponse>({
        path: "/stats",
      }),
    getSchedulerStats: () =>
      request<SchedulerStats>({
        path: "/scheduler/stats",
      }),
    getSandboxSupport: () =>
      request<SandboxSupportResponse>({
        path: "/system/sandbox-support",
      }),
    sendSystemEvent: (body) =>
      request<AdminSystemEventResponse, AdminSystemEventRequest>({
        path: "/admin/system-event",
        method: "POST",
        body,
      }),
    listProjects: (params) =>
      request<Project[]>({
        path: "/projects",
        query: {
          limit: params?.limit,
          offset: params?.offset,
        },
      }).then((items) => (Array.isArray(items) ? items : [])),
    createProject: (body) =>
      request<Project, CreateProjectRequest>({
        path: "/projects",
        method: "POST",
        body,
      }),
    getProject: (projectId) =>
      request<Project>({
        path: `/projects/${projectId}`,
      }),
    updateProject: (projectId, body) =>
      request<Project, UpdateProjectRequest>({
        path: `/projects/${projectId}`,
        method: "PUT",
        body,
      }),
    deleteProject: (projectId) =>
      request<void>({
        path: `/projects/${projectId}`,
        method: "DELETE",
      }),

    listProjectResources: (projectId) =>
      request<ResourceBinding[]>({
        path: `/projects/${projectId}/resources`,
      }).then((items) => (Array.isArray(items) ? items : [])),
    createProjectResource: (projectId, body) =>
      request<ResourceBinding, CreateResourceBindingRequest>({
        path: `/projects/${projectId}/resources`,
        method: "POST",
        body,
      }),
    getResource: (resourceId) =>
      request<ResourceBinding>({
        path: `/resources/${resourceId}`,
      }),
    deleteResource: (resourceId) =>
      request<void>({
        path: `/resources/${resourceId}`,
        method: "DELETE",
      }),
    getArtifact: (artifactId) =>
      request<Artifact>({
        path: `/artifacts/${artifactId}`,
      }),
    getLatestArtifact: (stepId) =>
      request<Artifact>({
        path: `/steps/${stepId}/artifact`,
      }),
    listArtifactsByExecution: (execId) =>
      request<Artifact[]>({
        path: `/executions/${execId}/artifacts`,
      }).then((items) => (Array.isArray(items) ? items : [])),

    getBriefing: (briefingId) =>
      request<Briefing>({
        path: `/briefings/${briefingId}`,
      }),
    getBriefingByStep: (stepId) =>
      request<Briefing>({
        path: `/steps/${stepId}/briefing`,
      }),
    chat: (body) =>
      request<ChatResponse, ChatRequest>({
        path: "/chat",
        method: "POST",
        body,
      }),
    listChatSessions: () =>
      request<ChatSessionSummary[]>({
        path: "/chat/sessions",
      }).then((items) => (Array.isArray(items) ? items : [])),
    getChatSession: (sessionId) =>
      request<ChatSessionDetail>({
        path: `/chat/${encodeURIComponent(sessionId)}`,
      }),
    cancelChat: (sessionId) =>
      request<{ session_id: string; status: string }>({
        path: `/chat/${encodeURIComponent(sessionId)}/cancel`,
        method: "POST",
      }),
    closeChat: (sessionId) =>
      request<{ session_id: string; status: string }>({
        path: `/chat/${encodeURIComponent(sessionId)}`,
        method: "DELETE",
      }),
    getChatStatus: (sessionId) =>
      request<ChatStatusResponse>({
        path: `/chat/${encodeURIComponent(sessionId)}/status`,
      }),
    listFlows: (params) =>
      request<Flow[]>({
        path: "/flows",
        query: {
          project_id: params?.project_id,
          status: params?.status,
          archived: params?.archived === undefined ? undefined : String(params.archived),
          limit: params?.limit,
          offset: params?.offset,
        },
      }).then((items) => (Array.isArray(items) ? items : [])),
    createFlow: (body) =>
      request<Flow, CreateFlowRequest>({
        path: "/flows",
        method: "POST",
        body,
      }),
    getFlow: (flowId) =>
      request<Flow>({
        path: `/flows/${flowId}`,
      }),
    runFlow: (flowId) =>
      request<RunFlowResponse>({
        path: `/flows/${flowId}/run`,
        method: "POST",
      }),
    cancelFlow: (flowId) =>
      request<CancelFlowResponse>({
        path: `/flows/${flowId}/cancel`,
        method: "POST",
      }),

    listSteps: (flowId) =>
      request<Step[]>({
        path: `/flows/${flowId}/steps`,
      }).then((items) => (Array.isArray(items) ? items : [])),
    createStep: (flowId, body) =>
      request<Step, CreateStepRequest>({
        path: `/flows/${flowId}/steps`,
        method: "POST",
        body,
      }),
    generateSteps: (flowId, body) =>
      request<Step[], GenerateStepsRequest>({
        path: `/flows/${flowId}/generate-steps`,
        method: "POST",
        body,
      }).then((items) => (Array.isArray(items) ? items : [])),
    getStep: (stepId) =>
      request<Step>({
        path: `/steps/${stepId}`,
      }),
    updateStep: (stepId, body) =>
      request<Step, UpdateStepRequest>({
        path: `/steps/${stepId}`,
        method: "PUT",
        body,
      }),
    deleteStep: (stepId) =>
      request<void>({
        path: `/steps/${stepId}`,
        method: "DELETE",
      }),

    listExecutions: (stepId) =>
      request<Execution[]>({
        path: `/steps/${stepId}/executions`,
      }).then((items) => (Array.isArray(items) ? items : [])),
    getExecution: (execId) =>
      request<Execution>({
        path: `/executions/${execId}`,
      }),

    listEvents: (params) =>
      request<Event[]>({
        path: "/events",
        query: {
          flow_id: params?.flow_id,
          step_id: params?.step_id,
          session_id: params?.session_id,
          types: params?.types?.join(","),
          limit: params?.limit,
          offset: params?.offset,
        },
      }).then((items) => (Array.isArray(items) ? items : [])),
    listFlowEvents: (flowId, params) =>
      request<Event[]>({
        path: `/flows/${flowId}/events`,
        query: {
          types: params?.types?.join(","),
          limit: params?.limit,
          offset: params?.offset,
        },
      }).then((items) => (Array.isArray(items) ? items : [])),
    listDrivers: () =>
      request<AgentDriver[]>({
        path: "/agents/drivers",
      }).then((items) => (Array.isArray(items) ? items : [])),
    createDriver: (body) =>
      request<AgentDriver, AgentDriver>({
        path: "/agents/drivers",
        method: "POST",
        body,
      }),
    listProfiles: () =>
      request<AgentProfile[]>({
        path: "/agents/profiles",
      }).then((items) => (Array.isArray(items) ? items : [])),
    createProfile: (body) =>
      request<AgentProfile, AgentProfile>({
        path: "/agents/profiles",
        method: "POST",
        body,
      }),
    listSkills: () =>
      request<SkillInfo[]>({
        path: "/skills",
      }).then((items) => (Array.isArray(items) ? items : [])),
    getSkill: (name) =>
      request<SkillDetail>({
        path: `/skills/${encodeURIComponent(name)}`,
      }),
    createSkill: (body) =>
      request<SkillInfo, CreateSkillRequest>({
        path: "/skills",
        method: "POST",
        body,
      }),
    importGitHubSkill: (body) =>
      request<SkillInfo, ImportGitHubSkillRequest>({
        path: "/skills/import/github",
        method: "POST",
        body,
      }),

    // DAG Templates
    listDAGTemplates: (params) =>
      request<DAGTemplate[]>({
        path: "/templates",
        query: {
          project_id: params?.project_id,
          tag: params?.tag,
          search: params?.search,
          limit: params?.limit,
          offset: params?.offset,
        },
      }).then((items) => (Array.isArray(items) ? items : [])),
    createDAGTemplate: (body) =>
      request<DAGTemplate, CreateDAGTemplateRequest>({
        path: "/templates",
        method: "POST",
        body,
      }),
    getDAGTemplate: (templateId) =>
      request<DAGTemplate>({
        path: `/templates/${templateId}`,
      }),
    updateDAGTemplate: (templateId, body) =>
      request<DAGTemplate, UpdateDAGTemplateRequest>({
        path: `/templates/${templateId}`,
        method: "PUT",
        body,
      }),
    deleteDAGTemplate: (templateId) =>
      request<void>({
        path: `/templates/${templateId}`,
        method: "DELETE",
      }),
    saveFlowAsTemplate: (flowId, body) =>
      request<DAGTemplate, SaveFlowAsTemplateRequest>({
        path: `/flows/${flowId}/save-as-template`,
        method: "POST",
        body,
      }),
    createFlowFromTemplate: (templateId, body) =>
      request<CreateFlowFromTemplateResponse, CreateFlowFromTemplateRequest>({
        path: `/templates/${templateId}/create-flow`,
        method: "POST",
        body,
      }),
  };
};
