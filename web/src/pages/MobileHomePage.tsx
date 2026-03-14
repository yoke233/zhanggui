import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import {
  GitBranch,
  Search,
  Settings,
  Filter,
  Send,
  Paperclip,
  X,
  Loader2,
  ChevronDown,
} from "lucide-react";
import type {
  AgentDriver,
  AgentProfile,
  ChatSessionSummary,
} from "@/types/apiV2";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { getErrorMessage } from "@/lib/v2Workbench";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { cn } from "@/lib/utils";
import type { LeadDriverOption, SessionRecord } from "@/components/chat/chatTypes";
import { EMPTY_PROFILE_VALUE } from "@/components/chat/chatTypes";
import {
  toSummaryRecord,
  normalizeDriverKey,
  driverLabelForId,
  fallbackLabel,
} from "@/components/chat/chatUtils";

/** Status indicator dot color */
function sessionStatusColor(status: string): string {
  switch (status) {
    case "running":
      return "bg-blue-500";
    case "alive":
      return "bg-emerald-500";
    case "closed":
      return "bg-muted-foreground/40";
    default:
      return "bg-muted-foreground/40";
  }
}

/** Status indicator icon — animated for running */
function SessionStatusIcon({ status }: { status: string }) {
  if (status === "running") {
    return (
      <span className="relative flex h-2.5 w-2.5">
        <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-blue-400 opacity-75" />
        <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-blue-500" />
      </span>
    );
  }
  return <span className={cn("inline-block h-2.5 w-2.5 rounded-full", sessionStatusColor(status))} />;
}

/** Format relative date for session grouping (Today / Yesterday / older date) */
function formatSessionDate(dateStr: string): string {
  const date = new Date(dateStr);
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const yesterday = new Date(today.getTime() - 86400000);
  const sessionDay = new Date(date.getFullYear(), date.getMonth(), date.getDate());

  if (sessionDay.getTime() === today.getTime()) return "Today";
  if (sessionDay.getTime() === yesterday.getTime()) return "Yesterday";
  return date.toLocaleDateString("zh-CN", { month: "short", day: "numeric" });
}

interface SessionDateGroup {
  label: string;
  sessions: SessionRecord[];
}

function groupSessionsByDate(sessions: SessionRecord[]): SessionDateGroup[] {
  const groups = new Map<string, SessionDateGroup>();
  for (const session of sessions) {
    const label = formatSessionDate(session.updated_at);
    const existing = groups.get(label);
    if (existing) {
      existing.sessions.push(session);
    } else {
      groups.set(label, { label, sessions: [session] });
    }
  }
  return Array.from(groups.values());
}

