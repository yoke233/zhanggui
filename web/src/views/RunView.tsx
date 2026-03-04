import { useCallback, useEffect, useMemo, useState } from "react";
import type { ApiClient } from "../lib/apiClient";
import type { Run } from "../types/workflow";
import type { RunActionRequest, RunCheckpoint, RunEvent } from "../types/api";
import GitHubStatusBadge from "../components/GitHubStatusBadge";

interface RunViewProps {
  apiClient: ApiClient;
  projectId: string;
  refreshToken: number;
}

const Run_STAGE_ORDER: Record<string, string[]> = {
  standard: [
    "requirements",
    "setup",
    "implement",
    "review",
    "fixup",
    "test",
    "merge",
    "cleanup",
  ],
  quick: [
    "requirements",
    "setup",
    "implement",
    "review",
    "merge",
    "cleanup",
  ],
  hotfix: ["requirements", "setup", "implement", "merge", "cleanup"],
};

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error && error.message.trim().length > 0) {
    return error.message;
  }
  return "请求失败，请稍后重试";
};

const PAGE_LIMIT = 50;
const REFRESH_INTERVAL_MS = 10_000;

const getRunProgress = (Run: Run) => {
  const stages = Run_STAGE_ORDER[Run.template] ?? [];
  const totalStages = stages.length;
  if (totalStages === 0) {
    return {
      percentage: 0,
      stageText: "未知模板，无法计算运行进度",
    };
  }

  const currentIndex = stages.findIndex((stage) => stage === Run.current_stage);
  if (Run.status === "done" || Run.status === "failed" || Run.status === "timeout") {
    return {
      percentage: 100,
      stageText: `${totalStages}/${totalStages}`,
    };
  }
  if (Run.status === "created") {
    return {
      percentage: 0,
      stageText: `0/${totalStages}`,
    };
  }

  const safeIndex = currentIndex >= 0 ? currentIndex : 0;
  const completed = safeIndex + 0.5;
  const percentage = Math.min(100, Math.max(0, Math.round((completed / totalStages) * 100)));
  return {
    percentage,
    stageText: `${Math.max(0, safeIndex) + 1}/${totalStages}`,
  };
};

const EVENT_TONE_MAP: Record<string, "neutral" | "success" | "warning" | "danger"> = {
  run_done: "success",
  auto_merged: "success",
  run_failed: "danger",
  run_started: "neutral",
  stage_started: "neutral",
};

const EVENT_TONE_CLASS: Record<string, string> = {
  neutral: "border-[#d0d7de] bg-white",
  success: "border-green-300 bg-green-50",
  warning: "border-yellow-300 bg-yellow-50",
  danger: "border-red-300 bg-red-50",
};

const formatEventType = (eventType: string): string =>
  eventType
    .split("_")
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(" ");

