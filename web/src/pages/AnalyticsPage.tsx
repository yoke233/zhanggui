import { useEffect, useMemo, useState, useCallback } from "react";
import { Link } from "react-router-dom";
import {
  AlertTriangle,
  BarChart3,
  CalendarClock,
  Clock,
  Loader2,
  RefreshCw,
  TrendingDown,
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
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { getErrorMessage, formatRelativeTime } from "@/lib/v2Workbench";
import type { AnalyticsSummary, CronStatus } from "@/types/apiV2";

const TIME_RANGES = [
  { label: "24h", value: 1 },
  { label: "7d", value: 7 },
  { label: "30d", value: 30 },
  { label: "全部", value: 0 },
] as const;

function formatDuration(seconds: number): string {
  if (seconds < 1) return "<1s";
  if (seconds < 60) return `${Math.round(seconds)}s`;
  const m = Math.floor(seconds / 60);
  const s = Math.round(seconds % 60);
  if (m < 60) return `${m}m ${s}s`;
  const h = Math.floor(m / 60);
  return `${h}h ${m % 60}m`;
}

function pctBar(pct: number, color: string): React.ReactNode {
  return (
    <div className="flex items-center gap-2">
      <div className="h-2 w-20 rounded-full bg-slate-100">
        <div
          className={`h-2 rounded-full ${color}`}
          style={{ width: `${Math.min(100, Math.round(pct * 100))}%` }}
        />
      </div>
      <span className="text-xs text-muted-foreground">{Math.round(pct * 100)}%</span>
    </div>
  );
}

const STATUS_COLORS: Record<string, string> = {
  done: "bg-emerald-500",
  running: "bg-blue-500",
  failed: "bg-red-500",
  pending: "bg-slate-400",
  queued: "bg-amber-500",
  blocked: "bg-orange-500",
  cancelled: "bg-slate-300",
};

const ERROR_KIND_LABELS: Record<string, string> = {
  transient: "瞬态错误 (可重试)",
  permanent: "永久错误",
  need_help: "需要人工介入",
  unknown: "未分类",
};

export function AnalyticsPage() {
  const { apiClient, selectedProjectId } = useWorkbench();
  const [data, setData] = useState<AnalyticsSummary | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [rangeDays, setRangeDays] = useState(7);
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [cronFlows, setCronFlows] = useState<CronStatus[]>([]);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [resp, cronResp] = await Promise.all([
        apiClient.getAnalyticsSummary({
          project_id: selectedProjectId ?? undefined,
          since:
            rangeDays > 0
              ? new Date(Date.now() - rangeDays * 86400000).toISOString()
              : undefined,
        }),
        apiClient.listCronFlows(),
      ]);
      setData(resp);
      setCronFlows(cronResp);
    } catch (e) {
      setError(getErrorMessage(e));
    } finally {
      setLoading(false);
    }
  }, [apiClient, selectedProjectId, rangeDays]);

  useEffect(() => {
    void load();
  }, [load]);

  // Auto-refresh every 30 seconds.
  useEffect(() => {
    if (!autoRefresh) return;
    const id = setInterval(() => void load(), 30000);
    return () => clearInterval(id);
  }, [autoRefresh, load]);

  const totalFlows = useMemo(
    () => (data?.status_distribution ?? []).reduce((s, d) => s + d.count, 0),
    [data],
  );

  return (
    <div className="flex-1 space-y-6 p-8">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2">
            <BarChart3 className="h-5 w-5 text-muted-foreground" />
            <h1 className="text-2xl font-bold tracking-tight">运行分析</h1>
            {loading ? <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /> : null}
          </div>
          <p className="text-sm text-muted-foreground">
            Flow / Step / Execution 执行质量与瓶颈分析
          </p>
        </div>
        <div className="flex items-center gap-2">
          {/* Time range selector */}
          <div className="flex rounded-md border">
            {TIME_RANGES.map((tr) => (
              <button
                key={tr.value}
                onClick={() => setRangeDays(tr.value)}
                className={`px-3 py-1.5 text-xs font-medium transition-colors ${
                  rangeDays === tr.value
                    ? "bg-primary text-primary-foreground"
                    : "text-muted-foreground hover:bg-accent"
                } ${tr.value === TIME_RANGES[0].value ? "rounded-l-md" : ""} ${
                  tr.value === TIME_RANGES[TIME_RANGES.length - 1].value ? "rounded-r-md" : ""
                }`}
              >
                {tr.label}
              </button>
            ))}
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setAutoRefresh(!autoRefresh)}
            className={autoRefresh ? "border-emerald-300 text-emerald-600" : ""}
          >
            <RefreshCw className={`mr-1.5 h-3.5 w-3.5 ${autoRefresh ? "animate-spin" : ""}`} />
            {autoRefresh ? "自动刷新" : "手动"}
          </Button>
          <Button variant="outline" size="sm" onClick={() => void load()} disabled={loading}>
            刷新
          </Button>
        </div>
      </div>

      {error ? (
        <p className="rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p>
      ) : null}

      {/* Status distribution overview */}
      {data ? (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">总流程数</CardTitle>
              <BarChart3 className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{totalFlows}</div>
              <div className="mt-2 flex flex-wrap gap-1.5">
                {data.status_distribution.map((s) => (
                  <Badge key={s.status} variant="secondary" className="text-xs">
                    <span className={`mr-1 inline-block h-2 w-2 rounded-full ${STATUS_COLORS[s.status] ?? "bg-slate-400"}`} />
                    {s.status} {s.count}
                  </Badge>
                ))}
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">失败执行</CardTitle>
              <AlertTriangle className="h-4 w-4 text-red-500" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold text-red-600">
                {data.error_breakdown.reduce((s, e) => s + e.count, 0)}
              </div>
              <div className="mt-2 space-y-1">
                {data.error_breakdown.map((e) => (
                  <div key={e.error_kind} className="flex items-center justify-between text-xs">
                    <span className="text-muted-foreground">{ERROR_KIND_LABELS[e.error_kind] ?? e.error_kind}</span>
                    <span className="font-medium">{e.count}</span>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">最慢 Flow</CardTitle>
              <Clock className="h-4 w-4 text-amber-500" />
            </CardHeader>
            <CardContent>
              {data.duration_stats.length > 0 ? (
                <>
                  <div className="text-2xl font-bold">
                    {formatDuration(data.duration_stats[0].avg_duration_s)}
                  </div>
                  <p className="text-xs text-muted-foreground">
                    <Link to={`/flows/${data.duration_stats[0].flow_id}`} className="hover:underline">
                      {data.duration_stats[0].flow_name}
                    </Link>
                    {" / "}最大 {formatDuration(data.duration_stats[0].max_duration_s)}
                  </p>
                </>
              ) : (
                <p className="text-sm text-muted-foreground">暂无数据</p>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">最大卡点</CardTitle>
              <TrendingDown className="h-4 w-4 text-orange-500" />
            </CardHeader>
            <CardContent>
              {data.bottlenecks.length > 0 ? (
                <>
                  <div className="text-2xl font-bold">
                    {formatDuration(data.bottlenecks[0].avg_duration_s)}
                  </div>
                  <p className="text-xs text-muted-foreground">
                    {data.bottlenecks[0].step_name}
                    {" / 失败率 "}
                    {Math.round(data.bottlenecks[0].fail_rate * 100)}%
                  </p>
                </>
              ) : (
                <p className="text-sm text-muted-foreground">暂无数据</p>
              )}
            </CardContent>
          </Card>
        </div>
      ) : null}

      <div className="grid gap-6 lg:grid-cols-2">
        {/* Project error ranking */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <AlertTriangle className="h-4 w-4 text-red-500" />
              项目错误排名
            </CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>项目</TableHead>
                  <TableHead>Flow 总数</TableHead>
                  <TableHead>失败 Flow</TableHead>
                  <TableHead>失败率</TableHead>
                  <TableHead>失败执行</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {!data || data.project_errors.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={5} className="text-center text-muted-foreground">
                      暂无数据
                    </TableCell>
                  </TableRow>
                ) : (
                  data.project_errors.map((p) => (
                    <TableRow key={p.project_id}>
                      <TableCell className="font-medium">{p.project_name}</TableCell>
                      <TableCell>{p.total_flows}</TableCell>
                      <TableCell className={p.failed_flows > 0 ? "text-red-600 font-medium" : ""}>
                        {p.failed_flows}
                      </TableCell>
                      <TableCell>{pctBar(p.failure_rate, "bg-red-500")}</TableCell>
                      <TableCell>{p.failed_execs}</TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>

        {/* Step bottleneck analysis */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Clock className="h-4 w-4 text-amber-500" />
              Step 卡点分析
            </CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Step</TableHead>
                  <TableHead>Flow</TableHead>
                  <TableHead>平均耗时</TableHead>
                  <TableHead>最大耗时</TableHead>
                  <TableHead>失败率</TableHead>
                  <TableHead>重试</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {!data || data.bottlenecks.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={6} className="text-center text-muted-foreground">
                      暂无数据
                    </TableCell>
                  </TableRow>
                ) : (
                  data.bottlenecks.slice(0, 10).map((b) => (
                    <TableRow key={b.step_id}>
                      <TableCell className="font-medium">{b.step_name}</TableCell>
                      <TableCell>
                        <Link to={`/flows/${b.flow_id}`} className="text-blue-600 hover:underline">
                          {b.flow_name}
                        </Link>
                      </TableCell>
                      <TableCell>{formatDuration(b.avg_duration_s)}</TableCell>
                      <TableCell className="text-muted-foreground">{formatDuration(b.max_duration_s)}</TableCell>
                      <TableCell>{pctBar(b.fail_rate, "bg-red-500")}</TableCell>
                      <TableCell>
                        {b.retry_count > 0 ? (
                          <Badge variant="secondary" className="text-xs">{b.retry_count}</Badge>
                        ) : (
                          <span className="text-muted-foreground">-</span>
                        )}
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>

      {/* Flow duration stats */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <BarChart3 className="h-4 w-4 text-blue-500" />
            Flow 执行耗时统计
          </CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Flow</TableHead>
                <TableHead>执行次数</TableHead>
                <TableHead>平均耗时</TableHead>
                <TableHead>最短</TableHead>
                <TableHead>最长</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {!data || data.duration_stats.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className="text-center text-muted-foreground">
                    暂无数据
                  </TableCell>
                </TableRow>
              ) : (
                data.duration_stats.slice(0, 15).map((d) => (
                  <TableRow key={d.flow_id}>
                    <TableCell className="font-medium">
                      <Link to={`/flows/${d.flow_id}`} className="hover:underline">
                        {d.flow_name}
                      </Link>
                    </TableCell>
                    <TableCell>{d.exec_count}</TableCell>
                    <TableCell className="font-medium">{formatDuration(d.avg_duration_s)}</TableCell>
                    <TableCell className="text-muted-foreground">{formatDuration(d.min_duration_s)}</TableCell>
                    <TableCell className="text-muted-foreground">{formatDuration(d.max_duration_s)}</TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {/* Recent failures */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <AlertTriangle className="h-4 w-4 text-red-500" />
            最近失败记录
          </CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>时间</TableHead>
                <TableHead>项目</TableHead>
                <TableHead>Flow</TableHead>
                <TableHead>Step</TableHead>
                <TableHead>错误类型</TableHead>
                <TableHead>尝试次数</TableHead>
                <TableHead>耗时</TableHead>
                <TableHead>错误信息</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {!data || data.recent_failures.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={8} className="text-center text-muted-foreground">
                    暂无失败记录
                  </TableCell>
                </TableRow>
              ) : (
                data.recent_failures.slice(0, 20).map((f) => (
                  <TableRow key={f.exec_id}>
                    <TableCell className="whitespace-nowrap text-muted-foreground">
                      {formatRelativeTime(f.failed_at)}
                    </TableCell>
                    <TableCell>{f.project_name || "-"}</TableCell>
                    <TableCell>
                      <Link to={`/flows/${f.flow_id}`} className="text-blue-600 hover:underline">
                        {f.flow_name}
                      </Link>
                    </TableCell>
                    <TableCell className="font-medium">{f.step_name}</TableCell>
                    <TableCell>
                      <Badge
                        variant={f.error_kind === "permanent" ? "destructive" : "secondary"}
                        className="text-xs"
                      >
                        {ERROR_KIND_LABELS[f.error_kind] ?? f.error_kind}
                      </Badge>
                    </TableCell>
                    <TableCell>{f.attempt}</TableCell>
                    <TableCell className="text-muted-foreground">{formatDuration(f.duration_s)}</TableCell>
                    <TableCell className="max-w-xs truncate text-xs text-muted-foreground" title={f.error_message}>
                      {f.error_message || "-"}
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {/* Cron scheduled flows */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <CalendarClock className="h-4 w-4 text-indigo-500" />
            定时任务
          </CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Flow ID</TableHead>
                <TableHead>Cron 表达式</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>最大并发</TableHead>
                <TableHead>上次触发</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {cronFlows.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className="text-center text-muted-foreground">
                    暂无定时任务。可通过 API 对 Flow 设置 cron 触发：POST /api/flows/:id/cron
                  </TableCell>
                </TableRow>
              ) : (
                cronFlows.map((c) => (
                  <TableRow key={c.flow_id}>
                    <TableCell>
                      <Link to={`/flows/${c.flow_id}`} className="text-blue-600 hover:underline">
                        #{c.flow_id}
                      </Link>
                    </TableCell>
                    <TableCell className="font-mono text-xs">{c.schedule}</TableCell>
                    <TableCell>
                      <Badge variant={c.enabled ? "success" : "secondary"} className="text-xs">
                        {c.enabled ? "已启用" : "已停用"}
                      </Badge>
                    </TableCell>
                    <TableCell>{c.max_instances ?? 1}</TableCell>
                    <TableCell className="text-muted-foreground">
                      {c.last_triggered ? formatRelativeTime(c.last_triggered) : "从未触发"}
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
