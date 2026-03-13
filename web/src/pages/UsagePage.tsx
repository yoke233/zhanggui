import { useEffect, useState, useCallback } from "react";
import { useTranslation } from "react-i18next";
import {
  Coins,
  Loader2,
  RefreshCw,
  ArrowDownUp,
  Database,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { getErrorMessage } from "@/lib/v2Workbench";
import type { UsageAnalyticsSummary } from "@/types/apiV2";

function formatTokens(n: number): string {
  if (n === 0) return "0";
  if (n < 1000) return String(n);
  if (n < 1_000_000) return `${(n / 1000).toFixed(1)}K`;
  return `${(n / 1_000_000).toFixed(2)}M`;
}

function tokenBar(input: number, output: number, total: number): React.ReactNode {
  if (total === 0) return null;
  const inputPct = (input / total) * 100;
  const outputPct = (output / total) * 100;
  return (
    <div className="flex items-center gap-2">
      <div className="h-2.5 w-28 overflow-hidden rounded-full bg-slate-100">
        <div className="flex h-full">
          <div
            className="h-full bg-blue-500"
            style={{ width: `${inputPct}%` }}
            title={`Input: ${formatTokens(input)}`}
          />
          <div
            className="h-full bg-emerald-500"
            style={{ width: `${outputPct}%` }}
            title={`Output: ${formatTokens(output)}`}
          />
        </div>
      </div>
      <span className="text-xs text-muted-foreground">{formatTokens(total)}</span>
    </div>
  );
}

export function UsagePage() {
  const { t } = useTranslation();
  const { apiClient, selectedProjectId } = useWorkbench();
  const [data, setData] = useState<UsageAnalyticsSummary | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [rangeDays, setRangeDays] = useState(7);
  const [autoRefresh, setAutoRefresh] = useState(true);

  const TIME_RANGES = [
    { label: "24h", value: 1 },
    { label: "7d", value: 7 },
    { label: "30d", value: 30 },
    { label: t("common.all"), value: 0 },
  ] as const;

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const resp = await apiClient.getUsageSummary({
        project_id: selectedProjectId ?? undefined,
        since:
          rangeDays > 0
            ? new Date(Date.now() - rangeDays * 86400000).toISOString()
            : undefined,
      });
      setData(resp);
    } catch (e) {
      setError(getErrorMessage(e));
    } finally {
      setLoading(false);
    }
  }, [apiClient, selectedProjectId, rangeDays]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    if (!autoRefresh) return;
    const id = setInterval(() => void load(), 30000);
    return () => clearInterval(id);
  }, [autoRefresh, load]);

  return (
    <div className="flex-1 space-y-6 p-8">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2">
            <Coins className="h-5 w-5 text-muted-foreground" />
            <h1 className="text-2xl font-bold tracking-tight">{t("usage.title")}</h1>
            {loading ? <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /> : null}
          </div>
          <p className="text-sm text-muted-foreground">
            {t("usage.subtitle")}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <div className="flex rounded-md border">
            {TIME_RANGES.map((tr) => (
              <button
                key={tr.value}
                onClick={() => setRangeDays(tr.value)}
                className={`px-3 py-1.5 text-xs font-medium transition-colors ${
                  rangeDays === tr.value
                    ? "bg-primary text-primary-foreground"
                    : "text-muted-foreground hover:bg-accent"
                } ${tr.value === TIME_RANGES[0].value ? "rounded-l-md" : ""} ${
                  tr.value === TIME_RANGES[TIME_RANGES.length - 1].value ? "rounded-r-md" : ""
                }`}
              >
                {tr.label}
              </button>
            ))}
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setAutoRefresh(!autoRefresh)}
            className={autoRefresh ? "border-emerald-300 text-emerald-600" : ""}
          >
            <RefreshCw className={`mr-1.5 h-3.5 w-3.5 ${autoRefresh ? "animate-spin" : ""}`} />
            {autoRefresh ? t("common.autoRefresh") : t("common.manual")}
          </Button>
          <Button variant="outline" size="sm" onClick={() => void load()} disabled={loading}>
            {t("common.refresh")}
          </Button>
        </div>
      </div>

      {error ? (
        <p className="rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p>
      ) : null}

      {/* Overview cards */}
      {data?.totals ? (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">{t("usage.totalTokens")}</CardTitle>
              <Coins className="h-4 w-4 text-amber-500" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{formatTokens(data.totals.total_tokens)}</div>
              <div className="mt-2 flex flex-wrap gap-1.5">
                <Badge variant="secondary" className="text-xs">
                  <span className="mr-1 inline-block h-2 w-2 rounded-full bg-blue-500" />
                  Input {formatTokens(data.totals.input_tokens)}
                </Badge>
                <Badge variant="secondary" className="text-xs">
                  <span className="mr-1 inline-block h-2 w-2 rounded-full bg-emerald-500" />
                  Output {formatTokens(data.totals.output_tokens)}
                </Badge>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">{t("usage.execCount")}</CardTitle>
              <ArrowDownUp className="h-4 w-4 text-blue-500" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{data.totals.run_count}</div>
              <p className="text-xs text-muted-foreground">
                {t("usage.avgTokensPerExec", {
                  tokens: data.totals.run_count > 0
                    ? formatTokens(Math.round(data.totals.total_tokens / data.totals.run_count))
                    : "0",
                })}
              </p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">{t("usage.cacheHit")}</CardTitle>
              <Database className="h-4 w-4 text-purple-500" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{formatTokens(data.totals.cache_read_tokens)}</div>
              <p className="text-xs text-muted-foreground">
                {t("usage.cacheWrite", { tokens: formatTokens(data.totals.cache_write_tokens) })}
              </p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">{t("usage.reasoningTokens")}</CardTitle>
              <Coins className="h-4 w-4 text-indigo-500" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{formatTokens(data.totals.reasoning_tokens)}</div>
              <p className="text-xs text-muted-foreground">
                {data.totals.total_tokens > 0
                  ? t("usage.percentOfTotal", { pct: Math.round((data.totals.reasoning_tokens / data.totals.total_tokens) * 100) })
                  : t("common.noData")}
              </p>
            </CardContent>
          </Card>
        </div>
      ) : null}

      <div className="grid gap-6 lg:grid-cols-2">
        {/* By Project */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Coins className="h-4 w-4 text-amber-500" />
              {t("usage.byProject")}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("common.project")}</TableHead>
                  <TableHead>{t("usage.execCountCol")}</TableHead>
                  <TableHead>Input</TableHead>
                  <TableHead>Output</TableHead>
                  <TableHead>{t("usage.total")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {!data || data.by_project.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={5} className="text-center text-muted-foreground">
                      {t("common.noData")}
                    </TableCell>
                  </TableRow>
                ) : (
                  data.by_project.map((p) => (
                    <TableRow key={p.project_id}>
                      <TableCell className="font-medium">{p.project_name}</TableCell>
                      <TableCell>{p.run_count}</TableCell>
                      <TableCell className="text-blue-600">{formatTokens(p.input_tokens)}</TableCell>
                      <TableCell className="text-emerald-600">{formatTokens(p.output_tokens)}</TableCell>
                      <TableCell>
                        {tokenBar(p.input_tokens, p.output_tokens, p.total_tokens)}
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>

        {/* By Agent */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Coins className="h-4 w-4 text-blue-500" />
              {t("usage.byAgent")}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Agent</TableHead>
                  <TableHead>{t("common.project")}</TableHead>
                  <TableHead>{t("usage.execCountCol")}</TableHead>
                  <TableHead>Input</TableHead>
                  <TableHead>Output</TableHead>
                  <TableHead>{t("usage.total")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {!data || data.by_agent.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={6} className="text-center text-muted-foreground">
                      {t("common.noData")}
                    </TableCell>
                  </TableRow>
                ) : (
                  data.by_agent.map((a, i) => (
                    <TableRow key={`${a.agent_id}-${a.project_id ?? 0}-${i}`}>
                      <TableCell className="font-medium font-mono text-xs">{a.agent_id}</TableCell>
                      <TableCell className="text-muted-foreground">{a.project_name || "-"}</TableCell>
                      <TableCell>{a.run_count}</TableCell>
                      <TableCell className="text-blue-600">{formatTokens(a.input_tokens)}</TableCell>
                      <TableCell className="text-emerald-600">{formatTokens(a.output_tokens)}</TableCell>
                      <TableCell>
                        {tokenBar(a.input_tokens, a.output_tokens, a.total_tokens)}
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>

      {/* By Profile */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Coins className="h-4 w-4 text-indigo-500" />
            {t("usage.byProfile")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Profile</TableHead>
                <TableHead>Agent</TableHead>
                <TableHead>{t("common.project")}</TableHead>
                <TableHead>{t("usage.execCountCol")}</TableHead>
                <TableHead>Input</TableHead>
                <TableHead>Output</TableHead>
                <TableHead>Cache Read</TableHead>
                <TableHead>Cache Write</TableHead>
                <TableHead>Reasoning</TableHead>
                <TableHead>{t("usage.total")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {!data || data.by_profile.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={10} className="text-center text-muted-foreground">
                    {t("common.noData")}
                  </TableCell>
                </TableRow>
              ) : (
                data.by_profile.map((p, i) => (
                  <TableRow key={`${p.profile_id}-${p.project_id ?? 0}-${i}`}>
                    <TableCell className="font-medium font-mono text-xs">{p.profile_id}</TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">{p.agent_id}</TableCell>
                    <TableCell className="text-muted-foreground">{p.project_name || "-"}</TableCell>
                    <TableCell>{p.run_count}</TableCell>
                    <TableCell className="text-blue-600">{formatTokens(p.input_tokens)}</TableCell>
                    <TableCell className="text-emerald-600">{formatTokens(p.output_tokens)}</TableCell>
                    <TableCell className="text-purple-600">{formatTokens(p.cache_read_tokens)}</TableCell>
                    <TableCell className="text-purple-400">{formatTokens(p.cache_write_tokens)}</TableCell>
                    <TableCell className="text-indigo-600">{formatTokens(p.reasoning_tokens)}</TableCell>
                    <TableCell>
                      {tokenBar(p.input_tokens, p.output_tokens, p.total_tokens)}
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
