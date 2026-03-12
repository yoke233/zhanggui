import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import {
  ChevronRight,
  Loader2,
  Sparkles,
  Play,
  FileStack,
  Check,
  Wand2,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { getScmFlowProviderFromBindings } from "@/lib/scm";
import { getErrorMessage, normalizeStepTypeLabel } from "@/lib/v2Workbench";
import { cn } from "@/lib/utils";
import type { Step, DAGTemplate, ResourceBinding } from "@/types/apiV2";

const stepColors: Record<string, { bg: string; text: string }> = {
  exec: { bg: "bg-blue-50", text: "text-blue-600" },
  gate: { bg: "bg-amber-50", text: "text-amber-600" },
  composite: { bg: "bg-indigo-50", text: "text-indigo-600" },
};

export function CreateIssuePage() {
  const navigate = useNavigate();
  const { t } = useTranslation();
  const [searchParams] = useSearchParams();
  const { apiClient, projects, selectedProjectId } = useWorkbench();
  const [title, setTitle] = useState("");
  const [projectId, setProjectId] = useState<number | null>(
    searchParams.get("project_id") ? Number.parseInt(searchParams.get("project_id")!, 10) : selectedProjectId,
  );
  const [description, setDescription] = useState(searchParams.get("body") ?? "");
  const [generatingTitle, setGeneratingTitle] = useState(false);
  const [aiPrompt, setAiPrompt] = useState("");
  const [previewSteps, setPreviewSteps] = useState<Step[]>([]);
  const [draftIssueId, setDraftIssueId] = useState<number | null>(null);
  const [busy, setBusy] = useState<"idle" | "generating" | "saving" | "running" | "from_template">("idle");
  const [error, setError] = useState<string | null>(null);

  const [templates, setTemplates] = useState<DAGTemplate[]>([]);
  const [selectedTemplateId, setSelectedTemplateId] = useState<number | null>(null);
  const [templatesLoading, setTemplatesLoading] = useState(false);
  const [projectResources, setProjectResources] = useState<ResourceBinding[]>([]);

  const selectedProject = useMemo(
    () => projects.find((project) => project.id === projectId) ?? null,
    [projectId, projects],
  );
  const scmFlowProvider = useMemo(
    () => getScmFlowProviderFromBindings(projectResources),
    [projectResources],
  );

  const selectedTemplate = useMemo(
    () => templates.find((t) => t.id === selectedTemplateId) ?? null,
    [templates, selectedTemplateId],
  );

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

  useEffect(() => {
    if (scmFlowProvider) {
      setSelectedTemplateId(null);
      setPreviewSteps([]);
    }
  }, [scmFlowProvider]);

  useEffect(() => {
    if (projectId == null) {
      setProjectResources([]);
      return;
    }
    let cancelled = false;
    const loadResources = async () => {
      try {
        const resources = await apiClient.listProjectResources(projectId);
        if (!cancelled) {
          setProjectResources(resources);
        }
      } catch {
        if (!cancelled) {
          setProjectResources([]);
        }
      }
    };
    void loadResources();
    return () => {
      cancelled = true;
    };
  }, [apiClient, projectId]);

  const ensureDraftIssue = async (): Promise<number> => {
    if (draftIssueId != null) {
      return draftIssueId;
    }
    const created = await apiClient.createIssue({
      title: title.trim(),
      project_id: projectId ?? undefined,
      metadata: description.trim() ? { description: description.trim() } : undefined,
    });
    setDraftIssueId(created.id);
    return created.id;
  };

  const generateSteps = async () => {
    if (scmFlowProvider) {
      setError(t("createFlow.scmFlowError"));
      return;
    }
    if (!title.trim()) {
      setError(t("createFlow.nameRequired"));
      return;
    }
    setBusy("generating");
    setError(null);
    try {
      const issueId = await ensureDraftIssue();
      const steps = await apiClient.generateSteps(issueId, {
        description: aiPrompt.trim() || description.trim() || title.trim(),
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
    const issueTitle = title.trim() || selectedTemplate.name;
    setBusy(runImmediately ? "running" : "from_template");
    setError(null);
    try {
      const result = await apiClient.createIssueFromTemplate(selectedTemplate.id, {
        title: issueTitle,
        project_id: projectId ?? undefined,
      });
      if (runImmediately) {
        await apiClient.runIssue(result.issue.id);
      }
      navigate(`/issues/${result.issue.id}`);
    } catch (templateError) {
      setError(getErrorMessage(templateError));
    } finally {
      setBusy("idle");
    }
  };

  const finalizeIssue = async (runImmediately: boolean) => {
    if (selectedTemplate) {
      return createFromTemplate(runImmediately);
    }

    if (!title.trim()) {
      setError(t("createFlow.nameEmpty"));
      return;
    }
    setBusy(runImmediately ? "running" : "saving");
    setError(null);
    try {
      const issueId = await ensureDraftIssue();
      if (!scmFlowProvider && previewSteps.length === 0 && (aiPrompt.trim() || description.trim())) {
        const steps = await apiClient.generateSteps(issueId, {
          description: aiPrompt.trim() || description.trim(),
        });
        setPreviewSteps(steps);
      }
      if (runImmediately) {
        await apiClient.runIssue(issueId);
      }
      navigate(`/issues/${issueId}`);
    } catch (submitError) {
      setError(getErrorMessage(submitError));
    } finally {
      setBusy("idle");
    }
  };

  const handleGenerateTitle = async () => {
    const text = description.trim() || aiPrompt.trim();
    if (!text) return;
    setGeneratingTitle(true);
    setError(null);
    try {
      const result = await apiClient.generateTitle({ description: text });
      if (result.title) setTitle(result.title);
    } catch (genError) {
      setError(getErrorMessage(genError));
    } finally {
      setGeneratingTitle(false);
    }
  };

  const handleTemplateSelect = (templateId: number | null) => {
    setSelectedTemplateId(templateId);
    if (templateId) {
      const tmpl = templates.find((t) => t.id === templateId);
      if (tmpl && !title.trim()) {
        setTitle(tmpl.name);
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
          <Link to="/issues" className="hover:text-foreground">{t("createFlow.breadcrumbFlows")}</Link>
          <ChevronRight className="h-3 w-3" />
          <span className="font-medium text-foreground">{t("createFlow.breadcrumbNew")}</span>
        </div>
        <h1 className="text-2xl font-bold tracking-tight">{t("createFlow.title")}</h1>
        <p className="text-sm text-muted-foreground">{t("createFlow.subtitle")}</p>
      </div>

      {error ? <p className="rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p> : null}

      {templates.length > 0 && !scmFlowProvider ? (
        <Card>
          <CardHeader>
            <div className="flex items-center gap-3">
              <div className="flex h-9 w-9 items-center justify-center rounded-md bg-emerald-50">
                <FileStack className="h-[18px] w-[18px] text-emerald-500" />
              </div>
              <div>
                <CardTitle className="text-base">{t("createFlow.fromTemplate")}</CardTitle>
                <CardDescription>{t("createFlow.fromTemplateDesc")}</CardDescription>
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
                    <Badge variant="secondary" className="text-[10px]">{t("createFlow.nSteps", { count: tmpl.steps.length })}</Badge>
                    {(tmpl.tags ?? []).slice(0, 2).map((tag) => (
                      <Badge key={tag} variant="outline" className="text-[10px]">{tag}</Badge>
                    ))}
                  </div>
                </button>
              ))}
            </div>
            {selectedTemplate ? (
              <div className="mt-3 text-xs text-muted-foreground">
                {t("createFlow.templateSelected", { name: selectedTemplate.name, count: selectedTemplate.steps.length })} {t("createFlow.templateHint")}
              </div>
            ) : null}
          </CardContent>
        </Card>
      ) : templatesLoading && !scmFlowProvider ? (
        <div className="text-sm text-muted-foreground">{t("createFlow.loadingTemplates")}</div>
      ) : null}

      <div className="grid gap-6 lg:grid-cols-[1fr_380px]">
        <div className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">{t("createFlow.basicInfo")}</CardTitle>
              <CardDescription>{t("createFlow.basicInfoDesc")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-5">
              <div className="space-y-1.5">
                <label className="text-sm font-medium">{t("createFlow.flowName")}</label>
                <div className="flex gap-2">
                  <Input
                    className="flex-1"
                    placeholder={t("createFlow.flowNamePlaceholder")}
                    value={title}
                    onChange={(event) => setTitle(event.target.value)}
                  />
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="shrink-0 gap-1.5"
                    disabled={generatingTitle || (!description.trim() && !aiPrompt.trim())}
                    onClick={() => void handleGenerateTitle()}
                  >
                    {generatingTitle ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Wand2 className="h-3.5 w-3.5" />}
                    {t("createFlow.autoTitle")}
                  </Button>
                </div>
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <label className="text-sm font-medium">{t("createFlow.project")}</label>
                  <select
                    className="flex h-10 w-full rounded-md border bg-background px-3 text-sm"
                    value={projectId ?? ""}
                    onChange={(event) => setProjectId(event.target.value ? Number.parseInt(event.target.value, 10) : null)}
                  >
                    <option value="">{t("createFlow.allProjects")}</option>
                    {projects.map((project) => (
                      <option key={project.id} value={project.id}>
                        {project.name}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="space-y-1.5">
                  <label className="text-sm font-medium">{t("createFlow.draftStatus")}</label>
                  <div className="flex h-10 items-center rounded-md border bg-background px-3 text-sm">
                    {selectedTemplate
                      ? t("createFlow.templateDraft", { id: selectedTemplate.id })
                      : draftIssueId == null
                        ? t("createFlow.noDraft")
                        : `Issue #${draftIssueId}`}
                  </div>
                </div>
              </div>

              <div className="space-y-1.5">
                <label className="text-sm font-medium">{t("createFlow.flowDesc")}</label>
                <textarea
                  className="flex min-h-[100px] w-full rounded-md border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                  placeholder={t("createFlow.flowDescPlaceholder")}
                  value={description}
                  onChange={(event) => setDescription(event.target.value)}
                />
              </div>
            </CardContent>
          </Card>

          {scmFlowProvider ? (
            <Card>
              <CardHeader>
                <CardTitle className="text-base">{t("createFlow.scmFlowTitle")}</CardTitle>
                <CardDescription>
                  {t("createFlow.scmFlowDesc", { provider: scmFlowProvider === "codeup" ? "Codeup CR" : "GitHub PR" })}
                </CardDescription>
              </CardHeader>
            </Card>
          ) : !selectedTemplate ? (
            <Card>
              <CardHeader>
                <div className="flex items-center gap-3">
                  <div className="flex h-9 w-9 items-center justify-center rounded-md bg-indigo-50">
                    <Sparkles className="h-[18px] w-[18px] text-indigo-500" />
                  </div>
                  <div>
                    <CardTitle className="text-base">{t("createFlow.aiGenerate")}</CardTitle>
                    <CardDescription>{t("createFlow.aiGenerateDesc")}</CardDescription>
                  </div>
                </div>
              </CardHeader>
              <CardContent className="space-y-4">
                <textarea
                  className="flex min-h-[120px] w-full rounded-md border bg-background px-3 py-2 text-sm leading-relaxed focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                  value={aiPrompt}
                  onChange={(event) => setAiPrompt(event.target.value)}
                  placeholder={t("createFlow.aiPromptPlaceholder")}
                />
                <div className="flex items-center justify-between">
                  <span className="text-xs text-muted-foreground">
                    {t("createFlow.aiPromptHint")}
                  </span>
                  <Button
                    className="bg-indigo-500 hover:bg-indigo-600"
                    disabled={busy !== "idle"}
                    onClick={() => void generateSteps()}
                  >
                    <Sparkles className="mr-2 h-4 w-4" />
                    {busy === "generating" ? t("createFlow.generating") : t("createFlow.generateSteps")}
                  </Button>
                </div>
              </CardContent>
            </Card>
          ) : null}
        </div>

        <div className="space-y-6">
          <Card className="overflow-hidden p-0">
            <div className="flex items-center justify-between border-b px-5 py-3.5">
              <span className="text-sm font-semibold">{t("createFlow.stepPreview")}</span>
              <Badge variant="secondary">
                {selectedTemplate
                  ? t("createFlow.nSteps", { count: selectedTemplate.steps.length })
                  : t("createFlow.nSteps", { count: previewSteps.length })}
              </Badge>
            </div>
            <div>
              {selectedTemplate ? (
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
                          {step.depends_on?.length ? ` · ${t("createFlow.dependsOn", { deps: step.depends_on.join(", ") })}` : ""}
                        </div>
                      </div>
                    </div>
                  );
                })
              ) : scmFlowProvider ? (
                <div className="px-5 py-6 text-sm text-muted-foreground">
                  {t("createFlow.scmAutoSteps")}
                </div>
              ) : previewSteps.length === 0 ? (
                <div className="px-5 py-6 text-sm text-muted-foreground">
                  {t("createFlow.noSteps")}
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
                          {step.depends_on?.length ? ` · ${t("createFlow.dependsOn", { deps: step.depends_on.join(", ") })}` : ""}
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
              <div>{t("createFlow.projectLabel", { name: selectedProject?.name ?? t("createFlow.notSpecified") })}</div>
              {scmFlowProvider ? (
                <div>{t("createFlow.scmFlow", { provider: scmFlowProvider === "codeup" ? t("createFlow.codeupAuto") : t("createFlow.githubAuto") })}</div>
              ) : null}
              <div>
                {selectedTemplate
                  ? t("createFlow.sourceTemplate", { name: selectedTemplate.name })
                  : aiPrompt.trim()
                    ? t("createFlow.sourceAiPrompt")
                    : description.trim()
                      ? t("createFlow.sourceDesc")
                      : t("createFlow.sourceFlowName")}
              </div>
            </div>
          </Card>

          <div className="space-y-2.5">
            <Button className="w-full gap-2" disabled={busy !== "idle"} onClick={() => void finalizeIssue(true)}>
              <Play className="h-4 w-4" />
              {busy === "running" ? t("createFlow.running") : t("createFlow.createAndRun")}
            </Button>
            <Button variant="outline" className="w-full" disabled={busy !== "idle"} onClick={() => void finalizeIssue(false)}>
              {busy === "saving" || busy === "from_template" ? t("createFlow.saving") : t("createFlow.saveAsDraft")}
            </Button>
            <Link to="/issues" className="block">
              <Button variant="ghost" className="w-full text-muted-foreground">
                {t("common.cancel")}
              </Button>
            </Link>
          </div>
        </div>
      </div>
    </div>
  );
}

// Keep backward-compatible export
export { CreateIssuePage as CreateFlowPage };
