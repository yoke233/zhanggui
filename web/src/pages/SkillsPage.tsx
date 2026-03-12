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
  X,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
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
import { CreateSkillDialog } from "@/components/skills/CreateSkillDialog";
import { ImportGitHubDialog } from "@/components/skills/ImportGitHubDialog";
import { SkillDetailDialog } from "@/components/skills/SkillDetailDialog";
import type { SkillInfo, SkillDetail } from "@/types/apiV2";

export function SkillsPage() {
  const { t } = useTranslation();
  const { apiClient } = useWorkbench();
  const [skills, setSkills] = useState<SkillInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState("");

  const [createOpen, setCreateOpen] = useState(false);
  const [importOpen, setImportOpen] = useState(false);

  const [detailOpen, setDetailOpen] = useState(false);
  const [detailSkill, setDetailSkill] = useState<SkillDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

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
    setDetailSkill(null);
    try {
      const detail = await apiClient.getSkill(name);
      setDetailSkill(detail);
    } catch (err) {
      setError(getErrorMessage(err));
      setDetailOpen(false);
    } finally {
      setDetailLoading(false);
    }
  };

  const handleCreate = async (name: string, skillMd?: string) => {
    await apiClient.createSkill({ name, skill_md: skillMd });
    await load();
  };

  const handleImport = async (repoUrl: string, skillName: string) => {
    await apiClient.importGitHubSkill({ repo_url: repoUrl, skill_name: skillName });
    await load();
  };

  const handleSave = async (name: string, skillMd: string) => {
    await apiClient.updateSkill(name, { skill_md: skillMd });
    const refreshed = await apiClient.getSkill(name);
    setDetailSkill(refreshed);
    await load();
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setDeleteSubmitting(true);
    try {
      await apiClient.deleteSkill(deleteTarget);
      if (detailSkill?.name === deleteTarget) {
        setDetailOpen(false);
        setDetailSkill(null);
      }
      setDeleteTarget(null);
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
            {loading ? <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /> : null}
          </div>
          <p className="text-sm text-muted-foreground">{t("skills.subtitle")}</p>
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

      {/* Stats */}
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
                  <p className="line-clamp-2 text-sm text-muted-foreground">{skill.metadata.description}</p>
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
                    <Badge key={pid} variant="secondary" className="text-xs">{pid}</Badge>
                  ))}
                </div>
              ) : null}
            </CardContent>
          </Card>
        ))}
      </div>

      <CreateSkillDialog
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onCreate={handleCreate}
      />

      <ImportGitHubDialog
        open={importOpen}
        onClose={() => setImportOpen(false)}
        onImport={handleImport}
      />

      <SkillDetailDialog
        open={detailOpen}
        loading={detailLoading}
        skill={detailSkill}
        onClose={() => { setDetailOpen(false); }}
        onSave={handleSave}
        onDelete={(name) => setDeleteTarget(name)}
      />

      {/* Delete confirm */}
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
