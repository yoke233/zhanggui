/** @vitest-environment jsdom */

import { describe, expect, it } from "vitest";
import { buildFlow } from "./IssueFlowTree";
import type { TaskStep } from "../types/api";

const buildStep = (overrides: Partial<TaskStep>): TaskStep => ({
  id: "step-1",
  issue_id: "issue-1",
  run_id: "",
  agent_id: "",
  action: "queued",
  stage_id: "",
  input: "",
  output: "",
  note: "",
  ref_id: "",
  ref_type: "",
  created_at: "2026-03-09T10:00:00Z",
  ...overrides,
});

describe("IssueFlowTree", () => {
  it("按真实时序保留 run 前后的无 run_id 步骤", () => {
    const flow = buildFlow(
      [
        buildStep({
          id: "step-queued",
          action: "queued",
          created_at: "2026-03-09T10:00:00Z",
        }),
        buildStep({
          id: "step-run-created",
          run_id: "run-1",
          action: "run_created",
          created_at: "2026-03-09T10:01:00Z",
        }),
        buildStep({
          id: "step-stage-started",
          run_id: "run-1",
          action: "stage_started",
          stage_id: "implement",
          created_at: "2026-03-09T10:02:00Z",
        }),
        buildStep({
          id: "step-ready",
          action: "ready",
          created_at: "2026-03-09T10:03:00Z",
        }),
      ],
      "issue-1",
    );

    expect(flow).toHaveLength(1);
    expect(flow[0]?.children.map((node) => node.id)).toEqual([
      "step-queued",
      "run-run-1",
      "step-ready",
    ]);
    expect(flow[0]?.children[1]?.children.map((node) => node.id)).toEqual([
      "step-run-created",
      "step-stage-started",
    ]);
  });

  it("同一 issue 的多个无 run_id 步骤不会拆成多个根节点", () => {
    const flow = buildFlow(
      [
        buildStep({
          id: "step-submitted",
          action: "submitted_for_review",
          created_at: "2026-03-09T09:00:00Z",
        }),
        buildStep({
          id: "step-approved",
          action: "review_approved",
          created_at: "2026-03-09T09:01:00Z",
        }),
        buildStep({
          id: "step-queued",
          action: "queued",
          created_at: "2026-03-09T09:02:00Z",
        }),
        buildStep({
          id: "step-run-created",
          run_id: "run-2",
          action: "run_created",
          created_at: "2026-03-09T09:03:00Z",
        }),
      ],
      "issue-1",
    );

    expect(flow).toHaveLength(1);
    expect(flow[0]?.id).toBe("issue-issue-1");
    expect(flow[0]?.children).toHaveLength(4);
    expect(flow[0]?.children.map((node) => node.id)).toEqual([
      "step-submitted",
      "step-approved",
      "step-queued",
      "run-run-2",
    ]);
  });
});
