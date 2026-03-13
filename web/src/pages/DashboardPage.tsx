import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import {
  Activity,
  ArrowUpRight,
  CheckCircle2,
  Clock,
  GitBranch,
  Loader2,
  Play,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { StatusBadge } from "@/components/status-badge";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { cn } from "@/lib/utils";
import {
  formatIssueDuration,
  formatRelativeTime,
  getErrorMessage,
  isActiveIssueStatus,
} from "@/lib/v2Workbench";
import type { WorkItem, SchedulerStats, StatsResponse } from "@/types/apiV2";

interface StatCard {
  title: string;
  value: string | number;
  change?: string;
  changeType?: "up" | "down" | "neutral";
  icon: React.ReactNode;
}


export function DashboardPage() {
  const { t } = useTranslation();
  const { apiClient, selectedProject, selectedProjectId, projects } = useWorkbench();
  const [stats, setStats] = useState<StatsResponse | null>(null);
  const [workItems, setWorkItems] = useState<WorkItem[]>([]);
  const [schedulerStats, setSchedulerStats] = useState<SchedulerStats | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        const [statsResp, workItemsResp, schedulerResp] = await Promise.all([
          apiClient.getStats(),
          apiClient.listWorkItems({
            project_id: selectedProjectId ?? undefined,
            archived: false,
            limit: 50,
            offset: 0,
          }),
          apiClient.getSchedulerStats(),
        ]);
        if (cancelled) {
          return;
        }
        setStats(statsResp);
        setWorkItems(workItemsResp);
        setSchedulerStats(schedulerResp);
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
  }, [apiClient, selectedProjectId]);

  const activeWorkItems = useMemo(() => workItems.filter((workItem) => isActiveIssueStatus(workItem.status)), [workItems]);
  const doneWorkItems = useMemo(() => workItems.filter((workItem) => workItem.status === "done"), [workItems]);
  const queuedWorkItems = useMemo(
    () => workItems.filter((workItem) => workItem.status === "queued" || workItem.status === "running").slice(0, 4),
    [workItems],
  );

  const statsCards: StatCard[] = useMemo(() => {
    const successRate = typeof stats?.success_rate === "number" ? `${Math.round(stats.success_rate * 100)}%` : "--";
    return [
      {
        title: t("dashboard.activeFlows"),
        value: activeWorkItems.length,
        change: selectedProject ? t("dashboard.projectScope", { name: selectedProject.name }) : t("dashboard.projectCount", { count: projects.length }),
        changeType: "neutral",
        icon: <Activity className="h-4 w-4 text-muted-foreground" />,
      },
      {
        title: t("dashboard.doneFlows"),
        value: doneWorkItems.length,
        change: stats ? t("dashboard.totalFlows", { count: stats.total_work_items }) : t("dashboard.waitingStats"),
        changeType: "neutral",
        icon: <CheckCircle2 className="h-4 w-4 text-muted-foreground" />,
      },
      {
        title: t("dashboard.successRate"),
        value: successRate,
        change: stats ? t("dashboard.avgDuration", { duration: stats.avg_duration }) : t("dashboard.waitingStats"),
        changeType: "up",
        icon: <GitBranch className="h-4 w-4 text-muted-foreground" />,
      },
      {
        title: t("dashboard.queuedTasks"),
        value: workItems.filter((workItem) => workItem.status === "queued").length,
        change: schedulerStats?.enabled ? t("dashboard.schedulerEnabled") : schedulerStats?.message ?? t("dashboard.schedulerDisabled"),
        changeType: "neutral",
        icon: <Clock className="h-4 w-4 text-muted-foreground" />,
      },
    ];
  }, [activeWorkItems.length, doneWorkItems.length, workItems, projects.length, schedulerStats, selectedProject, stats]);

  if (!selectedProjectId && projects.length === 0) {
    return (
      <div className="flex-1 space-y-6 p-8">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">{t("dashboard.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("dashboard.noProjectHint")}</p>
        </div>
        <Card className="border-dashed">
          <CardHeader>
            <CardTitle>{t("dashboard.noWorkspace")}</CardTitle>
          </CardHeader>
          <CardContent className="flex items-center gap-3">
            <Link to="/projects/new">
              <Button>
                <Play className="mr-2 h-4 w-4" />
                {t("dashboard.createFirst")}
              </Button>
            </Link>
            <p className="text-sm text-muted-foreground">{t("dashboard.createHint")}</p>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="flex-1 space-y-6 p-8">
      <div className="flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-bold tracking-tight">{t("dashboard.title")}</h1>
            {loading ? <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /> : null}
          </div>
          <p className="text-sm text-muted-foreground">
            {selectedProject ? t("dashboard.currentProject", { name: selectedProject.name }) : t("dashboard.crossProjectOverview")}
            {stats ? t("dashboard.totalFlowsSuffix", { count: stats.total_work_items }) : ""}
          </p>
        </div>
        <Link to="/work-items/new">
          <Button>
            <Play className="mr-2 h-4 w-4" />
            {t("dashboard.newFlow")}
          </Button>
        </Link>
      </div>

      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        {statsCards.map((stat) => (
          <Card key={stat.title}>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">{stat.title}</CardTitle>
              {stat.icon}
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{stat.value}</div>
              {stat.change ? (
                <p
                  className={cn(
                    "text-xs",
                    stat.changeType === "up" ? "text-emerald-600" : "text-muted-foreground",
                  )}
                >
                  {stat.change}
                </p>
              ) : null}
            </CardContent>
          </Card>
        ))}
      </div>

      {error ? <p className="rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p> : null}

      <div className="grid gap-6 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader className="flex flex-row items-center justify-between">
            <CardTitle>{t("dashboard.runningFlows")}</CardTitle>
            <Link to="/work-items" className="text-sm text-muted-foreground hover:text-foreground">
              {t("dashboard.viewAll")} <ArrowUpRight className="ml-1 inline h-3 w-3" />
            </Link>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("dashboard.flowName")}</TableHead>
                  <TableHead>{t("common.status")}</TableHead>
                  <TableHead>{t("dashboard.createdAt")}</TableHead>
                  <TableHead>{t("dashboard.duration")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {activeWorkItems.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={4} className="text-center text-muted-foreground">
                      {t("dashboard.noRunningFlows")}
                    </TableCell>
                  </TableRow>
                ) : (
                  activeWorkItems.slice(0, 6).map((workItem) => (
                    <TableRow key={workItem.id}>
                      <TableCell className="font-medium">
                        <Link to={`/work-items/${workItem.id}`} className="hover:underline">
                          {workItem.title}
                        </Link>
                      </TableCell>
                      <TableCell>
                        <StatusBadge status={workItem.status} />
                      </TableCell>
                      <TableCell className="text-muted-foreground">{formatRelativeTime(workItem.created_at)}</TableCell>
                      <TableCell className="text-muted-foreground">{formatIssueDuration(workItem)}</TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>

        <div className="space-y-6">
          <Card className="overflow-hidden p-0">
            <div className="flex items-center justify-between border-b px-5 py-4">
              <h3 className="text-base font-semibold">{t("dashboard.scheduler")}</h3>
              <Badge variant={schedulerStats?.enabled ? "success" : "secondary"}>
                {schedulerStats?.enabled ? t("common.enabled") : t("common.disabled")}
              </Badge>
            </div>

            <div className="space-y-4 p-5">
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">{t("common.project")}</span>
                <span className="font-semibold">{selectedProject?.name ?? t("dashboard.allProjects")}</span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">{t("dashboard.runningCount")}</span>
                <span className="font-semibold text-blue-500">{activeWorkItems.length}</span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">{t("dashboard.queuedCount")}</span>
                <span className="font-semibold text-amber-500">
                  {workItems.filter((workItem) => workItem.status === "queued").length}
                </span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">{t("dashboard.avgDurationLabel")}</span>
                <span className="font-semibold">{stats?.avg_duration ?? "-"}</span>
              </div>
            </div>

            <div className="border-t" />

            <div className="px-5 py-2">
              <span className="text-[11px] font-medium tracking-wider text-muted-foreground">{t("dashboard.activeQueue")}</span>
            </div>

            <div>
              {queuedWorkItems.length === 0 ? (
                <div className="px-5 py-4 text-sm text-muted-foreground">{t("dashboard.queueEmpty")}</div>
              ) : (
                queuedWorkItems.map((workItem, index) => (
                  <div
                    key={workItem.id}
                    className={cn(
                      "flex items-center gap-2.5 px-5 py-2.5",
                      index < queuedWorkItems.length - 1 && "border-b",
                    )}
                  >
                    <div
                      className={cn(
                        "h-2 w-2 shrink-0 rounded-full",
                        workItem.status === "running" ? "bg-blue-500" : "bg-amber-500",
                      )}
                    />
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-sm font-medium">{workItem.title}</div>
                      <div className="text-[11px] text-muted-foreground">
                        {workItem.status === "running" ? t("dashboard.executing") : t("dashboard.waitingSchedule")} · {formatRelativeTime(workItem.updated_at)}
                      </div>
                    </div>
                  </div>
                ))
              )}
            </div>
          </Card>

        </div>
      </div>
    </div>
  );
}
