import type React from "react";
import { useTranslation } from "react-i18next";
import { Paperclip, Send, X } from "lucide-react";
import type { AgentDriver, AgentProfile } from "@/types/apiV2";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import type { LeadDriverOption } from "./chatTypes";
import { EMPTY_PROFILE_VALUE } from "./chatTypes";

interface DraftSessionSetupProps {
  projects: Array<{ id: number; name: string }>;
  draftProjectId: number | null;
  draftProfileId: string;
  draftDriverId: string;
  leadDriverOptions: LeadDriverOption[];
  leadProfiles: AgentProfile[];
  drivers: AgentDriver[];
  messageInput: string;
  pendingFiles: File[];
  draftSessionReady: boolean;
  submitting: boolean;
  currentDriverLabel: string;
  currentProjectLabel: string;
  fileInputRef: React.RefObject<HTMLInputElement>;
  onProjectChange: (id: number | null) => void;
  onProfileChange: (id: string) => void;
  onDriverChange: (id: string) => void;
  onMessageChange: (value: string) => void;
  onSend: () => void;
  onPaste: (e: React.ClipboardEvent) => void;
  onRemovePendingFile: (index: number) => void;
}

export function DraftSessionSetup(props: DraftSessionSetupProps) {
  const {
    projects,
    draftProjectId,
    draftProfileId,
    draftDriverId,
    leadDriverOptions,
    leadProfiles,
    drivers,
    messageInput,
    pendingFiles,
    draftSessionReady,
    submitting,
    currentDriverLabel,
    currentProjectLabel,
    fileInputRef,
    onProjectChange,
    onProfileChange,
    onDriverChange,
    onMessageChange,
    onSend,
    onPaste,
    onRemovePendingFile,
  } = props;
  const { t } = useTranslation();

  return (
    <div className="w-full max-w-[860px] rounded-[28px] border bg-gradient-to-br from-white via-slate-50 to-slate-100 p-7 shadow-sm">
      <div className="space-y-6">
        <div className="space-y-2">
          <p className="text-2xl font-semibold tracking-tight text-foreground">{t("chat.newSession")}</p>
          <p className="text-sm text-muted-foreground">{t("chat.newSessionHint")}</p>
        </div>
        <div className="grid gap-4 md:grid-cols-3">
          <div className="space-y-2">
            <label className="text-xs font-medium uppercase tracking-[0.18em] text-slate-500">{t("common.project")}</label>
            <Select
              value={draftProjectId == null ? "" : String(draftProjectId)}
              onValueChange={(next) => {
                const nextProjectId = next ? Number(next) : null;
                onProjectChange(nextProjectId);
              }}
            >
              <option value="">{t("chat.noProject")}</option>
              {projects.map((project) => (
                <option key={project.id} value={project.id}>{project.name}</option>
              ))}
            </Select>
          </div>
          <div className="space-y-2">
            <label className="text-xs font-medium uppercase tracking-[0.18em] text-slate-500">Profile</label>
            <Select
              value={draftProfileId || ""}
              onValueChange={(v) => onProfileChange(v)}
            >
              {leadProfiles.map((profile) => (
                <option key={profile.id} value={profile.id}>
                  {profile.name || profile.id} ({profile.role})
                </option>
              ))}
            </Select>
          </div>
          <div className="space-y-2">
            <label className="text-xs font-medium uppercase tracking-[0.18em] text-slate-500">Driver</label>
            <Select
              value={draftDriverId || EMPTY_PROFILE_VALUE}
              onValueChange={(next) => {
                onDriverChange(next === EMPTY_PROFILE_VALUE ? "" : next);
              }}
            >
              <option value={EMPTY_PROFILE_VALUE}>{t("chat.selectDriver")}</option>
              {leadDriverOptions.map((option) => (
                <option key={option.driverId} value={option.driverId}>
                  {option.label}
                </option>
              ))}
            </Select>
          </div>
        </div>
        <div className="space-y-3">
          <Textarea
            placeholder={t("chat.inputPlaceholderNew", { driver: currentDriverLabel, project: currentProjectLabel })}
            className="min-h-[180px] resize-none bg-white/90"
            value={messageInput}
            disabled={submitting || !draftSessionReady}
            onChange={(event) => onMessageChange(event.target.value)}
            onPaste={onPaste}
            onKeyDown={(event) => {
              if (event.key === "Enter" && !event.shiftKey) {
                event.preventDefault();
                onSend();
              }
            }}
          />
          {pendingFiles.length > 0 && (
            <div className="flex flex-wrap gap-2">
              {pendingFiles.map((file, idx) => (
                <Badge key={idx} variant="secondary" className="gap-1 text-xs">
                  {file.name}
                  <button type="button" onClick={() => onRemovePendingFile(idx)} className="ml-1 hover:text-red-500">
                    <X className="h-3 w-3" />
                  </button>
                </Badge>
              ))}
            </div>
          )}
          <div className="flex items-center justify-between gap-3">
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant="secondary" className="text-[10px]">
                {t("chat.projectDot")}{currentProjectLabel}
              </Badge>
              <Badge variant="secondary" className="text-[10px]">
                {leadProfiles.find((p) => p.id === draftProfileId)?.name || draftProfileId || "–"} · {currentDriverLabel}
              </Badge>
            </div>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="icon"
                className="h-10 w-10 shrink-0"
                disabled={submitting || !draftSessionReady}
                onClick={() => fileInputRef.current?.click()}
                title={t("chat.uploadFile")}
              >
                <Paperclip className="h-4 w-4" />
              </Button>
              <Button
                className="h-10 gap-2 px-4"
                disabled={submitting || !draftSessionReady}
                onClick={onSend}
              >
                <Send className="h-4 w-4" />
                {t("chat.send")}
              </Button>
            </div>
          </div>
          <div className="text-[10px] text-muted-foreground">{t("chat.inputHint")}</div>
        </div>
        {leadProfiles.length === 0 ? (
          <div className="rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
            {t("chat.noProfileAvailable")}
          </div>
        ) : drivers.length === 0 ? (
          <div className="rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
            {t("chat.noDriverAvailable")}
          </div>
        ) : null}
      </div>
    </div>
  );
}
