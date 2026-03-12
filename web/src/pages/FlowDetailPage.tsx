import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useParams, Link } from "react-router-dom";
import {
  ArrowDownLeft,
  ArrowUpRight,
  Check,
  ChevronRight,
  Clock,
  Loader2,
  MessageCircle,
  Pencil,
  Play,
  Plus,
  Square,
  XCircle,
} from "lucide-react";
import { Separator } from "@/components/ui/separator";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Select } from "@/components/ui/select";
import {
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
  DialogFooter,
} from "@/components/ui/dialog";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { cn } from "@/lib/utils";
import { formatIssueDuration, formatRelativeTime, getErrorMessage, normalizeStepTypeLabel } from "@/lib/v2Workbench";
import type { Execution, Issue, IssuePriority, IssueStatus, Step, ThreadWorkItemLink, Thread, UpdateIssueRequest } from "@/types/apiV2";

/* ── Status config ── */

const statusConfig: Record<string, { label: string; text: string; bg: string }> = {
  open: { label: "待评估", text: "text-blue-600", bg: "bg-blue-50" },
  accepted: { label: "已确认", text: "text-amber-600", bg: "bg-amber-50" },
  queued: { label: "排队中", text: "text-zinc-600", bg: "bg-zinc-100" },
  running: { label: "运行中", text: "text-blue-600", bg: "bg-blue-50" },
  blocked: { label: "阻塞", text: "text-amber-600", bg: "bg-amber-50" },
  failed: { label: "失败", text: "text-red-600", bg: "bg-red-50" },
  done: { label: "已完成", text: "text-emerald-600", bg: "bg-emerald-50" },
  cancelled: { label: "已取消", text: "text-zinc-500", bg: "bg-zinc-100" },
  closed: { label: "已关闭", text: "text-zinc-500", bg: "bg-zinc-100" },
};

const priorityConfig: Record<IssuePriority, { label: string; text: string; bg: string }> = {
  urgent: { label: "紧急", text: "text-red-600", bg: "bg-red-50" },
  high: { label: "高", text: "text-amber-600", bg: "bg-amber-50" },
  medium: { label: "中", text: "text-blue-600", bg: "bg-blue-50" },
  low: { label: "低", text: "text-zinc-500", bg: "bg-zinc-100" },
};

const stepStatusConfig: Record<string, { icon: React.ReactNode; text: string; bg: string }> = {
  done: { icon: <Check className="h-3.5 w-3.5" />, text: "text-emerald-600", bg: "bg-emerald-50" },
  running: { icon: <Loader2 className="h-3.5 w-3.5 animate-spin" />, text: "text-blue-600", bg: "bg-blue-50" },
  failed: { icon: <XCircle className="h-3.5 w-3.5" />, text: "text-red-600", bg: "bg-red-50" },
  waiting_gate: { icon: <Clock className="h-3.5 w-3.5" />, text: "text-amber-600", bg: "bg-amber-50" },
  blocked: { icon: <Clock className="h-3.5 w-3.5" />, text: "text-amber-600", bg: "bg-amber-50" },
  pending: { icon: null, text: "text-zinc-400", bg: "bg-zinc-100" },
  queued: { icon: null, text: "text-zinc-400", bg: "bg-zinc-100" },
  ready: { icon: <Play className="h-3 w-3" />, text: "text-blue-500", bg: "bg-blue-50" },
};

const stepTypeColors: Record<string, { text: string; bg: string }> = {
  exec: { text: "text-blue-600", bg: "bg-blue-50" },
  gate: { text: "text-amber-600", bg: "bg-amber-50" },
  composite: { text: "text-indigo-600", bg: "bg-indigo-50" },
};

const labelColors = [
  { text: "text-blue-600", bg: "bg-blue-50" },
  { text: "text-emerald-600", bg: "bg-emerald-50" },
  { text: "text-amber-600", bg: "bg-amber-50" },
  { text: "text-violet-600", bg: "bg-violet-50" },
  { text: "text-rose-600", bg: "bg-rose-50" },
];

/* ── Step row ── */

