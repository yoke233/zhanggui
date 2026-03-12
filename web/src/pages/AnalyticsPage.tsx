import { useEffect, useMemo, useState, useCallback } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
  AlertTriangle,
  BarChart3,
  CalendarClock,
  Clock,
  Loader2,
  Pause,
  Play,
  Plus,
  RefreshCw,
  TrendingDown,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
  DialogFooter,
} from "@/components/ui/dialog";
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
import type { TFunction } from "i18next";

// --- Cron expression parser & human-readable description ---

interface CronParseResult {
  valid: boolean;
  error?: string;
  description?: string;
}

function parseCronExpr(expr: string, t: TFunction): CronParseResult {
  const parts = expr.trim().split(/\s+/);
  if (parts.length !== 5) {
    return { valid: false, error: t("analytics.cronNeedsFields", { count: parts.length }) };
  }

  const fieldNames = [
    t("analytics.cronFieldMinute"),
    t("analytics.cronFieldHour"),
    t("analytics.cronFieldDay"),
    t("analytics.cronFieldMonth"),
    t("analytics.cronFieldWeekday"),
  ];
  const ranges: [number, number][] = [[0, 59], [0, 23], [1, 31], [1, 12], [0, 6]];

  for (let i = 0; i < 5; i++) {
    const err = validateField(parts[i], ranges[i][0], ranges[i][1], t);
    if (err) return { valid: false, error: `${fieldNames[i]}: ${err}` };
  }

  const desc = describeCron(parts[0], parts[1], parts[2], parts[3], parts[4], t);
  return { valid: true, description: desc };
}

function validateField(field: string, min: number, max: number, t: TFunction): string | null {
  if (field === "*") return null;
  for (const segment of field.split(",")) {
    const [rangePart, stepStr] = segment.split("/");
    if (stepStr !== undefined) {
      const step = Number(stepStr);
      if (!Number.isInteger(step) || step <= 0) return t("analytics.invalidStep", { value: stepStr });
    }
    if (rangePart === "*") continue;
    if (rangePart.includes("-")) {
      const [lo, hi] = rangePart.split("-").map(Number);
      if (!Number.isInteger(lo) || !Number.isInteger(hi)) return t("analytics.invalidRange", { value: rangePart });
      if (lo < min || hi > max || lo > hi) return t("analytics.rangeExceeded", { lo, hi, min, max });
    } else {
      const v = Number(rangePart);
      if (!Number.isInteger(v)) return t("analytics.invalidValue", { value: rangePart });
      if (v < min || v > max) return t("analytics.valueExceeded", { value: v, min, max });
    }
  }
  return null;
}

function getWeekdayNames(t: TFunction): string[] {
  return [
    t("analytics.weekdaySun"),
    t("analytics.weekdayMon"),
    t("analytics.weekdayTue"),
    t("analytics.weekdayWed"),
    t("analytics.weekdayThu"),
    t("analytics.weekdayFri"),
    t("analytics.weekdaySat"),
  ];
}

