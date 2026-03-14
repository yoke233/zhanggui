import { useEffect, useState, useCallback } from "react";
import { useTranslation } from "react-i18next";
import {
  AlertTriangle,
  CheckCircle2,
  Eye,
  Lightbulb,
  Loader2,
  Play,
  RefreshCw,
  Search,
  Shield,
  Sparkles,
  TrendingDown,
  TrendingUp,
  Minus,
  ChevronDown,
  ChevronRight,
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
import type {
  InspectionReport,
} from "@/types/apiV2";

const SEVERITY_COLORS: Record<string, string> = {
  critical: "bg-red-600 text-white",
  high: "bg-red-500 text-white",
  medium: "bg-amber-500 text-white",
  low: "bg-blue-500 text-white",
  info: "bg-slate-400 text-white",
};

const STATUS_COLORS: Record<string, string> = {
  completed: "bg-emerald-500 text-white",
  running: "bg-blue-500 text-white",
  pending: "bg-slate-400 text-white",
  failed: "bg-red-500 text-white",
};

const TREND_ICONS: Record<string, typeof TrendingUp> = {
  improving: TrendingUp,
  degrading: TrendingDown,
  stable: Minus,
  new: Sparkles,
};

function TrendBadge({ trend }: { trend?: string }) {
  if (!trend) return null;
  const Icon = TREND_ICONS[trend] || Minus;
  const colors: Record<string, string> = {
    improving: "text-emerald-600 bg-emerald-50",
    degrading: "text-red-600 bg-red-50",
    stable: "text-slate-600 bg-slate-50",
    new: "text-blue-600 bg-blue-50",
  };
  return (
    <span className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium ${colors[trend] ?? "text-slate-600 bg-slate-50"}`}>
      <Icon className="h-3 w-3" />
      {trend}
    </span>
  );
}

export function InspectionPage() {
  const { t } = useTranslation();
  const { apiClient, selectedProjectId } = useWorkbench();
  const [reports, setReports] = useState<InspectionReport[]>([]);
  const [selectedReport, setSelectedReport] = useState<InspectionReport | null>(null);
  const [loading, setLoading] = useState(false);
  const [triggering, setTriggering] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [expandedFindings, setExpandedFindings] = useState<Set<number>>(new Set());

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await apiClient.listInspections({
        project_id: selectedProjectId ?? undefined,
        limit: 20,
      });
      setReports(data);
      if (data.length > 0 && !selectedReport) {
        const detail = await apiClient.getInspection(data[0].id);
        setSelectedReport(detail);
      }
    } catch (e) {
      setError(getErrorMessage(e));
    } finally {
      setLoading(false);
    }
  }, [apiClient, selectedProjectId, selectedReport]);

  useEffect(() => {
    void load();
  }, [load]);

  const selectReport = async (id: number) => {
    try {
      const detail = await apiClient.getInspection(id);
      setSelectedReport(detail);
    } catch (e) {
      setError(getErrorMessage(e));
    }
  };

  const triggerInspection = async () => {
    setTriggering(true);
    setError(null);
    try {
      const report = await apiClient.triggerInspection({
        project_id: selectedProjectId ?? undefined,
        lookback_hours: 24,
      });
      setSelectedReport(report);
      void load();
    } catch (e) {
      setError(getErrorMessage(e));
    } finally {
      setTriggering(false);
    }
  };

  const toggleFinding = (id: number) => {
    setExpandedFindings((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  const findings = selectedReport?.findings ?? [];
  const insights = selectedReport?.insights ?? [];
  const suggestedSkills = selectedReport?.suggested_skills ?? [];
  const snapshot = selectedReport?.snapshot;

  return (
    <div className="flex flex-col gap-6 p-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Shield className="h-6 w-6 text-primary" />
          <div>
            <h1 className="text-2xl font-bold">{t("inspection.title", "System Inspection")}</h1>
            <p className="text-sm text-muted-foreground">
              {t("inspection.subtitle", "Self-evolving inspection: identify problems, track trends, crystallize skills")}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={load} disabled={loading}>
            <RefreshCw className={`mr-1 h-4 w-4 ${loading ? "animate-spin" : ""}`} />
            {t("common.refresh")}
          </Button>
          <Button size="sm" onClick={triggerInspection} disabled={triggering}>
            {triggering ? (
              <Loader2 className="mr-1 h-4 w-4 animate-spin" />
            ) : (
              <Play className="mr-1 h-4 w-4" />
            )}
            {t("inspection.trigger", "Run Inspection")}
          </Button>
        </div>
      </div>

      {error && (
        <div className="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700">
          <AlertTriangle className="mr-1 inline h-4 w-4" />
          {error}
        </div>
      )}

      <div className="grid gap-6 lg:grid-cols-[280px_1fr]">
        {/* Report List */}
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium">
              {t("inspection.reports", "Inspection Reports")}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-1 p-2">
            {reports.length === 0 && !loading && (
              <p className="px-2 py-4 text-center text-sm text-muted-foreground">
                {t("inspection.noReports", "No inspections yet. Click 'Run Inspection' to start.")}
              </p>
            )}
            {reports.map((r) => (
              <button
                key={r.id}
                onClick={() => selectReport(r.id)}
                className={`flex w-full items-center gap-2 rounded-md px-3 py-2 text-left text-sm transition-colors hover:bg-accent ${
                  selectedReport?.id === r.id ? "bg-accent" : ""
                }`}
              >
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <Badge className={`text-[10px] ${STATUS_COLORS[r.status] ?? "bg-slate-400 text-white"}`}>
                      {r.status}
                    </Badge>
                    <Badge variant="outline" className="text-[10px]">
                      {r.trigger}
                    </Badge>
                  </div>
                  <div className="mt-1 truncate text-xs text-muted-foreground">
                    {formatRelativeTime(r.created_at)}
                  </div>
                </div>
                {r.status === "completed" && (
                  <span className="text-xs font-medium text-muted-foreground">
                    {(r.findings?.length ?? 0)}
                  </span>
                )}
              </button>
            ))}
          </CardContent>
        </Card>

        {/* Report Detail */}
        <div className="space-y-6">
          {!selectedReport && !loading && (
            <Card>
              <CardContent className="flex flex-col items-center justify-center py-12 text-center">
                <Search className="mb-3 h-12 w-12 text-muted-foreground/30" />
                <p className="text-sm text-muted-foreground">
                  {t("inspection.selectReport", "Select an inspection report or run a new one")}
                </p>
              </CardContent>
            </Card>
          )}

          {selectedReport && (
            <>
              {/* Summary */}
              {selectedReport.summary && (
                <Card>
                  <CardHeader className="pb-2">
                    <CardTitle className="text-sm font-medium flex items-center gap-2">
                      <Eye className="h-4 w-4" />
                      {t("inspection.summary", "Summary")}
                    </CardTitle>
                  </CardHeader>
                  <CardContent>
                    <p className="text-sm leading-relaxed">{selectedReport.summary}</p>
                  </CardContent>
                </Card>
              )}

              {/* Snapshot */}
              {snapshot && (
                <div className="grid gap-3 sm:grid-cols-2 md:grid-cols-4">
                  <Card>
                    <CardContent className="pt-4">
                      <div className="text-2xl font-bold">{snapshot.total_work_items}</div>
                      <div className="text-xs text-muted-foreground">{t("inspection.totalWorkItems", "Total Work Items")}</div>
                    </CardContent>
                  </Card>
                  <Card>
                    <CardContent className="pt-4">
                      <div className="text-2xl font-bold">{Math.round(snapshot.success_rate * 100)}%</div>
                      <div className="text-xs text-muted-foreground">{t("inspection.successRate", "Success Rate")}</div>
                    </CardContent>
                  </Card>
                  <Card>
                    <CardContent className="pt-4">
                      <div className="text-2xl font-bold text-red-600">{snapshot.failed_work_items}</div>
                      <div className="text-xs text-muted-foreground">{t("inspection.failedItems", "Failed Items")}</div>
                    </CardContent>
                  </Card>
                  <Card>
                    <CardContent className="pt-4">
                      <div className="text-2xl font-bold text-amber-600">{snapshot.blocked_work_items}</div>
                      <div className="text-xs text-muted-foreground">{t("inspection.blockedItems", "Blocked Items")}</div>
                    </CardContent>
                  </Card>
                </div>
              )}

              {/* Findings */}
              {findings.length > 0 && (
                <Card>
                  <CardHeader className="pb-2">
                    <CardTitle className="text-sm font-medium flex items-center gap-2">
                      <AlertTriangle className="h-4 w-4" />
                      {t("inspection.findings", "Findings")} ({findings.length})
                    </CardTitle>
                  </CardHeader>
                  <CardContent>
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead className="w-8" />
                          <TableHead>{t("inspection.severity", "Severity")}</TableHead>
                          <TableHead>{t("inspection.category", "Category")}</TableHead>
                          <TableHead>{t("inspection.findingTitle", "Finding")}</TableHead>
                          <TableHead>{t("inspection.recurring", "Recurring")}</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {findings.map((f) => (
                          <>
                            <TableRow
                              key={f.id}
                              className="cursor-pointer hover:bg-muted/50"
                              onClick={() => toggleFinding(f.id)}
                            >
                              <TableCell>
                                {expandedFindings.has(f.id) ? (
                                  <ChevronDown className="h-4 w-4" />
                                ) : (
                                  <ChevronRight className="h-4 w-4" />
                                )}
                              </TableCell>
                              <TableCell>
                                <Badge className={SEVERITY_COLORS[f.severity] ?? "bg-slate-400 text-white"}>
                                  {f.severity}
                                </Badge>
                              </TableCell>
                              <TableCell>
                                <Badge variant="outline">{f.category}</Badge>
                              </TableCell>
                              <TableCell className="max-w-[400px] truncate font-medium">
                                {f.title}
                              </TableCell>
                              <TableCell>
                                {f.recurring ? (
                                  <Badge className="bg-orange-500 text-white">
                                    x{f.occurrence_count}
                                  </Badge>
                                ) : (
                                  <span className="text-muted-foreground">-</span>
                                )}
                              </TableCell>
                            </TableRow>
                            {expandedFindings.has(f.id) && (
                              <TableRow key={`${f.id}-detail`}>
                                <TableCell colSpan={5} className="bg-muted/30 px-6 py-4">
                                  <div className="space-y-2 text-sm">
                                    <p>{f.description}</p>
                                    {f.evidence && (
                                      <div className="rounded bg-muted px-3 py-2 font-mono text-xs">
                                        {f.evidence}
                                      </div>
                                    )}
                                    {f.recommendation && (
                                      <div className="flex items-start gap-2 text-emerald-700">
                                        <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0" />
                                        <span>{f.recommendation}</span>
                                      </div>
                                    )}
                                  </div>
                                </TableCell>
                              </TableRow>
                            )}
                          </>
                        ))}
                      </TableBody>
                    </Table>
                  </CardContent>
                </Card>
              )}

              {/* Insights */}
              {insights.length > 0 && (
                <Card>
                  <CardHeader className="pb-2">
                    <CardTitle className="text-sm font-medium flex items-center gap-2">
                      <Lightbulb className="h-4 w-4" />
                      {t("inspection.insights", "Evolution Insights")} ({insights.length})
                    </CardTitle>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    {insights.map((insight) => (
                      <div key={insight.id} className="rounded-lg border p-4">
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-2">
                            <Badge variant="outline">{insight.type}</Badge>
                            <span className="font-medium text-sm">{insight.title}</span>
                          </div>
                          <TrendBadge trend={insight.trend} />
                        </div>
                        <p className="mt-2 text-sm text-muted-foreground">{insight.description}</p>
                        {insight.action_items && insight.action_items.length > 0 && (
                          <ul className="mt-2 space-y-1">
                            {insight.action_items.map((item, idx) => (
                              <li key={idx} className="flex items-start gap-2 text-sm">
                                <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 shrink-0 text-emerald-500" />
                                <span>{item}</span>
                              </li>
                            ))}
                          </ul>
                        )}
                      </div>
                    ))}
                  </CardContent>
                </Card>
              )}

              {/* Suggested Skills */}
              {suggestedSkills.length > 0 && (
                <Card>
                  <CardHeader className="pb-2">
                    <CardTitle className="text-sm font-medium flex items-center gap-2">
                      <Sparkles className="h-4 w-4" />
                      {t("inspection.suggestedSkills", "Suggested Skills")} ({suggestedSkills.length})
                    </CardTitle>
                  </CardHeader>
                  <CardContent className="space-y-3">
                    {suggestedSkills.map((skill, idx) => (
                      <div key={idx} className="rounded-lg border p-4">
                        <div className="flex items-center gap-2">
                          <code className="rounded bg-muted px-2 py-0.5 text-sm font-mono">{skill.name}</code>
                        </div>
                        <p className="mt-1 text-sm">{skill.description}</p>
                        <p className="mt-1 text-xs text-muted-foreground">{skill.rationale}</p>
                      </div>
                    ))}
                  </CardContent>
                </Card>
              )}

              {/* Empty state */}
              {findings.length === 0 && insights.length === 0 && selectedReport.status === "completed" && (
                <Card>
                  <CardContent className="flex flex-col items-center justify-center py-12 text-center">
                    <CheckCircle2 className="mb-3 h-12 w-12 text-emerald-500" />
                    <p className="font-medium">{t("inspection.healthy", "System is Healthy")}</p>
                    <p className="text-sm text-muted-foreground">
                      {t("inspection.noFindings", "No issues found during this inspection period.")}
                    </p>
                  </CardContent>
                </Card>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}
