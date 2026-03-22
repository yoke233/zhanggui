import type {
  FeatureEntry,
  FeatureManifestSummary,
  FeatureManifestSnapshot,
  FeatureStatus,
  BootstrapPRWorkItemRequest,
  BootstrapPRWorkItemResponse,
  DriverConfig,
  CancelWorkItemResponse,
  AgentProfile,
  AnalyticsFilter,
  AnalyticsSummary,
  CronStatus,
  SetupCronRequest,
  CreateSkillRequest,
  StatsResponse,
  AdminSystemEventRequest,
  AdminSystemEventResponse,
  ChatRequest,
  ChatResponse,
  ChatSessionDetail,
  ChatSessionSummary,
  ChatStatusResponse,
  CreateResourceSpaceRequest,
  CreateProjectRequest,
  AnalyzeRequirementRequest,
  AnalyzeRequirementResponse,
  CreateThreadFromRequirementRequest,
  CreateThreadFromRequirementResponse,
  CreateWorkItemRequest,
  UpdateWorkItemRequest,
  WorkItem,
  CreateActionRequest,
  GenerateActionsRequest,
  Event,
  Run,
  ImportGitHubSkillRequest,
  Project,
  Resource,
  ResourceSpace,
  RunWorkItemResponse,
  SchedulerStats,
  SkillDetail,
  SkillInfo,
  Action,
  UpdateActionRequest,
  UpdateProjectRequest,
  DAGTemplate,
  CreateDAGTemplateRequest,
  UpdateDAGTemplateRequest,
  SaveWorkItemAsTemplateRequest,
  CreateWorkItemFromTemplateRequest,
  CreateWorkItemFromTemplateResponse,
  GitCommitEntry,
  GitStats,
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
  ThreadMember,
  AddThreadParticipantRequest,
  ThreadAttachment,
  ThreadFileRef,
  ThreadInitiativeLink,
  ThreadWorkItemLink,
  ThreadProposal,
  CreateThreadWorkItemLinkRequest,
  CreateThreadProposalRequest,
  UpdateThreadProposalRequest,
  ReplaceProposalDraftsRequest,
  ReviewProposalRequest,
  Initiative,
  InitiativeDetail,
  ApproveInitiativeRequest,
  RejectInitiativeRequest,
  Notification,
  CreateNotificationRequest,
  UnreadCountResponse,
  InspectionReport,
  InspectionFinding,
  InspectionInsight,
  TriggerInspectionRequest,
} from "../types/apiV2";
import type {
  LLMConfigResponse,
  SandboxSupportResponse,
  UpdateLLMConfigRequest,
  UpdateSandboxSupportRequest,
} from "../types/system";

export interface DetectGitInfoResponse {
  is_git: boolean;
  remote_url?: string;
  current_branch?: string;
  default_branch?: string;
}

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

const normalizeCronStatus = (value: unknown): CronStatus | null => {
  if (!value || typeof value !== "object") {
    return null;
  }

  const raw = value as Record<string, unknown>;
  const workItemID = raw.work_item_id;
  if (typeof workItemID !== "number") {
    return null;
  }

  return {
    work_item_id: workItemID,
    enabled: raw.enabled === true,
    is_template: raw.is_template === true,
    schedule: typeof raw.schedule === "string" ? raw.schedule : undefined,
    max_instances: typeof raw.max_instances === "number" ? raw.max_instances : undefined,
    last_triggered: typeof raw.last_triggered === "string" ? raw.last_triggered : undefined,
  };
};

export interface ApiClient {
  request<TResponse, TBody = unknown>(
    options: RequestOptions<TBody>,
  ): Promise<TResponse>;

  getStats(): Promise<StatsResponse>;
  getSchedulerStats(): Promise<SchedulerStats>;
  getSandboxSupport(): Promise<SandboxSupportResponse>;
  updateSandboxSupport(body: UpdateSandboxSupportRequest): Promise<SandboxSupportResponse>;
  getLLMConfig(): Promise<LLMConfigResponse>;
  updateLLMConfig(body: UpdateLLMConfigRequest): Promise<LLMConfigResponse>;
  sendSystemEvent(body: AdminSystemEventRequest): Promise<AdminSystemEventResponse>;

  listProjects(params?: { limit?: number; offset?: number }): Promise<Project[]>;
  createProject(body: CreateProjectRequest): Promise<Project>;
  getProject(projectId: number): Promise<Project>;
  updateProject(projectId: number, body: UpdateProjectRequest): Promise<Project>;
  deleteProject(projectId: number): Promise<void>;
  analyzeRequirement(body: AnalyzeRequirementRequest): Promise<AnalyzeRequirementResponse>;
  createThreadFromRequirement(body: CreateThreadFromRequirementRequest): Promise<CreateThreadFromRequirementResponse>;

