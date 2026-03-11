import { useCallback, useEffect, useMemo, useState } from "react";
import { useParams, Link } from "react-router-dom";
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  type Edge,
  type Node,
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
  Loader2,
  Pause,
  AlertCircle,
  FileStack,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { StatusBadge } from "@/components/status-badge";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { cn } from "@/lib/utils";
import { formatFlowDuration, getErrorMessage, normalizeStepTypeLabel } from "@/lib/v2Workbench";
import type { Execution, Flow, Step } from "@/types/apiV2";

interface StepNodeData extends Record<string, unknown> {
  label: string;
  type: Step["type"];
  status: Step["status"];
  role?: string;
}

const statusIcon: Record<string, React.ReactNode> = {
  done: <CheckCircle2 className="h-4 w-4 text-emerald-500" />,
  running: <Loader2 className="h-4 w-4 animate-spin text-blue-500" />,
  failed: <AlertCircle className="h-4 w-4 text-red-500" />,
  waiting_gate: <Pause className="h-4 w-4 text-amber-500" />,
  blocked: <Pause className="h-4 w-4 text-amber-500" />,
  pending: <Clock className="h-4 w-4 text-zinc-400" />,
  queued: <Clock className="h-4 w-4 text-zinc-400" />,
  ready: <Play className="h-4 w-4 text-blue-500" />,
};

const statusBorder: Record<string, string> = {
  done: "border-emerald-500",
  running: "border-blue-500",
  failed: "border-red-500",
  waiting_gate: "border-amber-500",
  blocked: "border-amber-500",
  pending: "border-zinc-200",
  queued: "border-zinc-200",
  ready: "border-blue-300",
};

