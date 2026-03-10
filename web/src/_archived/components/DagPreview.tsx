import { useEffect, useMemo, useRef, useState } from "react";
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
import type { ProposalItem } from "../types/api";

interface DagPreviewProps {
  items: ProposalItem[];
  summary: string;
  error?: string | null;
  loading?: boolean;
  onConfirm: (items: ProposalItem[]) => void | Promise<void>;
  onCancel: () => void;
}

const normalizeCSV = (raw: string): string[] =>
  raw
    .split(",")
    .map((item) => item.trim())
    .filter((item, index, list) => item.length > 0 && list.indexOf(item) === index);

const cleanupProposalItems = (items: ProposalItem[]): ProposalItem[] => {
  const validIDs = new Set(items.map((item) => item.temp_id.trim()).filter(Boolean));
  return items.map((item) => ({
    ...item,
    temp_id: item.temp_id.trim(),
    title: item.title.trim(),
    body: item.body,
    labels: item.labels.filter(Boolean),
    depends_on: item.depends_on.filter(
      (dep, index, list) =>
        dep.trim().length > 0 &&
        validIDs.has(dep.trim()) &&
        dep.trim() !== item.temp_id.trim() &&
        list.findIndex((candidate) => candidate.trim() === dep.trim()) === index,
    ),
  }));
};

const nextTempID = (items: ProposalItem[]): string => {
  const used = new Set(items.map((item) => item.temp_id.trim().toUpperCase()));
  const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ";
  for (const letter of alphabet) {
    if (!used.has(letter)) {
      return letter;
    }
  }
  let index = 1;
  while (used.has(`N${index}`)) {
    index += 1;
  }
  return `N${index}`;
};

const buildDepthMap = (items: ProposalItem[]): Record<string, number> => {
  const byID = new Map(items.map((item) => [item.temp_id, item] as const));
  const memo = new Map<string, number>();
  const visiting = new Set<string>();

  const visit = (id: string): number => {
    if (memo.has(id)) {
      return memo.get(id) ?? 0;
    }
    if (visiting.has(id)) {
      return 0;
    }
    visiting.add(id);
    const item = byID.get(id);
    if (!item || item.depends_on.length === 0) {
      memo.set(id, 0);
      visiting.delete(id);
      return 0;
    }
    const depth =
      Math.max(
        ...item.depends_on.map((dep) => (byID.has(dep) ? visit(dep) : 0)),
      ) + 1;
    memo.set(id, depth);
    visiting.delete(id);
    return depth;
  };

  items.forEach((item) => {
    visit(item.temp_id);
  });

  return Object.fromEntries(memo.entries());
};

const buildNodes = (items: ProposalItem[], selectedID: string | null): Node[] => {
  const depthMap = buildDepthMap(items);
  const columns = new Map<number, string[]>();
  items.forEach((item) => {
    const depth = depthMap[item.temp_id] ?? 0;
    const column = columns.get(depth) ?? [];
    column.push(item.temp_id);
    columns.set(depth, column);
  });

  return items.map((item) => {
    const depth = depthMap[item.temp_id] ?? 0;
    const siblings = columns.get(depth) ?? [item.temp_id];
    const index = siblings.indexOf(item.temp_id);
    const selected = selectedID === item.temp_id;
    return {
      id: item.temp_id,
      position: {
        x: depth * 260 + 40,
        y: index * 140 + 40,
      },
      sourcePosition: Position.Right,
      targetPosition: Position.Left,
      data: {
        label: (
          <div className="space-y-1 text-left">
            <div className="text-xs font-semibold text-[#57606a]">{item.temp_id}</div>
            <div className="text-sm font-semibold text-[#24292f]">{item.title || "未命名任务"}</div>
            {item.depends_on.length > 0 ? (
              <div className="text-[11px] text-[#57606a]">
                依赖: {item.depends_on.join(", ")}
              </div>
            ) : null}
          </div>
        ),
      },
      style: {
        width: 220,
        borderRadius: 12,
        border: selected ? "2px solid #0969da" : "1px solid #d0d7de",
        boxShadow: selected
          ? "0 0 0 4px rgba(9, 105, 218, 0.12)"
          : "0 8px 24px rgba(31, 35, 40, 0.08)",
        background: "#ffffff",
        padding: 12,
      },
    };
  });
};

const buildEdges = (items: ProposalItem[]): Edge[] => {
  const validIDs = new Set(items.map((item) => item.temp_id));
  return items.flatMap((item) =>
    item.depends_on
      .filter((dep) => validIDs.has(dep))
      .map((dep) => ({
        id: `${dep}->${item.temp_id}`,
        source: dep,
        target: item.temp_id,
        markerEnd: { type: MarkerType.ArrowClosed, color: "#6e7781" },
        style: { stroke: "#6e7781", strokeWidth: 1.5 },
      })),
  );
};