  listProjectResources(projectId: number): Promise<ResourceSpace[]>;
  createProjectResource(projectId: number, body: CreateResourceSpaceRequest): Promise<ResourceSpace>;
  getProjectResource(resourceId: number): Promise<ResourceSpace>;
  deleteProjectResource(resourceId: number): Promise<void>;
  getFileResource(resourceId: number): Promise<Resource>;
  deleteFileResource(resourceId: number): Promise<void>;
  listRunResources(runId: number): Promise<Resource[]>;

  chat(body: ChatRequest): Promise<ChatResponse>;
  listChatSessions(): Promise<ChatSessionSummary[]>;
  getChatSession(sessionId: string): Promise<ChatSessionDetail>;
  cancelChat(sessionId: string): Promise<{ session_id: string; status: string }>;
  closeChat(sessionId: string): Promise<{ session_id: string; status: string }>;
  archiveChatSession(sessionId: string, archived: boolean): Promise<{ session_id: string; archived: boolean }>;
  renameChatSession(sessionId: string, title: string): Promise<{ session_id: string; title: string }>;
  getChatStatus(sessionId: string): Promise<ChatStatusResponse>;
  createChatPR(sessionId: string, title?: string, body?: string): Promise<GitStats>;
  refreshChatPR(sessionId: string): Promise<GitStats>;

  listWorkItems(params?: {
    project_id?: number;
    status?: string;
    archived?: boolean | "all";
    limit?: number;
    offset?: number;
  }): Promise<WorkItem[]>;
  createWorkItem(body: CreateWorkItemRequest): Promise<WorkItem>;
  getWorkItem(workItemId: number): Promise<WorkItem>;
  runWorkItem(workItemId: number): Promise<RunWorkItemResponse>;
  cancelWorkItem(workItemId: number): Promise<CancelWorkItemResponse>;
  updateWorkItem(workItemId: number, body: UpdateWorkItemRequest): Promise<WorkItem>;
  archiveWorkItem(workItemId: number): Promise<void>;
  bootstrapPRWorkItem(workItemId: number, body?: BootstrapPRWorkItemRequest): Promise<BootstrapPRWorkItemResponse>;

  listActions(workItemId: number): Promise<Action[]>;
  createAction(workItemId: number, body: CreateActionRequest): Promise<Action>;
  generateActions(workItemId: number, body: GenerateActionsRequest): Promise<Action[]>;
  generateTitle(body: { description: string }): Promise<{ title: string }>;
  getAction(actionId: number): Promise<Action>;
  updateAction(actionId: number, body: UpdateActionRequest): Promise<Action>;
  deleteAction(actionId: number): Promise<void>;

  listRuns(actionId: number): Promise<Run[]>;
  getRun(runId: number): Promise<Run>;

  listEvents(params?: {
    work_item_id?: number;
    action_id?: number;
    session_id?: string;
    types?: string[];
    limit?: number;
    offset?: number;
  }): Promise<Event[]>;
  listWorkItemEvents(
    workItemId: number,
    params?: { types?: string[]; limit?: number; offset?: number },
  ): Promise<Event[]>;

  listProfiles(): Promise<AgentProfile[]>;
  createProfile(body: AgentProfile): Promise<AgentProfile>;
  updateProfile(profileId: string, body: AgentProfile): Promise<AgentProfile>;
  deleteProfile(profileId: string): Promise<void>;
  listDrivers(): Promise<DriverConfig[]>;
  createDriver(body: DriverConfig): Promise<DriverConfig>;
  updateDriver(driverId: string, body: DriverConfig): Promise<DriverConfig>;
  deleteDriver(driverId: string): Promise<void>;
  listSkills(): Promise<SkillInfo[]>;
  getSkill(name: string): Promise<SkillDetail>;
  createSkill(body: CreateSkillRequest): Promise<SkillInfo>;
  updateSkill(name: string, body: { skill_md: string }): Promise<SkillInfo>;
  deleteSkill(name: string): Promise<void>;
  importGitHubSkill(body: ImportGitHubSkillRequest): Promise<SkillInfo>;

  getAnalyticsSummary(params?: AnalyticsFilter): Promise<AnalyticsSummary>;
  getUsageSummary(params?: AnalyticsFilter): Promise<UsageAnalyticsSummary>;
  getUsageByRun(runId: number): Promise<UsageRecord>;

