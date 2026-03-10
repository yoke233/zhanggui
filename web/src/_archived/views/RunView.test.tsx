/** @vitest-environment jsdom */

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import RunView from "./RunView";
import type { ApiClient } from "../lib/apiClient";
import type { ApiRun } from "../types/api";

const buildRun = (id: string): ApiRun => ({
  id,
  project_id: "proj-1",
  issue_id: "issue-1",
  profile: "normal",
  status: "in_progress",
  conclusion: "",
  created_at: "2026-03-01T10:00:00.000Z",
  updated_at: "2026-03-01T10:10:00.000Z",
  github: {
    connection_status: "disconnected",
  },
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
    createProjectCreateRequest: vi.fn(),
    getProjectCreateRequest: vi.fn(),
    listIssues: vi.fn(),
    getIssue: vi.fn(),
    listWorkflowProfiles: vi.fn(),
    getWorkflowProfile: vi.fn(),
    listRuns: vi.fn().mockResolvedValue({
      items: [buildRun("run-1")],
      total: 1,
      offset: 0,
    }),
    getRun: vi.fn().mockResolvedValue(buildRun("run-1")),
    createIssue: vi.fn(),
    createIssueFromFiles: vi.fn(),
    submitIssueReview: vi.fn(),
    applyIssueAction: vi.fn(),
    getIssueDag: vi.fn(),
    listIssueReviews: vi.fn(),
    listIssueChanges: vi.fn(),
    listChats: vi.fn(),
    listChatRunEvents: vi.fn(),
    createChat: vi.fn(),
    cancelChat: vi.fn(),
    getChat: vi.fn(),
    setIssueAutoMerge: vi.fn(),
    applyTaskAction: vi.fn(),
    listIssueTimeline: vi.fn(),
    listAdminAuditLog: vi.fn(),
    getRepoTree: vi.fn(),
    getRepoStatus: vi.fn(),
    getRepoDiff: vi.fn(),
    listRunEvents: vi.fn().mockResolvedValue({
      items: [
        {
          id: 1,
          run_id: "run-1",
          project_id: "proj-1",
          event_type: "run_started",
          stage: "implement",
          agent: "codex",
          created_at: "2026-03-01T10:11:00.000Z",
        },
      ],
      total: 1,
    }),
  } as unknown as ApiClient;
};

const createDeferred = <T,>() => {
  let resolve: (value: T | PromiseLike<T>) => void = () => {};
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
};

describe("RunView", () => {
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it("调用 runs API 并渲染列表", async () => {
    const apiClient = createMockApiClient();
    render(<RunView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    await waitFor(() => {
      expect(apiClient.listRuns).toHaveBeenCalledWith("proj-1", {
        limit: 50,
        offset: 0,
      });
    });

    expect(screen.getByText("run-1")).toBeTruthy();
    expect(screen.getByText("in_progress")).toBeTruthy();
    expect(screen.getAllByTestId("run-row")).toHaveLength(1);
  });

  it("会加载并显示 run events", async () => {
    const apiClient = createMockApiClient();
    render(<RunView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    await waitFor(() => {
      expect(apiClient.listRunEvents).toHaveBeenCalledWith("run-1");
    });

    expect(screen.getByText("Run Started")).toBeTruthy();
    expect(screen.getByText(/stage: implement/)).toBeTruthy();
  });

  it("会循环拉取分页数据直到拉全量", async () => {
    const apiClient = createMockApiClient();
    vi.mocked(apiClient.listRuns)
      .mockResolvedValueOnce({
        items: Array.from({ length: 50 }, (_, index) => buildRun(`run-${index}`)),
        total: 51,
        offset: 0,
      })
      .mockResolvedValueOnce({
        items: [buildRun("run-50")],
        total: 51,
        offset: 50,
      });

    render(<RunView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    await waitFor(() => {
      expect(apiClient.listRuns).toHaveBeenNthCalledWith(1, "proj-1", {
        limit: 50,
        offset: 0,
      });
      expect(apiClient.listRuns).toHaveBeenNthCalledWith(2, "proj-1", {
        limit: 50,
        offset: 50,
      });
    });

    expect(screen.getAllByTestId("run-row")).toHaveLength(51);
  });

  it("项目切换后会忽略旧请求返回，避免脏回写", async () => {
    const staleDeferred = createDeferred<{
      items: ApiRun[];
      total: number;
      offset: number;
    }>();
    const apiClient = createMockApiClient();
    vi.mocked(apiClient.listRuns).mockImplementation((projectId) => {
      if (projectId === "proj-1") {
        return staleDeferred.promise;
      }
      return Promise.resolve({
        items: [buildRun("run-fresh")],
        total: 1,
        offset: 0,
      });
    });

    const { rerender } = render(<RunView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    rerender(<RunView apiClient={apiClient} projectId="proj-2" refreshToken={0} />);
    staleDeferred.resolve({
      items: [buildRun("run-stale")],
      total: 1,
      offset: 0,
    });

    await waitFor(() => {
      expect(apiClient.listRuns).toHaveBeenCalledWith("proj-2", {
        limit: 50,
        offset: 0,
      });
    });

    expect(screen.getByText("run-fresh")).toBeTruthy();
    expect(screen.queryByText("run-stale")).toBeNull();
  });

  it("refreshToken 变化后会立即触发一次刷新", async () => {
    const apiClient = createMockApiClient();
    const { rerender } = render(<RunView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    await waitFor(() => {
      expect(apiClient.listRuns).toHaveBeenCalledTimes(1);
    });

    rerender(<RunView apiClient={apiClient} projectId="proj-1" refreshToken={1} />);

    await waitFor(() => {
      expect(apiClient.listRuns).toHaveBeenCalledTimes(2);
    });
    expect(apiClient.listRuns).toHaveBeenNthCalledWith(2, "proj-1", {
      limit: 50,
      offset: 0,
    });
  });

  it("会通过定时拉取做刷新兜底", async () => {
    vi.useFakeTimers();
    const apiClient = createMockApiClient();
    render(<RunView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    expect(apiClient.listRuns).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(10_000);

    expect(apiClient.listRuns).toHaveBeenCalledTimes(2);
  });

  it("显示 GitHub issue/pr 链接与状态徽标", async () => {
    const apiClient = createMockApiClient();
    vi.mocked(apiClient.listRuns).mockResolvedValue({
      items: [
        {
          ...buildRun("run-github"),
          github: {
            connection_status: "connected",
            issue_number: 201,
            issue_url: "https://github.com/acme/ai-workflow/issues/201",
            pr_number: 301,
            pr_url: "https://github.com/acme/ai-workflow/pull/301",
          },
        },
      ],
      total: 1,
      offset: 0,
    });

    render(<RunView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    await waitFor(() => {
      expect(screen.getByText("run-github")).toBeTruthy();
    });

    expect(screen.getByText("Issue #201")).toBeTruthy();
    expect(screen.getByText("PR #301")).toBeTruthy();
    expect(screen.getByTestId("github-status-badge").textContent).toContain("Connected");
  });

  it("移除旧版人工动作入口", async () => {
    const apiClient = createMockApiClient();
    render(<RunView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    await waitFor(() => {
      expect(screen.getByText("run-1")).toBeTruthy();
    });

    expect(screen.queryByRole("button", { name: "Approve" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Abort" })).toBeNull();
  });
});
