import { useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import {
  ChevronRight,
  Plus,
  Trash2,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useV2Workbench } from "@/contexts/V2WorkbenchContext";
import { getErrorMessage } from "@/lib/v2Workbench";

interface ResourceDraft {
  kind: string;
  uri: string;
  label: string;
}

export function CreateProjectPage() {
  const navigate = useNavigate();
  const { apiClient, reloadProjects } = useV2Workbench();
  const [name, setName] = useState("");
  const [kind, setKind] = useState<"dev" | "general">("dev");
  const [description, setDescription] = useState("");
  const [resources, setResources] = useState<ResourceDraft[]>([
    { kind: "local_fs", uri: "", label: "工作目录" },
  ]);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const filledResourceCount = useMemo(
    () => resources.filter((resource) => resource.kind.trim() && resource.uri.trim()).length,
    [resources],
  );

  const updateResource = (index: number, patch: Partial<ResourceDraft>) => {
    setResources((current) =>
      current.map((resource, currentIndex) =>
        currentIndex === index ? { ...resource, ...patch } : resource,
      ),
    );
  };

  const addResource = () => {
    setResources((current) => [...current, { kind: "git", uri: "", label: "" }]);
  };

  const removeResource = (index: number) => {
    setResources((current) => current.filter((_, currentIndex) => currentIndex !== index));
  };

  const createProject = async () => {
    if (!name.trim()) {
      setError("项目名称不能为空。");
      return;
    }

    setSubmitting(true);
    setError(null);
    try {
      const project = await apiClient.createProject({
        name: name.trim(),
        kind,
        description: description.trim() || undefined,
      });

      const nextResources = resources.filter((resource) => resource.kind.trim() && resource.uri.trim());
      for (const resource of nextResources) {
        await apiClient.createProjectResource(project.id, {
          kind: resource.kind.trim(),
          uri: resource.uri.trim(),
          label: resource.label.trim() || undefined,
        });
      }

      await reloadProjects(project.id);
      navigate("/projects");
    } catch (submitError) {
      setError(getErrorMessage(submitError));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="flex-1 space-y-6 p-8">
      <div>
        <div className="mb-1 flex items-center gap-2 text-sm text-muted-foreground">
          <Link to="/projects" className="hover:text-foreground">项目</Link>
          <ChevronRight className="h-3 w-3" />
          <span className="font-medium text-foreground">新建项目</span>
        </div>
        <h1 className="text-2xl font-bold tracking-tight">新建项目</h1>
        <p className="text-sm text-muted-foreground">真实写入 v2 项目表，并可同时创建资源绑定。</p>
      </div>

      {error ? <p className="rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p> : null}

      <div className="grid gap-6 lg:grid-cols-[1fr_360px]">
        <div className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">基本信息</CardTitle>
              <CardDescription>填写项目的基本配置信息</CardDescription>
            </CardHeader>
            <CardContent className="space-y-5">
              <div className="space-y-1.5">
                <label className="text-sm font-medium">项目名称</label>
                <Input
                  placeholder="例如：ai-workflow"
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                />
              </div>

              <div className="space-y-1.5">
                <label className="text-sm font-medium">项目类型</label>
                <select
                  className="flex h-10 w-full rounded-md border bg-background px-3 text-sm"
                  value={kind}
                  onChange={(event) => setKind(event.target.value as "dev" | "general")}
                >
                  <option value="dev">dev</option>
                  <option value="general">general</option>
                </select>
              </div>

              <div className="space-y-1.5">
                <label className="text-sm font-medium">项目描述</label>
                <textarea
                  className="flex min-h-[80px] w-full rounded-md border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                  placeholder="描述项目的目标、技术栈和范围..."
                  value={description}
                  onChange={(event) => setDescription(event.target.value)}
                />
              </div>
            </CardContent>
          </Card>

          <Card className="overflow-hidden p-0">
            <div className="flex items-center justify-between border-b px-6 py-5">
              <div>
                <h3 className="text-base font-semibold">资源绑定</h3>
                <p className="mt-1 text-[13px] text-muted-foreground">支持本地目录、git 仓库等真实资源。</p>
              </div>
              <Button variant="outline" size="sm" className="gap-1.5" onClick={addResource}>
                <Plus className="h-3.5 w-3.5" />
                添加资源
              </Button>
            </div>
            <div className="space-y-4 p-6">
              {resources.map((resource, index) => (
                <div key={`${resource.kind}-${index}`} className="rounded-lg border p-4">
                  <div className="grid gap-3 md:grid-cols-[120px_minmax(0,1fr)_120px_40px]">
                    <select
                      className="h-10 rounded-md border bg-background px-3 text-sm"
                      value={resource.kind}
                      onChange={(event) => updateResource(index, { kind: event.target.value })}
                    >
                      <option value="local_fs">local_fs</option>
                      <option value="git">git</option>
                      <option value="s3">s3</option>
                    </select>
                    <Input
                      placeholder="URI / 路径 / 仓库地址"
                      value={resource.uri}
                      onChange={(event) => updateResource(index, { uri: event.target.value })}
                    />
                    <Input
                      placeholder="标签"
                      value={resource.label}
                      onChange={(event) => updateResource(index, { label: event.target.value })}
                    />
                    <Button variant="ghost" size="icon" onClick={() => removeResource(index)}>
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          </Card>
        </div>

        <div className="space-y-6">
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
                <Badge variant="secondary">{kind}</Badge>
              </div>
              <div className="flex justify-between text-[13px]">
                <span className="text-muted-foreground">有效资源</span>
                <span className="font-medium">{filledResourceCount} 个绑定</span>
              </div>
              <div className="flex justify-between gap-3 text-[13px]">
                <span className="text-muted-foreground">描述</span>
                <span className="text-right font-medium">{description || "未填写"}</span>
              </div>
            </div>
          </Card>

          <div className="space-y-2.5">
            <Button className="w-full gap-2" disabled={submitting} onClick={() => void createProject()}>
              <Plus className="h-4 w-4" />
              {submitting ? "创建中..." : "创建项目"}
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
