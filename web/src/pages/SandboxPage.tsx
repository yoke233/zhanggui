import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Shield, RefreshCw, CheckCircle2, XCircle, Cpu, Loader2, Save } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Select } from "@/components/ui/select";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { getErrorMessage } from "@/lib/v2Workbench";
import type { SandboxSupportResponse } from "@/types/system";

const PROVIDER_LABELS: Record<string, string> = {
  home_dir: "home_dir",
  litebox: "litebox",
  docker: "docker",
  bwrap: "bwrap",
};

export function SandboxPage() {
  const { t } = useTranslation();
  const { apiClient } = useWorkbench();
  const [data, setData] = useState<SandboxSupportResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [enabled, setEnabled] = useState(false);
  const [provider, setProvider] = useState("home_dir");

  const providers = useMemo(
    () => Object.entries(data?.providers ?? {}).sort(([left], [right]) => left.localeCompare(right)),
    [data],
  );

  const hydrateForm = (next: SandboxSupportResponse) => {
    setData(next);
    setEnabled(next.enabled);
    setProvider(next.configured_provider || "home_dir");
  };

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const next = await apiClient.getSandboxSupport();
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

  const save = async () => {
    setSaving(true);
    setError(null);
    try {
      const next = await apiClient.updateSandboxSupport({
        enabled,
        provider,
      });
      hydrateForm(next);
    } catch (saveError) {
      setError(getErrorMessage(saveError));
    } finally {
      setSaving(false);
    }
  };

  const changed = data != null && (enabled !== data.enabled || provider !== data.configured_provider);
  const selectedSupport = data?.providers?.[provider];

  const getProviderLabel = (key: string): string => {
    if (key === "noop") return t("sandbox.providerNoop");
    return PROVIDER_LABELS[key] ?? key;
  };

  return (
    <div className="flex-1 space-y-6 p-8">
      <div className="flex items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-2">
            <Shield className="h-6 w-6 text-primary" />
            <h1 className="text-2xl font-bold tracking-tight">{t("sandbox.title")}</h1>
          </div>
          <p className="mt-2 text-sm text-muted-foreground">{t("sandbox.subtitle")}</p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" onClick={() => void load()} disabled={loading || saving}>
            <RefreshCw className={`mr-2 h-4 w-4 ${loading ? "animate-spin" : ""}`} />
            {t("common.refresh")}
          </Button>
          <Button onClick={() => void save()} disabled={loading || saving || !changed}>
            {saving ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Save className="mr-2 h-4 w-4" />}
            {t("sandbox.saveConfig")}
          </Button>
        </div>
      </div>

      {error ? <p className="rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p> : null}

      <div className="grid gap-4 md:grid-cols-3">
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-base">{t("sandbox.currentStatus")}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <div className="flex items-center justify-between gap-3">
              <span className="text-sm text-muted-foreground">{t("sandbox.sandboxSwitch")}</span>
              <Badge variant={data?.enabled ? "default" : "secondary"}>
                {data?.enabled ? t("sandbox.sandboxOn") : t("sandbox.sandboxOff")}
              </Badge>
            </div>
            <div className="flex items-center justify-between gap-3">
              <span className="text-sm text-muted-foreground">{t("sandbox.configuredProvider")}</span>
              <Badge variant="outline">{data?.configured_provider ?? "-"}</Badge>
            </div>
            <div className="flex items-center justify-between gap-3">
              <span className="text-sm text-muted-foreground">{t("sandbox.currentProvider")}</span>
              <Badge variant="outline">{getProviderLabel(data?.current_provider ?? "") || data?.current_provider || "-"}</Badge>
            </div>
            <div className="flex items-center justify-between gap-3">
              <span className="text-sm text-muted-foreground">{t("sandbox.currentProviderAvailable")}</span>
              <Badge variant={data?.current_supported ? "success" : "destructive"}>
                {data?.current_supported ? t("common.supported") : t("common.notSupported")}
              </Badge>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2 text-base">
              <Cpu className="h-4 w-4" />
              {t("sandbox.runtimePlatform")}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <div className="flex items-center justify-between gap-3">
              <span className="text-sm text-muted-foreground">OS</span>
              <span className="font-mono text-sm">{data?.os ?? "-"}</span>
            </div>
            <div className="flex items-center justify-between gap-3">
              <span className="text-sm text-muted-foreground">Arch</span>
              <span className="font-mono text-sm">{data?.arch ?? "-"}</span>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-base">{t("sandbox.overview")}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <div className="flex items-center justify-between gap-3">
              <span className="text-sm text-muted-foreground">{t("sandbox.identifiedProviders")}</span>
              <Badge variant="secondary">{providers.length}</Badge>
            </div>
            <div className="flex items-center justify-between gap-3">
              <span className="text-sm text-muted-foreground">{t("sandbox.availableProviders")}</span>
              <Badge variant="outline">
                {providers.filter(([, support]) => support.supported).length}
              </Badge>
            </div>
            <div className="flex items-center justify-between gap-3">
              <span className="text-sm text-muted-foreground">{t("sandbox.connectedProviders")}</span>
              <Badge variant="outline">
                {providers.filter(([, support]) => support.implemented).length}
              </Badge>
            </div>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t("sandbox.frontendSwitch")}</CardTitle>
          <CardDescription>{t("sandbox.frontendSwitchDesc")}</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4 md:grid-cols-2">
          <label className="space-y-2">
            <span className="text-sm font-medium">{t("sandbox.enableSandbox")}</span>
            <button
              type="button"
              onClick={() => setEnabled((current) => !current)}
              className={[
                "flex h-11 w-full items-center justify-between rounded-[10px] border px-4 text-sm font-medium transition-colors",
                enabled
                  ? "border-emerald-200 bg-emerald-50 text-emerald-700"
                  : "border-slate-200 bg-white text-slate-600",
              ].join(" ")}
            >
              <span>{enabled ? t("common.on") : t("common.off")}</span>
              <span>{enabled ? t("common.ON") : t("common.OFF")}</span>
            </button>
          </label>

          <label className="space-y-2">
            <span className="text-sm font-medium">{t("sandbox.configProvider")}</span>
            <Select value={provider} onChange={(event) => setProvider(event.target.value)}>
              {providers.map(([name, support]) => (
                <option key={name} value={name}>
                  {name}
                  {support.supported ? "" : ` ${t("sandbox.platformNotSupported")}`}
                </option>
              ))}
            </Select>
          </label>

          <div className="rounded-xl border border-slate-200 bg-slate-50 p-4 md:col-span-2">
            <div className="flex items-center justify-between gap-3">
              <span className="text-sm text-slate-500">{t("sandbox.selectedProviderSupport")}</span>
              <Badge variant={selectedSupport?.supported ? "success" : "warning"}>
                {selectedSupport?.supported ? t("common.available") : t("common.unavailable")}
              </Badge>
            </div>
            <div className="mt-3 flex items-center justify-between gap-3">
              <span className="text-sm text-slate-500">{t("sandbox.selectedProviderConnected")}</span>
              <Badge variant={selectedSupport?.implemented ? "success" : "outline"}>
                {selectedSupport?.implemented ? t("common.connected") : t("common.notConnected")}
              </Badge>
            </div>
            <p className="mt-2 text-sm text-slate-600">{selectedSupport?.reason || t("sandbox.noAdditionalInfo")}</p>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t("sandbox.providerList")}</CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Provider</TableHead>
                <TableHead>{t("sandbox.hostSupport")}</TableHead>
                <TableHead>{t("sandbox.projectConnected")}</TableHead>
                <TableHead>{t("sandbox.explanation")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {providers.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={4} className="text-center text-muted-foreground">
                    {t("sandbox.noProviderInfo")}
                  </TableCell>
                </TableRow>
              ) : providers.map(([name, support]) => (
                <TableRow key={name}>
                  <TableCell className="font-medium">
                    <div className="flex items-center gap-2">
                      <code className="rounded bg-muted px-1.5 py-0.5 text-xs font-mono">{name}</code>
                      {name === data?.current_provider ? <Badge variant="secondary">{t("sandbox.current")}</Badge> : null}
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-2">
                      {support.supported ? (
                        <CheckCircle2 className="h-4 w-4 text-emerald-600" />
                      ) : (
                        <XCircle className="h-4 w-4 text-rose-600" />
                      )}
                      <span>{support.supported ? t("common.supported") : t("common.notSupported")}</span>
                    </div>
                  </TableCell>
                  <TableCell>
                    <Badge variant={support.implemented ? "success" : "outline"}>
                      {support.implemented ? t("common.connected") : t("common.notConnected")}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {support.reason || t("sandbox.noAdditionalInfo")}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
