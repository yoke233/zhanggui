import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useParams } from "react-router-dom";
import {
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
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
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
import type {
  WorkItem,
  WorkItemPriority,
  WorkItemStatus,
  Action,
  ThreadWorkItemLink,
  Thread,
  UpdateWorkItemRequest,
} from "@/types/apiV2";

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

const priorityConfig: Record<WorkItemPriority, { label: string; text: string; bg: string }> = {
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

function StepRow({ step, index, isLast }: { step: Action; index: number; isLast: boolean }) {
  const statusStyle = stepStatusConfig[step.status] ?? stepStatusConfig.pending;
  const typeStyle = stepTypeColors[step.type] ?? stepTypeColors.exec;
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
      <div className={cn("flex h-6 w-6 shrink-0 items-center justify-center rounded-full", statusStyle.bg)}>
        {statusStyle.icon ?? <span className={cn("text-[11px] font-semibold", statusStyle.text)}>{index + 1}</span>}
      </div>
      <div className="min-w-0 flex-1">
        <div className="text-[13px] font-medium text-foreground">{step.name}</div>
        {step.description ? (
          <div className="mt-0.5 truncate text-xs text-muted-foreground">{step.description}</div>
        ) : null}
      </div>
      <div className="flex shrink-0 items-center gap-1.5">
        <span className={cn("rounded px-1.5 py-0.5 text-[11px] font-medium", typeStyle.text, typeStyle.bg)}>
          {normalizeStepTypeLabel(step.type)}
        </span>
        {step.agent_role ? (
          <span className="rounded bg-muted px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground">
            {step.agent_role}
          </span>
        ) : null}
        <span className={cn("rounded px-1.5 py-0.5 text-[11px] font-medium", statusStyle.text, statusStyle.bg)}>
          {statusLabel}
        </span>
      </div>
    </Link>
  );
}

export function WorkItemDetailPage() {
  const { t } = useTranslation();
  const { workItemId: workItemIdParam } = useParams();
  const { apiClient, projects } = useWorkbench();
  const numericWorkItemId = Number.parseInt(workItemIdParam ?? "", 10);
  const [workItem, setWorkItem] = useState<WorkItem | null>(null);
  const [steps, setSteps] = useState<Action[]>([]);
  const [loading, setLoading] = useState(false);
  const [runningAction, setRunningAction] = useState<"idle" | "run" | "cancel">("idle");
  const [error, setError] = useState<string | null>(null);
  const [threadLinks, setThreadLinks] = useState<ThreadWorkItemLink[]>([]);
  const [linkedThreads, setLinkedThreads] = useState<Record<number, Thread>>({});
  const [dependencyWorkItems, setDependencyWorkItems] = useState<Record<number, WorkItem>>({});
  const [editOpen, setEditOpen] = useState(false);
  const [editForm, setEditForm] = useState<UpdateWorkItemRequest>({});
  const [saving, setSaving] = useState(false);

  const fetchWorkItemData = useCallback(async (id: number) => {
    return Promise.all([apiClient.getWorkItem(id), apiClient.listActions(id)]);
  }, [apiClient]);

  useEffect(() => {
    if (!Number.isFinite(numericWorkItemId)) {
      return;
    }
    let cancelled = false;
    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        const [workItemResponse, stepsResponse] = await fetchWorkItemData(numericWorkItemId);
        if (!cancelled) {
          setWorkItem(workItemResponse);
          setSteps(stepsResponse);
        }
      } catch (loadError) {
        if (!cancelled) {
          setError(getErrorMessage(loadError));
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };
    void load();
    return () => {
      cancelled = true;
    };
  }, [fetchWorkItemData, numericWorkItemId]);

  useEffect(() => {
    const dependencyIds = workItem?.depends_on ?? [];
    if (dependencyIds.length === 0) {
      setDependencyWorkItems({});
      return;
    }
    let cancelled = false;
    const loadDependencies = async () => {
      const workItemMap: Record<number, WorkItem> = {};
      const results = await Promise.allSettled(dependencyIds.map((id) => apiClient.getWorkItem(id)));
      results.forEach((result, index) => {
        if (result.status === "fulfilled") {
          workItemMap[dependencyIds[index]] = result.value;
        }
      });
      if (!cancelled) {
        setDependencyWorkItems(workItemMap);
      }
    };
    void loadDependencies();
    return () => {
      cancelled = true;
    };
  }, [apiClient, workItem?.depends_on]);

  useEffect(() => {
    if (!Number.isFinite(numericWorkItemId)) {
      return;
    }
    let cancelled = false;
    const loadThreadLinks = async () => {
      try {
        const listedLinks = await apiClient.listThreadsByWorkItem(numericWorkItemId);
        if (cancelled) {
          return;
        }
        setThreadLinks(listedLinks);
        const threadMap: Record<number, Thread> = {};
        const results = await Promise.allSettled(listedLinks.map((link) => apiClient.getThread(link.thread_id)));
        results.forEach((result, index) => {
          if (result.status === "fulfilled") {
            threadMap[listedLinks[index].thread_id] = result.value;
          }
        });
        if (!cancelled) {
          setLinkedThreads(threadMap);
        }
      } catch {
        if (!cancelled) {
          setThreadLinks([]);
        }
      }
    };
    void loadThreadLinks();
    return () => {
      cancelled = true;
    };
  }, [apiClient, numericWorkItemId]);

  const selectedProject = workItem?.project_id == null
    ? null
    : projects.find((project) => project.id === workItem.project_id) ?? null;

  const statusStyle = statusConfig[workItem?.status ?? "open"] ?? statusConfig.open;
  const priorityStyle = priorityConfig[workItem?.priority ?? "medium"] ?? priorityConfig.medium;

  const openEdit = () => {
    if (!workItem) {
      return;
    }
    setEditForm({
      title: workItem.title,
      body: workItem.body ?? "",
      status: workItem.status as WorkItemStatus,
      priority: workItem.priority,
      labels: workItem.labels ?? [],
    });
    setEditOpen(true);
  };

  const saveEdit = async () => {
    if (!workItem) {
      return;
    }
    setSaving(true);
    setError(null);
    try {
      const updatedWorkItem = await apiClient.updateWorkItem(workItem.id, editForm);
      setWorkItem(updatedWorkItem);
      setEditOpen(false);
    } catch (saveError) {
      setError(getErrorMessage(saveError));
    } finally {
      setSaving(false);
    }
  };

  const runAction = async (action: "run" | "cancel") => {
    if (!workItem) {
      return;
    }
    setRunningAction(action);
    setError(null);
    try {
      if (action === "run") {
        await apiClient.runWorkItem(workItem.id);
      } else {
        await apiClient.cancelWorkItem(workItem.id);
      }
      const refreshedWorkItem = await apiClient.getWorkItem(workItem.id);
      setWorkItem(refreshedWorkItem);
    } catch (actionError) {
      setError(getErrorMessage(actionError));
    } finally {
      setRunningAction("idle");
    }
  };

  return (
    <>
      <EditWorkItemDialog
        open={editOpen}
        form={editForm}
        saving={saving}
        onClose={() => setEditOpen(false)}
        onSave={() => void saveEdit()}
        onChange={(patch) => setEditForm((prev) => ({ ...prev, ...patch }))}
      />
      <div className="flex h-full flex-col overflow-hidden">
        <div className="shrink-0 border-b px-8 pb-5 pt-6">
          <div className="mb-2 flex items-center gap-1.5 text-[13px]">
            <Link to="/work-items" className="text-blue-600 hover:underline">{t("workItemDetail.workItems")}</Link>
            <ChevronRight className="h-3 w-3 text-muted-foreground" />
            {selectedProject ? (
              <>
                <span className="text-blue-600">{selectedProject.name}</span>
                <ChevronRight className="h-3 w-3 text-muted-foreground" />
              </>
            ) : null}
            <span className="font-medium text-foreground">WI-{workItem?.id ?? workItemIdParam}</span>
          </div>
          <div className="flex items-center justify-between gap-4">
            <div className="flex min-w-0 items-center gap-3">
              <h1 className="truncate text-xl font-bold tracking-tight">{workItem?.title ?? `Work Item #${workItemIdParam}`}</h1>
              {workItem ? (
                <span className={cn("inline-flex shrink-0 items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium", statusStyle.text, statusStyle.bg)}>
                  <span className={cn("h-1.5 w-1.5 rounded-full", statusStyle.text.replace("text-", "bg-"))} />
                  {statusStyle.label}
                </span>
              ) : null}
              {loading ? <Loader2 className="h-4 w-4 shrink-0 animate-spin text-muted-foreground" /> : null}
            </div>
            <div className="flex shrink-0 items-center gap-2">
              <Button variant="outline" size="sm" className="gap-1.5" onClick={openEdit}>
                <Pencil className="h-3.5 w-3.5" />
                {t("common.edit")}
              </Button>
              <Button variant="outline" size="sm" className="gap-1.5" disabled={runningAction !== "idle"} onClick={() => void runAction("cancel")}>
                <Square className="h-3.5 w-3.5" />
                {runningAction === "cancel" ? t("workItemDetail.cancelling") : t("common.cancel")}
              </Button>
              <Button size="sm" className="gap-1.5" disabled={runningAction !== "idle"} onClick={() => void runAction("run")}>
                <Play className="h-3.5 w-3.5" />
                {runningAction === "run" ? t("workItemDetail.running") : t("workItemDetail.run")}
              </Button>
            </div>
          </div>
        </div>

        {error ? (
          <p className="mx-8 mt-4 rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p>
        ) : null}

        <div className="flex flex-1 overflow-hidden">
          <div className="flex-1 overflow-y-auto px-8 py-6">
            {workItem?.body ? (
              <div className="mb-6">
                <h3 className="mb-3 text-sm font-semibold">{t("workItemDetail.description")}</h3>
                <div className="whitespace-pre-wrap text-[13px] leading-relaxed text-foreground">{workItem.body}</div>
              </div>
            ) : null}

            <Separator />

            <div className="pt-5">
              <div className="mb-3 flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <h3 className="text-sm font-semibold">{t("workItemDetail.steps")}</h3>
                  <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground">
                    {steps.length} {t("workItemDetail.stepsUnit")}
                  </span>
                </div>
                <Button variant="outline" size="sm" className="h-7 gap-1 text-xs text-muted-foreground">
                  <Plus className="h-3.5 w-3.5" />
                  {t("workItemDetail.addStep")}
                </Button>
              </div>
              {steps.length > 0 ? (
                <div className="rounded-lg border">
                  {steps.map((step, index) => (
                    <StepRow key={step.id} step={step} index={index} isLast={index === steps.length - 1} />
                  ))}
                </div>
              ) : (
                <div className="rounded-lg border px-4 py-8 text-center text-sm text-muted-foreground">
                  {t("workItemDetail.noSteps")}
                </div>
              )}
            </div>
          </div>

          <div className="w-80 shrink-0 overflow-y-auto border-l px-5 py-6">
            <div className="space-y-5">
              <div className="space-y-3">
                <h4 className="text-sm font-semibold">{t("workItemDetail.properties")}</h4>
                <div className="space-y-2.5">
                  <div className="flex items-center justify-between text-[13px]">
                    <span className="text-muted-foreground">{t("common.status")}</span>
                    <span className={cn("rounded-full px-2.5 py-0.5 text-xs font-medium", statusStyle.text, statusStyle.bg)}>{statusStyle.label}</span>
                  </div>
                  <div className="flex items-center justify-between text-[13px]">
                    <span className="text-muted-foreground">{t("workItemDetail.priority")}</span>
                    <span className={cn("rounded-full px-2.5 py-0.5 text-xs font-medium", priorityStyle.text, priorityStyle.bg)}>{priorityStyle.label}</span>
                  </div>
                  <div className="flex items-center justify-between text-[13px]">
                    <span className="text-muted-foreground">{t("common.project")}</span>
                    <span className="font-medium">{selectedProject?.name ?? t("workItemDetail.noProject")}</span>
                  </div>
                  <div className="flex items-center justify-between text-[13px]">
                    <span className="text-muted-foreground">{t("workItemDetail.id")}</span>
                    <span className="font-medium">#{workItem?.id ?? workItemIdParam}</span>
                  </div>
                </div>
              </div>

              <hr className="border-border" />

              <div className="space-y-2">
                <h4 className="text-[13px] text-muted-foreground">{t("workItemDetail.labels")}</h4>
                {(workItem?.labels ?? []).length > 0 ? (
                  <div className="flex flex-wrap gap-1.5">
                    {workItem?.labels?.map((label, index) => {
                      const color = labelColors[index % labelColors.length];
                      return (
                        <span key={label} className={cn("rounded px-2.5 py-0.5 text-[11px] font-medium", color.text, color.bg)}>
                          {label}
                        </span>
                      );
                    })}
                  </div>
                ) : (
                  <span className="text-xs italic text-muted-foreground">{t("workItemDetail.noLabels")}</span>
                )}
              </div>

              <hr className="border-border" />

              <div className="space-y-2.5">
                <h4 className="text-[13px] text-muted-foreground">{t("workItemDetail.linkedThreads")}</h4>
                {threadLinks.length > 0 ? (
                  <div className="space-y-2">
                    {threadLinks.map((link) => {
                      const linkedThread = linkedThreads[link.thread_id];
                      return (
                        <Link
                          key={link.id}
                          to={`/threads/${link.thread_id}`}
                          className="flex items-center gap-2 rounded-md border px-2.5 py-2 text-xs transition-colors hover:bg-muted/50"
                        >
                          <MessageCircle className={cn("h-3.5 w-3.5 shrink-0", link.is_primary ? "text-blue-500" : "text-muted-foreground")} />
                          <div className="min-w-0 flex-1">
                            <div className="truncate font-medium">{linkedThread?.title ?? `Thread #${link.thread_id}`}</div>
                            <div className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
                              <span>{link.relation_type}</span>
                              {link.is_primary ? (
                                <span className="rounded bg-foreground px-1.5 py-px text-[9px] font-medium text-background">primary</span>
                              ) : null}
                            </div>
                          </div>
                        </Link>
                      );
                    })}
                  </div>
                ) : (
                  <span className="text-xs italic text-muted-foreground">{t("workItemDetail.noThreads")}</span>
                )}
              </div>

              <hr className="border-border" />

              <div className="space-y-2">
                <h4 className="text-[13px] text-muted-foreground">{t("workItemDetail.time")}</h4>
                <div className="space-y-1.5 text-xs">
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">{t("workItemDetail.createdAt")}</span>
                    <span>{workItem ? formatRelativeTime(workItem.created_at) : "-"}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">{t("workItemDetail.updatedAt")}</span>
                    <span>{workItem ? formatRelativeTime(workItem.updated_at) : "-"}</span>
                  </div>
                  {workItem ? (
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">{t("workItemDetail.duration")}</span>
                      <span>{formatIssueDuration(workItem)}</span>
                    </div>
                  ) : null}
                </div>
              </div>

              <hr className="border-border" />

              <div className="space-y-2.5">
                <h4 className="text-[13px] text-muted-foreground">{t("workItemDetail.dependencies")}</h4>
                {(workItem?.depends_on ?? []).length > 0 ? (
                  <div className="space-y-2">
                    {workItem?.depends_on?.map((dependencyId) => {
                      const dependency = dependencyWorkItems[dependencyId];
                      const dependencyStatus = statusConfig[dependency?.status ?? "open"] ?? statusConfig.open;
                      return (
                        <Link
                          key={dependencyId}
                          to={`/work-items/${dependencyId}`}
                          className="flex items-center gap-2 rounded-md border px-2.5 py-2 text-xs transition-colors hover:bg-muted/50"
                        >
                          <ArrowUpRight className="h-3.5 w-3.5 shrink-0 text-amber-500" />
                          <div className="min-w-0 flex-1">
                            <div className="truncate font-medium">
                              WI-{dependencyId}{dependency ? ` · ${dependency.title}` : ""}
                            </div>
                            <div className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
                              <span>blocks</span>
                              <span className={cn("rounded px-1.5 py-px text-[9px] font-medium", dependencyStatus.text, dependencyStatus.bg)}>
                                {dependencyStatus.label}
                              </span>
                            </div>
                          </div>
                        </Link>
                      );
                    })}
                  </div>
                ) : (
                  <span className="text-xs italic text-muted-foreground">{t("workItemDetail.noDeps")}</span>
                )}
              </div>
            </div>
          </div>
        </div>
      </div>
    </>
  );
}

const FIELD_CLS = "flex flex-col gap-1.5";
const LABEL_CLS = "text-[11px] font-semibold uppercase tracking-widest text-muted-foreground/70";
const SELECT_WRAP = "relative";
const SELECT_CLS = [
  "w-full appearance-none rounded-lg border border-border/60 bg-muted/30 px-3 py-2",
  "text-sm text-foreground shadow-none transition",
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/40",
  "hover:border-border",
].join(" ");

function EditWorkItemDialog({
  open,
  form,
  saving,
  onClose,
  onSave,
  onChange,
}: {
  open: boolean;
  form: UpdateWorkItemRequest;
  saving: boolean;
  onClose: () => void;
  onSave: () => void;
  onChange: (patch: Partial<UpdateWorkItemRequest>) => void;
}) {
  const { t } = useTranslation();

  return (
    <Dialog open={open} onClose={onClose} className="max-w-md">
      <DialogHeader>
        <DialogTitle>{t("workItemDetail.editTitle")}</DialogTitle>
        <DialogDescription>{t("workItemDetail.editSubtitle")}</DialogDescription>
      </DialogHeader>
      <DialogBody>
        <div className={FIELD_CLS}>
          <label className={LABEL_CLS}>{t("workItemDetail.fieldTitle")}</label>
          <Input
            value={form.title ?? ""}
            onChange={(event) => onChange({ title: event.target.value })}
            placeholder={t("workItemDetail.fieldTitlePlaceholder")}
          />
        </div>

        <div className={FIELD_CLS}>
          <label className={LABEL_CLS}>{t("workItemDetail.fieldBody")}</label>
          <Textarea
            value={form.body ?? ""}
            onChange={(event) => onChange({ body: event.target.value })}
            placeholder={t("workItemDetail.fieldBodyPlaceholder")}
            className="min-h-[80px] resize-none text-sm"
          />
        </div>

        <div className="grid grid-cols-2 gap-3">
          <div className={FIELD_CLS}>
            <label className={LABEL_CLS}>{t("common.status")}</label>
            <div className={SELECT_WRAP}>
              <select
                value={form.status ?? ""}
                onChange={(event) => onChange({ status: event.target.value as WorkItemStatus })}
                className={SELECT_CLS}
              >
                {(["open", "accepted", "running", "blocked", "done", "closed", "cancelled"] as WorkItemStatus[]).map((status) => (
                  <option key={status} value={status}>{statusConfig[status]?.label ?? status}</option>
                ))}
              </select>
              <ChevronRight className="pointer-events-none absolute right-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 rotate-90 text-muted-foreground/60" />
            </div>
          </div>
          <div className={FIELD_CLS}>
            <label className={LABEL_CLS}>{t("workItemDetail.priority")}</label>
            <div className={SELECT_WRAP}>
              <select
                value={form.priority ?? ""}
                onChange={(event) => onChange({ priority: event.target.value as WorkItemPriority })}
                className={SELECT_CLS}
              >
                {(["urgent", "high", "medium", "low"] as WorkItemPriority[]).map((priority) => (
                  <option key={priority} value={priority}>{priorityConfig[priority]?.label ?? priority}</option>
                ))}
              </select>
              <ChevronRight className="pointer-events-none absolute right-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 rotate-90 text-muted-foreground/60" />
            </div>
          </div>
        </div>

        <div className={FIELD_CLS}>
          <label className={LABEL_CLS}>
            {t("workItemDetail.labels")}
            <span className="ml-1.5 normal-case tracking-normal text-muted-foreground/40">· 逗号分隔</span>
          </label>
          <Input
            value={(form.labels ?? []).join(", ")}
            onChange={(event) =>
              onChange({ labels: event.target.value.split(",").map((label) => label.trim()).filter(Boolean) })
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
        <Button onClick={onSave} disabled={saving || !form.title?.trim()} className="min-w-[72px] gap-1.5">
          {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : null}
          {t("common.save")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
}
