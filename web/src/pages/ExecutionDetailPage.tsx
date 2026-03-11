import { useEffect, useMemo, useRef, useState } from "react";
import { useParams, Link } from "react-router-dom";
import {
  ChevronRight,
  FileText,
  CheckCircle2,
  AlertTriangle,
  Clock,
  Bot,
  Terminal,
  Wrench,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { StatusBadge } from "@/components/status-badge";
import { useV2Workbench } from "@/contexts/V2WorkbenchContext";
import { formatRelativeTime, getErrorMessage, normalizeStepTypeLabel } from "@/lib/v2Workbench";
import type { Artifact, Briefing, Event, Execution, Flow, Step } from "@/types/apiV2";

interface LogLine {
  time: string;
  type: "text" | "tool_call" | "tool_result" | "thinking";
  content: string;
}

const eventToLogLine = (event: Event): LogLine | null => {
  const time = new Date(event.timestamp).toLocaleTimeString("zh-CN", {
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
  const { apiClient } = useV2Workbench();
  const numericExecId = Number.parseInt(execId ?? "", 10);
  const logEndRef = useRef<HTMLDivElement>(null);

  const [execution, setExecution] = useState<Execution | null>(null);
  const [step, setStep] = useState<Step | null>(null);
  const [flow, setFlow] = useState<Flow | null>(null);
  const [briefing, setBriefing] = useState<Briefing | null>(null);
  const [artifact, setArtifact] = useState<Artifact | null>(null);
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
        const execResp = await apiClient.getExecution(numericExecId);
        if (cancelled) {
          return;
        }
        setExecution(execResp);

        const [stepResp, flowResp, briefingResp, artifactsResp, eventResp] = await Promise.all([
          apiClient.getStep(execResp.step_id),
          apiClient.getFlow(execResp.flow_id),
          apiClient.getBriefingByStep(execResp.step_id).catch(() => null),
          apiClient.listArtifactsByExecution(execResp.id),
          apiClient.listEvents({
            flow_id: execResp.flow_id,
            step_id: execResp.step_id,
            limit: 200,
            offset: 0,
          }),
        ]);
        if (cancelled) {
          return;
        }
        setStep(stepResp);
        setFlow(flowResp);
        setBriefing(briefingResp);
        setArtifact(artifactsResp[0] ?? null);
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
        .map(eventToLogLine)
        .filter((line): line is LogLine => line != null),
    [events],
  );

  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [logs]);

  return (
    <div className="flex h-full flex-col">
      <div className="border-b px-8 py-4">
        <div className="mb-2 flex items-center gap-2 text-sm text-muted-foreground">
          <Link to="/flows" className="hover:text-foreground">流程</Link>
          <ChevronRight className="h-3 w-3" />
          {flow ? <Link to={`/flows/${flow.id}`} className="hover:text-foreground">{flow.name}</Link> : <span>Flow</span>}
          <ChevronRight className="h-3 w-3" />
          <span className="text-foreground">{step?.name ?? "Execution"}</span>
          <span className="mx-1">·</span>
          <span className="text-foreground">执行详情</span>
        </div>
        <div className="flex items-center gap-3">
          <h1 className="text-xl font-bold">
            {step?.name ?? "执行"} {execution ? `— 第 ${execution.attempt} 次尝试` : ""}
          </h1>
          {execution ? <StatusBadge status={execution.status} /> : null}
          <span className="ml-auto text-sm text-muted-foreground">Execution #{execId ?? "-"}</span>
        </div>
      </div>

      {loading ? <p className="px-8 py-4 text-sm text-muted-foreground">加载 execution 详情中...</p> : null}
      {error ? <p className="mx-8 mt-4 rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p> : null}

      <div className="flex flex-1 overflow-hidden">
        <div className="w-[420px] space-y-6 overflow-y-auto border-r p-6">
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="flex items-center gap-2 text-base">
                <FileText className="h-4 w-4" />
                任务说明
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div>
                <h4 className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">目标</h4>
                <p className="text-sm leading-relaxed">
                  {briefing?.objective || step?.description || "当前 step 没有额外 briefing。"}
                </p>
              </div>
              <div>
                <h4 className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">约束</h4>
                <ul className="space-y-1.5 text-sm">
                  {(briefing?.constraints ?? []).length === 0 ? (
                    <li className="text-muted-foreground">未设置约束</li>
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
                <h4 className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">验收标准</h4>
                <ul className="space-y-1.5 text-sm">
                  {(step?.acceptance_criteria ?? []).length === 0 ? (
                    <li className="text-muted-foreground">未设置验收标准</li>
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
                提交结果
              </CardTitle>
            </CardHeader>
            <CardContent>
              {artifact ? (
                <div className="space-y-3">
                  <div className="rounded-lg border bg-muted/30 p-3 text-sm text-muted-foreground">
                    {artifact.result_markdown || "结果为空"}
                  </div>
                  <div className="text-xs text-muted-foreground">Artifact #{artifact.id} · {formatRelativeTime(artifact.created_at)}</div>
                </div>
              ) : (
                <div className="flex items-center gap-3 rounded-lg border border-dashed p-4">
                  <Clock className="h-5 w-5 text-muted-foreground" />
                  <div>
                    <p className="text-sm font-medium">尚未生成 artifact</p>
                    <p className="text-xs text-muted-foreground">执行可能仍在进行中，或该 step 未提交结果。</p>
                  </div>
                </div>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="flex items-center gap-2 text-base">
                <Bot className="h-4 w-4" />
                执行信息
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-2 text-sm">
              <div className="flex justify-between">
                <span className="text-muted-foreground">Step 类型</span>
                <span className="font-medium">{step ? normalizeStepTypeLabel(step.type) : "-"}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">角色</span>
                <span className="font-medium">{step?.agent_role || execution?.agent_id || "-"}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">尝试</span>
                <span className="font-medium">{execution ? `第 ${execution.attempt} 次` : "-"}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">开始时间</span>
                <span className="font-medium">{execution?.started_at ? formatRelativeTime(execution.started_at) : "-"}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">完成时间</span>
                <span className="font-medium">{execution?.finished_at ? formatRelativeTime(execution.finished_at) : "-"}</span>
              </div>
            </CardContent>
          </Card>
        </div>

        <div className="flex flex-1 flex-col bg-zinc-950">
          <div className="flex items-center justify-between border-b border-zinc-800 px-5 py-3">
            <div className="flex items-center gap-2">
              <Terminal className="h-4 w-4 text-zinc-400" />
              <span className="text-sm font-medium text-zinc-300">事件流 / 代理输出</span>
            </div>
            <Badge variant="outline" className="border-zinc-700 text-xs text-zinc-400">
              {logs.length} 条
            </Badge>
          </div>

          <div className="flex-1 overflow-y-auto p-5 font-mono text-sm">
            {logs.length === 0 ? (
              <div className="text-zinc-500">暂无可展示日志</div>
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
