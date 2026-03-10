import { useState, useEffect } from "react";
import { Link } from "react-router-dom";
import {
  Activity,
  GitBranch,
  CheckCircle2,
  Clock,
  Play,
  ArrowUpRight,
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
import { StatusBadge } from "@/components/status-badge";
import { cn } from "@/lib/utils";

interface StatCard {
  title: string;
  value: string | number;
  change?: string;
  changeType?: "up" | "down" | "neutral";
  icon: React.ReactNode;
}

interface RunningFlow {
  id: number;
  name: string;
  project: string;
  status: string;
  steps: string;
  duration: string;
}

interface QueueItem {
  id: number;
  name: string;
  status: "running" | "queued" | "idle";
}

export function DashboardPage() {
  const [stats, setStats] = useState<StatCard[]>([
    { title: "执行中流程", value: 3, change: "+2 较昨日", changeType: "up", icon: <Activity className="h-4 w-4 text-muted-foreground" /> },
    { title: "完成流程", value: 5, change: "上次成功 1h 前", changeType: "neutral", icon: <CheckCircle2 className="h-4 w-4 text-muted-foreground" /> },
    { title: "今日完成", value: 12, change: "成功率 92%", changeType: "up", icon: <GitBranch className="h-4 w-4 text-muted-foreground" /> },
    { title: "排队子任务", value: 1, change: "预计 5 分钟", changeType: "neutral", icon: <Clock className="h-4 w-4 text-muted-foreground" /> },
  ]);

  const [flows] = useState<RunningFlow[]>([
    { id: 1, name: "后端 API 重构", project: "ai-workflow", status: "running", steps: "4/7", duration: "12m" },
    { id: 2, name: "前端 UI 更新", project: "ai-workflow", status: "running", steps: "2/5", duration: "8m" },
    { id: 3, name: "集成测试套件", project: "auth-service", status: "running", steps: "1/3", duration: "3m" },
  ]);

  const [queueItems] = useState<QueueItem[]>([
    { id: 1, name: "部署 API 服务", status: "running" },
    { id: 2, name: "运行集成测试", status: "running" },
    { id: 3, name: "代码审查", status: "queued" },
  ]);

  useEffect(() => {
    // TODO: fetch real stats from API
    void setStats;
  }, []);

  return (
    <div className="flex-1 space-y-6 p-8">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">仪表盘</h1>
          <p className="text-sm text-muted-foreground">
            运行 3 个流程 / 已完成 67 个流程 / 队列 1 个任务
          </p>
        </div>
        <Button>
          <Play className="mr-2 h-4 w-4" />
          新建流程
        </Button>
      </div>

      {/* Stats cards */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        {stats.map((stat) => (
          <Card key={stat.title}>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">{stat.title}</CardTitle>
              {stat.icon}
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{stat.value}</div>
              {stat.change && (
                <p className={cn(
                  "text-xs",
                  stat.changeType === "up" ? "text-emerald-600" : "text-muted-foreground",
                )}>
                  {stat.change}
                </p>
              )}
            </CardContent>
          </Card>
        ))}
      </div>

      {/* Main content row */}
      <div className="grid gap-6 lg:grid-cols-3">
        {/* Running flows table */}
        <Card className="lg:col-span-2">
          <CardHeader className="flex flex-row items-center justify-between">
            <CardTitle>运行中流程</CardTitle>
            <Link to="/flows" className="text-sm text-muted-foreground hover:text-foreground">
              查看全部 <ArrowUpRight className="ml-1 inline h-3 w-3" />
            </Link>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>流程名称</TableHead>
                  <TableHead>项目</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead>步骤</TableHead>
                  <TableHead>耗时</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {flows.map((flow) => (
                  <TableRow key={flow.id}>
                    <TableCell className="font-medium">
                      <Link to={`/flows/${flow.id}`} className="hover:underline">
                        {flow.name}
                      </Link>
                    </TableCell>
                    <TableCell className="text-muted-foreground">{flow.project}</TableCell>
                    <TableCell>
                      <StatusBadge status={flow.status} />
                    </TableCell>
                    <TableCell>{flow.steps}</TableCell>
                    <TableCell className="text-muted-foreground">{flow.duration}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>

        {/* Scheduler panel */}
        <Card className="overflow-hidden p-0">
          {/* Header */}
          <div className="flex items-center justify-between border-b px-5 py-4">
            <h3 className="text-base font-semibold">调度器</h3>
            <Badge variant="success">健康</Badge>
          </div>

          {/* Stats */}
          <div className="space-y-4 p-5">
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">最大并发</span>
              <span className="font-semibold">2</span>
            </div>
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">运行中</span>
              <span className="font-semibold text-blue-500">2 / 2</span>
            </div>
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">排队中</span>
              <span className="font-semibold text-amber-500">1</span>
            </div>
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">平均耗时</span>
              <span className="font-semibold">4m 32s</span>
            </div>
          </div>

          {/* Divider */}
          <div className="border-t" />

          {/* Queue label */}
          <div className="px-5 py-2">
            <span className="text-[11px] font-medium tracking-wider text-muted-foreground">队列</span>
          </div>

          {/* Queue items */}
          <div>
            {queueItems.map((item, i) => (
              <div
                key={item.id}
                className={cn(
                  "flex items-center gap-2.5 px-5 py-2.5",
                  i < queueItems.length - 1 && "border-b",
                )}
              >
                <div className={cn(
                  "h-2 w-2 shrink-0 rounded-full",
                  item.status === "running" ? "bg-blue-500" : "bg-amber-500",
                )} />
                <div className="flex-1 min-w-0">
                  <div className="text-sm font-medium truncate">{item.name}</div>
                  <div className="text-[11px] text-muted-foreground">
                    {item.status === "running" ? `步骤 ${item.id}/3 · 运行 ${item.id} 分钟` : "等待排队"}
                  </div>
                </div>
              </div>
            ))}
          </div>
        </Card>
      </div>
    </div>
  );
}