function StepRow({ step, index, isLast }: { step: Step; index: number; isLast: boolean }) {
  const { t } = useTranslation();
  const sCfg = stepStatusConfig[step.status] ?? stepStatusConfig.pending;
  const tCfg = stepTypeColors[step.type] ?? stepTypeColors.exec;
  const statusLabel = statusConfig[step.status]?.label ?? step.status;

  return (
    <Link
      to={step.status === "done" || step.status === "running" || step.status === "failed" ? `/executions/${step.id}` : "#"}
      className={cn(
        "flex items-center gap-3 px-3.5 py-3 transition-colors hover:bg-muted/50",
        !isLast && "border-b",
        step.status === "done" && "bg-muted/40",
        step.status === "running" && "bg-muted/30",
      )}
    >
      {/* Number / check icon */}
      <div className={cn("flex h-6 w-6 shrink-0 items-center justify-center rounded-full", sCfg.bg)}>
        {sCfg.icon ?? (
          <span className={cn("text-[11px] font-semibold", sCfg.text)}>{index + 1}</span>
        )}
      </div>
      {/* Name + description */}
      <div className="min-w-0 flex-1">
        <div className="text-[13px] font-medium text-foreground">{step.name}</div>
        {step.description && (
          <div className="mt-0.5 truncate text-xs text-muted-foreground">{step.description}</div>
        )}
      </div>
      {/* Badges */}
      <div className="flex shrink-0 items-center gap-1.5">
        <span className={cn("rounded px-1.5 py-0.5 text-[11px] font-medium", tCfg.text, tCfg.bg)}>
          {normalizeStepTypeLabel(step.type)}
        </span>
        {step.agent_role && (
          <span className="rounded bg-muted px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground">
            {step.agent_role}
          </span>
        )}
        <span className={cn("rounded px-1.5 py-0.5 text-[11px] font-medium", sCfg.text, sCfg.bg)}>
          {statusLabel}
        </span>
      </div>
    </Link>
  );
}

/* ── Main page ── */

