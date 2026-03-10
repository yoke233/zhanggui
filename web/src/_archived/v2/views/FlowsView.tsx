import { useEffect, useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import type { ApiClientV2 } from "@/lib/apiClientV2";
import { PageScaffold } from "@/v3/components/PageScaffold";
import type { Flow, Project } from "@/types/apiV2";

interface FlowsViewProps {
  apiClient: ApiClientV2;
  projectId: number | null;
  project: Project | null;
  selectedFlowId: number | null;
  refreshToken: number;
  onSelectFlow: (flowId: number) => void;
}

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error && error.message.trim().length > 0) {
    return error.message;
  }
  return "请求失败，请稍后重试";
};

const formatTime = (value?: string) => {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString("zh-CN", { hour12: false });
};

const statusTone = (status: string): "secondary" | "warning" | "success" | "danger" => {
  switch (status.trim().toLowerCase()) {
    case "running":
      return "warning";
    case "done":
      return "success";
    case "failed":
    case "blocked":
    case "cancelled":
      return "danger";
    default:
      return "secondary";
  }
};

const FlowsView = ({
  apiClient,
  projectId,
  project,
  selectedFlowId,
  refreshToken,
  onSelectFlow,
}: FlowsViewProps) => {
  const [flows, setFlows] = useState<Flow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [newFlowName, setNewFlowName] = useState("");
  const [createFeedback, setCreateFeedback] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        const listed = await apiClient.listFlows({
          project_id: projectId ?? undefined,
          limit: 200,
          offset: 0,
        });
        if (cancelled) {
          return;
        }
        setFlows(listed);
      } catch (err) {
        if (cancelled) {
          return;
        }
        setError(getErrorMessage(err));
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
  }, [apiClient, refreshToken]);

  const selectedFlow = useMemo(
    () => flows.find((flow) => flow.id === selectedFlowId) ?? null,
    [flows, selectedFlowId],
  );

  const handleCreate = async () => {
    const name = newFlowName.trim();
    if (!name) {
      setCreateFeedback("Flow 名称不能为空。");
      return;
    }
    if (!projectId) {
      setCreateFeedback("请先选择一个项目。");
      return;
    }
    setCreating(true);
    setCreateFeedback(null);
    try {
      const created = await apiClient.createFlow({ name, project_id: projectId });
      setNewFlowName("");
      setCreateFeedback(`已创建 Flow #${created.id}`);
      onSelectFlow(created.id);
      const listed = await apiClient.listFlows({
        project_id: projectId,
        limit: 200,
        offset: 0,
      });
      setFlows(listed);
    } catch (err) {
      setCreateFeedback(getErrorMessage(err));
    } finally {
      setCreating(false);
    }
  };

  const handleRunSelected = async () => {
    if (!selectedFlow) {
      return;
    }
    try {
      await apiClient.runFlow(selectedFlow.id);
      const listed = await apiClient.listFlows({
        project_id: projectId ?? undefined,
        limit: 200,
        offset: 0,
      });
      setFlows(listed);
    } catch (err) {
      setError(getErrorMessage(err));
    }
  };

  const handleCancelSelected = async () => {
    if (!selectedFlow) {
      return;
    }
    try {
      await apiClient.cancelFlow(selectedFlow.id);
      const listed = await apiClient.listFlows({
        project_id: projectId ?? undefined,
        limit: 200,
        offset: 0,
      });
      setFlows(listed);
    } catch (err) {
      setError(getErrorMessage(err));
    }
  };

  return (
    <PageScaffold
      eyebrow="Flow / 编排"
      title="Flow 列表"
      description="v2 将“项目/Issue/Run”收敛为 Flow/Step/Execution。先选择一个 Flow，再进入 Step / Execution / 事件流。"
      contextTitle={project ? `项目：${project.name}` : "项目：未选择"}
      contextMeta={
        project
          ? `kind=${project.kind}${project.description ? ` · ${project.description}` : ""}`
          : "请先选择项目或在 Ops 创建项目"
      }
      actions={
        selectedFlow
          ? [
              { label: "运行", onClick: () => void handleRunSelected(), variant: "outline" },
              { label: "取消", onClick: () => void handleCancelSelected(), variant: "outline" },
            ]
          : undefined
      }
    >
      <Card className="rounded-2xl border-slate-200 shadow-none">
        <CardHeader className="p-5">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div>
              <Badge variant="secondary" className="bg-indigo-50 text-indigo-600">
                V2 / Flows
              </Badge>
              <CardTitle className="mt-3 text-[18px] font-semibold tracking-[-0.02em]">
                创建与选择
              </CardTitle>
              <CardDescription className="mt-2 text-slate-600">
                Flow 归属在项目下；列表会按当前项目过滤。
              </CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-4 px-5 pb-5">
          <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-end">
            <div className="grid gap-1">
              <label
                htmlFor="v2-flow-name"
                className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400"
              >
                新建 Flow
              </label>
              <Input
                id="v2-flow-name"
                value={newFlowName}
                onChange={(event) => setNewFlowName(event.target.value)}
                placeholder="例如：upgrade-to-v2"
              />
            </div>
            <Button onClick={() => void handleCreate()} disabled={creating || !projectId}>
              {creating ? "创建中..." : "创建"}
            </Button>
          </div>
          {createFeedback ? <p className="text-sm text-slate-600">{createFeedback}</p> : null}
          {error ? <p className="text-sm text-red-600">{error}</p> : null}

          <div className="grid gap-3">
            {loading ? <p className="text-sm text-slate-500">加载中...</p> : null}
            {!loading && flows.length === 0 ? <p className="text-sm text-slate-500">暂无 Flow。</p> : null}
            {flows.map((flow) => (
              <button
                key={flow.id}
                type="button"
                onClick={() => onSelectFlow(flow.id)}
                className={[
                  "flex w-full items-start justify-between gap-3 rounded-2xl border px-4 py-3 text-left transition",
                  flow.id === selectedFlowId
                    ? "border-indigo-200 bg-indigo-50/40"
                    : "border-slate-200 bg-white hover:bg-slate-50",
                ].join(" ")}
              >
                <div>
                  <p className="text-sm font-semibold text-slate-900">
                    #{flow.id} · {flow.name}
                  </p>
                  <p className="mt-1 text-[11px] text-slate-500">
                    创建 {formatTime(flow.created_at)} · 更新 {formatTime(flow.updated_at)}
                  </p>
                </div>
                <Badge variant={statusTone(flow.status)}>{flow.status}</Badge>
              </button>
            ))}
          </div>
        </CardContent>
      </Card>
    </PageScaffold>
  );
};

export default FlowsView;
