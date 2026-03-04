/** @vitest-environment jsdom */

import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import BoardView, { groupBoardTasks, toBoardStatus, type BoardTask } from "./BoardView";
import type { ApiClient } from "../lib/apiClient";
import type { ApiIssue, IssueTimelineEntry } from "../types/api";

const buildIssue = (overrides?: Partial<ApiIssue>): ApiIssue => {
  return {
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
    state: "open",
    status: "draft",
    run_id: "",
    version: 1,
    superseded_by: "",
    external_id: "",
    fail_policy: "block",
    created_at: "2026-03-01T10:00:00.000Z",
    updated_at: "2026-03-01T10:00:00.000Z",
    ...overrides,
  };
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
    createRun: vi.fn(),
    listChats: vi.fn(),
    listChatRunEvents: vi.fn(),
    createChat: vi.fn(),
    cancelChat: vi.fn(),
    getChat: vi.fn(),
    createPlan: vi.fn(),
    submitIssueReview: vi.fn().mockResolvedValue({ status: "reviewing" }),
    applyIssueAction: vi.fn().mockResolvedValue({ status: "executing" }),
    applyTaskAction: vi.fn(),
    listIssues: vi.fn().mockResolvedValue({
      items: [buildIssue()],
      total: 1,
      offset: 0,
    }),
    getPlanDag: vi.fn(),
    listPlanReviews: vi.fn(),
    listPlanChanges: vi.fn(),
    listIssueTimeline: vi.fn().mockResolvedValue({
      items: [],
      total: 0,
      offset: 0,
    }),
    listAdminAuditLog: vi.fn(),
    getRun: vi.fn(),
    getRunLogs: vi.fn(),
    getRunCheckpoints: vi.fn(),
    getRepoTree: vi.fn(),
    getRepoStatus: vi.fn(),
    getRepoDiff: vi.fn(),
    applyRunAction: vi.fn(),
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
  it("toBoardStatus 兼容 issue/task 状态映射", () => {
    expect(toBoardStatus("queued")).toBe("ready");
    expect(toBoardStatus("executing")).toBe("running");
    expect(toBoardStatus("blocked_by_failure")).toBe("failed");
    expect(toBoardStatus("skipped")).toBe("done");
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

  it("从 plans 主实体渲染 issue 列表（无 tasks 也可展示）", async () => {
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
    expect(screen.getByText("log · implement/stage_start")).toBeTruthy();
    expect(screen.getAllByText(/stage started/).length).toBeGreaterThan(0);
    expect(screen.queryByText("展开完整输出")).toBeNull();
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
});




