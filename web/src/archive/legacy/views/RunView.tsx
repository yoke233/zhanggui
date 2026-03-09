import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import GitHubStatusBadge from "@/archive/legacy/components/GitHubStatusBadge";
import type { ApiClient } from "@/lib/apiClient";
import type { ApiRun, RunEvent } from "@/types/api";

interface RunViewProps {
  apiClient: ApiClient;
  projectId: string;
  refreshToken: number;
}

const PAGE_LIMIT = 50;
const REFRESH_INTERVAL_MS = 10_000;

const EVENT_TONE_MAP: Record<string, "neutral" | "success" | "warning" | "danger"> = {
  run_completed: "success",
  auto_merged: "success",
  run_failed: "danger",
  run_cancelled: "warning",
  run_started: "neutral",
  stage_started: "neutral",
};

const EVENT_TONE_CLASS: Record<string, string> = {
  neutral: "border-[#d0d7de] bg-white",
  success: "border-green-300 bg-green-50",
  warning: "border-yellow-300 bg-yellow-50",
  danger: "border-red-300 bg-red-50",
};

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error && error.message.trim().length > 0) {
    return error.message;
  }
  return "请求失败，请稍后重试";
};

const formatEventType = (eventType: string): string =>
  eventType
    .split("_")
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(" ");

const normalizeRunStatus = (run: ApiRun): string => {
  const raw = String(run.status ?? "").trim();
  return raw.length > 0 ? raw : "unknown";
};

const getRunTitle = (run: ApiRun): string => {
  const issueID = String(run.issue_id ?? "").trim();
  if (issueID.length > 0) {
    return `Issue ${issueID}`;
  }
  return `Run ${run.id}`;
};

const getRunProgress = (run: ApiRun): { percentage: number; stageText: string } => {
  const status = normalizeRunStatus(run).toLowerCase();
  switch (status) {
    case "queued":
      return { percentage: 15, stageText: "Queued" };
    case "in_progress":
      return { percentage: 60, stageText: "In Progress" };
    case "action_required":
      return { percentage: 85, stageText: "Action Required" };
    case "completed":
      return { percentage: 100, stageText: "Completed" };
    default:
      return { percentage: 0, stageText: "Unknown" };
  }
};

