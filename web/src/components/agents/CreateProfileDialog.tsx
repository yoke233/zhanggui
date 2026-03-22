import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogBody,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Select, SelectItem } from "@/components/ui/select";
import type { DriverConfig, AgentProfile, SkillInfo } from "@/types/apiV2";
import type { LLMConfigItem } from "@/types/system";

type ProfileLLMMode = "system" | LLMConfigItem["type"];

const PROFILE_LLM_MODE_OPTIONS: Array<{ value: ProfileLLMMode; label: string }> = [
  { value: "system", label: "System" },
  { value: "openai_chat_completion", label: "OpenAI ChatCompletion" },
  { value: "openai_response", label: "OpenAI Response" },
  { value: "anthropic", label: "Anthropic" },
];

const DEFAULT_CAPS = "backend,frontend";
const DEFAULT_ACTIONS = "read_context,search_files,fs_write,terminal,submit,mark_blocked,request_help";
const DEFAULT_MAX_TURNS = "12";

function normalizeTextValue(value: unknown): string {
  if (typeof value === "string") {
    return value;
  }
  if (value == null) {
    return "";
  }
  return String(value);
}

function parseDurationToNanoseconds(input: string): number | null {
  const value = input.trim();
  if (!value) {
    return null;
  }
  if (/^\d+$/.test(value)) {
    return Number(value);
  }

  const unitMap: Record<string, number> = {
    ns: 1,
    us: 1_000,
    "µs": 1_000,
    ms: 1_000_000,
    s: 1_000_000_000,
    m: 60 * 1_000_000_000,
    h: 60 * 60 * 1_000_000_000,
  };

  const pattern = /(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)/g;
  let total = 0;
  let consumed = 0;
  let match: RegExpExecArray | null;

  while ((match = pattern.exec(value)) !== null) {
    if (match.index !== consumed) {
      return Number.NaN;
    }
    total += Number(match[1]) * unitMap[match[2]];
    consumed += match[0].length;
  }

  if (consumed !== value.length) {
    return Number.NaN;
  }

  return Math.round(total);
}

interface Props {
  open: boolean;
  profile?: AgentProfile | null;
  drivers: DriverConfig[];
  llmConfigs: LLMConfigItem[];
  availableSkills: SkillInfo[];
  onClose: () => void;
  onSubmit: (payload: AgentProfile) => Promise<void>;
}

function splitCSV(value: string): string[] {
  const seen = new Set<string>();
  return value
    .split(",")
    .map((item) => item.trim())
    .filter((item) => {
      if (!item || seen.has(item)) {
        return false;
      }
      seen.add(item);
      return true;
    });
}

function detectDriverKind(driver?: DriverConfig | null): "codex-acp" | "claude-acp" | "agentsdk-go" | "unknown" {
  if (!driver) {
    return "unknown";
  }
  const haystack = [
    driver.id,
    driver.launch_command,
    ...(driver.launch_args ?? []),
  ]
    .join(" ")
    .toLowerCase()
    .trim();

  if (haystack.includes("@zed-industries/codex-acp") || haystack.includes("codex-acp")) {
    return "codex-acp";
  }
  if (
    haystack.includes("@zed-industries/claude-agent-acp")
    || haystack.includes("claude-agent-acp")
    || haystack.includes("claude-acp")
  ) {
    return "claude-acp";
  }
  if (haystack.includes("agentsdk-go") || haystack.includes("agentsdk")) {
    return "agentsdk-go";
  }
  return "unknown";
}

function supportedLLMModes(driver?: DriverConfig | null): ProfileLLMMode[] {
  switch (detectDriverKind(driver)) {
    case "codex-acp":
      return ["system", "openai_response"];
    case "claude-acp":
      return ["system", "anthropic"];
    case "agentsdk-go":
      return ["system", "openai_chat_completion", "openai_response", "anthropic"];
    default:
      return ["system", "openai_chat_completion", "openai_response", "anthropic"];
  }
}

function isSystemLLMConfig(id: string | null | undefined): boolean {
  const trimmed = id?.trim();
  return !trimmed || trimmed.toLowerCase() === "system";
}