function StepNode({ data }: { data: StepNodeData }) {
  return (
    <div
      className={cn(
        "min-w-[180px] rounded-lg border-2 bg-white px-4 py-3 shadow-sm",
        statusBorder[data.status] ?? "border-zinc-200",
      )}
    >
      <Handle type="target" position={Position.Top} className="!bg-zinc-400" />
      <div className="flex items-center gap-2">
        {statusIcon[data.status] ?? statusIcon.pending}
        <span className="text-sm font-medium">{data.label}</span>
      </div>
      <div className="mt-1 flex items-center gap-1.5">
        <Badge variant="outline" className="px-1.5 py-0 text-[10px]">{normalizeStepTypeLabel(data.type)}</Badge>
        {data.role ? <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">{data.role}</Badge> : null}
      </div>
      <Handle type="source" position={Position.Bottom} className="!bg-zinc-400" />
    </div>
  );
}

const nodeTypes: NodeTypes = {
  step: StepNode,
};

const buildGraph = (steps: Step[]): { nodes: Node<StepNodeData>[]; edges: Edge[] } => {
  const depthMap = new Map<number, number>();
  const stepMap = new Map(steps.map((step) => [step.id, step]));

  const resolveDepth = (step: Step): number => {
    const cached = depthMap.get(step.id);
    if (cached != null) {
      return cached;
    }
    const dependsOn = step.depends_on ?? [];
    if (dependsOn.length === 0) {
      depthMap.set(step.id, 0);
      return 0;
    }
    const depth = Math.max(
      ...dependsOn.map((dependencyId) => {
        const dependency = stepMap.get(dependencyId);
        return dependency ? resolveDepth(dependency) + 1 : 0;
      }),
    );
    depthMap.set(step.id, depth);
    return depth;
  };

  const columns = new Map<number, Step[]>();
  steps.forEach((step) => {
    const depth = resolveDepth(step);
    const list = columns.get(depth) ?? [];
    list.push(step);
    columns.set(depth, list);
  });

  const nodes = steps.map((step) => {
    const depth = depthMap.get(step.id) ?? 0;
    const siblings = columns.get(depth) ?? [];
    const rowIndex = siblings.findIndex((candidate) => candidate.id === step.id);
    return {
      id: String(step.id),
      type: "step",
      position: { x: depth * 260, y: rowIndex * 150 },
      data: {
        label: step.name,
        type: step.type,
        status: step.status,
        role: step.agent_role,
      },
    } satisfies Node<StepNodeData>;
  });

  const edges = steps.flatMap((step) =>
    (step.depends_on ?? []).map((dependencyId) => ({
      id: `e${dependencyId}-${step.id}`,
      source: String(dependencyId),
      target: String(step.id),
      animated: step.status === "running",
    })),
  );

  return { nodes, edges };
};

export function FlowDetailPage() {
  const { flowId } = useParams();
  const { apiClient, projects } = useWorkbench();
  const numericFlowId = Number.parseInt(flowId ?? "", 10);
  const [flow, setFlow] = useState<Flow | null>(null);
  const [steps, setSteps] = useState<Step[]>([]);
  const [selectedStepId, setSelectedStepId] = useState<number | null>(null);
  const [executions, setExecutions] = useState<Execution[]>([]);
  const [loading, setLoading] = useState(false);
  const [runningAction, setRunningAction] = useState<"idle" | "run" | "cancel" | "save_template">("idle");
  const [error, setError] = useState<string | null>(null);
  const [templateSaved, setTemplateSaved] = useState(false);

  useEffect(() => {
    if (!Number.isFinite(numericFlowId)) {
      return;
    }
    let cancelled = false;

    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        const [flowResp, stepsResp] = await Promise.all([
          apiClient.getFlow(numericFlowId),
          apiClient.listSteps(numericFlowId),
        ]);
        if (cancelled) {
          return;
        }
        setFlow(flowResp);
        setSteps(stepsResp);
        setSelectedStepId((current) => current ?? stepsResp[0]?.id ?? null);
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

    void load();
    return () => {
      cancelled = true;
    };
  }, [apiClient, numericFlowId]);

  useEffect(() => {
    if (selectedStepId == null) {
      setExecutions([]);
      return;
    }
    let cancelled = false;
    const loadExecutions = async () => {
      try {
        const listed = await apiClient.listExecutions(selectedStepId);
        if (!cancelled) {
          const sorted = [...listed].sort((left, right) => right.attempt - left.attempt);
          setExecutions(sorted);
        }
      } catch (loadError) {
        if (!cancelled) {
          setError(getErrorMessage(loadError));
        }
      }
    };
    void loadExecutions();
    return () => {
      cancelled = true;
    };
  }, [apiClient, selectedStepId]);

  const { nodes, edges } = useMemo(() => buildGraph(steps), [steps]);
  const selectedStep = steps.find((step) => step.id === selectedStepId) ?? null;
  const selectedProject = flow?.project_id == null
    ? null
    : projects.find((project) => project.id === flow.project_id) ?? null;

  const onNodeClick = useCallback((_: React.MouseEvent, node: Node) => {
    const nextStepId = Number.parseInt(node.id, 10);
    if (Number.isFinite(nextStepId)) {
      setSelectedStepId(nextStepId);
    }
  }, []);

  const runAction = async (action: "run" | "cancel") => {
    if (!flow) {
      return;
    }
    setRunningAction(action);
    setError(null);
    try {
      if (action === "run") {
        await apiClient.runFlow(flow.id);
      } else {
        await apiClient.cancelFlow(flow.id);
      }
      const refreshed = await apiClient.getFlow(flow.id);
      setFlow(refreshed);
    } catch (actionError) {
      setError(getErrorMessage(actionError));
    } finally {
      setRunningAction("idle");
    }
  };

  const saveAsTemplate = async () => {
    if (!flow || steps.length === 0) return;
    setRunningAction("save_template");
    setError(null);
    try {
      await apiClient.saveFlowAsTemplate(flow.id, {
        name: flow.name,
        description: flow.metadata?.description,
      });
      setTemplateSaved(true);
    } catch (saveError) {
      setError(getErrorMessage(saveError));
    } finally {
      setRunningAction("idle");
    }
  };

  return (
    <div className="flex h-full flex-col">
      <div className="border-b px-8 py-4">
        <div className="mb-2 flex items-center gap-2 text-sm text-muted-foreground">
          <Link to="/flows" className="hover:text-foreground">流程</Link>
          <ChevronRight className="h-3 w-3" />
          <span>{selectedProject?.name ?? "未指定项目"}</span>
          <ChevronRight className="h-3 w-3" />
          <span className="text-foreground">{flow?.name ?? `Flow #${flowId}`}</span>
        </div>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <h1 className="text-xl font-bold">{flow?.name ?? `Flow #${flowId}`}</h1>
            {flow ? <StatusBadge status={flow.status} /> : null}
            {loading ? <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /> : null}
          </div>
          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground">Flow #{flow?.id ?? flowId}</span>
            <span className="text-sm text-muted-foreground">· {steps.length} 步骤</span>
            {flow ? <span className="text-sm text-muted-foreground">· {formatFlowDuration(flow)}</span> : null}
            <Button
              variant="outline"
              size="sm"
              disabled={runningAction !== "idle" || steps.length === 0 || templateSaved}
              onClick={() => void saveAsTemplate()}
              title="保存为模板"
            >
              <FileStack className="mr-2 h-3 w-3" />
              {runningAction === "save_template" ? "保存中..." : templateSaved ? "已保存模板" : "存为模板"}
            </Button>
            <Button variant="outline" size="sm" disabled={runningAction !== "idle"} onClick={() => void runAction("cancel")}>
              <Square className="mr-2 h-3 w-3" />
              {runningAction === "cancel" ? "取消中..." : "取消"}
            </Button>
            <Button size="sm" disabled={runningAction !== "idle"} onClick={() => void runAction("run")}>
              <Play className="mr-2 h-3 w-3" />
              {runningAction === "run" ? "提交中..." : "运行"}
            </Button>
          </div>
        </div>
      </div>

      {error ? <p className="mx-8 mt-4 rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p> : null}

      <div className="flex flex-1 overflow-hidden">
        <div className="relative flex-1">
          <ReactFlow
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            onNodeClick={onNodeClick}
            fitView
            fitViewOptions={{ padding: 0.2 }}
            proOptions={{ hideAttribution: true }}
          >
            <Background gap={16} size={1} color="#e2e8f0" />
            <Controls className="!rounded-lg !border !bg-white !shadow-sm" showInteractive={false} />
            <MiniMap
              className="!rounded-lg !border !bg-white !shadow-sm"
              nodeColor={(node) => {
                const status = (node.data as unknown as StepNodeData)?.status;
                if (status === "done") return "#10b981";
                if (status === "running") return "#3b82f6";
                if (status === "failed") return "#ef4444";
                if (status === "waiting_gate" || status === "blocked") return "#f59e0b";
                return "#d4d4d8";
              }}
              maskColor="rgba(0,0,0,0.05)"
            />
          </ReactFlow>
        </div>

        {selectedStep ? (
          <div className="w-80 overflow-y-auto border-l">
            <div className="space-y-5 p-5">
              <div>
                <div className="flex items-center gap-2">
                  <h3 className="font-semibold">{selectedStep.name}</h3>
                  <StatusBadge status={selectedStep.status} />
                </div>
                <div className="mt-2 flex flex-wrap gap-1.5">
                  <Badge variant="outline" className="text-xs">{normalizeStepTypeLabel(selectedStep.type)}</Badge>
                  {selectedStep.agent_role ? <Badge variant="info" className="text-xs">{selectedStep.agent_role}</Badge> : null}
                </div>
                {selectedStep.description ? (
                  <p className="mt-3 text-sm text-muted-foreground">{selectedStep.description}</p>
                ) : null}
              </div>

              <div>
                <h4 className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">能力要求</h4>
                <div className="flex flex-wrap gap-1.5">
                  {(selectedStep.required_capabilities ?? []).length === 0 ? (
                    <span className="text-sm text-muted-foreground">未设置</span>
                  ) : (
                    selectedStep.required_capabilities?.map((capability) => (
                      <Badge key={capability} variant="secondary" className="text-xs">{capability}</Badge>
                    ))
                  )}
                </div>
              </div>

              <div>
                <h4 className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">验收标准</h4>
                <ul className="space-y-1.5">
                  {(selectedStep.acceptance_criteria ?? []).length === 0 ? (
                    <li className="text-sm text-muted-foreground">未设置</li>
                  ) : (
                    selectedStep.acceptance_criteria?.map((criteria, index) => (
                      <li key={`${criteria}-${index}`} className="flex items-start gap-2 text-sm">
                        <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                        {criteria}
                      </li>
                    ))
                  )}
                </ul>
              </div>

              <div>
                <h4 className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">代理与约束</h4>
                <div className="space-y-2 text-sm">
                  <div className="flex items-center gap-2">
                    <Bot className="h-4 w-4 text-muted-foreground" />
                    <span>{selectedStep.agent_role || "未指定"}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <Clock className="h-4 w-4 text-muted-foreground" />
                    <span>{selectedStep.timeout ?? "未设置 timeout"}</span>
                  </div>
                </div>
              </div>

              <div>
                <h4 className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">执行历史</h4>
                <div className="space-y-2">
                  {executions.length === 0 ? (
                    <div className="text-sm text-muted-foreground">还没有 execution</div>
                  ) : (
                    executions.map((execution) => (
                      <Link
                        key={execution.id}
                        to={`/executions/${execution.id}`}
                        className="flex items-center justify-between rounded-md border p-2.5 text-sm transition-colors hover:bg-muted/50"
                      >
                        <div className="flex items-center gap-2">
                          <span className="font-medium">第 {execution.attempt} 次</span>
                          <StatusBadge status={execution.status} />
                        </div>
                        <span className="text-xs text-muted-foreground">
                          {execution.started_at ?? execution.created_at}
                        </span>
                      </Link>
                    ))
                  )}
                </div>
              </div>
            </div>
          </div>
        ) : null}
      </div>
    </div>
  );
}

