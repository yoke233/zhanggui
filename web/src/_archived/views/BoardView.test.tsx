/** @vitest-environment jsdom */

import type { ReactNode } from "react";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import BoardView, { groupBoardTasks, toBoardStatus, type BoardTask } from "./BoardView";
import type { ApiClient } from "../lib/apiClient";
import type { ApiIssue, IssueTimelineEntry } from "../types/api";

vi.mock("@xyflow/react", () => ({
  Background: () => <div data-testid="react-flow-background" />,
  Controls: () => <div data-testid="react-flow-controls" />,
  MarkerType: { ArrowClosed: "arrow-closed" },
  Position: { Left: "left", Right: "right" },
  ReactFlow: ({ children }: { children?: ReactNode }) => (
    <div data-testid="react-flow">{children}</div>
  ),
}));

vi.mock("../components/DagPreview", () => ({
  default: ({
    items,
    summary,
    error,
    loading,
    onConfirm,
    onCancel,
  }: {
    items: Array<{ temp_id: string; title: string }>;
    summary: string;
    error?: string | null;
    loading?: boolean;
    onConfirm: (items: Array<{ temp_id: string; title: string }>) => void;
    onCancel: () => void;
  }) => (
    <div data-testid="dag-preview">
      <div>{summary}</div>
      {error ? <div role="alert">{error}</div> : null}
      <button
        type="button"
        onClick={() => {
          onConfirm(items);
        }}
        disabled={loading}
      >
        confirm dag
      </button>
      <button type="button" onClick={onCancel}>
        cancel dag
      </button>
    </div>
  ),
}));

vi.mock("../components/IssueFlowTree", () => ({
  default: () => <div data-testid="issue-flow-tree" />,
}));

const buildIssue = (overrides?: Partial<ApiIssue>): ApiIssue => {
  const issue: ApiIssue = {
    id: "issue-1",
    project_id: "proj-1",
    session_id: "chat-1",
    title: "Issue One",
    body: "",
    labels: [],
    milestone_id: "",
    attachments: [],
    depends_on: [],
    blocks: [],
    priority: 0,
    template: "standard",
    auto_merge: false,
    children_mode: "",
    state: "open",
    status: "draft",
    run_id: "",
    version: 1,
    superseded_by: "",
    parent_id: "",
    external_id: "",
    submitted_by: "",
    merge_retries: 0,
    triage_instructions: "",
    fail_policy: "block",
    created_at: "2026-03-01T10:00:00.000Z",
    updated_at: "2026-03-01T10:00:00.000Z",
    ...overrides,
  };
  issue.children_mode = overrides?.children_mode ?? issue.children_mode;
  return issue;
};

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
    createProjectCreateRequest: vi.fn(),
    getProjectCreateRequest: vi.fn(),
    listRuns: vi.fn(),
    listChats: vi.fn(),
    listChatRunEvents: vi.fn(),
    createChat: vi.fn(),
    cancelChat: vi.fn(),
    getChat: vi.fn(),
    createIssue: vi.fn(),
    decompose: vi.fn().mockResolvedValue({
      proposal_id: "prop-1",
      project_id: "proj-1",
      prompt: "做一个用户注册系统",
      summary: "拆成两个依赖任务",
      issues: [
        {
          temp_id: "A",
          title: "设计 schema",
          body: "设计用户表",
          labels: ["backend"],
          depends_on: [],
        },
        {
          temp_id: "B",
          title: "实现注册 API",
          body: "实现 POST /register",
          labels: ["backend"],
          depends_on: ["A"],
        },
      ],
    }),
    confirmDecompose: vi.fn().mockResolvedValue({
      created_issues: [
        { temp_id: "A", issue_id: "issue-a" },
        { temp_id: "B", issue_id: "issue-b" },
      ],
    }),
    submitIssueReview: vi.fn().mockResolvedValue({ status: "reviewing" }),
    applyIssueAction: vi.fn().mockResolvedValue({ status: "executing" }),
    applyTaskAction: vi.fn(),
    listIssues: vi.fn().mockResolvedValue({
      items: [buildIssue()],
      total: 1,
      offset: 0,
    }),
    getIssueDag: vi.fn(),
    listIssueReviews: vi.fn(),
    listIssueChanges: vi.fn(),
    listIssueTimeline: vi.fn().mockResolvedValue({
      items: [],
      total: 0,
      offset: 0,
    }),
    listIssueTaskSteps: vi.fn().mockResolvedValue({
      steps: [],
      total: 0,
    }),
    listAdminAuditLog: vi.fn(),
    getRun: vi.fn(),
    getRepoTree: vi.fn(),
    getRepoStatus: vi.fn(),
    getRepoDiff: vi.fn(),
  } as unknown as ApiClient;
};