  listCronWorkItems(): Promise<CronStatus[]>;
  getWorkItemCronStatus(workItemId: number): Promise<CronStatus>;
  setupWorkItemCron(workItemId: number, body: SetupCronRequest): Promise<CronStatus>;
  disableWorkItemCron(workItemId: number): Promise<CronStatus>;

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
  saveWorkItemAsTemplate(workItemId: number, body: SaveWorkItemAsTemplateRequest): Promise<DAGTemplate>;
  createWorkItemFromTemplate(templateId: number, body: CreateWorkItemFromTemplateRequest): Promise<CreateWorkItemFromTemplateResponse>;

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
  listThreadParticipants(threadId: number): Promise<ThreadMember[]>;
  addThreadParticipant(threadId: number, body: AddThreadParticipantRequest): Promise<ThreadMember>;
  removeThreadParticipant(threadId: number, userId: string): Promise<void>;
  listThreadProposals(threadId: number, params?: { status?: string }): Promise<ThreadProposal[]>;
  createThreadProposal(threadId: number, body: CreateThreadProposalRequest): Promise<ThreadProposal>;
  getProposal(proposalId: number): Promise<ThreadProposal>;
  updateProposal(proposalId: number, body: UpdateThreadProposalRequest): Promise<ThreadProposal>;
  deleteProposal(proposalId: number): Promise<void>;
  replaceProposalDrafts(proposalId: number, body: ReplaceProposalDraftsRequest): Promise<ThreadProposal>;
  submitProposal(proposalId: number): Promise<ThreadProposal>;
  approveProposal(proposalId: number, body: ReviewProposalRequest): Promise<ThreadProposal>;
  rejectProposal(proposalId: number, body: ReviewProposalRequest): Promise<ThreadProposal>;
  reviseProposal(proposalId: number, body: ReviewProposalRequest): Promise<ThreadProposal>;

  // Thread-WorkItem Links
  createThreadWorkItemLink(threadId: number, body: CreateThreadWorkItemLinkRequest): Promise<ThreadWorkItemLink>;
  listWorkItemsByThread(threadId: number): Promise<ThreadWorkItemLink[]>;
  deleteThreadWorkItemLink(threadId: number, workItemId: number): Promise<void>;
  listThreadsByWorkItem(workItemId: number): Promise<ThreadWorkItemLink[]>;
  createWorkItemFromThread(threadId: number, body: { title: string; body?: string; project_id?: number }): Promise<WorkItem>;
  listInitiatives(params?: { status?: string; limit?: number; offset?: number }): Promise<Initiative[]>;
  getInitiative(initiativeId: number): Promise<InitiativeDetail>;
  proposeInitiative(initiativeId: number): Promise<Initiative>;
  approveInitiative(initiativeId: number, body: ApproveInitiativeRequest): Promise<Initiative>;
  rejectInitiative(initiativeId: number, body: RejectInitiativeRequest): Promise<Initiative>;
  cancelInitiative(initiativeId: number): Promise<Initiative>;
  listInitiativeThreads(initiativeId: number): Promise<ThreadInitiativeLink[]>;
  // Thread Agent Sessions
  inviteThreadAgent(threadId: number, body: { agent_profile_id: string }): Promise<ThreadMember>;
  listThreadAgents(threadId: number): Promise<ThreadMember[]>;
  removeThreadAgent(threadId: number, agentSessionId: number): Promise<void>;

  // Thread Attachments
  uploadThreadAttachment(threadId: number, file: File, opts?: { note?: string; uploadedBy?: string }): Promise<ThreadAttachment>;
  listThreadAttachments(threadId: number): Promise<ThreadAttachment[]>;
  deleteThreadAttachment(threadId: number, attachmentId: number): Promise<void>;
  getThreadAttachmentDownloadUrl(threadId: number, attachmentId: number): string;
  searchThreadFiles(threadId: number, query?: string, source?: "all" | "attachment" | "project" | "workspace", limit?: number): Promise<ThreadFileRef[]>;

  // Work Item Attachments
  uploadWorkItemAttachment(workItemId: number, file: File): Promise<Resource>;
  listWorkItemAttachments(workItemId: number): Promise<Resource[]>;
  getWorkItemAttachment(attachmentId: number): Promise<Resource>;
  deleteWorkItemAttachment(attachmentId: number): Promise<void>;
  getAttachmentDownloadUrl(attachmentId: number): string;

  // Notifications
  listNotifications(params?: {
    category?: string;
    level?: string;
    read?: boolean;
    project_id?: number;
    work_item_id?: number;
    limit?: number;
    offset?: number;
  }): Promise<Notification[]>;
  createNotification(body: CreateNotificationRequest): Promise<Notification>;
  getNotification(notificationId: number): Promise<Notification>;
  markNotificationRead(notificationId: number): Promise<void>;
  markAllNotificationsRead(): Promise<void>;
  deleteNotification(notificationId: number): Promise<void>;
  getUnreadNotificationCount(): Promise<UnreadCountResponse>;

  // Utility
  detectGitInfo(path: string): Promise<DetectGitInfoResponse>;

