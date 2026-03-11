import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import {
  Plus,
  Search,
  FileStack,
  Loader2,
  Trash2,
  Play,
  ChevronRight,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
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
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { formatRelativeTime, getErrorMessage } from "@/lib/v2Workbench";
import type { DAGTemplate } from "@/types/apiV2";

export function TemplatesPage() {
  const navigate = useNavigate();
  const { apiClient, selectedProjectId } = useWorkbench();
  const [search, setSearch] = useState("");
  const [templates, setTemplates] = useState<DAGTemplate[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<number | null>(null);

  const loadTemplates = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const listed = await apiClient.listDAGTemplates({
        project_id: selectedProjectId ?? undefined,
        limit: 200,
      });
      setTemplates(listed);
    } catch (loadError) {
      setError(getErrorMessage(loadError));
    } finally {
      setLoading(false);
    }
  }, [apiClient, selectedProjectId]);

  useEffect(() => {
    void loadTemplates();
  }, [loadTemplates]);

  const filtered = useMemo(
    () =>
      templates.filter(
        (t) =>
          t.name.toLowerCase().includes(search.toLowerCase()) ||
          (t.description ?? "").toLowerCase().includes(search.toLowerCase()),
      ),
    [templates, search],
  );

  const handleDelete = async (id: number) => {
    setDeleting(id);
    try {
      await apiClient.deleteDAGTemplate(id);
      setTemplates((prev) => prev.filter((t) => t.id !== id));
    } catch (deleteError) {
      setError(getErrorMessage(deleteError));
    } finally {
      setDeleting(null);
    }
  };

  const handleCreateFlow = async (template: DAGTemplate) => {
    setError(null);
    try {
      const result = await apiClient.createFlowFromTemplate(template.id, {
        name: template.name,
        project_id: selectedProjectId ?? undefined,
      });
      navigate(`/flows/${result.flow.id}`);
    } catch (createError) {
      setError(getErrorMessage(createError));
    }
  };

  return (
    <div className="flex-1 space-y-6 p-8">
      <div className="flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-bold tracking-tight">模板</h1>
            {loading ? (
              <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            ) : null}
          </div>
          <p className="text-sm text-muted-foreground">
            管理 DAG 模板，可从模板快速创建流程
          </p>
        </div>
        <Link to="/flows/new">
          <Button>
            <Plus className="mr-2 h-4 w-4" />
            新建流程
          </Button>
        </Link>
      </div>

      {error ? (
        <p className="rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
          {error}
        </p>
      ) : null}

      <Card>
        <CardHeader className="flex flex-row items-center gap-4 space-y-0">
          <CardTitle className="flex items-center gap-2">
            <FileStack className="h-5 w-5" />
            全部模板
          </CardTitle>
          <div className="ml-auto flex w-72 items-center gap-2">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="搜索模板..."
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
                <TableHead>模板名称</TableHead>
                <TableHead>步骤数</TableHead>
                <TableHead>标签</TableHead>
                <TableHead>创建时间</TableHead>
                <TableHead>操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={5}
                    className="text-center text-muted-foreground"
                  >
                    {loading
                      ? "加载中..."
                      : "还没有模板。可以从已有流程保存为模板，或手动创建。"}
                  </TableCell>
                </TableRow>
              ) : (
                filtered.map((template) => (
                  <TableRow key={template.id}>
                    <TableCell>
                      <div>
                        <div className="font-medium">{template.name}</div>
                        {template.description ? (
                          <div className="text-xs text-muted-foreground line-clamp-1">
                            {template.description}
                          </div>
                        ) : null}
                      </div>
                    </TableCell>
                    <TableCell>
                      <Badge variant="secondary">
                        {template.steps.length} 步骤
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-wrap gap-1">
                        {(template.tags ?? []).map((tag) => (
                          <Badge key={tag} variant="outline" className="text-xs">
                            {tag}
                          </Badge>
                        ))}
                      </div>
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {formatRelativeTime(template.created_at)}
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => void handleCreateFlow(template)}
                          title="从此模板创建流程"
                        >
                          <Play className="mr-1 h-3.5 w-3.5" />
                          创建流程
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="text-destructive hover:text-destructive"
                          disabled={deleting === template.id}
                          onClick={() => void handleDelete(template.id)}
                          title="删除模板"
                        >
                          <Trash2 className="h-3.5 w-3.5" />
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

      {/* Template detail cards */}
      {filtered.length > 0 ? (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {filtered.map((template) => (
            <Card key={template.id} className="overflow-hidden">
              <CardHeader className="pb-3">
                <CardTitle className="text-base">{template.name}</CardTitle>
                {template.description ? (
                  <p className="text-xs text-muted-foreground line-clamp-2">
                    {template.description}
                  </p>
                ) : null}
              </CardHeader>
              <CardContent className="space-y-2">
                {template.steps.map((step, i) => (
                  <div
                    key={step.name}
                    className="flex items-center gap-2 text-sm"
                  >
                    <div className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-muted text-[10px] font-semibold">
                      {i + 1}
                    </div>
                    <span className="truncate">{step.name}</span>
                    {step.depends_on?.length ? (
                      <ChevronRight className="h-3 w-3 shrink-0 text-muted-foreground" />
                    ) : null}
                    <Badge variant="outline" className="ml-auto text-[10px]">
                      {step.type}
                    </Badge>
                  </div>
                ))}
              </CardContent>
            </Card>
          ))}
        </div>
      ) : null}
    </div>
  );
}
