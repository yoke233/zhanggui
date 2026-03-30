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
import {
  createHttpTransport,
  type HttpTransportOptions,
  type Primitive,
} from "./httpTransport";
import { buildAgentAdminApi } from "./apiClient.agentAdmin";
import { buildCollaborationApi } from "./apiClient.collaboration";
import { buildInsightApi } from "./apiClient.insight";
import { buildProjectApi } from "./apiClient.projects";
import { normalizeCronStatus } from "./apiClient.shared";
import { buildWorkflowApi } from "./apiClient.workflow";

export interface DetectGitInfoResponse {
  is_git: boolean;
  remote_url?: string;
  current_branch?: string;
  default_branch?: string;
}

export interface RequestOptions<TBody = unknown> {
  path: string;
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  query?: Record<string, Primitive | null | undefined>;
  body?: TBody;
  headers?: HeadersInit;
  signal?: AbortSignal;
  responseType?: "auto" | "json" | "text" | "void";
  bodyMode?: "json" | "raw";
  omitAuth?: boolean;
}

export interface ApiClientOptions extends HttpTransportOptions {}

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
  submitChatCode(sessionId: string, message?: string): Promise<GitStats>;
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
  const transport = createHttpTransport(opts);

  const request = async <TResponse, TBody = unknown>(
    options: RequestOptions<TBody>,
  ): Promise<TResponse> => transport.request<TResponse, TBody>(options);

  const client: ApiClient = {
    request,
    ...buildAgentAdminApi({
      request,
      buildUrl: transport.buildUrl,
      normalizeCronStatus,
    }),
    ...buildProjectApi({
      request,
      buildUrl: transport.buildUrl,
      normalizeCronStatus,
    }),
    ...buildWorkflowApi({
      request,
      buildUrl: transport.buildUrl,
      normalizeCronStatus,
    }),
    ...buildCollaborationApi({
      request,
      buildUrl: transport.buildUrl,
      normalizeCronStatus,
    }),
    ...buildInsightApi({
      request,
      buildUrl: transport.buildUrl,
      normalizeCronStatus,
    }),
  };

  return client;
};
