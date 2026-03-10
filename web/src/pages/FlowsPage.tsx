import { useState } from "react";
import { Link } from "react-router-dom";
import { Plus, Search, GitBranch } from "lucide-react";
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

interface FlowItem {
  id: number;
  name: string;
  project: string;
  status: string;
  steps: number;
  created: string;
  duration: string;
}

export function FlowsPage() {
  const [search, setSearch] = useState("");

  const [flows] = useState<FlowItem[]>([
    { id: 1, name: "后端 API 重构", project: "ai-workflow", status: "running", steps: 7, created: "10 分钟前", duration: "12m" },
    { id: 2, name: "前端 UI 更新", project: "ai-workflow", status: "running", steps: 5, created: "20 分钟前", duration: "8m" },
    { id: 3, name: "集成测试套件", project: "auth-service", status: "done", steps: 4, created: "1 小时前", duration: "25m" },
    { id: 4, name: "数据库迁移", project: "ai-workflow", status: "done", steps: 3, created: "2 小时前", duration: "15m" },
    { id: 5, name: "部署配置更新", project: "infra-deploy", status: "failed", steps: 6, created: "3 小时前", duration: "45m" },
    { id: 6, name: "API 文档生成", project: "ai-workflow", status: "pending", steps: 2, created: "5 小时前", duration: "-" },
  ]);

  const filtered = flows.filter((f) =>
    f.name.toLowerCase().includes(search.toLowerCase()) ||
    f.project.toLowerCase().includes(search.toLowerCase()),
  );

  return (
    <div className="flex-1 space-y-6 p-8">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">流程</h1>
          <p className="text-sm text-muted-foreground">管理和监控工作流程的执行</p>
        </div>
        <Button>
          <Plus className="mr-2 h-4 w-4" />
          新建流程
        </Button>
      </div>

      <Card>
        <CardHeader className="flex flex-row items-center gap-4 space-y-0">
          <CardTitle className="flex items-center gap-2">
            <GitBranch className="h-5 w-5" />
            全部流程
          </CardTitle>
          <div className="relative ml-auto w-72">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder="搜索流程..."
              className="pl-9"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>流程名称</TableHead>
                <TableHead>项目</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>步骤数</TableHead>
                <TableHead>创建时间</TableHead>
                <TableHead>耗时</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.map((flow) => (
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
                  <TableCell className="text-muted-foreground">{flow.created}</TableCell>
                  <TableCell className="text-muted-foreground">{flow.duration}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
