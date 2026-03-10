import { useEffect, useMemo, useState } from "react";
import {
  Background,
  Controls,
  MarkerType,
  ReactFlow,
  type Edge,
  type Node,
  Position,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import type { Step } from "@/types/apiV2";

interface StepsDagProps {
  flowId: number;
  steps: Step[];
  selectedStepId: number | null;
  onSelectStep?: (stepId: number) => void;
  onUpdateStepDependsOn?: (stepId: number, dependsOn: number[]) => Promise<void> | void;
  onCreateStep?: (input: { name: string; type: "exec" | "gate" | "composite"; depends_on?: number[] }) => Promise<void> | void;
  editable?: boolean;
}

const safeNumber = (value: unknown): number | null => {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === "string") {
    const parsed = Number.parseInt(value, 10);
    if (Number.isFinite(parsed)) {
      return parsed;
    }
  }
  return null;
};

const buildDepthMap = (steps: Step[]): Record<number, number> => {
  const byID = new Map(steps.map((step) => [step.id, step] as const));
  const memo = new Map<number, number>();
  const visiting = new Set<number>();

  const visit = (id: number): number => {
    if (memo.has(id)) {
      return memo.get(id) ?? 0;
    }
    if (visiting.has(id)) {
      return 0;
    }
    visiting.add(id);
    const step = byID.get(id);
    const deps = Array.isArray(step?.depends_on) ? step?.depends_on : [];
    if (!step || deps.length === 0) {
      memo.set(id, 0);
      visiting.delete(id);
      return 0;
    }
    const depth =
      Math.max(
        ...deps.map((dep) => (byID.has(dep) ? visit(dep) : 0)),
      ) + 1;
    memo.set(id, depth);
    visiting.delete(id);
    return depth;
  };

  steps.forEach((step) => {
    visit(step.id);
  });

  return Object.fromEntries(memo.entries());
};

const buildNodes = (
  steps: Step[],
  selectedStepId: number | null,
  overrides: Record<number, { x: number; y: number }>,
): Node[] => {
  const depthMap = buildDepthMap(steps);
  const columns = new Map<number, number[]>();
  steps.forEach((step) => {
    const depth = depthMap[step.id] ?? 0;
    const column = columns.get(depth) ?? [];
    column.push(step.id);
    columns.set(depth, column);
  });

  return steps.map((step) => {
    const id = String(step.id);
    const depth = depthMap[step.id] ?? 0;
    const siblings = columns.get(depth) ?? [step.id];
    const index = siblings.indexOf(step.id);
    const selected = step.id === selectedStepId;
    const override = overrides[step.id];
    return {
      id,
      position: override ?? {
        x: depth * 300 + 32,
        y: index * 120 + 32,
      },
      sourcePosition: Position.Right,
      targetPosition: Position.Left,
      data: {
        label: (
          <div className="space-y-1 text-left">
            <div className="flex items-center justify-between gap-2">
              <div className="text-xs font-semibold text-slate-500">#{step.id}</div>
              <Badge variant="outline" className="border-slate-200 bg-slate-50 text-[11px] text-slate-600">
                {step.type}
              </Badge>
            </div>
            <div className="text-sm font-semibold text-slate-900">{step.name || "未命名 Step"}</div>
            {Array.isArray(step.depends_on) && step.depends_on.length > 0 ? (
              <div className="text-[11px] text-slate-500">depends: {step.depends_on.join(", ")}</div>
            ) : null}
          </div>
        ),
      },
      style: {
        width: 240,
        borderRadius: 16,
        border: selected ? "2px solid rgb(199, 210, 254)" : "1px solid rgb(226, 232, 240)",
        boxShadow: selected ? "0 0 0 4px rgba(99, 102, 241, 0.10)" : "0 8px 24px rgba(15, 23, 42, 0.08)",
        background: "#ffffff",
        padding: 12,
      },
    };
  });
};

const buildEdges = (steps: Step[]): Edge[] => {
  const valid = new Set(steps.map((step) => step.id));
  return steps.flatMap((step) =>
    (Array.isArray(step.depends_on) ? step.depends_on : [])
      .filter((dep) => valid.has(dep))
      .map((dep) => ({
        id: `${dep}->${step.id}`,
        source: String(dep),
        target: String(step.id),
        markerEnd: { type: MarkerType.ArrowClosed, color: "#94a3b8" },
        style: { stroke: "#94a3b8", strokeWidth: 1.5 },
      })),
  );
};

const storageKey = (flowId: number) => `v2:flow:${flowId}:dag_positions`;

