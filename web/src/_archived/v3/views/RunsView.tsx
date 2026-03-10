import { useEffect, useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Select } from "@/components/ui/select";
import type { ApiClient } from "@/lib/apiClient";
import type { ApiRun, RunCheckpoint, RunEvent } from "@/types/api";

interface RunsViewProps {
  apiClient: ApiClient;
  projectId: string;
  refreshToken: number;
}

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error && error.message.trim().length > 0) {
    return error.message;
  }
  return "请求失败，请稍后重试";
};

const formatTime = (value?: string): string => {
  if (!value) {
    return "-";
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toLocaleString("zh-CN", { hour12: false });
};

const checkpointTone = (status: string): string => {
  switch (status.trim().toLowerCase()) {
    case "success":
      return "border-emerald-200 bg-emerald-50";
    case "failed":
      return "border-rose-200 bg-rose-50";
    case "in_progress":
      return "border-blue-200 bg-blue-50";
    case "invalidated":
      return "border-amber-200 bg-amber-50";
    default:
      return "border-slate-200 bg-slate-50";
  }
};

const eventTone = (type: string): string => {
  if (type.includes("failed")) {
    return "border-rose-200 bg-rose-50";
  }
  if (type.includes("completed") || type.includes("merged")) {
    return "border-emerald-200 bg-emerald-50";
  }
  if (type.includes("started") || type.includes("progress")) {
    return "border-blue-200 bg-blue-50";
  }
  return "border-slate-200 bg-slate-50";
};

const formatEventName = (type: string): string => {
  return type
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
};

const statusVariant = (status: string) => {
  switch (status.trim().toLowerCase()) {
    case "completed":
      return "success" as const;
    case "action_required":
      return "warning" as const;
    case "in_progress":
      return "secondary" as const;
    default:
      return "outline" as const;
  }
};

const formatStatusLabel = (status?: string): string => {
  switch (String(status ?? "").trim().toLowerCase()) {
    case "queued":
      return "排队中";
    case "in_progress":
      return "执行中";
    case "action_required":
      return "待人工动作";
    case "completed":
      return "已完成";
    default:
      return String(status ?? "-");
  }
};

const buildRunMeta = (run: ApiRun | null): string => {
  if (!run) {
    return "等待选择 run";
  }
  return [
    run.profile ? `profile ${run.profile}` : "",
    run.issue_id ? `issue ${run.issue_id}` : "issue -",
    run.github?.issue_number ? `#${run.github.issue_number}` : "",
  ]
    .filter(Boolean)
    .join(" · ");
};

const RunsView = ({ apiClient, projectId, refreshToken }: RunsViewProps) => {
  const [runs, setRuns] = useState<ApiRun[]>([]);
  const [runLoading, setRunLoading] = useState(true);
  const [runError, setRunError] = useState<string | null>(null);
  const [selectedRunId, setSelectedRunId] = useState<string>("");
  const [stageFilter, setStageFilter] = useState("all");
  const [events, setEvents] = useState<RunEvent[]>([]);
  const [checkpoints, setCheckpoints] = useState<RunCheckpoint[]>([]);
  const [detailLoading, setDetailLoading] = useState(false);

  useEffect(() => {
    let cancelled = false;
    const loadRuns = async () => {
      setRunLoading(true);
      setRunError(null);
      try {
        const response = await apiClient.listRuns(projectId, { limit: 40, offset: 0 });
        if (cancelled) {
          return;
        }
        const nextRuns = Array.isArray(response.items) ? response.items : [];
        setRuns(nextRuns);
        setSelectedRunId((current) => {
          if (current && nextRuns.some((run) => run.id === current)) {
            return current;
          }
          return nextRuns[0]?.id ?? "";
        });
      } catch (error) {
        if (cancelled) {
          return;
        }
        setRuns([]);
        setSelectedRunId("");
        setRunError(getErrorMessage(error));
      } finally {
        if (!cancelled) {
          setRunLoading(false);
        }
      }
    };

    void loadRuns();
    return () => {
      cancelled = true;
    };
  }, [apiClient, projectId, refreshToken]);

  useEffect(() => {
    if (!selectedRunId) {
      setEvents([]);
      setCheckpoints([]);
      return;
    }

    let cancelled = false;
    const loadRunDetail = async () => {
      setDetailLoading(true);
      try {
        const [eventResponse, checkpointResponse] = await Promise.all([
          apiClient.listRunEvents(selectedRunId),
          apiClient.getRunCheckpoints(selectedRunId),
        ]);
        if (cancelled) {
          return;
        }
        setEvents(Array.isArray(eventResponse.items) ? eventResponse.items : []);
        setCheckpoints(Array.isArray(checkpointResponse.items) ? checkpointResponse.items : []);
      } catch {
        if (cancelled) {
          return;
        }
        setEvents([]);
        setCheckpoints([]);
      } finally {
        if (!cancelled) {
          setDetailLoading(false);
        }
      }
    };

    void loadRunDetail();
    return () => {
      cancelled = true;
    };
  }, [apiClient, selectedRunId]);

  const selectedRun = runs.find((run) => run.id === selectedRunId) ?? null;

  const stageOptions = useMemo(() => {
    const next = new Set<string>();
    checkpoints.forEach((checkpoint) => {
      if (checkpoint.stage_name.trim()) {
        next.add(checkpoint.stage_name);
      }
    });
    return ["all", ...Array.from(next)];
  }, [checkpoints]);

  const filteredEvents = useMemo(() => {
    if (stageFilter === "all") {
      return events;
    }
    return events.filter((item) => String(item.stage ?? "") === stageFilter);
  }, [events, stageFilter]);

  const runStats = useMemo(() => {
    return runs.reduce(
      (summary, run) => {
        summary.total += 1;
        if (run.status === "in_progress") {
          summary.active += 1;
        }
        if (run.status === "action_required") {
          summary.waiting += 1;
        }
        if (run.status === "completed") {
          summary.completed += 1;
        }
        return summary;
      },
      { total: 0, active: 0, waiting: 0, completed: 0 },
    );
  }, [runs]);

  return (
    <section className="flex flex-col gap-4">
      <Card className="rounded-2xl border-slate-200 shadow-none">
        <CardHeader className="p-5">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div>
              <div className="flex items-center gap-2">
                <Badge variant="secondary" className="bg-indigo-50 text-indigo-600">
                  Run Detail & Events
                </Badge>
                <Badge variant="outline" className="bg-emerald-50 text-emerald-700">
                  先判断再操作
                </Badge>
              </div>
              <CardTitle className="mt-3 text-[24px] font-semibold tracking-[-0.02em]">
                Run 详情与事件流
              </CardTitle>
              <CardDescription className="mt-1">
                把阶段推进、事件时间线、检查点和阶段会话控制放在同一页，先判断再操作。
              </CardDescription>
            </div>
            <div className="grid gap-2 rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3">
              <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">当前运行</p>
              <p className="text-sm font-semibold text-slate-950">{selectedRun?.id || "尚未选择"}</p>
              <p className="text-xs text-slate-500">{buildRunMeta(selectedRun)}</p>
            </div>
          </div>
        </CardHeader>
        <CardContent className="grid gap-3 px-5 pb-5 md:grid-cols-4">
          <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
            <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">总 Run</p>
            <p className="mt-2 text-2xl font-semibold text-slate-950">{runStats.total}</p>
          </div>
          <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
            <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">执行中</p>
            <p className="mt-2 text-2xl font-semibold text-slate-950">{runStats.active}</p>
          </div>
          <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
            <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">待人工动作</p>
            <p className="mt-2 text-2xl font-semibold text-slate-950">{runStats.waiting}</p>
          </div>
          <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
            <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">已完成</p>
            <p className="mt-2 text-2xl font-semibold text-slate-950">{runStats.completed}</p>
          </div>
        </CardContent>
      </Card>

      <div className="rounded-xl border border-slate-200 bg-white px-3 py-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline" className="bg-indigo-50 text-indigo-700">
              profile {selectedRun?.profile || "none"}
            </Badge>
            <Badge variant="outline" className="bg-slate-50 text-slate-600">
              项目 {projectId}
            </Badge>
            <Badge variant="outline" className="bg-slate-50 text-slate-600">
              {checkpoints.length} checkpoints
            </Badge>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="outline" size="sm" onClick={() => setStageFilter("all")}>
              清空阶段筛选
            </Button>
            <Button variant="secondary" size="sm" onClick={() => window.location.reload()}>
              刷新视图
            </Button>
          </div>
        </div>
      </div>

      <div className="grid gap-4 xl:grid-cols-[240px_minmax(0,1fr)_380px]">
        <Card className="rounded-2xl border-slate-200 shadow-none">
          <CardHeader>
            <CardTitle className="text-base">阶段轨道</CardTitle>
            <CardDescription>按阶段观察当前推进位置、错误和关联 issue。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {runLoading ? (
              <p className="text-sm text-slate-500">加载中...</p>
            ) : runError ? (
              <p className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">
                {runError}
              </p>
            ) : runs.length === 0 ? (
              <p className="text-sm text-slate-500">当前项目没有 Run。</p>
            ) : (
              runs.map((run) => (
                <button
                  key={run.id}
                  type="button"
                  onClick={() => setSelectedRunId(run.id)}
                  className={`w-full rounded-xl border px-4 py-3 text-left transition ${
                    selectedRunId === run.id
                      ? "border-blue-300 bg-blue-50"
                      : "border-slate-200 bg-white hover:bg-slate-50"
                  }`}
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-semibold text-slate-950">{run.id}</p>
                      <p className="mt-1 text-xs text-slate-500">
                        issue {run.issue_id || "-"} · profile {run.profile}
                      </p>
                    </div>
                    <Badge variant={statusVariant(String(run.status ?? ""))}>{formatStatusLabel(run.status)}</Badge>
                  </div>
                  <p className="mt-2 text-[11px] text-slate-400">更新于 {formatTime(run.updated_at)}</p>
                </button>
              ))
            )}
          </CardContent>
        </Card>

        <Card className="rounded-2xl border-slate-200 shadow-none">
          <CardHeader>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <CardTitle className="text-base">事件时间线</CardTitle>
                <CardDescription>直接映射运行事件流，先看推进与失败链路，再决定人工动作。</CardDescription>
              </div>
              <div className="min-w-[180px]">
                <Select value={stageFilter} onChange={(event) => setStageFilter(event.target.value)}>
                  {stageOptions.map((option) => (
                    <option key={option} value={option}>
                      {option === "all" ? "全部阶段" : option}
                    </option>
                  ))}
                </Select>
              </div>
            </div>
          </CardHeader>
          <CardContent className="space-y-3">
            {detailLoading ? (
              <p className="text-sm text-slate-500">正在加载事件...</p>
            ) : filteredEvents.length === 0 ? (
              <p className="text-sm text-slate-500">当前没有事件记录。</p>
            ) : (
              filteredEvents.map((event) => (
                <div key={event.id} className={`rounded-xl border px-4 py-3 ${eventTone(event.event_type)}`}>
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <p className="text-sm font-semibold text-slate-950">{formatEventName(event.event_type)}</p>
                      <p className="mt-1 text-xs text-slate-500">
                        {event.stage || "no-stage"} · {event.agent || "system"} · {formatTime(event.created_at)}
                      </p>
                    </div>
                    <Badge variant="outline">{event.id}</Badge>
                  </div>
                  {event.error ? (
                    <p className="mt-2 text-xs leading-5 text-rose-700">{event.error}</p>
                  ) : event.data ? (
                    <div className="mt-2 flex flex-wrap gap-2">
                      {Object.entries(event.data).slice(0, 4).map(([key, value]) => (
                        <span key={key} className="rounded-full bg-white/80 px-2.5 py-1 text-[11px] text-slate-600">
                          {key}: {String(value)}
                        </span>
                      ))}
                    </div>
                  ) : null}
                </div>
              ))
            )}
          </CardContent>
        </Card>

        <div className="flex flex-col gap-4">
          <Card className="rounded-2xl border-slate-200 shadow-none">
            <CardHeader>
              <CardTitle className="text-base">检查点 / 会话工具</CardTitle>
              <CardDescription>检查 gate、会话、重试与错误，而不是只看原始事件。</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              {selectedRun ? (
                <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
                  <p className="text-sm font-semibold text-slate-950">{selectedRun.id}</p>
                  <div className="mt-2 space-y-1 text-xs text-slate-500">
                    <p>profile: {selectedRun.profile}</p>
                    <p>status: {formatStatusLabel(selectedRun.status)}</p>
                    <p>issue: {selectedRun.issue_id || "-"}</p>
                    <p>started: {formatTime(selectedRun.started_at)}</p>
                    <p>finished: {formatTime(selectedRun.finished_at)}</p>
                    <p>更新时间: {formatTime(selectedRun.updated_at)}</p>
                  </div>
                </div>
              ) : null}
              {checkpoints.length === 0 ? (
                <p className="text-sm text-slate-500">暂无检查点数据。</p>
              ) : (
                checkpoints.map((checkpoint) => (
                  <div
                    key={`${checkpoint.run_id}-${checkpoint.stage_name}`}
                    className={`rounded-xl border px-4 py-3 ${checkpointTone(checkpoint.status)}`}
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div>
                        <p className="text-sm font-semibold text-slate-950">{checkpoint.stage_name}</p>
                        <p className="mt-1 text-xs text-slate-500">
                          {checkpoint.agent_used || "agent-unknown"} · retry {checkpoint.retry_count} · tokens {checkpoint.tokens_used}
                        </p>
                      </div>
                      <Badge variant="outline">{checkpoint.status}</Badge>
                    </div>
                    <p className="mt-2 text-xs leading-5 text-slate-600">
                      session {checkpoint.agent_session_id || "未绑定"} · {formatTime(checkpoint.finished_at || checkpoint.started_at)}
                    </p>
                    {checkpoint.error ? (
                      <p className="mt-2 text-xs leading-5 text-rose-700">{checkpoint.error}</p>
                    ) : null}
                  </div>
                ))
              )}
            </CardContent>
          </Card>

          <Card className="rounded-2xl border-slate-200 shadow-none">
            <CardHeader>
              <CardTitle className="text-base">诊断 / 依赖</CardTitle>
              <CardDescription>聚合当前 run 的阻塞、GitHub 关联与人工动作线索。</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
                <p className="text-sm font-semibold text-slate-950">GitHub 关联</p>
                <p className="mt-1 text-xs leading-5 text-slate-500">
                  issue #{selectedRun?.github?.issue_number || "-"} · PR #{selectedRun?.github?.pr_number || "-"} ·{" "}
                  {selectedRun?.github?.connection_status || "unknown"}
                </p>
              </div>
              {selectedRun?.error ? (
                <div className="rounded-2xl border border-rose-200 bg-rose-50 p-4">
                  <p className="text-sm font-semibold text-rose-900">运行错误</p>
                  <p className="mt-1 text-xs leading-5 text-rose-700">{selectedRun.error}</p>
                </div>
              ) : null}
              <div className="rounded-2xl border border-slate-200 bg-amber-50 p-4">
                <p className="text-sm font-semibold text-slate-950">检查建议</p>
                <p className="mt-1 text-xs leading-5 text-slate-600">
                  先看失败事件，再对照检查点错误；只有在 action_required 时再考虑人工唤醒阶段会话。
                </p>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </section>
  );
};

export default RunsView;
