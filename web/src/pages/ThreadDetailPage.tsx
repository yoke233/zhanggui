import { useEffect, useRef, useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
  ArrowLeft,
  Bot,
  ExternalLink,
  Loader2,
  MessageSquare,
  Send,
  Settings2,
  Users,
  X,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { ThreadAgentsPanel } from "@/components/threads/ThreadAgentsPanel";
import { ThreadDetailsPanel } from "@/components/threads/ThreadDetailsPanel";
import { ThreadMessageList } from "@/components/threads/ThreadMessageList";
import { InvitePickerDialog } from "@/components/threads/InvitePickerDialog";
import { cn } from "@/lib/utils";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { formatRelativeTime, getErrorMessage } from "@/lib/v2Workbench";
import type {
  AgentProfile,
  Thread,
  ThreadMessage,
  ThreadParticipant,
  ThreadWorkItemLink,
  ThreadAgentSession,
  Issue,
  WorkItemTrack,
} from "@/types/apiV2";
import type { ThreadAckPayload, ThreadEventPayload } from "@/types/ws";

/* ── helper functions (unchanged) ── */

function hasSavedSummary(thread: Thread | null): boolean {
  return Boolean(thread?.summary?.trim());
}

function deriveWorkItemTitle(thread: Thread): string {
  const firstMeaningfulLine = (thread.summary ?? "")
    .split(/\r?\n/)
    .map((line) => line.replace(/^[-*#\d.)\s]+/, "").trim())
    .find((line) => line.length > 0);
  const title = firstMeaningfulLine || thread.title.trim();
  return title.length > 80 ? `${title.slice(0, 77)}...` : title;
}

function readTargetAgentID(metadata: Record<string, unknown> | undefined): string | null {
  const value = metadata?.target_agent_id;
  return typeof value === "string" && value.trim().length > 0 ? value.trim() : null;
}

function readAutoRoutedTo(metadata: Record<string, unknown> | undefined): string[] {
  const value = metadata?.auto_routed_to;
  if (!Array.isArray(value)) return [];
  return value.filter((v): v is string => typeof v === "string" && v.trim().length > 0).map((v) => v.trim());
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

function readAgentRoutingMode(thread: Thread | null): "mention_only" | "broadcast" | "auto" {
  const value = thread?.metadata?.agent_routing_mode;
  if (value === "broadcast") return "broadcast";
  if (value === "auto") return "auto";
  return "mention_only";
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

// Invite intent detection: match phrases like "把 XX 拉进来", "invite XX", "加个 XX" etc.
const INVITE_PATTERNS = [
  // Chinese patterns
  /(?:把|让|请|叫|邀请)\s*(.+?)\s*(?:拉进来|加进来|拉入|加入|进来|进群|加到|拉到)/,
  /(?:拉|加|邀请)\s*(?:个|一个|一位)?\s*(.+?)\s*(?:进来|进群|到群里|到线程|吧|$)/,
  /(?:需要|想要|想)\s*(.+?)\s*(?:加入|参与|进来|帮忙)/,
  // English patterns
  /(?:invite|add|bring|pull)\s+(?:in\s+)?(.+?)(?:\s+(?:in|to\s+(?:the\s+)?(?:thread|chat|group))|\s*$)/i,
  /(?:let's?\s+)?(?:get|bring)\s+(.+?)\s+(?:in|here|on\s+board)/i,
];

interface InviteIntentMatch {
  query: string;
  matchedProfiles: AgentProfile[];
}

function detectInviteIntent(message: string, inviteableProfiles: AgentProfile[]): InviteIntentMatch | null {
  const trimmed = message.trim();
  if (!trimmed) return null;

  for (const pattern of INVITE_PATTERNS) {
    const match = trimmed.match(pattern);
    if (!match || !match[1]) continue;

    const query = match[1].trim().toLowerCase();
    if (!query) continue;

    // Match query against profile name, id, role, capabilities
    const matched = inviteableProfiles.filter((profile) => {
      const name = (profile.name ?? "").toLowerCase();
      const id = profile.id.toLowerCase();
      const role = (typeof profile.role === "string" ? profile.role : "").toLowerCase();
      const caps = (profile.capabilities ?? []).map((c) => c.toLowerCase());

      // Check if query contains or is contained by any field
      return name.includes(query) || query.includes(name)
        || id.includes(query) || query.includes(id)
        || role.includes(query) || query.includes(role)
        || caps.some((c) => c.includes(query) || query.includes(c));
    });

    if (matched.length > 0) {
      return { query, matchedProfiles: matched };
    }
  }
  return null;
}

function trackStatusTone(status: string): string {
  switch (status) {
    case "awaiting_confirmation":
      return "border-amber-200 bg-amber-50 text-amber-700";
    case "materialized":
    case "done":
      return "border-emerald-200 bg-emerald-50 text-emerald-700";
    case "executing":
    case "planning":
    case "reviewing":
      return "border-blue-200 bg-blue-50 text-blue-700";
    case "failed":
    case "cancelled":
      return "border-rose-200 bg-rose-50 text-rose-700";
    case "paused":
      return "border-slate-200 bg-slate-50 text-slate-700";
    default:
      return "border-border bg-muted/40 text-muted-foreground";
  }
}

function canMaterializeTrack(track: WorkItemTrack): boolean {
  return track.status === "awaiting_confirmation" && !track.work_item_id;
}

function canSubmitTrackReview(track: WorkItemTrack): boolean {
  return track.status === "draft" || track.status === "planning" || track.status === "paused";
}

function canConfirmTrackExecution(track: WorkItemTrack): boolean {
  return track.status === "awaiting_confirmation" || track.status === "materialized";
}

type SidebarTab = "agents" | "details";
type ThreadAgentSessionWithProfileID = ThreadAgentSession & { agent_profile_id: string };

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
  const [tracks, setTracks] = useState<WorkItemTrack[]>([]);
  const [tracksLoading, setTracksLoading] = useState(false);
  const [startingTrack, setStartingTrack] = useState(false);
  const [materializingTrackID, setMaterializingTrackID] = useState<number | null>(null);
  const [trackActionBusyKey, setTrackActionBusyKey] = useState<string | null>(null);
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
  const [selectedInviteIDs, setSelectedInviteIDs] = useState<Set<string>>(new Set());
  const [invitingAgent, setInvitingAgent] = useState(false);
  const [removingAgentID, setRemovingAgentID] = useState<number | null>(null);
  const [savingRoutingMode, setSavingRoutingMode] = useState(false);
  const [mentionDraft, setMentionDraft] = useState<{ start: number; end: number; query: string } | null>(null);
  const [selectedMentionIndex, setSelectedMentionIndex] = useState(0);
  const [highlightedAgentProfileID, setHighlightedAgentProfileID] = useState<string | null>(null);
  const [hoveredMentionProfileID, setHoveredMentionProfileID] = useState<string | null>(null);
  const [sidebarTab, setSidebarTab] = useState<SidebarTab>("agents");
  const [summaryCollapsed, setSummaryCollapsed] = useState(true);
  const [thinkingAgentIDs, setThinkingAgentIDs] = useState<Set<string>>(new Set());
  const [invitePickerCandidates, setInvitePickerCandidates] = useState<AgentProfile[]>([]);
  const [invitePickerSelected, setInvitePickerSelected] = useState<Set<string>>(new Set());
  const [invitePickerBusy, setInvitePickerBusy] = useState(false);
  const pendingThreadRequestIdRef = useRef<string | null>(null);
  const syntheticMessageIDRef = useRef(-1);
  const messageInputRef = useRef<HTMLInputElement | null>(null);
  const agentCardRefs = useRef<Record<string, HTMLDivElement | null>>({});
  const messagesEndRef = useRef<HTMLDivElement | null>(null);

  const id = Number(threadId);
  const agentSessionsWithProfileID = agentSessions.filter(
    (session): session is ThreadAgentSessionWithProfileID =>
      typeof session.agent_profile_id === "string" && session.agent_profile_id.trim().length > 0,
  );
  const joinedAgentProfileIDs = new Set(agentSessionsWithProfileID.map((session) => session.agent_profile_id));
  const inviteableProfiles = availableProfiles.filter((profile) => !joinedAgentProfileIDs.has(profile.id));
  const activeAgentProfileIDs = agentSessionsWithProfileID
    .filter((session) => session.status === "active" || session.status === "booting")
    .map((session) => session.agent_profile_id);
  const agentRoutingMode = readAgentRoutingMode(thread);
  const profileByID = new Map(availableProfiles.map((profile) => [profile.id, profile]));
  const agentSessionByProfileID = new Map(agentSessionsWithProfileID.map((session) => [session.agent_profile_id, session]));
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
  const selectedMentionCandidate = mentionCandidates[selectedMentionIndex];
  const orderedWorkItemLinks = [...workItemLinks].sort((a, b) => {
    if (a.is_primary === b.is_primary) {
      return a.id - b.id;
    }
    return a.is_primary ? -1 : 1;
  });
  const orderedTracks = [...tracks].sort((a, b) => b.id - a.id);

  /* ── auto-scroll to bottom on new messages ── */
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages.length]);

  useEffect(() => {
    if (!id || isNaN(id)) return;
    let cancelled = false;

    const load = async () => {
      setLoading(true);
      setTracksLoading(true);
      setError(null);
      try {
        const [th, msgs, parts, links, trackItems, agents, profiles] = await Promise.all([
          apiClient.getThread(id),
          apiClient.listThreadMessages(id, { limit: 100 }),
          apiClient.listThreadParticipants(id),
          apiClient.listWorkItemsByThread(id),
          apiClient.listThreadTracks(id),
          apiClient.listThreadAgents(id),
          apiClient.listProfiles(),
        ]);
        if (!cancelled) {
          setThread(th);
          setSummaryDraft(th.summary ?? "");
          setMessages(msgs);
          setParticipants(parts);
          setWorkItemLinks(links);
          setTracks(trackItems);
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
        if (!cancelled) setTracksLoading(false);
        if (!cancelled) setLoading(false);
      }
    };
    void load();
    return () => { cancelled = true; };
  }, [apiClient, id]);

  useEffect(() => {
    // Remove selections that are no longer inviteable (e.g. agent already joined)
    setSelectedInviteIDs((prev) => {
      const inviteableSet = new Set(inviteableProfiles.map((p) => p.id));
      const next = new Set([...prev].filter((id) => inviteableSet.has(id)));
      return next.size === prev.size ? prev : next;
    });
  }, [inviteableProfiles]);

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

      const msgMetadata: Record<string, unknown> = {};
      if (payload.target_agent_id) {
        msgMetadata.target_agent_id = payload.target_agent_id;
      }
      if (Array.isArray(payload.auto_routed_to) && payload.auto_routed_to.length > 0) {
        msgMetadata.auto_routed_to = payload.auto_routed_to;
      }

      setMessages((prev) => [
        ...prev,
        {
          id: syntheticMessageIDRef.current--,
          thread_id: id,
          sender_id: senderID,
          role,
          content,
          metadata: Object.keys(msgMetadata).length > 0 ? msgMetadata : undefined,
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

    const syncTrackFromPayload = async (payload: ThreadEventPayload) => {
      if (payload.thread_id !== id) return;

      const rawTrack = payload.track;
      const nextTrack = rawTrack && typeof rawTrack === "object"
        ? rawTrack as unknown as WorkItemTrack
        : null;

      if (nextTrack) {
        setTracks((prev) => [nextTrack, ...prev.filter((item) => item.id !== nextTrack.id)]);
      } else if (typeof payload.track_id === "number") {
        try {
          const fetched = await apiClient.getTrack(payload.track_id);
          setTracks((prev) => [fetched, ...prev.filter((item) => item.id !== fetched.id)]);
        } catch {
          // Ignore background refresh failures
        }
      }

      if (typeof payload.work_item_id === "number") {
        try {
          const [links, issue] = await Promise.all([
            apiClient.listWorkItemsByThread(id),
            apiClient.getWorkItem(payload.work_item_id),
          ]);
          setWorkItemLinks(links);
          setLinkedIssues((prev) => ({
            ...prev,
            [payload.work_item_id as number]: issue,
          }));
        } catch {
          // Ignore background refresh failures
        }
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
      // Clear thinking state for this agent since it has responded.
      const agentID = payload.profile_id?.trim() || payload.sender_id?.trim();
      if (agentID) {
        setThinkingAgentIDs((prev) => {
          if (!prev.has(agentID)) return prev;
          const next = new Set(prev);
          next.delete(agentID);
          return next;
        });
      }
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
      // Clear thinking state for failed agent.
      const failedID = payload.profile_id?.trim();
      if (failedID) {
        setThinkingAgentIDs((prev) => {
          if (!prev.has(failedID)) return prev;
          const next = new Set(prev);
          next.delete(failedID);
          return next;
        });
      }
      setError(payload.error?.trim() || t("threads.agentFailed", "An agent in this thread failed."));
      void refreshAgentSessions();
    });
    const unsubscribeThreadAgentThinking = wsClient.subscribe<ThreadEventPayload>("thread.agent_thinking", (payload) => {
      if (payload.thread_id !== id) return;
      const thinkingID = payload.profile_id?.trim();
      if (thinkingID) {
        setThinkingAgentIDs((prev) => {
          if (prev.has(thinkingID)) return prev;
          const next = new Set(prev);
          next.add(thinkingID);
          return next;
        });
      }
    });
    const unsubscribeTrackCreated = wsClient.subscribe<ThreadEventPayload>("thread.track.created", (payload) => {
      void syncTrackFromPayload(payload);
    });
    const unsubscribeTrackUpdated = wsClient.subscribe<ThreadEventPayload>("thread.track.updated", (payload) => {
      void syncTrackFromPayload(payload);
    });
    const unsubscribeTrackStateChanged = wsClient.subscribe<ThreadEventPayload>("thread.track.state_changed", (payload) => {
      void syncTrackFromPayload(payload);
    });
    const unsubscribeTrackReviewApproved = wsClient.subscribe<ThreadEventPayload>("thread.track.review_approved", (payload) => {
      void syncTrackFromPayload(payload);
    });
    const unsubscribeTrackReviewRejected = wsClient.subscribe<ThreadEventPayload>("thread.track.review_rejected", (payload) => {
      void syncTrackFromPayload(payload);
    });
    const unsubscribeTrackMaterialized = wsClient.subscribe<ThreadEventPayload>("thread.track.materialized", (payload) => {
      void syncTrackFromPayload(payload);
    });
    const unsubscribeTrackExecutionConfirmed = wsClient.subscribe<ThreadEventPayload>("thread.track.execution_confirmed", (payload) => {
      void syncTrackFromPayload(payload);
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
      unsubscribeThreadAgentThinking();
      unsubscribeTrackCreated();
      unsubscribeTrackUpdated();
      unsubscribeTrackStateChanged();
      unsubscribeTrackReviewApproved();
      unsubscribeTrackReviewRejected();
      unsubscribeTrackMaterialized();
      unsubscribeTrackExecutionConfirmed();
      unsubscribeStatus();
      pendingThreadRequestIdRef.current = null;
      setThinkingAgentIDs(new Set());
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

    // Detect invite intent before sending as a regular message.
    const inviteIntent = detectInviteIntent(newMessage, inviteableProfiles);
    if (inviteIntent) {
      if (inviteIntent.matchedProfiles.length === 1) {
        // Single match → auto-invite directly.
        const profile = inviteIntent.matchedProfiles[0];
        setNewMessage("");
        setInvitingAgent(true);
        setError(null);
        try {
          await apiClient.inviteThreadAgent(id, { agent_profile_id: profile.id });
          const sessions = await apiClient.listThreadAgents(id);
          setAgentSessions(sessions);
          // Inject a local system message to confirm.
          setMessages((prev) => [
            ...prev,
            {
              id: syntheticMessageIDRef.current--,
              thread_id: id,
              sender_id: "system",
              role: "system",
              content: `已邀请 ${profile.name ?? profile.id} 加入对话`,
              created_at: new Date().toISOString(),
            },
          ]);
        } catch (e) {
          setError(getErrorMessage(e));
        } finally {
          setInvitingAgent(false);
        }
        return;
      }
      // Multiple matches → show picker dialog.
      setInvitePickerCandidates(inviteIntent.matchedProfiles);
      setInvitePickerSelected(new Set());
      return;
    }

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

  const handleInvitePickerConfirm = async () => {
    if (!id || invitePickerSelected.size === 0) return;
    setInvitePickerBusy(true);
    setError(null);
    const ids = [...invitePickerSelected];
    try {
      for (const profileID of ids) {
        await apiClient.inviteThreadAgent(id, { agent_profile_id: profileID });
      }
      const sessions = await apiClient.listThreadAgents(id);
      setAgentSessions(sessions);
      // Inject system message.
      const names = ids.map((pid) => {
        const p = invitePickerCandidates.find((c) => c.id === pid);
        return p?.name ?? pid;
      });
      setMessages((prev) => [
        ...prev,
        {
          id: syntheticMessageIDRef.current--,
          thread_id: id,
          sender_id: "system",
          role: "system",
          content: `已邀请 ${names.join(", ")} 加入对话`,
          created_at: new Date().toISOString(),
        },
      ]);
      setNewMessage("");
    } catch (e) {
      setError(getErrorMessage(e));
    } finally {
      setInvitePickerBusy(false);
      setInvitePickerCandidates([]);
      setInvitePickerSelected(new Set());
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

  const handleStartTrack = async () => {
    if (!id || !thread) return;
    setStartingTrack(true);
    setError(null);
    try {
      const track = await apiClient.createThreadTrack(id, {
        title: deriveWorkItemTitle(thread),
        objective: summaryDraft.trim() || thread.summary?.trim() || thread.title.trim(),
        created_by: thread.owner_id ?? undefined,
        metadata: {
          source: "thread_detail_page",
        },
      });
      setTracks((prev) => [track, ...prev.filter((item) => item.id !== track.id)]);
    } catch (e) {
      setError(getErrorMessage(e));
    } finally {
      setStartingTrack(false);
    }
  };

  const handleMaterializeTrack = async (track: WorkItemTrack) => {
    setMaterializingTrackID(track.id);
    setError(null);
    try {
      const result = await apiClient.materializeTrack(track.id);
      setTracks((prev) => prev.map((item) => (item.id === track.id ? result.track : item)));
      setWorkItemLinks(result.links);
      setLinkedIssues((prev) => ({
        ...prev,
        [result.work_item.id]: result.work_item,
      }));
    } catch (e) {
      setError(getErrorMessage(e));
    } finally {
      setMaterializingTrackID(null);
    }
  };

  const handleOpenTrackWorkItem = (track: WorkItemTrack) => {
    if (!track.work_item_id) return;
    navigate(`/work-items/${track.work_item_id}`);
  };

  const replaceTrack = (nextTrack: WorkItemTrack) => {
    setTracks((prev) => [nextTrack, ...prev.filter((item) => item.id !== nextTrack.id)]);
  };

  const handleSubmitTrackReview = async (track: WorkItemTrack) => {
    setTrackActionBusyKey(`submit-${track.id}`);
    setError(null);
    try {
      const updated = await apiClient.submitTrackReview(track.id, {
        latest_summary: track.latest_summary?.trim() || track.objective?.trim() || thread?.summary?.trim(),
      });
      replaceTrack(updated);
    } catch (e) {
      setError(getErrorMessage(e));
    } finally {
      setTrackActionBusyKey(null);
    }
  };

  const handleApproveTrackReview = async (track: WorkItemTrack) => {
    setTrackActionBusyKey(`approve-${track.id}`);
    setError(null);
    try {
      const updated = await apiClient.approveTrackReview(track.id, {
        latest_summary: track.latest_summary?.trim() || track.objective?.trim(),
      });
      replaceTrack(updated);
    } catch (e) {
      setError(getErrorMessage(e));
    } finally {
      setTrackActionBusyKey(null);
    }
  };

  const handleRejectTrackReview = async (track: WorkItemTrack) => {
    setTrackActionBusyKey(`reject-${track.id}`);
    setError(null);
    try {
      const updated = await apiClient.rejectTrackReview(track.id, {
        latest_summary: track.latest_summary?.trim() || track.objective?.trim(),
      });
      replaceTrack(updated);
    } catch (e) {
      setError(getErrorMessage(e));
    } finally {
      setTrackActionBusyKey(null);
    }
  };

  const handlePauseTrack = async (track: WorkItemTrack) => {
    setTrackActionBusyKey(`pause-${track.id}`);
    setError(null);
    try {
      const updated = await apiClient.pauseTrack(track.id);
      replaceTrack(updated);
    } catch (e) {
      setError(getErrorMessage(e));
    } finally {
      setTrackActionBusyKey(null);
    }
  };

  const handleCancelTrack = async (track: WorkItemTrack) => {
    setTrackActionBusyKey(`cancel-${track.id}`);
    setError(null);
    try {
      const updated = await apiClient.cancelTrack(track.id);
      replaceTrack(updated);
    } catch (e) {
      setError(getErrorMessage(e));
    } finally {
      setTrackActionBusyKey(null);
    }
  };

  const handleConfirmTrackExecution = async (track: WorkItemTrack) => {
    setTrackActionBusyKey(`confirm-${track.id}`);
    setError(null);
    try {
      const result = await apiClient.confirmTrackExecution(track.id);
      replaceTrack(result.track);
      setLinkedIssues((prev) => ({
        ...prev,
        [result.work_item.id]: result.work_item,
      }));
      try {
        const links = await apiClient.listWorkItemsByThread(id);
        setWorkItemLinks(links);
      } catch {
        // Ignore follow-up refresh failures
      }
    } catch (e) {
      setError(getErrorMessage(e));
    } finally {
      setTrackActionBusyKey(null);
    }
  };

  const toggleInviteSelection = (profileID: string) => {
    setSelectedInviteIDs((prev) => {
      const next = new Set(prev);
      if (next.has(profileID)) {
        next.delete(profileID);
      } else {
        next.add(profileID);
      }
      return next;
    });
  };

  const handleInviteAgent = async () => {
    if (!id || selectedInviteIDs.size === 0) return;
    setInvitingAgent(true);
    setError(null);
    const ids = [...selectedInviteIDs];
    try {
      for (const profileID of ids) {
        await apiClient.inviteThreadAgent(id, { agent_profile_id: profileID });
      }
      const sessions = await apiClient.listThreadAgents(id);
      setAgentSessions(sessions);
      setSelectedInviteIDs(new Set());
    } catch (e) {
      setError(getErrorMessage(e));
      // Refresh sessions in case some succeeded
      try {
        const sessions = await apiClient.listThreadAgents(id);
        setAgentSessions(sessions);
      } catch { /* ignore */ }
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

  const handleSetRoutingMode = async (nextMode: "mention_only" | "broadcast" | "auto") => {
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
                {t("threads.turns", "Turns")}: {session?.turn_count ?? 0} | {(((session?.total_input_tokens ?? 0) + (session?.total_output_tokens ?? 0)) / 1000).toFixed(1)}k {t("threads.tokens", "tokens")}
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
            <button
              type="button"
              className={cn(
                "rounded-md px-2.5 py-1 transition-colors",
                agentRoutingMode === "auto" ? "bg-background font-medium shadow-sm" : "text-muted-foreground hover:text-foreground",
              )}
              onClick={() => void handleSetRoutingMode("auto")}
              disabled={savingRoutingMode}
            >
              {t("threads.routingAuto", "Auto")}
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

      {/* ── Invite picker dialog ── */}
      <InvitePickerDialog
        candidates={invitePickerCandidates}
        selectedIDs={invitePickerSelected}
        busy={invitePickerBusy}
        onToggle={(profileID) => {
          setInvitePickerSelected((prev) => {
            const next = new Set(prev);
            if (next.has(profileID)) next.delete(profileID);
            else next.add(profileID);
            return next;
          });
        }}
        onClose={() => {
          setInvitePickerCandidates([]);
          setInvitePickerSelected(new Set());
        }}
        onConfirm={handleInvitePickerConfirm}
      />

      <div className="border-b bg-muted/10 px-5 py-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="min-w-0">
            <div className="text-[11px] font-semibold uppercase tracking-widest text-muted-foreground">
              Thread WorkItem Track
            </div>
            <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <span>{tracks.length} tracks</span>
              {tracksLoading ? (
                <span className="inline-flex items-center gap-1">
                  <Loader2 className="h-3 w-3 animate-spin" />
                  加载中
                </span>
              ) : null}
            </div>
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-8 text-xs"
            onClick={() => void handleStartTrack()}
            disabled={startingTrack || !thread}
          >
            {startingTrack ? <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" /> : null}
            开始孵化
          </Button>
        </div>

        {orderedTracks.length === 0 ? (
          <div className="mt-3 rounded-lg border border-dashed bg-background/70 px-3 py-3 text-xs text-muted-foreground">
            当前 thread 还没有 Track。可先保存 summary，再点击“开始孵化”创建第一条任务轨道。
          </div>
        ) : (
          <div className="mt-3 flex flex-col gap-2">
            {orderedTracks.map((track) => {
              const linkedIssue = track.work_item_id ? linkedIssues[track.work_item_id] : undefined;
              const materializing = materializingTrackID === track.id;
              const submitting = trackActionBusyKey === `submit-${track.id}`;
              const approving = trackActionBusyKey === `approve-${track.id}`;
              const rejecting = trackActionBusyKey === `reject-${track.id}`;
              const pausing = trackActionBusyKey === `pause-${track.id}`;
              const cancelling = trackActionBusyKey === `cancel-${track.id}`;
              const confirming = trackActionBusyKey === `confirm-${track.id}`;
              return (
                <div
                  key={track.id}
                  className="rounded-xl border bg-background/80 px-3 py-3"
                >
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="truncate text-sm font-semibold text-foreground">{track.title}</span>
                        <Badge
                          variant="outline"
                          className={cn("text-[10px] normal-case", trackStatusTone(track.status))}
                        >
                          {track.status}
                        </Badge>
                        {track.work_item_id ? (
                          <Badge variant="secondary" className="text-[10px]">
                            WorkItem #{track.work_item_id}
                          </Badge>
                        ) : null}
                      </div>
                      <div className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                        {track.latest_summary?.trim() || track.objective?.trim() || "暂无轨道摘要"}
                      </div>
                      <div className="mt-2 flex flex-wrap items-center gap-3 text-[11px] text-muted-foreground">
                        <span>Track #{track.id}</span>
                        <span>更新于 {formatRelativeTime(track.updated_at)}</span>
                        {linkedIssue ? <span>{linkedIssue.title}</span> : null}
                      </div>
                    </div>

                    <div className="flex shrink-0 items-center gap-2">
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        className="h-8 text-xs"
                        onClick={() => void handleSubmitTrackReview(track)}
                        disabled={!canSubmitTrackReview(track) || submitting}
                      >
                        {submitting ? <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" /> : null}
                        送审
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        className="h-8 text-xs"
                        onClick={() => void handleApproveTrackReview(track)}
                        disabled={track.status !== "reviewing" || approving}
                      >
                        {approving ? <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" /> : null}
                        审核通过
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        className="h-8 text-xs"
                        onClick={() => void handleRejectTrackReview(track)}
                        disabled={track.status !== "reviewing" || rejecting}
                      >
                        {rejecting ? <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" /> : null}
                        打回
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        className="h-8 text-xs"
                        onClick={() => void handleMaterializeTrack(track)}
                        disabled={!canMaterializeTrack(track) || materializing}
                      >
                        {materializing ? <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" /> : null}
                        生成待办
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        className="h-8 text-xs"
                        onClick={() => void handleConfirmTrackExecution(track)}
                        disabled={!canConfirmTrackExecution(track) || confirming}
                      >
                        {confirming ? <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" /> : null}
                        生成并执行
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        className="h-8 text-xs"
                        onClick={() => void handlePauseTrack(track)}
                        disabled={pausing || ["done", "cancelled", "failed"].includes(track.status)}
                      >
                        {pausing ? <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" /> : null}
                        暂停
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        className="h-8 text-xs text-rose-600 hover:text-rose-700"
                        onClick={() => void handleCancelTrack(track)}
                        disabled={cancelling || ["done", "cancelled", "failed"].includes(track.status)}
                      >
                        {cancelling ? <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" /> : null}
                        取消
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        className="h-8 text-xs"
                        onClick={() => handleOpenTrackWorkItem(track)}
                        disabled={!track.work_item_id}
                      >
                        <ExternalLink className="mr-1 h-3.5 w-3.5" />
                        查看关联 WorkItem
                      </Button>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* ── Main content: chat + sidebar ── */}
      <div className="flex min-h-0 flex-1">
        {/* ── Chat area ── */}
        <div className="flex min-w-0 flex-1 flex-col">
          {/* ── Messages ── */}
          <div className="flex-1 overflow-y-auto px-5 py-4">
            <ThreadMessageList
              messages={messages}
              profileByID={profileByID}
              thinkingAgentIDs={thinkingAgentIDs}
              sending={sending}
              messagesEndRef={messagesEndRef}
              renderMessageContent={renderMessageContent}
              focusAgentProfile={focusAgentProfile}
              readTargetAgentID={readTargetAgentID}
              readAutoRoutedTo={readAutoRoutedTo}
              formatRelativeTime={formatRelativeTime}
            />
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
                        : agentRoutingMode === "auto"
                          ? t("threads.messagePlaceholderAuto", "Type a message (auto-routed to the best-fit agent)...")
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
                          if (selectedMentionCandidate) {
                            applyMentionCandidate(selectedMentionCandidate.id);
                          }
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
                  {agentRoutingMode === "auto"
                    ? t("threads.mentionHintAuto", "Auto mode: messages are automatically routed to the best-fit agent based on content analysis.")
                    : agentRoutingMode === "broadcast"
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
              <ThreadAgentsPanel
                inviteableProfiles={inviteableProfiles}
                selectedInviteIDs={selectedInviteIDs}
                invitingAgent={invitingAgent}
                onToggleInviteSelection={toggleInviteSelection}
                onInviteAgent={() => {
                  void handleInviteAgent();
                }}
                agentSessionsWithProfileID={agentSessionsWithProfileID}
                profileByID={profileByID}
                highlightedAgentProfileID={highlightedAgentProfileID}
                agentCardRefs={agentCardRefs}
                removingAgentID={removingAgentID}
                onRemoveAgent={(agentSessionID) => {
                  void handleRemoveAgent(agentSessionID);
                }}
                participants={participants}
                agentStatusColor={agentStatusColor}
              />
            ) : (
              /* ── Details tab ── */
              <ThreadDetailsPanel
                thread={thread}
                messagesCount={messages.length}
                summaryCollapsed={summaryCollapsed}
                summaryDraft={summaryDraft}
                savingSummary={savingSummary}
                showSummaryMissingHint={!hasSavedSummary(thread)}
                showCreateWI={showCreateWI}
                newWITitle={newWITitle}
                newWIBody={newWIBody}
                showLinkWI={showLinkWI}
                linkWIId={linkWIId}
                workItemLinks={workItemLinks}
                orderedWorkItemLinks={orderedWorkItemLinks}
                linkedIssues={linkedIssues}
                onSummaryCollapsedChange={setSummaryCollapsed}
                onSummaryDraftChange={setSummaryDraft}
                onSaveSummary={handleSaveSummary}
                onOpenCreateWorkItem={handleOpenCreateWorkItem}
                onShowCreateWIChange={setShowCreateWI}
                onNewWITitleChange={setNewWITitle}
                onNewWIBodyChange={setNewWIBody}
                onCreateWorkItem={handleCreateWorkItem}
                onShowLinkWIChange={setShowLinkWI}
                onLinkWIIdChange={setLinkWIId}
                onLinkWorkItem={handleLinkWorkItem}
                onResetCreateWorkItemDraft={() => {
                  setNewWITitle("");
                  setNewWIBody("");
                }}
              />
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