function describeCron(minute: string, hour: string, day: string, month: string, weekday: string, t: TFunction): string {
  const parts: string[] = [];
  const weekdayNames = getWeekdayNames(t);

  // Month
  if (month !== "*") {
    parts.push(describeList(month, (v) => t("analytics.cronMonthN", { n: v }), t));
  }

  // Day of month
  if (day !== "*") {
    parts.push(describeList(day, (v) => t("analytics.cronDayN", { n: v }), t));
  }

  // Weekday
  if (weekday !== "*") {
    if (weekday === "1-5") {
      parts.push(t("analytics.cronWeekdays"));
    } else if (weekday === "0,6") {
      parts.push(t("analytics.cronWeekends"));
    } else {
      parts.push(describeList(weekday, (v) => t("analytics.cronWeekdayN", { name: weekdayNames[v] ?? v }), t));
    }
  }

  // Hour + minute combined
  if (hour === "*" && minute === "*") {
    parts.push(t("analytics.cronEveryMinute"));
  } else if (hour === "*") {
    if (minute.startsWith("*/")) {
      parts.push(t("analytics.cronEveryNMinutes", { n: minute.slice(2) }));
    } else if (minute === "0") {
      parts.push(t("analytics.cronEveryHourOnTheHour"));
    } else {
      parts.push(t("analytics.cronEveryHourAtMinute", { minute }));
    }
  } else if (minute === "*") {
    parts.push(describeList(hour, (v) => t("analytics.cronHourN", { n: v }), t) + t("analytics.cronEveryMinuteOf"));
  } else {
    // Both specified
    if (hour.startsWith("*/")) {
      const m = minute === "0" ? t("analytics.cronOnTheHour") : t("analytics.cronAtMinute", { m: minute });
      parts.push(t("analytics.cronEveryNHours", { n: hour.slice(2), m }));
    } else {
      const hours = expandList(hour);
      const mins = minute === "0" ? "00" : minute.padStart(2, "0");
      if (hours.length <= 3) {
        parts.push(hours.map((h) => `${String(h).padStart(2, "0")}:${mins}`).join(", "));
      } else {
        parts.push(`${describeList(hour, (v) => t("analytics.cronHourN", { n: v }), t)} ${mins}${t("analytics.cronMinuteSuffix")}`);
      }
    }
  }

  return parts.join(" ") || t("analytics.cronEveryMinute");
}

function describeList(field: string, fmt: (v: number) => string, t: TFunction): string {
  if (field.startsWith("*/")) return t("analytics.cronEveryN", { n: field.slice(2) });
  const values = expandList(field);
  if (values.length <= 5) return values.map(fmt).join(", ");
  return t("analytics.cronRangeSummary", { from: fmt(values[0]), to: fmt(values[values.length - 1]), count: values.length });
}

function expandList(field: string): number[] {
  const result = new Set<number>();
  for (const segment of field.split(",")) {
    const [rangePart, stepStr] = segment.split("/");
    const step = stepStr ? Number(stepStr) : 1;
    if (rangePart.includes("-")) {
      const [lo, hi] = rangePart.split("-").map(Number);
      for (let i = lo; i <= hi; i += step) result.add(i);
    } else {
      result.add(Number(rangePart));
    }
  }
  return [...result].sort((a, b) => a - b);
}

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

