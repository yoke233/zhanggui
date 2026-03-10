import { useState } from "react";
import { Link } from "react-router-dom";
import {
  ChevronRight,
  ChevronDown,
  Plus,
  GitBranch,
  FolderOpen,
  X,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

interface ResourceItem {
  id: number;
  kind: string;
  uri: string;
  icon: React.ReactNode;
  iconBg: string;
}

export function CreateProjectPage() {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");

  const [resources] = useState<ResourceItem[]>([
    { id: 1, kind: "Git 仓库", uri: "github.com/yoke233/ai-workflow", icon: <GitBranch className="h-4 w-4 text-emerald-600" />, iconBg: "bg-emerald-50" },
    { id: 2, kind: "本地文件系统", uri: "/home/user/projects/ai-workflow", icon: <FolderOpen className="h-4 w-4 text-blue-600" />, iconBg: "bg-blue-50" },
  ]);

  return (
    <div className="flex-1 space-y-6 p-8">
      {/* Header */}
      <div>
        <div className="flex items-center gap-2 text-sm text-muted-foreground mb-1">
          <Link to="/projects" className="hover:text-foreground">项目</Link>
          <ChevronRight className="h-3 w-3" />
          <span className="text-foreground font-medium">新建项目</span>
        </div>
        <h1 className="text-2xl font-bold tracking-tight">新建项目</h1>
        <p className="text-sm text-muted-foreground">创建一个项目来组织流程和资源</p>
      </div>

      <div className="grid gap-6 lg:grid-cols-[1fr_360px]">
        {/* Left column */}
        <div className="space-y-6">
          {/* Basic info */}
          <Card>
            <CardHeader>
              <CardTitle className="text-base">基本信息</CardTitle>
              <CardDescription>填写项目的基本配置信息</CardDescription>
            </CardHeader>
            <CardContent className="space-y-5">
              <div className="space-y-1.5">
                <label className="text-sm font-medium">项目名称</label>
                <Input
                  placeholder="例如：my-awesome-project"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                />
              </div>

              <div className="space-y-1.5">
                <label className="text-sm font-medium">项目类型</label>
                <button className="flex h-10 w-full items-center justify-between rounded-md border bg-background px-3 text-sm">
                  <span>dev</span>
                  <ChevronDown className="h-4 w-4 text-muted-foreground" />
                </button>
              </div>

              <div className="space-y-1.5">
                <label className="text-sm font-medium">项目描述</label>
                <textarea
                  className="flex min-h-[80px] w-full rounded-md border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                  placeholder="描述项目的目标、技术栈和范围..."
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                />
              </div>
            </CardContent>
          </Card>

          {/* Resource bindings */}
          <Card className="overflow-hidden p-0">
            <div className="flex items-center justify-between border-b px-6 py-5">
              <div>
                <h3 className="text-base font-semibold">资源绑定</h3>
                <p className="text-[13px] text-muted-foreground mt-1">关联代码仓库、文件系统或存储服务</p>
              </div>
              <Button variant="outline" size="sm" className="gap-1.5">
                <Plus className="h-3.5 w-3.5" />
                添加资源
              </Button>
            </div>
            <div>
              {resources.map((res, i) => (
                <div
                  key={res.id}
                  className={cn(
                    "flex items-center gap-3 px-6 py-3.5",
                    i < resources.length - 1 && "border-b",
                  )}
                >
                  <div className={cn("flex h-8 w-8 shrink-0 items-center justify-center rounded-md", res.iconBg)}>
                    {res.icon}
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="text-[13px] font-medium">{res.kind}</div>
                    <div className="text-xs text-muted-foreground truncate">{res.uri}</div>
                  </div>
                  <button className="shrink-0 text-muted-foreground hover:text-foreground transition-colors">
                    <X className="h-4 w-4" />
                  </button>
                </div>
              ))}
            </div>
          </Card>
        </div>

        {/* Right column */}
        <div className="space-y-6">
          {/* Preview */}
          <Card className="overflow-hidden p-0">
            <div className="border-b px-5 py-3.5">
              <span className="text-sm font-semibold">项目预览</span>
            </div>
            <div className="space-y-4 p-5">
              <div className="flex justify-between text-[13px]">
                <span className="text-muted-foreground">名称</span>
                <span className="font-medium">{name || "未填写"}</span>
              </div>
              <div className="flex items-center justify-between text-[13px]">
                <span className="text-muted-foreground">类型</span>
                <Badge variant="secondary">dev</Badge>
              </div>
              <div className="flex justify-between text-[13px]">
                <span className="text-muted-foreground">资源</span>
                <span className="font-medium">{resources.length} 个绑定</span>
              </div>
              <div className="flex justify-between text-[13px]">
                <span className="text-muted-foreground">描述</span>
                <span className={cn("font-medium", !description && "italic text-muted-foreground")}>
                  {description || "未填写"}
                </span>
              </div>
            </div>
          </Card>

          {/* Actions */}
          <div className="space-y-2.5">
            <Button className="w-full gap-2">
              <Plus className="h-4 w-4" />
              创建项目
            </Button>
            <Link to="/projects" className="block">
              <Button variant="ghost" className="w-full text-muted-foreground">
                取消
              </Button>
            </Link>
          </div>
        </div>
      </div>
    </div>
  );
}
