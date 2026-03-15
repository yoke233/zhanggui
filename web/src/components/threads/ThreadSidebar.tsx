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
  ThreadWorkItemLink,
  ThreadTaskGroup,
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
  profileByID: Map<string, AgentProfile>;
  highlightedAgentProfileID: string | null;
  agentCardRefs: MutableRefObject<Record<string, HTMLDivElement | null>>;
  removingAgentID: number | null;
  onRemoveAgent: (agentSessionID: number) => void;
  agentStatusColor: (status: string) => string;

  // Members: participants
  participants: ThreadParticipant[];

  // Tasks: task groups
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
    () => new Set(["members"]),
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
  const taskCount = props.taskGroups.length + props.workItemLinks.length;

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
  profileByID,
  highlightedAgentProfileID,
  agentCardRefs,
  removingAgentID,
  onRemoveAgent,
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
      {/* Agent list */}
      {agentSessionsWithProfileID.map((session) => {
        const profile = profileByID.get(session.agent_profile_id);
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
                : "border-border/50",
            )}
          >
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

function readIssueSourceType(issue: Issue | undefined): string | null {
  const value = issue?.metadata?.source_type;
  return typeof value === "string" && value.trim().length > 0 ? value.trim() : null;
}

function TasksSection({
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
            {taskGroupsLoading && (
              <Loader2 className="ml-1 inline h-3 w-3 animate-spin" />
            )}
          </span>
        </div>
        {taskGroups.length === 0 ? (
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