export function MobileHomePage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const {
    apiClient,
    wsClient,
    projects,
    selectedProjectId,
    setSelectedProjectId,
  } = useWorkbench();

  // Sessions
  const [sessions, setSessions] = useState<SessionRecord[]>([]);
  const [loadingSessions, setLoadingSessions] = useState(false);

  // Lead drivers/profiles
  const [drivers, setDrivers] = useState<AgentDriver[]>([]);
  const [leadProfiles, setLeadProfiles] = useState<AgentProfile[]>([]);
  const [draftProjectId, setDraftProjectId] = useState<number | null>(selectedProjectId);
  const [draftDriverId, setDraftDriverId] = useState("");

  // Input state
  const [messageInput, setMessageInput] = useState("");
  const [pendingFiles, setPendingFiles] = useState<File[]>([]);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Search / filter
  const [sessionSearch, setSessionSearch] = useState("");
  const [showSearch, setShowSearch] = useState(false);
  const [showFilters, setShowFilters] = useState(false);

  // Load sessions
  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      setLoadingSessions(true);
      try {
        const list = await apiClient.listChatSessions();
        if (!cancelled) {
          const sorted = list
            .map((s) => toSummaryRecord(s, t))
            .sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime());
          setSessions(sorted);
        }
      } catch (e) {
        if (!cancelled) setError(getErrorMessage(e));
      } finally {
        if (!cancelled) setLoadingSessions(false);
      }
    };
    void load();
    return () => { cancelled = true; };
  }, [apiClient, t]);

  // Load drivers/profiles
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [profiles, driverList] = await Promise.all([
          apiClient.listProfiles(),
          apiClient.listDrivers(),
        ]);
        if (cancelled) return;
        const leads = profiles.filter((p) => p.role === "lead");
        setDrivers(driverList);
        setLeadProfiles(leads);
        setDraftDriverId((cur) => {
          if (cur && driverList.some((d) => d.id === cur)) return cur;
          return driverList[0]?.id ?? "";
        });
      } catch (e) {
        if (!cancelled) setError(getErrorMessage(e));
      }
    })();
    return () => { cancelled = true; };
  }, [apiClient]);

  // Computed
  const leadDriverOptions = useMemo<LeadDriverOption[]>(() => {
    const grouped = new Map<string, LeadDriverOption>();
    for (const driver of drivers) {
      const key = normalizeDriverKey(driver.id);
      if (!key) continue;
      if (!grouped.has(key)) {
        grouped.set(key, {
          key,
          label: driverLabelForId(driver.id, t),
          driverId: driver.id,
        });
      }
    }
    return Array.from(grouped.values());
  }, [drivers, t]);

  const draftProfileId = useMemo(() => leadProfiles[0]?.id ?? "", [leadProfiles]);
  const draftSessionReady = Boolean(draftProfileId && draftDriverId);
  const currentProjectLabel = fallbackLabel(
    projects.find((p) => p.id === draftProjectId)?.name,
    t("chat.noProject"),
  );
  const currentDriverLabel = draftDriverId
    ? (leadDriverOptions.find((o) => o.driverId === draftDriverId)?.label ?? driverLabelForId(draftDriverId, t))
    : t("chat.noDriver");

  const filteredSessions = useMemo(() => {
    const query = sessionSearch.trim().toLowerCase();
    if (!query) return sessions;
    return sessions.filter((s) =>
      [s.title, s.project_name, s.branch].some((v) => (v ?? "").toLowerCase().includes(query)),
    );
  }, [sessions, sessionSearch]);

  const dateGroups = useMemo(() => groupSessionsByDate(filteredSessions), [filteredSessions]);

  // Send message -> navigate to chat page with new session
  const sendMessage = useCallback(async () => {
    const content = messageInput.trim();
    if ((!content && pendingFiles.length === 0) || submitting) return;

    if (!draftProfileId || !draftDriverId) {
      setError(t("chat.selectDriverFirst"));
      return;
    }

    const attachments: { name: string; mime_type: string; data: string }[] = [];
    for (const file of pendingFiles) {
      const buf = await file.arrayBuffer();
      const b64 = btoa(String.fromCharCode(...new Uint8Array(buf)));
      attachments.push({ name: file.name, mime_type: file.type || "application/octet-stream", data: b64 });
    }

    setSubmitting(true);
    setError(null);

    try {
      const requestId = `chat-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
      const resolvedProjectId = draftProjectId ?? undefined;
      const resolvedProjectName = resolvedProjectId != null
        ? projects.find((p) => p.id === resolvedProjectId)?.name
        : undefined;

      wsClient.send({
        type: "chat.send",
        data: {
          request_id: requestId,
          message: content || t("chat.attachment"),
          attachments: attachments.length > 0 ? attachments : undefined,
          project_id: resolvedProjectId,
          project_name: resolvedProjectName,
          profile_id: draftProfileId,
          driver_id: draftDriverId,
        },
      });

      // Navigate to chat page — the ack handler there will pick up the new session
      navigate("/chat");
    } catch (sendError) {
      setError(getErrorMessage(sendError));
      setSubmitting(false);
    }
  }, [messageInput, pendingFiles, submitting, draftProfileId, draftDriverId, draftProjectId, projects, wsClient, navigate, t]);

  const handlePaste = (e: React.ClipboardEvent) => {
    const items = e.clipboardData?.items;
    if (!items) return;
    const newFiles: File[] = [];
    for (const item of items) {
      if (item.kind === "file") {
        const file = item.getAsFile();
        if (file) newFiles.push(file);
      }
    }
    if (newFiles.length > 0) {
      setPendingFiles((prev) => [...prev, ...newFiles]);
    }
  };

  const handleKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      void sendMessage();
    }
  };

  return (
    <div className="flex h-full flex-col overflow-hidden bg-background">
      {/* ======= Header ======= */}
      <header className="shrink-0 border-b px-4 py-3 md:px-6">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary text-primary-foreground">
              <GitBranch className="h-4 w-4" />
            </div>
            <div>
              <h1 className="text-base font-semibold tracking-tight">AI Workflow</h1>
            </div>
          </div>
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8"
              onClick={() => setShowSearch(!showSearch)}
            >
              <Search className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8"
              onClick={() => setShowFilters(!showFilters)}
            >
              <Filter className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </header>

      {/* ======= Main scrollable area ======= */}
      <div className="flex-1 overflow-y-auto">
        {/* Chat input card */}
        <div className="px-4 pt-4 md:px-6">
          <div className="rounded-2xl border bg-gradient-to-br from-white via-slate-50 to-slate-100 p-4 shadow-sm dark:from-slate-900 dark:via-slate-800 dark:to-slate-900">
            {/* Input area */}
            <div className="space-y-3">
              <textarea
                placeholder={
                  draftSessionReady
                    ? t("mobileHome.inputPlaceholder", {
                        defaultValue: "输入消息，开始与 Lead 对话...",
                      })
                    : t("chat.selectDriverFirst")
                }
                className="w-full resize-none rounded-xl border bg-white/90 px-4 py-3 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-primary/20 disabled:opacity-50 dark:bg-slate-800/90"
                rows={3}
                value={messageInput}
                disabled={submitting || !draftSessionReady}
                onChange={(e) => setMessageInput(e.target.value)}
                onPaste={handlePaste}
                onKeyDown={handleKeyDown}
              />

              {/* Pending files */}
              {pendingFiles.length > 0 && (
                <div className="flex flex-wrap gap-2">
                  {pendingFiles.map((file, idx) => (
                    <Badge key={idx} variant="secondary" className="gap-1 text-xs">
                      {file.name}
                      <button
                        type="button"
                        onClick={() => setPendingFiles((prev) => prev.filter((_, i) => i !== idx))}
                        className="ml-1 hover:text-red-500"
                      >
                        <X className="h-3 w-3" />
                      </button>
                    </Badge>
                  ))}
                </div>
              )}

              {/* Bottom bar: selectors + send */}
              <div className="flex items-center justify-between gap-2">
                <div className="flex items-center gap-2 overflow-x-auto">
                  {/* Project selector */}
                  <Select
                    value={draftProjectId == null ? "" : String(draftProjectId)}
                    onChange={(e) => {
                      const v = e.target.value;
                      const nextId = v ? Number(v) : null;
                      setDraftProjectId(nextId);
                      setSelectedProjectId(nextId);
                    }}
                    className="h-8 max-w-[140px] text-xs"
                  >
                    <option value="">{t("chat.noProject")}</option>
                    {projects.map((p) => (
                      <option key={p.id} value={p.id}>{p.name}</option>
                    ))}
                  </Select>

                  {/* Driver selector */}
                  <Select
                    value={draftDriverId || EMPTY_PROFILE_VALUE}
                    onChange={(e) => {
                      const v = e.target.value;
                      setDraftDriverId(v === EMPTY_PROFILE_VALUE ? "" : v);
                    }}
                    className="h-8 max-w-[140px] text-xs"
                  >
                    <option value={EMPTY_PROFILE_VALUE}>{t("chat.selectDriver")}</option>
                    {leadDriverOptions.map((opt) => (
                      <option key={opt.driverId} value={opt.driverId}>{opt.label}</option>
                    ))}
                  </Select>
                </div>

                <div className="flex shrink-0 items-center gap-1.5">
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8"
                    disabled={submitting || !draftSessionReady}
                    onClick={() => fileInputRef.current?.click()}
                    title={t("chat.uploadFile")}
                  >
                    <Paperclip className="h-4 w-4" />
                  </Button>
                  <Button
                    size="icon"
                    className="h-8 w-8 rounded-full"
                    disabled={submitting || !draftSessionReady}
                    onClick={() => void sendMessage()}
                  >
                    {submitting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
                  </Button>
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* Search bar (conditional) */}
        {showSearch && (
          <div className="px-4 pt-3 md:px-6">
            <div className="relative">
              <Search className="absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder={t("chat.searchSessions")}
                className="h-9 pl-9 text-sm"
                value={sessionSearch}
                onChange={(e) => setSessionSearch(e.target.value)}
                autoFocus
              />
            </div>
          </div>
        )}

        {/* Filter bar (conditional) */}
        {showFilters && (
          <div className="flex items-center gap-2 overflow-x-auto px-4 pt-3 md:px-6">
            <Badge
              variant={draftProjectId == null ? "default" : "secondary"}
              className="cursor-pointer whitespace-nowrap text-xs"
              onClick={() => { setDraftProjectId(null); setSelectedProjectId(null); }}
            >
              {t("mobileHome.allProjects", { defaultValue: "All projects" })}
            </Badge>
            {projects.map((p) => (
              <Badge
                key={p.id}
                variant={draftProjectId === p.id ? "default" : "secondary"}
                className="cursor-pointer whitespace-nowrap text-xs"
                onClick={() => { setDraftProjectId(p.id); setSelectedProjectId(p.id); }}
              >
                {p.name}
              </Badge>
            ))}
          </div>
        )}

        {/* Error */}
        {error && (
          <div className="px-4 pt-3 md:px-6">
            <p className="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">{error}</p>
          </div>
        )}

        {/* Session list */}
        <div className="px-4 pb-4 pt-4 md:px-6">
          {loadingSessions && sessions.length === 0 ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
            </div>
          ) : dateGroups.length === 0 ? (
            <div className="py-8 text-center text-sm text-muted-foreground">
              {t("chat.noSessions")}
            </div>
          ) : (
            <div className="space-y-4">
              {dateGroups.map((group) => (
                <div key={group.label}>
                  <div className="mb-2 text-xs font-medium text-muted-foreground">
                    {group.label}
                  </div>
                  <div className="space-y-1.5">
                    {group.sessions.map((session) => (
                      <SessionListItem
                        key={session.session_id}
                        session={session}
                        onClick={() => navigate(`/chat?session=${session.session_id}`)}
                      />
                    ))}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Hidden file input */}
      <input
        ref={fileInputRef}
        type="file"
        multiple
        accept="image/*,.txt,.md,.json,.csv,.pdf,.yaml,.yml,.toml,.xml,.html,.css,.js,.ts,.tsx,.jsx,.go,.py,.rs,.java,.c,.cpp,.h,.hpp,.sh,.bat,.ps1,.sql,.log"
        className="hidden"
        onChange={(e) => {
          const files = e.target.files;
          if (files && files.length > 0) {
            setPendingFiles((prev) => [...prev, ...Array.from(files)]);
          }
          e.target.value = "";
        }}
      />
    </div>
  );
}

/** Individual session list item — shows title, status, branch, file changes */
function SessionListItem({
  session,
  onClick,
}: {
  session: SessionRecord;
  onClick: () => void;
}) {
  const { t } = useTranslation();

  // Derive stats from session title/branch (these would come from API in real data)
  const hasGit = Boolean(session.branch);

  return (
    <button
      type="button"
      onClick={onClick}
      className="flex w-full items-start gap-3 rounded-xl border bg-card px-4 py-3 text-left transition-colors hover:bg-accent/50 active:bg-accent"
    >
      {/* Status indicator */}
      <div className="mt-1.5 shrink-0">
        <SessionStatusIcon status={session.status} />
      </div>

      {/* Content */}
      <div className="min-w-0 flex-1">
        <div className="flex items-start justify-between gap-2">
          <span className="line-clamp-1 text-sm font-medium text-foreground">
            {session.title ?? t("chat.newSession")}
          </span>
          {session.message_count > 0 && (
            <span className="shrink-0 text-[11px] tabular-nums text-muted-foreground">
              {session.message_count} {t("chat.turns")}
            </span>
          )}
        </div>

        {/* Meta row: project + branch + time */}
        <div className="mt-1 flex flex-wrap items-center gap-1.5 text-[11px] text-muted-foreground">
          {session.project_name && (
            <span className="inline-flex items-center rounded bg-secondary px-1.5 py-0.5 font-medium">
              {session.project_name}
            </span>
          )}
          {hasGit && (
            <span className="inline-flex items-center gap-0.5 rounded bg-secondary px-1.5 py-0.5 font-mono">
              <GitBranch className="h-3 w-3" />
              {session.branch}
            </span>
          )}
          <span>
            {new Date(session.updated_at).toLocaleTimeString("zh-CN", {
              hour: "2-digit",
              minute: "2-digit",
            })}
          </span>
        </div>
      </div>
    </button>
  );
}
