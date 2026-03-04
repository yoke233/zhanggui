import { useEffect, useMemo, useRef, useState } from "react";
import type { ApiClient } from "../lib/apiClient";
import type { IssueTimelineEntry } from "../types/api";

interface BoardViewProps {
  apiClient: ApiClient;
  projectId: string;
  refreshToken: number;
}

export type BoardStatus = "pending" | "ready" | "running" | "done" | "failed";
type BoardStatusFilter = "all" | BoardStatus;

export interface BoardTask {
  id: string;
  title: string;
  status: BoardStatus;
  raw_status: string;
  run_id: string;
  github_issue_number?: number;
  github_issue_url?: string;
  created_at?: string;
  updated_at?: string;
}

type TaskActionType = "submit_review" | "approve" | "abort";

interface TimelineItem {
  id: string;
  title: string;
  executor: string;
  timestamp: string;
  resultSummary: string;
  resultDetail: string;
  referenceTag: string;
  tone: "neutral" | "success" | "warning" | "danger";
}

export const BOARD_COLUMNS: BoardStatus[] = [
  "pending",
  "ready",
  "running",
  "done",
  "failed",
];
const BOARD_FILTERS: BoardStatusFilter[] = ["all", ...BOARD_COLUMNS];

const BOARD_STATUS_LABELS: Record<BoardStatus, string> = {
  pending: "Pending",
  ready: "Ready",
  running: "Running",
  done: "Done",
  failed: "Failed",
};

const BOARD_FILTER_LABELS: Record<BoardStatusFilter, string> = {
  all: "All",
  pending: "Pending",
  ready: "Ready",
  running: "Running",
  done: "Done",
  failed: "Failed",
};

const STATUS_BADGE_CLASS: Record<BoardStatus, string> = {
  pending: "border-[#d0d7de] bg-[#f6f8fa] text-[#57606a]",
  ready: "border-[#2f81f7] bg-[#ddf4ff] text-[#0969da]",
  running: "border-[#d4a72c] bg-[#fff8c5] text-[#9a6700]",
  done: "border-[#1a7f37] bg-[#dafbe1] text-[#1a7f37]",
  failed: "border-[#cf222e] bg-[#ffebe9] text-[#cf222e]",
};

const TIMELINE_TONE_CLASS: Record<TimelineItem["tone"], string> = {
  neutral: "border-[#d0d7de] bg-white text-[#24292f]",
  success: "border-[#1a7f37] bg-[#dafbe1] text-[#1a7f37]",
  warning: "border-[#bf8700] bg-[#fff8c5] text-[#9a6700]",
  danger: "border-[#cf222e] bg-[#ffebe9] text-[#cf222e]",
};

const TASK_ACTION_CONFIG: Record<
  TaskActionType,
  { label: string; description: string }
> = {
  submit_review: {
    label: "Submit review",
    description: "提交进入 review 阶段",
  },
  approve: {
    label: "Approve",
    description: "批准并推进到下一步",
  },
  abort: {
    label: "Abandon",
    description: "终止当前 issue 流程",
  },
};

export const toBoardStatus = (status: string): BoardStatus => {
  switch (status.trim().toLowerCase()) {
    case "draft":
    case "pending":
      return "pending";
    case "reviewing":
      return "ready";
    case "queued":
    case "ready":
      return "ready";
    case "executing":
    case "running":
      return "running";
    case "partially_done":
      return "running";
    case "success":
    case "completed":
    case "done":
      return "done";
    case "skipped":
      return "done";
    case "waiting_review":
    case "failed":
    case "abandoned":
    case "blocked_by_failure":
      return "failed";
    default:
      return "pending";
  }
};

export const groupBoardTasks = (
  tasks: BoardTask[],
): Record<BoardStatus, BoardTask[]> => {
  const grouped: Record<BoardStatus, BoardTask[]> = {
    pending: [],
    ready: [],
    running: [],
    done: [],
    failed: [],
  };
  tasks.forEach((task) => {
    grouped[task.status].push(task);
  });
  return grouped;
};

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error && error.message.trim().length > 0) {
    return error.message;
  }
  return "请求失败，请稍后重试";
};

