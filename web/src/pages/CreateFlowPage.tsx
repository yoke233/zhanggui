import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import {
  ChevronRight,
  Sparkles,
  Play,
  FileStack,
  Check,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { getErrorMessage, normalizeStepTypeLabel } from "@/lib/v2Workbench";
import { cn } from "@/lib/utils";
import type { Step, DAGTemplate } from "@/types/apiV2";

const stepColors: Record<string, { bg: string; text: string }> = {
  exec: { bg: "bg-blue-50", text: "text-blue-600" },
  gate: { bg: "bg-amber-50", text: "text-amber-600" },
  composite: { bg: "bg-indigo-50", text: "text-indigo-600" },
};

export function CreateFlowPage() {
  const navigate = useNavigate();
  const { apiClient, projects, selectedProjectId } = useWorkbench();
  const [name, setName] = useState("");
  const [projectId, setProjectId] = useState<number | null>(selectedProjectId);
  const [description, setDescription] = useState("");
  const [aiPrompt, setAiPrompt] = useState("");
  const [previewSteps, setPreviewSteps] = useState<Step[]>([]);
  const [draftFlowId, setDraftFlowId] = useState<number | null>(null);
  const [busy, setBusy] = useState<"idle" | "generating" | "saving" | "running" | "from_template">("idle");
  const [error, setError] = useState<string | null>(null);

  // Template selection state
  const [templates, setTemplates] = useState<DAGTemplate[]>([]);
  const [selectedTemplateId, setSelectedTemplateId] = useState<number | null>(null);
  const [templatesLoading, setTemplatesLoading] = useState(false);

  const selectedProject = useMemo(
    () => projects.find((project) => project.id === projectId) ?? null,
    [projectId, projects],
  );

  const selectedTemplate = useMemo(
    () => templates.find((t) => t.id === selectedTemplateId) ?? null,
    [templates, selectedTemplateId],
  );

  // Load available templates
  const loadTemplates = useCallback(async () => {
    setTemplatesLoading(true);
    try {
      const listed = await apiClient.listDAGTemplates({ limit: 100 });
      setTemplates(listed);
    } catch {
      // Silently ignore template loading errors; user can still create manually
    } finally {
      setTemplatesLoading(false);
    }
  }, [apiClient]);

  useEffect(() => {
    void loadTemplates();
  }, [loadTemplates]);

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

  const createFromTemplate = async (runImmediately: boolean) => {
    if (!selectedTemplate) return;
    const flowName = name.trim() || selectedTemplate.name;
    setBusy(runImmediately ? "running" : "from_template");
    setError(null);
    try {
      const result = await apiClient.createFlowFromTemplate(selectedTemplate.id, {
        name: flowName,
        project_id: projectId ?? undefined,
      });
      if (runImmediately) {
        await apiClient.runFlow(result.flow.id);
      }
      navigate(`/flows/${result.flow.id}`);
    } catch (templateError) {
      setError(getErrorMessage(templateError));
    } finally {
      setBusy("idle");
    }
  };

  const finalizeFlow = async (runImmediately: boolean) => {
    // If a template is selected, use the template flow path
    if (selectedTemplate) {
      return createFromTemplate(runImmediately);
    }

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

  const handleTemplateSelect = (templateId: number | null) => {
    setSelectedTemplateId(templateId);
    if (templateId) {
      const tmpl = templates.find((t) => t.id === templateId);
      if (tmpl && !name.trim()) {
        setName(tmpl.name);
      }
      if (tmpl && !description.trim() && tmpl.description) {
        setDescription(tmpl.description);
      }
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
        <p className="text-sm text-muted-foreground">从模板创建、AI 生成或手动配置 Flow。</p>
      </div>

      {error ? <p className="rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p> : null}

      {/* Template selector */}
      {templates.length > 0 ? (
        <Card>
          <CardHeader>
            <div className="flex items-center gap-3">
              <div className="flex h-9 w-9 items-center justify-center rounded-md bg-emerald-50">
                <FileStack className="h-[18px] w-[18px] text-emerald-500" />
              </div>
              <div>
                <CardTitle className="text-base">从模板创建</CardTitle>
                <CardDescription>选择一个已有模板快速创建流程，或跳过此步使用 AI 生成。</CardDescription>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              {templates.map((tmpl) => (
                <button
                  key={tmpl.id}
                  onClick={() => handleTemplateSelect(selectedTemplateId === tmpl.id ? null : tmpl.id)}
                  className={cn(
                    "relative rounded-lg border p-3 text-left transition-colors hover:bg-accent",
                    selectedTemplateId === tmpl.id && "border-emerald-500 bg-emerald-50/50 ring-1 ring-emerald-500",
                  )}
                >
                  {selectedTemplateId === tmpl.id ? (
                    <div className="absolute right-2 top-2 flex h-5 w-5 items-center justify-center rounded-full bg-emerald-500">
                      <Check className="h-3 w-3 text-white" />
                    </div>
                  ) : null}
                  <div className="text-sm font-medium">{tmpl.name}</div>
                  {tmpl.description ? (
                    <div className="mt-0.5 text-xs text-muted-foreground line-clamp-1">{tmpl.description}</div>
                  ) : null}
                  <div className="mt-2 flex items-center gap-2">
                    <Badge variant="secondary" className="text-[10px]">{tmpl.steps.length} 步骤</Badge>
                    {(tmpl.tags ?? []).slice(0, 2).map((tag) => (
                      <Badge key={tag} variant="outline" className="text-[10px]">{tag}</Badge>
                    ))}
                  </div>
                </button>
              ))}
            </div>
            {selectedTemplate ? (
              <div className="mt-3 text-xs text-muted-foreground">
                已选择模板「{selectedTemplate.name}」，包含 {selectedTemplate.steps.length} 个步骤。
                点击「创建并运行」将直接从模板创建流程。
              </div>
            ) : null}
          </CardContent>
        </Card>
      ) : templatesLoading ? (
        <div className="text-sm text-muted-foreground">正在加载模板...</div>
      ) : null}

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
                    {selectedTemplate
                      ? `模板 #${selectedTemplate.id}`
                      : draftFlowId == null
                        ? "未创建草稿"
                        : `Flow #${draftFlowId}`}
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

          {!selectedTemplate ? (
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
          ) : null}
        </div>

        <div className="space-y-6">
          <Card className="overflow-hidden p-0">
            <div className="flex items-center justify-between border-b px-5 py-3.5">
              <span className="text-sm font-semibold">步骤预览</span>
              <Badge variant="secondary">
                {selectedTemplate
                  ? `${selectedTemplate.steps.length} 步骤`
                  : `${previewSteps.length} 步骤`}
              </Badge>
            </div>
            <div>
              {selectedTemplate ? (
                // Show template steps preview
                selectedTemplate.steps.map((step, index) => {
                  const color = stepColors[step.type] ?? stepColors.exec;
                  return (
                    <div
                      key={step.name}
                      className={cn(
                        "flex items-center gap-3 px-5 py-3",
                        index < selectedTemplate.steps.length - 1 && "border-b",
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
                          {step.depends_on?.length ? ` · 依赖 ${step.depends_on.join(", ")}` : ""}
                        </div>
                      </div>
                    </div>
                  );
                })
              ) : previewSteps.length === 0 ? (
                <div className="px-5 py-6 text-sm text-muted-foreground">
                  还没有生成步骤。可以先选择模板、填写描述再调用后端生成。
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
              <div>
                来源：{selectedTemplate
                  ? `模板「${selectedTemplate.name}」`
                  : aiPrompt.trim()
                    ? "AI 生成提示词"
                    : description.trim()
                      ? "流程描述"
                      : "流程名称"}
              </div>
            </div>
          </Card>

          <div className="space-y-2.5">
            <Button className="w-full gap-2" disabled={busy !== "idle"} onClick={() => void finalizeFlow(true)}>
              <Play className="h-4 w-4" />
              {busy === "running" ? "启动中..." : "创建并运行"}
            </Button>
            <Button variant="outline" className="w-full" disabled={busy !== "idle"} onClick={() => void finalizeFlow(false)}>
              {busy === "saving" || busy === "from_template" ? "保存中..." : "仅保存草稿"}
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
