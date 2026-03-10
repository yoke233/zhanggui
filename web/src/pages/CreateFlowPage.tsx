import { useState } from "react";
import { Link } from "react-router-dom";
import {
  ChevronRight,
  Sparkles,
  Play,
  ChevronDown,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

interface PreviewStep {
  id: number;
  name: string;
  type: "exec" | "gate" | "composite";
  role: string;
  depends: string;
}

const stepColors: Record<string, { bg: string; text: string }> = {
  exec: { bg: "bg-blue-50", text: "text-blue-600" },
  gate: { bg: "bg-amber-50", text: "text-amber-600" },
  composite: { bg: "bg-indigo-50", text: "text-indigo-600" },
};

export function CreateFlowPage() {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [aiPrompt, setAiPrompt] = useState(
    "我需要重构后端 API 模块，主要包括：\n1. 提取 Service 层，分离业务逻辑\n2. 统一错误处理和响应格式\n3. 补充集成测试，覆盖率达到 80%\n4. 代码审查后部署到 staging 环境",
  );

  const [previewSteps] = useState<PreviewStep[]>([
    { id: 1, name: "需求分析", type: "exec", role: "worker", depends: "" },
    { id: 2, name: "实现 API", type: "exec", role: "worker", depends: "依赖: #1" },
    { id: 3, name: "编写测试", type: "exec", role: "worker", depends: "依赖: #2" },
    { id: 4, name: "集成测试", type: "gate", role: "gate", depends: "依赖: #2, #3" },
    { id: 5, name: "代码审查", type: "gate", role: "gate", depends: "依赖: #4" },
    { id: 6, name: "性能测试", type: "exec", role: "worker", depends: "依赖: #4" },
    { id: 7, name: "部署发布", type: "composite", role: "worker", depends: "依赖: #5, #6" },
  ]);

  return (
    <div className="flex-1 space-y-6 p-8">
      {/* Header */}
      <div>
        <div className="flex items-center gap-2 text-sm text-muted-foreground mb-1">
          <Link to="/flows" className="hover:text-foreground">流程</Link>
          <ChevronRight className="h-3 w-3" />
          <span className="text-foreground font-medium">新建流程</span>
        </div>
        <h1 className="text-2xl font-bold tracking-tight">新建流程</h1>
        <p className="text-sm text-muted-foreground">
          创建一个新的工作流程，可以手动添加步骤或使用 AI 自动生成
        </p>
      </div>

      <div className="grid gap-6 lg:grid-cols-[1fr_380px]">
        {/* Left column */}
        <div className="space-y-6">
          {/* Basic info card */}
          <Card>
            <CardHeader>
              <CardTitle className="text-base">基本信息</CardTitle>
              <CardDescription>填写流程的基本配置</CardDescription>
            </CardHeader>
            <CardContent className="space-y-5">
              {/* Flow name */}
              <div className="space-y-1.5">
                <label className="text-sm font-medium">流程名称</label>
                <Input
                  placeholder="例如：后端 API 重构"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                />
              </div>

              {/* Project + Kind row */}
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <label className="text-sm font-medium">所属项目</label>
                  <button className="flex h-10 w-full items-center justify-between rounded-md border bg-background px-3 text-sm">
                    <span>ai-workflow</span>
                    <ChevronDown className="h-4 w-4 text-muted-foreground" />
                  </button>
                </div>
                <div className="space-y-1.5">
                  <label className="text-sm font-medium">类型</label>
                  <button className="flex h-10 w-full items-center justify-between rounded-md border bg-background px-3 text-sm">
                    <span>开发</span>
                    <ChevronDown className="h-4 w-4 text-muted-foreground" />
                  </button>
                </div>
              </div>

              {/* Description */}
              <div className="space-y-1.5">
                <label className="text-sm font-medium">流程描述</label>
                <textarea
                  className="flex min-h-[100px] w-full rounded-md border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                  placeholder="描述这个流程要完成的目标和范围..."
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                />
              </div>
            </CardContent>
          </Card>

          {/* AI Generate card */}
          <Card>
            <CardHeader>
              <div className="flex items-center gap-3">
                <div className="flex h-9 w-9 items-center justify-center rounded-md bg-indigo-50">
                  <Sparkles className="h-[18px] w-[18px] text-indigo-500" />
                </div>
                <div>
                  <CardTitle className="text-base">AI 生成步骤</CardTitle>
                  <CardDescription>描述你的需求，AI 将自动生成 DAG 步骤</CardDescription>
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              <textarea
                className="flex min-h-[120px] w-full rounded-md border bg-background px-3 py-2 text-sm leading-relaxed focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                value={aiPrompt}
                onChange={(e) => setAiPrompt(e.target.value)}
              />
              <div className="flex items-center justify-between">
                <span className="text-xs text-muted-foreground">
                  AI 将根据描述自动推断步骤依赖和角色分配
                </span>
                <Button className="bg-indigo-500 hover:bg-indigo-600">
                  <Sparkles className="mr-2 h-4 w-4" />
                  生成步骤
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>

        {/* Right column */}
        <div className="space-y-6">
          {/* Step preview */}
          <Card className="overflow-hidden p-0">
            <div className="flex items-center justify-between border-b px-5 py-3.5">
              <span className="text-sm font-semibold">步骤预览</span>
              <Badge variant="secondary">{previewSteps.length} 步骤</Badge>
            </div>
            <div>
              {previewSteps.map((step, i) => {
                const color = stepColors[step.type] ?? stepColors.exec;
                return (
                  <div
                    key={step.id}
                    className={cn(
                      "flex items-center gap-3 px-5 py-3",
                      i < previewSteps.length - 1 && "border-b",
                    )}
                  >
                    <div className={cn("flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-[11px] font-semibold", color.bg, color.text)}>
                      {step.id}
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="text-[13px] font-medium">{step.name}</div>
                      <div className="text-[11px] text-muted-foreground">
                        {step.type} · {step.role}{step.depends ? ` · ${step.depends}` : ""}
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          </Card>

          {/* Action buttons */}
          <div className="space-y-2.5">
            <Button className="w-full gap-2">
              <Play className="h-4 w-4" />
              创建并运行
            </Button>
            <Button variant="outline" className="w-full">
              仅保存草稿
            </Button>
            <Link to="/flows" className="block">
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
