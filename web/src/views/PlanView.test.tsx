/** @vitest-environment jsdom */

import type { ReactNode } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import PlanView, { resolveMiniMapNodeColor } from "./PlanView";
import type { ApiClient } from "../lib/apiClient";
import type { WsClient } from "../lib/wsClient";
import type { ApiTaskItem, ApiTaskPlan, ListPlansResponse } from "../types/api";

vi.mock("@xyflow/react", () => {
  return {
    BackgroundVariant: { Dots: "dots" },
    MarkerType: { ArrowClosed: "arrowclosed" },
    ReactFlowProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
    ReactFlow: ({ nodes }: { nodes: Array<{ id: string; data: { label: string } }> }) => (
      <div data-testid="mock-react-flow">
        {nodes.map((node) => (
          <div key={node.id}>{node.data.label}</div>
        ))}
      </div>
    ),
    Background: () => <div data-testid="flow-background" />,
    Controls: () => <div data-testid="flow-controls" />,
    MiniMap: () => <div data-testid="flow-minimap" />,
  };
});

const buildPlan = (
  id: string,
  name: string,
  overrides?: Partial<ApiTaskPlan>,
): ApiTaskPlan => ({
  id,
  project_id: "proj-1",
  session_id: "chat-1",
  name,
  status: "draft",
  wait_reason: "",
  tasks: [],
  fail_policy: "block",
  review_round: 0,
  spec_profile: "default",
  contract_version: "v1",
  contract_checksum: "checksum",
  created_at: "2026-03-01T10:00:00.000Z",
  updated_at: "2026-03-01T10:00:00.000Z",
  ...overrides,
});

const buildTask = (id: string, overrides?: Partial<ApiTaskItem>): ApiTaskItem => ({
  id,
  plan_id: "plan-1",
  title: `Task ${id}`,
  description: "task",
  labels: [],
  depends_on: [],
  inputs: [],
  outputs: [],
  acceptance: [],
  constraints: [],
  template: "standard",
  pipeline_id: "",
  external_id: "",
  status: "pending",
  created_at: "2026-03-01T10:00:00.000Z",
  updated_at: "2026-03-01T10:00:00.000Z",
  ...overrides,
});

const createMockApiClient = (): ApiClient => {
  return {
    request: vi.fn(),
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    del: vi.fn(),
    getStats: vi.fn(),
    listProjects: vi.fn(),
    createProject: vi.fn(),
    listPipelines: vi.fn(),
    createPipeline: vi.fn(),
    createChat: vi.fn(),
    getChat: vi.fn(),
    createPlan: vi.fn(),
    createPlanFromFiles: vi.fn(),
    submitPlanReview: vi.fn().mockResolvedValue({ status: "reviewing" }),
    applyPlanAction: vi.fn().mockResolvedValue({ status: "executing" }),
    applyTaskAction: vi.fn(),
    getPipeline: vi.fn(),
    getPipelineCheckpoints: vi.fn(),
    applyPipelineAction: vi.fn(),
    listPlans: vi.fn().mockResolvedValue({
      items: [buildPlan("plan-1", "Plan One"), buildPlan("plan-2", "Plan Two")],
      total: 2,
      offset: 0,
    }),
    getPlanDag: vi.fn().mockImplementation(async (_projectID: string, planID: string) => ({
      nodes: [
        { id: `${planID}-a`, title: "Task A", status: "pending", pipeline_id: "" },
        { id: `${planID}-b`, title: "Task B", status: "running", pipeline_id: "" },
      ],
      edges: [{ from: `${planID}-a`, to: `${planID}-b` }],
      stats: {
        total: 2,
        pending: 1,
        ready: 0,
        running: 1,
        done: 0,
        failed: 0,
      },
    })),
  } as unknown as ApiClient;
};

const createDeferred = <T,>() => {
  let resolve: (value: T | PromiseLike<T>) => void = () => {};
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
};

const createMockWsClient = (): WsClient => {
  return {
    connect: vi.fn(),
    disconnect: vi.fn(),
    send: vi.fn(),
    subscribe: vi.fn().mockReturnValue(() => {}),
    onStatusChange: vi.fn().mockReturnValue(() => {}),
    getStatus: vi.fn().mockReturnValue("open"),
  } as unknown as WsClient;
};

