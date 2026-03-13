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
import { AttachmentUploader } from "@/components/AttachmentUploader";
import { getScmFlowProviderFromBindings } from "@/lib/scm";
import { getErrorMessage, normalizeStepTypeLabel } from "@/lib/v2Workbench";
import { cn } from "@/lib/utils";
import type { Action, DAGTemplate, ResourceBinding, WorkItemAttachment } from "@/types/apiV2";

const stepColors: Record<string, { bg: string; text: string }> = {
  exec: { bg: "bg-blue-50", text: "text-blue-600" },
  gate: { bg: "bg-amber-50", text: "text-amber-600" },
  composite: { bg: "bg-indigo-50", text: "text-indigo-600" },
};

export function CreateWorkItemPage() {
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
  const [previewSteps, setPreviewSteps] = useState<Action[]>([]);
  const [draftWorkItemId, setDraftWorkItemId] = useState<number | null>(null);
  const [busy, setBusy] = useState<"idle" | "generating" | "saving" | "running" | "from_template">("idle");
  const [error, setError] = useState<string | null>(null);
  const [pendingFiles, setPendingFiles] = useState<File[]>([]);
  const [uploadedAttachments, setUploadedAttachments] = useState<WorkItemAttachment[]>([]);
  const [uploading, setUploading] = useState(false);
  const [templates, setTemplates] = useState<DAGTemplate[]>([]);
  const [selectedTemplateId, setSelectedTemplateId] = useState<number | null>(null);
  const [templatesLoading, setTemplatesLoading] = useState(false);
  const [projectResources, setProjectResources] = useState<ResourceBinding[]>([]);

  const selectedProject = useMemo(
    () => projects.find((project) => project.id === projectId) ?? null,
    [projectId, projects],
  );
  const scmProvider = useMemo(
    () => getScmFlowProviderFromBindings(projectResources),
    [projectResources],
  );
  const selectedTemplate = useMemo(
    () => templates.find((template) => template.id === selectedTemplateId) ?? null,
    [templates, selectedTemplateId],
  );

  const loadTemplates = useCallback(async () => {
    setTemplatesLoading(true);
    try {
      const listed = await apiClient.listDAGTemplates({ limit: 100 });
      setTemplates(listed);
    } catch {
      // 模板读取失败不阻断手工创建流程。
    } finally {
      setTemplatesLoading(false);
    }
  }, [apiClient]);

  useEffect(() => {
    void loadTemplates();
  }, [loadTemplates]);

  useEffect(() => {
    if (scmProvider) {
      setSelectedTemplateId(null);
      setPreviewSteps([]);
    }
  }, [scmProvider]);

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

  const ensureDraftWorkItem = async (): Promise<number> => {
    if (draftWorkItemId != null) {
      return draftWorkItemId;
    }
    const created = await apiClient.createWorkItem({
      title: title.trim(),
      project_id: projectId ?? undefined,
      metadata: description.trim() ? { description: description.trim() } : undefined,
    });
    setDraftWorkItemId(created.id);
    return created.id;
  };

  const handleFilesSelected = (files: File[]) => {
    setPendingFiles((previous) => [...previous, ...files]);
  };

  const removePendingFile = (index: number) => {
    setPendingFiles((previous) => previous.filter((_, currentIndex) => currentIndex !== index));
  };

  const removeUploadedAttachment = async (attachment: { id: number }) => {
    try {
      await apiClient.deleteWorkItemAttachment(attachment.id);
      setUploadedAttachments((previous) => previous.filter((currentAttachment) => currentAttachment.id !== attachment.id));
    } catch (deleteError) {
      setError(getErrorMessage(deleteError));
    }
  };

  const uploadPendingFiles = async (workItemId: number) => {
    if (pendingFiles.length === 0) {
      return;
    }
    setUploading(true);
    try {
      const results: WorkItemAttachment[] = [];
      for (const file of pendingFiles) {
        const attachment = await apiClient.uploadWorkItemAttachment(workItemId, file);
        results.push(attachment);
      }
      setUploadedAttachments((previous) => [...previous, ...results]);
      setPendingFiles([]);
    } catch (uploadError) {
      setError(getErrorMessage(uploadError));
    } finally {
      setUploading(false);
    }
  };

  const generateSteps = async () => {
    if (scmProvider) {
      setError(t("createWorkItem.scmFlowError"));
      return;
    }
    if (!title.trim()) {
      setError(t("createWorkItem.nameRequired"));
      return;
    }
    setBusy("generating");
    setError(null);
    try {
      const workItemId = await ensureDraftWorkItem();
      const generatedSteps = await apiClient.generateActions(workItemId, {
        description: aiPrompt.trim() || description.trim() || title.trim(),
      });
      setPreviewSteps(generatedSteps);
    } catch (generateError) {
      setError(getErrorMessage(generateError));
    } finally {
      setBusy("idle");
    }
  };

  const createFromTemplate = async (runImmediately: boolean) => {
    if (!selectedTemplate) {
      return;
    }
    const workItemTitle = title.trim() || selectedTemplate.name;
    setBusy(runImmediately ? "running" : "from_template");
    setError(null);
    try {
      const result = await apiClient.createWorkItemFromTemplate(selectedTemplate.id, {
        title: workItemTitle,
        project_id: projectId ?? undefined,
      });
      if (runImmediately) {
        await apiClient.runWorkItem(result.issue.id);
      }
      navigate(`/work-items/${result.issue.id}`);
    } catch (templateError) {
      setError(getErrorMessage(templateError));
    } finally {
      setBusy("idle");
    }
  };

  const finalizeWorkItem = async (runImmediately: boolean) => {
    if (selectedTemplate) {
      return createFromTemplate(runImmediately);
    }
    if (!title.trim()) {
      setError(t("createWorkItem.nameEmpty"));
      return;
    }
    setBusy(runImmediately ? "running" : "saving");
    setError(null);
    try {
      const workItemId = await ensureDraftWorkItem();
      await uploadPendingFiles(workItemId);
      if (!scmProvider && previewSteps.length === 0 && (aiPrompt.trim() || description.trim())) {
        const generatedSteps = await apiClient.generateActions(workItemId, {
          description: aiPrompt.trim() || description.trim(),
        });
        setPreviewSteps(generatedSteps);
      }
      if (runImmediately) {
        await apiClient.runWorkItem(workItemId);
      }
      navigate(`/work-items/${workItemId}`);
    } catch (submitError) {
      setError(getErrorMessage(submitError));
    } finally {
      setBusy("idle");
    }
  };

  const handleGenerateTitle = async () => {
    const sourceText = description.trim() || aiPrompt.trim();
    if (!sourceText) {
      return;
    }
    setGeneratingTitle(true);
    setError(null);
    try {
      const result = await apiClient.generateTitle({ description: sourceText });
      if (result.title) {
        setTitle(result.title);
      }
    } catch (generateError) {
      setError(getErrorMessage(generateError));
    } finally {
      setGeneratingTitle(false);
    }
  };

  const handleTemplateSelect = (templateId: number | null) => {
    setSelectedTemplateId(templateId);
    if (!templateId) {
      return;
    }
    const template = templates.find((candidate) => candidate.id === templateId);
    if (template && !title.trim()) {
      setTitle(template.name);
    }
    if (template && !description.trim() && template.description) {
      setDescription(template.description);
    }
  };

  return (
    <div className="flex-1 space-y-6 p-8">
      <div>
        <div className="mb-1 flex items-center gap-2 text-sm text-muted-foreground">
          <Link to="/work-items" className="hover:text-foreground">{t("createWorkItem.breadcrumbWorkItems")}</Link>
          <ChevronRight className="h-3 w-3" />
          <span className="font-medium text-foreground">{t("createWorkItem.breadcrumbNew")}</span>
        </div>
        <h1 className="text-2xl font-bold tracking-tight">{t("createWorkItem.title")}</h1>
        <p className="text-sm text-muted-foreground">{t("createWorkItem.subtitle")}</p>
      </div>

      {error ? <p className="rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p> : null}

      {templates.length > 0 && !scmProvider ? (
        <Card>
          <CardHeader>
            <div className="flex items-center gap-3">
              <div className="flex h-9 w-9 items-center justify-center rounded-md bg-emerald-50">
                <FileStack className="h-[18px] w-[18px] text-emerald-500" />
              </div>
              <div>
                <CardTitle className="text-base">{t("createWorkItem.fromTemplate")}</CardTitle>
                <CardDescription>{t("createWorkItem.fromTemplateDesc")}</CardDescription>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              {templates.map((template) => (
                <button
                  key={template.id}
                  onClick={() => handleTemplateSelect(selectedTemplateId === template.id ? null : template.id)}
                  className={cn(
                    "relative rounded-lg border p-3 text-left transition-colors hover:bg-accent",
                    selectedTemplateId === template.id && "border-emerald-500 bg-emerald-50/50 ring-1 ring-emerald-500",
                  )}
                >
                  {selectedTemplateId === template.id ? (
                    <div className="absolute right-2 top-2 flex h-5 w-5 items-center justify-center rounded-full bg-emerald-500">
                      <Check className="h-3 w-3 text-white" />
                    </div>
                  ) : null}
                  <div className="text-sm font-medium">{template.name}</div>
                  {template.description ? (
                    <div className="mt-0.5 text-xs text-muted-foreground line-clamp-1">{template.description}</div>
                  ) : null}
                  <div className="mt-2 flex items-center gap-2">
                    <Badge variant="secondary" className="text-[10px]">{t("createWorkItem.nSteps", { count: template.actions.length })}</Badge>
                    {(template.tags ?? []).slice(0, 2).map((tag) => (
                      <Badge key={tag} variant="outline" className="text-[10px]">{tag}</Badge>
                    ))}
                  </div>
                </button>
              ))}
            </div>
            {selectedTemplate ? (
              <div className="mt-3 text-xs text-muted-foreground">
                {t("createWorkItem.templateSelected", { name: selectedTemplate.name, count: selectedTemplate.actions.length })} {t("createWorkItem.templateHint")}
              </div>
            ) : null}
          </CardContent>
        </Card>
      ) : templatesLoading && !scmProvider ? (
        <div className="text-sm text-muted-foreground">{t("createWorkItem.loadingTemplates")}</div>
      ) : null}

      <div className="grid gap-6 lg:grid-cols-[1fr_380px]">
        <div className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">{t("createWorkItem.basicInfo")}</CardTitle>
              <CardDescription>{t("createWorkItem.basicInfoDesc")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-5">
              <div className="space-y-1.5">
                <label className="text-sm font-medium">{t("createWorkItem.titleLabel")}</label>
                <div className="flex gap-2">
                  <Input
                    className="flex-1"
                    placeholder={t("createWorkItem.titlePlaceholder")}
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
                    {t("createWorkItem.autoTitle")}
                  </Button>
                </div>
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <label className="text-sm font-medium">{t("createWorkItem.project")}</label>
                  <select
                    className="flex h-10 w-full rounded-md border bg-background px-3 text-sm"
                    value={projectId ?? ""}
                    onChange={(event) => setProjectId(event.target.value ? Number.parseInt(event.target.value, 10) : null)}
                  >
                    <option value="">{t("createWorkItem.allProjects")}</option>
                    {projects.map((project) => (
                      <option key={project.id} value={project.id}>
                        {project.name}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="space-y-1.5">
                  <label className="text-sm font-medium">{t("createWorkItem.draftStatus")}</label>
                  <div className="flex h-10 items-center rounded-md border bg-background px-3 text-sm">
                    {selectedTemplate
                      ? t("createWorkItem.templateDraft", { id: selectedTemplate.id })
                      : draftWorkItemId == null
                        ? t("createWorkItem.noDraft")
                        : t("createWorkItem.draftId", { id: draftWorkItemId })}
                  </div>
                </div>
              </div>

              <div className="space-y-1.5">
                <label className="text-sm font-medium">{t("createWorkItem.descriptionLabel")}</label>
                <textarea
                  className="flex min-h-[100px] w-full rounded-md border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                  placeholder={t("createWorkItem.descriptionPlaceholder")}
                  value={description}
                  onChange={(event) => setDescription(event.target.value)}
                />
              </div>

              <AttachmentUploader
                pendingFiles={pendingFiles}
                uploadedAttachments={uploadedAttachments}
                uploading={uploading}
                onFilesSelected={handleFilesSelected}
                onRemovePending={removePendingFile}
                onRemoveUploaded={(attachment) => void removeUploadedAttachment(attachment)}
              />
            </CardContent>
          </Card>

          {scmProvider ? (
            <Card>
              <CardHeader>
                <CardTitle className="text-base">{t("createWorkItem.scmFlowTitle")}</CardTitle>
                <CardDescription>
                  {t("createWorkItem.scmFlowDesc", { provider: scmProvider === "codeup" ? "Codeup CR" : "GitHub PR" })}
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
                    <CardTitle className="text-base">{t("createWorkItem.aiGenerate")}</CardTitle>
                    <CardDescription>{t("createWorkItem.aiGenerateDesc")}</CardDescription>
                  </div>
                </div>
              </CardHeader>
              <CardContent className="space-y-4">
                <textarea
                  className="flex min-h-[120px] w-full rounded-md border bg-background px-3 py-2 text-sm leading-relaxed focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                  value={aiPrompt}
                  onChange={(event) => setAiPrompt(event.target.value)}
                  placeholder={t("createWorkItem.aiPromptPlaceholder")}
                />
                <div className="flex items-center justify-between">
                  <span className="text-xs text-muted-foreground">
                    {t("createWorkItem.aiPromptHint")}
                  </span>
                  <Button
                    className="bg-indigo-500 hover:bg-indigo-600"
                    disabled={busy !== "idle"}
                    onClick={() => void generateSteps()}
                  >
                    <Sparkles className="mr-2 h-4 w-4" />
                    {busy === "generating" ? t("createWorkItem.generating") : t("createWorkItem.generateSteps")}
                  </Button>
                </div>
              </CardContent>
            </Card>
          ) : null}
        </div>

        <div className="space-y-6">
          <Card className="overflow-hidden p-0">
            <div className="flex items-center justify-between border-b px-5 py-3.5">
              <span className="text-sm font-semibold">{t("createWorkItem.stepPreview")}</span>
              <Badge variant="secondary">
                {selectedTemplate
                  ? t("createWorkItem.nSteps", { count: selectedTemplate.actions.length })
                  : t("createWorkItem.nSteps", { count: previewSteps.length })}
              </Badge>
            </div>
            <div>
              {selectedTemplate ? (
                selectedTemplate.actions.map((step, index) => {
                  const color = stepColors[step.type] ?? stepColors.exec;
                  return (
                    <div
                      key={step.name}
                      className={cn(
                        "flex items-center gap-3 px-5 py-3",
                        index < selectedTemplate.actions.length - 1 && "border-b",
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
                          {step.depends_on?.length ? ` · ${t("createWorkItem.dependsOn", { deps: step.depends_on.join(", ") })}` : ""}
                        </div>
                      </div>
                    </div>
                  );
                })
              ) : scmProvider ? (
                <div className="px-5 py-6 text-sm text-muted-foreground">
                  {t("createWorkItem.scmAutoSteps")}
                </div>
              ) : previewSteps.length === 0 ? (
                <div className="px-5 py-6 text-sm text-muted-foreground">
                  {t("createWorkItem.noSteps")}
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
                          {step.depends_on?.length ? ` · ${t("createWorkItem.dependsOn", { deps: step.depends_on.join(", ") })}` : ""}
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
              <div>{t("createWorkItem.projectLabel", { name: selectedProject?.name ?? t("createWorkItem.notSpecified") })}</div>
              {scmProvider ? (
                <div>{t("createWorkItem.scmFlow", { provider: scmProvider === "codeup" ? t("createWorkItem.codeupAuto") : t("createWorkItem.githubAuto") })}</div>
              ) : null}
              <div>
                {selectedTemplate
                  ? t("createWorkItem.sourceTemplate", { name: selectedTemplate.name })
                  : aiPrompt.trim()
                    ? t("createWorkItem.sourceAiPrompt")
                    : description.trim()
                      ? t("createWorkItem.sourceDescription")
                      : t("createWorkItem.sourceTitle")}
              </div>
            </div>
          </Card>

          <div className="space-y-2.5">
            <Button className="w-full gap-2" disabled={busy !== "idle"} onClick={() => void finalizeWorkItem(true)}>
              <Play className="h-4 w-4" />
              {busy === "running" ? t("createWorkItem.running") : t("createWorkItem.createAndRun")}
            </Button>
            <Button variant="outline" className="w-full" disabled={busy !== "idle"} onClick={() => void finalizeWorkItem(false)}>
              {busy === "saving" || busy === "from_template" ? t("createWorkItem.saving") : t("createWorkItem.saveAsDraft")}
            </Button>
            <Link to="/work-items" className="block">
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
