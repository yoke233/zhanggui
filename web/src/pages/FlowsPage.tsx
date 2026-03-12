import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { Plus, Search, GitBranch, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { StatusBadge } from "@/components/status-badge";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { formatIssueDuration, formatRelativeTime, getErrorMessage } from "@/lib/v2Workbench";
import type { Issue } from "@/types/apiV2";

export function FlowsPage() {
  const { apiClient, selectedProject, selectedProjectId } = useWorkbench();
  const [search, setSearch] = useState("");
  const [issues, setIssues] = useState<Issue[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        const listed = await apiClient.listIssues({
          project_id: selectedProjectId ?? undefined,
          archived: false,
          limit: 200,
          offset: 0,
        });
        if (!cancelled) {
          setIssues(listed);
        }
      } catch (loadError) {
        if (!cancelled) {
          setError(getErrorMessage(loadError));
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };
    void load();
    return () => {
      cancelled = true;
    };
  }, [apiClient, selectedProjectId]);

  const filtered = useMemo(
    () =>
      issues.filter((issue) =>
        issue.title.toLowerCase().includes(search.toLowerCase()) ||
        String(issue.id).includes(search),
      ),
    [issues, search],
  );

  return (
    <div className="flex-1 space-y-6 p-8">
      <div className="flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-bold tracking-tight">流程</h1>
            {loading ? <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /> : null}
          </div>
          <p className="text-sm text-muted-foreground">
            {selectedProject ? `当前项目：${selectedProject.name}` : "当前展示全部项目的 issue"}
          </p>
        </div>
        <Link to="/issues/new">
          <Button>
            <Plus className="mr-2 h-4 w-4" />
            新建流程
          </Button>
        </Link>
      </div>

      {error ? <p className="rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p> : null}

      <Card>
        <CardHeader className="flex flex-row items-center gap-4 space-y-0">
          <CardTitle className="flex items-center gap-2">
            <GitBranch className="h-5 w-5" />
            全部流程
          </CardTitle>
          <div className="ml-auto flex w-72 items-center gap-2">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="搜索流程..."
                className="pl-9"
                value={search}
                onChange={(event) => setSearch(event.target.value)}
              />
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>流程名称</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>创建时间</TableHead>
                <TableHead>最近更新</TableHead>
                <TableHead>耗时</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className="text-center text-muted-foreground">
                    当前没有可展示的 issue
                  </TableCell>
                </TableRow>
              ) : (
                filtered.map((issue) => (
                  <TableRow key={issue.id}>
                    <TableCell className="font-medium">
                      <Link to={`/issues/${issue.id}`} className="hover:underline">
                        {issue.title}
                      </Link>
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={issue.status} />
                    </TableCell>
                    <TableCell className="text-muted-foreground">{formatRelativeTime(issue.created_at)}</TableCell>
                    <TableCell className="text-muted-foreground">{formatRelativeTime(issue.updated_at)}</TableCell>
                    <TableCell className="text-muted-foreground">{formatIssueDuration(issue)}</TableCell>
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