const RunView = ({ apiClient, projectId, refreshToken }: RunViewProps) => {
  const [Runs, setRuns] = useState<Run[]>([]);
  const [selectedRunId, setSelectedRunId] = useState<string | null>(null);
  const [checkpoints, setCheckpoints] = useState<RunCheckpoint[]>([]);
  const [runEvents, setRunEvents] = useState<RunEvent[]>([]);
  const [loading, setLoading] = useState(false);
  const [checkpointsLoading, setCheckpointsLoading] = useState(false);
  const [eventsLoading, setEventsLoading] = useState(false);
  const [actionLoading, setActionLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [actionNotice, setActionNotice] = useState<string | null>(null);
  const [actionMessage, setActionMessage] = useState("");
  const [changeRoleValue, setChangeRoleValue] = useState("");

  useEffect(() => {
    let cancelled = false;
    let inFlight = false;
    const loadRuns = async () => {
      if (inFlight) {
        return;
      }
      inFlight = true;
      setLoading(true);
      setError(null);

      try {
        const allRuns: Run[] = [];
        let offset = 0;
        while (true) {
          const response = await apiClient.listRuns(projectId, {
            limit: PAGE_LIMIT,
            offset,
          });
          if (cancelled) {
            return;
          }
          allRuns.push(...response.items);
          const currentCount = response.items.length;
          if (currentCount === 0) {
            break;
          }
          offset += currentCount;
          if (currentCount < PAGE_LIMIT) {
            break;
          }
        }
        if (!cancelled) {
          setRuns(allRuns);
          setSelectedRunId((current) => {
            if (current && allRuns.some((item) => item.id === current)) {
              return current;
            }
            return allRuns[0]?.id ?? null;
          });
        }
      } catch (requestError) {
        if (!cancelled) {
          setRuns([]);
          setSelectedRunId(null);
          setCheckpoints([]);
          setError(getErrorMessage(requestError));
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
        inFlight = false;
      }
    };

    void loadRuns();
    // Fallback refresh for non-board scenarios where WS events are not enough to keep list current.
    const intervalId = setInterval(() => {
      void loadRuns();
    }, REFRESH_INTERVAL_MS);
    return () => {
      cancelled = true;
      clearInterval(intervalId);
    };
  }, [apiClient, projectId, refreshToken]);

  const loadCheckpoints = useCallback(
    async (RunId: string) => {
      setCheckpointsLoading(true);
      setActionError(null);
      try {
        const response = await apiClient.getRunCheckpoints(projectId, RunId);
        setCheckpoints(response);
      } catch (requestError) {
        setCheckpoints([]);
        setActionError(getErrorMessage(requestError));
      } finally {
        setCheckpointsLoading(false);
      }
    },
    [apiClient, projectId],
  );

  useEffect(() => {
    if (!selectedRunId) {
      setCheckpoints([]);
      return;
    }
    void loadCheckpoints(selectedRunId);
  }, [selectedRunId, loadCheckpoints]);

  useEffect(() => {
    if (!selectedRunId) {
      setRunEvents([]);
      return;
    }
    let cancelled = false;
    const loadEvents = async () => {
      setEventsLoading(true);
      try {
        const response = await apiClient.listRunEvents(selectedRunId);
        if (!cancelled) {
          setRunEvents(response.items);
        }
      } catch {
        if (!cancelled) {
          setRunEvents([]);
        }
      } finally {
        if (!cancelled) {
          setEventsLoading(false);
        }
      }
    };
    void loadEvents();
    return () => {
      cancelled = true;
    };
  }, [apiClient, selectedRunId]);

  const selectedRun = useMemo(
    () => Runs.find((Run) => Run.id === selectedRunId) ?? null,
    [Runs, selectedRunId],
  );
  const progress = selectedRun ? getRunProgress(selectedRun) : null;

  const currentStageTeamLeader = useMemo(() => {
    if (!selectedRun) return null;
    let cp: RunCheckpoint | undefined;
    for (let i = checkpoints.length - 1; i >= 0; i -= 1) {
      if (checkpoints[i]?.stage_name === selectedRun.current_stage) {
        cp = checkpoints[i];
        break;
      }
    }
    return cp?.agent_used || null;
  }, [selectedRun, checkpoints]);

  const isTerminal = selectedRun
    ? ["done", "failed", "failed"].includes(selectedRun.status)
    : true;

  const handleRunAction = async (
    action: RunActionRequest["action"],
  ) => {
    if (!selectedRun) {
      return;
    }

    setActionLoading(true);
    setActionError(null);
    setActionNotice(null);
    try {
      const body: RunActionRequest = { action };
      const trimmedMessage = actionMessage.trim();
      if (action === "reject") {
        body.stage = selectedRun.current_stage || undefined;
        body.message = trimmedMessage || "人工驳回，请调整后重试。";
      } else if (action === "change_role") {
        body.role = changeRoleValue.trim();
        body.stage = selectedRun.current_stage || undefined;
        if (trimmedMessage) {
          body.message = trimmedMessage;
        }
      } else if (trimmedMessage) {
        body.message = trimmedMessage;
      }

      const response = await apiClient.applyRunAction(
        projectId,
        selectedRun.id,
        body,
      );
      setRuns((current) =>
        current.map((Run) =>
          Run.id === selectedRun.id
            ? {
                ...Run,
                status: response.status as Run["status"],
                current_stage: response.current_stage ?? Run.current_stage,
              }
            : Run,
        ),
      );
      setActionNotice(`动作 ${action} 已提交，状态：${response.status}`);
      await loadCheckpoints(selectedRun.id);
    } catch (requestError) {
      setActionError(getErrorMessage(requestError));
    } finally {
      setActionLoading(false);
    }
  };

  return (
    <section className="flex flex-col gap-4">
      <header className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
        <h1 className="text-xl font-bold">Runs</h1>
        <p className="mt-1 text-sm text-slate-600">
          Team Leader 视图：运行进度、输出区、checkpoint 区与人工动作入口。
        </p>
      </header>

      {error ? (
        <p className="rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">
          {error}
        </p>
      ) : null}

      <section className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
        {loading ? (
          <p className="text-sm text-slate-500">加载中...</p>
        ) : Runs.length === 0 ? (
          <p className="text-sm text-slate-500">当前项目暂无运行记录。</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="min-w-full table-auto border-collapse text-sm">
              <thead>
                <tr className="border-b border-slate-200 text-left text-xs text-slate-500">
                  <th className="px-2 py-2 font-semibold">ID</th>
                  <th className="px-2 py-2 font-semibold">Name</th>
                  <th className="px-2 py-2 font-semibold">Status</th>
                  <th className="px-2 py-2 font-semibold">Current Stage</th>
                  <th className="px-2 py-2 font-semibold">Updated</th>
                </tr>
              </thead>
              <tbody>
                {Runs.map((Run) => (
                  <tr
                    key={Run.id}
                    data-testid="Run-row"
                    className={`cursor-pointer border-b border-slate-100 ${
                      selectedRunId === Run.id ? "bg-slate-50" : ""
                    }`}
                    onClick={() => {
                      setSelectedRunId(Run.id);
                    }}
                  >
                    <td className="px-2 py-2 font-mono text-xs">{Run.id}</td>
                    <td className="px-2 py-2">{Run.name}</td>
                    <td className="px-2 py-2">{Run.status}</td>
                    <td className="px-2 py-2">{Run.current_stage || "-"}</td>
                    <td className="px-2 py-2">
                      {Run.updated_at ? new Date(Run.updated_at).toLocaleString("zh-CN") : "-"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <section className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
        <h2 className="text-sm font-semibold">阶段进度</h2>
        {!selectedRun || !progress ? (
          <p className="mt-2 text-xs text-slate-500">请选择运行记录查看阶段进度。</p>
        ) : (
          <>
            <div className="mt-2">
              <GitHubStatusBadge status={selectedRun.github?.connection_status} />
            </div>
            <div className="mt-2 h-3 overflow-hidden rounded-full bg-slate-200">
              <div
                data-testid="Run-progress-value"
                className="h-full bg-slate-900 transition-all"
                style={{ width: `${progress.percentage}%` }}
              />
            </div>
            <p className="mt-2 text-xs text-slate-600">
              stage={selectedRun.current_stage || "-"}
              {currentStageTeamLeader ? ` · team_leader=${currentStageTeamLeader}` : ""} · 进度{" "}
              {progress.stageText} · {progress.percentage}%
            </p>
          </>
        )}
      </section>

      <section className="grid gap-4 xl:grid-cols-2">
        <article className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
          <h3 className="text-sm font-semibold">输出区</h3>
          {!selectedRun ? (
            <p className="mt-2 text-xs text-slate-500">请选择运行记录查看输出。</p>
          ) : (
            <div className="mt-2 space-y-2 text-xs">
              <p className="text-slate-600">GitHub</p>
              <div data-testid="Run-github-links" className="rounded-md border border-slate-200 px-2 py-2">
                <div className="flex flex-wrap gap-3">
                  {selectedRun.github?.issue_url ? (
                    <a
                      href={selectedRun.github.issue_url}
                      target="_blank"
                      rel="noreferrer"
                      className="text-blue-700 underline"
                    >
                      {selectedRun.github.issue_number
                        ? `Issue #${selectedRun.github.issue_number}`
                        : "Issue Link"}
                    </a>
                  ) : null}
                  {selectedRun.github?.pr_url ? (
                    <a
                      href={selectedRun.github.pr_url}
                      target="_blank"
                      rel="noreferrer"
                      className="text-blue-700 underline"
                    >
                      {selectedRun.github.pr_number
                        ? `PR #${selectedRun.github.pr_number}`
                        : "PR Link"}
                    </a>
                  ) : null}
                </div>
                {!selectedRun.github?.issue_url && !selectedRun.github?.pr_url ? (
                  <p className="text-slate-500">暂无 GitHub Issue/PR 关联。</p>
                ) : null}
              </div>
              <p className="text-slate-600">Artifacts</p>
              <pre className="max-h-52 overflow-auto rounded-md bg-slate-950 p-3 text-slate-100">
                {JSON.stringify(selectedRun.artifacts ?? {}, null, 2)}
              </pre>
              <p className="text-slate-600">Error</p>
              <pre className="max-h-24 overflow-auto rounded-md bg-slate-100 p-2 text-slate-800">
                {selectedRun.error_message || "-"}
              </pre>
            </div>
          )}
        </article>

        <article className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
          <h3 className="text-sm font-semibold">Checkpoint 区</h3>
          {checkpointsLoading ? (
            <p className="mt-2 text-xs text-slate-500">checkpoint 加载中...</p>
          ) : checkpoints.length === 0 ? (
            <p className="mt-2 text-xs text-slate-500">暂无 checkpoint 数据。</p>
          ) : (
            <ul className="mt-2 space-y-1 text-xs text-slate-700">
              {checkpoints.map((checkpoint, index) => (
                <li
                  key={`${checkpoint.stage_name}-${checkpoint.started_at}-${index}`}
                  className="rounded border border-slate-200 px-2 py-1"
                >
                  <span className="font-medium">{checkpoint.stage_name}</span> ·{" "}
                  <span>{checkpoint.status}</span> · team_leader={checkpoint.agent_used || "-"} · retry={checkpoint.retry_count}
                </li>
              ))}
            </ul>
          )}
        </article>
      </section>

      <section className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
        <h3 className="text-sm font-semibold">Events</h3>
        {eventsLoading ? (
          <p className="mt-2 text-xs text-slate-500">事件加载中...</p>
        ) : runEvents.length === 0 ? (
          <p className="mt-2 text-xs text-slate-500">No events recorded</p>
        ) : (
          <ul className="mt-3 space-y-3">
            {runEvents.map((event) => {
              const tone = EVENT_TONE_MAP[event.event_type] ?? "neutral";
              return (
                <li
                  key={event.id}
                  className={`rounded-lg border px-4 py-3 ${EVENT_TONE_CLASS[tone]}`}
                >
                  <div className="flex items-center gap-2">
                    <span
                      className={`inline-block h-2 w-2 rounded-full ${
                        tone === "success"
                          ? "bg-green-500"
                          : tone === "danger"
                            ? "bg-red-500"
                            : tone === "warning"
                              ? "bg-yellow-500"
                              : "bg-slate-400"
                      }`}
                    />
                    <span className="text-sm font-semibold text-slate-800">
                      {formatEventType(event.event_type)}
                    </span>
                    <span className="ml-auto text-xs text-slate-500">
                      {new Date(event.created_at).toLocaleString("zh-CN")}
                    </span>
                  </div>
                  <div className="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-xs text-slate-600">
                    <span>executor: {event.agent ?? "system"}</span>
                    {event.stage ? <span>stage: {event.stage}</span> : null}
                  </div>
                  {event.error ? (
                    <p className="mt-1 text-xs text-red-600">{event.error}</p>
                  ) : null}
                  {event.data && Object.keys(event.data).length > 0 ? (
                    <dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-xs">
                      {Object.entries(event.data).map(([key, value]) => (
                        <div key={key} className="contents">
                          <dt className="font-medium text-slate-500">{key}</dt>
                          <dd className="text-slate-700">{value}</dd>
                        </div>
                      ))}
                    </dl>
                  ) : null}
                </li>
              );
            })}
          </ul>
        )}
      </section>

      <section className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
        <h3 className="text-sm font-semibold">人工动作</h3>
        <p className="mt-1 text-xs text-slate-500">
          Run Action API：审批、流程控制与 Team Leader 切换。
        </p>
        <label htmlFor="Run-action-message" className="mt-2 block text-xs text-slate-700">
          动作备注（可选）
        </label>
        <input
          id="Run-action-message"
          className="mt-1 w-full rounded-md border border-slate-300 px-2 py-1 text-sm"
          value={actionMessage}
          onChange={(event) => {
            setActionMessage(event.target.value);
          }}
          disabled={!selectedRun || actionLoading}
        />
        <div className="mt-3 grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
          <button
            type="button"
            className="rounded-md border border-emerald-300 px-3 py-2 text-sm text-emerald-700 disabled:opacity-50"
            disabled={!selectedRun || actionLoading}
            onClick={() => {
              void handleRunAction("approve");
            }}
          >
            Approve
          </button>
          <button
            type="button"
            className="rounded-md border border-rose-300 px-3 py-2 text-sm text-rose-700 disabled:opacity-50"
            disabled={!selectedRun || actionLoading}
            onClick={() => {
              void handleRunAction("reject");
            }}
          >
            Reject
          </button>
          <button
            type="button"
            className="rounded-md border border-amber-300 px-3 py-2 text-sm text-amber-700 disabled:opacity-50"
            disabled={!selectedRun || actionLoading}
            onClick={() => {
              void handleRunAction("skip");
            }}
          >
            Skip
          </button>
          <button
            type="button"
            className="rounded-md border border-slate-300 px-3 py-2 text-sm disabled:opacity-50"
            disabled={!selectedRun || actionLoading}
            onClick={() => {
              void handleRunAction("abort");
            }}
          >
            Abort
          </button>
        </div>
        <div className="mt-2 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
          <button
            type="button"
            className="rounded-md border border-sky-300 px-3 py-2 text-sm text-sky-700 disabled:opacity-50"
            disabled={!selectedRun || actionLoading || selectedRun?.status !== "running"}
            onClick={() => {
              void handleRunAction("pause");
            }}
          >
            Pause
          </button>
          <button
            type="button"
            className="rounded-md border border-sky-300 px-3 py-2 text-sm text-sky-700 disabled:opacity-50"
            disabled={!selectedRun || actionLoading || selectedRun?.status !== "waiting_review"}
            onClick={() => {
              void handleRunAction("resume");
            }}
          >
            Resume
          </button>
          <button
            type="button"
            className="rounded-md border border-indigo-300 px-3 py-2 text-sm text-indigo-700 disabled:opacity-50"
            disabled={
              !selectedRun ||
              actionLoading ||
              (selectedRun?.status !== "failed" && selectedRun?.status !== "done")
            }
            onClick={() => {
              void handleRunAction("rerun");
            }}
          >
            Rerun
          </button>
        </div>
        <div className="mt-2 flex gap-2">
          <input
            id="Run-change-role"
            className="flex-1 rounded-md border border-slate-300 px-2 py-1 text-sm"
            placeholder="目标 Team Leader（如 claude, codex）"
            value={changeRoleValue}
            onChange={(event) => {
              setChangeRoleValue(event.target.value);
            }}
            disabled={!selectedRun || actionLoading || isTerminal}
          />
          <button
            type="button"
            className="rounded-md border border-violet-300 px-3 py-2 text-sm text-violet-700 disabled:opacity-50"
            disabled={
              !selectedRun ||
              actionLoading ||
              isTerminal ||
              changeRoleValue.trim().length === 0
            }
            onClick={() => {
              void handleRunAction("change_role");
            }}
          >
            Change Team Leader
          </button>
        </div>
        {actionNotice ? (
          <p className="mt-2 rounded-md border border-emerald-200 bg-emerald-50 px-2 py-1 text-xs text-emerald-700">
            {actionNotice}
          </p>
        ) : null}
        {actionError ? (
          <p className="mt-2 rounded-md border border-rose-200 bg-rose-50 px-2 py-1 text-xs text-rose-700">
            {actionError}
          </p>
        ) : null}
      </section>
    </section>
  );
};

export default RunView;

