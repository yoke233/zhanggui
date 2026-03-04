/** @vitest-environment jsdom */

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import RunView from "./RunView";
import type { ApiClient } from "../lib/apiClient";
import type { ApiRun } from "../types/api";

const buildRun = (id: string): ApiRun => ({
  id,
  project_id: "proj-1",  name: `Run ${id}`,
  description: "",
  template: "standard",
  status: "running",
  current_stage: "implement",
  artifacts: {},
  config: {},
  branch_name: "",
  worktree_path: "",
  max_total_retries: 5,
  total_retries: 0,
  started_at: "2026-03-01T10:00:00.000Z",
  finished_at: "",
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
    listRuns: vi.fn().mockResolvedValue({
      items: [buildRun("pipe-1")],
      total: 1,
      offset: 0,
    }),
    createRun: vi.fn(),
    createChat: vi.fn(),
    getChat: vi.fn(),
    createPlan: vi.fn(),
    submitPlanReview: vi.fn(),
    applyPlanAction: vi.fn(),
    applyTaskAction: vi.fn(),
    getRun: vi.fn().mockResolvedValue(buildRun("pipe-1")),
    getRunCheckpoints: vi.fn().mockResolvedValue([
      {
        run_id: "pipe-1",
        stage_name: "requirements",
        status: "success",
        artifacts: { summary: "ok" },
        started_at: "2026-03-01T10:00:00.000Z",
        finished_at: "2026-03-01T10:01:00.000Z",
        agent_used: "claude",
        tokens_used: 12,
        retry_count: 0,
        error: "",
      },
    ]),
    applyRunAction: vi.fn().mockResolvedValue({
      status: "failed",
      current_stage: "requirements",
    }),
    listPlans: vi.fn(),
    getPlanDag: vi.fn(),
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

  it("调用 Runs API 并渲染最小列表", async () => {
    const apiClient = createMockApiClient();
    render(<RunView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    await waitFor(() => {
      expect(apiClient.listRuns).toHaveBeenCalledWith("proj-1", {
        limit: 50,
        offset: 0,
      });
    });

    expect(screen.getByText("Run pipe-1")).toBeTruthy();
    expect(screen.getByText("running")).toBeTruthy();
    expect(screen.getAllByTestId("Run-row")).toHaveLength(1);
  });

  it("会加载并显示 checkpoint 区，且人工动作按钮可调用 API", async () => {
    const apiClient = createMockApiClient();
    render(<RunView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    await waitFor(() => {
      expect(apiClient.getRunCheckpoints).toHaveBeenCalledWith("proj-1", "pipe-1");
    });
    expect(screen.getByText("requirements")).toBeTruthy();
    expect(screen.getByText("success")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Abort" }));
    await waitFor(() => {
      expect(apiClient.applyRunAction).toHaveBeenCalledWith("proj-1", "pipe-1", {
        action: "abort",
      });
    });
  });

  it("会循环拉取分页数据直到拉全量", async () => {
    const apiClient = createMockApiClient();
    vi.mocked(apiClient.listRuns)
      .mockResolvedValueOnce({
        items: Array.from({ length: 50 }, (_, index) => buildRun(`pipe-${index}`)),
        total: 50,
        offset: 0,
      })
      .mockResolvedValueOnce({
        items: [buildRun("pipe-50")],
        total: 1,
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

    expect(screen.getAllByTestId("Run-row")).toHaveLength(51);
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
        items: [buildRun("pipe-fresh")],
        total: 1,
        offset: 0,
      });
    });

    const { rerender } = render(<RunView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    rerender(<RunView apiClient={apiClient} projectId="proj-2" refreshToken={0} />);
    staleDeferred.resolve({
      items: [buildRun("pipe-stale")],
      total: 1,
      offset: 0,
    });

    await waitFor(() => {
      expect(apiClient.listRuns).toHaveBeenCalledWith("proj-2", {
        limit: 50,
        offset: 0,
      });
    });

    expect(screen.getByText("Run pipe-fresh")).toBeTruthy();
    expect(screen.queryByText("Run pipe-stale")).toBeNull();
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

  it("checkpoint 区展示 team_leader 字段", async () => {
    const apiClient = createMockApiClient();
    render(<RunView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    await waitFor(() => {
      expect(apiClient.getRunCheckpoints).toHaveBeenCalled();
    });

    expect(screen.getByText(/team_leader=claude/)).toBeTruthy();
  });

  it("change_role 按钮在无角色名时 disabled，有值时提交含 role 字段", async () => {
    const apiClient = createMockApiClient();
    render(<RunView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    await waitFor(() => {
      expect(screen.getByText("Run pipe-1")).toBeTruthy();
    });

    const changeRoleBtn = screen.getByRole("button", { name: "Change Team Leader" });
    expect((changeRoleBtn as HTMLButtonElement).disabled).toBe(true);

    const roleInput = screen.getByPlaceholderText(/目标 Team Leader/);
    fireEvent.change(roleInput, { target: { value: "codex" } });
    expect((changeRoleBtn as HTMLButtonElement).disabled).toBe(false);

    fireEvent.click(changeRoleBtn);
    await waitFor(() => {
      expect(apiClient.applyRunAction).toHaveBeenCalledWith("proj-1", "pipe-1", {
        action: "change_role",
        role: "codex",
        stage: "implement",
      });
    });
  });

  it("Pause 仅在 running 时启用, Resume 仅在 waiting_review 时启用", async () => {
    const apiClient = createMockApiClient();
    render(<RunView apiClient={apiClient} projectId="proj-1" refreshToken={0} />);

    await waitFor(() => {
      expect(screen.getByText("Run pipe-1")).toBeTruthy();
    });

    expect((screen.getByRole("button", { name: "Pause" }) as HTMLButtonElement).disabled).toBe(false);
    expect((screen.getByRole("button", { name: "Resume" }) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole("button", { name: "Rerun" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("显示 GitHub issue/pr 链接与状态徽标", async () => {
    const apiClient = createMockApiClient();
    vi.mocked(apiClient.listRuns).mockResolvedValue({
      items: [
        {
          ...buildRun("pipe-github"),
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
      expect(screen.getByText("Run pipe-github")).toBeTruthy();
    });

    expect(screen.getByText("Issue #201")).toBeTruthy();
    expect(screen.getByText("PR #301")).toBeTruthy();
    expect(screen.getByTestId("github-status-badge").textContent).toContain("Connected");
  });
});



