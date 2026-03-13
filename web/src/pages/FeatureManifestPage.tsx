import { useEffect, useState, useCallback } from "react";
import { useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { Plus, Loader2, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Select } from "@/components/ui/select";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { getErrorMessage } from "@/lib/v2Workbench";
import type { FeatureEntry, FeatureManifest, FeatureManifestSummary, FeatureStatus } from "@/types/apiV2";

const STATUS_OPTIONS: { value: FeatureStatus | "all"; labelKey: string }[] = [
  { value: "all", labelKey: "manifest.statusAll" },
  { value: "pending", labelKey: "manifest.statusPending" },
  { value: "pass", labelKey: "manifest.statusPass" },
  { value: "fail", labelKey: "manifest.statusFail" },
  { value: "skipped", labelKey: "manifest.statusSkipped" },
];

function SummaryBar({ summary }: { summary: FeatureManifestSummary }) {
  const { t } = useTranslation();
  const { pass, fail, pending, skipped, total } = summary;
  if (total === 0) return null;
  const pct = (n: number) => Math.round((n / total) * 100);
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-4 text-sm">
        <span className="text-green-600 font-medium">{t("manifest.statusPass")} {pass}</span>
        <span className="text-red-500 font-medium">{t("manifest.statusFail")} {fail}</span>
        <span className="text-muted-foreground">{t("manifest.statusPending")} {pending}</span>
        <span className="text-muted-foreground">{t("manifest.statusSkipped")} {skipped}</span>
        <span className="ml-auto text-muted-foreground">{t("manifest.total")} {total}</span>
      </div>
      <div className="flex h-2 w-full overflow-hidden rounded-full bg-muted">
        {pass > 0 && <div className="bg-green-500" style={{ width: `${pct(pass)}%` }} />}
        {fail > 0 && <div className="bg-red-500" style={{ width: `${pct(fail)}%` }} />}
        {skipped > 0 && <div className="bg-gray-300" style={{ width: `${pct(skipped)}%` }} />}
        {pending > 0 && <div className="bg-amber-300" style={{ width: `${pct(pending)}%` }} />}
      </div>
    </div>
  );
}

