import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
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

interface Props {
  open: boolean;
  onClose: () => void;
  onCreate: (name: string, skillMd?: string) => Promise<void>;
}

export function CreateSkillDialog({ open, onClose, onCreate }: Props) {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [skillMd, setSkillMd] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const handleClose = () => {
    setName("");
    setSkillMd("");
    onClose();
  };

  const handleCreate = async () => {
    if (!name.trim()) return;
    setSubmitting(true);
    try {
      await onCreate(name.trim(), skillMd.trim() || undefined);
      handleClose();
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onClose={handleClose} className="max-w-lg">
      <DialogHeader>
        <DialogTitle>{t("skills.createTitle")}</DialogTitle>
        <DialogDescription>{t("skills.createDesc")}</DialogDescription>
      </DialogHeader>
      <DialogBody>
        <div className="space-y-1.5">
          <label className="text-sm font-medium">{t("skills.skillName")}</label>
          <Input
            placeholder={t("skills.skillNamePlaceholder")}
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
          <p className="text-xs text-muted-foreground">{t("skills.skillNameHint")}</p>
        </div>
        <div className="space-y-1.5">
          <label className="text-sm font-medium">
            {t("skills.skillMdLabel")}{" "}
            <span className="font-normal text-muted-foreground">{t("skills.skillMdOptional")}</span>
          </label>
          <Textarea
            placeholder={"---\nname: code-review\ndescription: Code review skill\nassign_when: When code review is needed\nversion: 1\n---\n\n# Skill Description\n\n..."}
            value={skillMd}
            onChange={(e) => setSkillMd(e.target.value)}
            className="min-h-[200px] font-mono text-xs"
          />
          <p className="text-xs text-muted-foreground">{t("skills.skillMdHint")}</p>
        </div>
      </DialogBody>
      <DialogFooter>
        <Button variant="outline" onClick={handleClose}>{t("common.cancel")}</Button>
        <Button onClick={() => void handleCreate()} disabled={!name.trim() || submitting}>
          {submitting ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
          {t("skills.createSkill")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
}
