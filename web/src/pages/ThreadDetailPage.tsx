import { useEffect, useRef, useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
  ArrowLeft,
  Bot,
  ChevronDown,
  ChevronUp,
  Link2,
  Loader2,
  MessageSquare,
  Plus,
  Save,
  Send,
  Settings2,
  User,
  Users,
  X,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { formatRelativeTime, getErrorMessage } from "@/lib/v2Workbench";
import { Link } from "react-router-dom";
import type { AgentProfile, Thread, ThreadMessage, ThreadParticipant, ThreadWorkItemLink, ThreadAgentSession, Issue } from "@/types/apiV2";
import type { ThreadAckPayload, ThreadEventPayload } from "@/types/ws";

/* ── helper functions (unchanged) ── */

function hasSavedSummary(thread: Thread | null): boolean {
  return Boolean(thread?.summary?.trim());
}

function deriveWorkItemTitle(thread: Thread): string {
  const firstMeaningfulLine = (thread.summary ?? "")
    .split(/\r?\n/)
    .map((line) => line.replace(/^[-*#\d.\)\s]+/, "").trim())
    .find((line) => line.length > 0);
  const title = firstMeaningfulLine || thread.title.trim();
  return title.length > 80 ? `${title.slice(0, 77)}...` : title;
}

function readSourceType(issue: Issue | undefined): string | null {
  const value = issue?.metadata?.source_type;
  return typeof value === "string" && value.trim().length > 0 ? value.trim() : null;
}

function readTargetAgentID(metadata: Record<string, unknown> | undefined): string | null {
  const value = metadata?.target_agent_id;
  return typeof value === "string" && value.trim().length > 0 ? value.trim() : null;
}

function parseMentionTarget(message: string, activeAgentProfileIDs: string[]): { targetAgentID: string | null; error: string | null } {
  const trimmed = message.trim();
  const match = trimmed.match(/^@([A-Za-z0-9._:-]+)\s+(.+)$/s);
  if (!match) {
    return { targetAgentID: null, error: null };
  }

  const targetAgentID = match[1].trim();
  if (!activeAgentProfileIDs.includes(targetAgentID)) {
    return { targetAgentID: null, error: `未找到活跃 agent：${targetAgentID}` };
  }

  return { targetAgentID, error: null };
}

function readAgentRoutingMode(thread: Thread | null): "mention_only" | "broadcast" {
  const value = thread?.metadata?.agent_routing_mode;
  return value === "broadcast" ? "broadcast" : "mention_only";
}

function detectMentionDraft(message: string, caretPosition: number | null): { start: number; end: number; query: string } | null {
  if (caretPosition == null || caretPosition < 0) {
    return null;
  }

  const left = message.slice(0, caretPosition);
  const leftMatch = left.match(/(^|\s)@([A-Za-z0-9._:-]*)$/);
  if (!leftMatch) {
    return null;
  }

  const prefixLength = leftMatch[1]?.length ?? 0;
  const fullMatchLength = leftMatch[0]?.length ?? 0;
  const start = left.length - fullMatchLength + prefixLength;
  const right = message.slice(caretPosition);
  const rightMatch = right.match(/^[A-Za-z0-9._:-]*/);
  const end = caretPosition + (rightMatch?.[0]?.length ?? 0);

  return {
    start,
    end,
    query: message.slice(start + 1, end),
  };
}

function replaceMentionDraft(message: string, draft: { start: number; end: number }, profileID: string): { nextMessage: string; caretPosition: number } {
  const replacement = `@${profileID} `;
  const nextMessage = `${message.slice(0, draft.start)}${replacement}${message.slice(draft.end)}`;
  return {
    nextMessage,
    caretPosition: draft.start + replacement.length,
  };
}

function splitMessageMentions(content: string): Array<{ type: "text" | "mention"; value: string; profileID?: string }> {
  const parts: Array<{ type: "text" | "mention"; value: string; profileID?: string }> = [];
  const mentionPattern = /@([A-Za-z0-9._:-]+)/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null = mentionPattern.exec(content);
  while (match) {
    if (match.index > lastIndex) {
      parts.push({ type: "text", value: content.slice(lastIndex, match.index) });
    }
    parts.push({ type: "mention", value: match[0], profileID: match[1] });
    lastIndex = match.index + match[0].length;
    match = mentionPattern.exec(content);
  }
  if (lastIndex < content.length) {
    parts.push({ type: "text", value: content.slice(lastIndex) });
  }
  return parts.length > 0 ? parts : [{ type: "text", value: content }];
}

function readCommittedMentionTarget(message: string, activeAgentProfileIDs: string[]): string | null {
  const trimmed = message.trimStart();
  const match = trimmed.match(/^@([A-Za-z0-9._:-]+)(?:\s|$)/);
  if (!match) {
    return null;
  }
  const profileID = match[1].trim();
  return activeAgentProfileIDs.includes(profileID) ? profileID : null;
}

function agentStatusColor(status: string): string {
  switch (status) {
    case "active": return "bg-emerald-500";
    case "booting": return "bg-amber-500";
    case "paused": return "bg-slate-400";
    case "joining": return "bg-blue-400";
    default: return "bg-rose-500";
  }
}

type SidebarTab = "agents" | "details";

export function ThreadDetailPage() {
  const { t } = useTranslation();
  const { threadId } = useParams<{ threadId: string }>();
  const navigate = useNavigate();
  const { apiClient, wsClient } = useWorkbench();

  const [thread, setThread] = useState<Thread | null>(null);
  const [messages, setMessages] = useState<ThreadMessage[]>([]);
  const [participants, setParticipants] = useState<ThreadParticipant[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [workItemLinks, setWorkItemLinks] = useState<ThreadWorkItemLink[]>([]);
  const [linkedIssues, setLinkedIssues] = useState<Record<number, Issue>>({});
  const [newMessage, setNewMessage] = useState("");
  const [sending, setSending] = useState(false);
  const [summaryDraft, setSummaryDraft] = useState("");
  const [savingSummary, setSavingSummary] = useState(false);
  const [showCreateWI, setShowCreateWI] = useState(false);
  const [newWITitle, setNewWITitle] = useState("");
  const [newWIBody, setNewWIBody] = useState("");
  const [showLinkWI, setShowLinkWI] = useState(false);
  const [linkWIId, setLinkWIId] = useState("");
  const [agentSessions, setAgentSessions] = useState<ThreadAgentSession[]>([]);
  const [availableProfiles, setAvailableProfiles] = useState<AgentProfile[]>([]);
  const [inviteProfileID, setInviteProfileID] = useState("");
  const [invitingAgent, setInvitingAgent] = useState(false);
  const [removingAgentID, setRemovingAgentID] = useState<number | null>(null);
  const [savingRoutingMode, setSavingRoutingMode] = useState(false);
  const [mentionDraft, setMentionDraft] = useState<{ start: number; end: number; query: string } | null>(null);
  const [selectedMentionIndex, setSelectedMentionIndex] = useState(0);
  const [highlightedAgentProfileID, setHighlightedAgentProfileID] = useState<string | null>(null);
  const [hoveredMentionProfileID, setHoveredMentionProfileID] = useState<string | null>(null);
  const [sidebarTab, setSidebarTab] = useState<SidebarTab>("agents");
  const [summaryCollapsed, setSummaryCollapsed] = useState(true);
  const pendingThreadRequestIdRef = useRef<string | null>(null);
  const syntheticMessageIDRef = useRef(-1);
  const messageInputRef = useRef<HTMLInputElement | null>(null);
  const agentCardRefs = useRef<Record<string, HTMLDivElement | null>>({});
  const messagesEndRef = useRef<HTMLDivElement | null>(null);

  const id = Number(threadId);
  const joinedAgentProfileIDs = new Set(agentSessions.map((session) => session.agent_profile_id));
  const inviteableProfiles = availableProfiles.filter((profile) => !joinedAgentProfileIDs.has(profile.id));
  const activeAgentProfileIDs = agentSessions
    .filter((session) => session.status === "active" || session.status === "booting")
    .map((session) => session.agent_profile_id);
  const agentRoutingMode = readAgentRoutingMode(thread);
  const profileByID = new Map(availableProfiles.map((profile) => [profile.id, profile]));
  const agentSessionByProfileID = new Map(agentSessions.map((session) => [session.agent_profile_id, session]));
  const committedMentionTargetID = readCommittedMentionTarget(newMessage, activeAgentProfileIDs);
  const committedMentionProfile = committedMentionTargetID ? profileByID.get(committedMentionTargetID) : undefined;
  const committedMentionSession = committedMentionTargetID ? agentSessionByProfileID.get(committedMentionTargetID) : undefined;
  const mentionCandidates = activeAgentProfileIDs
    .map((profileID) => {
      const profile = profileByID.get(profileID);
      const session = agentSessionByProfileID.get(profileID);
      return {
        id: profileID,
        label: profile?.name ? `${profile.name} (${profileID})` : profileID,
        status: session?.status ?? "active",
      };
    })
    .filter((candidate) => {
      if (!mentionDraft) {
        return false;
      }
      const query = mentionDraft.query.trim().toLowerCase();
      return query === ""
        || candidate.id.toLowerCase().includes(query)
        || candidate.label.toLowerCase().includes(query);
    })
    .slice(0, 6);
  const orderedWorkItemLinks = [...workItemLinks].sort((a, b) => {
    if (a.is_primary === b.is_primary) {
      return a.id - b.id;
    }
    return a.is_primary ? -1 : 1;
  });

  /* ── auto-scroll to bottom on new messages ── */
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages.length]);

  useEffect(() => {
    if (!id || isNaN(id)) return;
    let cancelled = false;

    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        const [th, msgs, parts, links, agents, profiles] = await Promise.all([
          apiClient.getThread(id),
          apiClient.listThreadMessages(id, { limit: 100 }),
          apiClient.listThreadParticipants(id),
          apiClient.listWorkItemsByThread(id),
          apiClient.listThreadAgents(id),
          apiClient.listProfiles(),
        ]);
        if (!cancelled) {
          setThread(th);
          setSummaryDraft(th.summary ?? "");
          setMessages(msgs);
          setParticipants(parts);
          setWorkItemLinks(links);
          setAgentSessions(agents);
          setAvailableProfiles(profiles);
          const issueMap: Record<number, Issue> = {};
          const issueResults = await Promise.allSettled(
            links.map((l) => apiClient.getWorkItem(l.work_item_id)),
          );
          issueResults.forEach((r, i) => {
            if (r.status === "fulfilled") issueMap[links[i].work_item_id] = r.value;
          });
          if (!cancelled) setLinkedIssues(issueMap);
        }
      } catch (e) {
        if (!cancelled) setError(getErrorMessage(e));
      } finally {
        if (!cancelled) setLoading(false);
      }
    };
    void load();
    return () => { cancelled = true; };
  }, [apiClient, id]);

  useEffect(() => {
    if (inviteableProfiles.some((profile) => profile.id === inviteProfileID)) {
      return;
    }
    setInviteProfileID(inviteableProfiles[0]?.id ?? "");
  }, [inviteProfileID, inviteableProfiles]);

  useEffect(() => {
    if (mentionCandidates.length === 0) {
      setSelectedMentionIndex(0);
      return;
    }
    if (selectedMentionIndex >= mentionCandidates.length) {
      setSelectedMentionIndex(0);
    }
  }, [mentionCandidates.length, selectedMentionIndex]);

  useEffect(() => {
    if (!id || isNaN(id)) {
      return;
    }

    const appendRealtimeMessage = (payload: ThreadEventPayload, roleFallback: "human" | "agent") => {
      const content = typeof payload.content === "string" && payload.content.trim().length > 0
        ? payload.content
        : typeof payload.message === "string"
          ? payload.message
          : "";
      if (!content.trim()) {
        return;
      }

      const senderID = typeof payload.sender_id === "string" && payload.sender_id.trim().length > 0
        ? payload.sender_id.trim()
        : typeof payload.profile_id === "string" && payload.profile_id.trim().length > 0
          ? payload.profile_id.trim()
          : roleFallback;
      const role = typeof payload.role === "string" && payload.role.trim().length > 0
        ? payload.role.trim()
        : roleFallback;

      setMessages((prev) => [
        ...prev,
        {
          id: syntheticMessageIDRef.current--,
          thread_id: id,
          sender_id: senderID,
          role,
          content,
          metadata: payload.target_agent_id
            ? { target_agent_id: payload.target_agent_id }
            : undefined,
          created_at: new Date().toISOString(),
        },
      ]);
    };

    const refreshAgentSessions = async () => {
      try {
        const sessions = await apiClient.listThreadAgents(id);
        setAgentSessions(sessions);
      } catch {
        // Ignore background refresh failures
      }
    };

    const sendThreadSubscription = (type: "subscribe_thread" | "unsubscribe_thread") => {
      try {
        wsClient.send({
          type,
          data: { thread_id: id },
        });
      } catch {
        // Ignore send errors here
      }
    };

    const unsubscribeThreadMessage = wsClient.subscribe<ThreadEventPayload>("thread.message", (payload) => {
      if (payload.thread_id !== id) return;
      appendRealtimeMessage(payload, "human");
    });
    const unsubscribeThreadOutput = wsClient.subscribe<ThreadEventPayload>("thread.agent_output", (payload) => {
      if (payload.thread_id !== id) return;
      appendRealtimeMessage(payload, "agent");
    });
    const unsubscribeThreadAck = wsClient.subscribe<ThreadAckPayload>("thread.ack", (payload) => {
      if (payload.thread_id !== id) return;
      if (pendingThreadRequestIdRef.current && payload.request_id && payload.request_id !== pendingThreadRequestIdRef.current) return;
      pendingThreadRequestIdRef.current = null;
      setSending(false);
      clearMentionComposerState();
    });
    const unsubscribeThreadError = wsClient.subscribe<{ request_id?: string; error?: string }>("thread.error", (payload) => {
      if (pendingThreadRequestIdRef.current && payload.request_id && payload.request_id !== pendingThreadRequestIdRef.current) return;
      pendingThreadRequestIdRef.current = null;
      setSending(false);
      clearMentionComposerState();
      setError(payload.error?.trim() || t("threads.sendFailed", "Thread message failed to send"));
    });
    const unsubscribeThreadAgentEvent = wsClient.subscribe<ThreadEventPayload>("thread.agent_joined", (payload) => {
      if (payload.thread_id === id) void refreshAgentSessions();
    });
    const unsubscribeThreadAgentLeft = wsClient.subscribe<ThreadEventPayload>("thread.agent_left", (payload) => {
      if (payload.thread_id === id) void refreshAgentSessions();
    });
    const unsubscribeThreadAgentBooted = wsClient.subscribe<ThreadEventPayload>("thread.agent_booted", (payload) => {
      if (payload.thread_id === id) void refreshAgentSessions();
    });
    const unsubscribeThreadAgentFailed = wsClient.subscribe<ThreadEventPayload>("thread.agent_failed", (payload) => {
      if (payload.thread_id !== id) return;
      setError(payload.error?.trim() || t("threads.agentFailed", "An agent in this thread failed."));
      void refreshAgentSessions();
    });
    const unsubscribeStatus = wsClient.onStatusChange((status) => {
      if (status === "open") sendThreadSubscription("subscribe_thread");
    });

    if (wsClient.getStatus() === "open") {
      sendThreadSubscription("subscribe_thread");
    }

    return () => {
      unsubscribeThreadMessage();
      unsubscribeThreadOutput();
      unsubscribeThreadAck();
      unsubscribeThreadError();
      unsubscribeThreadAgentEvent();
      unsubscribeThreadAgentLeft();
      unsubscribeThreadAgentBooted();
      unsubscribeThreadAgentFailed();
      unsubscribeStatus();
      pendingThreadRequestIdRef.current = null;
      if (wsClient.getStatus() === "open") {
        sendThreadSubscription("unsubscribe_thread");
      }
    };
  }, [apiClient, id, t, wsClient]);

  /* ── handlers (unchanged) ── */

  const updateMentionDraft = (value: string, caretPosition: number | null) => {
    const nextDraft = detectMentionDraft(value, caretPosition);
    setMentionDraft(nextDraft);
    setSelectedMentionIndex(0);
  };

  const handleMessageInputChange = (value: string, caretPosition: number | null) => {
    setNewMessage(value);
    updateMentionDraft(value, caretPosition);
  };

  const applyMentionCandidate = (profileID: string) => {
    if (!mentionDraft) return;
    const { nextMessage, caretPosition } = replaceMentionDraft(newMessage, mentionDraft, profileID);
    setNewMessage(nextMessage);
    setMentionDraft(null);
    setSelectedMentionIndex(0);
    requestAnimationFrame(() => {
      messageInputRef.current?.focus();
      messageInputRef.current?.setSelectionRange(caretPosition, caretPosition);
    });
  };

  const focusAgentProfile = (profileID: string) => {
    setHighlightedAgentProfileID(profileID);
    setSidebarTab("agents");
    const node = agentCardRefs.current[profileID];
    if (node) {
      node.scrollIntoView({ behavior: "smooth", block: "nearest" });
    }
  };

  const clearMentionComposerState = () => {
    setNewMessage("");
    setMentionDraft(null);
    setSelectedMentionIndex(0);
  };

  const handleSend = async () => {
    if (!newMessage.trim() || !id) return;
    const mention = parseMentionTarget(newMessage, activeAgentProfileIDs);
    if (mention.error) {
      setError(mention.error);
      return;
    }
    setSending(true);
    setError(null);
    try {
      const requestId = `thread-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
      pendingThreadRequestIdRef.current = requestId;
      wsClient.send({
        type: "thread.send",
        data: {
          request_id: requestId,
          thread_id: id,
          message: newMessage.trim(),
          sender_id: thread?.owner_id || "human",
          target_agent_id: mention.targetAgentID ?? undefined,
        },
      });
    } catch (e) {
      pendingThreadRequestIdRef.current = null;
      setSending(false);
      setError(getErrorMessage(e));
    } finally {
      if (!pendingThreadRequestIdRef.current) {
        setSending(false);
      }
    }
  };

  const handleSaveSummary = async () => {
    if (!thread || !id) return;
    setSavingSummary(true);
    setError(null);
    try {
      const updated = await apiClient.updateThread(id, { summary: summaryDraft.trim() });
      setThread(updated);
      setSummaryDraft(updated.summary ?? "");
      if (showCreateWI) {
        const nextSummary = updated.summary?.trim() ?? "";
        setNewWIBody(nextSummary);
        setNewWITitle(nextSummary ? deriveWorkItemTitle(updated) : "");
      }
    } catch (e) {
      setError(getErrorMessage(e));
    } finally {
      setSavingSummary(false);
    }
  };

  const handleOpenCreateWorkItem = () => {
    if (!thread) return;
    if (!hasSavedSummary(thread)) {
      setError("请先生成或填写 summary，再创建 WorkItem。");
      setShowCreateWI(false);
      return;
    }
    setError(null);
    setShowCreateWI((prev) => {
      const next = !prev;
      if (next) {
        setNewWITitle(deriveWorkItemTitle(thread));
        setNewWIBody(thread.summary?.trim() ?? "");
      }
      return next;
    });
  };

  const handleCreateWorkItem = async () => {
    if (!newWITitle.trim() || !id) return;
    setError(null);
    try {
      const trimmedBody = newWIBody.trim();
      const savedSummary = thread?.summary?.trim() ?? "";
      const issue = await apiClient.createWorkItemFromThread(id, {
        title: newWITitle.trim(),
        body: trimmedBody !== "" && trimmedBody !== savedSummary ? trimmedBody : undefined,
      });
      const links = await apiClient.listWorkItemsByThread(id);
      setWorkItemLinks(links);
      setLinkedIssues((prev) => ({ ...prev, [issue.id]: issue }));
      setNewWITitle("");
      setNewWIBody("");
      setShowCreateWI(false);
    } catch (e) {
      setError(getErrorMessage(e));
    }
  };

  const handleLinkWorkItem = async () => {
    const wiId = Number(linkWIId);
    if (!wiId || isNaN(wiId) || !id) return;
    setError(null);
    try {
      await apiClient.createThreadWorkItemLink(id, { work_item_id: wiId, relation_type: "related" });
      const links = await apiClient.listWorkItemsByThread(id);
      setWorkItemLinks(links);
      try {
        const issue = await apiClient.getWorkItem(wiId);
        setLinkedIssues((prev) => ({ ...prev, [wiId]: issue }));
      } catch { /* ignore */ }
      setLinkWIId("");
      setShowLinkWI(false);
    } catch (e) {
      setError(getErrorMessage(e));
    }
  };

  const handleInviteAgent = async () => {
    if (!id || !inviteProfileID) return;
    setInvitingAgent(true);
    setError(null);
    try {
      await apiClient.inviteThreadAgent(id, { agent_profile_id: inviteProfileID });
      const sessions = await apiClient.listThreadAgents(id);
      setAgentSessions(sessions);
    } catch (e) {
      setError(getErrorMessage(e));
    } finally {
      setInvitingAgent(false);
    }
  };

  const handleRemoveAgent = async (agentSessionID: number) => {
    if (!id) return;
    setRemovingAgentID(agentSessionID);
    setError(null);
    try {
      await apiClient.removeThreadAgent(id, agentSessionID);
      const sessions = await apiClient.listThreadAgents(id);
      setAgentSessions(sessions);
    } catch (e) {
      setError(getErrorMessage(e));
    } finally {
      setRemovingAgentID(null);
    }
  };

  const handleSetRoutingMode = async (nextMode: "mention_only" | "broadcast") => {
    if (!thread || !id || nextMode === agentRoutingMode) return;
    setSavingRoutingMode(true);
    setError(null);
    try {
      const updated = await apiClient.updateThread(id, {
        metadata: {
          ...(thread.metadata ?? {}),
          agent_routing_mode: nextMode,
        },
      });
      setThread(updated);
    } catch (e) {
      setError(getErrorMessage(e));
    } finally {
      setSavingRoutingMode(false);
    }
  };

  /* ── render helpers ── */

  const renderMessageContent = (msg: ThreadMessage) => {
    return splitMessageMentions(msg.content).map((part, index) => {
      if (part.type === "text") {
        return <span key={`${msg.id}-text-${index}`}>{part.value}</span>;
      }
      const profileID = part.profileID ?? "";
      const session = agentSessionByProfileID.get(profileID);
      const profile = profileByID.get(profileID);
      return (
        <span key={`${msg.id}-mention-${index}`} className="relative mx-0.5 inline-flex align-baseline">
          <button
            type="button"
            className="inline-flex items-center rounded-md bg-blue-100 px-1.5 py-0.5 text-xs font-semibold text-blue-800 transition-colors hover:bg-blue-200"
            onClick={() => focusAgentProfile(profileID)}
            onMouseEnter={() => setHoveredMentionProfileID(profileID)}
            onMouseLeave={() => setHoveredMentionProfileID((c) => (c === profileID ? null : c))}
          >
            {part.value}
          </button>
          {hoveredMentionProfileID === profileID ? (
            <span
              data-testid={`mention-hover-card-${profileID}`}
              className="pointer-events-none absolute bottom-full left-0 z-30 mb-2 w-56 rounded-lg border border-slate-200 bg-white p-3 text-left shadow-xl"
            >
              <span className="block text-sm font-semibold text-slate-900">
                {profile?.name ?? profileID}
              </span>
              <span className="mt-0.5 block text-xs text-slate-500">@{profileID}</span>
              <span className="mt-2 inline-flex items-center gap-1.5 rounded-full bg-slate-100 px-2 py-0.5 text-[10px] font-medium text-slate-700">
                <span className={cn("h-1.5 w-1.5 rounded-full", agentStatusColor(session?.status ?? "unknown"))} />
                {session?.status ?? "not_joined"}
              </span>
              <span className="mt-2 block text-xs text-slate-500">
                {t("threads.turns", "Turns")}: {session?.turn_count ?? 0} | {((session ? (session.total_input_tokens + session.total_output_tokens) : 0) / 1000).toFixed(1)}k {t("threads.tokens", "tokens")}
              </span>
            </span>
          ) : null}
        </span>
      );
    });
  };

  /* ── loading / not-found states ── */

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="flex flex-col items-center gap-3">
          <Loader2 className="h-8 w-8 animate-spin text-blue-500" />
          <span className="text-sm text-muted-foreground">{t("common.loading", "Loading...")}</span>
        </div>
      </div>
    );
  }

  if (!thread) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-4">
        <div className="rounded-xl border border-destructive/20 bg-destructive/5 px-6 py-4 text-center">
          <p className="text-sm text-destructive">{error || t("threads.notFound", "Thread not found")}</p>
        </div>
        <Button variant="ghost" size="sm" onClick={() => navigate("/threads")}>
          <ArrowLeft className="mr-1.5 h-4 w-4" />
          {t("threads.backToList", "Back to Threads")}
        </Button>
      </div>
    );
  }

  /* ── main layout ── */

  return (
    <div className="flex h-full flex-col overflow-hidden">
      {/* ── Header ── */}
      <div className="flex h-14 shrink-0 items-center justify-between border-b px-5">
        <div className="flex items-center gap-3">
          <button
            type="button"
            className="flex h-8 w-8 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            onClick={() => navigate("/threads")}
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div className="flex items-center gap-2">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-blue-50 text-blue-600">
              <MessageSquare className="h-4 w-4" />
            </div>
            <div className="min-w-0">
              <h1 className="truncate text-sm font-semibold leading-tight">{thread.title}</h1>
              <div className="flex items-center gap-2 text-xs text-muted-foreground">
                <span className="flex items-center gap-1">
                  <span className={cn(
                    "h-1.5 w-1.5 rounded-full",
                    thread.status === "active" ? "bg-emerald-500" : "bg-slate-400",
                  )} />
                  {thread.status}
                </span>
                {thread.owner_id && (
                  <>
                    <span className="text-border">|</span>
                    <span>{thread.owner_id}</span>
                  </>
                )}
                <span className="text-border">|</span>
                <span>{formatRelativeTime(thread.updated_at)}</span>
              </div>
            </div>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <div className="flex items-center gap-1 rounded-lg border bg-muted/30 px-1 py-0.5 text-xs">
            <button
              type="button"
              className={cn(
                "rounded-md px-2.5 py-1 transition-colors",
                agentRoutingMode === "mention_only" ? "bg-background font-medium shadow-sm" : "text-muted-foreground hover:text-foreground",
              )}
              onClick={() => void handleSetRoutingMode("mention_only")}
              disabled={savingRoutingMode}
            >
              {t("threads.routingMentionOnly", "@ Only")}
            </button>
            <button
              type="button"
              className={cn(
                "rounded-md px-2.5 py-1 transition-colors",
                agentRoutingMode === "broadcast" ? "bg-background font-medium shadow-sm" : "text-muted-foreground hover:text-foreground",
              )}
              onClick={() => void handleSetRoutingMode("broadcast")}
              disabled={savingRoutingMode}
            >
              {t("threads.routingBroadcast", "Broadcast")}
            </button>
          </div>
          <Badge variant="secondary" className="gap-1 text-xs">
            <Users className="h-3 w-3" />
            {participants.length}
          </Badge>
          <Badge variant="secondary" className="gap-1 text-xs">
            <Bot className="h-3 w-3" />
            {agentSessions.length}
          </Badge>
        </div>
      </div>

      {/* ── Error banner ── */}
      {error ? (
        <div className="flex items-center justify-between border-b border-destructive/20 bg-destructive/5 px-5 py-2">
          <span className="text-xs text-destructive">{error}</span>
          <button type="button" className="text-destructive/60 hover:text-destructive" onClick={() => setError(null)}>
            <X className="h-3.5 w-3.5" />
          </button>
        </div>
      ) : null}

      {/* ── Main content: chat + sidebar ── */}
      <div className="flex min-h-0 flex-1">
        {/* ── Chat area ── */}
        <div className="flex min-w-0 flex-1 flex-col">
          {/* ── Messages ── */}
          <div className="flex-1 overflow-y-auto px-5 py-4">
            {messages.length === 0 ? (
              <div className="flex h-full flex-col items-center justify-center gap-3 text-muted-foreground">
                <MessageSquare className="h-10 w-10 text-muted-foreground/30" />
                <p className="text-sm">{t("threads.noMessages", "No messages yet. Start the conversation.")}</p>
                {agentSessions.length === 0 && (
                  <p className="text-xs">{t("threads.inviteHint", "Invite an agent from the sidebar to get started.")}</p>
                )}
              </div>
            ) : (
              <div className="mx-auto max-w-3xl space-y-4">
                {messages.map((msg) => {
                  const isAgent = msg.role === "agent";
                  const targetAgent = readTargetAgentID(msg.metadata);
                  const profile = isAgent ? profileByID.get(msg.sender_id) : undefined;
                  return (
                    <div key={msg.id} className={cn("flex gap-3", !isAgent && "flex-row-reverse")}>
                      {/* Avatar */}
                      <div className={cn(
                        "flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-xs font-bold",
                        isAgent
                          ? "bg-emerald-100 text-emerald-700"
                          : "bg-blue-100 text-blue-700",
                      )}>
                        {isAgent ? <Bot className="h-4 w-4" /> : <User className="h-4 w-4" />}
                      </div>
                      {/* Bubble */}
                      <div className={cn("group/msg max-w-[75%] min-w-0")}>
                        {/* Sender line */}
                        <div className={cn(
                          "mb-1 flex items-center gap-1.5 text-[11px] text-muted-foreground",
                          !isAgent && "flex-row-reverse",
                        )}>
                          <span className="font-medium text-foreground/70">
                            {isAgent ? (profile?.name ?? msg.sender_id) : (msg.sender_id || "You")}
                          </span>
                          {targetAgent ? (
                            <span className="rounded bg-blue-50 px-1 py-px text-[10px] text-blue-600">
                              @{targetAgent}
                            </span>
                          ) : null}
                          <span>{formatRelativeTime(msg.created_at)}</span>
                        </div>
                        {/* Content */}
                        <div className={cn(
                          "rounded-2xl px-4 py-2.5 text-sm leading-relaxed",
                          isAgent
                            ? "rounded-tl-md bg-muted/80 text-foreground"
                            : "rounded-tr-md bg-blue-600 text-white",
                        )}>
                          <p className="whitespace-pre-wrap break-words">
                            {renderMessageContent(msg)}
                          </p>
                        </div>
                      </div>
                    </div>
                  );
                })}
                {sending && (
                  <div className="flex items-center gap-2 px-11 text-xs text-muted-foreground">
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                    <span>{t("chat.thinking", "Thinking")}...</span>
                  </div>
                )}
                <div ref={messagesEndRef} />
              </div>
            )}
          </div>

          {/* ── Input area ── */}
          <div className="shrink-0 border-t bg-background px-5 py-3">
            <div className="mx-auto max-w-3xl">
              {/* Mention target indicator */}
              {committedMentionTargetID ? (
                <div className="mb-2 flex items-center gap-2 rounded-lg bg-blue-50 px-3 py-1.5 text-xs">
                  <Bot className="h-3.5 w-3.5 text-blue-500" />
                  <span className="text-slate-600">{t("threads.mentionResolved", "Target agent")}:</span>
                  <button
                    type="button"
                    className="inline-flex items-center gap-1 rounded-full bg-white px-2 py-0.5 font-semibold text-blue-800 shadow-sm transition-colors hover:bg-blue-100"
                    onClick={() => focusAgentProfile(committedMentionTargetID)}
                  >
                    @{committedMentionTargetID}
                  </button>
                  <span className="text-slate-500">
                    {committedMentionProfile?.name ?? committedMentionTargetID}
                  </span>
                  <span className="inline-flex items-center gap-1 rounded-full bg-white px-2 py-0.5 text-[10px] font-medium text-slate-600">
                    <span className={cn("h-1.5 w-1.5 rounded-full", agentStatusColor(committedMentionSession?.status ?? "active"))} />
                    {committedMentionSession?.status ?? "active"}
                  </span>
                </div>
              ) : null}

              {/* Input container */}
              <div className="relative">
                {/* Mention autocomplete popup */}
                {mentionDraft && mentionCandidates.length > 0 ? (
                  <div className="absolute bottom-full left-0 right-0 z-20 mb-2 overflow-hidden rounded-xl border bg-popover shadow-lg">
                    <div className="border-b px-3 py-1.5">
                      <span className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
                        {t("threads.mentionCandidates", "Select agent")}
                      </span>
                    </div>
                    <div className="py-1">
                      {mentionCandidates.map((candidate, index) => (
                        <button
                          key={candidate.id}
                          type="button"
                          className={cn(
                            "flex w-full items-center justify-between px-3 py-2 text-left text-sm transition-colors",
                            index === selectedMentionIndex ? "bg-accent" : "hover:bg-accent/50",
                          )}
                          onMouseDown={(e) => {
                            e.preventDefault();
                            applyMentionCandidate(candidate.id);
                          }}
                        >
                          <div className="flex items-center gap-2">
                            <div className="flex h-6 w-6 items-center justify-center rounded-full bg-emerald-100 text-emerald-700">
                              <Bot className="h-3 w-3" />
                            </div>
                            <span className="font-medium">@{candidate.id}</span>
                          </div>
                          <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                            <span className={cn("h-1.5 w-1.5 rounded-full", agentStatusColor(candidate.status))} />
                            {candidate.status}
                          </span>
                        </button>
                      ))}
                    </div>
                  </div>
                ) : null}

                <div className="flex items-center gap-2 rounded-xl border bg-muted/30 px-3 py-2 transition-colors focus-within:border-blue-300 focus-within:bg-background focus-within:ring-2 focus-within:ring-blue-100">
                  <Input
                    ref={messageInputRef}
                    placeholder={
                      thread.status !== "active"
                        ? t("threads.threadClosed", "Thread is closed")
                        : agentRoutingMode === "broadcast"
                          ? t("threads.messagePlaceholderBroadcast", "Type a message (broadcasts to all agents)...")
                          : t("threads.messagePlaceholder", "Type @ to mention an agent, or just send a message...")
                    }
                    className="h-auto flex-1 border-0 bg-transparent p-0 text-sm shadow-none focus-visible:ring-0"
                    value={newMessage}
                    onChange={(e) => handleMessageInputChange(e.target.value, e.target.selectionStart)}
                    onClick={(e) => updateMentionDraft(e.currentTarget.value, e.currentTarget.selectionStart)}
                    onKeyUp={(e) => updateMentionDraft(e.currentTarget.value, e.currentTarget.selectionStart)}
                    onBlur={() => {
                      window.setTimeout(() => setMentionDraft(null), 120);
                    }}
                    onKeyDown={(e) => {
                      if (mentionDraft && mentionCandidates.length > 0) {
                        if (e.key === "ArrowDown") {
                          e.preventDefault();
                          setSelectedMentionIndex((prev) => (prev + 1) % mentionCandidates.length);
                          return;
                        }
                        if (e.key === "ArrowUp") {
                          e.preventDefault();
                          setSelectedMentionIndex((prev) => (prev - 1 + mentionCandidates.length) % mentionCandidates.length);
                          return;
                        }
                        if (e.key === "Enter") {
                          e.preventDefault();
                          applyMentionCandidate(mentionCandidates[selectedMentionIndex].id);
                          return;
                        }
                        if (e.key === "Escape") {
                          setMentionDraft(null);
                          return;
                        }
                      }
                      if (e.key === "Enter" && !e.shiftKey) {
                        e.preventDefault();
                        void handleSend();
                      }
                    }}
                    disabled={sending || thread.status !== "active"}
                  />
                  <Button
                    size="icon"
                    className="h-8 w-8 shrink-0 rounded-lg"
                    onClick={handleSend}
                    disabled={!newMessage.trim() || sending || thread.status !== "active"}
                  >
                    <Send className="h-4 w-4" />
                  </Button>
                </div>
                <p className="mt-1.5 text-[11px] text-muted-foreground">
                  {agentRoutingMode === "broadcast"
                    ? t("threads.mentionHintBroadcast", "Broadcast mode: messages go to all active agents. Use @agent-id for targeting.")
                    : t("threads.mentionHintMentionOnly", "Mention-only mode: use @agent-id to direct messages to specific agents.")}
                </p>
              </div>
            </div>
          </div>
        </div>

        {/* ── Sidebar ── */}
        <div className="flex w-80 shrink-0 flex-col border-l bg-muted/10">
          {/* Tab bar */}
          <div className="flex shrink-0 border-b">
            <button
              type="button"
              className={cn(
                "flex flex-1 items-center justify-center gap-1.5 border-b-2 px-3 py-2.5 text-xs font-medium transition-colors",
                sidebarTab === "agents"
                  ? "border-blue-500 text-blue-600"
                  : "border-transparent text-muted-foreground hover:text-foreground",
              )}
              onClick={() => setSidebarTab("agents")}
            >
              <Bot className="h-3.5 w-3.5" />
              {t("threads.agents", "Agents")}
              {agentSessions.length > 0 && (
                <span className="rounded-full bg-muted px-1.5 text-[10px]">{agentSessions.length}</span>
              )}
            </button>
            <button
              type="button"
              className={cn(
                "flex flex-1 items-center justify-center gap-1.5 border-b-2 px-3 py-2.5 text-xs font-medium transition-colors",
                sidebarTab === "details"
                  ? "border-blue-500 text-blue-600"
                  : "border-transparent text-muted-foreground hover:text-foreground",
              )}
              onClick={() => setSidebarTab("details")}
            >
              <Settings2 className="h-3.5 w-3.5" />
              {t("threads.details", "Details")}
            </button>
          </div>

          {/* Tab content */}
          <div className="flex-1 overflow-y-auto">
            {sidebarTab === "agents" ? (
              <div className="space-y-4 p-4">
                {/* Invite Agent section */}
                <div className="space-y-2">
                  <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                    {t("threads.inviteAgent", "Invite Agent")}
                  </h3>
                  <div className="flex gap-2">
                    <Select
                      aria-label={t("threads.agentProfile", "Agent profile")}
                      className="flex-1 text-xs"
                      value={inviteProfileID}
                      onChange={(event) => setInviteProfileID(event.target.value)}
                      disabled={invitingAgent || inviteableProfiles.length === 0}
                    >
                      {inviteableProfiles.length === 0 ? (
                        <option value="">
                          {t("threads.noInviteableAgents", "No available agents")}
                        </option>
                      ) : (
                        inviteableProfiles.map((profile) => (
                          <option key={profile.id} value={profile.id}>
                            {profile.name ? `${profile.name} (${profile.id})` : profile.id}
                          </option>
                        ))
                      )}
                    </Select>
                    <Button
                      size="sm"
                      onClick={handleInviteAgent}
                      disabled={invitingAgent || !inviteProfileID}
                      className="shrink-0"
                    >
                      {invitingAgent ? (
                        <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" />
                      ) : (
                        <Plus className="mr-1 h-3.5 w-3.5" />
                      )}
                      {t("threads.inviteBtn", "Add")}
                    </Button>
                  </div>
                </div>

                {/* Agent Cards */}
                {agentSessions.length === 0 ? (
                  <div className="rounded-xl border border-dashed py-8 text-center">
                    <Bot className="mx-auto h-8 w-8 text-muted-foreground/30" />
                    <p className="mt-2 text-xs text-muted-foreground">
                      {t("threads.noAgents", "No agents joined yet")}
                    </p>
                    <p className="mt-1 text-[11px] text-muted-foreground/60">
                      {t("threads.noAgentsHint", "Use the selector above to invite an agent")}
                    </p>
                  </div>
                ) : (
                  <div className="space-y-2">
                    <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                      {t("threads.activeAgents", "Active Agents")} ({agentSessions.length})
                    </h3>
                    {agentSessions.map((s) => {
                      const profile = profileByID.get(s.agent_profile_id);
                      return (
                        <div
                          key={s.id}
                          ref={(node) => { agentCardRefs.current[s.agent_profile_id] = node; }}
                          data-testid={`agent-card-${s.agent_profile_id}`}
                          className={cn(
                            "rounded-xl border p-3 transition-all",
                            highlightedAgentProfileID === s.agent_profile_id
                              ? "border-blue-300 bg-blue-50 shadow-md"
                              : "border-border/60 bg-background hover:border-border",
                          )}
                        >
                          <div className="flex items-start gap-2.5">
                            <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-emerald-100 text-emerald-700">
                              <Bot className="h-4 w-4" />
                            </div>
                            <div className="min-w-0 flex-1">
                              <div className="flex items-center gap-1.5">
                                <span className="truncate text-sm font-medium">
                                  {profile?.name ?? s.agent_profile_id}
                                </span>
                                <span className="flex items-center gap-1 rounded-full bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground">
                                  <span className={cn("h-1.5 w-1.5 rounded-full", agentStatusColor(s.status))} />
                                  {s.status}
                                </span>
                              </div>
                              {profile?.name && (
                                <p className="mt-0.5 truncate text-[11px] text-muted-foreground">@{s.agent_profile_id}</p>
                              )}
                              <div className="mt-1.5 flex items-center gap-3 text-[11px] text-muted-foreground">
                                <span>{t("threads.turns", "Turns")}: {s.turn_count}</span>
                                <span>{((s.total_input_tokens + s.total_output_tokens) / 1000).toFixed(1)}k tokens</span>
                              </div>
                            </div>
                            <button
                              type="button"
                              className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
                              onClick={() => void handleRemoveAgent(s.id)}
                              disabled={removingAgentID === s.id}
                              aria-label={t("threads.removeAgentAria", { defaultValue: "Remove {{agent}}", agent: s.agent_profile_id })}
                            >
                              {removingAgentID === s.id ? (
                                <Loader2 className="h-3.5 w-3.5 animate-spin" />
                              ) : (
                                <X className="h-3.5 w-3.5" />
                              )}
                            </button>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                )}

                {/* Participants */}
                {participants.length > 0 && (
                  <div className="space-y-2">
                    <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                      {t("threads.participants", "Participants")} ({participants.length})
                    </h3>
                    <div className="space-y-1.5">
                      {participants.map((p) => (
                        <div key={p.id} className="flex items-center gap-2 rounded-lg px-2 py-1.5 text-sm">
                          <div className="flex h-6 w-6 items-center justify-center rounded-full bg-slate-100 text-slate-600">
                            <User className="h-3 w-3" />
                          </div>
                          <span className="truncate text-xs">{p.user_id}</span>
                          <Badge variant="outline" className="ml-auto text-[10px]">{p.role}</Badge>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            ) : (
              /* ── Details tab ── */
              <div className="space-y-4 p-4">
                {/* Summary */}
                <div className="space-y-2">
                  <button
                    type="button"
                    className="flex w-full items-center justify-between text-xs font-semibold uppercase tracking-wider text-muted-foreground"
                    onClick={() => setSummaryCollapsed(!summaryCollapsed)}
                  >
                    <span>{t("threads.summary", "Summary")}</span>
                    {summaryCollapsed ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronUp className="h-3.5 w-3.5" />}
                  </button>
                  {!summaryCollapsed && (
                    <div className="space-y-2">
                      <p className="text-[11px] text-muted-foreground">
                        {t(
                          "threads.summaryEntryHint",
                          "Capture decisions, scope, risks, and next actions.",
                        )}
                      </p>
                      <Textarea
                        value={summaryDraft}
                        onChange={(e) => setSummaryDraft(e.target.value)}
                        placeholder={t(
                          "threads.summaryPlaceholder",
                          "Capture the current consensus, decisions, scope, risks, and next actions for this thread.",
                        )}
                        className="min-h-[100px] resize-y text-xs"
                      />
                      <div className="flex justify-end">
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={handleSaveSummary}
                          disabled={savingSummary || summaryDraft.trim() === (thread.summary?.trim() ?? "")}
                        >
                          {savingSummary ? (
                            <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" />
                          ) : (
                            <Save className="mr-1 h-3.5 w-3.5" />
                          )}
                          {t("common.save", "Save")}
                        </Button>
                      </div>
                      {!hasSavedSummary(thread) && (
                        <p className="text-[11px] text-amber-600">
                          {t("threads.summaryMissingHint", "Save a summary first to create work items.")}
                        </p>
                      )}
                    </div>
                  )}
                </div>

                {/* Linked Work Items */}
                <div className="space-y-2">
                  <div className="flex items-center justify-between">
                    <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                      {t("threads.linkedWorkItems", "Work Items")} ({workItemLinks.length})
                    </h3>
                    <div className="flex gap-1">
                      <button
                        type="button"
                        className="flex h-6 items-center gap-1 rounded-md px-1.5 text-[11px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                        onClick={handleOpenCreateWorkItem}
                      >
                        <Plus className="h-3 w-3" />
                        {t("threads.createWorkItem", "Create")}
                      </button>
                      <button
                        type="button"
                        className="flex h-6 items-center gap-1 rounded-md px-1.5 text-[11px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                        onClick={() => setShowLinkWI(!showLinkWI)}
                      >
                        <Link2 className="h-3 w-3" />
                        {t("threads.linkExisting", "Link")}
                      </button>
                    </div>
                  </div>

                  {showCreateWI && (
                    <div className="space-y-2 rounded-lg border bg-muted/20 p-3">
                      <p className="text-[11px] font-medium">{t("threads.summaryToWorkItem", "Create from Summary")}</p>
                      <Input
                        placeholder={t("threads.workItemTitle", "Title...")}
                        className="text-xs"
                        value={newWITitle}
                        onChange={(e) => setNewWITitle(e.target.value)}
                        onKeyDown={(e) => e.key === "Enter" && !e.shiftKey && handleCreateWorkItem()}
                      />
                      <Textarea
                        placeholder={t("threads.workItemBody", "Body...")}
                        value={newWIBody}
                        onChange={(e) => setNewWIBody(e.target.value)}
                        className="min-h-[80px] resize-y text-xs"
                      />
                      <div className="flex justify-end gap-2">
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 text-xs"
                          onClick={() => { setShowCreateWI(false); setNewWITitle(""); setNewWIBody(""); }}
                        >
                          {t("common.cancel", "Cancel")}
                        </Button>
                        <Button size="sm" className="h-7 text-xs" onClick={handleCreateWorkItem} disabled={!newWITitle.trim() || !newWIBody.trim()}>
                          {t("common.create", "Create")}
                        </Button>
                      </div>
                    </div>
                  )}

                  {showLinkWI && (
                    <div className="flex gap-2">
                      <Input
                        placeholder={t("threads.workItemId", "Work item ID...")}
                        className="text-xs"
                        value={linkWIId}
                        onChange={(e) => setLinkWIId(e.target.value)}
                        onKeyDown={(e) => e.key === "Enter" && handleLinkWorkItem()}
                      />
                      <Button size="sm" className="h-8 text-xs" onClick={handleLinkWorkItem} disabled={!linkWIId.trim()}>
                        {t("threads.linkBtn", "Link")}
                      </Button>
                    </div>
                  )}

                  {workItemLinks.length === 0 ? (
                    <p className="py-4 text-center text-[11px] text-muted-foreground">
                      {t("threads.noLinkedWorkItems", "No linked work items")}
                    </p>
                  ) : (
                    <div className="space-y-1.5">
                      {orderedWorkItemLinks.map((link) => {
                        const issue = linkedIssues[link.work_item_id];
                        const sourceType = readSourceType(issue);
                        return (
                          <div
                            key={link.id}
                            className={cn(
                              "rounded-lg border px-3 py-2 text-xs",
                              link.is_primary ? "border-blue-200 bg-blue-50/50" : "border-border/60",
                            )}
                          >
                            <div className="flex items-center gap-1.5">
                              {link.is_primary && (
                                <Badge variant="default" className="text-[9px]">primary</Badge>
                              )}
                              <Badge variant="outline" className="text-[9px]">{link.relation_type}</Badge>
                              {sourceType ? (
                                <Badge variant="secondary" className="text-[9px]">
                                  {sourceType === "thread_summary" ? "summary" : sourceType === "thread_manual" ? "manual" : sourceType}
                                </Badge>
                              ) : null}
                              <Link
                                to={`/work-items/${link.work_item_id}`}
                                className="min-w-0 flex-1 truncate font-medium text-primary hover:underline"
                              >
                                {issue ? issue.title : `#${link.work_item_id}`}
                              </Link>
                              {issue && (
                                <Badge variant="secondary" className="text-[9px]">{issue.status}</Badge>
                              )}
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  )}
                </div>

                {/* Thread Metadata */}
                <div className="space-y-2">
                  <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                    {t("threads.info", "Thread Info")}
                  </h3>
                  <div className="space-y-1 rounded-lg border bg-muted/20 p-3 text-xs">
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">ID</span>
                      <span className="font-mono">{thread.id}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">{t("threads.status", "Status")}</span>
                      <span>{thread.status}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">{t("threads.owner", "Owner")}</span>
                      <span>{thread.owner_id || "—"}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">{t("threads.updated", "Updated")}</span>
                      <span>{formatRelativeTime(thread.updated_at)}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">{t("threads.messages", "Messages")}</span>
                      <span>{messages.length}</span>
                    </div>
                  </div>
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
