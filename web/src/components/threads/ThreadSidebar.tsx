import { useRef, useState } from "react";
import type { MutableRefObject, ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import {
  Bot,
  Check,
  ChevronDown,
  ChevronRight,
  ClipboardList,
  File,
  FileText,
  Paperclip,
  Info,
  Link2,
  Loader2,
  Plus,
  User,
  Users,
  X,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import { formatRelativeTime } from "@/lib/v2Workbench";
import type {
  AgentProfile,
  Issue,
  Thread,
  ThreadAttachment,
  ThreadAgentSession,
  ThreadParticipant,
  ThreadProposal,
  ThreadWorkItemLink,
  ThreadTaskGroup,
  WorkItemPriority,
} from "@/types/apiV2";

/* ── Accordion section primitive ── */

function SidebarSection({
  id,
  icon: Icon,
  label,
  count,
  badge,
  openSections,
  onToggle,
  children,
}: {
  id: string;
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  count?: number;
  badge?: ReactNode;
  openSections: Set<string>;
  onToggle: (id: string) => void;
  children: ReactNode;
}) {
  const isOpen = openSections.has(id);
  return (
    <div className="border-b border-border/40 last:border-b-0">
      <button
        type="button"
        className="flex w-full items-center gap-2 px-4 py-2.5 text-left text-xs font-medium text-slate-600 transition-colors hover:bg-slate-50"
        onClick={() => onToggle(id)}
      >
        {isOpen ? (
          <ChevronDown className="h-3 w-3 shrink-0 text-slate-400" />
        ) : (
          <ChevronRight className="h-3 w-3 shrink-0 text-slate-400" />
        )}
        <Icon className="h-3.5 w-3.5 shrink-0 text-slate-500" />
        <span className="flex-1 uppercase tracking-wider">{label}</span>
        {count != null && count > 0 && (
          <span className="rounded-full bg-slate-100 px-1.5 py-0.5 text-[10px] tabular-nums text-slate-500">
            {count}
          </span>
        )}
        {badge}
      </button>
      {isOpen && <div className="px-4 pb-3">{children}</div>}
    </div>
  );
}

/* ── Types ── */

type AgentSessionWithProfileID = ThreadAgentSession & { agent_profile_id: string };

type ProposalDraftEditor = {
  temp_id: string;
  project_id: string;
  title: string;
  body: string;
  priority: WorkItemPriority;
  depends_on: string;
  labels: string;
};

type ProposalEditor = {
  proposalId: number | null;
  title: string;
  summary: string;
  content: string;
  proposedBy: string;
  sourceMessageId: string;
  drafts: ProposalDraftEditor[];
};

type ProposalReviewInput = {
  reviewedBy: string;
  reviewNote: string;
};

export interface ThreadSidebarProps {
  thread: Thread;
  messagesCount: number;

  // Members: agents
  inviteableProfiles: AgentProfile[];
  selectedInviteIDs: Set<string>;
  invitingAgent: boolean;
  onToggleInviteSelection: (profileID: string) => void;
  onInviteAgent: () => void;
  onClearInviteSelection?: () => void;
  agentSessionsWithProfileID: AgentSessionWithProfileID[];
  selectedDiscussionAgentIDs: Set<string>;
  profileByID: Map<string, AgentProfile>;
  highlightedAgentProfileID: string | null;
  agentCardRefs: MutableRefObject<Record<string, HTMLDivElement | null>>;
  removingAgentID: number | null;
  onRemoveAgent: (agentSessionID: number) => void;
  onToggleDiscussionAgentSelection: (profileID: string) => void;
  onStartDiscussionWithAgents: () => void;
  onClearDiscussionAgents: () => void;
  canStartDiscussionWithAgent: (status: string) => boolean;
  agentStatusColor: (status: string) => string;

  // Members: participants
  participants: ThreadParticipant[];

  // Proposals
  proposals: ThreadProposal[];
  proposalsLoading: boolean;
  showProposalEditor: boolean;
  proposalEditor: ProposalEditor;
  savingProposal: boolean;
  proposalActionLoadingID: number | null;
  proposalReviewInputs: Record<number, ProposalReviewInput>;
  onOpenCreateProposal: () => void;
  onOpenEditProposal: (proposal: ThreadProposal) => void;
  onShowProposalEditorChange: (open: boolean) => void;
  onProposalEditorFieldChange: (
    field: Exclude<keyof ProposalEditor, "drafts">,
    value: string,
  ) => void;
  onProposalDraftChange: (
    index: number,
    field: keyof ProposalDraftEditor,
    value: string,
  ) => void;
  onAddProposalDraft: () => void;
  onRemoveProposalDraft: (index: number) => void;
  onSaveProposal: () => void;
  onProposalReviewInputChange: (
    proposalId: number,
    field: keyof ProposalReviewInput,
    value: string,
  ) => void;
  onSubmitProposal: (proposalId: number) => void;
  onApproveProposal: (proposalId: number) => void;
  onRejectProposal: (proposalId: number) => void;
  onReviseProposal: (proposalId: number) => void;

  // Tasks: task groups
  threadTaskGroupsEnabled: boolean;
  onToggleThreadTaskGroups: () => void;
  taskGroups: ThreadTaskGroup[];
  taskGroupsLoading: boolean;
  taskGroupStatusTone: (status: string) => string;
  onDeleteTaskGroup: (groupId: number) => void;
  onRetryTaskGroup: (groupId: number) => void;

  // Tasks: work items
  workItemLinks: ThreadWorkItemLink[];
  orderedWorkItemLinks: ThreadWorkItemLink[];
  linkedIssues: Record<number, Issue>;
  showCreateWI: boolean;
  newWITitle: string;
  newWIBody: string;
  showLinkWI: boolean;
  linkWIId: string;
  onOpenCreateWorkItem: () => void;
  onShowCreateWIChange: (open: boolean) => void;
  onNewWITitleChange: (value: string) => void;
  onNewWIBodyChange: (value: string) => void;
  onCreateWorkItem: () => void;
  onShowLinkWIChange: (open: boolean) => void;
  onLinkWIIdChange: (value: string) => void;
  onLinkWorkItem: () => void;
  onResetCreateWorkItemDraft: () => void;

  // Files
  attachments: ThreadAttachment[];
  attachmentsLoading: boolean;
  onUploadAttachment: (file: File) => void;
  onDeleteAttachment: (id: number) => void;
  getAttachmentDownloadUrl: (threadId: number, attachmentId: number) => string;
}

/* ── Main component ── */

export function ThreadSidebar(props: ThreadSidebarProps) {
  const { t } = useTranslation();
  const [openSections, setOpenSections] = useState<Set<string>>(
    () => new Set(["members", "proposals"]),
  );
  const [showInvitePicker, setShowInvitePicker] = useState(false);

  const toggle = (id: string) => {
    setOpenSections((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const handleInvite = () => {
    props.onInviteAgent();
    setShowInvitePicker(false);
  };
  const handleCancelInvite = () => {
    setShowInvitePicker(false);
    props.onClearInviteSelection?.();
  };

  const memberCount =
    props.agentSessionsWithProfileID.length + props.participants.length;
  const taskCount =
    (props.threadTaskGroupsEnabled ? props.taskGroups.length : 0) +
    props.workItemLinks.length;

  return (
    <div className="flex h-full flex-col">
      <div className="flex-1 overflow-y-auto">
        {/* ── Members ── */}
        <SidebarSection
          id="members"
          icon={Users}
          label={t("threads.members", "Members")}
          count={memberCount}
          openSections={openSections}
          onToggle={toggle}
        >
          <MembersSection {...props} showInvitePicker={showInvitePicker} setShowInvitePicker={setShowInvitePicker} onInvite={handleInvite} onCancelInvite={handleCancelInvite} />
        </SidebarSection>

        {/* ── Tasks ── */}
        <SidebarSection
          id="proposals"
          icon={FileText}
          label={t("threads.proposals", "Proposals")}
          count={props.proposals.length}
          badge={
            props.proposalsLoading ? (
              <Loader2 className="h-3 w-3 animate-spin text-slate-400" />
            ) : undefined
          }
          openSections={openSections}
          onToggle={toggle}
        >
          <ProposalSection {...props} />
        </SidebarSection>

        <SidebarSection
          id="tasks"
          icon={ClipboardList}
          label={t("threads.tasks", "Tasks")}
          count={taskCount}
          openSections={openSections}
          onToggle={toggle}
        >
          <TasksSection {...props} />
        </SidebarSection>

        {/* ── Files ── */}
        <SidebarSection
          id="files"
          icon={Paperclip}
          label={t("threads.files", "Files")}
          count={props.attachments.length}
          openSections={openSections}
          onToggle={toggle}
        >
          <FilesSection
            threadId={props.thread.id}
            attachments={props.attachments}
            loading={props.attachmentsLoading}
            onUpload={props.onUploadAttachment}
            onDelete={props.onDeleteAttachment}
            getDownloadUrl={props.getAttachmentDownloadUrl}
          />
        </SidebarSection>

        {/* ── Info ── */}
        <SidebarSection
          id="info"
          icon={Info}
          label={t("threads.info", "Info")}
          openSections={openSections}
          onToggle={toggle}
        >
          <InfoSection thread={props.thread} messagesCount={props.messagesCount} />
        </SidebarSection>
      </div>
    </div>
  );
}

/* ── Members section ── */

function MembersSection({
  agentSessionsWithProfileID,
  selectedDiscussionAgentIDs,
  profileByID,
  highlightedAgentProfileID,
  agentCardRefs,
  removingAgentID,
  onRemoveAgent,
  onToggleDiscussionAgentSelection,
  onStartDiscussionWithAgents,
  onClearDiscussionAgents,
  canStartDiscussionWithAgent,
  agentStatusColor,
  participants,
  inviteableProfiles,
  selectedInviteIDs,
  invitingAgent,
  onToggleInviteSelection,
  showInvitePicker,
  setShowInvitePicker,
  onInvite,
  onCancelInvite,
}: ThreadSidebarProps & {
  showInvitePicker: boolean;
  setShowInvitePicker: (v: boolean) => void;
  onInvite: () => void;
  onCancelInvite: () => void;
}) {
  const { t } = useTranslation();

  return (
    <div className="space-y-3">
      {selectedDiscussionAgentIDs.size > 0 && (
        <div className="rounded-lg border border-blue-200 bg-blue-50/70 p-2.5">
          <div className="flex items-center justify-between gap-2">
            <span className="text-[11px] font-medium text-blue-700">
              {t("threads.selectedDiscussionAgents", {
                defaultValue: "已选择 {{count}} 个 agent 参与讨论",
                count: selectedDiscussionAgentIDs.size,
              })}
            </span>
            <div className="flex items-center gap-1">
              <Button
                size="sm"
                className="h-6 px-2 text-[10px]"
                onClick={onStartDiscussionWithAgents}
              >
                {t("threads.startDiscussion", {
                  defaultValue: "开始讨论",
                })}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                className="h-6 px-2 text-[10px]"
                onClick={onClearDiscussionAgents}
              >
                {t("common.clear", { defaultValue: "清空" })}
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* Agent list */}
      {agentSessionsWithProfileID.map((session) => {
        const profile = profileByID.get(session.agent_profile_id);
        const selectable = canStartDiscussionWithAgent(session.status ?? "unknown");
        const selectedForDiscussion = selectedDiscussionAgentIDs.has(
          session.agent_profile_id,
        );
        return (
          <div
            key={session.id}
            ref={(node) => {
              agentCardRefs.current[session.agent_profile_id] = node;
            }}
            data-testid={`agent-card-${session.agent_profile_id}`}
            className={cn(
              "flex items-center gap-2.5 rounded-lg border p-2 transition-all",
              highlightedAgentProfileID === session.agent_profile_id
                ? "border-blue-300 bg-blue-50 shadow-sm"
                : selectedForDiscussion
                  ? "border-emerald-300 bg-emerald-50/70"
                  : "border-border/50",
            )}
          >
            <button
              type="button"
              className={cn(
                "flex h-5 w-5 shrink-0 items-center justify-center rounded border transition-colors",
                selectedForDiscussion
                  ? "border-emerald-500 bg-emerald-500 text-white"
                  : "border-slate-300 bg-white text-transparent",
                !selectable && "cursor-not-allowed opacity-40",
              )}
              onClick={() =>
                selectable &&
                onToggleDiscussionAgentSelection(session.agent_profile_id)
              }
              disabled={!selectable}
              aria-label={t("threads.toggleDiscussionAgentAria", {
                defaultValue: "Select {{agent}} for discussion",
                agent: session.agent_profile_id,
              })}
              title={
                selectable
                  ? t("threads.selectAgentForDiscussion", {
                      defaultValue: "选择此 agent 参与讨论",
                    })
                  : t("threads.agentNotReadyForDiscussion", {
                      defaultValue: "只有 active 状态的 agent 才能参与讨论",
                    })
              }
            >
              <Check className="h-3 w-3" />
            </button>
            <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-emerald-100 text-emerald-700">
              <Bot className="h-3.5 w-3.5" />
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-1.5">
                <span className="truncate text-xs font-medium">
                  {profile?.name ?? session.agent_profile_id}
                </span>
                <span
                  className={cn(
                    "h-1.5 w-1.5 shrink-0 rounded-full",
                    agentStatusColor(session.status ?? "unknown"),
                  )}
                />
              </div>
              <div className="mt-0.5 text-[10px] text-slate-400">
                {session.turn_count ?? 0} turns
                {" / "}
                {(
                  ((session.total_input_tokens ?? 0) +
                    (session.total_output_tokens ?? 0)) /
                  1000
                ).toFixed(1)}
                k tokens
              </div>
            </div>
            <button
              type="button"
              className="flex h-5 w-5 shrink-0 items-center justify-center rounded text-slate-400 transition-colors hover:bg-rose-50 hover:text-rose-500"
              onClick={() => onRemoveAgent(session.id)}
              disabled={removingAgentID === session.id}
              aria-label={t("threads.removeAgentAria", {
                defaultValue: "Remove {{agent}}",
                agent: session.agent_profile_id,
              })}
            >
              {removingAgentID === session.id ? (
                <Loader2 className="h-3 w-3 animate-spin" />
              ) : (
                <X className="h-3 w-3" />
              )}
            </button>
          </div>
        );
      })}

      {/* Human participants (exclude agents — they're shown above) */}
      {participants.filter((p) => p.kind !== "agent").map((p) => (
        <div key={p.id} className="flex items-center gap-2.5 rounded-lg p-2">
          <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-slate-100 text-slate-500">
            <User className="h-3.5 w-3.5" />
          </div>
          <div className="min-w-0 flex-1">
            <span className="truncate text-xs font-medium">{p.user_id}</span>
          </div>
          <Badge variant="outline" className="text-[9px]">
            {p.role}
          </Badge>
        </div>
      ))}

      {/* Invite trigger / picker */}
      {!showInvitePicker ? (
        inviteableProfiles.length > 0 && (
          <button
            type="button"
            className="flex w-full items-center justify-center gap-1.5 rounded-lg border border-dashed border-slate-300 py-2 text-[11px] text-slate-500 transition-colors hover:border-slate-400 hover:bg-slate-50 hover:text-slate-600"
            onClick={() => setShowInvitePicker(true)}
          >
            <Plus className="h-3 w-3" />
            {t("threads.addAgent", "Add Agent")}
          </button>
        )
      ) : (
        <div className="space-y-2 rounded-lg border bg-slate-50/80 p-2.5">
          <div className="flex items-center justify-between">
            <span className="text-[11px] font-medium text-slate-600">
              {t("threads.selectAgents", "Select Agents")}
            </span>
            <div className="flex gap-1">
              {selectedInviteIDs.size > 0 && (
                <Button
                  size="sm"
                  className="h-6 px-2 text-[10px]"
                  onClick={onInvite}
                  disabled={invitingAgent}
                >
                  {invitingAgent ? (
                    <Loader2 className="mr-1 h-3 w-3 animate-spin" />
                  ) : (
                    <Check className="mr-1 h-3 w-3" />
                  )}
                  Add ({selectedInviteIDs.size})
                </Button>
              )}
              <Button
                size="sm"
                variant="ghost"
                className="h-6 px-2 text-[10px]"
                onClick={onCancelInvite}
                disabled={invitingAgent}
              >
                <X className="h-3 w-3" />
              </Button>
            </div>
          </div>
          {inviteableProfiles.map((profile) => {
            const isSelected = selectedInviteIDs.has(profile.id);
            return (
              <button
                key={profile.id}
                type="button"
                className={cn(
                  "flex w-full items-center gap-2 rounded-md border p-2 text-left transition-all",
                  isSelected
                    ? "border-blue-300 bg-blue-50"
                    : "border-transparent hover:bg-white",
                  invitingAgent && "pointer-events-none opacity-60",
                )}
                onClick={() => onToggleInviteSelection(profile.id)}
                disabled={invitingAgent}
              >
                <div
                  className={cn(
                    "flex h-3.5 w-3.5 shrink-0 items-center justify-center rounded border",
                    isSelected
                      ? "border-blue-500 bg-blue-500 text-white"
                      : "border-slate-300 bg-white",
                  )}
                >
                  {isSelected && <Check className="h-2.5 w-2.5" />}
                </div>
                <Bot className="h-3.5 w-3.5 shrink-0 text-emerald-600" />
                <span className="truncate text-[11px] font-medium">
                  {profile.name ?? profile.id}
                </span>
                <Badge variant="outline" className="ml-auto shrink-0 text-[8px]">
                  {profile.role}
                </Badge>
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}

/* ── Tasks section ── */

function proposalStatusTone(status: string): string {
  switch (status) {
    case "approved":
    case "merged":
      return "border-emerald-200 bg-emerald-50 text-emerald-700";
    case "open":
      return "border-blue-200 bg-blue-50 text-blue-700";
    case "rejected":
      return "border-rose-200 bg-rose-50 text-rose-700";
    case "revised":
      return "border-amber-200 bg-amber-50 text-amber-700";
    default:
      return "border-slate-200 bg-slate-50 text-slate-700";
  }
}

function ProposalSection({
  proposals,
  proposalsLoading,
  showProposalEditor,
  proposalEditor,
  savingProposal,
  proposalActionLoadingID,
  proposalReviewInputs,
  onOpenCreateProposal,
  onOpenEditProposal,
  onShowProposalEditorChange,
  onProposalEditorFieldChange,
  onProposalDraftChange,
  onAddProposalDraft,
  onRemoveProposalDraft,
  onSaveProposal,
  onProposalReviewInputChange,
  onSubmitProposal,
  onApproveProposal,
  onRejectProposal,
  onReviseProposal,
}: ThreadSidebarProps) {
  const { t } = useTranslation();

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <span className="text-[10px] font-semibold uppercase tracking-wider text-slate-400">
          Proposal Flow
          {proposalsLoading && (
            <Loader2 className="ml-1 inline h-3 w-3 animate-spin" />
          )}
        </span>
        <Button
          variant="ghost"
          size="sm"
          className="h-6 px-2 text-[10px]"
          onClick={onOpenCreateProposal}
        >
          <Plus className="mr-0.5 h-3 w-3" />
          {t("threads.newProposalAction", "New Proposal")}
        </Button>
      </div>

      {showProposalEditor && (
        <div className="space-y-2 rounded-lg border bg-slate-50/80 p-2.5">
          <div className="flex items-center justify-between">
            <span className="text-[11px] font-medium text-slate-700">
              {proposalEditor.proposalId == null
                ? t("threads.newProposal", "New proposal")
                : t("threads.editProposal", "Edit proposal")}
            </span>
            <Button
              variant="ghost"
              size="sm"
              className="h-6 px-2 text-[10px]"
              onClick={() => onShowProposalEditorChange(false)}
            >
              {t("common.cancel", "Cancel")}
            </Button>
          </div>
          <Input
            placeholder={t("threads.proposalTitle", "Proposal title")}
            className="h-7 text-xs"
            value={proposalEditor.title}
            onChange={(e) =>
              onProposalEditorFieldChange("title", e.target.value)
            }
          />
          <Input
            placeholder={t("threads.proposalSummary", "Short summary")}
            className="h-7 text-xs"
            value={proposalEditor.summary}
            onChange={(e) =>
              onProposalEditorFieldChange("summary", e.target.value)
            }
          />
          <Textarea
            placeholder={t("threads.proposalContent", "Decision details and plan")}
            className="min-h-[80px] resize-y text-xs"
            value={proposalEditor.content}
            onChange={(e) =>
              onProposalEditorFieldChange("content", e.target.value)
            }
          />
          <div className="grid grid-cols-2 gap-2">
            <Input
              placeholder={t("threads.proposedBy", "Proposed by")}
              className="h-7 text-xs"
              value={proposalEditor.proposedBy}
              onChange={(e) =>
                onProposalEditorFieldChange("proposedBy", e.target.value)
              }
            />
            <Input
              placeholder={t("threads.sourceMessageId", "Source message ID")}
              className="h-7 text-xs"
              value={proposalEditor.sourceMessageId}
              onChange={(e) =>
                onProposalEditorFieldChange("sourceMessageId", e.target.value)
              }
            />
          </div>

          <div className="space-y-2 rounded-md border border-dashed border-slate-200 p-2">
            <div className="flex items-center justify-between">
              <span className="text-[10px] font-semibold uppercase tracking-wider text-slate-400">
                Work Item Drafts
              </span>
              <Button
                variant="ghost"
                size="sm"
                className="h-6 px-2 text-[10px]"
                onClick={onAddProposalDraft}
              >
                <Plus className="mr-0.5 h-3 w-3" />
                {t("threads.addDraft", "Add")}
              </Button>
            </div>
            {proposalEditor.drafts.map((draft, index) => (
              <div
                key={`${proposalEditor.proposalId ?? "new"}-draft-${index}`}
                className="space-y-2 rounded-md border bg-white p-2"
              >
                <div className="grid grid-cols-2 gap-2">
                  <Input
                    placeholder="temp_id"
                    className="h-7 text-xs"
                    value={draft.temp_id}
                    onChange={(e) =>
                      onProposalDraftChange(index, "temp_id", e.target.value)
                    }
                  />
                  <Input
                    placeholder="project_id"
                    className="h-7 text-xs"
                    value={draft.project_id}
                    onChange={(e) =>
                      onProposalDraftChange(index, "project_id", e.target.value)
                    }
                  />
                </div>
                <Input
                  placeholder={t("threads.workItemTitle", "Title...")}
                  className="h-7 text-xs"
                  value={draft.title}
                  onChange={(e) =>
                    onProposalDraftChange(index, "title", e.target.value)
                  }
                />
                <Textarea
                  placeholder={t("threads.workItemBody", "Body...")}
                  className="min-h-[56px] resize-y text-xs"
                  value={draft.body}
                  onChange={(e) =>
                    onProposalDraftChange(index, "body", e.target.value)
                  }
                />
                <div className="grid grid-cols-2 gap-2">
                  <select
                    className="h-7 rounded-md border border-input bg-background px-2 text-xs"
                    value={draft.priority}
                    onChange={(e) =>
                      onProposalDraftChange(
                        index,
                        "priority",
                        e.target.value as WorkItemPriority,
                      )
                    }
                  >
                    <option value="low">low</option>
                    <option value="medium">medium</option>
                    <option value="high">high</option>
                    <option value="urgent">urgent</option>
                  </select>
                  <Input
                    placeholder="depends_on: api, ui"
                    className="h-7 text-xs"
                    value={draft.depends_on}
                    onChange={(e) =>
                      onProposalDraftChange(index, "depends_on", e.target.value)
                    }
                  />
                </div>
                <div className="flex items-center gap-2">
                  <Input
                    placeholder="labels: frontend, planning"
                    className="h-7 text-xs"
                    value={draft.labels}
                    onChange={(e) =>
                      onProposalDraftChange(index, "labels", e.target.value)
                    }
                  />
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-7 px-2 text-[10px] text-rose-600 hover:text-rose-700"
                    onClick={() => onRemoveProposalDraft(index)}
                  >
                    <X className="h-3 w-3" />
                  </Button>
                </div>
              </div>
            ))}
          </div>

          <div className="flex justify-end">
            <Button
              size="sm"
              className="h-7 px-3 text-[10px]"
              onClick={onSaveProposal}
              disabled={savingProposal || !proposalEditor.title.trim()}
            >
              {savingProposal && <Loader2 className="mr-1 h-3 w-3 animate-spin" />}
              {proposalEditor.proposalId == null
                ? t("threads.createProposal", "Create Proposal")
                : t("threads.saveProposal", "Save Proposal")}
            </Button>
          </div>
        </div>
      )}

      {proposals.length === 0 ? (
        <p className="text-[11px] text-slate-400">
          {t("threads.noProposals", "No proposals yet")}
        </p>
      ) : (
        <div className="space-y-2">
          {proposals.map((proposal) => {
            const review = proposalReviewInputs[proposal.id] ?? {
              reviewedBy: proposal.proposed_by,
              reviewNote: proposal.review_note ?? "",
            };
            const canEdit =
              proposal.status === "draft" || proposal.status === "revised";
            return (
              <div
                key={proposal.id}
                className="space-y-2 rounded-lg border border-border/50 p-2.5"
              >
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <div className="flex items-center gap-1.5">
                      <span className="truncate text-[11px] font-medium text-slate-800">
                        {proposal.title}
                      </span>
                      <Badge
                        variant="outline"
                        className={cn(
                          "text-[8px] normal-case",
                          proposalStatusTone(proposal.status),
                        )}
                      >
                        {proposal.status}
                      </Badge>
                    </div>
                    <p className="mt-0.5 text-[10px] text-slate-500">
                      {proposal.summary || proposal.content || "—"}
                    </p>
                    <div className="mt-1 flex flex-wrap gap-1">
                      <Badge variant="secondary" className="text-[8px]">
                        {proposal.work_item_drafts?.length ?? 0} drafts
                      </Badge>
                      {proposal.source_message_id != null && (
                        <Badge variant="secondary" className="text-[8px]">
                          msg #{proposal.source_message_id}
                        </Badge>
                      )}
                      {proposal.initiative_id != null && (
                        <Badge variant="secondary" className="text-[8px]">
                          initiative #{proposal.initiative_id}
                        </Badge>
                      )}
                    </div>
                  </div>
                  <span className="shrink-0 text-[10px] text-slate-400">
                    {formatRelativeTime(proposal.updated_at)}
                  </span>
                </div>

                {proposal.work_item_drafts && proposal.work_item_drafts.length > 0 && (
                  <div className="space-y-1 rounded-md bg-slate-50/80 p-2">
                    {proposal.work_item_drafts.map((draft) => (
                      <div
                        key={`${proposal.id}-${draft.temp_id}`}
                        className="text-[10px] text-slate-600"
                      >
                        <span className="font-medium">{draft.title || draft.temp_id}</span>
                        <span className="ml-1 text-slate-400">
                          [{draft.priority}]
                        </span>
                      </div>
                    ))}
                  </div>
                )}

                <div className="grid grid-cols-2 gap-2">
                  <Input
                    placeholder={t("threads.reviewedBy", "Reviewer")}
                    className="h-7 text-xs"
                    value={review.reviewedBy}
                    onChange={(e) =>
                      onProposalReviewInputChange(
                        proposal.id,
                        "reviewedBy",
                        e.target.value,
                      )
                    }
                  />
                  <Input
                    placeholder={t("threads.reviewNote", "Review note")}
                    className="h-7 text-xs"
                    value={review.reviewNote}
                    onChange={(e) =>
                      onProposalReviewInputChange(
                        proposal.id,
                        "reviewNote",
                        e.target.value,
                      )
                    }
                  />
                </div>

                <div className="flex flex-wrap gap-1">
                  {canEdit && (
                    <>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-6 px-2 text-[10px]"
                        onClick={() => onOpenEditProposal(proposal)}
                      >
                        {t("threads.editProposal", "Edit Proposal")}
                      </Button>
                      <Button
                        size="sm"
                        className="h-6 px-2 text-[10px]"
                        onClick={() => onSubmitProposal(proposal.id)}
                        disabled={proposalActionLoadingID === proposal.id}
                      >
                        {proposalActionLoadingID === proposal.id && (
                          <Loader2 className="mr-1 h-3 w-3 animate-spin" />
                        )}
                        {t("threads.submitProposal", "Submit Proposal")}
                      </Button>
                    </>
                  )}
                  {proposal.status === "open" && (
                    <>
                      <Button
                        variant="outline"
                        size="sm"
                        className="h-6 px-2 text-[10px]"
                        onClick={() => onApproveProposal(proposal.id)}
                        disabled={proposalActionLoadingID === proposal.id}
                      >
                        {t("threads.approveProposal", "Approve Proposal")}
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        className="h-6 px-2 text-[10px]"
                        onClick={() => onRejectProposal(proposal.id)}
                        disabled={proposalActionLoadingID === proposal.id}
                      >
                        {t("threads.rejectProposal", "Reject Proposal")}
                      </Button>
                    </>
                  )}
                  {proposal.status === "rejected" && (
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-6 px-2 text-[10px]"
                      onClick={() => onReviseProposal(proposal.id)}
                      disabled={proposalActionLoadingID === proposal.id}
                    >
                      {t("threads.reviseProposal", "Revise Proposal")}
                    </Button>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

function readIssueSourceType(issue: Issue | undefined): string | null {
  const value = issue?.metadata?.source_type;
  return typeof value === "string" && value.trim().length > 0 ? value.trim() : null;
}

function TasksSection({
  threadTaskGroupsEnabled,
  onToggleThreadTaskGroups,
  taskGroups,
  taskGroupsLoading,
  taskGroupStatusTone,
  onDeleteTaskGroup,
  onRetryTaskGroup,
  workItemLinks,
  orderedWorkItemLinks,
  linkedIssues,
  showCreateWI,
  newWITitle,
  newWIBody,
  showLinkWI,
  linkWIId,
  onOpenCreateWorkItem,
  onShowCreateWIChange,
  onNewWITitleChange,
  onNewWIBodyChange,
  onCreateWorkItem,
  onShowLinkWIChange,
  onLinkWIIdChange,
  onLinkWorkItem,
  onResetCreateWorkItemDraft,
}: ThreadSidebarProps) {
  const { t } = useTranslation();

  return (
    <div className="space-y-3">
      {/* Task Groups sub-section */}
      <div>
        <div className="flex items-center justify-between">
          <span className="text-[10px] font-semibold uppercase tracking-wider text-slate-400">
            Task Groups
            {threadTaskGroupsEnabled && taskGroupsLoading && (
              <Loader2 className="ml-1 inline h-3 w-3 animate-spin" />
            )}
          </span>
          <Button
            type="button"
            variant={threadTaskGroupsEnabled ? "default" : "outline"}
            size="sm"
            className="h-6 px-2 text-[10px]"
            onClick={onToggleThreadTaskGroups}
          >
            {threadTaskGroupsEnabled
              ? t("threads.taskGroupsDisable", { defaultValue: "关闭" })
              : t("threads.taskGroupsEnable", { defaultValue: "开启" })}
          </Button>
        </div>
        {!threadTaskGroupsEnabled ? (
          <p className="mt-1 text-[11px] text-slate-400">
            {t("threads.taskGroupsDisabledHint", {
              defaultValue: "前端已关闭 Task Group 流程；开启后才会加载和展示。",
            })}
          </p>
        ) : taskGroups.length === 0 ? (
          <p className="mt-1 text-[11px] text-slate-400">
            {t("threads.noTaskGroups", "No task groups yet")}
          </p>
        ) : (
          <div className="mt-1.5 space-y-1.5">
            {taskGroups.map((group) => (
              <div
                key={group.id}
                className="rounded-lg border border-border/50 p-2"
              >
                <div className="flex items-center justify-between gap-1.5">
                  <span className="truncate text-[11px] font-medium">
                    Group #{group.id}
                  </span>
                  <div className="flex items-center gap-1">
                    <Badge
                      variant="outline"
                      className={cn(
                        "shrink-0 text-[8px] normal-case",
                        taskGroupStatusTone(group.status),
                      )}
                    >
                      {group.status}
                    </Badge>
                    {group.status === "failed" && (
                      <button
                        type="button"
                        className="text-[10px] text-blue-500 hover:text-blue-700"
                        onClick={() => onRetryTaskGroup(group.id)}
                        title="Retry"
                      >
                        ↻
                      </button>
                    )}
                    {(group.status === "pending" || group.status === "failed") && (
                      <button
                        type="button"
                        className="text-[10px] text-rose-500 hover:text-rose-700"
                        onClick={() => onDeleteTaskGroup(group.id)}
                        title="Delete"
                      >
                        ✕
                      </button>
                    )}
                  </div>
                </div>
                <p className="mt-0.5 text-[10px] text-slate-400">
                  {group.completed_at ? `Completed ${group.completed_at}` : `Created ${group.created_at}`}
                </p>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Work Items sub-section */}
      <div>
        <div className="flex items-center justify-between">
          <span className="text-[10px] font-semibold uppercase tracking-wider text-slate-400">
            Work Items
          </span>
          <div className="flex gap-1">
            <Button
              variant="ghost"
              size="sm"
              className="h-6 px-2 text-[10px]"
              onClick={onOpenCreateWorkItem}
            >
              <Plus className="mr-0.5 h-3 w-3" />
              {t("threads.createWorkItem", "Create")}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className="h-6 px-2 text-[10px]"
              onClick={() => onShowLinkWIChange(!showLinkWI)}
            >
              <Link2 className="mr-0.5 h-3 w-3" />
              {t("threads.linkExisting", "Link")}
            </Button>
          </div>
        </div>

        {showCreateWI && (
          <div className="mt-1.5 space-y-2 rounded-lg border bg-slate-50/80 p-2.5">
            <Input
              placeholder={t("threads.workItemTitle", "Title...")}
              className="h-7 text-xs"
              value={newWITitle}
              onChange={(e) => onNewWITitleChange(e.target.value)}
              onKeyDown={(e) =>
                e.key === "Enter" && !e.shiftKey && onCreateWorkItem()
              }
            />
            <Textarea
              placeholder={t("threads.workItemBody", "Body...")}
              value={newWIBody}
              onChange={(e) => onNewWIBodyChange(e.target.value)}
              className="min-h-[60px] resize-y text-xs"
            />
            <div className="flex justify-end gap-1.5">
              <Button
                variant="ghost"
                size="sm"
                className="h-6 px-2 text-[10px]"
                onClick={() => {
                  onShowCreateWIChange(false);
                  onResetCreateWorkItemDraft();
                }}
              >
                {t("common.cancel", "Cancel")}
              </Button>
              <Button
                size="sm"
                className="h-6 px-2 text-[10px]"
                onClick={onCreateWorkItem}
                disabled={!newWITitle.trim() || !newWIBody.trim()}
              >
                {t("common.create", "Create")}
              </Button>
            </div>
          </div>
        )}

        {showLinkWI && (
          <div className="mt-1.5 flex gap-1.5">
            <Input
              placeholder={t("threads.workItemId", "Work item ID...")}
              className="h-7 text-xs"
              value={linkWIId}
              onChange={(e) => onLinkWIIdChange(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && onLinkWorkItem()}
            />
            <Button
              size="sm"
              className="h-7 text-[10px]"
              onClick={onLinkWorkItem}
              disabled={!linkWIId.trim()}
            >
              {t("threads.linkBtn", "Link")}
            </Button>
          </div>
        )}

        {workItemLinks.length === 0 ? (
          <p className="mt-1 text-[11px] text-slate-400">
            {t("threads.noLinkedWorkItems", "No linked work items")}
          </p>
        ) : (
          <div className="mt-1.5 space-y-1">
            {orderedWorkItemLinks.map((link) => {
              const issue = linkedIssues[link.work_item_id];
              const sourceType = readIssueSourceType(issue);
              return (
                <div
                  key={link.id}
                  className={cn(
                    "rounded-md border px-2 py-1.5 text-[11px]",
                    link.is_primary
                      ? "border-blue-200 bg-blue-50/50"
                      : "border-border/40",
                  )}
                >
                  <div className="flex items-center gap-1">
                    {link.is_primary && (
                      <Badge variant="default" className="text-[8px]">
                        primary
                      </Badge>
                    )}
                    <Badge variant="outline" className="text-[8px]">
                      {link.relation_type}
                    </Badge>
                    {sourceType && (
                      <Badge variant="secondary" className="text-[8px]">
                        {sourceType === "thread_summary"
                          ? "summary"
                          : sourceType === "thread_manual"
                            ? "manual"
                            : sourceType}
                      </Badge>
                    )}
                  </div>
                  <Link
                    to={`/work-items/${link.work_item_id}`}
                    className="mt-0.5 block truncate font-medium text-primary hover:underline"
                  >
                    {issue ? issue.title : `#${link.work_item_id}`}
                  </Link>
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}

/* ── Files section ── */

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function FilesSection({
  threadId,
  attachments,
  loading,
  onUpload,
  onDelete,
  getDownloadUrl,
}: {
  threadId: number;
  attachments: ThreadAttachment[];
  loading: boolean;
  onUpload: (file: File) => void;
  onDelete: (id: number) => void;
  getDownloadUrl: (threadId: number, attachmentId: number) => string;
}) {
  const { t } = useTranslation();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [dragOver, setDragOver] = useState(false);

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    const files = Array.from(e.dataTransfer.files);
    files.forEach((f) => onUpload(f));
  };

  return (
    <div
      className={cn(
        "space-y-2 rounded-lg border border-dashed p-2 transition-colors",
        dragOver ? "border-blue-400 bg-blue-50/50" : "border-transparent",
      )}
      onDragOver={(e) => { e.preventDefault(); setDragOver(true); }}
      onDragLeave={() => setDragOver(false)}
      onDrop={handleDrop}
    >
      <input
        ref={fileInputRef}
        type="file"
        className="hidden"
        multiple
        onChange={(e) => {
          const files = Array.from(e.target.files ?? []);
          files.forEach((f) => onUpload(f));
          e.target.value = "";
        }}
      />

      {loading && (
        <div className="flex items-center justify-center py-2">
          <Loader2 className="h-4 w-4 animate-spin text-slate-400" />
        </div>
      )}

      {attachments.length === 0 && !loading && (
        <button
          type="button"
          className="flex w-full flex-col items-center gap-1.5 rounded-lg py-4 text-[11px] text-slate-400 transition-colors hover:bg-slate-50 hover:text-slate-500"
          onClick={() => fileInputRef.current?.click()}
        >
          <Paperclip className="h-5 w-5" />
          <span>{t("threads.dropOrClick", "Drop files or click to upload")}</span>
        </button>
      )}

      {attachments.length > 0 && (
        <>
          <div className="space-y-1">
            {attachments.map((att) => (
              <div
                key={att.id}
                className="group flex items-center gap-2 rounded-md p-1.5 text-[11px] transition-colors hover:bg-slate-50"
              >
                <File className="h-3.5 w-3.5 shrink-0 text-slate-400" />
                <a
                  href={getDownloadUrl(threadId, att.id)}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="min-w-0 flex-1 truncate font-medium text-slate-700 hover:text-blue-600 hover:underline"
                  title={att.file_name}
                >
                  {att.file_name}
                </a>
                <span className="shrink-0 text-[10px] text-slate-400">
                  {formatFileSize(att.file_size)}
                </span>
                <button
                  type="button"
                  className="flex h-4 w-4 shrink-0 items-center justify-center rounded opacity-0 transition-opacity hover:bg-rose-50 hover:text-rose-500 group-hover:opacity-100"
                  onClick={() => onDelete(att.id)}
                >
                  <X className="h-3 w-3" />
                </button>
              </div>
            ))}
          </div>
          <button
            type="button"
            className="flex w-full items-center justify-center gap-1 rounded-md border border-dashed border-slate-300 py-1.5 text-[10px] text-slate-400 transition-colors hover:border-slate-400 hover:text-slate-500"
            onClick={() => fileInputRef.current?.click()}
          >
            <Plus className="h-3 w-3" />
            {t("threads.addFile", "Add file")}
          </button>
        </>
      )}
    </div>
  );
}

/* ── Info section ── */

function InfoSection({
  thread,
  messagesCount,
}: {
  thread: Thread;
  messagesCount: number;
}) {
  const { t } = useTranslation();
  const rows = [
    { label: "ID", value: <span className="font-mono">{thread.id}</span> },
    { label: t("threads.status", "Status"), value: thread.status },
    {
      label: t("threads.owner", "Owner"),
      value: thread.owner_id || "—",
    },
    {
      label: t("threads.updated", "Updated"),
      value: formatRelativeTime(thread.updated_at),
    },
    { label: t("threads.messages", "Messages"), value: messagesCount },
  ];

  return (
    <div className="space-y-1 text-[11px]">
      {rows.map((row) => (
        <div key={row.label} className="flex justify-between">
          <span className="text-slate-400">{row.label}</span>
          <span className="text-slate-600">{row.value}</span>
        </div>
      ))}
    </div>
  );
}
