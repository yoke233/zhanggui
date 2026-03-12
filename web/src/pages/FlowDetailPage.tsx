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
  GitBranch,
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
import { getScmFlowProviderFromBindings, type SupportedScmProvider } from "@/lib/scm";
import { cn } from "@/lib/utils";
import { formatIssueDuration, getErrorMessage, normalizeStepTypeLabel } from "@/lib/v2Workbench";
import type { Execution, Issue, ResourceBinding, Step } from "@/types/apiV2";

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
  const nodes = steps.map((step, index) => {
    return {
      id: String(step.id),
      type: "step",
      position: { x: 0, y: index * 150 },
      data: {
        label: step.name,
        type: step.type,
        status: step.status,
        role: step.agent_role,
      },
    } satisfies Node<StepNodeData>;
  });

  const edges: Edge[] = [];
  for (let i = 1; i < steps.length; i++) {
    edges.push({
      id: `e${steps[i - 1].id}-${steps[i].id}`,
      source: String(steps[i - 1].id),
      target: String(steps[i].id),
      animated: steps[i].status === "running",
    });
  }

  return { nodes, edges };
};

export function IssueDetailPage() {
  const { flowId: issueIdParam } = useParams();
  const { apiClient, projects } = useWorkbench();
  const numericIssueId = Number.parseInt(issueIdParam ?? "", 10);
  const [issue, setIssue] = useState<Issue | null>(null);
  const [steps, setSteps] = useState<Step[]>([]);
  const [selectedStepId, setSelectedStepId] = useState<number | null>(null);
  const [executions, setExecutions] = useState<Execution[]>([]);
  const [loading, setLoading] = useState(false);
  const [runningAction, setRunningAction] = useState<"idle" | "run" | "cancel" | "save_template">("idle");
  const [bootstrapingPRIssue, setBootstrapingPRIssue] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [templateSaved, setTemplateSaved] = useState(false);
  const [projectResources, setProjectResources] = useState<ResourceBinding[]>([]);

  const fetchIssueData = useCallback(async (targetIssueId: number) => {
    return Promise.all([
      apiClient.getIssue(targetIssueId),
      apiClient.listSteps(targetIssueId),
    ]);
  }, [apiClient]);

  const applyIssueData = useCallback((issueResp: Issue, stepsResp: Step[]) => {
    setIssue(issueResp);
    setSteps(stepsResp);
    setSelectedStepId((current) => (
      current != null && stepsResp.some((step) => step.id === current)
        ? current
        : stepsResp[0]?.id ?? null
    ));
  }, []);

  useEffect(() => {
    if (!Number.isFinite(numericIssueId)) {
      return;
    }
    let cancelled = false;

    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        const [issueResp, stepsResp] = await fetchIssueData(numericIssueId);
        if (!cancelled) {
          applyIssueData(issueResp, stepsResp);
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

    void load();
    return () => {
      cancelled = true;
    };
  }, [applyIssueData, fetchIssueData, numericIssueId]);

  useEffect(() => {
    if (issue?.project_id == null) {
      setProjectResources([]);
      return;
    }
    let cancelled = false;
    const loadResources = async () => {
      try {
        const resources = await apiClient.listProjectResources(issue.project_id!);
        if (!cancelled) {
          setProjectResources(resources);
        }
      } catch {
        if (!cancelled) {
          setProjectResources([]);
        }
      }
    };
    void loadResources();
    return () => {
      cancelled = true;
    };
  }, [apiClient, issue?.project_id]);

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
  const selectedProject = issue?.project_id == null
    ? null
    : projects.find((project) => project.id === issue.project_id) ?? null;
  const scmProvider = useMemo<SupportedScmProvider | null>(
    () => getScmFlowProviderFromBindings(projectResources),
    [projectResources],
  );
  const prIssueDisabledReason = useMemo(() => {
    if (!scmProvider) {
      return "当前项目没有启用 GitHub / Codeup 的 PR/CR 流程资源";
    }
    if (steps.length > 0) {
      return "当前流程已经存在步骤，不能再注入 PR/CR 模板步骤";
    }
    return "";
  }, [scmProvider, steps.length]);

  const onNodeClick = useCallback((_: React.MouseEvent, node: Node) => {
    const nextStepId = Number.parseInt(node.id, 10);
    if (Number.isFinite(nextStepId)) {
      setSelectedStepId(nextStepId);
    }
  }, []);

  const runAction = async (action: "run" | "cancel") => {
    if (!issue) {
      return;
    }
    setRunningAction(action);
    setError(null);
    try {
      if (action === "run") {
        await apiClient.runIssue(issue.id);
      } else {
        await apiClient.cancelIssue(issue.id);
      }
      const refreshed = await apiClient.getIssue(issue.id);
      setIssue(refreshed);
    } catch (actionError) {
      setError(getErrorMessage(actionError));
    } finally {
      setRunningAction("idle");
    }
  };

  const saveAsTemplate = async () => {
    if (!issue || steps.length === 0) return;
    setRunningAction("save_template");
    setError(null);
    try {
      await apiClient.saveIssueAsTemplate(issue.id, {
        name: issue.title,
        description: issue.metadata?.description as string | undefined,
      });
      setTemplateSaved(true);
    } catch (saveError) {
      setError(getErrorMessage(saveError));
    } finally {
      setRunningAction("idle");
    }
  };

  const bootstrapPRIssue = async () => {
    if (!issue || !scmProvider) {
      return;
    }
    setBootstrapingPRIssue(true);
    setError(null);
    try {
      await apiClient.bootstrapPRIssue(issue.id);
      const [issueResp, stepsResp] = await fetchIssueData(issue.id);
      applyIssueData(issueResp, stepsResp);
    } catch (bootstrapError) {
      setError(getErrorMessage(bootstrapError));
    } finally {
      setBootstrapingPRIssue(false);
    }
  };

  return (
    <div className="flex h-full flex-col">
      <div className="border-b px-8 py-4">
        <div className="mb-2 flex items-center gap-2 text-sm text-muted-foreground">
          <Link to="/issues" className="hover:text-foreground">流程</Link>
          <ChevronRight className="h-3 w-3" />
          <span>{selectedProject?.name ?? "未指定项目"}</span>
          <ChevronRight className="h-3 w-3" />
          <span className="text-foreground">{issue?.title ?? `Issue #${issueIdParam}`}</span>
        </div>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <h1 className="text-xl font-bold">{issue?.title ?? `Issue #${issueIdParam}`}</h1>
            {issue ? <StatusBadge status={issue.status} /> : null}
            {loading ? <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /> : null}
          </div>
          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground">Issue #{issue?.id ?? issueIdParam}</span>
            <span className="text-sm text-muted-foreground">· {steps.length} 步骤</span>
            {issue ? <span className="text-sm text-muted-foreground">· {formatIssueDuration(issue)}</span> : null}
            {scmProvider ? (
              <>
                <Badge variant="outline" className="text-xs">
                  {scmProvider === "codeup" ? "Codeup CR" : "GitHub PR"}
                </Badge>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={runningAction !== "idle" || bootstrapingPRIssue || !!prIssueDisabledReason}
                  onClick={() => void bootstrapPRIssue()}
                  title={prIssueDisabledReason || "为当前流程注入 PR/CR 自动化步骤"}
                >
                  <GitBranch className="mr-2 h-3 w-3" />
                  {bootstrapingPRIssue ? "创建中..." : "创建 PR/CR 流程"}
                </Button>
              </>
            ) : null}
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

// Keep backward-compatible export
export { IssueDetailPage as FlowDetailPage };
