import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useParams, Link } from "react-router-dom";
import {
  ChevronRight,
  FileText,
  CheckCircle2,
  AlertTriangle,
  Clock,
  Bot,
  Loader2,
  Terminal,
  Wrench,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { StatusBadge } from "@/components/status-badge";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { formatRelativeTime, getErrorMessage, normalizeStepTypeLabel } from "@/lib/v2Workbench";
import type { Resource, Event, Run, WorkItem, Action } from "@/types/apiV2";

interface LogLine {
  time: string;
  type: "text" | "tool_call" | "tool_result" | "thinking";
  content: string;
}

const eventToLogLine = (event: Event, locale: string): LogLine | null => {
  const time = new Date(event.timestamp).toLocaleTimeString(locale, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
  if (event.type === "exec.agent_output" || event.type === "chat.output") {
    const subtype = String(event.data?.type ?? "");
    const content = String(event.data?.content ?? "");
    if (!content) {
      return null;
    }
    if (subtype === "tool_call") {
      return { time, type: "tool_call", content };
    }
    if (subtype === "tool_call_completed") {
      return { time, type: "tool_result", content };
    }
    if (subtype === "agent_thought") {
      return { time, type: "thinking", content };
    }
    return { time, type: "text", content };
  }
  return {
    time,
    type: "text",
    content: `${event.type}${event.data?.content ? `: ${String(event.data.content)}` : ""}`,
  };
};

export function ExecutionDetailPage() {
  const { execId } = useParams();
  const { t, i18n } = useTranslation();
  const { apiClient } = useWorkbench();
  const numericExecId = Number.parseInt(execId ?? "", 10);
  const logEndRef = useRef<HTMLDivElement>(null);

  const [execution, setExecution] = useState<Run | null>(null);
  const [step, setStep] = useState<Action | null>(null);
  const [workItem, setWorkItem] = useState<WorkItem | null>(null);
  const [briefing, setBriefing] = useState<{ objective: string; constraints?: string[]; context_refs?: unknown[] } | null>(null);
  const [resources, setResources] = useState<Resource[]>([]);
  const [events, setEvents] = useState<Event[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!Number.isFinite(numericExecId)) {
      return;
    }
    let cancelled = false;

    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        const execResp = await apiClient.getRun(numericExecId);
        if (cancelled) {
          return;
        }
        setExecution(execResp);

        const [stepResp, workItemResp, briefingResp, runResources, eventResp] = await Promise.all([
          apiClient.getAction(execResp.action_id),
          apiClient.getWorkItem(execResp.work_item_id),
          apiClient.getAction(execResp.action_id).then((a) => ({
            objective: a.description ?? "",
            constraints: [] as string[],
          })).catch(() => null),
          apiClient.listRunResources(execResp.id),
          apiClient.listEvents({
            issue_id: execResp.work_item_id,
            step_id: execResp.action_id,
            limit: 200,
            offset: 0,
          }),
        ]);
        if (cancelled) {
          return;
        }
        setStep(stepResp);
        setWorkItem(workItemResp);
        setBriefing(briefingResp);
        setResources(runResources);
        setEvents(eventResp);
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
  }, [apiClient, numericExecId]);

  const logs = useMemo(
    () =>
      events
        .map((event) => eventToLogLine(event, i18n.language))
        .filter((line): line is LogLine => line != null),
    [events, i18n.language],
  );

  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [logs]);

  return (
    <div className="flex h-full flex-col">
      <div className="border-b px-8 py-4">
        <div className="mb-2 flex items-center gap-2 text-sm text-muted-foreground">
          <Link to="/work-items" className="hover:text-foreground">{t("execDetail.workItems")}</Link>
          <ChevronRight className="h-3 w-3" />
          {workItem ? <Link to={`/work-items/${workItem.id}`} className="hover:text-foreground">{workItem.title}</Link> : <span>Work Item</span>}
          <ChevronRight className="h-3 w-3" />
          <span className="text-foreground">{step?.name ?? "Execution"}</span>
          <span className="mx-1">·</span>
          <span className="text-foreground">{t("execDetail.title")}</span>
        </div>
        <div className="flex items-center gap-3">
          <h1 className="text-xl font-bold">
            {step?.name ?? t("execDetail.execution")} {execution ? `— ${t("execDetail.attemptN", { n: execution.attempt })}` : ""}
          </h1>
          {execution ? <StatusBadge status={execution.status} /> : null}
          {loading ? <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /> : null}
          <span className="ml-auto text-sm text-muted-foreground">Execution #{execId ?? "-"}</span>
        </div>
      </div>

      {error ? <p className="mx-8 mt-4 rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p> : null}

      <div className="flex flex-1 overflow-hidden">
        <div className="w-[420px] space-y-6 overflow-y-auto border-r p-6">
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="flex items-center gap-2 text-base">
                <FileText className="h-4 w-4" />
                {t("execDetail.taskBriefing")}
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div>
                <h4 className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">{t("execDetail.objective")}</h4>
                <p className="text-sm leading-relaxed">
                  {briefing?.objective || step?.description || t("execDetail.noObjective")}
                </p>
              </div>
              <div>
                <h4 className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">{t("execDetail.constraints")}</h4>
                <ul className="space-y-1.5 text-sm">
                  {(briefing?.constraints ?? []).length === 0 ? (
                    <li className="text-muted-foreground">{t("execDetail.noConstraints")}</li>
                  ) : (
                    briefing?.constraints?.map((constraint, index) => (
                      <li key={`${constraint}-${index}`} className="flex items-start gap-2">
                        <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-amber-500" />
                        {constraint}
                      </li>
                    ))
                  )}
                </ul>
              </div>
              <div>
                <h4 className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">{t("execDetail.acceptanceCriteria")}</h4>
                <ul className="space-y-1.5 text-sm">
                  {(step?.acceptance_criteria ?? []).length === 0 ? (
                    <li className="text-muted-foreground">{t("execDetail.noAcceptanceCriteria")}</li>
                  ) : (
                    step?.acceptance_criteria?.map((criteria, index) => (
                      <li key={`${criteria}-${index}`} className="flex items-start gap-2">
                        <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 shrink-0 text-emerald-500" />
                        {criteria}
                      </li>
                    ))
                  )}
                </ul>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="flex items-center gap-2 text-base">
                <CheckCircle2 className="h-4 w-4" />
                {t("execDetail.submitResult")}
              </CardTitle>
            </CardHeader>
            <CardContent>
              {execution?.result_markdown ? (
                <div className="space-y-3">
                  <div className="rounded-lg border bg-muted/30 p-3 text-sm text-muted-foreground">
                    {execution.result_markdown || t("execDetail.emptyResult")}
                  </div>
                  <div className="text-xs text-muted-foreground">
                    Run #{execution.id} · {formatRelativeTime(execution.created_at)}
                  </div>
                  {resources.length > 0 ? (
                    <div className="rounded-lg border bg-background p-3">
                      <div className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
                        资源文件
                      </div>
                      <ul className="space-y-1 text-sm">
                        {resources.map((resource) => (
                          <li key={resource.id}>
                            {resource.file_name}
                          </li>
                        ))}
                      </ul>
                    </div>
                  ) : null}
                </div>
              ) : (
                <div className="flex items-center gap-3 rounded-lg border border-dashed p-4">
                  <Clock className="h-5 w-5 text-muted-foreground" />
                  <div>
                    <p className="text-sm font-medium">{t("execDetail.noArtifact")}</p>
                    <p className="text-xs text-muted-foreground">{t("execDetail.noArtifactDesc")}</p>
                  </div>
                </div>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="flex items-center gap-2 text-base">
                <Bot className="h-4 w-4" />
                {t("execDetail.execInfo")}
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-2 text-sm">
              <div className="flex justify-between">
                <span className="text-muted-foreground">{t("execDetail.stepType")}</span>
                <span className="font-medium">{step ? normalizeStepTypeLabel(step.type) : "-"}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">{t("execDetail.role")}</span>
                <span className="font-medium">{step?.agent_role || execution?.agent_id || "-"}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">{t("execDetail.attempt")}</span>
                <span className="font-medium">{execution ? t("execDetail.attemptCount", { n: execution.attempt }) : "-"}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">{t("execDetail.startTime")}</span>
                <span className="font-medium">{execution?.started_at ? formatRelativeTime(execution.started_at) : "-"}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">{t("execDetail.endTime")}</span>
                <span className="font-medium">{execution?.finished_at ? formatRelativeTime(execution.finished_at) : "-"}</span>
              </div>
            </CardContent>
          </Card>
        </div>

        <div className="flex flex-1 flex-col bg-zinc-950">
          <div className="flex items-center justify-between border-b border-zinc-800 px-5 py-3">
            <div className="flex items-center gap-2">
              <Terminal className="h-4 w-4 text-zinc-400" />
              <span className="text-sm font-medium text-zinc-300">{t("execDetail.eventStream")}</span>
            </div>
            <Badge variant="outline" className="border-zinc-700 text-xs text-zinc-400">
              {t("execDetail.logCount", { count: logs.length })}
            </Badge>
          </div>

          <div className="flex-1 overflow-y-auto p-5 font-mono text-sm">
            {logs.length === 0 ? (
              <div className="text-zinc-500">{t("execDetail.noLogs")}</div>
            ) : (
              logs.map((line, index) => (
                <div key={`${line.time}-${index}`} className="flex gap-3 leading-6">
                  <span className="shrink-0 select-none text-zinc-600">{line.time}</span>
                  {line.type === "tool_call" ? (
                    <span className="flex items-center gap-1.5 text-blue-400">
                      <Wrench className="h-3 w-3" />
                      {line.content}
                    </span>
                  ) : line.type === "tool_result" ? (
                    <span className="italic text-zinc-500">{line.content}</span>
                  ) : line.type === "thinking" ? (
                    <span className="italic text-amber-400">{line.content}</span>
                  ) : line.content.includes("fail") || line.content.includes("error") ? (
                    <span className="text-red-400">{line.content}</span>
                  ) : line.content.includes("done") || line.content.includes("success") ? (
                    <span className="text-emerald-400">{line.content}</span>
                  ) : (
                    <span className="text-zinc-300">{line.content}</span>
                  )}
                </div>
              ))
            )}
            <div ref={logEndRef} />
          </div>
        </div>
      </div>
    </div>
  );
}
