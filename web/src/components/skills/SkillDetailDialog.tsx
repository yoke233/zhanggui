import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Eye, Loader2, Save, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
  DialogFooter,
} from "@/components/ui/dialog";
import type { SkillDetail } from "@/types/apiV2";

interface Props {
  open: boolean;
  loading: boolean;
  skill: SkillDetail | null;
  onClose: () => void;
  onSave: (name: string, skillMd: string) => Promise<void>;
  onDelete: (name: string) => void;
}

export function SkillDetailDialog({ open, loading, skill, onClose, onSave, onDelete }: Props) {
  const { t } = useTranslation();
  const [isEditing, setIsEditing] = useState(false);
  const [editingMd, setEditingMd] = useState("");
  const [saveSubmitting, setSaveSubmitting] = useState(false);

  const handleClose = () => {
    setIsEditing(false);
    onClose();
  };

  const startEdit = () => {
    setEditingMd(skill?.skill_md ?? "");
    setIsEditing(true);
  };

  const handleSave = async () => {
    if (!skill) return;
    setSaveSubmitting(true);
    try {
      await onSave(skill.name, editingMd);
      setIsEditing(false);
    } finally {
      setSaveSubmitting(false);
    }
  };

  const inUse = (skill?.profiles_using?.length ?? 0) > 0;

  return (
    <Dialog
      open={open}
      onClose={handleClose}
      className="max-w-2xl"
    >
      {loading ? (
        <div className="flex items-center justify-center py-16">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      ) : skill ? (
        <>
          <DialogHeader>
            <div className="flex items-center gap-3">
              <DialogTitle>{skill.name}</DialogTitle>
              {skill.valid ? (
                <Badge variant="success">{t("skills.valid")}</Badge>
              ) : (
                <Badge variant="warning">{t("skills.invalid")}</Badge>
              )}
            </div>
            {skill.metadata ? (
              <DialogDescription>{skill.metadata.description}</DialogDescription>
            ) : null}
          </DialogHeader>
          <DialogBody>
            {skill.metadata ? (
              <div className="grid grid-cols-2 gap-3 text-sm">
                <div>
                  <span className="text-muted-foreground">{t("skills.triggerCondition")}</span>
                  <span>{skill.metadata.assign_when}</span>
                </div>
                <div>
                  <span className="text-muted-foreground">{t("skills.version")}</span>
                  <span>v{skill.metadata.version}</span>
                </div>
              </div>
            ) : null}

            {skill.validation_errors && skill.validation_errors.length > 0 ? (
              <div className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3">
                <p className="mb-1 text-sm font-medium text-amber-800">{t("skills.validationErrors")}</p>
                {skill.validation_errors.map((err, i) => (
                  <p key={i} className="text-xs text-amber-700">{err}</p>
                ))}
              </div>
            ) : null}

            {skill.profiles_using && skill.profiles_using.length > 0 ? (
              <div>
                <p className="mb-1.5 text-sm font-medium">{t("skills.profilesUsing")}</p>
                <div className="flex flex-wrap gap-1">
                  {skill.profiles_using.map((pid) => (
                    <Badge key={pid} variant="info" className="text-xs">{pid}</Badge>
                  ))}
                </div>
              </div>
            ) : null}

            <div>
              <div className="mb-1.5 flex items-center justify-between">
                <p className="text-sm font-medium">SKILL.md</p>
                {!isEditing ? (
                  <Button variant="ghost" size="sm" onClick={startEdit}>
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
                  {skill.skill_md || t("skills.empty")}
                </pre>
              )}
            </div>
          </DialogBody>
          <DialogFooter>
            <Button
              variant="destructive"
              size="sm"
              onClick={() => onDelete(skill.name)}
              disabled={inUse}
              title={inUse ? t("skills.skillInUse") : undefined}
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
                  {saveSubmitting ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <Save className="mr-1 h-3.5 w-3.5" />
                  )}
                  {t("common.save")}
                </Button>
              </>
            ) : (
              <Button variant="outline" onClick={handleClose}>
                {t("common.close")}
              </Button>
            )}
          </DialogFooter>
        </>
      ) : null}
    </Dialog>
  );
}