export default function StepsDag({
  flowId,
  steps,
  selectedStepId,
  onSelectStep,
  onUpdateStepDependsOn,
  onCreateStep,
  editable = true,
}: StepsDagProps) {
  const [positions, setPositions] = useState<Record<number, { x: number; y: number }>>({});
  const [addOpen, setAddOpen] = useState(false);
  const [addName, setAddName] = useState("");
  const [addType, setAddType] = useState<"exec" | "gate" | "composite">("exec");
  const [addDependsOn, setAddDependsOn] = useState("");
  const [addBusy, setAddBusy] = useState(false);
  const [addFeedback, setAddFeedback] = useState<string | null>(null);

  useEffect(() => {
    try {
      const raw = localStorage.getItem(storageKey(flowId));
      if (!raw) {
        setPositions({});
        return;
      }
      const parsed = JSON.parse(raw) as Record<string, unknown>;
      const next: Record<number, { x: number; y: number }> = {};
      Object.entries(parsed).forEach(([key, value]) => {
        const stepId = safeNumber(key);
        const pos = value as { x?: unknown; y?: unknown };
        if (stepId == null) {
          return;
        }
        const x = safeNumber(pos?.x);
        const y = safeNumber(pos?.y);
        if (x == null || y == null) {
          return;
        }
        next[stepId] = { x, y };
      });
      setPositions(next);
    } catch {
      setPositions({});
    }
  }, [flowId]);

  useEffect(() => {
    setAddOpen(false);
    setAddName("");
    setAddType("exec");
    setAddDependsOn("");
    setAddBusy(false);
    setAddFeedback(null);
  }, [flowId]);

  const nodes = useMemo(() => buildNodes(steps, selectedStepId, positions), [steps, selectedStepId, positions]);
  const edges = useMemo(() => buildEdges(steps), [steps]);
  const byId = useMemo(() => new Map(steps.map((step) => [step.id, step] as const)), [steps]);

  const parseDependsOn = (raw: string): number[] =>
    raw
      .split(",")
      .map((item) => item.trim())
      .filter((item) => item.length > 0)
      .map((item) => Number.parseInt(item, 10))
      .filter((num) => Number.isFinite(num) && num > 0);

  const handleQuickCreate = async () => {
    if (!onCreateStep) {
      setAddFeedback("当前页面未启用快捷创建。");
      return;
    }
    if (!editable) {
      setAddFeedback("当前 Flow 已进入执行期，DAG 结构为只读，无法新增 Step。");
      return;
    }
    const name = addName.trim();
    if (!name) {
      setAddFeedback("Step 名称不能为空。");
      return;
    }
    const dependsOn = parseDependsOn(addDependsOn);
    setAddBusy(true);
    setAddFeedback(null);
    try {
      await onCreateStep({
        name,
        type: addType,
        depends_on: dependsOn.length > 0 ? dependsOn : undefined,
      });
      setAddName("");
      setAddDependsOn("");
      setAddFeedback("已创建。");
      setAddOpen(false);
    } catch (err) {
      setAddFeedback(err instanceof Error && err.message.trim() ? err.message : "创建失败，请稍后重试");
    } finally {
      setAddBusy(false);
    }
  };

  if (steps.length === 0) {
    return (
      <div className="rounded-2xl border border-slate-200 bg-white p-4">
        <p className="text-sm font-semibold text-slate-900">DAG 视图</p>
        <p className="mt-2 text-sm text-slate-500">还没有 Step，先在左侧创建 Step，或使用右侧快捷创建。</p>
        {onCreateStep && editable ? (
          <div className="mt-4 rounded-2xl border border-slate-200 bg-slate-50 p-4">
            <div className="flex items-center justify-between gap-3">
              <p className="text-sm font-semibold text-slate-900">快捷添加节点</p>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setAddOpen((v) => !v)}
              >
                {addOpen ? "收起" : "添加 Step"}
              </Button>
            </div>
            {addOpen ? (
              <div className="mt-3 grid gap-3">
                <div className="grid gap-1">
                  <label className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
                    名称
                  </label>
                  <Input value={addName} onChange={(e) => setAddName(e.target.value)} placeholder="例如：implement-api-client" />
                </div>
                <div className="grid gap-3 md:grid-cols-2">
                  <div className="grid gap-1">
                    <label className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
                      类型
                    </label>
                    <Select value={addType} onChange={(e) => setAddType(e.target.value as typeof addType)}>
                      <option value="exec">exec</option>
                      <option value="gate">gate</option>
                      <option value="composite">composite</option>
                    </Select>
                  </div>
                  <div className="grid gap-1">
                    <label className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
                      depends_on（可选）
                    </label>
                    <Input value={addDependsOn} onChange={(e) => setAddDependsOn(e.target.value)} placeholder="例如：12,13" />
                  </div>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button onClick={() => void handleQuickCreate()} disabled={addBusy}>
                    {addBusy ? "创建中..." : "创建"}
                  </Button>
                  <Button
                    variant="outline"
                    onClick={() => {
                      setAddOpen(false);
                      setAddFeedback(null);
                    }}
                    disabled={addBusy}
                  >
                    取消
                  </Button>
                </div>
                {addFeedback ? <p className="text-sm text-slate-600">{addFeedback}</p> : null}
              </div>
            ) : null}
          </div>
        ) : onCreateStep && !editable ? (
          <p className="mt-4 text-sm text-slate-500">当前 Flow 已进入执行期，DAG 结构只读，无法新增 Step。</p>
        ) : null}
      </div>
    );
  }

  return (
    <div className="rounded-2xl border border-slate-200 bg-white p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-sm font-semibold text-slate-900">DAG 视图（编排预览）</p>
          <p className="mt-1 text-xs leading-5 text-slate-500">
            可拖拽调整布局（仅本地保存）。依赖关系来自 Step 的 depends_on；拖拽连线会更新 depends_on。
            {!editable ? "（当前只读）" : null}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {onCreateStep && editable ? (
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setAddOpen((v) => !v);
                setAddFeedback(null);
              }}
            >
              添加 Step
            </Button>
          ) : null}
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setPositions({});
              localStorage.removeItem(storageKey(flowId));
            }}
          >
            重置布局
          </Button>
        </div>
      </div>

      {addOpen && onCreateStep && editable ? (
        <div className="mt-3 rounded-2xl border border-slate-200 bg-slate-50 p-4">
          <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,220px)_minmax(0,220px)_auto] md:items-end">
            <div className="grid gap-1">
              <label className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
                名称
              </label>
              <Input value={addName} onChange={(e) => setAddName(e.target.value)} placeholder="例如：implement-api-client" />
            </div>
            <div className="grid gap-1">
              <label className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
                类型
              </label>
              <Select value={addType} onChange={(e) => setAddType(e.target.value as typeof addType)}>
                <option value="exec">exec</option>
                <option value="gate">gate</option>
                <option value="composite">composite</option>
              </Select>
            </div>
            <div className="grid gap-1">
              <label className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
                depends_on（可选）
              </label>
              <Input value={addDependsOn} onChange={(e) => setAddDependsOn(e.target.value)} placeholder="例如：12,13" />
            </div>
            <div className="flex flex-wrap gap-2">
              <Button onClick={() => void handleQuickCreate()} disabled={addBusy}>
                {addBusy ? "创建中..." : "创建"}
              </Button>
              <Button
                variant="outline"
                onClick={() => {
                  setAddOpen(false);
                  setAddFeedback(null);
                }}
                disabled={addBusy}
              >
                取消
              </Button>
            </div>
          </div>
          {addFeedback ? <p className="mt-3 text-sm text-slate-600">{addFeedback}</p> : null}
        </div>
      ) : null}

      <div className="mt-3 h-[520px] overflow-hidden rounded-2xl border border-slate-200 bg-slate-50">
        <ReactFlow
          nodes={nodes}
          edges={edges}
          fitView
          nodesDraggable
          nodesConnectable={editable && Boolean(onUpdateStepDependsOn)}
          elementsSelectable
          onNodeClick={(_, node) => {
            const id = safeNumber(node.id);
            if (id != null) {
              onSelectStep?.(id);
            }
          }}
          onConnect={(connection) => {
            if (!editable || !onUpdateStepDependsOn) {
              return;
            }
            const source = safeNumber(connection.source);
            const target = safeNumber(connection.target);
            if (source == null || target == null) {
              return;
            }
            if (source === target) {
              return;
            }
            const step = byId.get(target);
            const current = Array.isArray(step?.depends_on) ? step?.depends_on : [];
            const next = Array.from(new Set([...current, source])).filter((id) => id !== target);
            onUpdateStepDependsOn(target, next);
          }}
          onNodeDragStop={(_, node) => {
            const id = safeNumber(node.id);
            if (id == null) {
              return;
            }
            const next = { ...positions, [id]: node.position };
            setPositions(next);
            try {
              localStorage.setItem(storageKey(flowId), JSON.stringify(next));
            } catch {
              // ignore
            }
          }}
          proOptions={{ hideAttribution: true }}
        >
          <Background color="#cbd5e1" gap={18} />
          <Controls showInteractive={false} />
        </ReactFlow>
      </div>
    </div>
  );
}