  // Feature Manifest
  getOrCreateManifest(projectId: number): Promise<FeatureManifestSummary>;
  getManifest(projectId: number): Promise<FeatureManifestSummary>;
  getManifestSummary(projectId: number): Promise<FeatureManifestSummary>;
  getManifestSnapshot(projectId: number): Promise<FeatureManifestSnapshot>;
  listManifestEntries(
    projectId: number,
    params?: { status?: FeatureStatus; limit?: number; offset?: number },
  ): Promise<FeatureEntry[]>;
  createManifestEntry(
    projectId: number,
    body: { key: string; description: string; status?: FeatureStatus; tags?: string[] },
  ): Promise<FeatureEntry>;
  updateManifestEntryStatus(entryId: number, status: FeatureStatus): Promise<FeatureEntry>;
  updateManifestEntry(
    entryId: number,
    body: Partial<{ key: string; description: string; status: FeatureStatus; tags: string[] }>,
  ): Promise<FeatureEntry>;
  deleteManifestEntry(entryId: number): Promise<void>;

  // Inspections (self-evolving inspection system)
  listInspections(params?: {
    project_id?: number;
    status?: string;
    since?: string;
    until?: string;
    limit?: number;
    offset?: number;
  }): Promise<InspectionReport[]>;
  getInspection(inspectionId: number): Promise<InspectionReport>;
  triggerInspection(body?: TriggerInspectionRequest): Promise<InspectionReport>;
  listInspectionFindings(inspectionId: number): Promise<InspectionFinding[]>;
  listInspectionInsights(inspectionId: number): Promise<InspectionInsight[]>;
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

  // -- Work Items (primary) --
  const listWorkItems: ApiClient["listWorkItems"] = (params) =>
    request<WorkItem[]>({
      path: "/work-items",
      query: {
        project_id: params?.project_id,
        status: params?.status,
        archived: params?.archived === undefined ? undefined : String(params.archived),
        limit: params?.limit,
        offset: params?.offset,
      },
    }).then((items) => (Array.isArray(items) ? items : []));

  const createWorkItem: ApiClient["createWorkItem"] = (body) =>
    request<WorkItem, CreateWorkItemRequest>({
      path: "/work-items",
      method: "POST",
      body,
    });

  const getWorkItem: ApiClient["getWorkItem"] = (workItemId) =>
    request<WorkItem>({
      path: `/work-items/${workItemId}`,
    });

  const runWorkItem: ApiClient["runWorkItem"] = (workItemId) =>
    request<RunWorkItemResponse>({
      path: `/work-items/${workItemId}/run`,
      method: "POST",
    });

  const cancelWorkItem: ApiClient["cancelWorkItem"] = (workItemId) =>
    request<CancelWorkItemResponse>({
      path: `/work-items/${workItemId}/cancel`,
      method: "POST",
    });

  const updateWorkItem: ApiClient["updateWorkItem"] = (workItemId, body) =>
    request<WorkItem, UpdateWorkItemRequest>({
      path: `/work-items/${workItemId}`,
      method: "PUT",
      body,
    });

  const archiveWorkItem: ApiClient["archiveWorkItem"] = (workItemId) =>
    request<void>({
      path: `/work-items/${workItemId}/archive`,
      method: "POST",
    });

  const bootstrapPRWorkItem: ApiClient["bootstrapPRWorkItem"] = (workItemId, body) =>
    request<BootstrapPRWorkItemResponse, BootstrapPRWorkItemRequest>({
      path: `/work-items/${workItemId}/bootstrap-pr`,
      method: "POST",
      body,
    });

  // -- Actions (primary) --
  const listActions: ApiClient["listActions"] = (workItemId) =>
    request<Action[]>({
      path: `/work-items/${workItemId}/actions`,
    }).then((items) => (Array.isArray(items) ? items : []));

  const createAction: ApiClient["createAction"] = (workItemId, body) =>
    request<Action, CreateActionRequest>({
      path: `/work-items/${workItemId}/actions`,
      method: "POST",
      body,
    });

  const generateActions: ApiClient["generateActions"] = (workItemId, body) =>
    request<Action[], GenerateActionsRequest>({
      path: `/work-items/${workItemId}/generate-actions`,
      method: "POST",
      body,
    }).then((items) => (Array.isArray(items) ? items : []));

  const getAction: ApiClient["getAction"] = (actionId) =>
    request<Action>({
      path: `/actions/${actionId}`,
    });

  const updateAction: ApiClient["updateAction"] = (actionId, body) =>
    request<Action, UpdateActionRequest>({
      path: `/actions/${actionId}`,
      method: "PUT",
      body,
    });

  const deleteAction: ApiClient["deleteAction"] = (actionId) =>
    request<void>({
      path: `/actions/${actionId}`,
      method: "DELETE",
    });

  // -- Runs (primary) --
  const listRuns: ApiClient["listRuns"] = (actionId) =>
    request<Run[]>({
      path: `/actions/${actionId}/runs`,
    }).then((items) => (Array.isArray(items) ? items : []));

  const getRun: ApiClient["getRun"] = (runId) =>
    request<Run>({
      path: `/runs/${runId}`,
    });

