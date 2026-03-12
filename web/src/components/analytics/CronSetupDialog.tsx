import { useState } from "react";
import { useTranslation } from "react-i18next";
import { AlertTriangle, CalendarClock, Loader2 } from "lucide-react";
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
import { parseCronExpr } from "@/lib/cronParser";

interface Props {
  open: boolean;
  onClose: () => void;
  onSave: (issueId: number, schedule: string, maxInstances: number) => Promise<void>;
}

export function CronSetupDialog({ open, onClose, onSave }: Props) {
  const { t } = useTranslation();
  const [form, setForm] = useState({ issueId: "", schedule: "", maxInstances: "1" });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleClose = () => {
    setForm({ issueId: "", schedule: "", maxInstances: "1" });
    setError(null);
    onClose();
  };

  const handleSave = async () => {
    setSaving(true);
    setError(null);
    try {
      await onSave(Number(form.issueId), form.schedule, Number(form.maxInstances) || 1);
      handleClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };

  const cronResult = form.schedule.trim() ? parseCronExpr(form.schedule, t) : null;
  const canSave = !saving && !!form.issueId && !!form.schedule && (cronResult?.valid ?? false);

  return (
    <Dialog open={open} onClose={handleClose}>
      <DialogHeader>
        <DialogTitle>{t("analytics.addCronTitle")}</DialogTitle>
        <DialogDescription>{t("analytics.addCronDesc")}</DialogDescription>
      </DialogHeader>
      <DialogBody>
        {error ? (
          <p className="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">{error}</p>
        ) : null}
        <div className="space-y-4">
          <div className="space-y-1.5">
            <label className="text-sm font-medium">{t("analytics.flowId")}</label>
            <Input
              type="number"
              placeholder={t("analytics.enterFlowId")}
              value={form.issueId}
              onChange={(e) => setForm((f) => ({ ...f, issueId: e.target.value }))}
            />
          </div>
          <div className="space-y-1.5">
            <label className="text-sm font-medium">{t("analytics.cronExpression")}</label>
            <Input
              placeholder={t("analytics.cronPlaceholder")}
              value={form.schedule}
              onChange={(e) => setForm((f) => ({ ...f, schedule: e.target.value }))}
              className={
                form.schedule.trim()
                  ? cronResult?.valid
                    ? "border-emerald-300 focus:border-emerald-400"
                    : "border-rose-300 focus:border-rose-400"
                  : undefined
              }
            />
            {cronResult ? (
              cronResult.valid ? (
                <p className="flex items-center gap-1.5 rounded-md bg-emerald-50 px-2.5 py-1.5 text-xs text-emerald-700">
                  <CalendarClock className="h-3 w-3 shrink-0" />
                  {cronResult.description}
                </p>
              ) : (
                <p className="flex items-center gap-1.5 rounded-md bg-rose-50 px-2.5 py-1.5 text-xs text-rose-600">
                  <AlertTriangle className="h-3 w-3 shrink-0" />
                  {cronResult.error}
                </p>
              )
            ) : (
              <p className="text-xs text-muted-foreground">{t("analytics.cronHelp")}</p>
            )}
          </div>
          <div className="space-y-1.5">
            <label className="text-sm font-medium">{t("analytics.maxConcurrentInstances")}</label>
            <Input
              type="number"
              min={1}
              max={10}
              value={form.maxInstances}
              onChange={(e) => setForm((f) => ({ ...f, maxInstances: e.target.value }))}
            />
          </div>
        </div>
      </DialogBody>
      <DialogFooter>
        <Button variant="outline" onClick={handleClose}>{t("common.cancel")}</Button>
        <Button disabled={!canSave} onClick={() => void handleSave()}>
          {saving ? <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" /> : null}
          {t("common.confirm")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
}