export function IssueDetailPage() {
  const { t } = useTranslation();
  const { flowId: issueIdParam } = useParams();
  const { apiClient, projects } = useWorkbench();
  const numericIssueId = Number.parseInt(issueIdParam ?? "", 10);
  const [issue, setIssue] = useState<Issue | null>(null);
  const [steps, setSteps] = useState<Step[]>([]);
  const [loading, setLoading] = useState(false);
  const [runningAction, setRunningAction] = useState<"idle" | "run" | "cancel">("idle");
  const [error, setError] = useState<string | null>(null);
  const [threadLinks, setThreadLinks] = useState<ThreadWorkItemLink[]>([]);
  const [linkedThreads, setLinkedThreads] = useState<Record<number, Thread>>({});
  const [depIssues, setDepIssues] = useState<Record<number, Issue>>({});
  const [editOpen, setEditOpen] = useState(false);
  const [editForm, setEditForm] = useState<UpdateIssueRequest>({});
  const [saving, setSaving] = useState(false);

  /* ── Data loading ── */

  const fetchIssueData = useCallback(async (id: number) => {
    return Promise.all([apiClient.getIssue(id), apiClient.listSteps(id)]);
  }, [apiClient]);

  useEffect(() => {
    if (!Number.isFinite(numericIssueId)) return;
    let cancelled = false;
    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        const [issueResp, stepsResp] = await fetchIssueData(numericIssueId);
        if (!cancelled) {
          setIssue(issueResp);
          setSteps(stepsResp);
        }
      } catch (loadError) {
        if (!cancelled) setError(getErrorMessage(loadError));
      } finally {
        if (!cancelled) setLoading(false);
      }
    };
    void load();
    return () => { cancelled = true; };
  }, [fetchIssueData, numericIssueId]);

  /* ── Load dependency issues ── */
  useEffect(() => {
    const depIds = issue?.depends_on ?? [];
    if (depIds.length === 0) { setDepIssues({}); return; }
    let cancelled = false;
    const load = async () => {
      const map: Record<number, Issue> = {};
      const results = await Promise.allSettled(depIds.map((id) => apiClient.getIssue(id)));
      results.forEach((r, i) => {
        if (r.status === "fulfilled") map[depIds[i]] = r.value;
      });
      if (!cancelled) setDepIssues(map);
    };
    void load();
    return () => { cancelled = true; };
  }, [apiClient, issue?.depends_on]);

  useEffect(() => {
    if (!Number.isFinite(numericIssueId)) return;
    let cancelled = false;
    const loadThreadLinks = async () => {
      try {
        const links = await apiClient.listThreadsByWorkItem(numericIssueId);
        if (cancelled) return;
        setThreadLinks(links);
        const threadMap: Record<number, Thread> = {};
        const results = await Promise.allSettled(
          links.map((l) => apiClient.getThread(l.thread_id)),
        );
        results.forEach((r, i) => {
          if (r.status === "fulfilled") threadMap[links[i].thread_id] = r.value;
        });
        if (!cancelled) setLinkedThreads(threadMap);
      } catch {
        if (!cancelled) setThreadLinks([]);
      }
    };
    void loadThreadLinks();
    return () => { cancelled = true; };
  }, [apiClient, numericIssueId]);

  /* ── Derived ── */

  const selectedProject = issue?.project_id == null
    ? null
    : projects.find((p) => p.id === issue.project_id) ?? null;

  const sCfg = statusConfig[issue?.status ?? "open"] ?? statusConfig.open;
  const pCfg = priorityConfig[issue?.priority ?? "medium"] ?? priorityConfig.medium;

  /* ── Edit ── */

  const openEdit = () => {
    if (!issue) return;
    setEditForm({
      title: issue.title,
      body: issue.body ?? "",
      status: issue.status as IssueStatus,
      priority: issue.priority,
      labels: issue.labels ?? [],
    });
    setEditOpen(true);
  };

  const saveEdit = async () => {
    if (!issue) return;
    setSaving(true);
    setError(null);
    try {
      const updated = await apiClient.updateIssue(issue.id, editForm);
      setIssue(updated);
      setEditOpen(false);
    } catch (saveError) {
      setError(getErrorMessage(saveError));
    } finally {
      setSaving(false);
    }
  };

  /* ── Actions ── */

  const runAction = async (action: "run" | "cancel") => {
    if (!issue) return;
    setRunningAction(action);
    setError(null);
    try {
      if (action === "run") await apiClient.runIssue(issue.id);
      else await apiClient.cancelIssue(issue.id);
      const refreshed = await apiClient.getIssue(issue.id);
      setIssue(refreshed);
    } catch (actionError) {
      setError(getErrorMessage(actionError));
    } finally {
      setRunningAction("idle");
    }
  };

  /* ── Render ── */

  return (
    <>
    <EditIssueDialog
      open={editOpen}
      form={editForm}
      saving={saving}
      projects={projects}
      onClose={() => setEditOpen(false)}
      onSave={() => void saveEdit()}
      onChange={(patch) => setEditForm((prev) => ({ ...prev, ...patch }))}
    />
    <div className="flex h-full flex-col overflow-hidden">
      {/* Header */}
      <div className="shrink-0 border-b px-8 pb-5 pt-6">
        {/* Breadcrumb */}
        <div className="mb-2 flex items-center gap-1.5 text-[13px]">
          <Link to="/work-items" className="text-blue-600 hover:underline">{t("workItemDetail.workItems", "工作项")}</Link>
          <ChevronRight className="h-3 w-3 text-muted-foreground" />
          {selectedProject && (
            <>
              <span className="text-blue-600">{selectedProject.name}</span>
              <ChevronRight className="h-3 w-3 text-muted-foreground" />
            </>
          )}
          <span className="font-medium text-foreground">WI-{issue?.id ?? issueIdParam}</span>
        </div>
        {/* Title + actions */}
        <div className="flex items-center justify-between gap-4">
          <div className="flex min-w-0 items-center gap-3">
            <h1 className="truncate text-xl font-bold tracking-tight">{issue?.title ?? `Work Item #${issueIdParam}`}</h1>
            {issue && (
              <span className={cn("inline-flex shrink-0 items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium", sCfg.text, sCfg.bg)}>
                <span className={cn("h-1.5 w-1.5 rounded-full", sCfg.text.replace("text-", "bg-"))} />
                {sCfg.label}
              </span>
            )}
            {loading && <Loader2 className="h-4 w-4 shrink-0 animate-spin text-muted-foreground" />}
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Button variant="outline" size="sm" className="gap-1.5" onClick={openEdit}>
              <Pencil className="h-3.5 w-3.5" />
              {t("common.edit")}
            </Button>
            <Button variant="outline" size="sm" className="gap-1.5" disabled={runningAction !== "idle"} onClick={() => void runAction("cancel")}>
              <Square className="h-3.5 w-3.5" />
              {runningAction === "cancel" ? t("workItemDetail.cancelling", "取消中...") : t("common.cancel")}
            </Button>
            <Button size="sm" className="gap-1.5" disabled={runningAction !== "idle"} onClick={() => void runAction("run")}>
              <Play className="h-3.5 w-3.5" />
              {runningAction === "run" ? t("workItemDetail.running", "启动中...") : t("workItemDetail.run", "运行")}
            </Button>
          </div>
        </div>
      </div>

      {error && (
        <p className="mx-8 mt-4 rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p>
      )}

      {/* Body: left content + right panel */}
      <div className="flex flex-1 overflow-hidden">
        {/* Left content */}
        <div className="flex-1 overflow-y-auto px-8 py-6">
          {/* Description */}
          {issue?.body && (
            <div className="mb-6">
              <h3 className="mb-3 text-sm font-semibold">{t("workItemDetail.description", "描述")}</h3>
              <div className="whitespace-pre-wrap text-[13px] leading-relaxed text-foreground">{issue.body}</div>
            </div>
          )}

          <Separator />

          {/* Steps */}
          <div className="pt-5">
            <div className="mb-3 flex items-center justify-between">
              <div className="flex items-center gap-2">
                <h3 className="text-sm font-semibold">{t("workItemDetail.steps", "执行步骤")}</h3>
                <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground">
                  {steps.length} {t("workItemDetail.stepsUnit", "步")}
                </span>
              </div>
              <Button variant="outline" size="sm" className="h-7 gap-1 text-xs text-muted-foreground">
                <Plus className="h-3.5 w-3.5" />
                {t("workItemDetail.addStep", "添加步骤")}
              </Button>
            </div>
            {steps.length > 0 ? (
              <div className="rounded-lg border">
                {steps.map((step, i) => (
                  <StepRow key={step.id} step={step} index={i} isLast={i === steps.length - 1} />
                ))}
              </div>
            ) : (
              <div className="rounded-lg border px-4 py-8 text-center text-sm text-muted-foreground">
                {t("workItemDetail.noSteps", "暂无执行步骤")}
              </div>
            )}
          </div>
        </div>

        {/* Right panel */}
        <div className="w-80 shrink-0 overflow-y-auto border-l px-5 py-6">
          <div className="space-y-5">
            {/* Properties */}
            <div className="space-y-3">
              <h4 className="text-sm font-semibold">{t("workItemDetail.properties", "属性")}</h4>
              <div className="space-y-2.5">
                <div className="flex items-center justify-between text-[13px]">
                  <span className="text-muted-foreground">{t("common.status")}</span>
                  <span className={cn("rounded-full px-2.5 py-0.5 text-xs font-medium", sCfg.text, sCfg.bg)}>{sCfg.label}</span>
                </div>
                <div className="flex items-center justify-between text-[13px]">
                  <span className="text-muted-foreground">{t("workItemDetail.priority", "优先级")}</span>
                  <span className={cn("rounded-full px-2.5 py-0.5 text-xs font-medium", pCfg.text, pCfg.bg)}>{pCfg.label}</span>
                </div>
                <div className="flex items-center justify-between text-[13px]">
                  <span className="text-muted-foreground">{t("common.project")}</span>
                  <span className="font-medium">{selectedProject?.name ?? t("workItemDetail.noProject", "未指定")}</span>
                </div>
                <div className="flex items-center justify-between text-[13px]">
                  <span className="text-muted-foreground">{t("workItemDetail.id", "编号")}</span>
                  <span className="font-medium">#{issue?.id ?? issueIdParam}</span>
                </div>
              </div>
            </div>

            <hr className="border-border" />

            {/* Labels */}
            <div className="space-y-2">
              <h4 className="text-[13px] text-muted-foreground">{t("workItemDetail.labels", "标签")}</h4>
              {(issue?.labels ?? []).length > 0 ? (
                <div className="flex flex-wrap gap-1.5">
                  {issue!.labels!.map((label, i) => {
                    const lc = labelColors[i % labelColors.length];
                    return (
                      <span key={label} className={cn("rounded px-2.5 py-0.5 text-[11px] font-medium", lc.text, lc.bg)}>
                        {label}
                      </span>
                    );
                  })}
                </div>
              ) : (
                <span className="text-xs italic text-muted-foreground">{t("workItemDetail.noLabels", "无标签")}</span>
              )}
            </div>

            <hr className="border-border" />

            {/* Linked threads */}
            <div className="space-y-2.5">
              <h4 className="text-[13px] text-muted-foreground">{t("workItemDetail.linkedThreads", "关联讨论")}</h4>
              {threadLinks.length > 0 ? (
                <div className="space-y-2">
                  {threadLinks.map((link) => {
                    const th = linkedThreads[link.thread_id];
                    return (
                      <Link
                        key={link.id}
                        to={`/threads/${link.thread_id}`}
                        className="flex items-center gap-2 rounded-md border px-2.5 py-2 text-xs transition-colors hover:bg-muted/50"
                      >
                        <MessageCircle className={cn("h-3.5 w-3.5 shrink-0", link.is_primary ? "text-blue-500" : "text-muted-foreground")} />
                        <div className="min-w-0 flex-1">
                          <div className="truncate font-medium">{th?.title ?? `Thread #${link.thread_id}`}</div>
                          <div className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
                            <span>{link.relation_type}</span>
                            {link.is_primary && (
                              <span className="rounded bg-foreground px-1.5 py-px text-[9px] font-medium text-background">primary</span>
                            )}
                          </div>
                        </div>
                      </Link>
                    );
                  })}
                </div>
              ) : (
                <span className="text-xs italic text-muted-foreground">{t("workItemDetail.noThreads", "无关联讨论")}</span>
              )}
            </div>

            <hr className="border-border" />

            {/* Time */}
            <div className="space-y-2">
              <h4 className="text-[13px] text-muted-foreground">{t("workItemDetail.time", "时间")}</h4>
              <div className="space-y-1.5 text-xs">
                <div className="flex justify-between">
                  <span className="text-muted-foreground">{t("workItemDetail.createdAt", "创建时间")}</span>
                  <span>{issue ? formatRelativeTime(issue.created_at) : "-"}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">{t("workItemDetail.updatedAt", "更新时间")}</span>
                  <span>{issue ? formatRelativeTime(issue.updated_at) : "-"}</span>
                </div>
                {issue && (
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">{t("workItemDetail.duration", "耗时")}</span>
                    <span>{formatIssueDuration(issue)}</span>
                  </div>
                )}
              </div>
            </div>

            <hr className="border-border" />

            {/* Dependencies */}
            <div className="space-y-2.5">
              <h4 className="text-[13px] text-muted-foreground">{t("workItemDetail.dependencies", "依赖")}</h4>
              {(issue?.depends_on ?? []).length > 0 ? (
                <div className="space-y-2">
                  {issue!.depends_on!.map((depId) => {
                    const dep = depIssues[depId];
                    const depStatus = statusConfig[dep?.status ?? "open"] ?? statusConfig.open;
                    return (
                      <Link
                        key={depId}
                        to={`/work-items/${depId}`}
                        className="flex items-center gap-2 rounded-md border px-2.5 py-2 text-xs transition-colors hover:bg-muted/50"
                      >
                        <ArrowUpRight className="h-3.5 w-3.5 shrink-0 text-amber-500" />
                        <div className="min-w-0 flex-1">
                          <div className="truncate font-medium">
                            WI-{depId}{dep ? ` · ${dep.title}` : ""}
                          </div>
                          <div className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
                            <span>blocks</span>
                            <span className={cn("rounded px-1.5 py-px text-[9px] font-medium", depStatus.text, depStatus.bg)}>
                              {depStatus.label}
                            </span>
                          </div>
                        </div>
                      </Link>
                    );
                  })}
                </div>
              ) : (
                <span className="text-xs italic text-muted-foreground">{t("workItemDetail.noDeps", "无前置依赖")}</span>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
    </>
  );
}

// Keep backward-compatible export
export { IssueDetailPage as FlowDetailPage };

/* ── Edit dialog ── */

const FIELD_CLS = "flex flex-col gap-1.5";
const LABEL_CLS = "text-[11px] font-semibold uppercase tracking-widest text-muted-foreground/70";
const SELECT_WRAP = "relative";
const SELECT_CLS = [
  "w-full appearance-none rounded-lg border border-border/60 bg-muted/30 px-3 py-2",
  "text-sm text-foreground shadow-none transition",
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/40",
  "hover:border-border",
].join(" ");

function EditIssueDialog({
  open,
  form,
  saving,
  projects: _projects,
  onClose,
  onSave,
  onChange,
}: {
  open: boolean;
  form: UpdateIssueRequest;
  saving: boolean;
  projects: { id: number; name: string }[];
  onClose: () => void;
  onSave: () => void;
  onChange: (patch: Partial<UpdateIssueRequest>) => void;
}) {
  const { t } = useTranslation();

  return (
    <Dialog open={open} onClose={onClose} className="max-w-md">
      <DialogHeader>
        <DialogTitle>{t("workItemDetail.editTitle", "编辑工作项")}</DialogTitle>
        <DialogDescription>{t("workItemDetail.editSubtitle", "修改标题、描述、状态和优先级")}</DialogDescription>
      </DialogHeader>
      <DialogBody>
        {/* Title */}
        <div className={FIELD_CLS}>
          <label className={LABEL_CLS}>{t("workItemDetail.fieldTitle", "标题")}</label>
          <Input
            value={form.title ?? ""}
            onChange={(e) => onChange({ title: e.target.value })}
            placeholder={t("workItemDetail.fieldTitlePlaceholder", "工作项标题")}
          />
        </div>

        {/* Description */}
        <div className={FIELD_CLS}>
          <label className={LABEL_CLS}>{t("workItemDetail.fieldBody", "描述")}</label>
          <Textarea
            value={form.body ?? ""}
            onChange={(e) => onChange({ body: e.target.value })}
            placeholder={t("workItemDetail.fieldBodyPlaceholder", "详细描述（可选）")}
            className="min-h-[80px] resize-none text-sm"
          />
        </div>

        {/* Status + Priority */}
        <div className="grid grid-cols-2 gap-3">
          <div className={FIELD_CLS}>
            <label className={LABEL_CLS}>{t("common.status")}</label>
            <div className={SELECT_WRAP}>
              <select
                value={form.status ?? ""}
                onChange={(e) => onChange({ status: e.target.value as IssueStatus })}
                className={SELECT_CLS}
              >
                {(["open","accepted","running","blocked","done","closed","cancelled"] as IssueStatus[]).map((s) => (
                  <option key={s} value={s}>{statusConfig[s]?.label ?? s}</option>
                ))}
              </select>
              <ChevronRight className="pointer-events-none absolute right-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 rotate-90 text-muted-foreground/60" />
            </div>
          </div>
          <div className={FIELD_CLS}>
            <label className={LABEL_CLS}>{t("workItemDetail.priority", "优先级")}</label>
            <div className={SELECT_WRAP}>
              <select
                value={form.priority ?? ""}
                onChange={(e) => onChange({ priority: e.target.value as IssuePriority })}
                className={SELECT_CLS}
              >
                {(["urgent","high","medium","low"] as IssuePriority[]).map((p) => (
                  <option key={p} value={p}>{priorityConfig[p]?.label ?? p}</option>
                ))}
              </select>
              <ChevronRight className="pointer-events-none absolute right-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 rotate-90 text-muted-foreground/60" />
            </div>
          </div>
        </div>

        {/* Labels */}
        <div className={FIELD_CLS}>
          <label className={LABEL_CLS}>
            {t("workItemDetail.labels", "标签")}
            <span className="ml-1.5 normal-case tracking-normal text-muted-foreground/40">· 逗号分隔</span>
          </label>
          <Input
            value={(form.labels ?? []).join(", ")}
            onChange={(e) =>
              onChange({ labels: e.target.value.split(",").map((s) => s.trim()).filter(Boolean) })
            }
            placeholder="bug, frontend, urgent"
            className="font-mono text-xs"
          />
        </div>
      </DialogBody>
      <DialogFooter>
        <Button variant="outline" onClick={onClose} disabled={saving}>
          {t("common.cancel")}
        </Button>
        <Button
          onClick={onSave}
          disabled={saving || !form.title?.trim()}
          className="min-w-[72px] gap-1.5"
        >
          {saving && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
          {t("common.save", "保存")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
}
