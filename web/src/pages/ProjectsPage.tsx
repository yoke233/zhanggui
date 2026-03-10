import { useState } from "react";
import { Plus, Search, FolderOpen, GitBranch, Server, Cloud } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

interface ProjectItem {
  id: number;
  name: string;
  kind: string;
  description: string;
  icon: React.ReactNode;
  status: "active" | "archived";
  resources: string[];
  flows: number;
  running: number;
  successRate: number;
}

export function ProjectsPage() {
  const [search, setSearch] = useState("");

  const [projects] = useState<ProjectItem[]>([
    {
      id: 1,
      name: "ai-workflow",
      kind: "dev",
      description: "工作流编排引擎，目标打造 AI 全流程自动化",
      icon: <GitBranch className="h-5 w-5" />,
      status: "active",
      resources: ["git", "local_fs"],
      flows: 8,
      running: 3,
      successRate: 92,
    },
    {
      id: 2,
      name: "auth-service",
      kind: "dev",
      description: "认证授权微服务，基于 OAuth2.1",
      icon: <Server className="h-5 w-5" />,
      status: "active",
      resources: ["git"],
      flows: 5,
      running: 1,
      successRate: 88,
    },
    {
      id: 3,
      name: "infra-deploy",
      kind: "general",
      description: "基础设施部署配置和运维自动化",
      icon: <Cloud className="h-5 w-5" />,
      status: "active",
      resources: ["s3"],
      flows: 3,
      running: 0,
      successRate: 75,
    },
  ]);

  const filtered = projects.filter((p) =>
    p.name.toLowerCase().includes(search.toLowerCase()) ||
    p.description.toLowerCase().includes(search.toLowerCase()),
  );

  return (
    <div className="flex-1 space-y-6 p-8">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">项目</h1>
          <p className="text-sm text-muted-foreground">管理工作流项目和资源绑定</p>
        </div>
        <div className="flex items-center gap-3">
          <div className="relative w-64">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder="搜索项目..."
              className="pl-9"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
          <Button>
            <Plus className="mr-2 h-4 w-4" />
            新建项目
          </Button>
        </div>
      </div>

      <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
        {filtered.map((project) => (
          <Card key={project.id} className="group cursor-pointer transition-shadow hover:shadow-md">
            <CardContent className="p-6">
              <div className="flex items-start justify-between">
                <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-muted">
                  {project.icon}
                </div>
                <Badge variant={project.status === "active" ? "success" : "secondary"}>
                  {project.status === "active" ? "活跃" : "归档"}
                </Badge>
              </div>

              <div className="mt-4">
                <h3 className="font-semibold">{project.name}</h3>
                <p className="mt-1 text-sm text-muted-foreground line-clamp-2">
                  {project.description}
                </p>
              </div>

              <div className="mt-4 flex flex-wrap gap-1.5">
                <Badge variant="outline" className="text-xs">{project.kind}</Badge>
                {project.resources.map((r) => (
                  <Badge key={r} variant="secondary" className="text-xs">{r}</Badge>
                ))}
              </div>

              <div className="mt-5 grid grid-cols-3 gap-4 border-t pt-4">
                <div>
                  <div className="text-lg font-bold">{project.flows}</div>
                  <div className="text-xs text-muted-foreground">流程</div>
                </div>
                <div>
                  <div className="text-lg font-bold">{project.running}</div>
                  <div className="text-xs text-muted-foreground">运行中</div>
                </div>
                <div>
                  <div className={cn(
                    "text-lg font-bold",
                    project.successRate >= 90 ? "text-emerald-600" :
                    project.successRate >= 80 ? "text-amber-600" : "text-red-600",
                  )}>
                    {project.successRate}%
                  </div>
                  <div className="text-xs text-muted-foreground">成功率</div>
                </div>
              </div>
            </CardContent>
          </Card>
        ))}

        {/* New project card */}
        <Card className="flex cursor-pointer items-center justify-center border-dashed transition-colors hover:border-primary hover:bg-muted/50">
          <CardContent className="flex flex-col items-center gap-2 p-6 text-muted-foreground">
            <FolderOpen className="h-8 w-8" />
            <span className="text-sm font-medium">创建新项目</span>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
