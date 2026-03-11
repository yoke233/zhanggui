import { useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import {
  ChevronRight,
  Sparkles,
  Play,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useV2Workbench } from "@/contexts/V2WorkbenchContext";
import { getErrorMessage, normalizeStepTypeLabel } from "@/lib/v2Workbench";
import { cn } from "@/lib/utils";
import type { Step } from "@/types/apiV2";

const stepColors: Record<string, { bg: string; text: string }> = {
  exec: { bg: "bg-blue-50", text: "text-blue-600" },
  gate: { bg: "bg-amber-50", text: "text-amber-600" },
  composite: { bg: "bg-indigo-50", text: "text-indigo-600" },
};

export function CreateFlowPage() {
  const navigate = useNavigate();
  const { apiClient, projects, selectedProjectId } = useV2Workbench();
  const [name, setName] = useState("");
  const [projectId, setProjectId] = useState<number | null>(selectedProjectId);
  const [description, setDescription] = useState("");
  const [aiPrompt, setAiPrompt] = useState("");
  const [previewSteps, setPreviewSteps] = useState<Step[]>([]);
  const [draftFlowId, setDraftFlowId] = useState<number | null>(null);
  const [busy, setBusy] = useState<"idle" | "generating" | "saving" | "running">("idle");
  const [error, setError] = useState<string | null>(null);

  const selectedProject = useMemo(
    () => projects.find((project) => project.id === projectId) ?? null,
    [projectId, projects],
  );

  const ensureDraftFlow = async (): Promise<number> => {
    if (draftFlowId != null) {
      return draftFlowId;
    }
    const created = await apiClient.createFlow({
      name: name.trim(),
      project_id: projectId ?? undefined,
      metadata: description.trim() ? { description: description.trim() } : undefined,
    });
    setDraftFlowId(created.id);
    return created.id;
  };

  const generateSteps = async () => {
    if (!name.trim()) {
      setError("生成步骤前请先填写流程名称。");
      return;
    }
    setBusy("generating");
    setError(null);
    try {
      const flowId = await ensureDraftFlow();
      const steps = await apiClient.generateSteps(flowId, {
        description: aiPrompt.trim() || description.trim() || name.trim(),
      });
      setPreviewSteps(steps);
    } catch (generateError) {
      setError(getErrorMessage(generateError));
    } finally {
      setBusy("idle");
    }
  };

  const finalizeFlow = async (runImmediately: boolean) => {
    if (!name.trim()) {
      setError("流程名称不能为空。");
      return;
    }
    setBusy(runImmediately ? "running" : "saving");
    setError(null);
    try {
      const flowId = await ensureDraftFlow();
      if (previewSteps.length === 0 && (aiPrompt.trim() || description.trim())) {
        const steps = await apiClient.generateSteps(flowId, {
          description: aiPrompt.trim() || description.trim(),
        });
        setPreviewSteps(steps);
      }
      if (runImmediately) {
        await apiClient.runFlow(flowId);
      }
      navigate(`/flows/${flowId}`);
    } catch (submitError) {
      setError(getErrorMessage(submitError));
    } finally {
      setBusy("idle");
    }
  };

  return (
    <div className="flex-1 space-y-6 p-8">
      <div>
        <div className="mb-1 flex items-center gap-2 text-sm text-muted-foreground">
          <Link to="/flows" className="hover:text-foreground">流程</Link>
          <ChevronRight className="h-3 w-3" />
          <span className="font-medium text-foreground">新建流程</span>
        </div>
        <h1 className="text-2xl font-bold tracking-tight">新建流程</h1>
        <p className="text-sm text-muted-foreground">真实创建 v2 Flow，并支持调用后端 DAG 生成器。</p>
      </div>

      {error ? <p className="rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p> : null}

      <div className="grid gap-6 lg:grid-cols-[1fr_380px]">
        <div className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">基本信息</CardTitle>
              <CardDescription>填写流程的基础元数据</CardDescription>
            </CardHeader>
            <CardContent className="space-y-5">
              <div className="space-y-1.5">
                <label className="text-sm font-medium">流程名称</label>
                <Input
                  placeholder="例如：后端 API 重构"
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                />
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <label className="text-sm font-medium">所属项目</label>
                  <select
                    className="flex h-10 w-full rounded-md border bg-background px-3 text-sm"
                    value={projectId ?? ""}
                    onChange={(event) => setProjectId(event.target.value ? Number.parseInt(event.target.value, 10) : null)}
                  >
                    <option value="">全部项目</option>
                    {projects.map((project) => (
                      <option key={project.id} value={project.id}>
                        {project.name}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="space-y-1.5">
                  <label className="text-sm font-medium">草稿状态</label>
                  <div className="flex h-10 items-center rounded-md border bg-background px-3 text-sm">
                    {draftFlowId == null ? "未创建草稿" : `Flow #${draftFlowId}`}
                  </div>
                </div>
              </div>

              <div className="space-y-1.5">
                <label className="text-sm font-medium">流程描述</label>
                <textarea
                  className="flex min-h-[100px] w-full rounded-md border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                  placeholder="描述这个流程要完成的目标和范围..."
                  value={description}
                  onChange={(event) => setDescription(event.target.value)}
                />
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <div className="flex items-center gap-3">
                <div className="flex h-9 w-9 items-center justify-center rounded-md bg-indigo-50">
                  <Sparkles className="h-[18px] w-[18px] text-indigo-500" />
                </div>
                <div>
                  <CardTitle className="text-base">AI 生成步骤</CardTitle>
                  <CardDescription>通过 `/flows/:id/generate-steps` 让后端生成 DAG。</CardDescription>
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              <textarea
                className="flex min-h-[120px] w-full rounded-md border bg-background px-3 py-2 text-sm leading-relaxed focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                value={aiPrompt}
                onChange={(event) => setAiPrompt(event.target.value)}
                placeholder="描述目标、依赖、角色要求、验收标准..."
              />
              <div className="flex items-center justify-between">
                <span className="text-xs text-muted-foreground">
                  生成按钮会在后端先创建草稿 flow，再写入 steps。
                </span>
                <Button
                  className="bg-indigo-500 hover:bg-indigo-600"
                  disabled={busy !== "idle"}
                  onClick={() => void generateSteps()}
                >
                  <Sparkles className="mr-2 h-4 w-4" />
                  {busy === "generating" ? "生成中..." : "生成步骤"}
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>

        <div className="space-y-6">
          <Card className="overflow-hidden p-0">
            <div className="flex items-center justify-between border-b px-5 py-3.5">
              <span className="text-sm font-semibold">步骤预览</span>
              <Badge variant="secondary">{previewSteps.length} 步骤</Badge>
            </div>
            <div>
              {previewSteps.length === 0 ? (
                <div className="px-5 py-6 text-sm text-muted-foreground">
                  还没有生成步骤。可以先填写描述，再调用后端生成。
                </div>
              ) : (
                previewSteps.map((step, index) => {
                  const color = stepColors[step.type] ?? stepColors.exec;
                  return (
                    <div
                      key={step.id}
                      className={cn(
                        "flex items-center gap-3 px-5 py-3",
                        index < previewSteps.length - 1 && "border-b",
                      )}
                    >
                      <div className={cn("flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-[11px] font-semibold", color.bg, color.text)}>
                        {index + 1}
                      </div>
                      <div className="min-w-0 flex-1">
                        <div className="text-[13px] font-medium">{step.name}</div>
                        <div className="text-[11px] text-muted-foreground">
                          {normalizeStepTypeLabel(step.type)}
                          {step.agent_role ? ` · ${step.agent_role}` : ""}
                          {step.depends_on?.length ? ` · 依赖 #${step.depends_on.join(", #")}` : ""}
                        </div>
                      </div>
                    </div>
                  );
                })
              )}
            </div>
          </Card>

          <Card className="p-4">
            <div className="space-y-2 text-sm text-muted-foreground">
              <div>项目：{selectedProject?.name ?? "未指定"}</div>
              <div>描述来源：{aiPrompt.trim() ? "AI 生成提示词" : description.trim() ? "流程描述" : "流程名称"}</div>
            </div>
          </Card>

          <div className="space-y-2.5">
            <Button className="w-full gap-2" disabled={busy !== "idle"} onClick={() => void finalizeFlow(true)}>
              <Play className="h-4 w-4" />
              {busy === "running" ? "启动中..." : "创建并运行"}
            </Button>
            <Button variant="outline" className="w-full" disabled={busy !== "idle"} onClick={() => void finalizeFlow(false)}>
              {busy === "saving" ? "保存中..." : "仅保存草稿"}
            </Button>
            <Link to="/flows" className="block">
              <Button variant="ghost" className="w-full text-muted-foreground">
                取消
              </Button>
            </Link>
          </div>
        </div>
      </div>
    </div>
  );
}
