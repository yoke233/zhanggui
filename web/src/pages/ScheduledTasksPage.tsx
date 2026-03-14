import { useCallback, useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
  CalendarClock,
  Clock3,
  Loader2,
  Pause,
  Play,
  Plus,
  RefreshCw,
  Search,
  Settings2,
  Sparkles,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogBody, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { parseCronExpr } from "@/lib/cronParser";
import { formatRelativeTime, getErrorMessage } from "@/lib/v2Workbench";
import { cn } from "@/lib/utils";
import type { CronStatus, WorkItem } from "@/types/apiV2";

type StatusFilter = "all" | "enabled" | "disabled";

interface CronEditorDialogProps {
  open: boolean;
  mode: "create" | "edit";
  cronItem: CronStatus | null;
  workItems: WorkItem[];
  existingCronIDs: Set<number>;
  onClose: () => void;
  onSave: (workItemId: number, schedule: string, maxInstances: number) => Promise<void>;
}

function MetricCard({
  icon: Icon,
  label,
  value,
  iconClass,
  barClass,
  caption,
}: {
  icon: typeof CalendarClock;
  label: string;
  value: number;
  iconClass: string;
  barClass: string;
  caption: string;
}) {
  return (
    <Card className="overflow-hidden border-slate-200/80 bg-white/90 shadow-[0_18px_50px_rgba(15,23,42,0.06)]">
      <CardContent className="relative p-5">
        <div className={cn("absolute inset-x-0 top-0 h-1", barClass)} />
        <div className="flex items-start justify-between">
          <div>
            <p className="text-[11px] font-semibold uppercase tracking-[0.18em] text-slate-500">{label}</p>
            <div className="mt-3 text-3xl font-semibold tracking-tight text-slate-950">{value}</div>
            <p className="mt-2 text-xs text-slate-500">{caption}</p>
          </div>
          <div className={cn("rounded-2xl border border-current/10 bg-current/5 p-3", iconClass)}>
            <Icon className="h-5 w-5" />
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function CronEditorDialog({
  open,
  mode,
  cronItem,
  workItems,
  existingCronIDs,
  onClose,
  onSave,
}: CronEditorDialogProps) {
  const { t } = useTranslation();
  const [search, setSearch] = useState("");
  const [selectedWorkItemId, setSelectedWorkItemId] = useState("");
  const [schedule, setSchedule] = useState("");
  const [maxInstances, setMaxInstances] = useState("1");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) {
      return;
    }
    setSearch("");
    setError(null);
    setSelectedWorkItemId(cronItem ? String(cronItem.work_item_id) : "");
    setSchedule(cronItem?.schedule ?? "");
    setMaxInstances(String(cronItem?.max_instances ?? 1));
  }, [cronItem, open]);

  const filteredWorkItems = useMemo(() => {
    const keyword = search.trim().toLowerCase();
    return workItems.filter((item) => {
      if (mode === "create" && existingCronIDs.has(item.id)) {
        return false;
      }
      if (!keyword) {
        return true;
      }
      return item.title.toLowerCase().includes(keyword) || String(item.id).includes(keyword);
    });
  }, [existingCronIDs, mode, search, workItems]);

  const cronResult = schedule.trim() ? parseCronExpr(schedule, t) : null;
  const canSave = !saving
    && !!selectedWorkItemId
    && !!schedule.trim()
    && (cronResult?.valid ?? false);

  const handleSave = async () => {
    setSaving(true);
    setError(null);
    try {
      await onSave(Number(selectedWorkItemId), schedule.trim(), Number(maxInstances) || 1);
      onClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onClose={onClose} className="max-w-2xl">
      <DialogHeader className="border-b border-slate-100 bg-[radial-gradient(circle_at_top_left,_rgba(14,165,233,0.16),_transparent_40%),linear-gradient(180deg,rgba(248,250,252,0.9),rgba(255,255,255,0.98))]">
        <div className="flex items-center gap-3">
          <div className="rounded-2xl border border-sky-200 bg-sky-50 p-3 text-sky-700">
            <CalendarClock className="h-5 w-5" />
          </div>
          <div>
            <DialogTitle>
              {mode === "create" ? t("scheduledTasks.addTitle") : t("scheduledTasks.editTitle")}
            </DialogTitle>
            <DialogDescription>{t("scheduledTasks.dialogDesc")}</DialogDescription>
          </div>
        </div>
      </DialogHeader>
      <DialogBody className="space-y-4 pt-5">
        {error ? (
          <p className="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p>
        ) : null}
        <div className="grid gap-4 md:grid-cols-[1.2fr_0.8fr]">
          <div className="space-y-4">
            <div className="space-y-1.5">
              <label className="text-sm font-medium text-slate-700">{t("scheduledTasks.targetWorkItem")}</label>
              {mode === "create" ? (
                <>
                  <div className="relative">
                    <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-400" />
                    <Input
                      value={search}
                      onChange={(event) => setSearch(event.target.value)}
                      placeholder={t("scheduledTasks.searchWorkItems")}
                      className="pl-9"
                    />
                  </div>
                  <Select
                    value={selectedWorkItemId}
                    onChange={(event) => setSelectedWorkItemId(event.target.value)}
                  >
                    <option value="">{t("scheduledTasks.selectWorkItem")}</option>
                    {filteredWorkItems.map((item) => (
                      <option key={item.id} value={item.id}>
                        #{item.id} · {item.title}
                      </option>
                    ))}
                  </Select>
                </>
              ) : (
                <div className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3">
                  <p className="text-sm font-medium text-slate-900">#{cronItem?.work_item_id}</p>
                  <p className="mt-1 text-xs text-slate-500">
                    {workItems.find((item) => item.id === cronItem?.work_item_id)?.title ?? t("scheduledTasks.workItemMissing")}
                  </p>
                </div>
              )}
            </div>

            <div className="space-y-1.5">
              <label className="text-sm font-medium text-slate-700">{t("scheduledTasks.cronExpression")}</label>
              <Input
                placeholder={t("analytics.cronPlaceholder")}
                value={schedule}
                onChange={(event) => setSchedule(event.target.value)}
                className={schedule.trim() ? (cronResult?.valid ? "border-emerald-300" : "border-rose-300") : undefined}
              />
              {cronResult ? (
                cronResult.valid ? (
                  <p className="rounded-xl border border-emerald-200 bg-emerald-50 px-3 py-2 text-xs text-emerald-700">
                    {cronResult.description}
                  </p>
                ) : (
                  <p className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-700">
                    {cronResult.error}
                  </p>
                )
              ) : (
                <p className="text-xs text-slate-500">{t("analytics.cronHelp")}</p>
              )}
            </div>
          </div>

          <div className="space-y-4 rounded-[22px] border border-slate-200 bg-[linear-gradient(180deg,rgba(248,250,252,0.92),rgba(241,245,249,0.7))] p-4">
            <div className="space-y-1.5">
              <label className="text-sm font-medium text-slate-700">{t("scheduledTasks.maxInstances")}</label>
              <Input
                type="number"
                min={1}
                max={10}
                value={maxInstances}
                onChange={(event) => setMaxInstances(event.target.value)}
              />
            </div>
            <div className="rounded-2xl border border-slate-200 bg-white/90 p-4">
              <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-500">
                {t("scheduledTasks.templateHintTitle")}
              </p>
              <p className="mt-2 text-sm leading-6 text-slate-600">{t("scheduledTasks.templateHintBody")}</p>
            </div>
          </div>
        </div>
      </DialogBody>
      <DialogFooter>
        <Button variant="outline" onClick={onClose}>{t("common.cancel")}</Button>
        <Button onClick={() => void handleSave()} disabled={!canSave}>
          {saving ? <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" /> : null}
          {mode === "create" ? t("scheduledTasks.createTask") : t("common.save")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
}

export function ScheduledTasksPage() {
  const { t } = useTranslation();
  const { apiClient, selectedProjectId, selectedProject, projects } = useWorkbench();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [cronItems, setCronItems] = useState<CronStatus[]>([]);
  const [workItems, setWorkItems] = useState<WorkItem[]>([]);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingCron, setEditingCron] = useState<CronStatus | null>(null);

  const projectNameMap = useMemo(() => {
    const map = new Map<number, string>();
    for (const project of projects) {
      map.set(project.id, project.name);
    }
    return map;
  }, [projects]);

  const workItemMap = useMemo(() => {
    const map = new Map<number, WorkItem>();
    for (const item of workItems) {
      map.set(item.id, item);
    }
    return map;
  }, [workItems]);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [cronResp, workItemResp] = await Promise.all([
        apiClient.listCronWorkItems(),
        apiClient.listWorkItems({
          project_id: selectedProjectId ?? undefined,
          archived: false,
          limit: 200,
          offset: 0,
        }),
      ]);
      setCronItems(cronResp);
      setWorkItems(workItemResp);
    } catch (e) {
      setError(getErrorMessage(e));
    } finally {
      setLoading(false);
    }
  }, [apiClient, selectedProjectId]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    if (!autoRefresh) {
      return;
    }
    const id = window.setInterval(() => void load(), 30000);
    return () => window.clearInterval(id);
  }, [autoRefresh, load]);

  const enrichedRows = useMemo(() => {
    return cronItems
      .map((cron) => {
        const item = workItemMap.get(cron.work_item_id);
        return {
          cron,
          workItem: item ?? null,
          projectName: item?.project_id ? projectNameMap.get(item.project_id) ?? null : null,
        };
      })
      .filter((row) => {
        if (selectedProjectId != null && row.workItem?.project_id !== selectedProjectId) {
          return false;
        }
        if (statusFilter === "enabled" && !row.cron.enabled) {
          return false;
        }
        if (statusFilter === "disabled" && row.cron.enabled) {
          return false;
        }
        const keyword = search.trim().toLowerCase();
        if (!keyword) {
          return true;
        }
        return String(row.cron.work_item_id).includes(keyword)
          || (row.workItem?.title.toLowerCase().includes(keyword) ?? false)
          || (row.projectName?.toLowerCase().includes(keyword) ?? false);
      })
      .sort((a, b) => Number(b.cron.enabled) - Number(a.cron.enabled));
  }, [cronItems, projectNameMap, search, selectedProjectId, statusFilter, workItemMap]);

  const summary = useMemo(() => ({
    total: cronItems.length,
    enabled: cronItems.filter((item) => item.enabled).length,
    disabled: cronItems.filter((item) => !item.enabled).length,
  }), [cronItems]);

  const existingCronIDs = useMemo(() => new Set(cronItems.map((item) => item.work_item_id)), [cronItems]);

  const handleToggle = async (cron: CronStatus) => {
    try {
      if (cron.enabled) {
        await apiClient.disableWorkItemCron(cron.work_item_id);
      } else {
        await apiClient.setupWorkItemCron(cron.work_item_id, {
          schedule: cron.schedule ?? "0 * * * *",
          max_instances: cron.max_instances,
        });
      }
      void load();
    } catch (e) {
      setError(getErrorMessage(e));
    }
  };

  const handleSave = async (workItemId: number, schedule: string, maxInstances: number) => {
    await apiClient.setupWorkItemCron(workItemId, {
      schedule,
      max_instances: maxInstances,
    });
    setEditingCron(null);
    await load();
  };

  return (
    <div className="relative flex-1 overflow-hidden bg-[radial-gradient(circle_at_top_left,_rgba(56,189,248,0.12),_transparent_35%),radial-gradient(circle_at_top_right,_rgba(14,165,233,0.08),_transparent_28%),linear-gradient(180deg,#f8fbff_0%,#ffffff_28%,#f8fafc_100%)]">
      <div className="pointer-events-none absolute inset-0 opacity-50 [background-image:linear-gradient(rgba(148,163,184,0.08)_1px,transparent_1px),linear-gradient(90deg,rgba(148,163,184,0.08)_1px,transparent_1px)] [background-size:28px_28px]" />
      <div className="relative flex-1 space-y-6 p-8">
        <section className="overflow-hidden rounded-[30px] border border-slate-200/80 bg-white/80 shadow-[0_25px_80px_rgba(14,165,233,0.08)] backdrop-blur">
          <div className="grid gap-6 p-8 lg:grid-cols-[1.4fr_0.6fr]">
            <div>
              <div className="inline-flex items-center gap-2 rounded-full border border-sky-200 bg-sky-50 px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.2em] text-sky-700">
                <Sparkles className="h-3.5 w-3.5" />
                {t("scheduledTasks.heroBadge")}
              </div>
              <div className="mt-4 flex items-center gap-3">
                <div className="rounded-[24px] border border-slate-200 bg-white p-4 text-slate-950 shadow-sm">
                  <CalendarClock className="h-7 w-7" />
                </div>
                <div>
                  <h1 className="text-3xl font-semibold tracking-tight text-slate-950">
                    {t("scheduledTasks.title")}
                  </h1>
                  <p className="mt-1 max-w-2xl text-sm leading-6 text-slate-600">
                    {t("scheduledTasks.subtitle")}
                  </p>
                </div>
                {loading ? <Loader2 className="h-4 w-4 animate-spin text-slate-500" /> : null}
              </div>
              <div className="mt-6 flex flex-wrap items-center gap-3">
                <Badge variant="outline" className="rounded-full px-3 py-1 text-[10px] tracking-[0.16em]">
                  {selectedProject ? t("scheduledTasks.projectScope", { name: selectedProject.name }) : t("scheduledTasks.allProjects")}
                </Badge>
                <Badge variant="info" className="rounded-full px-3 py-1 text-[10px] tracking-[0.16em]">
                  {t("scheduledTasks.autoRefreshEvery")}
                </Badge>
              </div>
            </div>

            <div className="flex items-end justify-start gap-2 lg:justify-end">
              <Button variant="outline" size="sm" onClick={() => setAutoRefresh(!autoRefresh)}>
                <RefreshCw className={cn("mr-1.5 h-3.5 w-3.5", autoRefresh && "animate-spin")} />
                {autoRefresh ? t("common.autoRefresh") : t("common.manual")}
              </Button>
              <Button variant="outline" size="sm" onClick={() => void load()} disabled={loading}>
                {t("common.refresh")}
              </Button>
              <Button
                size="sm"
                onClick={() => {
                  setEditingCron(null);
                  setDialogOpen(true);
                }}
                className="bg-slate-950 text-white hover:bg-slate-800"
              >
                <Plus className="mr-1.5 h-4 w-4" />
                {t("scheduledTasks.addAction")}
              </Button>
            </div>
          </div>
        </section>

        {error ? (
          <p className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p>
        ) : null}

        <section className="grid gap-4 md:grid-cols-3">
          <MetricCard
            icon={CalendarClock}
            label={t("scheduledTasks.metricTotal")}
            value={summary.total}
            iconClass="text-sky-600"
            barClass="bg-sky-500"
            caption={t("scheduledTasks.metricTotalHint")}
          />
          <MetricCard
            icon={Play}
            label={t("scheduledTasks.metricEnabled")}
            value={summary.enabled}
            iconClass="text-emerald-600"
            barClass="bg-emerald-500"
            caption={t("scheduledTasks.metricEnabledHint")}
          />
          <MetricCard
            icon={Pause}
            label={t("scheduledTasks.metricPaused")}
            value={summary.disabled}
            iconClass="text-amber-600"
            barClass="bg-amber-500"
            caption={t("scheduledTasks.metricPausedHint")}
          />
        </section>

        <Card className="border-slate-200/80 bg-white/90 shadow-[0_20px_60px_rgba(15,23,42,0.06)]">
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2 text-slate-900">
              <Settings2 className="h-4 w-4 text-sky-600" />
              {t("scheduledTasks.controlCenter")}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div className="relative w-full max-w-md">
                <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-400" />
                <Input
                  value={search}
                  onChange={(event) => setSearch(event.target.value)}
                  placeholder={t("scheduledTasks.searchPlaceholder")}
                  className="pl-9"
                />
              </div>
              <div className="flex items-center gap-3">
                <Select
                  value={statusFilter}
                  onChange={(event) => setStatusFilter(event.target.value as StatusFilter)}
                  className="min-w-[180px]"
                >
                  <option value="all">{t("scheduledTasks.filterAll")}</option>
                  <option value="enabled">{t("scheduledTasks.filterEnabled")}</option>
                  <option value="disabled">{t("scheduledTasks.filterDisabled")}</option>
                </Select>
                <Link to="/analytics" className="text-xs font-medium text-slate-500 hover:text-slate-900">
                  {t("scheduledTasks.openAnalytics")}
                </Link>
              </div>
            </div>

            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("scheduledTasks.workItem")}</TableHead>
                  <TableHead>{t("common.project")}</TableHead>
                  <TableHead>{t("scheduledTasks.cronExpression")}</TableHead>
                  <TableHead>{t("scheduledTasks.maxInstances")}</TableHead>
                  <TableHead>{t("scheduledTasks.lastRun")}</TableHead>
                  <TableHead>{t("common.status")}</TableHead>
                  <TableHead>{t("common.operations")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {enrichedRows.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={7} className="py-12 text-center">
                      <div className="mx-auto max-w-md space-y-3">
                        <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-full border border-slate-200 bg-slate-50">
                          <Clock3 className="h-6 w-6 text-slate-400" />
                        </div>
                        <div>
                          <p className="text-sm font-medium text-slate-900">{t("scheduledTasks.emptyTitle")}</p>
                          <p className="mt-1 text-sm text-slate-500">{t("scheduledTasks.emptyDesc")}</p>
                        </div>
                      </div>
                    </TableCell>
                  </TableRow>
                ) : (
                  enrichedRows.map(({ cron, workItem, projectName }) => {
                    const cronSummary = cron.schedule ? parseCronExpr(cron.schedule, t).description : "";
                    return (
                      <TableRow key={cron.work_item_id} className="hover:bg-slate-50/80">
                        <TableCell>
                          <div className="space-y-1">
                            <Link
                              to={`/work-items/${cron.work_item_id}`}
                              className="font-medium text-slate-900 hover:text-sky-700 hover:underline"
                            >
                              {workItem?.title ?? t("scheduledTasks.workItemFallback", { id: cron.work_item_id })}
                            </Link>
                            <p className="text-xs text-slate-500">#{cron.work_item_id}</p>
                          </div>
                        </TableCell>
                        <TableCell className="text-slate-600">{projectName ?? t("scheduledTasks.noProject")}</TableCell>
                        <TableCell>
                          <div className="space-y-1">
                            <code className="rounded bg-slate-100 px-2 py-1 text-[11px] text-slate-800">{cron.schedule ?? "-"}</code>
                            <p className="text-xs text-slate-500">{cronSummary || t("common.noData")}</p>
                          </div>
                        </TableCell>
                        <TableCell>{cron.max_instances ?? 1}</TableCell>
                        <TableCell className="text-slate-500">
                          {cron.last_triggered ? formatRelativeTime(cron.last_triggered) : t("analytics.neverTriggered")}
                        </TableCell>
                        <TableCell>
                          <Badge variant={cron.enabled ? "success" : "warning"}>
                            {cron.enabled ? t("scheduledTasks.statusEnabled") : t("scheduledTasks.statusPaused")}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            <Button variant="outline" size="sm" onClick={() => void handleToggle(cron)}>
                              {cron.enabled ? (
                                <>
                                  <Pause className="mr-1.5 h-3.5 w-3.5" />
                                  {t("analytics.disable")}
                                </>
                              ) : (
                                <>
                                  <Play className="mr-1.5 h-3.5 w-3.5" />
                                  {t("analytics.enable")}
                                </>
                              )}
                            </Button>
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => {
                                setEditingCron(cron);
                                setDialogOpen(true);
                              }}
                            >
                              {t("common.edit")}
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    );
                  })
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>

        <CronEditorDialog
          open={dialogOpen}
          mode={editingCron ? "edit" : "create"}
          cronItem={editingCron}
          workItems={workItems}
          existingCronIDs={existingCronIDs}
          onClose={() => {
            setDialogOpen(false);
            setEditingCron(null);
          }}
          onSave={handleSave}
        />
      </div>
    </div>
  );
}
