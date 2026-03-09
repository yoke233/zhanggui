import { useMemo, useState } from "react";
import type { TaskStep } from "../types/api";

type IssueFlowTreeProps = {
  projectId: string;
  issueId: string;
  steps: TaskStep[];
};

export type FlowNode = {
  id: string;
  label: string;
  meta?: string;
  note?: string;
  timestamp?: string;
  children: FlowNode[];
};

const ACTION_LABELS: Record<string, string> = {
  created: "Created",
  submitted_for_review: "Submitted for review",
  review_approved: "Review approved",
  review_rejected: "Review rejected",
  queued: "Queued",
  ready: "Ready",
  execution_started: "Execution started",
  merge_started: "Merge started",
  completed: "Completed",
  merge_completed: "Merge completed",
  failed: "Failed",
  abandoned: "Abandoned",
  decompose_started: "Decompose started",
  decomposed: "Decomposed",
  superseded: "Superseded",
  run_created: "Run created",
  run_started: "Run started",
  stage_started: "Stage started",
  stage_completed: "Stage completed",
  stage_failed: "Stage failed",
  run_completed: "Run completed",
  run_failed: "Run failed",
};

const ACTION_ICONS: Record<string, string> = {
  created: "C",
  submitted_for_review: "R",
  review_approved: "A",
  review_rejected: "X",
  queued: "Q",
  ready: "Y",
  execution_started: "E",
  merge_started: "M",
  completed: "D",
  merge_completed: "G",
  failed: "F",
  abandoned: "B",
  decompose_started: "P",
  decomposed: "S",
  superseded: "U",
  run_created: "RC",
  run_started: "RS",
  stage_started: "SS",
  stage_completed: "SC",
  stage_failed: "SF",
  run_completed: "RD",
  run_failed: "RF",
};

const formatAction = (action: string) => ACTION_LABELS[action] ?? action.replace(/_/g, " ");

const formatTime = (value: string) => {
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString();
};

const stepLabel = (step: TaskStep) => `${ACTION_ICONS[step.action] ?? "?"} ${formatAction(step.action)}`;

const stepMeta = (step: TaskStep) =>
  [step.agent_id ? `agent: ${step.agent_id}` : "", step.stage_id ? `stage: ${step.stage_id}` : ""]
    .filter(Boolean)
    .join(" · ");

const stepNote = (step: TaskStep) => step.note || (step.ref_id ? `ref: ${step.ref_type || "unknown"}/${step.ref_id}` : "");

export const buildFlow = (steps: TaskStep[], issueId: string): FlowNode[] => {
  const ordered = [...steps].sort((left, right) => {
    const leftTime = new Date(left.created_at).getTime();
    const rightTime = new Date(right.created_at).getTime();
    if (Number.isNaN(leftTime) && Number.isNaN(rightTime)) {
      return left.id.localeCompare(right.id);
    }
    if (Number.isNaN(leftTime)) {
      return -1;
    }
    if (Number.isNaN(rightTime)) {
      return 1;
    }
    return leftTime - rightTime;
  });

  const issueRoot: FlowNode = {
    id: `issue-${issueId}`,
    label: `Issue ${issueId}`,
    meta: "lifecycle",
    children: [],
  };

  const runNodes = new Map<string, FlowNode>();
  const issueChildren: FlowNode[] = [];

  ordered.forEach((step) => {
    const node: FlowNode = {
      id: step.id,
      label: stepLabel(step),
      meta: stepMeta(step),
      note: stepNote(step),
      timestamp: step.created_at,
      children: [],
    };

    if (!step.run_id) {
      issueChildren.push(node);
      return;
    }

    let runNode = runNodes.get(step.run_id);
    if (!runNode) {
      runNode = {
        id: `run-${step.run_id}`,
        label: `Run ${step.run_id}`,
        meta: "execution trace",
        children: [],
      };
      runNodes.set(step.run_id, runNode);
      issueChildren.push(runNode);
    }
    runNode.children.push(node);
  });

  issueRoot.children = issueChildren;
  return [issueRoot];
};

function FlowBranch({ node, level }: { node: FlowNode; level: number }) {
  const [expanded, setExpanded] = useState(true);
  const hasChildren = node.children.length > 0;

  return (
    <li>
      <div
        className="flex items-start gap-2 rounded-md px-2 py-1 hover:bg-[#f6f8fa]"
        style={{ marginLeft: `${level * 16}px` }}
      >
        <button
          type="button"
          className="mt-0.5 h-5 w-5 shrink-0 rounded border border-[#d0d7de] bg-white text-[10px] text-[#57606a] disabled:opacity-40"
          disabled={!hasChildren}
          onClick={() => setExpanded((current) => !current)}
        >
          {hasChildren ? (expanded ? "-" : "+") : "."}
        </button>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2 text-xs text-[#57606a]">
            <span className="font-semibold text-[#24292f]">{node.label}</span>
            {node.meta ? <span>{node.meta}</span> : null}
            {node.timestamp ? <span className="ml-auto">{formatTime(node.timestamp)}</span> : null}
          </div>
          {node.note ? <p className="mt-1 text-xs text-[#57606a]">{node.note}</p> : null}
        </div>
      </div>
      {expanded && hasChildren ? (
        <ol className="mt-1 space-y-1">
          {node.children.map((child) => (
            <FlowBranch key={child.id} node={child} level={level + 1} />
          ))}
        </ol>
      ) : null}
    </li>
  );
}

export default function IssueFlowTree({ projectId, issueId, steps }: IssueFlowTreeProps) {
  const flow = useMemo(() => buildFlow(steps, issueId), [steps, issueId]);

  return (
    <section className="rounded-md border border-[#d0d7de] bg-white p-3">
      <div className="flex items-center justify-between gap-3">
        <div>
          <p className="text-xs font-semibold text-[#24292f]">Issue Flow</p>
          <p className="text-[11px] text-[#57606a]">
            {projectId} / {issueId}
          </p>
        </div>
        <span className="rounded-full border border-[#d0d7de] px-2 py-0.5 text-[11px] text-[#57606a]">
          {steps.length} steps
        </span>
      </div>

      {steps.length === 0 ? (
        <p className="mt-3 text-xs text-[#57606a]">No flow data yet.</p>
      ) : (
        <ol className="mt-3 space-y-1">
          {flow.map((node) => (
            <FlowBranch key={node.id} node={node} level={0} />
          ))}
        </ol>
      )}
    </section>
  );
}
