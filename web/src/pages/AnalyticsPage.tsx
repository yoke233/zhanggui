import { useEffect, useMemo, useState, useCallback } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
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
import type { AnalyticsSummary } from "@/types/apiV2";

function formatDuration(seconds: number): string {
  if (seconds < 1) return "<1s";
  if (seconds < 60) return `${Math.round(seconds)}s`;
  const m = Math.floor(seconds / 60);
  const s = Math.round(seconds % 60);
  if (m < 60) return `${m}m ${s}s`;
  const h = Math.floor(m / 60);
  return `${h}h ${m % 60}m`;
}

function PctBar({ pct, color }: { pct: number; color: string }) {
  return (
    <div className="flex items-center gap-2">
      <div className="h-2 w-20 rounded-full bg-slate-100">
        <div className={`h-2 rounded-full ${color}`} style={{ width: `${Math.min(100, Math.round(pct * 100))}%` }} />
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

export function AnalyticsPage() {
  const { t } = useTranslation();
  const { apiClient, selectedProjectId } = useWorkbench();
  const [data, setData] = useState<AnalyticsSummary | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [rangeDays, setRangeDays] = useState(7);
  const [autoRefresh, setAutoRefresh] = useState(true);

  const TIME_RANGES = [
    { label: "24h", value: 1 },
    { label: "7d", value: 7 },
    { label: "30d", value: 30 },
    { label: t("common.all"), value: 0 },
  ] as const;

  const ERROR_KIND_LABELS: Record<string, string> = {
    transient: t("analytics.errorTransient"),
    permanent: t("analytics.errorPermanent"),
    need_help: t("analytics.errorNeedHelp"),
    unknown: t("analytics.errorUnknown"),
  };

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const resp = await apiClient.getAnalyticsSummary({
        project_id: selectedProjectId ?? undefined,
        since: rangeDays > 0 ? new Date(Date.now() - rangeDays * 86400000).toISOString() : undefined,
      });
      setData(resp);
    } catch (e) {
      setError(getErrorMessage(e));
    } finally {
      setLoading(false);
    }
  }, [apiClient, selectedProjectId, rangeDays]);

  useEffect(() => { void load(); }, [load]);

  useEffect(() => {
    if (!autoRefresh) return;
    const id = setInterval(() => void load(), 30000);
    return () => clearInterval(id);
  }, [autoRefresh, load]);

  const totalIssues = useMemo(
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
            <h1 className="text-2xl font-bold tracking-tight">{t("analytics.title")}</h1>
            {loading ? <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /> : null}
          </div>
          <p className="text-sm text-muted-foreground">{t("analytics.subtitle")}</p>
        </div>
        <div className="flex items-center gap-2">
          <div className="flex rounded-md border">
            {TIME_RANGES.map((tr) => (
              <button
                key={tr.value}
                onClick={() => setRangeDays(tr.value)}
                className={`px-3 py-1.5 text-xs font-medium transition-colors ${
                  rangeDays === tr.value ? "bg-primary text-primary-foreground" : "text-muted-foreground hover:bg-accent"
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
            {autoRefresh ? t("common.autoRefresh") : t("common.manual")}
          </Button>
          <Button variant="outline" size="sm" onClick={() => void load()} disabled={loading}>
            {t("common.refresh")}
          </Button>
        </div>
      </div>

      {error ? (
        <p className="rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p>
      ) : null}

      {/* Status overview cards */}
      {data ? (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">{t("analytics.totalFlows")}</CardTitle>
              <BarChart3 className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{totalIssues}</div>
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
              <CardTitle className="text-sm font-medium">{t("analytics.failedExecs")}</CardTitle>
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
              <CardTitle className="text-sm font-medium">{t("analytics.slowestFlow")}</CardTitle>
              <Clock className="h-4 w-4 text-amber-500" />
            </CardHeader>
            <CardContent>
              {data.duration_stats.length > 0 ? (
                <>
                  <div className="text-2xl font-bold">{formatDuration(data.duration_stats[0].avg_duration_s)}</div>
                  <p className="text-xs text-muted-foreground">
                    <Link to={`/work-items/${data.duration_stats[0].work_item_id}`} className="hover:underline">
                      {data.duration_stats[0].work_item_title}
                    </Link>
                    {" / "}{t("analytics.max")} {formatDuration(data.duration_stats[0].max_duration_s)}
                  </p>
                </>
              ) : (
                <p className="text-sm text-muted-foreground">{t("common.noData")}</p>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">{t("analytics.biggestBottleneck")}</CardTitle>
              <TrendingDown className="h-4 w-4 text-orange-500" />
            </CardHeader>
            <CardContent>
              {data.bottlenecks.length > 0 ? (
                <>
                  <div className="text-2xl font-bold">{formatDuration(data.bottlenecks[0].avg_duration_s)}</div>
                  <p className="text-xs text-muted-foreground">
                    {data.bottlenecks[0].action_name}
                    {" / "}{t("analytics.failRate")}{" "}
                    {Math.round(data.bottlenecks[0].fail_rate * 100)}%
                  </p>
                </>
              ) : (
                <p className="text-sm text-muted-foreground">{t("common.noData")}</p>
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
              {t("analytics.projectErrorRanking")}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("common.project")}</TableHead>
                  <TableHead>{t("analytics.flowTotal")}</TableHead>
                  <TableHead>{t("analytics.failedFlows")}</TableHead>
                  <TableHead>{t("analytics.failureRate")}</TableHead>
                  <TableHead>{t("analytics.failedExecs")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {!data || data.project_errors.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={5} className="text-center text-muted-foreground">{t("common.noData")}</TableCell>
                  </TableRow>
                ) : (
                  data.project_errors.map((p) => (
                    <TableRow key={p.project_id}>
                      <TableCell className="font-medium">{p.project_name}</TableCell>
                      <TableCell>{p.total_work_items}</TableCell>
                      <TableCell className={p.failed_work_items > 0 ? "text-red-600 font-medium" : ""}>{p.failed_work_items}</TableCell>
                      <TableCell><PctBar pct={p.failure_rate} color="bg-red-500" /></TableCell>
                      <TableCell>{p.failed_runs}</TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>

        {/* Step bottleneck */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Clock className="h-4 w-4 text-amber-500" />
              {t("analytics.stepBottleneck")}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("analytics.step")}</TableHead>
                  <TableHead>{t("analytics.flow")}</TableHead>
                  <TableHead>{t("analytics.avgDuration")}</TableHead>
                  <TableHead>{t("analytics.maxDuration")}</TableHead>
                  <TableHead>{t("analytics.failureRate")}</TableHead>
                  <TableHead>{t("analytics.retries")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {!data || data.bottlenecks.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={6} className="text-center text-muted-foreground">{t("common.noData")}</TableCell>
                  </TableRow>
                ) : (
                  data.bottlenecks.slice(0, 10).map((b) => (
                    <TableRow key={b.action_id}>
                      <TableCell className="font-medium">{b.action_name}</TableCell>
                      <TableCell>
                        <Link to={`/work-items/${b.work_item_id}`} className="text-blue-600 hover:underline">{b.work_item_title}</Link>
                      </TableCell>
                      <TableCell>{formatDuration(b.avg_duration_s)}</TableCell>
                      <TableCell className="text-muted-foreground">{formatDuration(b.max_duration_s)}</TableCell>
                      <TableCell><PctBar pct={b.fail_rate} color="bg-red-500" /></TableCell>
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
            {t("analytics.flowDurationStats")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("analytics.flow")}</TableHead>
                <TableHead>{t("analytics.execCount")}</TableHead>
                <TableHead>{t("analytics.avgDuration")}</TableHead>
                <TableHead>{t("analytics.minDuration")}</TableHead>
                <TableHead>{t("analytics.maxDuration")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {!data || data.duration_stats.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className="text-center text-muted-foreground">{t("common.noData")}</TableCell>
                </TableRow>
              ) : (
                data.duration_stats.slice(0, 15).map((d) => (
                  <TableRow key={d.work_item_id}>
                    <TableCell className="font-medium">
                      <Link to={`/work-items/${d.work_item_id}`} className="hover:underline">{d.work_item_title}</Link>
                    </TableCell>
                    <TableCell>{d.run_count}</TableCell>
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
            {t("analytics.recentFailures")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("analytics.time")}</TableHead>
                <TableHead>{t("common.project")}</TableHead>
                <TableHead>{t("analytics.flow")}</TableHead>
                <TableHead>{t("analytics.step")}</TableHead>
                <TableHead>{t("analytics.errorType")}</TableHead>
                <TableHead>{t("analytics.attempts")}</TableHead>
                <TableHead>{t("analytics.duration")}</TableHead>
                <TableHead>{t("analytics.errorMessage")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {!data || data.recent_failures.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={8} className="text-center text-muted-foreground">{t("analytics.noFailures")}</TableCell>
                </TableRow>
              ) : (
                data.recent_failures.slice(0, 20).map((f) => (
                  <TableRow key={f.run_id}>
                    <TableCell className="whitespace-nowrap text-muted-foreground">{formatRelativeTime(f.failed_at)}</TableCell>
                    <TableCell>{f.project_name || "-"}</TableCell>
                    <TableCell>
                      <Link to={`/work-items/${f.work_item_id}`} className="text-blue-600 hover:underline">{f.work_item_title}</Link>
                    </TableCell>
                    <TableCell className="font-medium">{f.action_name}</TableCell>
                    <TableCell>
                      <Badge variant={f.error_kind === "permanent" ? "destructive" : "secondary"} className="text-xs">
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
      <Card className="border-slate-200/80 bg-[linear-gradient(180deg,rgba(248,250,252,0.96),rgba(255,255,255,0.94))]">
        <CardHeader>
          <div className="flex items-center justify-between gap-4">
            <div>
              <CardTitle className="flex items-center gap-2">
                <CalendarClock className="h-4 w-4 text-sky-600" />
                {t("scheduledTasks.title")}
              </CardTitle>
              <p className="mt-2 text-sm text-muted-foreground">{t("scheduledTasks.analyticsCardDesc")}</p>
            </div>
            <Link to="/scheduled-tasks">
              <Button variant="outline" size="sm">
                {t("scheduledTasks.openControlCenter")}
              </Button>
            </Link>
          </div>
        </CardHeader>
      </Card>
    </div>
  );
}
