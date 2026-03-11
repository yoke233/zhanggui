import { useEffect, useState, useCallback } from "react";
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

const TIME_RANGES = [
  { label: "24h", value: 1 },
  { label: "7d", value: 7 },
  { label: "30d", value: 30 },
  { label: "全部", value: 0 },
] as const;

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
  const { apiClient, selectedProjectId } = useWorkbench();
  const [data, setData] = useState<UsageAnalyticsSummary | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [rangeDays, setRangeDays] = useState(7);
  const [autoRefresh, setAutoRefresh] = useState(true);

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
            <h1 className="text-2xl font-bold tracking-tight">用量统计</h1>
            {loading ? <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /> : null}
          </div>
          <p className="text-sm text-muted-foreground">
            Token 消耗分析 - 按项目 / Agent / Profile 维度统计
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
            {autoRefresh ? "自动刷新" : "手动"}
          </Button>
          <Button variant="outline" size="sm" onClick={() => void load()} disabled={loading}>
            刷新
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
              <CardTitle className="text-sm font-medium">总 Token 消耗</CardTitle>
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
              <CardTitle className="text-sm font-medium">执行次数</CardTitle>
              <ArrowDownUp className="h-4 w-4 text-blue-500" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{data.totals.execution_count}</div>
              <p className="text-xs text-muted-foreground">
                平均 {data.totals.execution_count > 0
                  ? formatTokens(Math.round(data.totals.total_tokens / data.totals.execution_count))
                  : "0"} tokens/次
              </p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">缓存命中</CardTitle>
              <Database className="h-4 w-4 text-purple-500" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{formatTokens(data.totals.cache_read_tokens)}</div>
              <p className="text-xs text-muted-foreground">
                缓存写入 {formatTokens(data.totals.cache_write_tokens)}
              </p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">推理 Token</CardTitle>
              <Coins className="h-4 w-4 text-indigo-500" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{formatTokens(data.totals.reasoning_tokens)}</div>
              <p className="text-xs text-muted-foreground">
                {data.totals.total_tokens > 0
                  ? `占总量 ${Math.round((data.totals.reasoning_tokens / data.totals.total_tokens) * 100)}%`
                  : "暂无数据"}
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
              按项目
            </CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>项目</TableHead>
                  <TableHead>执行数</TableHead>
                  <TableHead>Input</TableHead>
                  <TableHead>Output</TableHead>
                  <TableHead>总计</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {!data || data.by_project.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={5} className="text-center text-muted-foreground">
                      暂无数据
                    </TableCell>
                  </TableRow>
                ) : (
                  data.by_project.map((p) => (
                    <TableRow key={p.project_id}>
                      <TableCell className="font-medium">{p.project_name}</TableCell>
                      <TableCell>{p.execution_count}</TableCell>
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
              按 Agent
            </CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Agent</TableHead>
                  <TableHead>项目</TableHead>
                  <TableHead>执行数</TableHead>
                  <TableHead>Input</TableHead>
                  <TableHead>Output</TableHead>
                  <TableHead>总计</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {!data || data.by_agent.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={6} className="text-center text-muted-foreground">
                      暂无数据
                    </TableCell>
                  </TableRow>
                ) : (
                  data.by_agent.map((a, i) => (
                    <TableRow key={`${a.agent_id}-${a.project_id ?? 0}-${i}`}>
                      <TableCell className="font-medium font-mono text-xs">{a.agent_id}</TableCell>
                      <TableCell className="text-muted-foreground">{a.project_name || "-"}</TableCell>
                      <TableCell>{a.execution_count}</TableCell>
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
            按 Profile
          </CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Profile</TableHead>
                <TableHead>Agent</TableHead>
                <TableHead>项目</TableHead>
                <TableHead>执行数</TableHead>
                <TableHead>Input</TableHead>
                <TableHead>Output</TableHead>
                <TableHead>Cache Read</TableHead>
                <TableHead>Cache Write</TableHead>
                <TableHead>Reasoning</TableHead>
                <TableHead>总计</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {!data || data.by_profile.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={10} className="text-center text-muted-foreground">
                    暂无数据
                  </TableCell>
                </TableRow>
              ) : (
                data.by_profile.map((p, i) => (
                  <TableRow key={`${p.profile_id}-${p.project_id ?? 0}-${i}`}>
                    <TableCell className="font-medium font-mono text-xs">{p.profile_id}</TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">{p.agent_id}</TableCell>
                    <TableCell className="text-muted-foreground">{p.project_name || "-"}</TableCell>
                    <TableCell>{p.execution_count}</TableCell>
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
