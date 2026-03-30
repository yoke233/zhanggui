import type {
  ApproveInitiativeRequest,
  ChatRequest,
  ChatResponse,
  ChatSessionDetail,
  ChatSessionSummary,
  ChatStatusResponse,
  CreateThreadMessageRequest,
  CreateThreadProposalRequest,
  CreateThreadRequest,
  CreateThreadWorkItemLinkRequest,
  Initiative,
  InitiativeDetail,
  RejectInitiativeRequest,
  ReplaceProposalDraftsRequest,
  ReviewProposalRequest,
  Thread,
  ThreadAttachment,
  ThreadFileRef,
  ThreadInitiativeLink,
  ThreadMember,
  ThreadMessage,
  ThreadProposal,
  ThreadWorkItemLink,
  UpdateThreadProposalRequest,
  UpdateThreadRequest,
  GitStats,
  WorkItem,
  AddThreadParticipantRequest,
} from "../types/apiV2";
import type { ApiClient } from "./apiClient";
import type { ApiBuilderContext } from "./apiClient.shared";

export const buildCollaborationApi = ({
  request,
  buildUrl,
}: ApiBuilderContext): Pick<
  ApiClient,
  | "chat"
  | "listChatSessions"
  | "getChatSession"
  | "cancelChat"
  | "closeChat"
  | "archiveChatSession"
  | "renameChatSession"
  | "getChatStatus"
  | "submitChatCode"
  | "createChatPR"
  | "refreshChatPR"
  | "listThreads"
  | "createThread"
  | "getThread"
  | "updateThread"
  | "deleteThread"
  | "listThreadMessages"
  | "createThreadMessage"
  | "listThreadParticipants"
  | "addThreadParticipant"
  | "removeThreadParticipant"
  | "listThreadProposals"
  | "createThreadProposal"
  | "getProposal"
  | "updateProposal"
  | "deleteProposal"
  | "replaceProposalDrafts"
  | "submitProposal"
  | "approveProposal"
  | "rejectProposal"
  | "reviseProposal"
  | "createThreadWorkItemLink"
  | "listWorkItemsByThread"
  | "deleteThreadWorkItemLink"
  | "listThreadsByWorkItem"
  | "createWorkItemFromThread"
  | "listInitiatives"
  | "getInitiative"
  | "proposeInitiative"
  | "approveInitiative"
  | "rejectInitiative"
  | "cancelInitiative"
  | "listInitiativeThreads"
  | "inviteThreadAgent"
  | "listThreadAgents"
  | "removeThreadAgent"
  | "uploadThreadAttachment"
  | "listThreadAttachments"
  | "deleteThreadAttachment"
  | "getThreadAttachmentDownloadUrl"
  | "searchThreadFiles"
> => ({
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
  submitChatCode: (sessionId, message) =>
    request<GitStats>({
      path: `/chat/sessions/${encodeURIComponent(sessionId)}/submit-code`,
      method: "POST",
      body: { message },
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
  listThreads: (params) =>
    request<Thread[]>({
      path: "/threads",
      query: { status: params?.status, limit: params?.limit, offset: params?.offset },
    }).then((items) => (Array.isArray(items) ? items : [])),
  createThread: (body) =>
    request<Thread, CreateThreadRequest>({ path: "/threads", method: "POST", body }),
  getThread: (threadId) =>
    request<Thread>({ path: `/threads/${threadId}` }),
  updateThread: (threadId, body) =>
    request<Thread, UpdateThreadRequest>({ path: `/threads/${threadId}`, method: "PUT", body }),
  deleteThread: (threadId) =>
    request<void>({ path: `/threads/${threadId}`, method: "DELETE" }),
  listThreadMessages: (threadId, params) =>
    request<ThreadMessage[]>({
      path: `/threads/${threadId}/messages`,
      query: { limit: params?.limit, offset: params?.offset },
    }).then((items) => (Array.isArray(items) ? items : [])),
  createThreadMessage: (threadId, body) =>
    request<ThreadMessage, CreateThreadMessageRequest>({
      path: `/threads/${threadId}/messages`,
      method: "POST",
      body,
    }),
  listThreadParticipants: (threadId) =>
    request<ThreadMember[]>({
      path: `/threads/${threadId}/participants`,
    }).then((items) => (Array.isArray(items) ? items : [])),
  addThreadParticipant: (threadId, body) =>
    request<ThreadMember, AddThreadParticipantRequest>({
      path: `/threads/${threadId}/participants`,
      method: "POST",
      body,
    }),
  removeThreadParticipant: (threadId, userId) =>
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
  uploadThreadAttachment: async (threadId, file, opts) => {
    const form = new FormData();
    form.append("file", file);
    if (opts?.note) form.append("note", opts.note);
    if (opts?.uploadedBy) form.append("uploaded_by", opts.uploadedBy);
    return request<ThreadAttachment, FormData>({
      path: `/threads/${threadId}/attachments`,
      method: "POST",
      body: form,
      bodyMode: "raw",
      responseType: "json",
    });
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
    buildUrl(`/threads/${threadId}/attachments/${attachmentId}`),
  searchThreadFiles: (threadId, query, source, limit) =>
    request<ThreadFileRef[]>({
      path: `/threads/${threadId}/files`,
      query: {
        q: query,
        source,
        limit,
      },
    }).then((items) => (Array.isArray(items) ? items : [])),
});
