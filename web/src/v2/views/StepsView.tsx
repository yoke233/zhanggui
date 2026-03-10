import { useEffect, useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import type { ApiClientV2 } from "@/lib/apiClientV2";
import type { Step, StepType } from "@/types/apiV2";
import { PageScaffold } from "@/v3/components/PageScaffold";

interface StepsViewProps {
  apiClient: ApiClientV2;
  flowId: number;
  selectedStepId: number | null;
  refreshToken: number;
  onSelectStep: (stepId: number) => void;
}

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error && error.message.trim().length > 0) {
    return error.message;
  }
  return "请求失败，请稍后重试";
};

const statusTone = (status: string): "secondary" | "warning" | "success" | "danger" => {
  switch (status.trim().toLowerCase()) {
    case "ready":
    case "waiting_gate":
      return "warning";
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

const StepsView = ({
  apiClient,
  flowId,
  selectedStepId,
  refreshToken,
  onSelectStep,
}: StepsViewProps) => {
  const [steps, setSteps] = useState<Step[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [creating, setCreating] = useState(false);
  const [newStepName, setNewStepName] = useState("");
  const [newStepType, setNewStepType] = useState<StepType>("exec");
  const [newStepDependsOn, setNewStepDependsOn] = useState("");
  const [createFeedback, setCreateFeedback] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        const listed = await apiClient.listSteps(flowId);
        if (cancelled) {
          return;
        }
        setSteps(listed);
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
  }, [apiClient, flowId, refreshToken]);

  const selectedStep = useMemo(
    () => steps.find((step) => step.id === selectedStepId) ?? null,
    [selectedStepId, steps],
  );

  const handleCreate = async () => {
    const name = newStepName.trim();
    if (!name) {
      setCreateFeedback("Step 名称不能为空。");
      return;
    }
    const dependsOn = newStepDependsOn
      .split(",")
      .map((raw) => raw.trim())
      .filter((raw) => raw.length > 0)
      .map((raw) => Number.parseInt(raw, 10))
      .filter((num) => Number.isFinite(num) && num > 0);

    setCreating(true);
    setCreateFeedback(null);
    try {
      const created = await apiClient.createStep(flowId, {
        name,
        type: newStepType === "gate" ? "gate" : newStepType === "composite" ? "composite" : "exec",
        depends_on: dependsOn.length > 0 ? dependsOn : undefined,
      });
      setNewStepName("");
      setNewStepDependsOn("");
      setCreateFeedback(`已创建 Step #${created.id}`);
      onSelectStep(created.id);
      const listed = await apiClient.listSteps(flowId);
      setSteps(listed);
    } catch (err) {
      setCreateFeedback(getErrorMessage(err));
    } finally {
      setCreating(false);
    }
  };

  return (
    <PageScaffold
      eyebrow="Steps / DAG"
      title={`Steps（Flow #${flowId}）`}
      description="Step 是 Flow DAG 的节点：exec 负责执行、gate 负责约束、composite 可挂子 Flow。"
      contextTitle={selectedStep ? `当前 Step #${selectedStep.id}` : "当前 Step：未选择"}
      contextMeta={selectedStep ? `status: ${selectedStep.status}` : "请选择一个 Step 进入 Execution / Artifact / Briefing"}
    >
      <Card className="rounded-2xl border-slate-200 shadow-none">
        <CardContent className="space-y-4 px-5 pb-5">
          <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,220px)_auto] md:items-end">
            <div className="grid gap-1">
              <label htmlFor="v2-step-name" className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
                新建 Step
              </label>
              <Input
                id="v2-step-name"
                value={newStepName}
                onChange={(event) => setNewStepName(event.target.value)}
                placeholder="例如：implement-api-client"
              />
            </div>
            <div className="grid gap-1">
              <label htmlFor="v2-step-type" className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
                类型
              </label>
              <Select
                id="v2-step-type"
                value={newStepType}
                onChange={(event) => setNewStepType(event.target.value as StepType)}
              >
                <option value="exec">exec</option>
                <option value="gate">gate</option>
                <option value="composite">composite</option>
              </Select>
            </div>
            <Button onClick={handleCreate} disabled={creating}>
              {creating ? "创建中..." : "创建"}
            </Button>
          </div>
          <div className="grid gap-1">
            <label htmlFor="v2-step-depends" className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
              Depends On（可选，逗号分隔 StepID）
            </label>
            <Input
              id="v2-step-depends"
              value={newStepDependsOn}
              onChange={(event) => setNewStepDependsOn(event.target.value)}
              placeholder="例如：12,13"
            />
          </div>
          {createFeedback ? <p className="text-sm text-slate-600">{createFeedback}</p> : null}
          {error ? <p className="text-sm text-red-600">{error}</p> : null}

          <div className="grid gap-3">
            {loading ? <p className="text-sm text-slate-500">加载中...</p> : null}
            {!loading && steps.length === 0 ? (
              <p className="text-sm text-slate-500">暂无 Step。</p>
            ) : null}
            {steps.map((step) => (
              <button
                key={step.id}
                type="button"
                onClick={() => onSelectStep(step.id)}
                className={[
                  "flex w-full items-start justify-between gap-3 rounded-2xl border px-4 py-3 text-left transition",
                  step.id === selectedStepId ? "border-indigo-200 bg-indigo-50/40" : "border-slate-200 bg-white hover:bg-slate-50",
                ].join(" ")}
              >
                <div>
                  <p className="text-sm font-semibold text-slate-900">
                    #{step.id} · {step.name}
                  </p>
                  <p className="mt-1 text-[11px] text-slate-500">
                    {step.type} · depends {Array.isArray(step.depends_on) ? step.depends_on.join(", ") : "-"}
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  <Badge variant="outline" className="bg-slate-50 text-slate-600">
                    {step.type}
                  </Badge>
                  <Badge variant={statusTone(step.status)}>{step.status}</Badge>
                </div>
              </button>
            ))}
          </div>
        </CardContent>
      </Card>
    </PageScaffold>
  );
};

export default StepsView;