export function AnalyticsPage() {
  const { t } = useTranslation();
  const { apiClient, selectedProjectId } = useWorkbench();
  const [data, setData] = useState<AnalyticsSummary | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [rangeDays, setRangeDays] = useState(7);
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [cronIssues, setCronIssues] = useState<CronStatus[]>([]);
  const [cronDialogOpen, setCronDialogOpen] = useState(false);
  const [cronForm, setCronForm] = useState({ issueId: "", schedule: "", maxInstances: "1" });
  const [cronSaving, setCronSaving] = useState(false);
  const [cronError, setCronError] = useState<string | null>(null);

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
      const [resp, cronResp] = await Promise.all([
        apiClient.getAnalyticsSummary({
          project_id: selectedProjectId ?? undefined,
          since:
            rangeDays > 0
              ? new Date(Date.now() - rangeDays * 86400000).toISOString()
              : undefined,
        }),
        apiClient.listCronIssues(),
      ]);
      setData(resp);
      setCronIssues(cronResp);
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
          <p className="text-sm text-muted-foreground">
            {t("analytics.subtitle")}
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

      {/* Status distribution overview */}
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
                  <div className="text-2xl font-bold">
                    {formatDuration(data.duration_stats[0].avg_duration_s)}
                  </div>
                  <p className="text-xs text-muted-foreground">
                    <Link to={`/issues/${data.duration_stats[0].issue_id}`} className="hover:underline">
                      {data.duration_stats[0].issue_title}
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
                  <div className="text-2xl font-bold">
                    {formatDuration(data.bottlenecks[0].avg_duration_s)}
                  </div>
                  <p className="text-xs text-muted-foreground">
                    {data.bottlenecks[0].step_name}
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
                    <TableCell colSpan={5} className="text-center text-muted-foreground">
                      {t("common.noData")}
                    </TableCell>
                  </TableRow>
                ) : (
                  data.project_errors.map((p) => (
                    <TableRow key={p.project_id}>
                      <TableCell className="font-medium">{p.project_name}</TableCell>
                      <TableCell>{p.total_issues}</TableCell>
                      <TableCell className={p.failed_issues > 0 ? "text-red-600 font-medium" : ""}>
                        {p.failed_issues}
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
                    <TableCell colSpan={6} className="text-center text-muted-foreground">
                      {t("common.noData")}
                    </TableCell>
                  </TableRow>
                ) : (
                  data.bottlenecks.slice(0, 10).map((b) => (
                    <TableRow key={b.step_id}>
                      <TableCell className="font-medium">{b.step_name}</TableCell>
                      <TableCell>
                        <Link to={`/issues/${b.issue_id}`} className="text-blue-600 hover:underline">
                          {b.issue_title}
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

      {/* Issue duration stats */}
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
                  <TableCell colSpan={5} className="text-center text-muted-foreground">
                    {t("common.noData")}
                  </TableCell>
                </TableRow>
              ) : (
                data.duration_stats.slice(0, 15).map((d) => (
                  <TableRow key={d.issue_id}>
                    <TableCell className="font-medium">
                      <Link to={`/issues/${d.issue_id}`} className="hover:underline">
                        {d.issue_title}
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
                  <TableCell colSpan={8} className="text-center text-muted-foreground">
                    {t("analytics.noFailures")}
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
                      <Link to={`/issues/${f.issue_id}`} className="text-blue-600 hover:underline">
                        {f.issue_title}
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

      {/* Cron scheduled issues */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle className="flex items-center gap-2">
              <CalendarClock className="h-4 w-4 text-indigo-500" />
              {t("analytics.cronJobs")}
            </CardTitle>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setCronForm({ issueId: "", schedule: "", maxInstances: "1" });
                setCronError(null);
                setCronDialogOpen(true);
              }}
            >
              <Plus className="mr-1.5 h-3.5 w-3.5" />
              {t("analytics.addCronJob")}
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("analytics.flowId")}</TableHead>
                <TableHead>{t("analytics.cronExpression")}</TableHead>
                <TableHead>{t("analytics.status")}</TableHead>
                <TableHead>{t("analytics.maxConcurrent")}</TableHead>
                <TableHead>{t("analytics.lastTriggered")}</TableHead>
                <TableHead>{t("analytics.actions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {cronIssues.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={6} className="text-center text-muted-foreground">
                    {t("analytics.noCronJobs")}
                  </TableCell>
                </TableRow>
              ) : (
                cronIssues.map((c) => (
                  <TableRow key={c.issue_id}>
                    <TableCell>
                      <Link to={`/issues/${c.issue_id}`} className="text-blue-600 hover:underline">
                        #{c.issue_id}
                      </Link>
                    </TableCell>
                    <TableCell>
                      <span className="font-mono text-xs">{c.schedule}</span>
                      {c.schedule ? (
                        <span className="ml-2 text-xs text-muted-foreground">
                          {parseCronExpr(c.schedule, t).description ?? ""}
                        </span>
                      ) : null}
                    </TableCell>
                    <TableCell>
                      <Badge variant={c.enabled ? "success" : "secondary"} className="text-xs">
                        {c.enabled ? t("analytics.cronEnabled") : t("analytics.cronDisabled")}
                      </Badge>
                    </TableCell>
                    <TableCell>{c.max_instances ?? 1}</TableCell>
                    <TableCell className="text-muted-foreground">
                      {c.last_triggered ? formatRelativeTime(c.last_triggered) : t("analytics.neverTriggered")}
                    </TableCell>
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-7 px-2"
                        onClick={async () => {
                          try {
                            if (c.enabled) {
                              await apiClient.disableIssueCron(c.issue_id);
                            } else {
                              await apiClient.setupIssueCron(c.issue_id, {
                                schedule: c.schedule ?? "0 * * * *",
                                max_instances: c.max_instances,
                              });
                            }
                            void load();
                          } catch (e) {
                            setError(getErrorMessage(e));
                          }
                        }}
                      >
                        {c.enabled ? (
                          <>
                            <Pause className="mr-1 h-3 w-3" />
                            {t("analytics.disable")}
                          </>
                        ) : (
                          <>
                            <Play className="mr-1 h-3 w-3" />
                            {t("analytics.enable")}
                          </>
                        )}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {/* Setup Cron Dialog */}
      <Dialog open={cronDialogOpen} onClose={() => setCronDialogOpen(false)}>
        <DialogHeader>
          <DialogTitle>{t("analytics.addCronTitle")}</DialogTitle>
          <DialogDescription>
            {t("analytics.addCronDesc")}
          </DialogDescription>
        </DialogHeader>
        <DialogBody>
          {cronError ? (
            <p className="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">
              {cronError}
            </p>
          ) : null}
          <div className="space-y-4">
            <div className="space-y-1.5">
              <label className="text-sm font-medium">{t("analytics.flowId")}</label>
              <Input
                type="number"
                placeholder={t("analytics.enterFlowId")}
                value={cronForm.issueId}
                onChange={(e) => setCronForm((f) => ({ ...f, issueId: e.target.value }))}
              />
            </div>
            <div className="space-y-1.5">
              <label className="text-sm font-medium">{t("analytics.cronExpression")}</label>
              <Input
                placeholder={t("analytics.cronPlaceholder")}
                value={cronForm.schedule}
                onChange={(e) => setCronForm((f) => ({ ...f, schedule: e.target.value }))}
                className={
                  cronForm.schedule.trim()
                    ? parseCronExpr(cronForm.schedule, t).valid
                      ? "border-emerald-300 focus:border-emerald-400"
                      : "border-rose-300 focus:border-rose-400"
                    : undefined
                }
              />
              {cronForm.schedule.trim() ? (
                (() => {
                  const result = parseCronExpr(cronForm.schedule, t);
                  if (result.valid) {
                    return (
                      <p className="flex items-center gap-1.5 rounded-md bg-emerald-50 px-2.5 py-1.5 text-xs text-emerald-700">
                        <CalendarClock className="h-3 w-3 shrink-0" />
                        {result.description}
                      </p>
                    );
                  }
                  return (
                    <p className="flex items-center gap-1.5 rounded-md bg-rose-50 px-2.5 py-1.5 text-xs text-rose-600">
                      <AlertTriangle className="h-3 w-3 shrink-0" />
                      {result.error}
                    </p>
                  );
                })()
              ) : (
                <p className="text-xs text-muted-foreground">
                  {t("analytics.cronHelp")}
                </p>
              )}
            </div>
            <div className="space-y-1.5">
              <label className="text-sm font-medium">{t("analytics.maxConcurrentInstances")}</label>
              <Input
                type="number"
                min={1}
                max={10}
                value={cronForm.maxInstances}
                onChange={(e) => setCronForm((f) => ({ ...f, maxInstances: e.target.value }))}
              />
            </div>
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => setCronDialogOpen(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={cronSaving || !cronForm.issueId || !cronForm.schedule || !parseCronExpr(cronForm.schedule, t).valid}
            onClick={async () => {
              setCronSaving(true);
              setCronError(null);
              try {
                await apiClient.setupIssueCron(Number(cronForm.issueId), {
                  schedule: cronForm.schedule,
                  max_instances: Number(cronForm.maxInstances) || 1,
                });
                setCronDialogOpen(false);
                void load();
              } catch (e) {
                setCronError(getErrorMessage(e));
              } finally {
                setCronSaving(false);
              }
            }}
          >
            {cronSaving ? <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" /> : null}
            {t("common.confirm")}
          </Button>
        </DialogFooter>
      </Dialog>
    </div>
  );
}
