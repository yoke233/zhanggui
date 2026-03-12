import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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
  onImport: (repoUrl: string, skillName: string) => Promise<void>;
}

export function ImportGitHubDialog({ open, onClose, onImport }: Props) {
  const { t } = useTranslation();
  const [repoUrl, setRepoUrl] = useState("");
  const [skillName, setSkillName] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const handleClose = () => {
    setRepoUrl("");
    setSkillName("");
    onClose();
  };

  const handleImport = async () => {
    if (!repoUrl.trim() || !skillName.trim()) return;
    setSubmitting(true);
    try {
      await onImport(repoUrl.trim(), skillName.trim());
      handleClose();
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onClose={handleClose} className="max-w-lg">
      <DialogHeader>
        <DialogTitle>{t("skills.importTitle")}</DialogTitle>
        <DialogDescription>{t("skills.importDesc")}</DialogDescription>
      </DialogHeader>
      <DialogBody>
        <div className="space-y-1.5">
          <label className="text-sm font-medium">{t("skills.githubUrl")}</label>
          <Input
            placeholder="https://github.com/owner/repo"
            value={repoUrl}
            onChange={(e) => setRepoUrl(e.target.value)}
          />
        </div>
        <div className="space-y-1.5">
          <label className="text-sm font-medium">{t("skills.importSkillName")}</label>
          <Input
            placeholder={t("skills.importNamePlaceholder")}
            value={skillName}
            onChange={(e) => setSkillName(e.target.value)}
          />
          <p className="text-xs text-muted-foreground">{t("skills.importNameHint")}</p>
        </div>
      </DialogBody>
      <DialogFooter>
        <Button variant="outline" onClick={handleClose}>{t("common.cancel")}</Button>
        <Button
          onClick={() => void handleImport()}
          disabled={!repoUrl.trim() || !skillName.trim() || submitting}
        >
          {submitting ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
          {t("skills.import")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
}
