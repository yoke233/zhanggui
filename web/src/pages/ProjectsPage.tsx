import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { Plus, Search, FolderOpen, GitBranch, Loader2, Tag } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { getErrorMessage } from "@/lib/v2Workbench";

interface ProjectMetrics {
  issueCount: number;
  activeIssueCount: number;
  successRate: number | null;
  resources: string[];
  hasGit: boolean;
}

export function ProjectsPage() {
  const { apiClient, projects, selectedProjectId, setSelectedProjectId, reloadProjects } = useWorkbench();
  const [search, setSearch] = useState("");
  const [metrics, setMetrics] = useState<Record<number, ProjectMetrics>>({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    const loadMetrics = async () => {
      setLoading(true);
      setError(null);
      try {
        const entries = await Promise.all(
          projects.map(async (project) => {
            const [issues, resources] = await Promise.all([
              apiClient.listIssues({
                project_id: project.id,
                archived: false,
                limit: 200,
                offset: 0,
              }),
              apiClient.listProjectResources(project.id),
            ]);
            const finished = issues.filter((issue) => issue.status === "done" || issue.status === "failed" || issue.status === "cancelled");
            const succeeded = finished.filter((issue) => issue.status === "done");
            const successRate = finished.length > 0 ? Math.round((succeeded.length / finished.length) * 100) : null;
            return [
              project.id,
              {
                issueCount: issues.length,
                activeIssueCount: issues.filter((issue) => issue.status === "queued" || issue.status === "running" || issue.status === "blocked").length,
                successRate,
                resources: resources.map((resource) => resource.kind),
                hasGit: resources.some((resource) => resource.kind.trim().toLowerCase() === "git"),
              },
            ] as const;
          }),
        );
        if (!cancelled) {
          setMetrics(Object.fromEntries(entries));
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

    if (projects.length > 0) {
      void loadMetrics();
    } else {
      setMetrics({});
    }

    return () => {
      cancelled = true;
    };
  }, [apiClient, projects]);

  const filtered = useMemo(
    () =>
      projects.filter((project) =>
        project.name.toLowerCase().includes(search.toLowerCase()) ||
        (project.description ?? "").toLowerCase().includes(search.toLowerCase()),
      ),
    [projects, search],
  );

  return (
    <div className="flex-1 space-y-6 p-8">
      <div className="flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-bold tracking-tight">项目</h1>
            {loading ? <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /> : null}
          </div>
          <p className="text-sm text-muted-foreground">真实读取项目与资源绑定，支持切换当前工作区。</p>
        </div>
        <div className="flex items-center gap-3">
          <Button variant="outline" onClick={() => void reloadProjects(selectedProjectId)}>
            刷新
          </Button>
          <div className="relative w-64">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder="搜索项目..."
              className="pl-9"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
          </div>
          <Link to="/projects/new">
            <Button>
              <Plus className="mr-2 h-4 w-4" />
              新建项目
            </Button>
          </Link>
        </div>
      </div>

      {error ? <p className="rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p> : null}

      <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
        {filtered.map((project) => {
          const projectMetrics = metrics[project.id];
          const isSelected = project.id === selectedProjectId;
          return (
            <Card
              key={project.id}
              className={`group cursor-pointer transition-shadow hover:shadow-md ${isSelected ? "ring-2 ring-primary/30" : ""}`}
              onClick={() => setSelectedProjectId(project.id)}
            >
              <CardContent className="p-6">
                <div className="flex items-start justify-between">
                  <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-muted">
                    <GitBranch className="h-5 w-5" />
                  </div>
                  <Badge variant={isSelected ? "success" : "secondary"}>
                    {isSelected ? "当前项目" : project.kind}
                  </Badge>
                </div>

                <div className="mt-4">
                  <h3 className="font-semibold">{project.name}</h3>
                  <p className="mt-1 line-clamp-2 text-sm text-muted-foreground">
                    {project.description || "未填写项目描述"}
                  </p>
                </div>

                <div className="mt-4 flex flex-wrap gap-1.5">
                  <Badge variant="outline" className="text-xs">{project.kind}</Badge>
                  {(projectMetrics?.resources ?? []).map((resource) => (
                    <Badge key={resource} variant="secondary" className="text-xs">{resource}</Badge>
                  ))}
                </div>

                <div className="mt-5 grid grid-cols-3 gap-4 border-t pt-4">
                  <div>
                    <div className="text-lg font-bold">{projectMetrics?.issueCount ?? 0}</div>
                    <div className="text-xs text-muted-foreground">流程</div>
                  </div>
                  <div>
                    <div className="text-lg font-bold">{projectMetrics?.activeIssueCount ?? 0}</div>
                    <div className="text-xs text-muted-foreground">运行中</div>
                  </div>
                  <div>
                    <div className="text-lg font-bold">
                      {projectMetrics?.successRate == null ? "--" : `${projectMetrics.successRate}%`}
                    </div>
                    <div className="text-xs text-muted-foreground">成功率</div>
                  </div>
                </div>

                {projectMetrics?.hasGit ? (
                  <div className="mt-3 border-t pt-3">
                    <Link
                      to={`/projects/${project.id}/git-tags`}
                      onClick={(e) => e.stopPropagation()}
                    >
                      <Button variant="outline" size="sm" className="h-7 w-full text-xs">
                        <Tag className="mr-1.5 h-3 w-3" />
                        版本标签
                      </Button>
                    </Link>
                  </div>
                ) : null}
              </CardContent>
            </Card>
          );
        })}

        <Link to="/projects/new">
          <Card className="flex cursor-pointer items-center justify-center border-dashed transition-colors hover:border-primary hover:bg-muted/50">
            <CardContent className="flex flex-col items-center gap-2 p-6 text-muted-foreground">
              <FolderOpen className="h-8 w-8" />
              <span className="text-sm font-medium">创建新项目</span>
            </CardContent>
          </Card>
        </Link>
      </div>
    </div>
  );
}