export function DagPreview({
  items,
  summary,
  error = null,
  loading = false,
  onConfirm,
  onCancel,
}: DagPreviewProps) {
  const [draftItems, setDraftItems] = useState<ProposalItem[]>(items);
  const [selectedID, setSelectedID] = useState<string | null>(items[0]?.temp_id ?? null);
  const [confirmLocked, setConfirmLocked] = useState(false);
  const [childrenMode, setChildrenMode] = useState<"parallel" | "sequential">(
    items[0]?.children_mode === "sequential" ? "sequential" : "parallel",
  );
  const confirmLockedRef = useRef(false);

  useEffect(() => {
    setDraftItems(items);
    setSelectedID(items[0]?.temp_id ?? null);
    setChildrenMode(items[0]?.children_mode === "sequential" ? "sequential" : "parallel");
    setConfirmLocked(false);
    confirmLockedRef.current = false;
  }, [items]);

  useEffect(() => {
    if (!loading) {
      setConfirmLocked(false);
      confirmLockedRef.current = false;
    }
  }, [loading]);

  const normalizedItems = useMemo(
    () => cleanupProposalItems(draftItems.filter((item) => item.temp_id.trim().length > 0)),
    [draftItems],
  );
  const nodes = useMemo(() => buildNodes(normalizedItems, selectedID), [normalizedItems, selectedID]);
  const edges = useMemo(() => buildEdges(normalizedItems), [normalizedItems]);
  const confirmDisabled = loading || confirmLocked || normalizedItems.length === 0;

  const updateItem = (tempID: string, updater: (item: ProposalItem) => ProposalItem) => {
    setDraftItems((current) =>
      current.map((item) => (item.temp_id === tempID ? updater(item) : item)),
    );
  };

  const removeItem = (tempID: string) => {
    setDraftItems((current) =>
      current
        .filter((item) => item.temp_id !== tempID)
        .map((item) => ({
          ...item,
          depends_on: item.depends_on.filter((dep) => dep !== tempID),
        })),
    );
    setSelectedID((current) => (current === tempID ? null : current));
  };

  const addItem = () => {
    const tempID = nextTempID(draftItems);
    setDraftItems((current) => [
      ...current,
      {
        temp_id: tempID,
        title: `新任务 ${tempID}`,
        body: "",
        labels: [],
        depends_on: [],
      },
    ]);
    setSelectedID(tempID);
  };

  return (
    <div className="overflow-hidden rounded-2xl border border-[#d0d7de] bg-white shadow-[0_24px_80px_rgba(31,35,40,0.18)]">
      <div className="flex items-start justify-between gap-4 border-b border-[#d8dee4] px-5 py-4">
        <div className="space-y-1">
          <p className="text-xs font-semibold uppercase tracking-[0.2em] text-[#57606a]">
            DAG Proposal
          </p>
          <h3 className="text-lg font-semibold text-[#24292f]">确认需求拆解方案</h3>
          {summary.trim().length > 0 ? (
            <p className="max-w-3xl text-sm text-[#57606a]">{summary}</p>
          ) : null}
          <div className="pt-1 text-xs text-[#57606a]">
            <span className="mr-2 font-medium">执行模式:</span>
            <button
              type="button"
              className={`rounded-md border px-2 py-1 ${childrenMode === "parallel" ? "border-[#0969da] bg-[#ddf4ff] text-[#0969da]" : "border-[#d0d7de] text-[#57606a]"}`}
              onClick={() => {
                setChildrenMode("parallel");
              }}
              disabled={loading}
            >
              并行
            </button>
            <button
              type="button"
              className={`ml-2 rounded-md border px-2 py-1 ${childrenMode === "sequential" ? "border-[#0969da] bg-[#ddf4ff] text-[#0969da]" : "border-[#d0d7de] text-[#57606a]"}`}
              onClick={() => {
                setChildrenMode("sequential");
              }}
              disabled={loading}
            >
              顺序
            </button>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            type="button"
            className="rounded-md border border-[#d0d7de] px-3 py-2 text-sm text-[#24292f] hover:bg-[#f6f8fa]"
            onClick={addItem}
            disabled={loading}
          >
            新增节点
          </button>
          <button
            type="button"
            className="rounded-md border border-[#d0d7de] px-3 py-2 text-sm text-[#24292f] hover:bg-[#f6f8fa]"
            onClick={onCancel}
            disabled={loading}
          >
            取消
          </button>
          <button
            type="button"
            className="rounded-md bg-[#0969da] px-3 py-2 text-sm font-medium text-white hover:bg-[#0858ba] disabled:cursor-not-allowed disabled:opacity-60"
            onClick={() => {
              if (confirmLockedRef.current || loading || normalizedItems.length === 0) {
                return;
              }
              confirmLockedRef.current = true;
              setConfirmLocked(true);
              const confirmItems = normalizedItems.map((item) => ({
                ...item,
                children_mode: childrenMode,
              }));
              void Promise.resolve(onConfirm(confirmItems))
                .catch(() => undefined)
                .finally(() => {
                  confirmLockedRef.current = false;
                  setConfirmLocked(false);
                });
            }}
            disabled={confirmDisabled}
          >
            {loading || confirmLocked ? "创建中..." : `创建 ${normalizedItems.length} 个 Issue`}
          </button>
        </div>
      </div>

      {error ? (
        <div className="border-b border-[#d8dee4] bg-[#ffebe9] px-5 py-3">
          <p
            role="alert"
            className="rounded-md border border-[#cf222e] bg-[#ffebe9] px-3 py-2 text-sm text-[#cf222e]"
          >
            {error}
          </p>
        </div>
      ) : null}

      <div className="grid min-h-[640px] grid-cols-1 xl:grid-cols-[minmax(0,1.3fr)_420px]">
        <div className="border-b border-[#d8dee4] xl:border-b-0 xl:border-r">
          <div className="h-[360px] bg-[#f6f8fa]">
            <ReactFlow
              nodes={nodes}
              edges={edges}
              fitView
              nodesDraggable={false}
              nodesConnectable={false}
              elementsSelectable
              onNodeClick={(_, node) => {
                setSelectedID(node.id);
              }}
              proOptions={{ hideAttribution: true }}
            >
              <Background color="#d8dee4" gap={18} />
              <Controls showInteractive={false} />
            </ReactFlow>
          </div>
        </div>

        <div className="max-h-[640px] overflow-y-auto bg-white">
          <div className="space-y-3 p-4">
            {normalizedItems.map((item) => {
              const selected = selectedID === item.temp_id;
              return (
                <div
                  key={item.temp_id}
                  className={`rounded-xl border p-3 transition ${
                    selected
                      ? "border-[#0969da] bg-[#ddf4ff]"
                      : "border-[#d8dee4] bg-[#f6f8fa]"
                  }`}
                >
                  <div className="flex items-center justify-between gap-2">
                    <button
                      type="button"
                      className="inline-flex items-center gap-2 text-left"
                      onClick={() => {
                        setSelectedID(item.temp_id);
                      }}
                    >
                      <span className="inline-flex h-6 min-w-6 items-center justify-center rounded-full bg-white px-2 text-xs font-semibold text-[#57606a]">
                        {item.temp_id}
                      </span>
                      <span className="text-sm font-semibold text-[#24292f]">
                        {item.title || "未命名任务"}
                      </span>
                    </button>
                    <button
                      type="button"
                      className="text-xs text-[#cf222e] hover:underline"
                      onClick={() => {
                        removeItem(item.temp_id);
                      }}
                      disabled={loading}
                    >
                      删除
                    </button>
                  </div>

                  <div className="mt-3 space-y-2">
                    <label className="block space-y-1 text-xs font-medium text-[#57606a]">
                      <span>标题</span>
                      <input
                        type="text"
                        value={item.title}
                        className="w-full rounded-md border border-[#d0d7de] bg-white px-2 py-1.5 text-sm text-[#24292f] focus:border-[#0969da] focus:outline-none"
                        onChange={(event) => {
                          updateItem(item.temp_id, (current) => ({
                            ...current,
                            title: event.target.value,
                          }));
                        }}
                      />
                    </label>

                    <label className="block space-y-1 text-xs font-medium text-[#57606a]">
                      <span>描述</span>
                      <textarea
                        value={item.body}
                        rows={4}
                        className="w-full rounded-md border border-[#d0d7de] bg-white px-2 py-1.5 text-sm text-[#24292f] focus:border-[#0969da] focus:outline-none"
                        onChange={(event) => {
                          updateItem(item.temp_id, (current) => ({
                            ...current,
                            body: event.target.value,
                          }));
                        }}
                      />
                    </label>

                    <label className="block space-y-1 text-xs font-medium text-[#57606a]">
                      <span>标签（逗号分隔）</span>
                      <input
                        type="text"
                        value={item.labels.join(", ")}
                        className="w-full rounded-md border border-[#d0d7de] bg-white px-2 py-1.5 text-sm text-[#24292f] focus:border-[#0969da] focus:outline-none"
                        onChange={(event) => {
                          updateItem(item.temp_id, (current) => ({
                            ...current,
                            labels: normalizeCSV(event.target.value),
                          }));
                        }}
                      />
                    </label>

                    <label className="block space-y-1 text-xs font-medium text-[#57606a]">
                      <span>依赖节点（逗号分隔 temp_id）</span>
                      <input
                        type="text"
                        value={item.depends_on.join(", ")}
                        className="w-full rounded-md border border-[#d0d7de] bg-white px-2 py-1.5 text-sm text-[#24292f] focus:border-[#0969da] focus:outline-none"
                        onChange={(event) => {
                          updateItem(item.temp_id, (current) => ({
                            ...current,
                            depends_on: normalizeCSV(event.target.value),
                          }));
                        }}
                      />
                    </label>
                  </div>
                </div>
              );
            })}

            {normalizedItems.length === 0 ? (
              <div className="rounded-xl border border-dashed border-[#d0d7de] p-6 text-center text-sm text-[#57606a]">
                当前没有可创建的节点。
              </div>
            ) : null}
          </div>
        </div>
      </div>
    </div>
  );
}

export default DagPreview;
