import { useEffect, useMemo, useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import type { ApiClientV2 } from "@/lib/apiClientV2";
import type { Project, StatsResponse } from "@/types/apiV2";
import { PageScaffold } from "@/v3/components/PageScaffold";

interface OverviewViewProps {
  apiClient: ApiClientV2;
  projectId: number | null;
  projects: Project[];
  selectedProject: Project | null;
  refreshToken: number;
  onNavigate: (view: "chat" | "flows" | "steps" | "ops") => void;
}

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error && error.message.trim().length > 0) {
    return error.message;
  }
  return "请求失败，请稍后重试";
};

const OverviewView = ({
  apiClient,
  projectId,
  projects,
  selectedProject,
  refreshToken,
  onNavigate,
}: OverviewViewProps) => {
  const [stats, setStats] = useState<StatsResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!projectId) {
      return;
    }
    let cancelled = false;
    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        const next = await apiClient.getStats();
        if (!cancelled) {
          setStats(next);
        }
      } catch (err) {
        if (!cancelled) {
          setStats(null);
          setError(getErrorMessage(err));
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
  }, [apiClient, projectId, refreshToken]);

  const statsCards = useMemo(() => {
    if (!stats) {
      return [];
    }
    const successRate = typeof stats.success_rate === "number" ? `${Math.round(stats.success_rate * 100)}%` : "--";
    return [
      { label: "Total Flows", value: String(stats.total_flows ?? 0), helper: "当前项目范围内的 Flow 总数（后端可按 project_id 进一步细化）" },
      { label: "Active Flows", value: String(stats.active_flows ?? 0), helper: "running / queued / blocked 的活跃 Flow" },
      { label: "Success Rate", value: successRate, helper: "近似口径，后续可扩展为项目维度统计" },
      { label: "Avg Duration", value: String(stats.avg_duration ?? "-"), helper: "平均耗时（口径后续可按项目/时间窗细分）" },
    ];
  }, [stats]);

  if (!projectId) {
    return (
      <PageScaffold
        eyebrow="Command Center"
        title="总览指挥台"
        description="今天需要你处理的不是所有事，而是最关键的 8 件事。"
        contextTitle="尚未选择项目"
        contextMeta="先在运维页创建项目并绑定资源，再回到总览查看关键进展。"
        stats={[
          { label: "当前项目", value: "0", helper: "还没有可进入的工作区" },
          { label: "下一步", value: "Create", helper: "先创建项目，再进入 Flow / Chat / Events" },
          { label: "入口", value: "Ops", helper: "项目初始化和资源绑定入口收口到运维页" },
        ]}
        actions={[
          {
            label: "前往协议 / 运维",
            onClick: () => onNavigate("ops"),
          },
        ]}
      >
        <Card className="border-dashed border-slate-300 bg-white/80">
          <CardHeader>
            <CardTitle>当前没有可展示的业务总览</CardTitle>
            <CardDescription>
              v2 总览依赖项目上下文。请先在“协议 / 运维”页面创建项目并绑定 resources，然后刷新项目列表。
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-wrap items-center gap-3 text-sm text-slate-600">
            <span>已发现项目数：{projects.length}</span>
            <Button variant="outline" onClick={() => onNavigate("ops")}>
              打开项目创建入口
            </Button>
          </CardContent>
        </Card>
      </PageScaffold>
    );
  }

  return (
    <PageScaffold
      eyebrow="Command Center"
      title="总览指挥台"
      description="先看项目健康与关键指标，再进入 Flow 编排 / Chat / 事件流。"
      contextTitle={selectedProject ? `项目 ${selectedProject.name}` : `project_id=${projectId}`}
      contextMeta={selectedProject ? `kind=${selectedProject.kind}${selectedProject.description ? ` · ${selectedProject.description}` : ""}` : undefined}
      stats={statsCards}
      actions={[
        { label: "打开任务列表", onClick: () => onNavigate("flows") },
        { label: "打开 Lead Chat", onClick: () => onNavigate("chat"), variant: "outline" },
        { label: "打开 运维", onClick: () => onNavigate("ops"), variant: "outline" },
      ]}
    >
      <Card className="rounded-2xl border-slate-200 shadow-none">
        <CardHeader className="p-5">
          <CardTitle className="text-[18px] font-semibold tracking-[-0.02em]">快速入口</CardTitle>
          <CardDescription className="mt-1">
            先创建/选择 Flow，再进入 Steps/Executions/Events；Chat 用于沉淀目标与触发执行。
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3 px-5 pb-5 md:grid-cols-3">
          <button
            type="button"
            onClick={() => onNavigate("flows")}
            className="rounded-2xl border border-slate-200 bg-white p-4 text-left transition hover:bg-slate-50"
          >
            <p className="text-sm font-semibold text-slate-950">Flow / 编排</p>
            <p className="mt-1 text-xs leading-5 text-slate-500">创建与选择 Flow，进入 Steps / Executions。</p>
          </button>
          <button
            type="button"
            onClick={() => onNavigate("chat")}
            className="rounded-2xl border border-slate-200 bg-white p-4 text-left transition hover:bg-slate-50"
          >
            <p className="text-sm font-semibold text-slate-950">Lead Chat</p>
            <p className="mt-1 text-xs leading-5 text-slate-500">WebSocket 流式消息，支持 cancel / close。</p>
          </button>
          <button
            type="button"
            onClick={() => onNavigate("steps")}
            className="rounded-2xl border border-slate-200 bg-white p-4 text-left transition hover:bg-slate-50"
          >
            <p className="text-sm font-semibold text-slate-950">运行视图</p>
            <p className="mt-1 text-xs leading-5 text-slate-500">Steps / Executions / Events / Artifact / Briefing。</p>
          </button>
        </CardContent>
      </Card>

      {loading ? <p className="text-sm text-slate-500">加载统计中...</p> : null}
      {error ? <p className="rounded-xl border border-rose-200 bg-rose-50 p-3 text-sm text-rose-700">{error}</p> : null}
    </PageScaffold>
  );
};

export default OverviewView;
