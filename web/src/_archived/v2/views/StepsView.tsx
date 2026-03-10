import { useEffect, useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import type { ApiClientV2 } from "@/lib/apiClientV2";
import type { Step, StepType } from "@/types/apiV2";
import StepsDag from "@/v2/components/StepsDag";
import { PageScaffold } from "@/v3/components/PageScaffold";

interface StepsViewProps {
  apiClient: ApiClientV2;
  flowId: number;
  selectedStepId: number | null;
  refreshToken: number;
  onSelectStep: (stepId: number | null) => void;
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

  const [genDescription, setGenDescription] = useState("");
  const [generating, setGenerating] = useState(false);
  const [genFeedback, setGenFeedback] = useState<string | null>(null);

  const [editingName, setEditingName] = useState("");
  const [editingType, setEditingType] = useState<StepType>("exec");
  const [editingDependsOn, setEditingDependsOn] = useState("");
  const [editingAgentRole, setEditingAgentRole] = useState("");
  const [editingAcceptance, setEditingAcceptance] = useState("");
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [editFeedback, setEditFeedback] = useState<string | null>(null);

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

  useEffect(() => {
    if (!selectedStep) {
      setEditingName("");
      setEditingType("exec");
      setEditingDependsOn("");
      setEditingAgentRole("");
      setEditingAcceptance("");
      setEditFeedback(null);
      return;
    }
    setEditingName(selectedStep.name ?? "");
    setEditingType(selectedStep.type ?? "exec");
    setEditingDependsOn(Array.isArray(selectedStep.depends_on) ? selectedStep.depends_on.join(",") : "");
    setEditingAgentRole(selectedStep.agent_role ?? "");
    setEditingAcceptance(Array.isArray(selectedStep.acceptance_criteria) ? selectedStep.acceptance_criteria.join("\n") : "");
    setEditFeedback(null);
  }, [selectedStep]);

  const parseDependsOn = (raw: string): number[] =>
    raw
      .split(",")
      .map((item) => item.trim())
      .filter((item) => item.length > 0)
      .map((item) => Number.parseInt(item, 10))
      .filter((num) => Number.isFinite(num) && num > 0);

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

  const handleGenerate = async () => {
    const description = genDescription.trim();
    if (!description) {
      setGenFeedback("请先填写任务描述。");
      return;
    }
    setGenerating(true);
    setGenFeedback(null);
    try {
      const created = await apiClient.generateSteps(flowId, { description });
      setGenFeedback(created.length > 0 ? `已生成 ${created.length} 个 Step。` : "已触发生成，但未返回 Step 列表。");
      const listed = await apiClient.listSteps(flowId);
      setSteps(listed);
      if (listed.length > 0) {
        onSelectStep(listed[0].id);
      }
    } catch (err) {
      setGenFeedback(getErrorMessage(err));
    } finally {
      setGenerating(false);
    }
  };

  const refreshSteps = async (nextSelectedId?: number | null) => {
    const listed = await apiClient.listSteps(flowId);
    setSteps(listed);
    if (nextSelectedId === null) {
      onSelectStep(null);
      return;
    }
    if (typeof nextSelectedId === "number") {
      onSelectStep(nextSelectedId);
      return;
    }
    if (listed.length === 0) {
      onSelectStep(null);
      return;
    }
    if (selectedStepId != null && listed.some((s) => s.id === selectedStepId)) {
      return;
    }
    onSelectStep(listed[0].id);
  };

  const handleSaveSelected = async () => {
    if (!selectedStep) {
      return;
    }
    const name = editingName.trim();
    if (!name) {
      setEditFeedback("Step 名称不能为空。");
      return;
    }
    const dependsOn = parseDependsOn(editingDependsOn).filter((id) => id !== selectedStep.id);
    const acceptance = editingAcceptance
      .split("\n")
      .map((line) => line.trim())
      .filter((line) => line.length > 0);

    setSaving(true);
    setEditFeedback(null);
    try {
      await apiClient.updateStep(selectedStep.id, {
        name,
        type: editingType === "gate" ? "gate" : editingType === "composite" ? "composite" : "exec",
        depends_on: dependsOn.length > 0 ? dependsOn : [],
        agent_role: editingAgentRole.trim() || undefined,
        acceptance_criteria: acceptance.length > 0 ? acceptance : [],
      });
      setEditFeedback("已保存。");
      await refreshSteps(selectedStep.id);
    } catch (err) {
      setEditFeedback(getErrorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const handleDeleteSelected = async () => {
    if (!selectedStep) {
      return;
    }
    const confirmed = window.confirm(`确定删除 Step #${selectedStep.id} · ${selectedStep.name} 吗？此操作不可撤销。`);
    if (!confirmed) {
      return;
    }
    setDeleting(true);
    setEditFeedback(null);
    try {
      await apiClient.deleteStep(selectedStep.id);
      await refreshSteps(null);
    } catch (err) {
      setEditFeedback(getErrorMessage(err));
    } finally {
      setDeleting(false);
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

          <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <p className="text-sm font-semibold text-slate-950">AI 生成整个 DAG</p>
                <p className="mt-1 text-xs leading-5 text-slate-500">
                  仅对 <span className="font-semibold text-slate-700">pending</span> 的 Flow 生效；会批量创建 Steps（含 depends_on）。
                </p>
              </div>
              <Button variant="outline" size="sm" onClick={() => void handleGenerate()} disabled={generating}>
                {generating ? "生成中..." : "生成 DAG"}
              </Button>
            </div>
            <div className="mt-3 grid gap-2">
              <label htmlFor="v2-step-gen-desc" className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
                任务描述
              </label>
              <Textarea
                id="v2-step-gen-desc"
                value={genDescription}
                onChange={(event) => setGenDescription(event.target.value)}
                placeholder="例如：把前端 v3 界面像素级迁到 v2 接口，保留样式，全屏化，并补齐 ws 支持。"
              />
              {genFeedback ? <p className="text-sm text-slate-600">{genFeedback}</p> : null}
            </div>
          </div>

          <div className="grid gap-4 lg:grid-cols-[minmax(0,420px)_minmax(0,1fr)]">
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
                    step.id === selectedStepId
                      ? "border-indigo-200 bg-indigo-50/40"
                      : "border-slate-200 bg-white hover:bg-slate-50",
                  ].join(" ")}
                >
                  <div>
                    <p className="text-sm font-semibold text-slate-900">
                      #{step.id} · {step.name}
                    </p>
                    <p className="mt-1 text-[11px] text-slate-500">
                      {step.type} · depends{" "}
                      {Array.isArray(step.depends_on) ? step.depends_on.join(", ") : "-"}
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

            <div className="grid gap-4">
              <StepsDag
                flowId={flowId}
                steps={steps}
                selectedStepId={selectedStepId}
                onSelectStep={(stepId) => onSelectStep(stepId)}
                onCreateStep={async (input) => {
                  const created = await apiClient.createStep(flowId, {
                    name: input.name,
                    type: input.type,
                    depends_on: input.depends_on,
                  });
                  await refreshSteps(created.id);
                }}
                onUpdateStepDependsOn={async (stepId, dependsOn) => {
                  await apiClient.updateStep(stepId, { depends_on: dependsOn });
                  await refreshSteps(stepId);
                }}
              />

              <div className="rounded-2xl border border-slate-200 bg-white p-4">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <p className="text-sm font-semibold text-slate-900">节点属性</p>
                    <p className="mt-1 text-xs leading-5 text-slate-500">编辑后点保存；拖拽连线会自动更新 depends_on。</p>
                  </div>
                  <div className="flex items-center gap-2">
                    <Button variant="outline" size="sm" onClick={() => void handleSaveSelected()} disabled={!selectedStep || saving}>
                      {saving ? "保存中..." : "保存"}
                    </Button>
                    <Button variant="destructive" size="sm" onClick={() => void handleDeleteSelected()} disabled={!selectedStep || deleting}>
                      {deleting ? "删除中..." : "删除"}
                    </Button>
                  </div>
                </div>

                {!selectedStep ? (
                  <p className="mt-3 text-sm text-slate-500">请先选择一个 Step。</p>
                ) : (
                  <div className="mt-3 grid gap-3">
                    <div className="grid gap-1">
                      <label htmlFor="v2-step-edit-name" className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
                        名称
                      </label>
                      <Input
                        id="v2-step-edit-name"
                        value={editingName}
                        onChange={(event) => setEditingName(event.target.value)}
                        placeholder="例如：implement-api-client"
                      />
                    </div>

                    <div className="grid gap-1 md:grid-cols-2 md:items-end">
                      <div className="grid gap-1">
                        <label htmlFor="v2-step-edit-type" className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
                          类型
                        </label>
                        <Select
                          id="v2-step-edit-type"
                          value={editingType}
                          onChange={(event) => setEditingType(event.target.value as StepType)}
                        >
                          <option value="exec">exec</option>
                          <option value="gate">gate</option>
                          <option value="composite">composite</option>
                        </Select>
                      </div>
                      <div className="grid gap-1">
                        <label htmlFor="v2-step-edit-agent" className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
                          agent_role（可选）
                        </label>
                        <Input
                          id="v2-step-edit-agent"
                          value={editingAgentRole}
                          onChange={(event) => setEditingAgentRole(event.target.value)}
                          placeholder="worker / gate / lead"
                        />
                      </div>
                    </div>

                    <div className="grid gap-1">
                      <label htmlFor="v2-step-edit-deps" className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
                        depends_on（逗号分隔 StepID）
                      </label>
                      <Input
                        id="v2-step-edit-deps"
                        value={editingDependsOn}
                        onChange={(event) => setEditingDependsOn(event.target.value)}
                        placeholder="例如：12,13"
                      />
                    </div>

                    <div className="grid gap-1">
                      <label htmlFor="v2-step-edit-acc" className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
                        acceptance_criteria（可选，每行一条）
                      </label>
                      <Textarea
                        id="v2-step-edit-acc"
                        value={editingAcceptance}
                        onChange={(event) => setEditingAcceptance(event.target.value)}
                        placeholder="例如：\n- 前端所有 v2 API 对齐且编译通过\n- 关键路径 e2e 通过"
                        className="min-h-[96px]"
                      />
                    </div>

                    {editFeedback ? <p className="text-sm text-slate-600">{editFeedback}</p> : null}
                  </div>
                )}
              </div>
            </div>
          </div>
        </CardContent>
      </Card>
    </PageScaffold>
  );
};

export default StepsView;
