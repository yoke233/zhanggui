import { useEffect, useMemo, useState } from "react";
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
import type { Issue, SchedulerStats, StatsResponse } from "@/types/apiV2";
import type { SandboxSupportResponse } from "@/types/system";

interface StatCard {
  title: string;
  value: string | number;
  change?: string;
  changeType?: "up" | "down" | "neutral";
  icon: React.ReactNode;
}

const SANDBOX_PROVIDER_LABELS: Record<string, string> = {
  home_dir: "Home Dir",
  litebox: "LiteBox",
  boxlite: "BoxLite",
  docker: "Docker",
  bwrap: "Bubblewrap",
};

const sandboxBadgeVariant = (
  support?: { supported: boolean; implemented: boolean },
): "success" | "warning" | "secondary" => {
  if (!support) {
    return "secondary";
  }
  if (support.supported && support.implemented) {
    return "success";
  }
  if (support.supported) {
    return "warning";
  }
  return "secondary";
};

const formatSandboxProvider = (provider?: string): string => {
  if (!provider) {
    return "-";
  }
  return SANDBOX_PROVIDER_LABELS[provider] ?? provider;
};

export function DashboardPage() {
  const { apiClient, selectedProject, selectedProjectId, projects } = useWorkbench();
  const [stats, setStats] = useState<StatsResponse | null>(null);
  const [issues, setIssues] = useState<Issue[]>([]);
  const [schedulerStats, setSchedulerStats] = useState<SchedulerStats | null>(null);
  const [sandboxSupport, setSandboxSupport] = useState<SandboxSupportResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        const [statsResp, issuesResp, schedulerResp, sandboxResp] = await Promise.all([
          apiClient.getStats(),
          apiClient.listIssues({
            project_id: selectedProjectId ?? undefined,
            archived: false,
            limit: 50,
            offset: 0,
          }),
          apiClient.getSchedulerStats(),
          apiClient.getSandboxSupport(),
        ]);
        if (cancelled) {
          return;
        }
        setStats(statsResp);
        setIssues(issuesResp);
        setSchedulerStats(schedulerResp);
        setSandboxSupport(sandboxResp);
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

  const activeIssues = useMemo(() => issues.filter((issue) => isActiveIssueStatus(issue.status)), [issues]);
  const doneIssues = useMemo(() => issues.filter((issue) => issue.status === "done"), [issues]);
  const queueIssues = useMemo(
    () => issues.filter((issue) => issue.status === "queued" || issue.status === "running").slice(0, 4),
    [issues],
  );

  const statsCards: StatCard[] = useMemo(() => {
    const successRate = typeof stats?.success_rate === "number" ? `${Math.round(stats.success_rate * 100)}%` : "--";
    return [
      {
        title: "执行中流程",
        value: activeIssues.length,
        change: selectedProject ? `${selectedProject.name} 范围` : `${projects.length} 个项目`,
        changeType: "neutral",
        icon: <Activity className="h-4 w-4 text-muted-foreground" />,
      },
      {
        title: "完成流程",
        value: doneIssues.length,
        change: stats ? `总计 ${stats.total_issues} 个 issue` : "等待统计",
        changeType: "neutral",
        icon: <CheckCircle2 className="h-4 w-4 text-muted-foreground" />,
      },
      {
        title: "成功率",
        value: successRate,
        change: stats ? `平均耗时 ${stats.avg_duration}` : "等待统计",
        changeType: "up",
        icon: <GitBranch className="h-4 w-4 text-muted-foreground" />,
      },
      {
        title: "排队任务",
        value: issues.filter((issue) => issue.status === "queued").length,
        change: schedulerStats?.enabled ? "调度器已启用" : schedulerStats?.message ?? "调度器未启用",
        changeType: "neutral",
        icon: <Clock className="h-4 w-4 text-muted-foreground" />,
      },
    ];
  }, [activeIssues.length, doneIssues.length, issues, projects.length, schedulerStats, selectedProject, stats]);

  if (!selectedProjectId && projects.length === 0) {
    return (
      <div className="flex-1 space-y-6 p-8">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">仪表盘</h1>
          <p className="text-sm text-muted-foreground">当前还没有项目，先创建项目并绑定资源。</p>
        </div>
        <Card className="border-dashed">
          <CardHeader>
            <CardTitle>尚未建立工作区</CardTitle>
          </CardHeader>
          <CardContent className="flex items-center gap-3">
            <Link to="/projects/new">
              <Button>
                <Play className="mr-2 h-4 w-4" />
                创建第一个项目
              </Button>
            </Link>
            <p className="text-sm text-muted-foreground">创建完成后，仪表盘会自动展示真实 Issue 数据。</p>
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
            <h1 className="text-2xl font-bold tracking-tight">仪表盘</h1>
            {loading ? <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /> : null}
          </div>
          <p className="text-sm text-muted-foreground">
            {selectedProject ? `当前项目：${selectedProject.name}` : "跨项目总览"}
            {stats ? ` / 总计 ${stats.total_issues} 个流程` : ""}
          </p>
        </div>
        <Link to="/issues/new">
          <Button>
            <Play className="mr-2 h-4 w-4" />
            新建流程
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
            <CardTitle>运行中流程</CardTitle>
            <Link to="/issues" className="text-sm text-muted-foreground hover:text-foreground">
              查看全部 <ArrowUpRight className="ml-1 inline h-3 w-3" />
            </Link>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>流程名称</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead>创建时间</TableHead>
                  <TableHead>耗时</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {activeIssues.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={4} className="text-center text-muted-foreground">
                      当前没有运行中的流程
                    </TableCell>
                  </TableRow>
                ) : (
                  activeIssues.slice(0, 6).map((issue) => (
                    <TableRow key={issue.id}>
                      <TableCell className="font-medium">
                        <Link to={`/issues/${issue.id}`} className="hover:underline">
                          {issue.title}
                        </Link>
                      </TableCell>
                      <TableCell>
                        <StatusBadge status={issue.status} />
                      </TableCell>
                      <TableCell className="text-muted-foreground">{formatRelativeTime(issue.created_at)}</TableCell>
                      <TableCell className="text-muted-foreground">{formatIssueDuration(issue)}</TableCell>
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
              <h3 className="text-base font-semibold">调度器</h3>
              <Badge variant={schedulerStats?.enabled ? "success" : "secondary"}>
                {schedulerStats?.enabled ? "已启用" : "未启用"}
              </Badge>
            </div>

            <div className="space-y-4 p-5">
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">项目</span>
                <span className="font-semibold">{selectedProject?.name ?? "全部项目"}</span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">运行中</span>
                <span className="font-semibold text-blue-500">{activeIssues.length}</span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">排队中</span>
                <span className="font-semibold text-amber-500">
                  {issues.filter((issue) => issue.status === "queued").length}
                </span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">平均耗时</span>
                <span className="font-semibold">{stats?.avg_duration ?? "-"}</span>
              </div>
            </div>

            <div className="border-t" />

            <div className="px-5 py-2">
              <span className="text-[11px] font-medium tracking-wider text-muted-foreground">活跃队列</span>
            </div>

            <div>
              {queueIssues.length === 0 ? (
                <div className="px-5 py-4 text-sm text-muted-foreground">队列为空</div>
              ) : (
                queueIssues.map((issue, index) => (
                  <div
                    key={issue.id}
                    className={cn(
                      "flex items-center gap-2.5 px-5 py-2.5",
                      index < queueIssues.length - 1 && "border-b",
                    )}
                  >
                    <div
                      className={cn(
                        "h-2 w-2 shrink-0 rounded-full",
                        issue.status === "running" ? "bg-blue-500" : "bg-amber-500",
                      )}
                    />
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-sm font-medium">{issue.title}</div>
                      <div className="text-[11px] text-muted-foreground">
                        {issue.status === "running" ? "正在执行" : "等待调度"} · {formatRelativeTime(issue.updated_at)}
                      </div>
                    </div>
                  </div>
                ))
              )}
            </div>
          </Card>

          <Card>
            <CardHeader className="space-y-3">
              <div className="flex items-center justify-between gap-3">
                <CardTitle>沙盒状态</CardTitle>
                <Badge variant={sandboxSupport?.enabled ? "success" : "secondary"}>
                  {sandboxSupport?.enabled ? "已开启" : "未开启"}
                </Badge>
              </div>
              <div className="flex flex-wrap gap-2">
                <Badge variant={sandboxSupport?.current_supported ? "success" : "secondary"}>
                  当前 Provider: {formatSandboxProvider(sandboxSupport?.current_provider)}
                </Badge>
                <Badge variant={sandboxSupport?.current_supported ? "success" : "warning"}>
                  {sandboxSupport?.current_supported ? "当前可用" : "当前不可用"}
                </Badge>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid gap-2 text-sm">
                <div className="flex items-center justify-between gap-3">
                  <span className="text-muted-foreground">宿主平台</span>
                  <span className="font-medium">
                    {sandboxSupport ? `${sandboxSupport.os} / ${sandboxSupport.arch}` : "-"}
                  </span>
                </div>
                <div className="flex items-center justify-between gap-3">
                  <span className="text-muted-foreground">配置 Provider</span>
                  <span className="font-medium">{formatSandboxProvider(sandboxSupport?.configured_provider)}</span>
                </div>
              </div>

              <div className="space-y-2">
                {sandboxSupport ? Object.entries(sandboxSupport.providers).map(([provider, support]) => (
                  <div key={provider} className="rounded-lg border px-3 py-3">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div className="font-medium">{formatSandboxProvider(provider)}</div>
                      <div className="flex flex-wrap gap-2">
                        <Badge variant={support.supported ? "success" : "secondary"}>
                          {support.supported ? "宿主支持" : "宿主不支持"}
                        </Badge>
                        <Badge variant={sandboxBadgeVariant(support)}>
                          {support.implemented ? "已接入" : "未接入"}
                        </Badge>
                      </div>
                    </div>
                    {support.reason ? (
                      <p className="mt-2 text-xs leading-5 text-muted-foreground">{support.reason}</p>
                    ) : null}
                  </div>
                )) : (
                  <div className="text-sm text-muted-foreground">正在读取沙盒支持矩阵…</div>
                )}
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
