import { useEffect, useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import type { ApiClient } from "@/lib/apiClient";
import type { ApiIssue, ApiRun, ApiStatsResponse } from "@/types/api";
import type { Project } from "@/types/workflow";

interface CommandCenterViewProps {
  apiClient: ApiClient;
  projectId: string;
  projects: Project[];
  selectedProject: Project | null;
  refreshToken: number;
  onNavigate: (view: "chat" | "board" | "runs" | "ops") => void;
}

interface CommandCenterState {
  stats: ApiStatsResponse | null;
  issues: ApiIssue[];
  runs: ApiRun[];
}

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
  return parsed.toLocaleString("zh-CN", { hour12: false });
};

const formatPercent = (value?: number): string => {
  if (typeof value !== "number" || Number.isNaN(value)) {
    return "--";
  }
  return `${Math.round(value * 100)}%`;
};

const toIssueTone = (status: string): "secondary" | "warning" | "success" | "danger" => {
  switch (status.trim().toLowerCase()) {
    case "reviewing":
    case "queued":
    case "ready":
      return "warning";
    case "done":
      return "success";
    case "failed":
    case "abandoned":
    case "superseded":
      return "danger";
    default:
      return "secondary";
  }
};

const toRunTone = (status: string): "secondary" | "warning" | "success" | "danger" => {
  switch (status.trim().toLowerCase()) {
    case "in_progress":
    case "action_required":
      return "warning";
    case "completed":
      return "success";
    default:
      return "secondary";
  }
};

const sumTokens = (stats: ApiStatsResponse | null): number => {
  if (!stats) {
    return 0;
  }
  return (stats.tokens_used?.claude ?? 0) + (stats.tokens_used?.codex ?? 0);
};

