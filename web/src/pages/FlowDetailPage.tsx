import { useState, useCallback, useMemo } from "react";
import { useParams, Link } from "react-router-dom";
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  type Node,
  type Edge,
  type NodeTypes,
  Handle,
  Position,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import {
  ChevronRight,
  Play,
  Square,
  Clock,
  Bot,
  CheckCircle2,
  AlertCircle,
  Loader2,
  Pause,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { StatusBadge } from "@/components/status-badge";
import { cn } from "@/lib/utils";

/* ── Custom Step Node ── */
interface StepNodeData {
  label: string;
  type: "exec" | "gate" | "composite";
  status: string;
  role?: string;
  [key: string]: unknown;
}

const statusIcon: Record<string, React.ReactNode> = {
  done: <CheckCircle2 className="h-4 w-4 text-emerald-500" />,
  running: <Loader2 className="h-4 w-4 animate-spin text-blue-500" />,
  failed: <AlertCircle className="h-4 w-4 text-red-500" />,
  waiting_gate: <Pause className="h-4 w-4 text-amber-500" />,
  pending: <Clock className="h-4 w-4 text-zinc-400" />,
  ready: <Play className="h-4 w-4 text-blue-500" />,
};

const statusBorder: Record<string, string> = {
  done: "border-emerald-500",
  running: "border-blue-500",
  failed: "border-red-500",
  waiting_gate: "border-amber-500",
  pending: "border-zinc-200",
  ready: "border-blue-300",
};

function StepNode({ data }: { data: StepNodeData }) {
  return (
    <div className={cn(
      "rounded-lg border-2 bg-white px-4 py-3 shadow-sm min-w-[160px]",
      statusBorder[data.status] ?? "border-zinc-200",
    )}>
      <Handle type="target" position={Position.Top} className="!bg-zinc-400" />
      <div className="flex items-center gap-2">
        {statusIcon[data.status] ?? statusIcon.pending}
        <span className="text-sm font-medium">{data.label}</span>
      </div>
      <div className="mt-1 flex items-center gap-1.5">
        <Badge variant="outline" className="text-[10px] px-1.5 py-0">{data.type}</Badge>
        {data.role && (
          <Badge variant="secondary" className="text-[10px] px-1.5 py-0">{data.role}</Badge>
        )}
      </div>
      <Handle type="source" position={Position.Bottom} className="!bg-zinc-400" />
    </div>
  );
}

const nodeTypes: NodeTypes = {
  step: StepNode,
};

/* ── Mock data ── */
const mockNodes: Node<StepNodeData>[] = [
  { id: "1", type: "step", position: { x: 50, y: 0 }, data: { label: "需求分析", type: "exec", status: "done", role: "worker" } },
  { id: "2", type: "step", position: { x: 0, y: 120 }, data: { label: "实现 API", type: "exec", status: "done", role: "worker" } },
  { id: "3", type: "step", position: { x: 200, y: 120 }, data: { label: "编写测试", type: "exec", status: "running", role: "worker" } },
  { id: "4", type: "step", position: { x: 100, y: 240 }, data: { label: "集成测试", type: "gate", status: "pending", role: "gate" } },
  { id: "5", type: "step", position: { x: 0, y: 360 }, data: { label: "代码审查", type: "gate", status: "pending", role: "gate" } },
  { id: "6", type: "step", position: { x: 200, y: 360 }, data: { label: "性能测试", type: "exec", status: "pending", role: "worker" } },
  { id: "7", type: "step", position: { x: 100, y: 480 }, data: { label: "部署发布", type: "composite", status: "pending", role: "worker" } },
];

const mockEdges: Edge[] = [
  { id: "e1-2", source: "1", target: "2", animated: false },
  { id: "e1-3", source: "1", target: "3", animated: true },
  { id: "e2-4", source: "2", target: "4" },
  { id: "e3-4", source: "3", target: "4" },
  { id: "e4-5", source: "4", target: "5" },
  { id: "e4-6", source: "4", target: "6" },
  { id: "e5-7", source: "5", target: "7" },
  { id: "e6-7", source: "6", target: "7" },
];

interface StepDetail {
  id: string;
  label: string;
  type: string;
  status: string;
  role?: string;
  capabilities: string[];
  acceptance: string[];
  agent?: string;
  executions: { id: number; attempt: number; status: string; duration: string }[];
}

const mockDetail: StepDetail = {
  id: "3",
  label: "编写测试",
  type: "exec",
  status: "running",
  role: "worker",
  capabilities: ["backend", "testing"],
  acceptance: ["覆盖 API 所有公共端点", "测试通过率 100%", "无已知缺陷"],
  agent: "claude-worker",
  executions: [
    { id: 101, attempt: 1, status: "failed", duration: "3m 20s" },
    { id: 102, attempt: 2, status: "running", duration: "1m 45s" },
  ],
};

export function FlowDetailPage() {
  const { flowId } = useParams();
  const [selectedStep, setSelectedStep] = useState<string | null>("3");
  const [nodes] = useState(mockNodes);
  const [edges] = useState(mockEdges);

  const onNodeClick = useCallback((_: React.MouseEvent, node: Node) => {
    setSelectedStep(node.id);
  }, []);

  const detail = selectedStep ? mockDetail : null;

  const edgeStyles = useMemo(() => ({
    stroke: "#94a3b8",
    strokeWidth: 1.5,
  }), []);

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div className="border-b px-8 py-4">
        <div className="flex items-center gap-2 text-sm text-muted-foreground mb-2">
          <Link to="/flows" className="hover:text-foreground">流程</Link>
          <ChevronRight className="h-3 w-3" />
          <span>ai-workflow</span>
          <ChevronRight className="h-3 w-3" />
          <span className="text-foreground">后端 API 重构</span>
        </div>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <h1 className="text-xl font-bold">后端 API 重构</h1>
            <StatusBadge status="running" />
          </div>
          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground">Flow #{flowId ?? 1}</span>
            <span className="text-sm text-muted-foreground">· 7 步骤</span>
            <span className="text-sm text-muted-foreground">· 12 分钟</span>
            <Button variant="outline" size="sm">
              <Square className="mr-2 h-3 w-3" />
              取消
            </Button>
            <Button size="sm">
              <Play className="mr-2 h-3 w-3" />
              运行下一步
            </Button>
          </div>
        </div>
      </div>

      {/* DAG + Detail panel */}
      <div className="flex flex-1 overflow-hidden">
        {/* DAG Canvas */}
        <div className="flex-1 relative">
          <ReactFlow
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            onNodeClick={onNodeClick}
            defaultEdgeOptions={{ style: edgeStyles }}
            fitView
            fitViewOptions={{ padding: 0.3 }}
            proOptions={{ hideAttribution: true }}
          >
            <Background gap={16} size={1} color="#e2e8f0" />
            <Controls
              className="!bg-white !border !shadow-sm !rounded-lg"
              showInteractive={false}
            />
            <MiniMap
              className="!bg-white !border !shadow-sm !rounded-lg"
              nodeColor={(n) => {
                const s = (n.data as StepNodeData)?.status;
                if (s === "done") return "#10b981";
                if (s === "running") return "#3b82f6";
                if (s === "failed") return "#ef4444";
                return "#d4d4d8";
              }}
              maskColor="rgba(0,0,0,0.05)"
            />
          </ReactFlow>
        </div>

        {/* Step detail panel */}
        {detail && (
          <div className="w-80 border-l overflow-y-auto">
            <div className="p-5 space-y-5">
              <div>
                <div className="flex items-center gap-2">
                  <h3 className="font-semibold">{detail.label}</h3>
                  <StatusBadge status={detail.status} />
                </div>
                <div className="mt-2 flex flex-wrap gap-1.5">
                  <Badge variant="outline" className="text-xs">{detail.type}</Badge>
                  {detail.role && <Badge variant="info" className="text-xs">{detail.role}</Badge>}
                </div>
              </div>

              <div>
                <h4 className="text-xs font-medium text-muted-foreground uppercase tracking-wider mb-2">能力要求</h4>
                <div className="flex flex-wrap gap-1.5">
                  {detail.capabilities.map((c) => (
                    <Badge key={c} variant="secondary" className="text-xs">{c}</Badge>
                  ))}
                </div>
              </div>

              <div>
                <h4 className="text-xs font-medium text-muted-foreground uppercase tracking-wider mb-2">验收标准</h4>
                <ul className="space-y-1.5">
                  {detail.acceptance.map((a, i) => (
                    <li key={i} className="flex items-start gap-2 text-sm">
                      <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 text-muted-foreground shrink-0" />
                      {a}
                    </li>
                  ))}
                </ul>
              </div>

              <div>
                <h4 className="text-xs font-medium text-muted-foreground uppercase tracking-wider mb-2">代理</h4>
                <div className="flex items-center gap-2">
                  <Bot className="h-4 w-4 text-muted-foreground" />
                  <span className="text-sm font-medium">{detail.agent}</span>
                </div>
              </div>

              <div>
                <h4 className="text-xs font-medium text-muted-foreground uppercase tracking-wider mb-2">执行历史</h4>
                <div className="space-y-2">
                  {detail.executions.map((exec) => (
                    <Link
                      key={exec.id}
                      to={`/executions/${exec.id}`}
                      className="flex items-center justify-between rounded-md border p-2.5 text-sm hover:bg-muted/50 transition-colors"
                    >
                      <div className="flex items-center gap-2">
                        <span className="font-medium">第 {exec.attempt} 次</span>
                        <StatusBadge status={exec.status} />
                      </div>
                      <span className="text-xs text-muted-foreground">{exec.duration}</span>
                    </Link>
                  ))}
                </div>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
