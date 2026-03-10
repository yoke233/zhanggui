import { useState, useEffect, useRef } from "react";
import { useParams, Link } from "react-router-dom";
import {
  ChevronRight,
  FileText,
  CheckCircle2,
  AlertTriangle,
  Clock,
  Bot,
  Terminal,
  Wrench,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { StatusBadge } from "@/components/status-badge";
import { cn } from "@/lib/utils";

interface LogLine {
  time: string;
  type: "text" | "tool_call" | "tool_result" | "thinking";
  content: string;
}

export function ExecutionDetailPage() {
  const { execId } = useParams();
  const logEndRef = useRef<HTMLDivElement>(null);

  const [logs] = useState<LogLine[]>([
    { time: "14:23:01", type: "text", content: "> go test ./internal/v2/api/..." },
    { time: "14:23:02", type: "text", content: "=== RUN   TestFlowLifecycle" },
    { time: "14:23:02", type: "text", content: "    PASS  TestCreateFlow" },
    { time: "14:23:03", type: "text", content: "    PASS  TestListFlows" },
    { time: "14:23:03", type: "text", content: "    PASS  TestRunFlow" },
    { time: "14:23:04", type: "text", content: "    FAIL  TestCancelFlow" },
    { time: "14:23:04", type: "text", content: "        Expected status 'cancelled', got 'running'" },
    { time: "14:23:05", type: "thinking", content: "分析 TestCancelFlow 失败原因..." },
    { time: "14:23:06", type: "tool_call", content: "tool_call: read_file(\"internal/v2/engine/flow_scheduler.go\")" },
    { time: "14:23:08", type: "tool_result", content: "读取了 245 行代码" },
    { time: "14:23:10", type: "text", content: "找到了问题：Cancel 方法在 flow 状态为 running 时未正确处理..." },
    { time: "14:23:12", type: "tool_call", content: "tool_call: edit_file(\"internal/v2/engine/flow_scheduler.go\", line 156)" },
    { time: "14:23:15", type: "text", content: "已修复 Cancel 方法的状态转换逻辑" },
    { time: "14:23:16", type: "text", content: "> go test ./internal/v2/api/... -run TestCancelFlow" },
    { time: "14:23:17", type: "text", content: "    PASS  TestCancelFlow (0.8s)" },
    { time: "14:23:18", type: "text", content: "" },
    { time: "14:23:18", type: "text", content: "PASS" },
    { time: "14:23:18", type: "text", content: "ok   github.com/yoke233/ai-workflow/internal/v2/api  3.4s" },
  ]);

  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [logs]);

  return (
    <div className="flex h-full flex-col">
      {/* Breadcrumb header */}
      <div className="border-b px-8 py-4">
        <div className="flex items-center gap-2 text-sm text-muted-foreground mb-2">
          <Link to="/flows" className="hover:text-foreground">流程</Link>
          <ChevronRight className="h-3 w-3" />
          <Link to="/flows/1" className="hover:text-foreground">后端 API 重构</Link>
          <ChevronRight className="h-3 w-3" />
          <span className="text-foreground">集成测试</span>
          <span className="mx-1">·</span>
          <span className="text-foreground">执行详情</span>
        </div>
        <div className="flex items-center gap-3">
          <h1 className="text-xl font-bold">集成测试 — 第 2 次尝试</h1>
          <StatusBadge status="running" />
          <span className="ml-auto text-sm text-muted-foreground">Execution #{execId ?? 102}</span>
        </div>
      </div>

      {/* Content */}
      <div className="flex flex-1 overflow-hidden">
        {/* Left column: Briefing + Submission */}
        <div className="w-[420px] border-r overflow-y-auto p-6 space-y-6">
          {/* Briefing */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="flex items-center gap-2 text-base">
                <FileText className="h-4 w-4" />
                任务说明
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div>
                <h4 className="text-xs font-medium text-muted-foreground uppercase tracking-wider mb-2">目标</h4>
                <p className="text-sm leading-relaxed">
                  对后端所有 API 进行集成测试。目标是验证 CRUD 操作、
                  状态转换和错误处理。请确保测试覆盖率不低于 80%。
                </p>
              </div>
              <div>
                <h4 className="text-xs font-medium text-muted-foreground uppercase tracking-wider mb-2">约束</h4>
                <ul className="space-y-1.5 text-sm">
                  <li className="flex items-start gap-2">
                    <AlertTriangle className="mt-0.5 h-3.5 w-3.5 text-amber-500 shrink-0" />
                    使用 Go 内置测试框架
                  </li>
                  <li className="flex items-start gap-2">
                    <AlertTriangle className="mt-0.5 h-3.5 w-3.5 text-amber-500 shrink-0" />
                    不修改生产代码逻辑
                  </li>
                  <li className="flex items-start gap-2">
                    <AlertTriangle className="mt-0.5 h-3.5 w-3.5 text-amber-500 shrink-0" />
                    测试需可重复运行
                  </li>
                </ul>
              </div>
              <div>
                <h4 className="text-xs font-medium text-muted-foreground uppercase tracking-wider mb-2">验收标准</h4>
                <ul className="space-y-1.5 text-sm">
                  <li className="flex items-start gap-2">
                    <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 text-emerald-500 shrink-0" />
                    覆盖 API 所有公共端点
                  </li>
                  <li className="flex items-start gap-2">
                    <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 text-emerald-500 shrink-0" />
                    测试通过率 100%
                  </li>
                  <li className="flex items-start gap-2">
                    <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 text-muted-foreground shrink-0" />
                    无已知缺陷
                  </li>
                </ul>
              </div>
            </CardContent>
          </Card>

          {/* Submission / Result */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="flex items-center gap-2 text-base">
                <CheckCircle2 className="h-4 w-4" />
                提交结果
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="flex items-center gap-3 rounded-lg border border-dashed p-4">
                <Clock className="h-5 w-5 text-muted-foreground" />
                <div>
                  <p className="text-sm font-medium">等待执行完成</p>
                  <p className="text-xs text-muted-foreground">Agent 正在运行中...</p>
                </div>
              </div>
            </CardContent>
          </Card>

          {/* Agent info */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="flex items-center gap-2 text-base">
                <Bot className="h-4 w-4" />
                代理信息
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-2 text-sm">
              <div className="flex justify-between">
                <span className="text-muted-foreground">配置</span>
                <span className="font-medium">claude-worker</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">驱动</span>
                <span className="font-medium">claude</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">尝试</span>
                <span className="font-medium">第 2 次 / 最多 3 次</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">耗时</span>
                <span className="font-medium">1m 45s</span>
              </div>
            </CardContent>
          </Card>
        </div>

        {/* Right column: Agent Output */}
        <div className="flex-1 flex flex-col bg-zinc-950">
          {/* Terminal header */}
          <div className="flex items-center justify-between border-b border-zinc-800 px-5 py-3">
            <div className="flex items-center gap-2">
              <Terminal className="h-4 w-4 text-zinc-400" />
              <span className="text-sm font-medium text-zinc-300">代理输出</span>
            </div>
            <Badge variant="outline" className="border-zinc-700 text-zinc-400 text-xs">
              实时
            </Badge>
          </div>

          {/* Log output */}
          <div className="flex-1 overflow-y-auto p-5 font-mono text-sm">
            {logs.map((line, i) => (
              <div key={i} className="flex gap-3 leading-6">
                <span className="shrink-0 text-zinc-600 select-none">{line.time}</span>
                {line.type === "tool_call" ? (
                  <span className="text-blue-400 flex items-center gap-1.5">
                    <Wrench className="h-3 w-3" />
                    {line.content}
                  </span>
                ) : line.type === "tool_result" ? (
                  <span className="text-zinc-500 italic">{line.content}</span>
                ) : line.type === "thinking" ? (
                  <span className="text-amber-400 italic">{line.content}</span>
                ) : line.content.includes("FAIL") || line.content.includes("Expected") ? (
                  <span className="text-red-400">{line.content}</span>
                ) : line.content.includes("PASS") ? (
                  <span className="text-emerald-400">{line.content}</span>
                ) : (
                  <span className="text-zinc-300">{line.content}</span>
                )}
              </div>
            ))}
            <div ref={logEndRef} />
          </div>

          {/* Tool call bar */}
          <div className="border-t border-zinc-800 px-5 py-2.5 flex items-center gap-2">
            <Wrench className="h-3.5 w-3.5 text-zinc-500" />
            <span className="text-xs text-zinc-500">
              tool_call: <span className="text-zinc-400">edit_file</span> · 耗时 2.3s · 已修改 1 文件
            </span>
          </div>
        </div>
      </div>
    </div>
  );
}
