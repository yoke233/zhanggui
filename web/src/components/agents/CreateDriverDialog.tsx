import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
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
import { cn } from "@/lib/utils";
import type { DriverConfig } from "@/types/apiV2";

const ALL_CAPS = ["fs_read", "fs_write", "terminal"] as const;
type Cap = (typeof ALL_CAPS)[number];

interface DriverPayload {
  id: string;
  launch_command: string;
  launch_args: string[];
  capabilities_max: Record<Cap, boolean>;
}

interface Props {
  open: boolean;
  onClose: () => void;
  driver?: DriverConfig | null;
  onSubmit: (payload: DriverPayload) => Promise<void>;
}

export function CreateDriverDialog({ open, onClose, driver, onSubmit }: Props) {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [cmd, setCmd] = useState("");
  const [args, setArgs] = useState("");
  const [caps, setCaps] = useState<Cap[]>(["fs_read", "fs_write", "terminal"]);
  const [submitting, setSubmitting] = useState(false);
  const isEditing = Boolean(driver);

  useEffect(() => {
    if (!open) {
      return;
    }
    setName(driver?.id ?? "");
    setCmd(driver?.launch_command ?? "");
    setArgs((driver?.launch_args ?? []).join(" "));
    setCaps(ALL_CAPS.filter((cap) => driver?.capabilities_max?.[cap] ?? true));
  }, [driver, open]);

  const handleClose = () => {
    setName("");
    setCmd("");
    setArgs("");
    setCaps(["fs_read", "fs_write", "terminal"]);
    onClose();
  };

  const toggleCap = (cap: Cap) => {
    setCaps((prev) => prev.includes(cap) ? prev.filter((c) => c !== cap) : [...prev, cap]);
  };

  const handleCreate = async () => {
    setSubmitting(true);
    try {
      await onSubmit({
        id: name.trim(),
        launch_command: cmd.trim(),
        launch_args: args.split(" ").map((s) => s.trim()).filter(Boolean),
        capabilities_max: {
          fs_read: caps.includes("fs_read"),
          fs_write: caps.includes("fs_write"),
          terminal: caps.includes("terminal"),
        },
      });
      handleClose();
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onClose={handleClose} className="max-w-md">
      <DialogHeader>
        <DialogTitle>{isEditing ? t("common.edit") : t("agents.newDriver")}</DialogTitle>
        <DialogDescription>{t("agents.createDriverDesc")}</DialogDescription>
      </DialogHeader>
      <DialogBody>
        <div className="space-y-1.5">
          <label className="text-sm font-medium">{t("agents.driverId")}</label>
          <Input value={name} onChange={(e) => setName(e.target.value)} disabled={isEditing} />
        </div>
        <div className="space-y-1.5">
          <label className="text-sm font-medium">{t("agents.launchCommand")}</label>
          <Input value={cmd} onChange={(e) => setCmd(e.target.value)} />
        </div>
        <div className="space-y-1.5">
          <label className="text-sm font-medium">{t("agents.launchArgs")}</label>
          <Input value={args} onChange={(e) => setArgs(e.target.value)} />
        </div>
        <div className="space-y-2">
          <label className="text-sm font-medium">{t("agents.maxCapabilities")}</label>
          <div className="flex gap-4">
            {ALL_CAPS.map((cap) => (
              <label key={cap} className="flex cursor-pointer items-center gap-2">
                <button
                  type="button"
                  onClick={() => toggleCap(cap)}
                  className={cn(
                    "flex h-[18px] w-[18px] items-center justify-center rounded transition-colors",
                    caps.includes(cap) ? "bg-primary text-primary-foreground" : "border border-input",
                  )}
                >
                  {caps.includes(cap) ? "✓" : ""}
                </button>
                <span className="text-sm">{cap}</span>
              </label>
            ))}
          </div>
        </div>
      </DialogBody>
      <DialogFooter>
        <Button variant="outline" onClick={handleClose}>{t("common.cancel")}</Button>
        <Button onClick={() => void handleCreate()} disabled={!name.trim() || !cmd.trim() || submitting}>
          {isEditing ? t("common.save") : t("agents.createDriver")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
}
