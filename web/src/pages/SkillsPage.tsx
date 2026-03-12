import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Plus,
  Loader2,
  Search,
  Sparkles,
  Github,
  CheckCircle2,
  AlertTriangle,
  FileText,
  Trash2,
  Eye,
  Save,
  X,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
  DialogFooter,
} from "@/components/ui/dialog";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { getErrorMessage } from "@/lib/v2Workbench";
import type { SkillInfo, SkillDetail } from "@/types/apiV2";

export function SkillsPage() {
  const { t } = useTranslation();
  const { apiClient } = useWorkbench();
  const [skills, setSkills] = useState<SkillInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState("");

  // Create skill dialog
  const [createOpen, setCreateOpen] = useState(false);
  const [createName, setCreateName] = useState("");
  const [createSkillMd, setCreateSkillMd] = useState("");
  const [createSubmitting, setCreateSubmitting] = useState(false);

  // Import GitHub dialog
  const [importOpen, setImportOpen] = useState(false);
  const [importRepoUrl, setImportRepoUrl] = useState("");
  const [importSkillName, setImportSkillName] = useState("");
  const [importSubmitting, setImportSubmitting] = useState(false);

  // Detail/edit dialog
  const [detailOpen, setDetailOpen] = useState(false);
  const [detailSkill, setDetailSkill] = useState<SkillDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [editingMd, setEditingMd] = useState("");
  const [isEditing, setIsEditing] = useState(false);
  const [saveSubmitting, setSaveSubmitting] = useState(false);

  // Delete confirm
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null);
  const [deleteSubmitting, setDeleteSubmitting] = useState(false);

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      const items = await apiClient.listSkills();
      setSkills(items);
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, []);

  const filtered = useMemo(
    () =>
      skills.filter(
        (s) =>
          s.name.toLowerCase().includes(search.toLowerCase()) ||
          (s.metadata?.description ?? "").toLowerCase().includes(search.toLowerCase()),
      ),
    [skills, search],
  );

  const validCount = skills.filter((s) => s.valid).length;
  const invalidCount = skills.filter((s) => !s.valid).length;

  const openDetail = async (name: string) => {
    setDetailOpen(true);
    setDetailLoading(true);
    setIsEditing(false);
    try {
      const detail = await apiClient.getSkill(name);
      setDetailSkill(detail);
      setEditingMd(detail.skill_md ?? "");
    } catch (err) {
      setError(getErrorMessage(err));
      setDetailOpen(false);
    } finally {
      setDetailLoading(false);
    }
  };

  const handleCreate = async () => {
    if (!createName.trim()) return;
    setCreateSubmitting(true);
    try {
      await apiClient.createSkill({
        name: createName.trim(),
        skill_md: createSkillMd.trim() || undefined,
      });
      setCreateOpen(false);
      setCreateName("");
      setCreateSkillMd("");
      await load();
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setCreateSubmitting(false);
    }
  };

  const handleImport = async () => {
    if (!importRepoUrl.trim() || !importSkillName.trim()) return;
    setImportSubmitting(true);
    try {
      await apiClient.importGitHubSkill({
        repo_url: importRepoUrl.trim(),
        skill_name: importSkillName.trim(),
      });
      setImportOpen(false);
      setImportRepoUrl("");
      setImportSkillName("");
      await load();
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setImportSubmitting(false);
    }
  };

  const handleSave = async () => {
    if (!detailSkill) return;
    setSaveSubmitting(true);
    try {
      await apiClient.updateSkill(detailSkill.name, {
        skill_md: editingMd,
      });
      const refreshed = await apiClient.getSkill(detailSkill.name);
      setDetailSkill(refreshed);
      setEditingMd(refreshed.skill_md ?? "");
      setIsEditing(false);
      await load();
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setSaveSubmitting(false);
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setDeleteSubmitting(true);
    try {
      await apiClient.deleteSkill(deleteTarget);
      setDeleteTarget(null);
      if (detailSkill?.name === deleteTarget) {
        setDetailOpen(false);
        setDetailSkill(null);
      }
      await load();
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setDeleteSubmitting(false);
    }
  };

  return (
    <div className="flex-1 space-y-6 p-8">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-bold tracking-tight">{t("skills.title")}</h1>
            {loading ? (
              <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            ) : null}
          </div>
          <p className="text-sm text-muted-foreground">
            {t("skills.subtitle")}
          </p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" onClick={() => setImportOpen(true)}>
            <Github className="mr-2 h-4 w-4" />
            {t("skills.importGithub")}
          </Button>
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            {t("skills.newSkill")}
          </Button>
        </div>
      </div>

      {error ? (
        <div className="flex items-center justify-between rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
          <span>{error}</span>
          <button onClick={() => setError(null)} className="ml-2 text-rose-500 hover:text-rose-700">
            <X className="h-4 w-4" />
          </button>
        </div>
      ) : null}

      {/* Stats cards */}
      <div className="grid grid-cols-3 gap-4">
        <Card>
          <CardContent className="flex items-center gap-3 p-4">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-blue-50">
              <Sparkles className="h-5 w-5 text-blue-600" />
            </div>
            <div>
              <p className="text-2xl font-bold">{skills.length}</p>
              <p className="text-xs text-muted-foreground">{t("skills.allSkills")}</p>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="flex items-center gap-3 p-4">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-emerald-50">
              <CheckCircle2 className="h-5 w-5 text-emerald-600" />
            </div>
            <div>
              <p className="text-2xl font-bold">{validCount}</p>
              <p className="text-xs text-muted-foreground">{t("skills.valid")}</p>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="flex items-center gap-3 p-4">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-amber-50">
              <AlertTriangle className="h-5 w-5 text-amber-600" />
            </div>
            <div>
              <p className="text-2xl font-bold">{invalidCount}</p>
              <p className="text-xs text-muted-foreground">{t("skills.needsFix")}</p>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Search */}
      <div className="relative max-w-sm">
        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          placeholder={t("skills.searchPlaceholder")}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="pl-9"
        />
      </div>

      {/* Skills grid */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {filtered.length === 0 && !loading ? (
          <div className="col-span-full py-12 text-center text-muted-foreground">
            {search ? t("skills.noMatchingSkills") : t("skills.noSkillsHint")}
          </div>
        ) : null}
        {filtered.map((skill) => (
          <Card
            key={skill.name}
            className="group cursor-pointer transition-shadow hover:shadow-md"
            onClick={() => void openDetail(skill.name)}
          >
            <CardHeader className="pb-2">
              <div className="flex items-start justify-between">
                <CardTitle className="flex items-center gap-2 text-sm font-semibold">
                  <FileText className="h-4 w-4 text-muted-foreground" />
                  {skill.name}
                </CardTitle>
                <div className="flex items-center gap-1">
                  {skill.valid ? (
                    <Badge variant="success" className="text-xs">{t("skills.valid")}</Badge>
                  ) : (
                    <Badge variant="warning" className="text-xs">{t("skills.invalid")}</Badge>
                  )}
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-3">
              {skill.metadata ? (
                <>
                  <p className="line-clamp-2 text-sm text-muted-foreground">
                    {skill.metadata.description}
                  </p>
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <Badge variant="outline" className="text-xs">v{skill.metadata.version}</Badge>
                  </div>
                </>
              ) : (
                <p className="text-sm italic text-muted-foreground">{t("skills.missingMeta")}</p>
              )}

              {skill.validation_errors && skill.validation_errors.length > 0 ? (
                <div className="space-y-1">
                  {skill.validation_errors.map((err, i) => (
                    <p key={i} className="text-xs text-rose-600">
                      <AlertTriangle className="mr-1 inline h-3 w-3" />
                      {err}
                    </p>
                  ))}
                </div>
              ) : null}

              {skill.profiles_using && skill.profiles_using.length > 0 ? (
                <div className="flex flex-wrap gap-1">
                  {skill.profiles_using.map((pid) => (
                    <Badge key={pid} variant="secondary" className="text-xs">
                      {pid}
                    </Badge>
                  ))}
                </div>
              ) : null}
            </CardContent>
          </Card>
        ))}
      </div>

      {/* Create Skill Dialog */}
      <Dialog open={createOpen} onClose={() => setCreateOpen(false)} className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("skills.createTitle")}</DialogTitle>
          <DialogDescription>
            {t("skills.createDesc")}
          </DialogDescription>
        </DialogHeader>
        <DialogBody>
          <div className="space-y-1.5">
            <label className="text-sm font-medium">{t("skills.skillName")}</label>
            <Input
              placeholder={t("skills.skillNamePlaceholder")}
              value={createName}
              onChange={(e) => setCreateName(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              {t("skills.skillNameHint")}
            </p>
          </div>
          <div className="space-y-1.5">
            <label className="text-sm font-medium">
              {t("skills.skillMdLabel")} <span className="font-normal text-muted-foreground">{t("skills.skillMdOptional")}</span>
            </label>
            <Textarea
              placeholder={"---\nname: code-review\ndescription: Code review skill\nassign_when: When code review is needed\nversion: 1\n---\n\n# Skill Description\n\n..."}
              value={createSkillMd}
              onChange={(e) => setCreateSkillMd(e.target.value)}
              className="min-h-[200px] font-mono text-xs"
            />
            <p className="text-xs text-muted-foreground">
              {t("skills.skillMdHint")}
            </p>
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => setCreateOpen(false)}>{t("common.cancel")}</Button>
          <Button onClick={() => void handleCreate()} disabled={!createName.trim() || createSubmitting}>
            {createSubmitting ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
            {t("skills.createSkill")}
          </Button>
        </DialogFooter>
      </Dialog>

      {/* Import GitHub Dialog */}
      <Dialog open={importOpen} onClose={() => setImportOpen(false)} className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("skills.importTitle")}</DialogTitle>
          <DialogDescription>
            {t("skills.importDesc")}
          </DialogDescription>
        </DialogHeader>
        <DialogBody>
          <div className="space-y-1.5">
            <label className="text-sm font-medium">{t("skills.githubUrl")}</label>
            <Input
              placeholder="https://github.com/owner/repo"
              value={importRepoUrl}
              onChange={(e) => setImportRepoUrl(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <label className="text-sm font-medium">{t("skills.importSkillName")}</label>
            <Input
              placeholder={t("skills.importNamePlaceholder")}
              value={importSkillName}
              onChange={(e) => setImportSkillName(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              {t("skills.importNameHint")}
            </p>
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => setImportOpen(false)}>{t("common.cancel")}</Button>
          <Button
            onClick={() => void handleImport()}
            disabled={!importRepoUrl.trim() || !importSkillName.trim() || importSubmitting}
          >
            {importSubmitting ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
            {t("skills.import")}
          </Button>
        </DialogFooter>
      </Dialog>

      {/* Skill Detail / Edit Dialog */}
      <Dialog
        open={detailOpen}
        onClose={() => {
          setDetailOpen(false);
          setIsEditing(false);
        }}
        className="max-w-2xl"
      >
        {detailLoading ? (
          <div className="flex items-center justify-center py-16">
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          </div>
        ) : detailSkill ? (
          <>
            <DialogHeader>
              <div className="flex items-center gap-3">
                <DialogTitle>{detailSkill.name}</DialogTitle>
                {detailSkill.valid ? (
                  <Badge variant="success">{t("skills.valid")}</Badge>
                ) : (
                  <Badge variant="warning">{t("skills.invalid")}</Badge>
                )}
              </div>
              {detailSkill.metadata ? (
                <DialogDescription>{detailSkill.metadata.description}</DialogDescription>
              ) : null}
            </DialogHeader>
            <DialogBody>
              {/* Metadata summary */}
              {detailSkill.metadata ? (
                <div className="grid grid-cols-2 gap-3 text-sm">
                  <div>
                    <span className="text-muted-foreground">{t("skills.triggerCondition")}</span>
                    <span>{detailSkill.metadata.assign_when}</span>
                  </div>
                  <div>
                    <span className="text-muted-foreground">{t("skills.version")}</span>
                    <span>v{detailSkill.metadata.version}</span>
                  </div>
                </div>
              ) : null}

              {/* Validation errors */}
              {detailSkill.validation_errors && detailSkill.validation_errors.length > 0 ? (
                <div className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3">
                  <p className="mb-1 text-sm font-medium text-amber-800">{t("skills.validationErrors")}</p>
                  {detailSkill.validation_errors.map((err, i) => (
                    <p key={i} className="text-xs text-amber-700">{err}</p>
                  ))}
                </div>
              ) : null}

              {/* Profiles using */}
              {detailSkill.profiles_using && detailSkill.profiles_using.length > 0 ? (
                <div>
                  <p className="mb-1.5 text-sm font-medium">{t("skills.profilesUsing")}</p>
                  <div className="flex flex-wrap gap-1">
                    {detailSkill.profiles_using.map((pid) => (
                      <Badge key={pid} variant="info" className="text-xs">{pid}</Badge>
                    ))}
                  </div>
                </div>
              ) : null}

              {/* SKILL.md content */}
              <div>
                <div className="mb-1.5 flex items-center justify-between">
                  <p className="text-sm font-medium">SKILL.md</p>
                  {!isEditing ? (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => {
                        setEditingMd(detailSkill.skill_md ?? "");
                        setIsEditing(true);
                      }}
                    >
                      <Eye className="mr-1 h-3.5 w-3.5" />
                      {t("skills.editBtn")}
                    </Button>
                  ) : null}
                </div>
                {isEditing ? (
                  <Textarea
                    value={editingMd}
                    onChange={(e) => setEditingMd(e.target.value)}
                    className="min-h-[300px] font-mono text-xs"
                  />
                ) : (
                  <pre className="max-h-[300px] overflow-auto rounded-lg border bg-slate-50 p-4 text-xs">
                    {detailSkill.skill_md || t("skills.empty")}
                  </pre>
                )}
              </div>
            </DialogBody>
            <DialogFooter>
              <Button
                variant="destructive"
                size="sm"
                onClick={() => setDeleteTarget(detailSkill.name)}
                disabled={
                  (detailSkill.profiles_using && detailSkill.profiles_using.length > 0) || false
                }
                title={
                  detailSkill.profiles_using && detailSkill.profiles_using.length > 0
                    ? t("skills.skillInUse")
                    : undefined
                }
              >
                <Trash2 className="mr-1 h-3.5 w-3.5" />
                {t("common.delete")}
              </Button>
              <div className="flex-1" />
              {isEditing ? (
                <>
                  <Button variant="outline" onClick={() => setIsEditing(false)}>
                    {t("skills.cancelEdit")}
                  </Button>
                  <Button onClick={() => void handleSave()} disabled={saveSubmitting}>
                    {saveSubmitting ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : (
                      <Save className="mr-1 h-3.5 w-3.5" />
                    )}
                    {t("common.save")}
                  </Button>
                </>
              ) : (
                <Button variant="outline" onClick={() => setDetailOpen(false)}>
                  {t("common.close")}
                </Button>
              )}
            </DialogFooter>
          </>
        ) : null}
      </Dialog>

      {/* Delete confirm dialog */}
      <Dialog open={!!deleteTarget} onClose={() => setDeleteTarget(null)} className="max-w-sm">
        <DialogHeader>
          <DialogTitle>{t("skills.confirmDelete")}</DialogTitle>
          <DialogDescription>
            {t("skills.confirmDeletePrefix")}<strong>{deleteTarget}</strong>{t("skills.confirmDeleteSuffix")}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => setDeleteTarget(null)}>{t("common.cancel")}</Button>
          <Button variant="destructive" onClick={() => void handleDelete()} disabled={deleteSubmitting}>
            {deleteSubmitting ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
            {t("skills.confirmDeleteBtn")}
          </Button>
        </DialogFooter>
      </Dialog>
    </div>
  );
}