function inferLLMMode(profile: AgentProfile | null | undefined, llmConfigs: LLMConfigItem[]): ProfileLLMMode {
  if (isSystemLLMConfig(profile?.llm_config_id)) {
    return "system";
  }
  const matched = llmConfigs.find((item) => item.id === profile!.llm_config_id);
  if (matched?.type) {
    return matched.type;
  }
  return "system";
}

export function CreateProfileDialog({
  open,
  profile,
  drivers,
  llmConfigs,
  availableSkills,
  onClose,
  onSubmit,
}: Props) {
  const { t } = useTranslation();
  const editing = profile != null;

  const [id, setID] = useState("");
  const [name, setName] = useState("");
  const [role, setRole] = useState("worker");
  const [driverId, setDriverId] = useState("");
  const [llmMode, setLLMMode] = useState<ProfileLLMMode>("system");
  const [llmConfigID, setLLMConfigID] = useState("");
  const [caps, setCaps] = useState(DEFAULT_CAPS);
  const [actions, setActions] = useState(DEFAULT_ACTIONS);
  const [promptTemplate, setPromptTemplate] = useState("");
  const [selectedSkills, setSelectedSkills] = useState<string[]>([]);
  const [sessionReuse, setSessionReuse] = useState(true);
  const [maxTurns, setMaxTurns] = useState(DEFAULT_MAX_TURNS);
  const [idleTTL, setIdleTTL] = useState("");
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (!open) {
      return;
    }

    const initialDriverId = profile?.driver_id ?? drivers[0]?.id ?? "";
    setID(profile?.id ?? "");
    setName(profile?.name ?? profile?.id ?? "");
    setRole(profile?.role ?? "worker");
    setDriverId(initialDriverId);
    setLLMMode(inferLLMMode(profile, llmConfigs));
    setLLMConfigID(isSystemLLMConfig(profile?.llm_config_id) ? "" : (profile?.llm_config_id ?? ""));
    setCaps((profile?.capabilities ?? []).join(",") || DEFAULT_CAPS);
    setActions((profile?.actions_allowed ?? []).join(",") || DEFAULT_ACTIONS);
    setPromptTemplate(normalizeTextValue(profile?.prompt_template));
    setSelectedSkills(profile?.skills ?? []);
    setSessionReuse(profile?.session?.reuse ?? true);
    setMaxTurns(String(profile?.session?.max_turns ?? DEFAULT_MAX_TURNS));
    setIdleTTL(normalizeTextValue(profile?.session?.idle_ttl));
    setSubmitError(null);
  }, [open, profile, drivers, llmConfigs]);

  const selectedDriver = useMemo(
    () => drivers.find((driver) => driver.id === driverId) ?? null,
    [drivers, driverId],
  );

  const allowedModes = useMemo(
    () => supportedLLMModes(selectedDriver),
    [selectedDriver],
  );

  const modeOptions = useMemo(
    () => PROFILE_LLM_MODE_OPTIONS.filter((option) => allowedModes.includes(option.value)),
    [allowedModes],
  );

  const compatibleConfigs = useMemo(
    () => llmConfigs.filter((item) => item.type === llmMode),
    [llmConfigs, llmMode],
  );

  useEffect(() => {
    if (!allowedModes.includes(llmMode)) {
      setLLMMode(allowedModes[0] ?? "system");
    }
  }, [allowedModes, llmMode]);

  useEffect(() => {
    if (llmMode === "system") {
      if (llmConfigID !== "") {
        setLLMConfigID("");
      }
      return;
    }
    if (!compatibleConfigs.some((item) => item.id === llmConfigID)) {
      setLLMConfigID(compatibleConfigs[0]?.id ?? "");
    }
  }, [compatibleConfigs, llmConfigID, llmMode]);

  const handleClose = () => {
    if (submitting) {
      return;
    }
    onClose();
  };

  const toggleSkill = (skillName: string) => {
    setSelectedSkills((current) => (
      current.includes(skillName)
        ? current.filter((item) => item !== skillName)
        : [...current, skillName]
    ));
  };

  const handleSubmit = async () => {
    const selectedDriverConfig = drivers.find((driver) => driver.id === driverId);
    if (!selectedDriverConfig) {
      return;
    }
    const normalizedIdleTTL = normalizeTextValue(idleTTL).trim();
    const parsedIdleTTL = normalizedIdleTTL ? parseDurationToNanoseconds(normalizedIdleTTL) : null;
    if (normalizedIdleTTL && (parsedIdleTTL == null || Number.isNaN(parsedIdleTTL))) {
      setSubmitError("idle TTL 格式无效，请填写数字纳秒值，或类似 30m / 2h / 90s。");
      return;
    }
    setSubmitError(null);
    setSubmitting(true);
    try {
      await onSubmit({
        id: id.trim(),
        name: name.trim(),
        driver_id: driverId,
        llm_config_id: llmMode === "system" ? "system" : llmConfigID || undefined,
        driver: selectedDriverConfig,
        role,
        capabilities: splitCSV(caps),
        actions_allowed: splitCSV(actions),
        prompt_template: promptTemplate.trim() || undefined,
        skills: selectedSkills,
        session: {
          reuse: sessionReuse,
          max_turns: Number.parseInt(maxTurns, 10) || 0,
          idle_ttl: normalizedIdleTTL || undefined,
        },
      });
      onClose();
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onClose={handleClose} className="flex max-h-[92vh] w-[min(1320px,96vw)] max-w-[1320px] flex-col">
      <DialogHeader>
        <DialogTitle>{editing ? t("agents.editProfileTitle") : t("agents.createProfileTitle")}</DialogTitle>
        <DialogDescription>
          {editing ? t("agents.editProfileDesc") : t("agents.createProfileDesc")}
        </DialogDescription>
      </DialogHeader>
      <DialogBody className="min-h-0 flex-1 overflow-y-auto px-7 pb-6">
        <div className="grid gap-4 lg:grid-cols-2">
          <div className="space-y-1.5">
            <label className="text-sm font-medium">{t("agents.profileId")}</label>
            <Input value={id} onChange={(event) => setID(event.target.value)} disabled={editing} />
          </div>
          <div className="space-y-1.5">
            <label className="text-sm font-medium">{t("agents.profileDisplayName")}</label>
            <Input value={name} onChange={(event) => setName(event.target.value)} />
          </div>
          <div className="space-y-1.5">
            <label className="text-sm font-medium">{t("agents.role")}</label>
            <Select value={role} onValueChange={setRole}>
              <SelectItem value="lead">lead</SelectItem>
              <SelectItem value="worker">worker</SelectItem>
              <SelectItem value="gate">gate</SelectItem>
              <SelectItem value="support">support</SelectItem>
            </Select>
          </div>
          <div className="space-y-1.5">
            <label className="text-sm font-medium">{t("agents.bindDriver")}</label>
            <Select value={driverId} onValueChange={setDriverId}>
              {drivers.map((driver) => (
                <SelectItem key={driver.id} value={driver.id}>{driver.id}</SelectItem>
              ))}
            </Select>
          </div>
        </div>

        <div className="grid gap-4 lg:grid-cols-[220px_minmax(0,1fr)]">
          <div className="space-y-1.5">
            <label className="text-sm font-medium">{t("agents.profileType")}</label>
            <Select value={llmMode} onValueChange={(value) => setLLMMode(value as ProfileLLMMode)}>
              {modeOptions.map((option) => (
                <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
              ))}
            </Select>
            <p className="text-xs text-slate-500">{t("agents.profileTypeHint")}</p>
          </div>
          <div className="space-y-1.5">
            <label className="text-sm font-medium">
              {t("agents.bindLLMConfig")}
              <span className="ml-1 text-xs font-normal text-muted-foreground">({t("agents.optionalField")})</span>
            </label>
            <Select
              value={llmConfigID}
              onValueChange={setLLMConfigID}
              disabled={llmMode === "system" || compatibleConfigs.length === 0}
              placeholder={llmMode === "system" ? t("agents.systemEnvMode") : t("llmConfig.noConfigOption")}
            >
              {compatibleConfigs.map((item) => (
                <SelectItem key={item.id} value={item.id}>
                  {item.id} · {item.model || item.type}
                </SelectItem>
              ))}
            </Select>
            <div className="flex flex-wrap gap-2">
              {modeOptions.map((option) => (
                <Badge key={option.value} variant={option.value === llmMode ? "secondary" : "outline"}>
                  {option.label}
                </Badge>
              ))}
            </div>
          </div>
        </div>

        <div className="space-y-1.5">
          <label className="text-sm font-medium">{t("agents.promptTemplate")}</label>
          <Input
            value={promptTemplate}
            onChange={(event) => setPromptTemplate(event.target.value)}
            placeholder={t("agents.promptTemplatePlaceholder")}
          />
        </div>

        <div className="grid gap-4 lg:grid-cols-2">
          <div className="space-y-1.5">
            <label className="text-sm font-medium">{t("agents.capabilityTagsComma")}</label>
            <Input value={caps} onChange={(event) => setCaps(event.target.value)} />
          </div>
          <div className="space-y-1.5">
            <label className="text-sm font-medium">{t("agents.allowedActionsComma")}</label>
            <Input value={actions} onChange={(event) => setActions(event.target.value)} />
          </div>
        </div>

        <div className="rounded-2xl border border-slate-200 bg-slate-50/60 p-4">
          <div className="mb-3 flex items-start justify-between gap-3">
            <div>
              <h3 className="text-sm font-semibold text-slate-900">{t("agents.skills")}</h3>
              <p className="text-xs text-slate-500">{t("agents.skillPickerDesc")}</p>
            </div>
            <Badge variant="secondary">{selectedSkills.length}</Badge>
          </div>
          {availableSkills.length === 0 ? (
            <p className="text-sm text-slate-500">{t("agents.noSkillsAvailable")}</p>
          ) : (
            <div className="grid gap-2 md:grid-cols-2 xl:grid-cols-3">
              {availableSkills.map((skill) => {
                const checked = selectedSkills.includes(skill.name);
                return (
                  <label
                    key={skill.name}
                    className={`flex cursor-pointer gap-3 rounded-xl border px-3 py-2.5 transition ${
                      checked ? "border-slate-900 bg-white shadow-sm" : "border-slate-200 bg-white/80 hover:border-slate-300"
                    }`}
                  >
                    <input
                      type="checkbox"
                      className="mt-0.5 h-4 w-4 rounded border-slate-300"
                      checked={checked}
                      onChange={() => toggleSkill(skill.name)}
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block text-sm font-medium text-slate-900">{skill.name}</span>
                      <span className="block text-xs text-slate-500">
                        {skill.metadata?.description || t("skills.missingMeta")}
                      </span>
                    </span>
                  </label>
                );
              })}
            </div>
          )}
        </div>

        <div className="grid gap-4 lg:grid-cols-[160px_1fr_1fr]">
          <label className="flex items-center gap-2 rounded-xl border border-slate-200 bg-slate-50/60 px-3 py-2.5 text-sm font-medium text-slate-700">
            <input
              type="checkbox"
              className="h-4 w-4 rounded border-slate-300"
              checked={sessionReuse}
              onChange={(event) => setSessionReuse(event.target.checked)}
            />
            {t("agents.sessionReuse")}
          </label>
          <div className="space-y-1.5">
            <label className="text-sm font-medium">{t("agents.maxTurns")}</label>
            <Input value={maxTurns} onChange={(event) => setMaxTurns(event.target.value)} />
          </div>
          <div className="space-y-1.5">
            <label className="text-sm font-medium">{t("agents.idleTTL")}</label>
            <Input
              value={idleTTL}
              onChange={(event) => setIdleTTL(event.target.value)}
              placeholder={t("agents.idleTTLPlaceholder")}
            />
          </div>
        </div>
        {submitError ? (
          <div className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">
            {submitError}
          </div>
        ) : null}
      </DialogBody>
      <DialogFooter className="shrink-0 border-t border-slate-200 bg-white/96 backdrop-blur">
        <Button variant="outline" onClick={handleClose}>{t("common.cancel")}</Button>
        <Button
          onClick={() => void handleSubmit()}
          disabled={!id.trim() || !driverId || submitting || (llmMode !== "system" && !llmConfigID)}
        >
          {editing ? t("agents.saveProfile") : t("agents.createProfile")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
}
