import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Cpu, KeyRound, Loader2, Plus, RefreshCw, Save, Trash2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { getErrorMessage } from "@/lib/v2Workbench";
import type { LLMConfigItem, LLMConfigResponse } from "@/types/system";

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

export function LLMConfigPage() {
  const { t } = useTranslation();
  const { apiClient } = useWorkbench();
  const [data, setData] = useState<LLMConfigResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [defaultConfigID, setDefaultConfigID] = useState("");
  const [configs, setConfigs] = useState<LLMConfigItem[]>([]);

  const hydrateForm = (next: LLMConfigResponse) => {
    setData(next);
    setDefaultConfigID(next.default_config_id ?? "");
    setConfigs(next.configs ?? []);
  };

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const next = await apiClient.getLLMConfig();
      hydrateForm(next);
    } catch (loadError) {
      setError(getErrorMessage(loadError));
    } finally {
      setLoading(false);
    }
  }, [apiClient]);

  useEffect(() => {
    void load();
  }, [load]);

  const payload = useMemo<LLMConfigResponse>(() => ({
    default_config_id: defaultConfigID,
    configs,
  }), [defaultConfigID, configs]);

  const changed = useMemo(() => {
    if (data == null) return false;
    return serializeConfig(payload) !== serializeConfig(data);
  }, [data, payload]);

  const save = async () => {
    setSaving(true);
    setError(null);
    try {
      const next = await apiClient.updateLLMConfig(payload);
      hydrateForm(next);
    } catch (saveError) {
      setError(getErrorMessage(saveError));
    } finally {
      setSaving(false);
    }
  };

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

  return (
    <div className="flex-1 space-y-6 p-8">
      <div className="flex items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-2">
            <Cpu className="h-6 w-6 text-primary" />
            <h1 className="text-2xl font-bold tracking-tight">{t("llmConfig.title")}</h1>
          </div>
          <p className="mt-2 text-sm text-muted-foreground">{t("llmConfig.subtitle")}</p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" onClick={() => void load()} disabled={loading || saving}>
            <RefreshCw className={`mr-2 h-4 w-4 ${loading ? "animate-spin" : ""}`} />
            {t("common.refresh")}
          </Button>
          <Button variant="outline" onClick={appendConfig} disabled={loading || saving}>
            <Plus className="mr-2 h-4 w-4" />
            {t("llmConfig.addConfig")}
          </Button>
          <Button onClick={() => void save()} disabled={loading || saving || !changed}>
            {saving ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Save className="mr-2 h-4 w-4" />}
            {t("llmConfig.saveConfig")}
          </Button>
        </div>
      </div>

      {error ? <p className="rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p> : null}

      <div className="grid gap-4 lg:grid-cols-3">
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-base">{t("llmConfig.currentProvider")}</CardTitle>
            <CardDescription>{t("llmConfig.currentProviderDesc")}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <Select value={defaultConfigID} onValueChange={(v) => setDefaultConfigID(v)}>
              {configs.length === 0 ? <option value="">{t("llmConfig.noConfigOption")}</option> : null}
              {configs.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.id} · {PROVIDER_OPTIONS.find((option) => option.value === item.type)?.label ?? item.type}
                </option>
              ))}
            </Select>
            <div className="flex items-center justify-between rounded-xl border border-slate-200 bg-slate-50 px-4 py-3">
              <span className="text-sm text-slate-500">{t("llmConfig.activeConfig")}</span>
              <Badge variant="secondary">{defaultConfigID || "-"}</Badge>
            </div>
          </CardContent>
        </Card>

        <Card className="lg:col-span-2">
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2 text-base">
              <KeyRound className="h-4 w-4" />
              {t("llmConfig.managementGuide")}
            </CardTitle>
          </CardHeader>
          <CardContent className="grid gap-3 md:grid-cols-3">
            <div className="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3">
              <div className="text-sm font-medium">{t("llmConfig.fieldConfigId")}</div>
              <p className="mt-1 text-sm text-muted-foreground">{t("llmConfig.configIdPlaceholder")}</p>
            </div>
            <div className="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3">
              <div className="text-sm font-medium">{t("llmConfig.fieldType")}</div>
              <p className="mt-1 text-sm text-muted-foreground">{t("llmConfig.providerCardDesc")}</p>
            </div>
            <div className="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3">
              <div className="text-sm font-medium">{t("llmConfig.fieldModel")}</div>
              <p className="mt-1 text-sm text-muted-foreground">{t("llmConfig.modelHint")}</p>
            </div>
            <div className="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3">
              <div className="text-sm font-medium">{t("llmConfig.fieldTemperature")}</div>
              <p className="mt-1 text-sm text-muted-foreground">{t("llmConfig.temperatureHint")}</p>
            </div>
            <div className="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3">
              <div className="text-sm font-medium">{t("llmConfig.fieldMaxOutputTokens")}</div>
              <p className="mt-1 text-sm text-muted-foreground">{t("llmConfig.maxOutputTokensHint")}</p>
            </div>
            <div className="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3">
              <div className="text-sm font-medium">{t("llmConfig.fieldReasoningEffort")}</div>
              <p className="mt-1 text-sm text-muted-foreground">{t("llmConfig.reasoningEffortHint")}</p>
            </div>
            <div className="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3">
              <div className="text-sm font-medium">{t("llmConfig.fieldThinkingBudgetTokens")}</div>
              <p className="mt-1 text-sm text-muted-foreground">{t("llmConfig.thinkingBudgetHint")}</p>
            </div>
          </CardContent>
        </Card>
      </div>

      <div className="space-y-4">
        {configs.length === 0 ? (
          <Card>
            <CardContent className="py-10 text-center text-sm text-muted-foreground">
              {t("llmConfig.emptyState")}
            </CardContent>
          </Card>
        ) : configs.map((item, index) => {
          const isActive = item.id === defaultConfigID;
          return (
            <Card key={index} className={isActive ? "border-primary/40 shadow-sm" : undefined}>
              <CardHeader>
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <CardTitle className="text-base">{item.id || t("llmConfig.unnamedConfig")}</CardTitle>
                    <CardDescription>{t("llmConfig.providerCardDesc")}</CardDescription>
                  </div>
                  <div className="flex items-center gap-2">
                    {isActive ? <Badge variant="default">{t("llmConfig.inUse")}</Badge> : null}
                    <Button variant="outline" size="sm" onClick={() => removeConfig(index)} disabled={saving || loading}>
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              </CardHeader>
              <CardContent className="grid gap-4 lg:grid-cols-2 xl:grid-cols-4">
                <label className="space-y-2 xl:col-span-1">
                  <span className="text-sm font-medium">{t("llmConfig.fieldConfigId")}</span>
                  <Input
                    value={item.id}
                    onChange={(event) => updateConfig(index, { id: event.target.value })}
                    placeholder={t("llmConfig.configIdPlaceholder")}
                  />
                </label>
                <label className="space-y-2 xl:col-span-1">
                  <span className="text-sm font-medium">{t("llmConfig.fieldType")}</span>
                  <Select value={item.type} onValueChange={(v) => updateConfig(index, { type: v as LLMConfigItem["type"] })}>
                    {PROVIDER_OPTIONS.map((option) => (
                      <option key={option.value} value={option.value}>{option.label}</option>
                    ))}
                  </Select>
                </label>
                <label className="space-y-2 xl:col-span-1">
                  <span className="text-sm font-medium">{t("llmConfig.fieldModel")}</span>
                  <Input
                    value={item.model}
                    onChange={(event) => updateConfig(index, { model: event.target.value })}
                    placeholder={t("llmConfig.modelPlaceholder")}
                  />
                </label>
                <label className="space-y-2 xl:col-span-1">
                  <span className="text-sm font-medium">{t("llmConfig.fieldTemperature")}</span>
                  <Input
                    type="number"
                    step="0.1"
                    value={item.temperature ?? 0}
                    onChange={(event) => updateConfig(index, { temperature: Number(event.target.value) || 0 })}
                    placeholder="0"
                  />
                </label>
                <label className="space-y-2 xl:col-span-1">
                  <span className="text-sm font-medium">{t("llmConfig.fieldMaxOutputTokens")}</span>
                  <Input
                    type="number"
                    min="0"
                    step="1"
                    value={item.max_output_tokens ?? 0}
                    onChange={(event) => updateConfig(index, { max_output_tokens: Number(event.target.value) || 0 })}
                    placeholder="0"
                  />
                </label>
                <label className="space-y-2 xl:col-span-1">
                  <span className="text-sm font-medium">{t("llmConfig.fieldReasoningEffort")}</span>
                  <Select
                    value={item.reasoning_effort ?? ""}
                    onValueChange={(v) => updateConfig(index, { reasoning_effort: v as LLMConfigItem["reasoning_effort"] })}
                  >
                    {REASONING_EFFORT_OPTIONS.map((option) => (
                      <option key={option.value} value={option.value}>{option.label}</option>
                    ))}
                  </Select>
                </label>
                <label className="space-y-2 xl:col-span-1">
                  <span className="text-sm font-medium">{t("llmConfig.fieldThinkingBudgetTokens")}</span>
                  <Input
                    type="number"
                    min="0"
                    step="1"
                    value={item.thinking_budget_tokens ?? 0}
                    onChange={(event) => updateConfig(index, { thinking_budget_tokens: Number(event.target.value) || 0 })}
                    placeholder="0"
                  />
                </label>
              </CardContent>
            </Card>
          );
        })}
      </div>
    </div>
  );
}