const RunView = ({ apiClient, projectId, refreshToken }: RunViewProps) => {
  const [runs, setRuns] = useState<ApiRun[]>([]);
  const [selectedRunId, setSelectedRunId] = useState<string | null>(null);
  const [runEvents, setRunEvents] = useState<RunEvent[]>([]);
  const [loading, setLoading] = useState(false);
  const [eventsLoading, setEventsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const requestSeqRef = useRef(0);

  const loadRuns = useCallback(async () => {
    const requestSeq = requestSeqRef.current + 1;
    requestSeqRef.current = requestSeq;
    setLoading(true);
    setError(null);

    try {
      const allRuns: ApiRun[] = [];
      let offset = 0;
      while (true) {
        const response = await apiClient.listRuns(projectId, {
          limit: PAGE_LIMIT,
          offset,
        });
        if (requestSeq !== requestSeqRef.current) {
          return;
        }
        const pageItems = Array.isArray(response.items) ? response.items : [];
        allRuns.push(...pageItems);
        const currentCount = pageItems.length;
        if (currentCount === 0 || currentCount < PAGE_LIMIT) {
          break;
        }
        offset += currentCount;
      }

      if (requestSeq !== requestSeqRef.current) {
        return;
      }

      setRuns(allRuns);
      setSelectedRunId((current) => {
        if (current && allRuns.some((run) => run.id === current)) {
          return current;
        }
        return allRuns[0]?.id ?? null;
      });
    } catch (requestError) {
      if (requestSeq !== requestSeqRef.current) {
        return;
      }
      setRuns([]);
      setSelectedRunId(null);
      setRunEvents([]);
      setError(getErrorMessage(requestError));
    } finally {
      if (requestSeq === requestSeqRef.current) {
        setLoading(false);
      }
    }
  }, [apiClient, projectId]);

  useEffect(() => {
    void loadRuns();
    const intervalId = setInterval(() => {
      void loadRuns();
    }, REFRESH_INTERVAL_MS);
    return () => {
      clearInterval(intervalId);
    };
  }, [loadRuns, refreshToken]);

  useEffect(() => {
    if (!selectedRunId) {
      setRunEvents([]);
      return;
    }

    let cancelled = false;
    const loadRunEvents = async () => {
      setEventsLoading(true);
      try {
        const response = await apiClient.listRunEvents(selectedRunId);
        if (cancelled) {
          return;
        }
        setRunEvents(Array.isArray(response.items) ? response.items : []);
      } catch {
        if (cancelled) {
          return;
        }
        setRunEvents([]);
      } finally {
        if (!cancelled) {
          setEventsLoading(false);
        }
      }
    };

    void loadRunEvents();
    return () => {
      cancelled = true;
    };
  }, [apiClient, selectedRunId]);

  const selectedRun = useMemo(
    () => runs.find((run) => run.id === selectedRunId) ?? null,
    [runs, selectedRunId],
  );

  const progress = selectedRun ? getRunProgress(selectedRun) : null;

  const latestStage = useMemo(() => {
    for (let i = runEvents.length - 1; i >= 0; i -= 1) {
      const stage = String(runEvents[i]?.stage ?? "").trim();
      if (stage.length > 0) {
        return stage;
      }
    }
    return "";
  }, [runEvents]);

  return (
    <section className="flex flex-col gap-4">
      <header className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
        <h1 className="text-xl font-bold">Runs</h1>
        <p className="mt-1 text-sm text-slate-600">
          Workflow Run 只读视图：列表、状态、事件流与 GitHub 关联信息。
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
        ) : runs.length === 0 ? (
          <p className="text-sm text-slate-500">当前项目暂无运行记录。</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="min-w-full table-auto border-collapse text-sm">
              <thead>
                <tr className="border-b border-slate-200 text-left text-xs text-slate-500">
                  <th className="px-2 py-2 font-semibold">ID</th>
                  <th className="px-2 py-2 font-semibold">Title</th>
                  <th className="px-2 py-2 font-semibold">Issue</th>
                  <th className="px-2 py-2 font-semibold">Profile</th>
                  <th className="px-2 py-2 font-semibold">Status</th>
                  <th className="px-2 py-2 font-semibold">Conclusion</th>
                  <th className="px-2 py-2 font-semibold">Updated</th>
                </tr>
              </thead>
              <tbody>
                {runs.map((run) => (
                  <tr
                    key={run.id}
                    data-testid="run-row"
                    className={`cursor-pointer border-b border-slate-100 ${
                      selectedRunId === run.id ? "bg-slate-50" : ""
                    }`}
                    onClick={() => {
                      setSelectedRunId(run.id);
                    }}
                  >
                    <td className="px-2 py-2 font-mono text-xs">{run.id}</td>
                    <td className="px-2 py-2">{getRunTitle(run)}</td>
                    <td className="px-2 py-2">{run.issue_id || "-"}</td>
                    <td className="px-2 py-2">{run.profile || "-"}</td>
                    <td className="px-2 py-2">{normalizeRunStatus(run)}</td>
                    <td className="px-2 py-2">{run.conclusion || "-"}</td>
                    <td className="px-2 py-2">
                      {run.updated_at ? new Date(run.updated_at).toLocaleString("zh-CN") : "-"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <section className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
        <h2 className="text-sm font-semibold">运行概览</h2>
        {!selectedRun || !progress ? (
          <p className="mt-2 text-xs text-slate-500">请选择运行记录查看运行概览。</p>
        ) : (
          <>
            <div className="mt-2">
              <GitHubStatusBadge status={selectedRun.github?.connection_status} />
            </div>
            <div className="mt-2 h-3 overflow-hidden rounded-full bg-slate-200">
              <div
                data-testid="run-progress-value"
                className="h-full bg-slate-900 transition-all"
                style={{ width: `${progress.percentage}%` }}
              />
            </div>
            <p className="mt-2 text-xs text-slate-600">
              status={normalizeRunStatus(selectedRun)} · stage={latestStage || "-"} ·{" "}
              {progress.stageText} · {progress.percentage}%
            </p>
          </>
        )}
      </section>

      <section className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
        <h3 className="text-sm font-semibold">输出区</h3>
        {!selectedRun ? (
          <p className="mt-2 text-xs text-slate-500">请选择运行记录查看输出。</p>
        ) : (
          <div className="mt-2 space-y-2 text-xs">
            <p className="text-slate-600">GitHub</p>
            <div data-testid="run-github-links" className="rounded-md border border-slate-200 px-2 py-2">
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
                    {selectedRun.github.pr_number ? `PR #${selectedRun.github.pr_number}` : "PR Link"}
                  </a>
                ) : null}
              </div>
              {!selectedRun.github?.issue_url && !selectedRun.github?.pr_url ? (
                <p className="text-slate-500">暂无 GitHub Issue/PR 关联。</p>
              ) : null}
            </div>
            <p className="text-slate-600">Run</p>
            <pre className="max-h-52 overflow-auto rounded-md bg-slate-950 p-3 text-slate-100">
              {JSON.stringify(
                {
                  issue_id: selectedRun.issue_id ?? null,
                  profile: selectedRun.profile ?? null,
                  status: selectedRun.status,
                  conclusion: selectedRun.conclusion ?? null,
                  error: selectedRun.error ?? null,
                  created_at: selectedRun.created_at,
                  started_at: selectedRun.started_at ?? null,
                  finished_at: selectedRun.finished_at ?? null,
                  updated_at: selectedRun.updated_at,
                },
                null,
                2,
              )}
            </pre>
            <p className="text-slate-600">Error</p>
            <pre className="max-h-24 overflow-auto rounded-md bg-slate-100 p-2 text-slate-800">
              {selectedRun.error || "-"}
            </pre>
          </div>
        )}
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
                          <dd className="text-slate-700">{String(value)}</dd>
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

    </section>
  );
};

export default RunView;