const CommandCenterView = ({
  apiClient,
  projectId,
  projects,
  selectedProject,
  refreshToken,
  onNavigate,
}: CommandCenterViewProps) => {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [state, setState] = useState<CommandCenterState>({
    stats: null,
    issues: [],
    runs: [],
  });

  useEffect(() => {
    if (!projectId.trim()) {
      setLoading(false);
      setError(null);
      setState({
        stats: null,
        issues: [],
        runs: [],
      });
      return;
    }

    let cancelled = false;
    const load = async () => {
      setLoading(true);
      setError(null);
      const [statsResult, issuesResult, runsResult] = await Promise.allSettled([
        apiClient.getStats(),
        apiClient.listIssues(projectId, { limit: 12, offset: 0 }),
        apiClient.listRuns(projectId, { limit: 8, offset: 0 }),
      ]);

      if (cancelled) {
        return;
      }

      const nextStats = statsResult.status === "fulfilled" ? statsResult.value : null;
      const nextIssues =
        issuesResult.status === "fulfilled" && Array.isArray(issuesResult.value.items)
          ? issuesResult.value.items
          : [];
      const nextRuns =
        runsResult.status === "fulfilled" && Array.isArray(runsResult.value.items)
          ? runsResult.value.items
          : [];

      setState({
        stats: nextStats,
        issues: nextIssues,
        runs: nextRuns,
      });

      const messages = [statsResult, issuesResult, runsResult]
        .filter((result): result is PromiseRejectedResult => result.status === "rejected")
        .map((result) => getErrorMessage(result.reason));
      setError(messages.length > 0 ? messages[0] : null);
      setLoading(false);
    };

    void load();
    return () => {
      cancelled = true;
    };
  }, [apiClient, projectId, refreshToken]);

  const issueStats = useMemo(() => {
    return state.issues.reduce(
      (summary, issue) => {
        const status = String(issue.status ?? "").trim().toLowerCase();
        if (status === "reviewing" || status === "queued" || status === "ready") {
          summary.ready += 1;
        } else if (status === "executing" || status === "merging") {
          summary.running += 1;
        } else if (status === "done") {
          summary.done += 1;
        } else if (status === "failed" || status === "abandoned" || status === "superseded") {
          summary.failed += 1;
        } else {
          summary.pending += 1;
        }
        return summary;
      },
      { pending: 0, ready: 0, running: 0, done: 0, failed: 0 },
    );
  }, [state.issues]);

  const activeRuns = useMemo(
    () => state.runs.filter((run) => String(run.status ?? "") !== "completed"),
    [state.runs],
  );

  const latestIssue = state.issues[0] ?? null;
  const latestRun = state.runs[0] ?? null;

  if (!projectId.trim()) {
    return (
      <section className="grid gap-6">
        <Card>
          <CardHeader>
            <CardTitle>Command Center</CardTitle>
            <CardDescription>先创建项目，再进入 v3 工作台。</CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-slate-600">
              当前还没有可用项目。创建项目后，这里会展示 Issue、Run 和整体健康状态。
            </p>
          </CardContent>
        </Card>
      </section>
    );
  }

  return (
    <section className="grid gap-4">
      <Card className="rounded-2xl border-slate-200 shadow-none">
        <CardHeader className="gap-4 p-5 md:flex-row md:items-start md:justify-between">
          <div className="max-w-3xl space-y-3">
            <div className="flex items-center gap-2">
              <Badge variant="secondary" className="bg-indigo-50 text-indigo-600">
                命令中心
              </Badge>
              <span className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">总览 / 指挥台</span>
            </div>
            <CardTitle className="text-[28px] font-semibold tracking-[-0.02em] text-slate-950">
              今天要处理的不是所有事情，而是最关键的 8 件事
            </CardTitle>
            <CardDescription className="max-w-2xl text-sm leading-6 text-slate-500">
              将项目、Issue、Run 和协议健康放到同一视角。高频动作直接可点，低频诊断只保留入口。
            </CardDescription>
          </div>
          <div className="grid min-w-[280px] gap-3 rounded-2xl border border-slate-200 bg-slate-50 p-4">
            <div>
              <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">当前项目</p>
              <p className="mt-2 text-lg font-semibold text-slate-950">{selectedProject?.name ?? projectId}</p>
            </div>
            <div className="space-y-1 text-xs text-slate-500">
              <p>Repo: {selectedProject?.repo_path ?? "--"}</p>
              <p>项目总数: {projects.length}</p>
              <p>活动 Run: {activeRuns.length}</p>
            </div>
          </div>
        </CardHeader>
        <CardContent className="grid gap-3 px-5 pb-5 md:grid-cols-3">
          <Button
            variant="outline"
            className="justify-start rounded-xl border-slate-200 bg-slate-50 text-slate-800"
            onClick={() => onNavigate("board")}
          >
            查看所有 Issue
          </Button>
          <Button
            variant="ghost"
            className="justify-start rounded-xl border border-slate-200 bg-white text-slate-700 hover:bg-slate-50"
            onClick={() => onNavigate("chat")}
          >
            进入会话工作区
          </Button>
          <Button
            variant="ghost"
            className="justify-start rounded-xl border border-slate-200 bg-white text-slate-700 hover:bg-slate-50"
            onClick={() => onNavigate("runs")}
          >
            打开 Run 事件流
          </Button>
        </CardContent>
      </Card>

      <div className="grid gap-4 xl:grid-cols-[1.24fr_0.76fr]">
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
          <Card className="rounded-2xl shadow-none">
            <CardHeader className="pb-3">
              <CardDescription>Active Runs</CardDescription>
              <CardTitle className="text-3xl">{state.stats?.active_Runs ?? activeRuns.length}</CardTitle>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-slate-500">正在进行或等待人工动作的流程数量。</p>
            </CardContent>
          </Card>
          <Card className="rounded-2xl shadow-none">
            <CardHeader className="pb-3">
              <CardDescription>Ready / Review</CardDescription>
              <CardTitle className="text-3xl">{issueStats.ready}</CardTitle>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-slate-500">已进入 review 或 ready 阶段，适合人工确认。</p>
            </CardContent>
          </Card>
          <Card className="rounded-2xl shadow-none">
            <CardHeader className="pb-3">
              <CardDescription>Success Rate</CardDescription>
              <CardTitle className="text-3xl">{formatPercent(state.stats?.success_rate)}</CardTitle>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-slate-500">后端统计接口返回的整体成功率。</p>
            </CardContent>
          </Card>
          <Card className="rounded-2xl shadow-none">
            <CardHeader className="pb-3">
              <CardDescription>Tokens Used</CardDescription>
              <CardTitle className="text-3xl">{sumTokens(state.stats).toLocaleString("en-US")}</CardTitle>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-slate-500">Claude 与 Codex 的累计 token 用量。</p>
            </CardContent>
          </Card>
        </div>

        <Card className="rounded-2xl border-slate-200 shadow-none">
          <CardHeader>
            <CardTitle>值班建议</CardTitle>
            <CardDescription>
              先处理 run 阻塞，再处理待审 Issue。首页只显示提醒，不直接暴露 force ready / unblock / replay。
            </CardDescription>
          </CardHeader>
          <CardContent className="grid gap-3 text-sm">
            <div className="rounded-2xl border border-slate-200 bg-emerald-50/70 p-4">
              <p className="font-medium text-slate-900">Run 健康</p>
              <p className="mt-1 text-slate-600">
                {latestRun
                  ? `${latestRun.id} 最近更新时间 ${formatTimestamp(
                      latestRun.updated_at ?? latestRun.created_at,
                    )}`
                  : "当前没有可展示的 Run 记录。"}
              </p>
            </div>
            <div className="rounded-2xl border border-slate-200 bg-indigo-50/70 p-4">
              <p className="font-medium text-slate-900">Issue 焦点</p>
              <p className="mt-1 text-slate-600">
                {latestIssue
                  ? `${latestIssue.title || latestIssue.id} 目前处于 ${latestIssue.status} 阶段。`
                  : "当前没有可展示的 Issue 记录。"}
              </p>
            </div>
            <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
              <p className="font-medium text-slate-900">兼容信息</p>
              <p className="mt-1 text-slate-600">v3 视图层优先解决认知负担，不改变现有后端动作接口。</p>
            </div>
          </CardContent>
        </Card>
      </div>

      <div className="grid gap-4 xl:grid-cols-[1.05fr_1.15fr_0.8fr]">
        <Card className="rounded-2xl shadow-none">
          <CardHeader>
            <CardTitle>待处理 Issue</CardTitle>
            <CardDescription>一句话需求已拆成任务后，这里应成为确认和推进入口。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {loading ? (
              <p className="text-sm text-slate-500">加载中...</p>
            ) : state.issues.length === 0 ? (
              <p className="text-sm text-slate-500">暂无 Issue。</p>
            ) : (
              state.issues.slice(0, 5).map((issue) => (
                <button
                  key={issue.id}
                  type="button"
                  className="flex w-full items-start justify-between gap-3 rounded-xl border border-slate-200 px-4 py-3 text-left transition hover:border-slate-300 hover:bg-slate-50"
                  onClick={() => onNavigate("board")}
                >
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium text-slate-900">{issue.title || issue.id}</p>
                    <p className="mt-1 text-xs text-slate-500">
                      {issue.github?.issue_number ? `#${issue.github.issue_number}` : issue.id} ·{" "}
                      {formatTimestamp(issue.updated_at ?? issue.created_at)}
                    </p>
                  </div>
                  <Badge variant={toIssueTone(String(issue.status ?? ""))}>{issue.status}</Badge>
                </button>
              ))
            )}
          </CardContent>
        </Card>

        <Card className="rounded-2xl shadow-none">
          <CardHeader>
            <CardTitle>运行与反馈</CardTitle>
            <CardDescription>把 Run、Issue、会话和协议健康放到同一视角，高频动作直接可点。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {loading ? (
              <p className="text-sm text-slate-500">加载中...</p>
            ) : state.runs.length === 0 ? (
              <p className="text-sm text-slate-500">暂无 Run 记录。</p>
            ) : (
              state.runs.slice(0, 5).map((run) => (
                <div key={run.id} className="rounded-xl border border-slate-200 p-4">
                  <div className="flex items-center justify-between gap-3">
                    <div>
                      <p className="text-sm font-medium text-slate-900">{run.id}</p>
                      <p className="mt-1 text-xs text-slate-500">
                        profile: {run.profile} · {formatTimestamp(run.updated_at ?? run.created_at)}
                      </p>
                    </div>
                    <Badge variant={toRunTone(String(run.status ?? ""))}>{run.status}</Badge>
                  </div>
                  <div className="mt-3 flex flex-wrap gap-2 text-xs text-slate-500">
                    <span>run: {run.id}</span>
                    <span>issue: {run.issue_id || "--"}</span>
                  </div>
                </div>
              ))
            )}
          </CardContent>
        </Card>

        <Card className="rounded-2xl shadow-none">
          <CardHeader>
            <CardTitle>低频诊断入口</CardTitle>
            <CardDescription>仅在排查问题时展开，避免首页成为杂讯集合。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
              <p className="text-sm font-medium text-slate-900">Issue 详情</p>
              <p className="mt-1 text-xs leading-5 text-slate-500">
                建议同时查看 Issue 详情、TaskStep 时间线和会话事件组。
              </p>
            </div>
            <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
              <p className="text-sm font-medium text-slate-900">协议兼容</p>
              <p className="mt-1 text-xs leading-5 text-slate-500">
                V1 / V3 共存；v3 负责认知模型和任务视图，动作接口保持不变。
              </p>
            </div>
            <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
              <p className="text-sm font-medium text-slate-900">接口状态</p>
              <p className="mt-1 text-xs leading-5 text-slate-500">
                {error ? `存在部分接口加载失败：${error}` : "运行、Issue 和统计接口已接入 v3 首页。"}
              </p>
            </div>
            <Button
              variant="outline"
              className="w-full justify-start rounded-2xl"
              onClick={() => onNavigate("ops")}
            >
              打开协议 / 运维控制台
            </Button>
          </CardContent>
        </Card>
      </div>
    </section>
  );
};

export default CommandCenterView;