  // -- Run resources --
  const listRunResources: ApiClient["listRunResources"] = (runId) =>
    request<Resource[]>({
      path: `/runs/${runId}/resources`,
    }).then((items) => (Array.isArray(items) ? items : []));

  // -- Events --
  const listEvents: ApiClient["listEvents"] = (params) =>
    request<Event[]>({
      path: "/events",
      query: {
        work_item_id: params?.work_item_id,
        action_id: params?.action_id,
        session_id: params?.session_id,
        types: params?.types?.join(","),
        limit: params?.limit,
        offset: params?.offset,
      },
    }).then((items) => (Array.isArray(items) ? items : []));

  const listWorkItemEvents: ApiClient["listWorkItemEvents"] = (workItemId, params) =>
    request<Event[]>({
      path: `/work-items/${workItemId}/events`,
      query: {
        types: params?.types?.join(","),
        limit: params?.limit,
        offset: params?.offset,
      },
    }).then((items) => (Array.isArray(items) ? items : []));

  // -- Cron (primary) --
  const listCronWorkItems: ApiClient["listCronWorkItems"] = () =>
    request<CronStatus[]>({
      path: "/work-items/cron",
    }).then((items) => (
      Array.isArray(items)
        ? items.map((item) => normalizeCronStatus(item)).filter((item): item is CronStatus => item !== null)
        : []
    ));

  const getWorkItemCronStatus: ApiClient["getWorkItemCronStatus"] = (workItemId) =>
    request<CronStatus>({
      path: `/work-items/${workItemId}/cron`,
    }).then((item) => {
      const normalized = normalizeCronStatus(item);
      if (!normalized) {
        throw new ApiError(500, "Invalid cron status response", item);
      }
      return normalized;
    });

  const setupWorkItemCron: ApiClient["setupWorkItemCron"] = (workItemId, body) =>
    request<CronStatus, SetupCronRequest>({
      path: `/work-items/${workItemId}/cron`,
      method: "POST",
      body,
    }).then((item) => {
      const normalized = normalizeCronStatus(item);
      if (!normalized) {
        throw new ApiError(500, "Invalid cron status response", item);
      }
      return normalized;
    });

  const disableWorkItemCron: ApiClient["disableWorkItemCron"] = (workItemId) =>
    request<CronStatus>({
      path: `/work-items/${workItemId}/cron`,
      method: "DELETE",
    }).then((item) => {
      const normalized = normalizeCronStatus(item);
      if (!normalized) {
        throw new ApiError(500, "Invalid cron status response", item);
      }
      return normalized;
    });

  // -- Templates (primary) --
  const saveWorkItemAsTemplate: ApiClient["saveWorkItemAsTemplate"] = (workItemId, body) =>
    request<DAGTemplate, SaveWorkItemAsTemplateRequest>({
      path: `/work-items/${workItemId}/save-as-template`,
      method: "POST",
      body,
    });

  const createWorkItemFromTemplate: ApiClient["createWorkItemFromTemplate"] = (templateId, body) =>
    request<CreateWorkItemFromTemplateResponse, CreateWorkItemFromTemplateRequest>({
      path: `/templates/${templateId}/create-work-item`,
      method: "POST",
      body,
    });

  // -- Work Item Attachments (primary) --
  const uploadWorkItemAttachment: ApiClient["uploadWorkItemAttachment"] = async (workItemId, file) => {
    const url = buildUrl(baseUrl, `/work-items/${workItemId}/resources`);
    const formData = new FormData();
    formData.append("file", file);
    const headers: Record<string, string> = {};
    const token = getToken?.();
    if (token) {
      headers["Authorization"] = `Bearer ${token}`;
    }
    const response = await fetchImpl(url, {
      method: "POST",
      headers,
      body: formData,
    });
    const data = await readResponseData(response);
    if (!response.ok) {
      throw new ApiError(response.status, extractErrorMessage(response.status, data), data);
    }
    return data as Resource;
  };

  const listWorkItemAttachments: ApiClient["listWorkItemAttachments"] = (workItemId) =>
    request<Resource[]>({
      path: `/work-items/${workItemId}/resources`,
    }).then((items) => (Array.isArray(items) ? items : []));

  const getWorkItemAttachment: ApiClient["getWorkItemAttachment"] = (attachmentId) =>
    request<Resource>({
      path: `/resources/${attachmentId}`,
    });

  const deleteWorkItemAttachment: ApiClient["deleteWorkItemAttachment"] = (attachmentId) =>
    request<void>({
      path: `/resources/${attachmentId}`,
      method: "DELETE",
    });