describe("PlanView", () => {
  afterEach(() => {
    cleanup();
  });

  it("加载计划列表并获取默认计划 DAG", async () => {
    const apiClient = createMockApiClient();
    const wsClient = createMockWsClient();

    render(
      <PlanView
        apiClient={apiClient}
        wsClient={wsClient}
        projectId="proj-1"
        refreshToken={0}
      />,
    );

    await waitFor(() => {
      expect(apiClient.listPlans).toHaveBeenCalledWith("proj-1", {
        limit: 50,
        offset: 0,
      });
      expect(apiClient.getPlanDag).toHaveBeenCalledWith("proj-1", "plan-1");
      expect(wsClient.send).toHaveBeenCalledWith({
        type: "subscribe_plan",
        plan_id: "plan-1",
      });
    });

    await waitFor(() => {
      expect(screen.getByTestId("mock-react-flow")).toBeTruthy();
    });
    expect(screen.getByText("Plan One")).toBeTruthy();
    expect(screen.getByText("total: 2")).toBeTruthy();
  });

  it("切换计划后拉取新 DAG", async () => {
    const apiClient = createMockApiClient();
    const wsClient = createMockWsClient();

    render(
      <PlanView
        apiClient={apiClient}
        wsClient={wsClient}
        projectId="proj-1"
        refreshToken={0}
      />,
    );

    await waitFor(() => {
      expect(apiClient.getPlanDag).toHaveBeenCalledWith("proj-1", "plan-1");
    });

    fireEvent.click(screen.getAllByTestId("plan-item")[1]);

    await waitFor(() => {
      expect(apiClient.getPlanDag).toHaveBeenCalledWith("proj-1", "plan-2");
    });
  });

  it("第一页满 50 条时会继续拉取第二页并渲染补齐数据", async () => {
    const apiClient = createMockApiClient();
    const wsClient = createMockWsClient();
    vi.mocked(apiClient.listPlans)
      .mockResolvedValueOnce({
        items: Array.from({ length: 50 }, (_, index) => buildPlan(`plan-${index}`, `Plan ${index}`)),
        total: 50,
        offset: 0,
      })
      .mockResolvedValueOnce({
        items: [buildPlan("plan-50", "Plan 50")],
        total: 51,
        offset: 50,
      });

    render(
      <PlanView
        apiClient={apiClient}
        wsClient={wsClient}
        projectId="proj-1"
        refreshToken={0}
      />,
    );

    await waitFor(() => {
      expect(apiClient.listPlans).toHaveBeenNthCalledWith(1, "proj-1", {
        limit: 50,
        offset: 0,
      });
      expect(apiClient.listPlans).toHaveBeenNthCalledWith(2, "proj-1", {
        limit: 50,
        offset: 50,
      });
    });

    expect(screen.getByText("Plan 50")).toBeTruthy();
  });

  it("项目切换后会忽略旧请求返回，避免脏回写", async () => {
    const oldProjectDeferred = createDeferred<ListPlansResponse>();
    const apiClient = createMockApiClient();
    const wsClient = createMockWsClient();
    vi.mocked(apiClient.listPlans).mockImplementation((projectId) => {
      if (projectId === "proj-1") {
        return oldProjectDeferred.promise;
      }
      return Promise.resolve({
        items: [buildPlan("plan-2", "Plan Two")],
        total: 1,
        offset: 0,
      });
    });

    const { rerender } = render(
      <PlanView
        apiClient={apiClient}
        wsClient={wsClient}
        projectId="proj-1"
        refreshToken={0}
      />,
    );

    rerender(
      <PlanView
        apiClient={apiClient}
        wsClient={wsClient}
        projectId="proj-2"
        refreshToken={0}
      />,
    );

    oldProjectDeferred.resolve({
      items: [buildPlan("plan-1", "Plan One")],
      total: 1,
      offset: 0,
    });

    await waitFor(() => {
      expect(apiClient.getPlanDag).toHaveBeenCalledWith("proj-2", "plan-2");
    });
    expect(screen.getByText("Plan Two")).toBeTruthy();
    expect(screen.queryByText("Plan One")).toBeNull();
  });

  it("可提交审核并调用 submitPlanReview API", async () => {
    const apiClient = createMockApiClient();
    const wsClient = createMockWsClient();

    render(
      <PlanView
        apiClient={apiClient}
        wsClient={wsClient}
        projectId="proj-1"
        refreshToken={0}
      />,
    );

    await waitFor(() => {
      expect(apiClient.getPlanDag).toHaveBeenCalledWith("proj-1", "plan-1");
    });

    fireEvent.click(screen.getByRole("button", { name: "提交审核" }));

    await waitFor(() => {
      expect(apiClient.submitPlanReview).toHaveBeenCalledWith("proj-1", "plan-1");
    });
  });

  it("可执行通过/驳回/放弃动作并调用 applyPlanAction API", async () => {
    const apiClient = createMockApiClient();
    vi.mocked(apiClient.listPlans).mockResolvedValue({
      items: [
        buildPlan("plan-1", "Plan One", {
          status: "waiting_human",
          wait_reason: "final_approval",
        }),
      ],
      total: 1,
      offset: 0,
    });
    const wsClient = createMockWsClient();

    render(
      <PlanView
        apiClient={apiClient}
        wsClient={wsClient}
        projectId="proj-1"
        refreshToken={0}
      />,
    );

    await waitFor(() => {
      expect(apiClient.getPlanDag).toHaveBeenCalledWith("proj-1", "plan-1");
    });

    fireEvent.click(screen.getByRole("button", { name: "通过" }));

    await waitFor(() => {
      expect(apiClient.applyPlanAction).toHaveBeenCalledWith("proj-1", "plan-1", {
        action: "approve",
      });
    });

    fireEvent.change(screen.getByLabelText("驳回类型"), {
      target: { value: "coverage_gap" },
    });
    fireEvent.change(screen.getByLabelText("驳回说明"), {
      target: { value: "缺少关键回滚路径与异常分支覆盖，请补齐后再提交审核。" },
    });
    fireEvent.change(screen.getByLabelText("期望方向（可选）"), {
      target: { value: "补齐异常流并拆分任务颗粒度" },
    });
    fireEvent.click(screen.getByRole("button", { name: "驳回" }));

    await waitFor(() => {
      expect(apiClient.applyPlanAction).toHaveBeenCalledWith("proj-1", "plan-1", {
        action: "reject",
        feedback: {
          category: "coverage_gap",
          detail: "缺少关键回滚路径与异常分支覆盖，请补齐后再提交审核。",
          expected_direction: "补齐异常流并拆分任务颗粒度",
        },
      });
    });

    fireEvent.click(screen.getByRole("button", { name: "放弃" }));

    await waitFor(() => {
      expect(apiClient.applyPlanAction).toHaveBeenCalledWith("proj-1", "plan-1", {
        action: "abort",
      });
    });
  });

  it("parse_failed 场景展示重试解析并触发 approve 动作", async () => {
    const apiClient = createMockApiClient();
    vi.mocked(apiClient.listPlans).mockResolvedValue({
      items: [
        buildPlan("plan-1", "Plan One", {
          status: "waiting_human",
          wait_reason: "parse_failed",
        }),
      ],
      total: 1,
      offset: 0,
    });
    const wsClient = createMockWsClient();

    render(
      <PlanView
        apiClient={apiClient}
        wsClient={wsClient}
        projectId="proj-1"
        refreshToken={0}
      />,
    );

    await waitFor(() => {
      expect(apiClient.getPlanDag).toHaveBeenCalledWith("proj-1", "plan-1");
    });

    expect(screen.getByText("解析失败（parse_failed），请修正输入后点击“重试解析”继续。")).toBeTruthy();
    const retryParseButton = screen.getByRole("button", { name: "重试解析" });
    expect(retryParseButton).toBeTruthy();
    expect((retryParseButton as HTMLButtonElement).disabled).toBe(false);

    fireEvent.click(retryParseButton);

    await waitFor(() => {
      expect(apiClient.applyPlanAction).toHaveBeenCalledWith("proj-1", "plan-1", {
        action: "approve",
      });
    });
  });

  it("展示 Task 的 GitHub Issue 链接", async () => {
    const apiClient = createMockApiClient();
    vi.mocked(apiClient.listPlans).mockResolvedValue({
      items: [
        buildPlan("plan-1", "Plan One", {
          tasks: [
            buildTask("task-1", {
              github: {
                issue_number: 201,
                issue_url: "https://github.com/acme/ai-workflow/issues/201",
              },
            }),
          ],
        }),
      ],
      total: 1,
      offset: 0,
    });
    const wsClient = createMockWsClient();

    render(
      <PlanView
        apiClient={apiClient}
        wsClient={wsClient}
        projectId="proj-1"
        refreshToken={0}
      />,
    );

    await waitFor(() => {
      expect(screen.getByTestId("plan-github-links")).toBeTruthy();
    });
    expect(screen.getByText("Issue #201")).toBeTruthy();
  });
});

describe("PlanView mini map color fallback", () => {
  it("未知状态时返回兜底色值", () => {
    expect(resolveMiniMapNodeColor(undefined)).toBe("#64748b");
    expect(resolveMiniMapNodeColor("not_supported_status")).toBe("#64748b");
  });
});
