import type {
  FeatureManifest,
  FeatureEntry,
  FeatureManifestSummary,
  FeatureManifestSnapshot,
  FeatureStatus,
  BootstrapPRIssueRequest,
  BootstrapPRIssueResponse,
  CancelIssueResponse,
  AgentDriver,
  AgentProfile,
  AnalyticsFilter,
  AnalyticsSummary,
  CronStatus,
  SetupCronRequest,
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
  CreateIssueRequest,
  Issue,
  CreateStepRequest,
  GenerateStepsRequest,
  Event,
  Execution,
  ImportGitHubSkillRequest,
  Project,
  ResourceBinding,
  RunIssueResponse,
  SchedulerStats,
  SkillDetail,
  SkillInfo,
  Step,
  UpdateStepRequest,
  UpdateProjectRequest,
  DAGTemplate,
  CreateDAGTemplateRequest,
  UpdateDAGTemplateRequest,
  SaveIssueAsTemplateRequest,
  CreateIssueFromTemplateRequest,
  CreateIssueFromTemplateResponse,
  GitCommitEntry,
  GitTagEntry,
  CreateGitTagRequest,
  CreateGitTagResponse,
  PushGitTagRequest,
  PushGitTagResponse,
  UsageAnalyticsSummary,
  UsageRecord,
  Thread,
  CreateThreadRequest,
  UpdateThreadRequest,
  ThreadMessage,
  CreateThreadMessageRequest,
  ThreadParticipant,
  AddThreadParticipantRequest,
  ThreadWorkItemLink,
  CreateThreadWorkItemLinkRequest,
  ThreadAgentSession,
} from "../types/apiV2";
import type { SandboxSupportResponse, UpdateSandboxSupportRequest } from "../types/system";

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
  updateSandboxSupport(body: UpdateSandboxSupportRequest): Promise<SandboxSupportResponse>;
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

  listIssues(params?: {
    project_id?: number;
    status?: string;
    archived?: boolean | "all";
    limit?: number;
    offset?: number;
  }): Promise<Issue[]>;
  createIssue(body: CreateIssueRequest): Promise<Issue>;
  getIssue(issueId: number): Promise<Issue>;
  runIssue(issueId: number): Promise<RunIssueResponse>;
  cancelIssue(issueId: number): Promise<CancelIssueResponse>;
  updateIssue(issueId: number, body: UpdateIssueRequest): Promise<Issue>;
  archiveIssue(issueId: number): Promise<void>;
  bootstrapPRIssue(issueId: number, body?: BootstrapPRIssueRequest): Promise<BootstrapPRIssueResponse>;

  listSteps(issueId: number): Promise<Step[]>;
  createStep(issueId: number, body: CreateStepRequest): Promise<Step>;
  generateSteps(issueId: number, body: GenerateStepsRequest): Promise<Step[]>;
  generateTitle(body: { description: string }): Promise<{ title: string }>;
  getStep(stepId: number): Promise<Step>;
  updateStep(stepId: number, body: UpdateStepRequest): Promise<Step>;
  deleteStep(stepId: number): Promise<void>;

  listExecutions(stepId: number): Promise<Execution[]>;
  getExecution(execId: number): Promise<Execution>;

  listEvents(params?: {
    issue_id?: number;
    step_id?: number;
    session_id?: string;
    types?: string[];
    limit?: number;
    offset?: number;
  }): Promise<Event[]>;
  listIssueEvents(
    issueId: number,
    params?: { types?: string[]; limit?: number; offset?: number },
  ): Promise<Event[]>;

  listDrivers(): Promise<AgentDriver[]>;
  createDriver(body: AgentDriver): Promise<AgentDriver>;
  listProfiles(): Promise<AgentProfile[]>;
  createProfile(body: AgentProfile): Promise<AgentProfile>;
  listSkills(): Promise<SkillInfo[]>;
  getSkill(name: string): Promise<SkillDetail>;
  createSkill(body: CreateSkillRequest): Promise<SkillInfo>;
  updateSkill(name: string, body: { skill_md: string }): Promise<SkillInfo>;
  deleteSkill(name: string): Promise<void>;
  importGitHubSkill(body: ImportGitHubSkillRequest): Promise<SkillInfo>;

  getAnalyticsSummary(params?: AnalyticsFilter): Promise<AnalyticsSummary>;
  getUsageSummary(params?: AnalyticsFilter): Promise<UsageAnalyticsSummary>;
  getUsageByExecution(execId: number): Promise<UsageRecord>;

  listCronIssues(): Promise<CronStatus[]>;
  getIssueCronStatus(issueId: number): Promise<CronStatus>;
  setupIssueCron(issueId: number, body: SetupCronRequest): Promise<CronStatus>;
  disableIssueCron(issueId: number): Promise<CronStatus>;

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
  saveIssueAsTemplate(issueId: number, body: SaveIssueAsTemplateRequest): Promise<DAGTemplate>;
  createIssueFromTemplate(templateId: number, body: CreateIssueFromTemplateRequest): Promise<CreateIssueFromTemplateResponse>;

  // Git Tags
  listGitCommits(projectId: number, params?: { limit?: number }): Promise<GitCommitEntry[]>;
  listGitTags(projectId: number): Promise<GitTagEntry[]>;
  createGitTag(projectId: number, body: CreateGitTagRequest): Promise<CreateGitTagResponse>;
  pushGitTag(projectId: number, body: PushGitTagRequest): Promise<PushGitTagResponse>;

  // Threads
  listThreads(params?: { status?: string; limit?: number; offset?: number }): Promise<Thread[]>;
  createThread(body: CreateThreadRequest): Promise<Thread>;
  getThread(threadId: number): Promise<Thread>;
  updateThread(threadId: number, body: UpdateThreadRequest): Promise<Thread>;
  deleteThread(threadId: number): Promise<void>;
  listThreadMessages(threadId: number, params?: { limit?: number; offset?: number }): Promise<ThreadMessage[]>;
  createThreadMessage(threadId: number, body: CreateThreadMessageRequest): Promise<ThreadMessage>;
  listThreadParticipants(threadId: number): Promise<ThreadParticipant[]>;
  addThreadParticipant(threadId: number, body: AddThreadParticipantRequest): Promise<ThreadParticipant>;
  removeThreadParticipant(threadId: number, userId: string): Promise<void>;

  // Thread-WorkItem Links
  createThreadWorkItemLink(threadId: number, body: CreateThreadWorkItemLinkRequest): Promise<ThreadWorkItemLink>;
  listWorkItemsByThread(threadId: number): Promise<ThreadWorkItemLink[]>;
  deleteThreadWorkItemLink(threadId: number, workItemId: number): Promise<void>;
  listThreadsByWorkItem(issueId: number): Promise<ThreadWorkItemLink[]>;
  createWorkItemFromThread(threadId: number, body: { title: string; body?: string; project_id?: number }): Promise<Issue>;

  // Thread Agent Sessions
  inviteThreadAgent(threadId: number, body: { agent_profile_id: string }): Promise<ThreadAgentSession>;
  listThreadAgents(threadId: number): Promise<ThreadAgentSession[]>;
  removeThreadAgent(threadId: number, agentSessionId: number): Promise<void>;
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
    updateSandboxSupport: (body) =>
      request<SandboxSupportResponse, UpdateSandboxSupportRequest>({
        path: "/admin/system/sandbox-support",
        method: "PUT",
        body,
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
    listIssues: (params) =>
      request<Issue[]>({
        path: "/issues",
        query: {
          project_id: params?.project_id,
          status: params?.status,
          archived: params?.archived === undefined ? undefined : String(params.archived),
          limit: params?.limit,
          offset: params?.offset,
        },
      }).then((items) => (Array.isArray(items) ? items : [])),
    createIssue: (body) =>
      request<Issue, CreateIssueRequest>({
        path: "/issues",
        method: "POST",
        body,
      }),
    getIssue: (issueId) =>
      request<Issue>({
        path: `/issues/${issueId}`,
      }),
    runIssue: (issueId) =>
      request<RunIssueResponse>({
        path: `/issues/${issueId}/run`,
        method: "POST",
      }),
    cancelIssue: (issueId) =>
      request<CancelIssueResponse>({
        path: `/issues/${issueId}/cancel`,
        method: "POST",
      }),
    updateIssue: (issueId, body) =>
      request<Issue, UpdateIssueRequest>({
        path: `/issues/${issueId}`,
        method: "PUT",
        body,
      }),
    archiveIssue: (issueId) =>
      request<void>({
        path: `/issues/${issueId}/archive`,
        method: "POST",
      }),
    bootstrapPRIssue: (issueId, body) =>
      request<BootstrapPRIssueResponse, BootstrapPRIssueRequest>({
        path: `/issues/${issueId}/bootstrap-pr`,
        method: "POST",
        body,
      }),

    listSteps: (issueId) =>
      request<Step[]>({
        path: `/issues/${issueId}/steps`,
      }).then((items) => (Array.isArray(items) ? items : [])),
    createStep: (issueId, body) =>
      request<Step, CreateStepRequest>({
        path: `/issues/${issueId}/steps`,
        method: "POST",
        body,
      }),
    generateSteps: (issueId, body) =>
      request<Step[], GenerateStepsRequest>({
        path: `/issues/${issueId}/generate-steps`,
        method: "POST",
        body,
      }).then((items) => (Array.isArray(items) ? items : [])),
    generateTitle: (body) =>
      request<{ title: string }, { description: string }>({
        path: `/issues/generate-title`,
        method: "POST",
        body,
      }),
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
          issue_id: params?.issue_id,
          step_id: params?.step_id,
          session_id: params?.session_id,
          types: params?.types?.join(","),
          limit: params?.limit,
          offset: params?.offset,
        },
      }).then((items) => (Array.isArray(items) ? items : [])),
    listIssueEvents: (issueId, params) =>
      request<Event[]>({
        path: `/issues/${issueId}/events`,
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
    updateSkill: (name, body) =>
      request<SkillInfo, { skill_md: string }>({
        path: `/skills/${encodeURIComponent(name)}`,
        method: "PUT",
        body,
      }),
    deleteSkill: (name) =>
      request<void>({
        path: `/skills/${encodeURIComponent(name)}`,
        method: "DELETE",
      }),
    importGitHubSkill: (body) =>
      request<SkillInfo, ImportGitHubSkillRequest>({
        path: "/skills/import/github",
        method: "POST",
        body,
      }),

    getAnalyticsSummary: (params) =>
      request<AnalyticsSummary>({
        path: "/analytics/summary",
        query: {
          project_id: params?.project_id,
          since: params?.since,
          until: params?.until,
          limit: params?.limit,
        },
      }),
    getUsageSummary: (params) =>
      request<UsageAnalyticsSummary>({
        path: "/analytics/usage",
        query: {
          project_id: params?.project_id,
          since: params?.since,
          until: params?.until,
          limit: params?.limit,
        },
      }),
    getUsageByExecution: (execId) =>
      request<UsageRecord>({
        path: `/executions/${execId}/usage`,
      }),

    listCronIssues: () =>
      request<CronStatus[]>({
        path: "/cron/issues",
      }).then((items) => (Array.isArray(items) ? items : [])),
    getIssueCronStatus: (issueId) =>
      request<CronStatus>({
        path: `/issues/${issueId}/cron`,
      }),
    setupIssueCron: (issueId, body) =>
      request<CronStatus, SetupCronRequest>({
        path: `/issues/${issueId}/cron`,
        method: "POST",
        body,
      }),
    disableIssueCron: (issueId) =>
      request<CronStatus>({
        path: `/issues/${issueId}/cron`,
        method: "DELETE",
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
    saveIssueAsTemplate: (issueId, body) =>
      request<DAGTemplate, SaveIssueAsTemplateRequest>({
        path: `/issues/${issueId}/save-as-template`,
        method: "POST",
        body,
      }),
    createIssueFromTemplate: (templateId, body) =>
      request<CreateIssueFromTemplateResponse, CreateIssueFromTemplateRequest>({
        path: `/templates/${templateId}/create-issue`,
        method: "POST",
        body,
      }),

    // Git Tags
    listGitCommits: (projectId, params) =>
      request<GitCommitEntry[]>({
        path: `/projects/${projectId}/git/commits`,
        query: { limit: params?.limit },
      }).then((items) => (Array.isArray(items) ? items : [])),
    listGitTags: (projectId) =>
      request<GitTagEntry[]>({
        path: `/projects/${projectId}/git/tags`,
      }).then((items) => (Array.isArray(items) ? items : [])),
    createGitTag: (projectId, body) =>
      request<CreateGitTagResponse, CreateGitTagRequest>({
        path: `/projects/${projectId}/git/tags`,
        method: "POST",
        body,
      }),
    pushGitTag: (projectId, body) =>
      request<PushGitTagResponse, PushGitTagRequest>({
        path: `/projects/${projectId}/git/tags/push`,
        method: "POST",
        body,
      }),

    // Threads
    listThreads: (params?: { status?: string; limit?: number; offset?: number }) =>
      request<Thread[]>({
        path: "/threads",
        query: { status: params?.status, limit: params?.limit, offset: params?.offset },
      }).then((items) => (Array.isArray(items) ? items : [])),
    createThread: (body: CreateThreadRequest) =>
      request<Thread, CreateThreadRequest>({ path: "/threads", method: "POST", body }),
    getThread: (threadId: number) =>
      request<Thread>({ path: `/threads/${threadId}` }),
    updateThread: (threadId: number, body: UpdateThreadRequest) =>
      request<Thread, UpdateThreadRequest>({ path: `/threads/${threadId}`, method: "PUT", body }),
    deleteThread: (threadId: number) =>
      request<void>({ path: `/threads/${threadId}`, method: "DELETE" }),

    // Thread Messages
    listThreadMessages: (threadId: number, params?: { limit?: number; offset?: number }) =>
      request<ThreadMessage[]>({
        path: `/threads/${threadId}/messages`,
        query: { limit: params?.limit, offset: params?.offset },
      }).then((items) => (Array.isArray(items) ? items : [])),
    createThreadMessage: (threadId: number, body: CreateThreadMessageRequest) =>
      request<ThreadMessage, CreateThreadMessageRequest>({
        path: `/threads/${threadId}/messages`,
        method: "POST",
        body,
      }),

    // Thread Participants
    listThreadParticipants: (threadId: number) =>
      request<ThreadParticipant[]>({
        path: `/threads/${threadId}/participants`,
      }).then((items) => (Array.isArray(items) ? items : [])),
    addThreadParticipant: (threadId: number, body: AddThreadParticipantRequest) =>
      request<ThreadParticipant, AddThreadParticipantRequest>({
        path: `/threads/${threadId}/participants`,
        method: "POST",
        body,
      }),
    removeThreadParticipant: (threadId: number, userId: string) =>
      request<void>({
        path: `/threads/${threadId}/participants/${encodeURIComponent(userId)}`,
        method: "DELETE",
      }),
    createThreadWorkItemLink: (threadId, body) =>
      request<ThreadWorkItemLink, CreateThreadWorkItemLinkRequest>({
        path: `/threads/${threadId}/links/work-items`,
        method: "POST",
        body,
      }),
    listWorkItemsByThread: (threadId) =>
      request<ThreadWorkItemLink[]>({
        path: `/threads/${threadId}/work-items`,
      }).then((items) => (Array.isArray(items) ? items : [])),
    deleteThreadWorkItemLink: (threadId, workItemId) =>
      request<void>({
        path: `/threads/${threadId}/links/work-items/${workItemId}`,
        method: "DELETE",
      }),
    listThreadsByWorkItem: (issueId) =>
      request<ThreadWorkItemLink[]>({
        path: `/issues/${issueId}/threads`,
      }).then((items) => (Array.isArray(items) ? items : [])),
    createWorkItemFromThread: (threadId, body) =>
      request<Issue, { title: string; body?: string; project_id?: number }>({
        path: `/threads/${threadId}/create-work-item`,
        method: "POST",
        body,
      }),
    inviteThreadAgent: (threadId, body) =>
      request<ThreadAgentSession, { agent_profile_id: string }>({
        path: `/threads/${threadId}/agents`,
        method: "POST",
        body,
      }),
    listThreadAgents: (threadId) =>
      request<ThreadAgentSession[]>({
        path: `/threads/${threadId}/agents`,
      }).then((items) => (Array.isArray(items) ? items : [])),
    removeThreadAgent: (threadId, agentSessionId) =>
      request<void>({
        path: `/threads/${threadId}/agents/${agentSessionId}`,
        method: "DELETE",
      }),

    // Feature Manifest
    getOrCreateManifest: async (projectId: number) => {
      try {
        return await request<FeatureManifest>({ path: `/projects/${projectId}/manifest` });
      } catch (err) {
        if (err instanceof ApiError && err.status === 404) {
          return request<FeatureManifest, { summary: string }>({
            path: `/projects/${projectId}/manifest`,
            method: "POST",
            body: { summary: "" },
          });
        }
        throw err;
      }
    },
    getManifest: (projectId: number) =>
      request<FeatureManifest>({ path: `/projects/${projectId}/manifest` }),
    getManifestSummary: (projectId: number) =>
      request<FeatureManifestSummary>({ path: `/projects/${projectId}/manifest/summary` }),
    getManifestSnapshot: (projectId: number) =>
      request<FeatureManifestSnapshot>({ path: `/projects/${projectId}/manifest/snapshot` }),
    listManifestEntries: (projectId: number, params?: { status?: FeatureStatus; limit?: number; offset?: number }) =>
      request<FeatureEntry[]>({ path: `/projects/${projectId}/manifest/entries`, query: params }).then(
        (items) => (Array.isArray(items) ? items : []),
      ),
    createManifestEntry: (projectId: number, body: { key: string; description: string; status?: FeatureStatus; tags?: string[] }) =>
      request<FeatureEntry, typeof body>({
        path: `/projects/${projectId}/manifest/entries`,
        method: "POST",
        body,
      }),
    updateManifestEntryStatus: (entryId: number, status: FeatureStatus) =>
      request<FeatureEntry, { status: FeatureStatus }>({
        path: `/manifest/entries/${entryId}/status`,
        method: "PATCH",
        body: { status },
      }),
    updateManifestEntry: (entryId: number, body: Partial<{ key: string; description: string; status: FeatureStatus; tags: string[] }>) =>
      request<FeatureEntry, typeof body>({
        path: `/manifest/entries/${entryId}`,
        method: "PUT",
        body,
      }),
    deleteManifestEntry: (entryId: number) =>
      request<void>({ path: `/manifest/entries/${entryId}`, method: "DELETE" }),
  };
};