const formatTimestamp = (value?: string): string => {
  if (!value) {
    return "时间未知";
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toLocaleString("zh-CN", {
    hour12: false,
  });
};

const formatRelativeTimestamp = (value?: string): string => {
  if (!value) {
    return "time unknown";
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
};

const getExecutor = (task: BoardTask): string => {
  if (task.run_id.trim().length > 0) {
    return `run/${task.run_id}`;
  }
  return "team leader";
};

const getReferenceTag = (task: BoardTask): string => {
  if (task.github_issue_number) {
    return `issue#${task.github_issue_number}`;
  }
  return `issue/${task.id}`;
};

const getIssueNumberLabel = (task: BoardTask): string => {
  if (typeof task.github_issue_number === "number") {
    return `#${task.github_issue_number}`;
  }
  return `#${task.id.slice(0, 8)}`;
};

const getAvatarPlaceholder = (value: string): string => {
  const normalized = value.trim();
  if (normalized.length === 0) {
    return "AI";
  }
  return normalized.slice(0, 2).toUpperCase();
};

const normalizeText = (value: unknown): string => {
  if (typeof value === "string") {
    return value;
  }
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  return "";
};

const parseTimelineTimestamp = (item: IssueTimelineEntry): string => {
  return normalizeText(item.created_at);
};

const summarizeText = (value: string, maxLength = 160): string => {
  const normalized = value.replace(/\s+/g, " ").trim();
  if (normalized.length <= maxLength) {
    return normalized;
  }
  return `${normalized.slice(0, maxLength)}...`;
};

const stringifyTimelineMeta = (meta: Record<string, unknown>): string => {
  const keys = Object.keys(meta);
  if (keys.length === 0) {
    return "";
  }
  try {
    return JSON.stringify(meta, null, 2);
  } catch {
    return "";
  }
};

const pickTimelineDetail = (entry: IssueTimelineEntry): string => {
  const meta = entry.meta ?? {};
  const rawOutput = normalizeText(meta.raw_output).trim();
  if (rawOutput.length > 0) {
    return rawOutput;
  }

  const body = normalizeText(entry.body).trim();
  if (body.length > 0) {
    return body;
  }

  const candidates = [
    normalizeText(meta.summary).trim(),
    normalizeText(meta.message).trim(),
    normalizeText(meta.error).trim(),
    normalizeText(meta.content).trim(),
    normalizeText(meta.verdict).trim(),
    normalizeText(meta.action).trim(),
    normalizeText(meta.type).trim(),
  ];
  for (const candidate of candidates) {
    if (candidate.length > 0) {
      return candidate;
    }
  }

  const metaJSON = stringifyTimelineMeta(meta);
  if (metaJSON.length > 0) {
    return metaJSON;
  }

  const status = normalizeText(entry.status).trim();
  if (status.length > 0) {
    return `status=${status}`;
  }
  return "";
};

const pickTimelineSummary = (entry: IssueTimelineEntry): string => {
  const summary = normalizeText(entry.meta?.summary).trim();
  if (summary.length > 0) {
    return summarizeText(summary);
  }
  return summarizeText(pickTimelineDetail(entry));
};

const dedupeTimelineItems = (items: TimelineItem[]): TimelineItem[] => {
  const out: TimelineItem[] = [];
  let previousSignature = "";
  items.forEach((item) => {
    const signature = [
      item.title.trim().toLowerCase(),
      item.executor.trim().toLowerCase(),
      item.referenceTag.trim().toLowerCase(),
      item.resultSummary.trim().toLowerCase(),
      item.tone,
    ].join("|");
    if (signature === previousSignature) {
      return;
    }
    previousSignature = signature;
    out.push(item);
  });
  return out;
};

const toTimelineTone = (status: unknown): TimelineItem["tone"] => {
  switch (normalizeText(status).trim().toLowerCase()) {
    case "success":
      return "success";
    case "failed":
      return "danger";
    case "warning":
      return "warning";
    default:
      return "neutral";
  }
};

const buildTimeline = (task: BoardTask, entries: IssueTimelineEntry[]): TimelineItem[] => {
  if (entries.length === 0) {
    return [];
  }

  const fallbackReference = getReferenceTag(task);
  const mapped = entries.map((entry, index) => {
    const stage = normalizeText(entry.refs?.stage).trim();
    const referenceTag = stage
      ? `${entry.refs?.issue_id ? `issue/${normalizeText(entry.refs.issue_id)}` : fallbackReference}-${stage}`
      : entry.refs?.issue_id
        ? `issue/${normalizeText(entry.refs.issue_id)}`
        : fallbackReference;
    const executor =
      normalizeText(entry.actor_name).trim() ||
      normalizeText(entry.actor_type).trim() ||
      "system";
    return {
      id: normalizeText(entry.event_id) || `${task.id}-${normalizeText(entry.kind)}-${index}`,
      title: normalizeText(entry.title).trim() || normalizeText(entry.kind) || "event",
      executor,
      timestamp: parseTimelineTimestamp(entry),
      resultSummary: pickTimelineSummary(entry),
      resultDetail: pickTimelineDetail(entry),
      referenceTag,
      tone: toTimelineTone(entry.status),
    };
  });
  return dedupeTimelineItems(mapped);
};

const PAGE_LIMIT = 50;
const DEFAULT_AUTO_REFRESH_INTERVAL_MS = 10_000;
const AUTO_REFRESH_INTERVAL_OPTIONS = [
  { label: "5 秒", value: 5_000 },
  { label: "10 秒", value: 10_000 },
  { label: "30 秒", value: 30_000 },
  { label: "60 秒", value: 60_000 },
];

const readRouteIssueID = (): string | null => {
  if (typeof window === "undefined") {
    return null;
  }
  const params = new URLSearchParams(window.location.search);
  const issueID = params.get("issue");
  if (!issueID) {
    return null;
  }
  const normalized = issueID.trim();
  return normalized.length > 0 ? normalized : null;
};

const writeRouteIssueID = (issueID: string | null, historyMode: "push" | "replace" = "push") => {
  if (typeof window === "undefined") {
    return;
  }
  const url = new URL(window.location.href);
  url.searchParams.set("view", "board");
  if (issueID && issueID.trim().length > 0) {
    url.searchParams.set("issue", issueID.trim());
  } else {
    url.searchParams.delete("issue");
  }
  const nextURL = `${url.pathname}${url.search}${url.hash}`;
  if (historyMode === "replace") {
    window.history.replaceState(null, "", nextURL);
    return;
  }
  window.history.pushState(null, "", nextURL);
};

const resolveRouteIssueTaskID = (routeIssueID: string | null, tasks: BoardTask[]): string | null => {
  const candidate = normalizeText(routeIssueID).trim();
  if (candidate.length === 0) {
    return null;
  }
  const byID = tasks.find((task) => task.id === candidate);
  return byID ? byID.id : null;
};

const BoardView = ({ apiClient, projectId, refreshToken }: BoardViewProps) => {
  const [tasks, setTasks] = useState<BoardTask[]>([]);
  const [loading, setLoading] = useState(true);
  const [backgroundRefreshing, setBackgroundRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null);
  const [routeIssueID, setRouteIssueID] = useState<string | null>(() => readRouteIssueID());
  const [statusFilter, setStatusFilter] = useState<BoardStatusFilter>("all");
  const [autoRefreshEnabled, setAutoRefreshEnabled] = useState(false);
  const [autoRefreshIntervalMs, setAutoRefreshIntervalMs] = useState(
    DEFAULT_AUTO_REFRESH_INTERVAL_MS,
  );
  const [manualReloadToken, setManualReloadToken] = useState(0);
  const [actionLoadingTaskId, setActionLoadingTaskId] = useState<string | null>(null);
  const [actionNotice, setActionNotice] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [timelineLoading, setTimelineLoading] = useState(false);
  const [timelineError, setTimelineError] = useState<string | null>(null);
  const [timelineEntries, setTimelineEntries] = useState<IssueTimelineEntry[]>([]);
  const [timelineReloadToken, setTimelineReloadToken] = useState(0);
  const hasLoadedTasksRef = useRef(false);
  const hasLoadedTimelineRef = useRef(false);
  const timelineIssueRef = useRef("");

  useEffect(() => {
    hasLoadedTasksRef.current = false;
    hasLoadedTimelineRef.current = false;
    timelineIssueRef.current = "";
    setRouteIssueID(readRouteIssueID());
    setLoading(true);
    setBackgroundRefreshing(false);
    setTimelineLoading(false);
    setTimelineError(null);
    setTimelineEntries([]);
  }, [projectId]);

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }
    const onPopState = () => {
      setRouteIssueID(readRouteIssueID());
    };
    window.addEventListener("popstate", onPopState);
    return () => {
      window.removeEventListener("popstate", onPopState);
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    let inFlight = false;
    const loadTasks = async () => {
      if (inFlight) {
        return;
      }
      inFlight = true;
      const isBackground = hasLoadedTasksRef.current;
      if (isBackground) {
        setBackgroundRefreshing(true);
      } else {
        setLoading(true);
      }
      setError(null);
      try {
        const allIssues: BoardTask[] = [];
        let offset = 0;
        while (true) {
          const response = await apiClient.listIssues(projectId, {
            limit: PAGE_LIMIT,
            offset,
          });
          if (cancelled) {
            return;
          }
          const items = Array.isArray(response.items) ? response.items : [];
          allIssues.push(
            ...items.map((issue) => ({
              id: issue.id,
              title: issue.title || issue.id,
              status: toBoardStatus(String(issue.status ?? "")),
              raw_status: String(issue.status ?? ""),
              run_id: issue.run_id ?? "",
              github_issue_number: issue.github?.issue_number,
              github_issue_url: issue.github?.issue_url,
              created_at: issue.created_at,
              updated_at: issue.updated_at,
            })),
          );
          const currentCount = items.length;
          if (currentCount === 0) {
            break;
          }
          offset += currentCount;
          if (currentCount < PAGE_LIMIT) {
            break;
          }
        }
        allIssues.sort((left, right) => {
          const leftTime = new Date(left.updated_at ?? left.created_at ?? 0).getTime();
          const rightTime = new Date(right.updated_at ?? right.created_at ?? 0).getTime();
          if (Number.isNaN(leftTime) && Number.isNaN(rightTime)) {
            return left.title.localeCompare(right.title);
          }
          if (Number.isNaN(leftTime)) {
            return 1;
          }
          if (Number.isNaN(rightTime)) {
            return -1;
          }
          return rightTime - leftTime;
        });
        if (!cancelled) {
          setTasks(allIssues);
          setSelectedTaskId((current) =>
            current && allIssues.some((task) => task.id === current)
              ? current
              : allIssues[0]?.id ?? null,
          );
          hasLoadedTasksRef.current = true;
        }
      } catch (requestError) {
        if (!cancelled) {
          setTasks([]);
          setSelectedTaskId(null);
          setError(getErrorMessage(requestError));
        }
      } finally {
        if (!cancelled) {
          if (isBackground) {
            setBackgroundRefreshing(false);
          } else {
            setLoading(false);
          }
        }
        inFlight = false;
      }
    };

    void loadTasks();
    const intervalId = autoRefreshEnabled
      ? setInterval(() => {
          void loadTasks();
        }, autoRefreshIntervalMs)
      : null;
    return () => {
      cancelled = true;
      if (intervalId !== null) {
        clearInterval(intervalId);
      }
    };
  }, [apiClient, projectId, refreshToken, autoRefreshEnabled, autoRefreshIntervalMs, manualReloadToken]);

  useEffect(() => {
    const timelineIssueID = selectedTaskId ?? "";
    if (!timelineIssueID) {
      setTimelineLoading(false);
      setTimelineError(null);
      setTimelineEntries([]);
      hasLoadedTimelineRef.current = false;
      timelineIssueRef.current = "";
      return;
    }

    const issueChanged = timelineIssueRef.current !== timelineIssueID;
    if (issueChanged) {
      timelineIssueRef.current = timelineIssueID;
      hasLoadedTimelineRef.current = false;
    }
    const showTimelineLoading = !hasLoadedTimelineRef.current;
    if (showTimelineLoading) {
      setTimelineLoading(true);
    }

    let cancelled = false;
    setTimelineError(null);
    const loadTimeline = async () => {
      try {
        const response = await apiClient.listIssueTimeline(projectId, timelineIssueID, {
          limit: 200,
          offset: 0,
        });
        if (cancelled) {
          return;
        }
        const sorted = [...response.items].sort((a, b) => {
          const left = new Date(parseTimelineTimestamp(a)).getTime();
          const right = new Date(parseTimelineTimestamp(b)).getTime();
          if (Number.isNaN(left) && Number.isNaN(right)) {
            return 0;
          }
          if (Number.isNaN(left)) {
            return 1;
          }
          if (Number.isNaN(right)) {
            return -1;
          }
          return right - left;
        });
        setTimelineEntries(sorted);
        hasLoadedTimelineRef.current = true;
      } catch (requestError) {
        if (!cancelled) {
          setTimelineError(getErrorMessage(requestError));
          setTimelineEntries([]);
          hasLoadedTimelineRef.current = false;
        }
      } finally {
        if (!cancelled && showTimelineLoading) {
          setTimelineLoading(false);
        }
      }
    };
    void loadTimeline();
    return () => {
      cancelled = true;
    };
  }, [apiClient, projectId, selectedTaskId, tasks, refreshToken, timelineReloadToken]);

  const groupedTasks = useMemo(() => groupBoardTasks(tasks), [tasks]);
  const visibleTasks = useMemo(() => {
    if (statusFilter === "all") {
      return tasks;
    }
    return groupedTasks[statusFilter];
  }, [groupedTasks, statusFilter, tasks]);
  const selectedTask = selectedTaskId
    ? tasks.find((task) => task.id === selectedTaskId) ?? null
    : null;
  const activeIssueTaskID = resolveRouteIssueTaskID(routeIssueID, tasks);
  const detailTask = activeIssueTaskID
    ? tasks.find((task) => task.id === activeIssueTaskID) ?? selectedTask
    : null;
  const isIssueDetailOpen = routeIssueID !== null;
  const timelineItems = useMemo(
    () => (detailTask ? buildTimeline(detailTask, timelineEntries) : []),
    [detailTask, timelineEntries],
  );

  useEffect(() => {
    const routeTaskID = resolveRouteIssueTaskID(routeIssueID, tasks);
    if (routeTaskID) {
      setSelectedTaskId((current) => (current === routeTaskID ? current : routeTaskID));
      return;
    }
    if (routeIssueID && tasks.length > 0) {
      setRouteIssueID(null);
      writeRouteIssueID(null, "replace");
    }
  }, [routeIssueID, tasks]);

  const canRunAction = (
    task: BoardTask | null,
    action: TaskActionType,
  ): boolean => {
    if (!task || actionLoadingTaskId === task.id) {
      return false;
    }
    const rawStatus = task.raw_status.trim().toLowerCase();
    if (action === "submit_review") {
      return rawStatus === "draft" || task.status === "pending";
    }
    if (action === "approve") {
      return (
        rawStatus === "reviewing" ||
        rawStatus === "waiting_review" ||
        task.status === "ready" ||
        task.status === "running"
      );
    }
    if (action === "abort") {
      return task.status !== "done";
    }
    return false;
  };

  const getNextActionHint = (task: BoardTask | null): string => {
    if (!task) {
      return "选择一个 issue 后可查看推荐下一步操作。";
    }
    switch (task.status) {
      case "pending":
        return "建议先执行 Submit review，让 issue 进入审核链路。";
      case "ready":
        return "建议执行 Approve，推进到 Run 执行阶段。";
      case "running":
        return "建议观察 timeline 输出；若方向错误可使用 Abandon。";
      case "done":
        return "当前已完成，可在 timeline 中复核每一步执行记录。";
      case "failed":
        return "建议先定位失败节点，再决定重新 review 还是 Abandon。";
      default:
        return "请结合当前状态选择下一步动作。";
    }
  };

  const runTaskAction = async (task: BoardTask, action: TaskActionType) => {
    setActionLoadingTaskId(task.id);
    setActionError(null);
    setActionNotice(null);
    try {
      let responseStatus = task.raw_status;
      if (action === "submit_review") {
        const response = await apiClient.submitIssueReview(projectId, task.id);
        responseStatus = String(response.status ?? "");
      } else if (action === "approve") {
        const response = await apiClient.applyIssueAction(projectId, task.id, {
          action: "approve",
        });
        responseStatus = String(response.status ?? "");
      } else {
        const response = await apiClient.applyIssueAction(projectId, task.id, {
          action: "abort",
        });
        responseStatus = String(response.status ?? "");
      }

      const nextStatus = toBoardStatus(responseStatus);
      setTasks((current) =>
        current.map((item) =>
          item.id === task.id
            ? {
                ...item,
                status: nextStatus,
                raw_status: responseStatus,
                updated_at: new Date().toISOString(),
              }
            : item,
        ),
      );
      setSelectedTaskId(task.id);
      setTimelineReloadToken((current) => current + 1);
      setActionNotice(
        `Issue ${task.title} 已执行 ${TASK_ACTION_CONFIG[action].label}，当前状态：${nextStatus}`,
      );
    } catch (requestError) {
      setActionError(getErrorMessage(requestError));
    } finally {
      setActionLoadingTaskId(null);
    }
  };

  const openIssueDetail = (task: BoardTask) => {
    setSelectedTaskId(task.id);
    setRouteIssueID(task.id);
    writeRouteIssueID(task.id);
  };

  const closeIssueDetail = () => {
    setRouteIssueID(null);
    writeRouteIssueID(null);
  };

  return (
    <section className="flex flex-col gap-4">
      <header className="rounded-md border border-[#d0d7de] bg-white p-4">
        <h1 className="text-xl font-semibold text-[#24292f]">Issues</h1>
        <div className="mt-3 flex flex-wrap items-center gap-2 text-xs">
          <button
            type="button"
            className="rounded-md border border-[#d0d7de] bg-[#f6f8fa] px-2 py-1 text-[#24292f] hover:bg-white disabled:cursor-not-allowed disabled:opacity-60"
            disabled={loading || backgroundRefreshing}
            onClick={() => {
              setManualReloadToken((current) => current + 1);
            }}
          >
            立即刷新
          </button>
          <label className="inline-flex items-center gap-2 rounded-md border border-[#d0d7de] bg-[#f6f8fa] px-2 py-1 text-[#24292f]">
            <input
              type="checkbox"
              checked={autoRefreshEnabled}
              onChange={(event) => {
                setAutoRefreshEnabled(event.target.checked);
              }}
            />
            <span>自动静默刷新</span>
          </label>
          <label className="inline-flex items-center gap-2 rounded-md border border-[#d0d7de] bg-[#f6f8fa] px-2 py-1 text-[#24292f]">
            <span>刷新间隔</span>
            <select
              value={String(autoRefreshIntervalMs)}
              disabled={!autoRefreshEnabled}
              className="rounded border border-[#d0d7de] bg-white px-1 py-0.5 text-xs text-[#24292f] disabled:cursor-not-allowed disabled:bg-[#f6f8fa]"
              onChange={(event) => {
                const nextValue = Number.parseInt(event.target.value, 10);
                if (Number.isFinite(nextValue) && nextValue > 0) {
                  setAutoRefreshIntervalMs(nextValue);
                }
              }}
            >
              {AUTO_REFRESH_INTERVAL_OPTIONS.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </label>
          {backgroundRefreshing ? (
            <span className="text-[#57606a]">后台刷新中...</span>
          ) : null}
        </div>
      </header>

      {error ? (
        <p className="rounded-md border border-[#cf222e] bg-[#ffebe9] px-3 py-2 text-sm text-[#cf222e]">
          {error}
        </p>
      ) : null}

      {isIssueDetailOpen ? (
        <section className="rounded-md border border-[#d0d7de] bg-white">
          {!detailTask ? (
            <div className="p-6">
              <p className="text-sm text-[#57606a]">当前 issue 不存在或已被删除。</p>
              <button
                type="button"
                className="mt-3 rounded-md border border-[#d0d7de] bg-[#f6f8fa] px-3 py-1.5 text-xs text-[#24292f] hover:bg-white"
                onClick={closeIssueDetail}
              >
                返回 Issues 列表
              </button>
            </div>
          ) : (
            <>
              <header className="border-b border-[#d8dee4] p-4">
                <button
                  type="button"
                  className="rounded-md border border-[#d0d7de] bg-[#f6f8fa] px-2 py-1 text-xs text-[#24292f] hover:bg-white"
                  onClick={closeIssueDetail}
                >
                  ← 返回 Issues
                </button>
                <div className="mt-3 space-y-1 text-xs text-[#57606a]">
                  <p className="text-xl font-semibold text-[#24292f]">{detailTask.title}</p>
                  <p className="flex flex-wrap items-center gap-2">
                    <span
                      className={`rounded-full border px-2 py-0.5 text-[11px] font-medium ${STATUS_BADGE_CLASS[detailTask.status]}`}
                    >
                      {BOARD_STATUS_LABELS[detailTask.status]}
                    </span>
                    <span>{detailTask.raw_status || detailTask.status}</span>
                    <span>{getExecutor(detailTask)}</span>
                    <span>{formatTimestamp(detailTask.updated_at ?? detailTask.created_at)}</span>
                  </p>
                  <p className="pt-1 text-[#24292f]">{getNextActionHint(detailTask)}</p>
                </div>
              </header>

              <div className="space-y-3 p-4">
                <section className="rounded-md border border-[#d0d7de] bg-[#f6f8fa] p-3">
                  <p className="text-xs font-semibold text-[#24292f]">Next step actions</p>
                  <div className="mt-2 flex flex-wrap gap-2">
                    {(["submit_review", "approve", "abort"] as TaskActionType[]).map((action) => (
                      <button
                        key={action}
                        type="button"
                        disabled={!canRunAction(detailTask, action)}
                        className={`rounded-md border px-3 py-1.5 text-xs font-medium transition ${
                          action === "approve"
                            ? "border-[#1a7f37] bg-[#dafbe1] text-[#1a7f37] enabled:hover:bg-[#c4f1cf]"
                            : action === "abort"
                              ? "border-[#cf222e] bg-[#ffebe9] text-[#cf222e] enabled:hover:bg-[#ffd8d3]"
                              : "border-[#0969da] bg-[#ddf4ff] text-[#0969da] enabled:hover:bg-[#c2e9ff]"
                        } disabled:cursor-not-allowed disabled:opacity-50`}
                        onClick={() => {
                          void runTaskAction(detailTask, action);
                        }}
                      >
                        {TASK_ACTION_CONFIG[action].label}
                      </button>
                    ))}
                  </div>
                  <p className="mt-2 text-[11px] text-[#57606a]">
                    {TASK_ACTION_CONFIG.submit_review.description} /{" "}
                    {TASK_ACTION_CONFIG.approve.description} /{" "}
                    {TASK_ACTION_CONFIG.abort.description}
                  </p>
                </section>

                {actionNotice ? (
                  <p className="rounded-md border border-[#1a7f37] bg-[#dafbe1] px-2 py-1 text-xs text-[#1a7f37]">
                    {actionNotice}
                  </p>
                ) : null}
                {actionError ? (
                  <p className="rounded-md border border-[#cf222e] bg-[#ffebe9] px-2 py-1 text-xs text-[#cf222e]">
                    {actionError}
                  </p>
                ) : null}
                {timelineError ? (
                  <p className="rounded-md border border-[#cf222e] bg-[#ffebe9] px-2 py-1 text-xs text-[#cf222e]">
                    {timelineError}
                  </p>
                ) : null}

                {timelineLoading && timelineItems.length === 0 ? (
                  <p className="text-xs text-[#57606a]">timeline 加载中...</p>
                ) : timelineItems.length === 0 ? (
                  <p className="text-xs text-[#57606a]">暂无 timeline 记录。</p>
                ) : (
                  <ol className="space-y-3">
                    {timelineItems.map((item) => {
                      const hasDetail = item.resultDetail.trim().length > 0;
                      return (
                        <li
                          key={item.id}
                          className={`rounded-md border p-3 ${TIMELINE_TONE_CLASS[item.tone]}`}
                        >
                          <div className="flex items-start gap-3">
                            <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full border border-[#d0d7de] bg-white text-[10px] font-semibold text-[#57606a]">
                              {getAvatarPlaceholder(item.executor)}
                            </span>
                            <div className="min-w-0 flex-1">
                              <div className="flex flex-wrap items-center justify-between gap-2">
                                <p className="text-sm font-semibold text-[#24292f]">{item.title}</p>
                                <time className="text-[11px] opacity-80">
                                  {formatTimestamp(item.timestamp)}
                                </time>
                              </div>
                              <p className="mt-1 text-xs text-[#57606a]">{item.executor}</p>
                              {hasDetail ? (
                                <pre className="mt-2 max-h-[360px] overflow-auto rounded-md border border-[#d0d7de] bg-[#f6f8fa] p-3 text-xs text-[#24292f]">
                                  {item.resultDetail}
                                </pre>
                              ) : null}
                            </div>
                          </div>
                        </li>
                      );
                    })}
                  </ol>
                )}
              </div>
            </>
          )}
        </section>
      ) : (
        <section className="overflow-hidden rounded-md border border-[#d0d7de] bg-white">
          <header className="border-b border-[#d8dee4] bg-[#f6f8fa] px-4 py-3">
            <div className="flex flex-wrap items-center gap-2">
              {BOARD_FILTERS.map((filterKey) => {
                const count =
                  filterKey === "all" ? tasks.length : groupedTasks[filterKey].length;
                const active = filterKey === statusFilter;
                return (
                  <button
                    key={filterKey}
                    type="button"
                    className={`inline-flex items-center gap-1 rounded-md border px-2 py-1 text-xs font-medium transition ${
                      active
                        ? "border-[#0969da] bg-white text-[#0969da]"
                        : "border-transparent text-[#57606a] hover:border-[#d0d7de] hover:bg-white"
                    }`}
                    onClick={() => {
                      setStatusFilter(filterKey);
                    }}
                  >
                    <span>{BOARD_FILTER_LABELS[filterKey]}</span>
                    <span className="rounded-full bg-[#eaeef2] px-1.5 py-0.5 text-[11px] text-[#57606a]">
                      {count}
                    </span>
                  </button>
                );
              })}
            </div>
            <p className="mt-2 text-xs text-[#57606a]">
              点击 issue 进入详情页（全宽展示 timeline 与操作面板）。
            </p>
          </header>

          {loading && tasks.length === 0 ? (
            <p className="px-4 py-6 text-sm text-[#57606a]">加载中...</p>
          ) : visibleTasks.length === 0 ? (
            <p className="px-4 py-6 text-sm text-[#57606a]">暂无 issue</p>
          ) : (
            <ul className="divide-y divide-[#d8dee4]">
              {visibleTasks.map((task) => (
                <li key={task.id}>
                  <button
                    type="button"
                    data-testid="board-task"
                    className={`w-full bg-white px-4 py-3 text-left transition hover:bg-[#f6f8fa] ${
                      actionLoadingTaskId === task.id ? "opacity-60" : ""
                    }`}
                    onClick={() => {
                      openIssueDetail(task);
                    }}
                  >
                    <div className="flex items-start gap-3">
                      <span
                        className={`mt-1 inline-flex h-4 w-4 shrink-0 rounded-full border ${
                          task.status === "failed"
                            ? "border-[#cf222e] bg-[#cf222e]"
                            : task.status === "done"
                              ? "border-[#1a7f37] bg-[#1a7f37]"
                              : task.status === "running"
                                ? "border-[#9a6700] bg-[#fff8c5]"
                                : task.status === "ready"
                                  ? "border-[#0969da] bg-[#0969da]"
                                  : "border-[#57606a] bg-[#57606a]"
                        }`}
                      />
                      <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="truncate text-[15px] font-semibold text-[#0969da]">
                            {task.title}
                          </span>
                          <span
                            className={`rounded-full border px-2 py-0.5 text-[11px] font-medium ${STATUS_BADGE_CLASS[task.status]}`}
                          >
                            {BOARD_STATUS_LABELS[task.status]}
                          </span>
                          {task.raw_status === "blocked_by_failure" ? (
                            <span className="rounded-full border border-[#cf222e] bg-[#ffebe9] px-2 py-0.5 text-[11px] text-[#cf222e]">
                              blocked
                            </span>
                          ) : null}
                          {task.raw_status === "skipped" ? (
                            <span className="rounded-full border border-[#d0d7de] bg-[#f6f8fa] px-2 py-0.5 text-[11px] text-[#57606a]">
                              skipped
                            </span>
                          ) : null}
                        </div>
                        <p className="mt-1 text-xs text-[#57606a]">
                          {getIssueNumberLabel(task)} opened on{" "}
                          {formatRelativeTimestamp(task.created_at)} by team leader
                        </p>
                      </div>
                    </div>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </section>
      )}
    </section>
  );
};

export default BoardView;
