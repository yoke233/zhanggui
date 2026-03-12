import { useState } from "react";
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
import type { AgentDriver } from "@/types/apiV2";

interface ProfilePayload {
  id: string;
  name: string;
  driver_id: string;
  role: string;
  capabilities: string[];
  actions_allowed: string[];
  session: { reuse: boolean; max_turns: number };
}

interface Props {
  open: boolean;
  drivers: AgentDriver[];
  onClose: () => void;
  onCreate: (payload: ProfilePayload) => Promise<void>;
}

export function CreateProfileDialog({ open, drivers, onClose, onCreate }: Props) {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [role, setRole] = useState("worker");
  const [driverId, setDriverId] = useState(() => drivers[0]?.id ?? "");
  const [caps, setCaps] = useState("backend,frontend");
  const [actions, setActions] = useState("read_context,search_files,fs_write,terminal,submit,mark_blocked,request_help");
  const [maxTurns, setMaxTurns] = useState("12");
  const [submitting, setSubmitting] = useState(false);

  const handleClose = () => {
    setName("");
    setRole("worker");
    setDriverId(drivers[0]?.id ?? "");
    setCaps("backend,frontend");
    setActions("read_context,search_files,fs_write,terminal,submit,mark_blocked,request_help");
    setMaxTurns("12");
    onClose();
  };

  const handleCreate = async () => {
    setSubmitting(true);
    try {
      await onCreate({
        id: name.trim(),
        name: name.trim(),
        driver_id: driverId,
        role,
        capabilities: caps.split(",").map((s) => s.trim()).filter(Boolean),
        actions_allowed: actions.split(",").map((s) => s.trim()).filter(Boolean),
        session: { reuse: true, max_turns: Number.parseInt(maxTurns, 10) || 12 },
      });
      handleClose();
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onClose={handleClose} className="max-w-lg">
      <DialogHeader>
        <DialogTitle>{t("agents.newProfile")}</DialogTitle>
        <DialogDescription>{t("agents.createProfileDesc")}</DialogDescription>
      </DialogHeader>
      <DialogBody>
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-1.5">
            <label className="text-sm font-medium">{t("agents.profileId")}</label>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="space-y-1.5">
            <label className="text-sm font-medium">{t("agents.role")}</label>
            <select
              className="flex h-10 w-full rounded-md border bg-background px-3 text-sm"
              value={role}
              onChange={(e) => setRole(e.target.value)}
            >
              <option value="lead">lead</option>
              <option value="worker">worker</option>
              <option value="gate">gate</option>
              <option value="support">support</option>
            </select>
          </div>
        </div>
        <div className="space-y-1.5">
          <label className="text-sm font-medium">{t("agents.bindDriver")}</label>
          <select
            className="flex h-10 w-full rounded-md border bg-background px-3 text-sm"
            value={driverId}
            onChange={(e) => setDriverId(e.target.value)}
          >
            {drivers.map((d) => (
              <option key={d.id} value={d.id}>{d.id}</option>
            ))}
          </select>
        </div>
        <div className="space-y-1.5">
          <label className="text-sm font-medium">{t("agents.capabilityTagsComma")}</label>
          <Input value={caps} onChange={(e) => setCaps(e.target.value)} />
        </div>
        <div className="space-y-1.5">
          <label className="text-sm font-medium">{t("agents.allowedActionsComma")}</label>
          <Input value={actions} onChange={(e) => setActions(e.target.value)} />
        </div>
        <div className="space-y-1.5">
          <label className="text-sm font-medium">{t("agents.maxTurns")}</label>
          <Input value={maxTurns} onChange={(e) => setMaxTurns(e.target.value)} />
        </div>
      </DialogBody>
      <DialogFooter>
        <Button variant="outline" onClick={handleClose}>{t("common.cancel")}</Button>
        <Button onClick={() => void handleCreate()} disabled={!name.trim() || !driverId || submitting}>
          {t("agents.createProfile")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
}