  // -- Usage --
  const getUsageByRun: ApiClient["getUsageByRun"] = (runId) =>
    request<UsageRecord>({
      path: `/runs/${runId}/usage`,
    });

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
    getLLMConfig: () =>
      request<LLMConfigResponse>({
        path: "/admin/system/llm-config",
      }),
    updateLLMConfig: (body) =>
      request<LLMConfigResponse, UpdateLLMConfigRequest>({
        path: "/admin/system/llm-config",
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
    analyzeRequirement: (body) =>
      request<AnalyzeRequirementResponse, AnalyzeRequirementRequest>({
        path: "/requirements/analyze",
        method: "POST",
        body,
      }),
    createThreadFromRequirement: (body) =>
      request<CreateThreadFromRequirementResponse, CreateThreadFromRequirementRequest>({
        path: "/requirements/create-thread",
        method: "POST",
        body,
      }),

    listProjectResources: (projectId) =>
      request<ResourceSpace[]>({
        path: `/projects/${projectId}/spaces`,
      }).then((items) => (Array.isArray(items) ? items : [])),
    createProjectResource: (projectId, body) =>
      request<ResourceSpace, CreateResourceSpaceRequest>({
        path: `/projects/${projectId}/spaces`,
        method: "POST",
        body,
      }),
    getProjectResource: (resourceId) =>
      request<ResourceSpace>({
        path: `/spaces/${resourceId}`,
      }),
    deleteProjectResource: (resourceId) =>
      request<void>({
        path: `/spaces/${resourceId}`,
        method: "DELETE",
      }),
    getFileResource: (resourceId) =>
      request<Resource>({
        path: `/resources/${resourceId}`,
      }),
    deleteFileResource: (resourceId) =>
      request<void>({
        path: `/resources/${resourceId}`,
        method: "DELETE",
      }),
    listRunResources,

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
    archiveChatSession: (sessionId, archived) =>
      request<{ session_id: string; archived: boolean }>({
        path: `/chat/sessions/${encodeURIComponent(sessionId)}/archive`,
        method: "POST",
        body: { archived },
      }),
    renameChatSession: (sessionId, title) =>
      request<{ session_id: string; title: string }>({
        path: `/chat/sessions/${encodeURIComponent(sessionId)}/rename`,
        method: "PATCH",
        body: { title },
      }),
    getChatStatus: (sessionId) =>
      request<ChatStatusResponse>({
        path: `/chat/${encodeURIComponent(sessionId)}/status`,
      }),
    createChatPR: (sessionId, title, body) =>
      request<GitStats>({
        path: `/chat/sessions/${encodeURIComponent(sessionId)}/create-pr`,
        method: "POST",
        body: { title, body },
      }),
    refreshChatPR: (sessionId) =>
      request<GitStats>({
        path: `/chat/sessions/${encodeURIComponent(sessionId)}/refresh-pr`,
        method: "POST",
      }),

    // Work Items
    listWorkItems,
    createWorkItem,
    getWorkItem,
    runWorkItem,
    cancelWorkItem,
    updateWorkItem,
    archiveWorkItem,
    bootstrapPRWorkItem,

    // Actions
    listActions,
    createAction,
    generateActions,
    generateTitle: (body) =>
      request<{ title: string }, { description: string }>({
        path: `/work-items/generate-title`,
        method: "POST",
        body,
      }),
    getAction,
    updateAction,
    deleteAction,

    // Runs
    listRuns,
    getRun,

    // Events
    listEvents,
    listWorkItemEvents,

    listProfiles: () =>
      request<AgentProfile[]>({
        path: "/agents/profiles",
      }).then((items) => (Array.isArray(items) ? items : [])),
    listDrivers: () =>
      request<DriverConfig[]>({
        path: "/agents/drivers",
      }).then((items) => (Array.isArray(items) ? items : [])),
    createDriver: (body) =>
      request<DriverConfig, DriverConfig>({
        path: "/agents/drivers",
        method: "POST",
        body,
      }),
    updateDriver: (driverId, body) =>
      request<DriverConfig, DriverConfig>({
        path: `/agents/drivers/${encodeURIComponent(driverId)}`,
        method: "PUT",
        body,
      }),
    deleteDriver: (driverId) =>
      request<void>({
        path: `/agents/drivers/${encodeURIComponent(driverId)}`,
        method: "DELETE",
      }),
    createProfile: (body) =>
      request<AgentProfile, AgentProfile>({
        path: "/agents/profiles",
        method: "POST",
        body,
      }),
    updateProfile: (profileId, body) =>
      request<AgentProfile, AgentProfile>({
        path: `/agents/profiles/${encodeURIComponent(profileId)}`,
        method: "PUT",
        body,
      }),
    deleteProfile: (profileId) =>
      request<void>({
        path: `/agents/profiles/${encodeURIComponent(profileId)}`,
        method: "DELETE",
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
    getUsageByRun,

    // Cron
    listCronWorkItems,
    getWorkItemCronStatus,
    setupWorkItemCron,
    disableWorkItemCron,

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
    saveWorkItemAsTemplate,
    createWorkItemFromTemplate,

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
      request<ThreadMember[]>({
        path: `/threads/${threadId}/participants`,
      }).then((items) => (Array.isArray(items) ? items : [])),
    addThreadParticipant: (threadId: number, body: AddThreadParticipantRequest) =>
      request<ThreadMember, AddThreadParticipantRequest>({
        path: `/threads/${threadId}/participants`,
        method: "POST",
        body,
      }),
    removeThreadParticipant: (threadId: number, userId: string) =>
      request<void>({
        path: `/threads/${threadId}/participants/${encodeURIComponent(userId)}`,
        method: "DELETE",
      }),
    listThreadProposals: (threadId, params) =>
      request<ThreadProposal[]>({
        path: `/threads/${threadId}/proposals`,
        query: { status: params?.status },
      }).then((items) => (Array.isArray(items) ? items : [])),
    createThreadProposal: (threadId, body) =>
      request<ThreadProposal, CreateThreadProposalRequest>({
        path: `/threads/${threadId}/proposals`,
        method: "POST",
        body,
      }),
    getProposal: (proposalId) =>
      request<ThreadProposal>({
        path: `/proposals/${proposalId}`,
      }),
    updateProposal: (proposalId, body) =>
      request<ThreadProposal, UpdateThreadProposalRequest>({
        path: `/proposals/${proposalId}`,
        method: "PUT",
        body,
      }),
    deleteProposal: (proposalId) =>
      request<void>({
        path: `/proposals/${proposalId}`,
        method: "DELETE",
      }),
    replaceProposalDrafts: (proposalId, body) =>
      request<ThreadProposal, ReplaceProposalDraftsRequest>({
        path: `/proposals/${proposalId}/drafts`,
        method: "PUT",
        body,
      }),
    submitProposal: (proposalId) =>
      request<ThreadProposal>({
        path: `/proposals/${proposalId}/submit`,
        method: "POST",
      }),
    approveProposal: (proposalId, body) =>
      request<ThreadProposal, ReviewProposalRequest>({
        path: `/proposals/${proposalId}/approve`,
        method: "POST",
        body,
      }),
    rejectProposal: (proposalId, body) =>
      request<ThreadProposal, ReviewProposalRequest>({
        path: `/proposals/${proposalId}/reject`,
        method: "POST",
        body,
      }),
    reviseProposal: (proposalId, body) =>
      request<ThreadProposal, ReviewProposalRequest>({
        path: `/proposals/${proposalId}/revise`,
        method: "POST",
        body,
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
    listThreadsByWorkItem: (workItemId) =>
      request<ThreadWorkItemLink[]>({
        path: `/work-items/${workItemId}/threads`,
      }).then((items) => (Array.isArray(items) ? items : [])),
    createWorkItemFromThread: (threadId, body) =>
      request<WorkItem, { title: string; body?: string; project_id?: number }>({
        path: `/threads/${threadId}/create-work-item`,
        method: "POST",
        body,
      }),
    listInitiatives: (params) =>
      request<Initiative[]>({
        path: "/initiatives",
        query: {
          status: params?.status,
          limit: params?.limit,
          offset: params?.offset,
        },
      }).then((items) => (Array.isArray(items) ? items : [])),
    getInitiative: (initiativeId) =>
      request<InitiativeDetail>({
        path: `/initiatives/${initiativeId}`,
      }),
    proposeInitiative: (initiativeId) =>
      request<Initiative>({
        path: `/initiatives/${initiativeId}/propose`,
        method: "POST",
      }),
    approveInitiative: (initiativeId, body) =>
      request<Initiative, ApproveInitiativeRequest>({
        path: `/initiatives/${initiativeId}/approve`,
        method: "POST",
        body,
      }),
    rejectInitiative: (initiativeId, body) =>
      request<Initiative, RejectInitiativeRequest>({
        path: `/initiatives/${initiativeId}/reject`,
        method: "POST",
        body,
      }),
    cancelInitiative: (initiativeId) =>
      request<Initiative>({
        path: `/initiatives/${initiativeId}/cancel`,
        method: "POST",
      }),
    listInitiativeThreads: (initiativeId) =>
      request<ThreadInitiativeLink[]>({
        path: `/initiatives/${initiativeId}/threads`,
      }).then((items) => (Array.isArray(items) ? items : [])),
    inviteThreadAgent: (threadId, body) =>
      request<ThreadMember, { agent_profile_id: string }>({
        path: `/threads/${threadId}/agents`,
        method: "POST",
        body,
      }),
    listThreadAgents: (threadId) =>
      request<ThreadMember[]>({
        path: `/threads/${threadId}/agents`,
      }).then((items) => (Array.isArray(items) ? items : [])),
    removeThreadAgent: (threadId, agentSessionId) =>
      request<void>({
        path: `/threads/${threadId}/agents/${agentSessionId}`,
        method: "DELETE",
      }),

    // Thread Attachments
    uploadThreadAttachment: async (threadId, file, opts) => {
      const form = new FormData();
      form.append("file", file);
      if (opts?.note) form.append("note", opts.note);
      if (opts?.uploadedBy) form.append("uploaded_by", opts.uploadedBy);
      const url = buildUrl(baseUrl, `/threads/${threadId}/attachments`);
      const headers: Record<string, string> = {};
      const tok = getToken?.();
      if (tok) headers["Authorization"] = `Bearer ${tok}`;
      const res = await fetch(url, { method: "POST", headers, body: form });
      if (!res.ok) {
        const text = await res.text().catch(() => res.statusText);
        throw new Error(text || `upload failed: ${res.status}`);
      }
      return res.json() as Promise<ThreadAttachment>;
    },
    listThreadAttachments: (threadId) =>
      request<ThreadAttachment[]>({
        path: `/threads/${threadId}/attachments`,
      }).then((items) => (Array.isArray(items) ? items : [])),
    deleteThreadAttachment: (threadId, attachmentId) =>
      request<void>({
        path: `/threads/${threadId}/attachments/${attachmentId}`,
        method: "DELETE",
      }),
    getThreadAttachmentDownloadUrl: (threadId, attachmentId) =>
      buildUrl(baseUrl, `/threads/${threadId}/attachments/${attachmentId}`),
    searchThreadFiles: (threadId, query, source, limit) => {
      const params = new URLSearchParams();
      if (query) params.set("q", query);
      if (source) params.set("source", source);
      if (limit) params.set("limit", String(limit));
      const qs = params.toString();
      return request<ThreadFileRef[]>({
        path: `/threads/${threadId}/files${qs ? `?${qs}` : ""}`,
      }).then((items) => (Array.isArray(items) ? items : []));
    },

    // Utility
    detectGitInfo: (path: string) =>
      request<DetectGitInfoResponse, { path: string }>({
        path: "/utils/detect-git",
        method: "POST",
        body: { path },
      }),

    // Feature Manifest
    getOrCreateManifest: (projectId: number) =>
      request<FeatureManifestSummary>({ path: `/projects/${projectId}/manifest` }),
    getManifest: (projectId: number) =>
      request<FeatureManifestSummary>({ path: `/projects/${projectId}/manifest` }),
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

    // Work Item Attachments
    uploadWorkItemAttachment,
    listWorkItemAttachments,
    getWorkItemAttachment,
    deleteWorkItemAttachment,
    getAttachmentDownloadUrl: (attachmentId) =>
      buildUrl(baseUrl, `/resources/${attachmentId}/download`),

    // Notifications
    listNotifications: (params) =>
      request<Notification[]>({
        path: "/notifications",
        query: {
          category: params?.category,
          level: params?.level,
          read: params?.read === undefined ? undefined : String(params.read),
          project_id: params?.project_id,
          work_item_id: params?.work_item_id,
          limit: params?.limit,
          offset: params?.offset,
        },
      }).then((items) => (Array.isArray(items) ? items : [])),
    createNotification: (body) =>
      request<Notification, CreateNotificationRequest>({
        path: "/notifications",
        method: "POST",
        body,
      }),
    getNotification: (notificationId) =>
      request<Notification>({ path: `/notifications/${notificationId}` }),
    markNotificationRead: (notificationId) =>
      request<void>({ path: `/notifications/${notificationId}/read`, method: "POST" }),
    markAllNotificationsRead: () =>
      request<void>({ path: "/notifications/read-all", method: "POST" }),
    deleteNotification: (notificationId) =>
      request<void>({ path: `/notifications/${notificationId}`, method: "DELETE" }),
    getUnreadNotificationCount: () =>
      request<UnreadCountResponse>({ path: "/notifications/unread-count" }),

    // Inspections (self-evolving inspection system)
    listInspections: (params) =>
      request<InspectionReport[]>({
        path: "/inspections",
        query: {
          project_id: params?.project_id,
          status: params?.status,
          since: params?.since,
          until: params?.until,
          limit: params?.limit,
          offset: params?.offset,
        },
      }).then((items) => (Array.isArray(items) ? items : [])),
    getInspection: (inspectionId) =>
      request<InspectionReport>({ path: `/inspections/${inspectionId}` }),
    triggerInspection: (body) =>
      request<InspectionReport, TriggerInspectionRequest>({
        path: "/inspections/trigger",
        method: "POST",
        body: body ?? {},
      }),
    listInspectionFindings: (inspectionId) =>
      request<InspectionFinding[]>({ path: `/inspections/${inspectionId}/findings` }).then(
        (items) => (Array.isArray(items) ? items : []),
      ),
    listInspectionInsights: (inspectionId) =>
      request<InspectionInsight[]>({ path: `/inspections/${inspectionId}/insights` }).then(
        (items) => (Array.isArray(items) ? items : []),
      ),
  };
};