export function FeatureManifestPage() {
  const { t } = useTranslation();
  const { projectId } = useParams<{ projectId: string }>();
  const { apiClient } = useWorkbench();
  const numProjectId = Number(projectId);

  const [manifest, setManifest] = useState<FeatureManifest | null>(null);
  const [summary, setSummary] = useState<FeatureManifestSummary | null>(null);
  const [entries, setEntries] = useState<FeatureEntry[]>([]);
  const [statusFilter, setStatusFilter] = useState<FeatureStatus | "all">("all");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [initing, setIniting] = useState(false);

  // Add form state
  const [showAdd, setShowAdd] = useState(false);
  const [newKey, setNewKey] = useState("");
  const [newDesc, setNewDesc] = useState("");
  const [newTags, setNewTags] = useState("");
  const [adding, setAdding] = useState(false);
  const [addError, setAddError] = useState<string | null>(null);

  const loadData = useCallback(async () => {
    if (!numProjectId) return;
    setLoading(true);
    setError(null);
    try {
      const [m, s, e] = await Promise.all([
        apiClient.getManifest(numProjectId).catch(() => null),
        apiClient.getManifestSummary(numProjectId).catch(() => null),
        apiClient.listManifestEntries(numProjectId).catch(() => [] as FeatureEntry[]),
      ]);
      setManifest(m);
      setSummary(s);
      setEntries(e);
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  }, [apiClient, numProjectId]);

  useEffect(() => { void loadData(); }, [loadData]);

  const handleInit = async () => {
    setIniting(true);
    try {
      await apiClient.getOrCreateManifest(numProjectId);
      await loadData();
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setIniting(false);
    }
  };

  const handleAddEntry = async () => {
    if (!newKey.trim()) return;
    setAdding(true);
    setAddError(null);
    try {
      const tags = newTags.split(",").map((s) => s.trim()).filter(Boolean);
      await apiClient.createManifestEntry(numProjectId, {
        key: newKey.trim(),
        description: newDesc.trim(),
        tags,
      });
      setNewKey("");
      setNewDesc("");
      setNewTags("");
      setShowAdd(false);
      await loadData();
    } catch (err) {
      setAddError(getErrorMessage(err));
    } finally {
      setAdding(false);
    }
  };

  const handleStatusChange = async (entry: FeatureEntry, status: FeatureStatus) => {
    try {
      await apiClient.updateManifestEntryStatus(entry.id, status);
      setEntries((prev) => prev.map((e) => (e.id === entry.id ? { ...e, status } : e)));
      // Reload summary
      const s = await apiClient.getManifestSummary(numProjectId).catch(() => null);
      setSummary(s);
    } catch (err) {
      setError(getErrorMessage(err));
    }
  };

  const handleDelete = async (entry: FeatureEntry) => {
    if (!window.confirm(t("manifest.confirmDelete"))) return;
    try {
      await apiClient.deleteManifestEntry(entry.id);
      await loadData();
    } catch (err) {
      setError(getErrorMessage(err));
    }
  };

  const filtered = statusFilter === "all" ? entries : entries.filter((e) => e.status === statusFilter);

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="flex-1 space-y-6 p-8">
      {/* Header */}
      <div className="flex items-start justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">{t("manifest.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("manifest.subtitle")}</p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={() => void loadData()}>
            {t("common.refresh")}
          </Button>
          {manifest && (
            <Button size="sm" onClick={() => setShowAdd(!showAdd)}>
              <Plus className="mr-1.5 h-4 w-4" />
              {t("manifest.addEntry")}
            </Button>
          )}
        </div>
      </div>

      {error && (
        <p className="rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p>
      )}

      {/* No manifest */}
      {!manifest && (
        <Card>
          <CardContent className="flex flex-col items-center gap-4 py-12">
            <p className="text-muted-foreground">{t("manifest.noManifest")}</p>
            <Button onClick={() => void handleInit()} disabled={initing}>
              {initing && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t("manifest.initBtn")}
            </Button>
          </CardContent>
        </Card>
      )}

      {manifest && (
        <>
          {/* Version + summary bar */}
          <Card>
            <CardContent className="space-y-3 py-4">
              <div className="flex items-center gap-2 text-xs text-muted-foreground">
                <span>{t("manifest.version")} {manifest.version}</span>
                {manifest.summary && <span>· {manifest.summary}</span>}
              </div>
              {summary && <SummaryBar summary={summary} />}
            </CardContent>
          </Card>

          {/* Add form */}
          {showAdd && (
            <Card className="border-dashed">
              <CardContent className="space-y-3 py-4">
                <div className="grid grid-cols-2 gap-3">
                  <div className="space-y-1">
                    <label className="text-xs font-medium text-muted-foreground">{t("manifest.entryKey")} *</label>
                    <Input
                      placeholder={t("manifest.entryKeyPlaceholder")}
                      value={newKey}
                      onChange={(e) => setNewKey(e.target.value)}
                    />
                  </div>
                  <div className="space-y-1">
                    <label className="text-xs font-medium text-muted-foreground">{t("manifest.entryTags")}</label>
                    <Input
                      placeholder={t("manifest.entryTagsPlaceholder")}
                      value={newTags}
                      onChange={(e) => setNewTags(e.target.value)}
                    />
                  </div>
                </div>
                <div className="space-y-1">
                  <label className="text-xs font-medium text-muted-foreground">{t("manifest.entryDescription")}</label>
                  <Textarea
                    placeholder={t("manifest.entryDescriptionPlaceholder")}
                    rows={2}
                    value={newDesc}
                    onChange={(e) => setNewDesc(e.target.value)}
                  />
                </div>
                {addError && <p className="text-xs text-rose-600">{addError}</p>}
                <div className="flex gap-2">
                  <Button size="sm" onClick={() => void handleAddEntry()} disabled={adding || !newKey.trim()}>
                    {adding && <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
                    {t("common.create")}
                  </Button>
                  <Button size="sm" variant="ghost" onClick={() => setShowAdd(false)}>
                    {t("common.cancel")}
                  </Button>
                </div>
              </CardContent>
            </Card>
          )}

          {/* Filter */}
          <div className="flex items-center gap-3">
            <Select
              value={statusFilter}
              className="w-36"
              onChange={(e) => setStatusFilter(e.target.value as FeatureStatus | "all")}
            >
              {STATUS_OPTIONS.map((opt) => (
                <option key={opt.value} value={opt.value}>{t(opt.labelKey)}</option>
              ))}
            </Select>
            <span className="text-sm text-muted-foreground">{filtered.length} {t("manifest.total").toLowerCase()}</span>
          </div>

          {/* Entries table */}
          {filtered.length === 0 ? (
            <p className="py-8 text-center text-sm text-muted-foreground">{t("manifest.noEntries")}</p>
          ) : (
            <div className="rounded-lg border">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-muted/40">
                    <th className="px-4 py-2.5 text-left font-medium text-muted-foreground w-56">{t("manifest.entryKey")}</th>
                    <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("manifest.entryDescription")}</th>
                    <th className="px-4 py-2.5 text-left font-medium text-muted-foreground w-28">{t("common.status")}</th>
                    <th className="px-4 py-2.5 text-left font-medium text-muted-foreground w-40">{t("manifest.entryTags")}</th>
                    <th className="px-4 py-2.5 w-10" />
                  </tr>
                </thead>
                <tbody>
                  {filtered.map((entry) => (
                    <tr key={entry.id} className="border-b last:border-0 hover:bg-muted/20 transition-colors">
                      <td className="px-4 py-3 font-mono text-xs text-foreground align-top break-all">{entry.key}</td>
                      <td className="px-4 py-3 text-muted-foreground align-top">{entry.description}</td>
                      <td className="px-4 py-3 align-top">
                        <Select
                          value={entry.status}
                          className="h-7 w-28 border-0 px-0 py-0 text-xs shadow-none focus-visible:ring-0"
                          onChange={(e) => void handleStatusChange(entry, e.target.value as FeatureStatus)}
                        >
                          {(["pending", "pass", "fail", "skipped"] as FeatureStatus[]).map((s) => (
                            <option key={s} value={s}>{s}</option>
                          ))}
                        </Select>
                      </td>
                      <td className="px-4 py-3 align-top">
                        <div className="flex flex-wrap gap-1">
                          {(entry.tags ?? []).map((tag) => (
                            <Badge key={tag} variant="secondary" className="text-xs">{tag}</Badge>
                          ))}
                        </div>
                      </td>
                      <td className="px-4 py-3 align-top">
                        <button
                          onClick={() => void handleDelete(entry)}
                          className="text-muted-foreground hover:text-destructive transition-colors"
                          title={t("common.delete")}
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}
    </div>
  );
}