const createDeferred = <T,>() => {
  let resolve: (value: T | PromiseLike<T>) => void = () => {};
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
};

describe("BoardView helpers", () => {
  it("toBoardStatus 映射当前 issue 状态", () => {
    expect(toBoardStatus("queued")).toBe("ready");
    expect(toBoardStatus("executing")).toBe("running");
    expect(toBoardStatus("merging")).toBe("running");
    expect(toBoardStatus("superseded")).toBe("failed");
  });

  it("groupBoardTasks 返回完整五列", () => {
    const tasks: BoardTask[] = [
      {
        id: "i-1",
        title: "Issue A",
        status: "running",
        raw_status: "executing",
        run_id: "pipe-1",
      },
    ];
    const grouped = groupBoardTasks(tasks);
    expect(Object.keys(grouped)).toEqual([
      "pending",
      "ready",
      "running",
      "done",
      "failed",
    ]);
    expect(grouped.running).toHaveLength(1);
  });
});

describe("BoardView", () => {
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it("从 issues 主实体渲染列表（无 tasks 也可展示）", async () => {
    const apiClient = createMockApiClient();
    vi.mocked(apiClient.listIssues).mockResolvedValue({
      items: [
        buildIssue({
          id: "issue-1",
          title: "Issue One",
          status: "executing",
          run_id: "pipe-1",
        }),
      ],
      total: 1,
      offset: 0,
    });

    render(<BoardView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    await waitFor(() => {
      expect(apiClient.listIssues).toHaveBeenCalledWith("proj-1", {
        limit: 50,
        offset: 0,
      });
    });

    expect(screen.getAllByText("Issue One").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Running").length).toBeGreaterThan(0);
  });

  it("看板会循环拉取所有分页计划数据", async () => {
    const apiClient = createMockApiClient();
    vi.mocked(apiClient.listIssues)
      .mockResolvedValueOnce({
        items: Array.from({ length: 50 }, (_, index) =>
          buildIssue({
            id: `issue-${index}`,
            title: `Issue ${index}`,
            status: "draft",
          }),
        ),
        total: 51,
        offset: 0,
      })
      .mockResolvedValueOnce({
        items: [
          buildIssue({
            id: "issue-last",
            title: "Issue Last",
            status: "reviewing",
          }),
        ],
        total: 51,
        offset: 50,
      });

    render(<BoardView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    await waitFor(() => {
      expect(apiClient.listIssues).toHaveBeenNthCalledWith(1, "proj-1", {
        limit: 50,
        offset: 0,
      });
      expect(apiClient.listIssues).toHaveBeenNthCalledWith(2, "proj-1", {
        limit: 50,
        offset: 50,
      });
    });

    expect(screen.getAllByText("Issue Last").length).toBeGreaterThan(0);
  });

  it("项目切换后会忽略旧请求返回，避免脏回写", async () => {
    const staleDeferred = createDeferred<{
      items: ApiIssue[];
      total: number;
      offset: number;
    }>();
    const apiClient = createMockApiClient();
    vi.mocked(apiClient.listIssues).mockImplementation((projectId) => {
      if (projectId === "proj-1") {
        return staleDeferred.promise;
      }
      return Promise.resolve({
        items: [
          buildIssue({
            id: "issue-fresh",
            title: "Issue Fresh",
            status: "reviewing",
          }),
        ],
        total: 1,
        offset: 0,
      });
    });

    const { rerender } = render(<BoardView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);
    rerender(<BoardView apiClient={apiClient} projectId="proj-2" refreshToken={0} />);

    staleDeferred.resolve({
      items: [
        buildIssue({
          id: "issue-stale",
          title: "Issue Stale",
          status: "draft",
        }),
      ],
      total: 1,
      offset: 0,
    });

    await waitFor(() => {
      expect(apiClient.listIssues).toHaveBeenCalledWith("proj-2", {
        limit: 50,
        offset: 0,
      });
    });

    expect(screen.getAllByText("Issue Fresh").length).toBeGreaterThan(0);
    expect(screen.queryAllByText("Issue Stale")).toHaveLength(0);
  });

  it("refreshToken 变化后会立即触发一次刷新", async () => {
    const apiClient = createMockApiClient();
    const { rerender } = render(<BoardView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    await waitFor(() => {
      expect(apiClient.listIssues).toHaveBeenCalledTimes(1);
    });

    rerender(<BoardView apiClient={apiClient} projectId="proj-1" refreshToken={1} />);
    await waitFor(() => {
      expect(apiClient.listIssues).toHaveBeenCalledTimes(2);
    });
  });

  it("定时刷新期间保持已渲染 issue 列表，避免闪屏", async () => {
    const deferred = createDeferred<{
      items: ApiIssue[];
      total: number;
      offset: number;
    }>();
    const apiClient = createMockApiClient();
    let callCount = 0;
    vi.mocked(apiClient.listIssues).mockImplementation(async () => {
      callCount += 1;
      if (callCount === 1) {
        return {
          items: [
            buildIssue({
              id: "issue-stable",
              title: "Issue Stable",
              status: "executing",
            }),
          ],
          total: 1,
          offset: 0,
        };
      }
      return deferred.promise;
    });

    render(<BoardView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    await waitFor(() => {
      expect(screen.getAllByText("Issue Stable").length).toBeGreaterThan(0);
    });

    vi.useFakeTimers();
    fireEvent.click(screen.getByLabelText("自动静默刷新"));
    await Promise.resolve();

    await vi.advanceTimersByTimeAsync(10_000);
    expect(apiClient.listIssues).toHaveBeenCalledTimes(2);

    expect(screen.getAllByText("Issue Stable").length).toBeGreaterThan(0);
    expect(screen.queryByText("加载中...")).toBeNull();

    deferred.resolve({
      items: [
        buildIssue({
          id: "issue-stable",
          title: "Issue Stable",
          status: "executing",
        }),
      ],
      total: 1,
      offset: 0,
    });
    await Promise.resolve();
  });

  it("详情区 Approve 按钮调用 issue action API", async () => {
    const apiClient = createMockApiClient();
    vi.mocked(apiClient.listIssues).mockResolvedValue({
      items: [
        buildIssue({
          id: "issue-approve",
          title: "Issue Approve",
          status: "reviewing",
        }),
      ],
      total: 1,
      offset: 0,
    });

    render(<BoardView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    await waitFor(() => {
      expect(screen.getAllByText("Issue Approve").length).toBeGreaterThan(0);
    });

    const approveButton = screen
      .getAllByTestId("board-task")
      .find((item) => within(item).queryByText("Issue Approve"));
    if (!approveButton) {
      throw new Error("Issue Approve card not found");
    }
    fireEvent.click(approveButton);
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));

    await waitFor(() => {
      expect(apiClient.applyIssueAction).toHaveBeenCalledWith("proj-1", "issue-approve", {
        action: "approve",
      });
    });
  });

  it("详情区 Submit review 按钮调用 submitIssueReview", async () => {
    const apiClient = createMockApiClient();
    vi.mocked(apiClient.listIssues).mockResolvedValue({
      items: [
        buildIssue({
          id: "issue-submit",
          title: "Issue Submit",
          status: "draft",
        }),
      ],
      total: 1,
      offset: 0,
    });

    render(<BoardView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    await waitFor(() => {
      expect(screen.getAllByText("Issue Submit").length).toBeGreaterThan(0);
    });

    const submitButton = screen
      .getAllByTestId("board-task")
      .find((item) => within(item).queryByText("Issue Submit"));
    if (!submitButton) {
      throw new Error("Issue Submit card not found");
    }
    fireEvent.click(submitButton);
    fireEvent.click(screen.getByRole("button", { name: "Submit review" }));

    await waitFor(() => {
      expect(apiClient.submitIssueReview).toHaveBeenCalledWith("proj-1", "issue-submit");
    });
  });

  it("QuickInput 拆解后可确认创建并刷新列表", async () => {
    const apiClient = createMockApiClient();
    vi.mocked(apiClient.decompose).mockResolvedValue({
      proposal_id: "prop-1",
      project_id: "proj-1",
      prompt: "做一个注册功能",
      summary: "先做 schema 再做 API",
      issues: [
        {
          temp_id: "A",
          title: "设计 schema",
          body: "设计用户表",
          labels: ["backend"],
          depends_on: [],
        },
        {
          temp_id: "B",
          title: "实现 API",
          body: "实现注册接口",
          labels: ["backend"],
          depends_on: ["A"],
        },
      ],
    });
    vi.mocked(apiClient.confirmDecompose).mockResolvedValue({
      created_issues: [
        { temp_id: "A", issue_id: "issue-a" },
        { temp_id: "B", issue_id: "issue-b" },
      ],
    });

    render(<BoardView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    await waitFor(() => {
      expect(apiClient.listIssues).toHaveBeenCalledTimes(1);
    });

    fireEvent.change(
      screen.getByPlaceholderText("描述你的需求，AI 将自动拆解为任务..."),
      { target: { value: "做一个注册功能" } },
    );
    fireEvent.click(screen.getByRole("button", { name: "DAG 拆解" }));

    await waitFor(() => {
      expect(apiClient.decompose).toHaveBeenCalledWith("proj-1", {
        prompt: "做一个注册功能",
      });
    });
    expect(screen.getByTestId("dag-preview")).toBeTruthy();
    expect(screen.getByText("先做 schema 再做 API")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "confirm dag" }));

    await waitFor(() => {
      expect(apiClient.confirmDecompose).toHaveBeenCalledWith("proj-1", {
        proposal_id: "prop-1",
        issues: [
          {
            temp_id: "A",
            title: "设计 schema",
            body: "设计用户表",
            labels: ["backend"],
            depends_on: [],
          },
          {
            temp_id: "B",
            title: "实现 API",
            body: "实现注册接口",
            labels: ["backend"],
            depends_on: ["A"],
          },
        ],
      });
    });

    await waitFor(() => {
      expect(apiClient.listIssues).toHaveBeenCalledTimes(2);
    });
    expect(screen.queryByTestId("dag-preview")).toBeNull();
  });

  it("projectId 切换后会清理 DAG 状态并忽略旧 proposal", async () => {
    const pendingDecompose = createDeferred<{
      proposal_id: string;
      project_id: string;
      prompt: string;
      summary: string;
      issues: Array<{
        temp_id: string;
        title: string;
        body: string;
        labels: string[];
        depends_on: string[];
      }>;
    }>();
    const apiClient = createMockApiClient();
    vi.mocked(apiClient.decompose).mockImplementation((projectId, body) => {
      if (projectId === "proj-1") {
        return pendingDecompose.promise;
      }
      return Promise.resolve({
        proposal_id: "prop-2",
        project_id: projectId,
        prompt: body.prompt,
        summary: "新项目提案",
        issues: [
          {
            temp_id: "C",
            title: "新任务",
            body: "new",
            labels: ["frontend"],
            depends_on: [],
          },
        ],
      });
    });

    const view = render(<BoardView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    fireEvent.change(
      screen.getByPlaceholderText("描述你的需求，AI 将自动拆解为任务..."),
      { target: { value: "旧项目需求" } },
    );
    fireEvent.click(screen.getByRole("button", { name: "DAG 拆解" }));

    await waitFor(() => {
      expect(apiClient.decompose).toHaveBeenCalledWith("proj-1", {
        prompt: "旧项目需求",
      });
    });

    view.rerender(<BoardView apiClient={apiClient} projectId="proj-2" refreshToken={0} />);

    pendingDecompose.resolve({
      proposal_id: "prop-stale",
      project_id: "proj-1",
      prompt: "旧项目需求",
      summary: "旧 proposal",
      issues: [
        {
          temp_id: "A",
          title: "旧任务",
          body: "",
          labels: [],
          depends_on: [],
        },
      ],
    });

    await waitFor(() => {
      expect(screen.queryByTestId("dag-preview")).toBeNull();
    });
    expect(screen.queryByText("旧 proposal")).toBeNull();

    fireEvent.change(
      screen.getByPlaceholderText("描述你的需求，AI 将自动拆解为任务..."),
      { target: { value: "新项目需求" } },
    );
    fireEvent.click(screen.getByRole("button", { name: "DAG 拆解" }));

    await waitFor(() => {
      expect(apiClient.decompose).toHaveBeenCalledWith("proj-2", {
        prompt: "新项目需求",
      });
    });
    expect(screen.getByTestId("dag-preview")).toBeTruthy();
    expect(screen.getByText("新项目提案")).toBeTruthy();
    expect(screen.queryByText("旧 proposal")).toBeNull();
  });

  it("confirm handler 会同步防重入，快双击只发一次请求", async () => {
    const confirmDeferred = createDeferred<{
      created_issues: Array<{ temp_id: string; issue_id: string }>;
    }>();
    const apiClient = createMockApiClient();
    vi.mocked(apiClient.confirmDecompose).mockImplementation(() => confirmDeferred.promise);

    render(<BoardView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    fireEvent.change(
      screen.getByPlaceholderText("描述你的需求，AI 将自动拆解为任务..."),
      { target: { value: "做一个用户注册系统" } },
    );
    fireEvent.click(screen.getByRole("button", { name: "DAG 拆解" }));

    await waitFor(() => {
      expect(screen.getByTestId("dag-preview")).toBeTruthy();
    });

    const confirmButton = screen.getByRole("button", { name: "confirm dag" });
    fireEvent.click(confirmButton);
    fireEvent.click(confirmButton);

    await waitFor(() => {
      expect(apiClient.confirmDecompose).toHaveBeenCalledTimes(1);
    });

    confirmDeferred.resolve({
      created_issues: [{ temp_id: "A", issue_id: "issue-a" }],
    });
  });

  it("confirm 失败时错误会显示在 DagPreview 模态内", async () => {
    const apiClient = createMockApiClient();
    vi.mocked(apiClient.confirmDecompose).mockRejectedValue(new Error("confirm exploded"));

    render(<BoardView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    fireEvent.change(
      screen.getByPlaceholderText("描述你的需求，AI 将自动拆解为任务..."),
      { target: { value: "做一个用户注册系统" } },
    );
    fireEvent.click(screen.getByRole("button", { name: "DAG 拆解" }));

    await waitFor(() => {
      expect(screen.getByTestId("dag-preview")).toBeTruthy();
    });

    fireEvent.click(screen.getByRole("button", { name: "confirm dag" }));

    await waitFor(() => {
      const preview = screen.getByTestId("dag-preview");
      expect(within(preview).getByRole("alert").textContent).toContain("confirm exploded");
    });
  });

  it("projectId 切换会重置 confirm 锁并允许新项目继续确认", async () => {
    const pendingConfirm = createDeferred<{
      created_issues: Array<{ temp_id: string; issue_id: string }>;
    }>();
    const apiClient = createMockApiClient();
    vi.mocked(apiClient.confirmDecompose).mockImplementation((projectId) => {
      if (projectId === "proj-1") {
        return pendingConfirm.promise;
      }
      return Promise.resolve({
        created_issues: [{ temp_id: "A", issue_id: "issue-next" }],
      });
    });
    vi.mocked(apiClient.decompose).mockImplementation((projectId) =>
      Promise.resolve({
        proposal_id: projectId === "proj-1" ? "prop-1" : "prop-2",
        project_id: projectId,
        prompt: "prompt",
        summary: projectId === "proj-1" ? "旧项目提案" : "新项目提案",
        issues: [
          {
            temp_id: "A",
            title: projectId === "proj-1" ? "旧任务" : "新任务",
            body: "",
            labels: [],
            depends_on: [],
          },
        ],
      }),
    );

    const view = render(<BoardView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    fireEvent.change(
      screen.getByPlaceholderText("描述你的需求，AI 将自动拆解为任务..."),
      { target: { value: "旧项目需求" } },
    );
    fireEvent.click(screen.getByRole("button", { name: "DAG 拆解" }));

    await waitFor(() => {
      expect(screen.getByText("旧项目提案")).toBeTruthy();
    });

    fireEvent.click(screen.getByRole("button", { name: "confirm dag" }));

    await waitFor(() => {
      expect(apiClient.confirmDecompose).toHaveBeenCalledWith("proj-1", {
        proposal_id: "prop-1",
        issues: [
          {
            temp_id: "A",
            title: "旧任务",
            body: "",
            labels: [],
            depends_on: [],
          },
        ],
      });
    });

    view.rerender(<BoardView apiClient={apiClient} projectId="proj-2" refreshToken={0} />);

    await waitFor(() => {
      expect(screen.queryByTestId("dag-preview")).toBeNull();
    });

    fireEvent.change(
      screen.getByPlaceholderText("描述你的需求，AI 将自动拆解为任务..."),
      { target: { value: "新项目需求" } },
    );
    fireEvent.click(screen.getByRole("button", { name: "DAG 拆解" }));

    await waitFor(() => {
      expect(screen.getByText("新项目提案")).toBeTruthy();
    });

    fireEvent.click(screen.getByRole("button", { name: "confirm dag" }));

    await waitFor(() => {
      expect(apiClient.confirmDecompose).toHaveBeenCalledWith("proj-2", {
        proposal_id: "prop-2",
        issues: [
          {
            temp_id: "A",
            title: "新任务",
            body: "",
            labels: [],
            depends_on: [],
          },
        ],
      });
    });
  });

  it("点击 issue 会调用 timeline API 并渲染事件", async () => {
    const apiClient = createMockApiClient();
    vi.mocked(apiClient.listIssues).mockResolvedValue({
      items: [
        buildIssue({
          id: "issue-timeline",
          title: "Issue Timeline",
          status: "executing",
          run_id: "pipe-99",
        }),
      ],
      total: 1,
      offset: 0,
    });
    vi.mocked(apiClient.listIssueTimeline).mockResolvedValue({
      items: [
        {
          event_id: "log:1",
          kind: "log",
          created_at: "2026-03-03T10:00:00Z",
          actor_type: "agent",
          actor_name: "codex",
          actor_avatar_seed: "codex",
          title: "log · implement/stage_start",
          body: "stage started",
          status: "running",
          refs: {
            issue_id: "issue-timeline",
            run_id: "pipe-99",
            stage: "implement",
          },
          meta: { type: "stage_start" },
        } as IssueTimelineEntry,
      ],
      total: 1,
      offset: 0,
    });

    render(<BoardView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    await waitFor(() => {
      expect(screen.getAllByText("Issue Timeline").length).toBeGreaterThan(0);
    });
    expect(apiClient.listIssueTimeline).not.toHaveBeenCalled();

    const timelineButton = screen
      .getAllByTestId("board-task")
      .find((item) => within(item).queryByText("Issue Timeline"));
    if (!timelineButton) {
      throw new Error("Issue Timeline card not found");
    }
    fireEvent.click(timelineButton);

    await waitFor(() => {
      expect(apiClient.listIssueTimeline).toHaveBeenCalledWith("proj-1", "issue-timeline", {
        limit: 200,
        offset: 0,
      });
    });
    expect(apiClient.listIssueTaskSteps).not.toHaveBeenCalled();
    expect(screen.getByText("log · implement/stage_start")).toBeTruthy();
    expect(screen.getAllByText(/stage started/).length).toBeGreaterThan(0);
    expect(screen.queryByText("展开完整输出")).toBeNull();
  });

  it("详情已打开时列表刷新不会重复拉取同一 issue timeline", async () => {
    const apiClient = createMockApiClient();
    vi.mocked(apiClient.listIssues).mockResolvedValue({
      items: [
        buildIssue({
          id: "issue-refresh-stable",
          title: "Issue Refresh Stable",
          status: "executing",
          run_id: "pipe-77",
        }),
      ],
      total: 1,
      offset: 0,
    });
    vi.mocked(apiClient.listIssueTimeline).mockResolvedValue({
      items: [
        {
          event_id: "log:refresh-stable",
          kind: "log",
          created_at: "2026-03-03T10:00:00Z",
          actor_type: "agent",
          actor_name: "codex",
          actor_avatar_seed: "codex",
          title: "log · implement/stage_start",
          body: "stage started",
          status: "running",
          refs: {
            issue_id: "issue-refresh-stable",
            run_id: "pipe-77",
            stage: "implement",
          },
          meta: { type: "stage_start" },
        } as IssueTimelineEntry,
      ],
      total: 1,
      offset: 0,
    });

    const view = render(<BoardView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    await waitFor(() => {
      expect(screen.getAllByText("Issue Refresh Stable").length).toBeGreaterThan(0);
    });
    expect(apiClient.listIssueTimeline).not.toHaveBeenCalled();

    const detailButton = screen
      .getAllByTestId("board-task")
      .find((item) => within(item).queryByText("Issue Refresh Stable"));
    if (!detailButton) {
      throw new Error("Issue Refresh Stable card not found");
    }
    fireEvent.click(detailButton);

    await waitFor(() => {
      expect(apiClient.listIssueTimeline).toHaveBeenCalledTimes(1);
    });

    view.rerender(<BoardView apiClient={apiClient} projectId="proj-1" refreshToken={1} />);

    await waitFor(() => {
      expect(apiClient.listIssues).toHaveBeenCalledTimes(2);
    });
    expect(apiClient.listIssueTimeline).toHaveBeenCalledTimes(1);
  });

  it("timeline 会折叠重复事件并生成可读摘要", async () => {
    const apiClient = createMockApiClient();
    vi.mocked(apiClient.listIssues).mockResolvedValue({
      items: [
        buildIssue({
          id: "issue-dedup",
          title: "Issue Dedup",
          status: "executing",
          run_id: "pipe-88",
        }),
      ],
      total: 1,
      offset: 0,
    });
    vi.mocked(apiClient.listIssueTimeline).mockResolvedValue({
      items: [
        {
          event_id: "checkpoint:1",
          kind: "checkpoint",
          created_at: "2026-03-03T10:04:00Z",
          actor_type: "system",
          actor_name: "system",
          actor_avatar_seed: "system",
          title: "checkpoint · implement",
          body: "",
          status: "failed",
          refs: {
            issue_id: "issue-dedup",
            run_id: "pipe-88",
            stage: "implement",
          },
          meta: { error: "worktree path is empty" },
        } as IssueTimelineEntry,
        {
          event_id: "checkpoint:2",
          kind: "checkpoint",
          created_at: "2026-03-03T10:05:00Z",
          actor_type: "system",
          actor_name: "system",
          actor_avatar_seed: "system",
          title: "checkpoint · implement",
          body: "",
          status: "failed",
          refs: {
            issue_id: "issue-dedup",
            run_id: "pipe-88",
            stage: "implement",
          },
          meta: { error: "worktree path is empty" },
        } as IssueTimelineEntry,
      ],
      total: 2,
      offset: 0,
    });

    render(<BoardView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    await waitFor(() => {
      expect(screen.getAllByText("Issue Dedup").length).toBeGreaterThan(0);
    });

    const detailButton = screen
      .getAllByTestId("board-task")
      .find((item) => within(item).queryByText("Issue Dedup"));
    if (!detailButton) {
      throw new Error("Issue Dedup card not found");
    }
    fireEvent.click(detailButton);

    await waitFor(() => {
      expect(apiClient.listIssueTimeline).toHaveBeenCalledWith("proj-1", "issue-dedup", {
        limit: 200,
        offset: 0,
      });
    });

    expect(screen.getAllByText("checkpoint · implement")).toHaveLength(1);
    expect(screen.getAllByText(/worktree path is empty/).length).toBeGreaterThan(0);
    expect(screen.queryByText(/引用标记：/)).toBeNull();
    expect(screen.queryByText(/^无详细输出$/)).toBeNull();
  });

  it("刷新控制区默认关闭自动刷新并支持切换间隔", async () => {
    const apiClient = createMockApiClient();
    vi.mocked(apiClient.listIssues).mockResolvedValue({
      items: [
        buildIssue({
          id: "issue-refresh-controls",
          title: "Issue Refresh Controls",
          status: "draft",
        }),
      ],
      total: 1,
      offset: 0,
    });

    render(<BoardView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    await waitFor(() => {
      expect(screen.getAllByText("Issue Refresh Controls").length).toBeGreaterThan(0);
    });

    const toggle = screen.getByLabelText("自动静默刷新") as HTMLInputElement;
    const interval = screen.getByLabelText("刷新间隔") as HTMLSelectElement;

    expect(toggle.checked).toBe(false);
    expect(interval.disabled).toBe(true);

    fireEvent.click(toggle);
    expect(toggle.checked).toBe(true);
    expect(interval.disabled).toBe(false);

    fireEvent.change(interval, { target: { value: "30000" } });
    expect(interval.value).toBe("30000");
  });

  it("支持从 QuickInput 触发 DAG 拆解并确认创建", async () => {
    const apiClient = createMockApiClient();

    render(<BoardView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    fireEvent.change(
      screen.getByPlaceholderText("描述你的需求，AI 将自动拆解为任务..."),
      { target: { value: "做一个用户注册系统" } },
    );
    fireEvent.click(screen.getByRole("button", { name: "DAG 拆解" }));

    await waitFor(() => {
      expect(apiClient.decompose).toHaveBeenCalledWith("proj-1", {
        prompt: "做一个用户注册系统",
      });
    });

    expect(screen.getByTestId("dag-preview")).toBeTruthy();
    expect(screen.getByText("拆成两个依赖任务")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "confirm dag" }));

    await waitFor(() => {
      expect(apiClient.confirmDecompose).toHaveBeenCalledWith("proj-1", {
        proposal_id: "prop-1",
        issues: [
          {
            temp_id: "A",
            title: "设计 schema",
            body: "设计用户表",
            labels: ["backend"],
            depends_on: [],
          },
          {
            temp_id: "B",
            title: "实现注册 API",
            body: "实现 POST /register",
            labels: ["backend"],
            depends_on: ["A"],
          },
        ],
      });
    });
  });
});

describe("DagPreview", () => {
  afterEach(() => {
    cleanup();
  });

  it("confirm 按钮本地防重入，快双击只触发一次 onConfirm", async () => {
    const { default: RealDagPreview } = await vi.importActual<typeof import("../components/DagPreview")>(
      "../components/DagPreview",
    );
    const deferred = createDeferred<void>();
    const onConfirm = vi.fn(() => deferred.promise);

    render(
      <RealDagPreview
        items={[
          {
            temp_id: "A",
            title: "设计 schema",
            body: "",
            labels: [],
            depends_on: [],
          },
        ]}
        summary="拆解摘要"
        onConfirm={onConfirm}
        onCancel={() => undefined}
      />,
    );

    const confirmButton = screen.getByRole("button", { name: "创建 1 个 Issue" });
    fireEvent.click(confirmButton);
    fireEvent.click(confirmButton);

    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: "创建中..." }).hasAttribute("disabled")).toBe(true);

    deferred.resolve();
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "创建 1 个 Issue" }).hasAttribute("disabled"),
      ).toBe(false);
    });
  });

  it("执行模式切换为顺序时，confirm 会附带 children_mode=sequential", async () => {
    const { default: RealDagPreview } = await vi.importActual<typeof import("../components/DagPreview")>(
      "../components/DagPreview",
    );
    const onConfirm = vi.fn().mockResolvedValue(undefined);

    render(
      <RealDagPreview
        items={[
          {
            temp_id: "A",
            title: "设计 schema",
            body: "",
            labels: [],
            depends_on: [],
          },
        ]}
        summary="拆解摘要"
        onConfirm={onConfirm}
        onCancel={() => undefined}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "顺序" }));
    fireEvent.click(screen.getByRole("button", { name: "创建 1 个 Issue" }));

    await waitFor(() => {
      expect(onConfirm).toHaveBeenCalledWith([
        {
          temp_id: "A",
          title: "设计 schema",
          body: "",
          labels: [],
          depends_on: [],
          children_mode: "sequential",
        },
      ]);
    });
  });
});
