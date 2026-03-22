import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Bot, Cpu, Loader2, Pencil, Plus, RefreshCw, Save, Settings2, Shield, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import {
  Dialog,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { getErrorMessage } from "@/lib/v2Workbench";
import { CreateProfileDialog } from "@/components/agents/CreateProfileDialog";
import { CreateDriverDialog } from "@/components/agents/CreateDriverDialog";
import type { DriverConfig, AgentProfile, SkillInfo } from "@/types/apiV2";
import type { LLMConfigItem, LLMConfigResponse, SandboxSupportResponse } from "@/types/system";
import type { RuntimeConfigReloadedPayload } from "@/types/ws";

const roleBadgeVariant: Record<string, "info" | "warning" | "default" | "secondary"> = {
  worker: "info",
  gate: "warning",
  lead: "default",
  support: "secondary",
};

const ALL_CAPS = ["fs_read", "fs_write", "terminal"] as const;
const PROVIDER_OPTIONS = [
  { value: "openai_chat_completion", label: "OpenAI ChatCompletion" },
  { value: "openai_response", label: "OpenAI Response" },
  { value: "anthropic", label: "Anthropic" },
] as const;

const REASONING_EFFORT_OPTIONS = [
  { value: "", label: "默认" },
  { value: "low", label: "Low" },
  { value: "medium", label: "Medium" },
  { value: "high", label: "High" },
] as const;

const EMPTY_ITEM = (index: number): LLMConfigItem => ({
  id: `llm-config-${index}`,
  type: "openai_response",
  model: "",
  temperature: 0,
  max_output_tokens: 0,
  reasoning_effort: "",
  thinking_budget_tokens: 0,
});

const nextDraftConfig = (current: LLMConfigItem[]): LLMConfigItem => {
  let index = current.length + 1;
  while (current.some((item) => item.id === `llm-config-${index}`)) {
    index += 1;
  }
  return EMPTY_ITEM(index);
};

const serializeConfig = (value: LLMConfigResponse): string => JSON.stringify({
  default_config_id: value.default_config_id,
  configs: value.configs,
});

export function AgentsPage() {
  const { t } = useTranslation();
  const { apiClient, wsClient } = useWorkbench();
  const [drivers, setDrivers] = useState<DriverConfig[]>([]);
  const [profiles, setProfiles] = useState<AgentProfile[]>([]);
  const [availableSkills, setAvailableSkills] = useState<SkillInfo[]>([]);
  const [llmData, setLLMData] = useState<LLMConfigResponse | null>(null);
  const [defaultConfigID, setDefaultConfigID] = useState("");
  const [configs, setConfigs] = useState<LLMConfigItem[]>([]);
  const [sandboxSupport, setSandboxSupport] = useState<SandboxSupportResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [savingLLM, setSavingLLM] = useState(false);
  const [sandboxLoading, setSandboxLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [llmError, setLLMError] = useState<string | null>(null);
  const [sandboxError, setSandboxError] = useState<string | null>(null);
  const [profileDialogOpen, setProfileDialogOpen] = useState(false);
  const [editingProfile, setEditingProfile] = useState<AgentProfile | null>(null);
  const [driverDialogOpen, setDriverDialogOpen] = useState(false);
  const [editingDriver, setEditingDriver] = useState<DriverConfig | null>(null);
  const [pendingDeleteProfile, setPendingDeleteProfile] = useState<AgentProfile | null>(null);
  const [deletingProfileId, setDeletingProfileId] = useState<string | null>(null);
  const [pendingDeleteDriver, setPendingDeleteDriver] = useState<DriverConfig | null>(null);
  const [deletingDriverId, setDeletingDriverId] = useState<string | null>(null);

  const hydrateLLM = (next: LLMConfigResponse) => {
    setLLMData(next);
    setDefaultConfigID(next.default_config_id ?? "");
    setConfigs(next.configs ?? []);
  };

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [driverResp, profileResp, llmResp, skillResp] = await Promise.all([
        apiClient.listDrivers(),
        apiClient.listProfiles(),
        apiClient.getLLMConfig(),
        apiClient.listSkills(),
      ]);
      setDrivers(driverResp);
      setProfiles(profileResp);
      setAvailableSkills(skillResp);
      hydrateLLM(llmResp);
    } catch (e) {
      setError(getErrorMessage(e));
    } finally {
      setLoading(false);
    }
  }, [apiClient]);

  const loadSandboxSupport = useCallback(async () => {
    setSandboxLoading(true);
    setSandboxError(null);
    try {
      setSandboxSupport(await apiClient.getSandboxSupport());
    } catch (e) {
      setSandboxError(getErrorMessage(e));
    } finally {
      setSandboxLoading(false);
    }
  }, [apiClient]);

  useEffect(() => {
    void Promise.all([load(), loadSandboxSupport()]);
  }, [load, loadSandboxSupport]);

  useEffect(() => {
    const unsubscribe = wsClient.subscribe<RuntimeConfigReloadedPayload>(
      "runtime.config_reloaded",
      () => {
        void load();
        void loadSandboxSupport();
      },
    );
    return unsubscribe;
  }, [load, loadSandboxSupport, wsClient]);

  const payload = useMemo<LLMConfigResponse>(() => ({
    default_config_id: defaultConfigID,
    configs,
  }), [defaultConfigID, configs]);

  const llmChanged = useMemo(() => {
    if (llmData == null) return false;
    return serializeConfig(payload) !== serializeConfig(llmData);
  }, [llmData, payload]);

  const activeConfig = useMemo(
    () => configs.find((item) => item.id === defaultConfigID) ?? configs[0] ?? null,
    [configs, defaultConfigID],
  );
  const llmConfigMap = useMemo(
    () => new Map(configs.map((item) => [item.id, item])),
    [configs],
  );

  const sandboxState = sandboxSupport?.enabled
    ? `${t("sandbox.sandboxOn")} · ${sandboxSupport.current_provider}`
    : t("sandbox.sandboxOff");

  const updateConfig = (index: number, patch: Partial<LLMConfigItem>) => {
    setConfigs((current) => {
      const previous = current[index];
      if (patch.id != null && previous) {
        setDefaultConfigID((currentDefault) => (previous.id === currentDefault ? patch.id ?? currentDefault : currentDefault));
      }
      return current.map((item, itemIndex) => (itemIndex === index ? { ...item, ...patch } : item));
    });
  };

  const appendConfig = () => {
    setConfigs((current) => {
      const next = [...current, nextDraftConfig(current)];
      setDefaultConfigID((currentDefault) => currentDefault || next[0]?.id || "");
      return next;
    });
  };

  const removeConfig = (index: number) => {
    setConfigs((current) => {
      const removed = current[index];
      const next = current.filter((_, itemIndex) => itemIndex !== index);
      if (removed) {
        setDefaultConfigID((currentDefault) => (removed.id === currentDefault ? next[0]?.id ?? "" : currentDefault));
      }
      return next;
    });
  };

  const saveLLM = async () => {
    setSavingLLM(true);
    setLLMError(null);
    try {
      const next = await apiClient.updateLLMConfig(payload);
      hydrateLLM(next);
    } catch (e) {
      setLLMError(getErrorMessage(e));
    } finally {
      setSavingLLM(false);
    }
  };

  const handleDeleteDriver = async (driverId: string) => {
    setDeletingDriverId(driverId);
    setError(null);
    try {
      await apiClient.deleteDriver(driverId);
      await load();
    } catch (e) {
      setError(getErrorMessage(e));
    } finally {
      setDeletingDriverId((current) => (current === driverId ? null : current));
    }
  };

  const handleDeleteProfile = async (profileId: string) => {
    setDeletingProfileId(profileId);
    setError(null);
    try {
      await apiClient.deleteProfile(profileId);
      await load();
    } catch (e) {
      setError(getErrorMessage(e));
    } finally {
      setDeletingProfileId((current) => (current === profileId ? null : current));
    }
  };

  const openProfileDialog = (profile?: AgentProfile | null) => {
    setEditingProfile(profile ?? null);
    setProfileDialogOpen(true);
  };

  return (
    <div className="min-h-full flex-1 bg-[linear-gradient(180deg,#f8fafc_0%,#eef2ff_46%,#ffffff_100%)] p-6 md:p-8">
      <div className="mx-auto max-w-[1440px] space-y-5">
        <div className="flex flex-wrap items-center gap-3">
          <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-slate-950 text-white shadow-sm">
            <Bot className="h-5 w-5" />
          </div>
          <div>
            <h1 className="text-2xl font-bold tracking-tight text-slate-950">{t("agents.title")}</h1>
            <p className="text-sm text-slate-500">{t("agents.subtitle")}</p>
          </div>
          {loading ? <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /> : null}
        </div>

        <div className="grid gap-3">
          <Card className="border-slate-200/80 bg-white/90 shadow-[0_20px_60px_-42px_rgba(15,23,42,0.32)] backdrop-blur">
            <CardHeader className="pb-3">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <CardDescription>{t("agents.profiles")}</CardDescription>
                  <CardTitle className="mt-1 text-[28px] leading-none">{profiles.length}</CardTitle>
                </div>
                <Badge variant="secondary" className="bg-blue-50 text-blue-700">
                  {activeConfig?.model || t("common.notFilled")}
                </Badge>
              </div>
            </CardHeader>
          </Card>
          <Card className="border-slate-200/80 bg-white/90 shadow-[0_20px_60px_-42px_rgba(15,23,42,0.32)] backdrop-blur">
            <CardHeader className="pb-3">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <CardDescription>{t("agents.drivers")}</CardDescription>
                  <CardTitle className="mt-1 text-[28px] leading-none">{drivers.length}</CardTitle>
                </div>
                <Badge variant={sandboxSupport?.enabled ? "success" : "outline"}>
                  {sandboxState}
                </Badge>
              </div>
            </CardHeader>
          </Card>
          <Card className="border-slate-200/80 bg-white/90 shadow-[0_20px_60px_-42px_rgba(15,23,42,0.32)] backdrop-blur">
            <CardHeader className="pb-3">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <CardDescription>{t("llmConfig.currentProvider")}</CardDescription>
                  <CardTitle className="mt-1 text-[28px] leading-none">{configs.length}</CardTitle>
                </div>
                <Badge variant="secondary">{defaultConfigID || "-"}</Badge>
              </div>
            </CardHeader>
          </Card>
        </div>

        {error ? <p className="rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p> : null}
        {llmError ? <p className="rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{llmError}</p> : null}
        {sandboxError ? <p className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">{sandboxError}</p> : null}

        <div className="grid gap-5">
          <Card className="overflow-hidden border-slate-200/80 bg-white/92 shadow-[0_18px_48px_-40px_rgba(15,23,42,0.55)]">
            <CardHeader className="border-b border-slate-100 bg-slate-50/70 pb-4">
              <div className="flex items-start justify-between gap-3">
                <div className="space-y-1">
                  <CardTitle className="flex items-center gap-2 text-base">
                    {t("agents.profiles")}
                    <Badge variant="secondary" className="ml-1">{profiles.length}</Badge>
                  </CardTitle>
                  <CardDescription>{t("agents.profilesDesc")}</CardDescription>
                </div>
                <Button size="sm" onClick={() => openProfileDialog()}>
                  <Plus className="mr-1.5 h-3.5 w-3.5" />
                  {t("agents.newProfile")}
                </Button>
              </div>
            </CardHeader>
            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="h-10 px-3 text-[11px] uppercase tracking-[0.16em]">{t("agents.profileName")}</TableHead>
                    <TableHead className="h-10 px-3 text-[11px] uppercase tracking-[0.16em]">{t("agents.role")}</TableHead>
                    <TableHead className="h-10 px-3 text-[11px] uppercase tracking-[0.16em]">{t("agents.boundDriver")}</TableHead>
                    <TableHead className="h-10 px-3 text-[11px] uppercase tracking-[0.16em]">{t("agents.model")}</TableHead>
                    <TableHead className="h-10 px-3 text-[11px] uppercase tracking-[0.16em]">{t("agents.skills")}</TableHead>
                    <TableHead className="h-10 px-3 text-[11px] uppercase tracking-[0.16em]">{t("common.status")}</TableHead>
                    <TableHead className="h-10 px-3 text-right text-[11px] uppercase tracking-[0.16em]">{t("common.operations")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {profiles.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={7} className="px-3 py-10 text-center text-muted-foreground">{t("agents.noProfiles")}</TableCell>
                    </TableRow>
                  ) : (
                    profiles.map((profile) => (
                      <TableRow
                        key={profile.id}
                        className="cursor-pointer transition-colors hover:bg-slate-50/80"
                        onClick={() => openProfileDialog(profile)}
                      >
                        <TableCell className="px-3 py-3">
                          <div className="space-y-1">
                            <div className="font-medium text-slate-900">{profile.name || profile.id}</div>
                            <div className="text-xs text-slate-500">{profile.id}</div>
                          </div>
                        </TableCell>
                        <TableCell className="px-3 py-3">
                          <Badge variant={roleBadgeVariant[profile.role] ?? "secondary"}>{profile.role}</Badge>
                        </TableCell>
                        <TableCell className="px-3 py-3 text-sm text-slate-600">
                          {profile.driver_id ?? profile.driver?.launch_command ?? "-"}
                        </TableCell>
                        <TableCell className="px-3 py-3">
                          <div className="space-y-1">
                            <div className="text-sm font-medium text-slate-900">
                              {profile.llm_config_id && profile.llm_config_id.toLowerCase() !== "system"
                                ? (llmConfigMap.get(profile.llm_config_id)?.model || "-")
                                : t("agents.systemEnvMode")}
                            </div>
                            <div className="text-xs text-slate-500">
                              {profile.llm_config_id && profile.llm_config_id.toLowerCase() !== "system"
                                ? `${profile.llm_config_id} · ${llmConfigMap.get(profile.llm_config_id)?.type ?? "custom"}`
                                : t("agents.systemEnvHint")}
                            </div>
                          </div>
                        </TableCell>
                        <TableCell className="px-3 py-3">
                          <div className="flex flex-wrap gap-1">
                            {(profile.skills ?? []).length > 0 ? (
                              (profile.skills ?? []).slice(0, 3).map((skill) => (
                                <Badge key={skill} variant="outline" className="text-[10px] normal-case tracking-normal">{skill}</Badge>
                              ))
                            ) : (
                              <span className="text-sm text-slate-400">-</span>
                            )}
                            {(profile.skills?.length ?? 0) > 3 ? (
                              <Badge variant="outline" className="text-[10px] normal-case tracking-normal">
                                +{(profile.skills?.length ?? 0) - 3}
                              </Badge>
                            ) : null}
                          </div>
                        </TableCell>
                        <TableCell className="px-3 py-3">
                          <Badge variant="outline">{profile.session?.reuse === false ? t("agents.ephemeral") : t("agents.activeState")}</Badge>
                        </TableCell>
                        <TableCell className="px-3 py-3 text-right">
                          <div className="flex justify-end gap-2">
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={(event) => {
                                event.stopPropagation();
                                openProfileDialog(profile);
                              }}
                            >
                              <Pencil className="h-3.5 w-3.5" />
                            </Button>
                            <Button
                              variant="outline"
                              size="sm"
                              disabled={deletingProfileId === profile.id}
                              onClick={(event) => {
                                event.stopPropagation();
                                setPendingDeleteProfile(profile);
                              }}
                            >
                              {deletingProfileId === profile.id ? (
                                <Loader2 className="h-3.5 w-3.5 animate-spin" />
                              ) : (
                                <Trash2 className="h-3.5 w-3.5" />
                              )}
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </CardContent>
          </Card>

          <Card className="overflow-hidden border-slate-200/80 bg-white/92 shadow-[0_18px_48px_-40px_rgba(15,23,42,0.55)]">
            <CardHeader className="border-b border-slate-100 bg-slate-50/70 pb-4">
              <div className="flex items-start justify-between gap-3">
                <div className="space-y-1">
                  <CardTitle className="flex items-center gap-2 text-base">
                    <Settings2 className="h-4 w-4" />
                    {t("agents.drivers")}
                    <Badge variant="secondary" className="ml-1">{drivers.length}</Badge>
                  </CardTitle>
                  <CardDescription>{t("agents.driversDesc")}</CardDescription>
                </div>
                <div className="flex gap-2">
                  <Button size="sm" onClick={() => setDriverDialogOpen(true)}>
                    <Plus className="mr-1.5 h-3.5 w-3.5" />
                    {t("agents.newDriver")}
                  </Button>
                  <Button variant="outline" size="sm" onClick={() => void loadSandboxSupport()} disabled={sandboxLoading}>
                    <RefreshCw className={`mr-1.5 h-3.5 w-3.5 ${sandboxLoading ? "animate-spin" : ""}`} />
                    {t("common.refresh")}
                  </Button>
                </div>
              </div>
              <div className="grid gap-2 pt-1 sm:grid-cols-3">
                <div className="rounded-xl border border-slate-200 bg-white px-3 py-2.5">
                  <div className="flex items-center gap-2 text-xs font-medium uppercase tracking-[0.14em] text-slate-500">
                    <Shield className="h-3.5 w-3.5" />
                    {t("sandbox.sandboxSwitch")}
                  </div>
                  <div className="mt-1 text-sm font-semibold text-slate-900">{sandboxState}</div>
                </div>
                <div className="rounded-xl border border-slate-200 bg-white px-3 py-2.5">
                  <div className="text-xs font-medium uppercase tracking-[0.14em] text-slate-500">{t("sandbox.configuredProvider")}</div>
                  <div className="mt-1 text-sm font-semibold text-slate-900">{sandboxSupport?.configured_provider || "-"}</div>
                </div>
                <div className="rounded-xl border border-slate-200 bg-white px-3 py-2.5">
                  <div className="text-xs font-medium uppercase tracking-[0.14em] text-slate-500">{t("sandbox.runtimeEnv")}</div>
                  <div className="mt-1 text-sm font-semibold text-slate-900">
                    {sandboxSupport ? `${sandboxSupport.os} / ${sandboxSupport.arch}` : "-"}
                  </div>
                </div>
              </div>
            </CardHeader>
            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="h-10 px-3 text-[11px] uppercase tracking-[0.16em]">{t("agents.driverName")}</TableHead>
                    <TableHead className="h-10 px-3 text-[11px] uppercase tracking-[0.16em]">{t("agents.launchCommand")}</TableHead>
                    <TableHead className="h-10 px-3 text-[11px] uppercase tracking-[0.16em]">{t("agents.maxCapabilities")}</TableHead>
                    <TableHead className="h-10 px-3 text-right text-[11px] uppercase tracking-[0.16em]">{t("common.operations")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {drivers.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={4} className="px-3 py-10 text-center text-muted-foreground">{t("agents.noDrivers")}</TableCell>
                    </TableRow>
                  ) : (
                    drivers.map((driver) => (
                      <TableRow key={driver.id}>
                        <TableCell className="px-3 py-3">
                          <div className="space-y-1">
                            <div className="font-medium text-slate-900">{driver.id}</div>
                            <div className="text-xs text-slate-500">{(driver.launch_args ?? []).length} args</div>
                          </div>
                        </TableCell>
                        <TableCell className="px-3 py-3">
                          <code className="inline-flex max-w-full rounded-lg border border-slate-200 bg-slate-50 px-2 py-1 text-[11px] font-mono text-slate-700">
                            {driver.launch_command} {(driver.launch_args ?? []).join(" ")}
                          </code>
                        </TableCell>
                        <TableCell className="px-3 py-3">
                          <div className="flex flex-wrap gap-1">
                            {ALL_CAPS.filter((cap) => driver.capabilities_max[cap]).map((cap) => (
                              <Badge key={cap} variant="outline" className="text-[10px] tracking-normal">{cap}</Badge>
                            ))}
                          </div>
                        </TableCell>
                        <TableCell className="px-3 py-3 text-right">
                          <div className="flex justify-end gap-2">
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => setEditingDriver(driver)}
                            >
                              <Pencil className="h-3.5 w-3.5" />
                            </Button>
                            <Button
                              variant="outline"
                              size="sm"
                              disabled={deletingDriverId === driver.id}
                              onClick={() => setPendingDeleteDriver(driver)}
                            >
                              {deletingDriverId === driver.id ? (
                                <Loader2 className="h-3.5 w-3.5 animate-spin" />
                              ) : (
                                <Trash2 className="h-3.5 w-3.5" />
                              )}
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </CardContent>
          </Card>

          <Card className="overflow-hidden border-slate-200/80 bg-white/92 shadow-[0_18px_48px_-40px_rgba(15,23,42,0.55)]">
            <CardHeader className="border-b border-slate-100 bg-slate-50/70 pb-4">
              <div className="flex items-start justify-between gap-3">
                <div className="space-y-1">
                  <CardTitle className="flex items-center gap-2 text-base">
                    <Cpu className="h-4 w-4" />
                    {t("agents.llmProviders")}
                    <Badge variant="secondary" className="ml-1">{configs.length}</Badge>
                  </CardTitle>
                  <CardDescription>{t("agents.llmProvidersDesc")}</CardDescription>
                </div>
                <div className="flex gap-2">
                  <Button variant="outline" size="sm" onClick={() => void load()} disabled={loading || savingLLM}>
                    <RefreshCw className={`mr-1.5 h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
                    {t("common.refresh")}
                  </Button>
                  <Button variant="outline" size="sm" onClick={appendConfig} disabled={loading || savingLLM}>
                    <Plus className="mr-1.5 h-3.5 w-3.5" />
                    {t("llmConfig.addConfig")}
                  </Button>
                  <Button size="sm" onClick={() => void saveLLM()} disabled={loading || savingLLM || !llmChanged}>
                    {savingLLM ? <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" /> : <Save className="mr-1.5 h-3.5 w-3.5" />}
                    {t("llmConfig.saveConfig")}
                  </Button>
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-3 p-3">
              <div className="grid gap-2 md:grid-cols-[1.1fr_1fr]">
                <div className="rounded-xl border border-slate-200 bg-slate-50/70 px-3 py-2.5">
                  <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-500">{t("llmConfig.currentProvider")}</div>
                  <div className="mt-1 flex items-center gap-2">
                    <Select value={defaultConfigID} onValueChange={(v) => setDefaultConfigID(v)} className="h-9">
                      {configs.length === 0 ? <option value="">{t("llmConfig.noConfigOption")}</option> : null}
                      {configs.map((item) => (
                        <option key={item.id} value={item.id}>
                          {item.id}
                        </option>
                      ))}
                    </Select>
                    <Badge variant="secondary">{activeConfig?.model || "-"}</Badge>
                  </div>
                </div>
                <div className="rounded-xl border border-slate-200 bg-slate-50/70 px-3 py-2.5">
                  <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-500">{t("llmConfig.activeConfig")}</div>
                  <div className="mt-1 text-sm font-semibold text-slate-900">{defaultConfigID || "-"}</div>
                  <div className="text-xs text-slate-500">{activeConfig?.type || t("common.notFilled")}</div>
                </div>
              </div>

              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="h-10 px-3 text-[11px] uppercase tracking-[0.16em]">{t("llmConfig.fieldConfigId")}</TableHead>
                    <TableHead className="h-10 px-3 text-[11px] uppercase tracking-[0.16em]">{t("llmConfig.fieldType")}</TableHead>
                    <TableHead className="h-10 px-3 text-[11px] uppercase tracking-[0.16em]">{t("llmConfig.fieldModel")}</TableHead>
                    <TableHead className="h-10 px-3 text-[11px] uppercase tracking-[0.16em]">{t("llmConfig.fieldTemperature")}</TableHead>
                    <TableHead className="h-10 px-3 text-[11px] uppercase tracking-[0.16em]">{t("llmConfig.fieldMaxOutputTokens")}</TableHead>
                    <TableHead className="h-10 px-3 text-[11px] uppercase tracking-[0.16em]">{t("llmConfig.fieldReasoningEffort")}</TableHead>
                    <TableHead className="h-10 px-3 text-[11px] uppercase tracking-[0.16em]">{t("llmConfig.fieldThinkingBudgetTokens")}</TableHead>
                    <TableHead className="h-10 px-3 text-right text-[11px] uppercase tracking-[0.16em]">{t("common.operations")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {configs.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={8} className="px-3 py-10 text-center text-muted-foreground">{t("llmConfig.emptyState")}</TableCell>
                    </TableRow>
                  ) : (
                    configs.map((item, index) => {
                      const isActive = item.id === defaultConfigID;
                      return (
                        <TableRow key={`${item.id}-${index}`} className={isActive ? "bg-blue-50/40" : undefined}>
                          <TableCell className="px-3 py-3">
                            <div className="space-y-1">
                              <Input
                                value={item.id}
                                onChange={(event) => updateConfig(index, { id: event.target.value })}
                                placeholder={t("llmConfig.configIdPlaceholder")}
                                className="h-9"
                              />
                              {isActive ? <Badge variant="secondary">{t("llmConfig.inUse")}</Badge> : null}
                            </div>
                          </TableCell>
                          <TableCell className="px-3 py-3">
                            <Select
                              value={item.type}
                              onValueChange={(v) => updateConfig(index, { type: v as LLMConfigItem["type"] })}
                              className="h-9"
                            >
                              {PROVIDER_OPTIONS.map((option) => (
                                <option key={option.value} value={option.value}>{option.label}</option>
                              ))}
                            </Select>
                          </TableCell>
                          <TableCell className="px-3 py-3">
                            <Input
                              value={item.model}
                              onChange={(event) => updateConfig(index, { model: event.target.value })}
                              placeholder={t("llmConfig.modelPlaceholder")}
                              className="h-9 min-w-[170px]"
                            />
                          </TableCell>
                          <TableCell className="px-3 py-3">
                            <Input
                              type="number"
                              step="0.1"
                              value={item.temperature ?? 0}
                              onChange={(event) => updateConfig(index, { temperature: Number(event.target.value) || 0 })}
                              placeholder="0"
                              className="h-9 min-w-[110px]"
                            />
                          </TableCell>
                          <TableCell className="px-3 py-3">
                            <Input
                              type="number"
                              min="0"
                              step="1"
                              value={item.max_output_tokens ?? 0}
                              onChange={(event) => updateConfig(index, { max_output_tokens: Number(event.target.value) || 0 })}
                              placeholder="0"
                              className="h-9 min-w-[120px]"
                            />
                          </TableCell>
                          <TableCell className="px-3 py-3">
                            <Select
                              value={item.reasoning_effort ?? ""}
                              onValueChange={(v) => updateConfig(index, { reasoning_effort: v as LLMConfigItem["reasoning_effort"] })}
                              className="h-9 min-w-[120px]"
                            >
                              {REASONING_EFFORT_OPTIONS.map((option) => (
                                <option key={option.value} value={option.value}>{option.label}</option>
                              ))}
                            </Select>
                          </TableCell>
                          <TableCell className="px-3 py-3">
                            <Input
                              type="number"
                              min="0"
                              step="1"
                              value={item.thinking_budget_tokens ?? 0}
                              onChange={(event) => updateConfig(index, { thinking_budget_tokens: Number(event.target.value) || 0 })}
                              placeholder="0"
                              className="h-9 min-w-[120px]"
                            />
                          </TableCell>
                          <TableCell className="px-3 py-3 text-right">
                            <Button variant="outline" size="sm" onClick={() => removeConfig(index)} disabled={savingLLM || loading}>
                              <Trash2 className="h-3.5 w-3.5" />
                            </Button>
                          </TableCell>
                        </TableRow>
                      );
                    })
                  )}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </div>
      </div>
      <CreateProfileDialog
        open={profileDialogOpen}
        profile={editingProfile}
        drivers={drivers}
        llmConfigs={configs}
        availableSkills={availableSkills}
        onClose={() => {
          setProfileDialogOpen(false);
          setEditingProfile(null);
        }}
        onSubmit={async (payload) => {
          if (editingProfile) {
            await apiClient.updateProfile(editingProfile.id, payload);
          } else {
            await apiClient.createProfile(payload);
          }
          await load();
          setProfileDialogOpen(false);
          setEditingProfile(null);
        }}
      />
      <CreateDriverDialog
        open={driverDialogOpen || editingDriver != null}
        driver={editingDriver}
        onClose={() => {
          setDriverDialogOpen(false);
          setEditingDriver(null);
        }}
        onSubmit={async (payload) => {
          if (editingDriver) {
            await apiClient.updateDriver(editingDriver.id, payload);
          } else {
            await apiClient.createDriver(payload);
          }
          await load();
          setDriverDialogOpen(false);
          setEditingDriver(null);
        }}
      />
      <Dialog
        open={pendingDeleteProfile != null}
        onClose={() => {
          if (deletingProfileId == null) {
            setPendingDeleteProfile(null);
          }
        }}
        className="max-w-md"
      >
        <DialogHeader>
          <DialogTitle>{t("common.confirm")}</DialogTitle>
          <DialogDescription>
            确定要删除 profile <strong>{pendingDeleteProfile?.id}</strong> 吗？此操作不可撤销。
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => setPendingDeleteProfile(null)}
            disabled={deletingProfileId != null}
          >
            {t("common.cancel")}
          </Button>
          <Button
            variant="destructive"
            disabled={pendingDeleteProfile == null || deletingProfileId != null}
            onClick={() => {
              if (!pendingDeleteProfile) return;
              void handleDeleteProfile(pendingDeleteProfile.id).then(() => {
                setPendingDeleteProfile(null);
              });
            }}
          >
            {deletingProfileId != null ? <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" /> : null}
            删除
          </Button>
        </DialogFooter>
      </Dialog>
      <Dialog
        open={pendingDeleteDriver != null}
        onClose={() => {
          if (deletingDriverId == null) {
            setPendingDeleteDriver(null);
          }
        }}
        className="max-w-md"
      >
        <DialogHeader>
          <DialogTitle>{t("common.confirm")}</DialogTitle>
          <DialogDescription>
            确定要删除 driver <strong>{pendingDeleteDriver?.id}</strong> 吗？此操作不可撤销。
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => setPendingDeleteDriver(null)}
            disabled={deletingDriverId != null}
          >
            {t("common.cancel")}
          </Button>
          <Button
            variant="destructive"
            disabled={pendingDeleteDriver == null || deletingDriverId != null}
            onClick={() => {
              if (!pendingDeleteDriver) return;
              void handleDeleteDriver(pendingDeleteDriver.id).then(() => {
                setPendingDeleteDriver(null);
              });
            }}
          >
            {deletingDriverId != null ? <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" /> : null}
            删除
          </Button>
        </DialogFooter>
      </Dialog>
    </div>
  );
}
