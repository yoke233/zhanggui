/** @vitest-environment jsdom */

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import GitStatusPanel from "./GitStatusPanel";
import type { ApiClient } from "../lib/apiClient";

const createDeferred = <T,>() => {
  let resolve: (value: T | PromiseLike<T>) => void = () => {};
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
};

const createMockApiClient = (): ApiClient => {
  const getRepoStatus = vi.fn().mockResolvedValue({
    items: [
      { path: "src/main.ts", name: "main.ts", type: "file", git_status: "M" },
      { path: "src/new.ts", name: "new.ts", type: "file", git_status: "A" },
      { path: "src/old.ts", name: "old.ts", type: "file", git_status: "D" },
      { path: "README.md", name: "README.md", type: "file", git_status: "?" },
    ],
  });
  const getRepoDiff = vi.fn().mockResolvedValue({
    file_path: "src/main.ts",
    diff: `diff --git a/src/main.ts b/src/main.ts
index 1111111..2222222 100644
--- a/src/main.ts
+++ b/src/main.ts
@@ -1 +1 @@
-old
+new
`,
  });

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
    listPipelines: vi.fn(),
    createPipeline: vi.fn(),
    createChat: vi.fn(),
    getChat: vi.fn(),
    createPlan: vi.fn(),
    createPlanFromFiles: vi.fn(),
    submitPlanReview: vi.fn(),
    applyPlanAction: vi.fn(),
    applyTaskAction: vi.fn(),
    listPlans: vi.fn(),
    getPlanDag: vi.fn(),
    getPipeline: vi.fn(),
    getPipelineCheckpoints: vi.fn(),
    applyPipelineAction: vi.fn(),
    getRepoTree: vi.fn(),
    getRepoStatus,
    getRepoDiff,
  } as unknown as ApiClient;
};

describe("GitStatusPanel", () => {
  afterEach(() => {
    cleanup();
  });

  it("会按分组展示状态文件，并点击后内联展示 diff", async () => {
    const apiClient = createMockApiClient();

    render(<GitStatusPanel apiClient={apiClient} projectId="proj-1" />);

    await waitFor(() => {
      expect(apiClient.getRepoStatus).toHaveBeenCalledWith("proj-1");
    });

    expect(screen.getByText("Modified (1)")).toBeTruthy();
    expect(screen.getByText("Added (1)")).toBeTruthy();
    expect(screen.getByText("Deleted (1)")).toBeTruthy();
    expect(screen.getByText("Renamed (0)")).toBeTruthy();
    expect(screen.getByText("Untracked (1)")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: /src\/main\.ts/ }));
    await waitFor(() => {
      expect(apiClient.getRepoDiff).toHaveBeenCalledWith("proj-1", "src/main.ts");
    });

    expect(screen.getByText("CHANGED")).toBeTruthy();
  });

  it("空 diff 也会渲染兜底文案并避免重复请求", async () => {
    const apiClient = {
      ...createMockApiClient(),
      getRepoDiff: vi.fn().mockResolvedValue({
        file_path: "src/main.ts",
        diff: "",
      }),
    } as unknown as ApiClient;

    render(<GitStatusPanel apiClient={apiClient} projectId="proj-1" />);

    await waitFor(() => {
      expect(apiClient.getRepoStatus).toHaveBeenCalledWith("proj-1");
    });

    const fileButton = screen.getByRole("button", { name: /src\/main\.ts/ });
    fireEvent.click(fileButton);
    await waitFor(() => {
      expect(apiClient.getRepoDiff).toHaveBeenCalledTimes(1);
    });
    expect(screen.getByText("暂无 diff 内容。")).toBeTruthy();

    fireEvent.click(fileButton);
    expect(apiClient.getRepoDiff).toHaveBeenCalledTimes(1);
  });

  it("同一文件在 diff 加载中重复点击不会重复请求", async () => {
    const deferredDiff = createDeferred<{ file_path: string; diff: string }>();
    const apiClient = {
      ...createMockApiClient(),
      getRepoDiff: vi.fn().mockImplementation(() => deferredDiff.promise),
    } as unknown as ApiClient;

    render(<GitStatusPanel apiClient={apiClient} projectId="proj-1" />);

    await waitFor(() => {
      expect(apiClient.getRepoStatus).toHaveBeenCalledWith("proj-1");
    });

    const fileButton = screen.getByRole("button", { name: /src\/main\.ts/ });
    fireEvent.click(fileButton);
    fireEvent.click(fileButton);
    expect(apiClient.getRepoDiff).toHaveBeenCalledTimes(1);

    deferredDiff.resolve({
      file_path: "src/main.ts",
      diff: "diff --git a/src/main.ts b/src/main.ts",
    });
    await waitFor(() => {
      expect(screen.getByText("src/main.ts")).toBeTruthy();
    });
  });

  it("项目切换后会忽略旧 diff 请求结果", async () => {
    const deferredDiff = createDeferred<{ file_path: string; diff: string }>();
    const apiClient = {
      ...createMockApiClient(),
      getRepoStatus: vi
        .fn()
        .mockResolvedValueOnce({
          items: [{ path: "src/main.ts", name: "main.ts", type: "file", git_status: "M" }],
        })
        .mockResolvedValueOnce({ items: [] }),
      getRepoDiff: vi.fn().mockImplementation(() => deferredDiff.promise),
    } as unknown as ApiClient;

    const { rerender } = render(<GitStatusPanel apiClient={apiClient} projectId="proj-1" />);

    await waitFor(() => {
      expect(apiClient.getRepoStatus).toHaveBeenNthCalledWith(1, "proj-1");
    });

    fireEvent.click(screen.getByRole("button", { name: /src\/main\.ts/ }));
    expect(apiClient.getRepoDiff).toHaveBeenCalledTimes(1);

    rerender(<GitStatusPanel apiClient={apiClient} projectId="proj-2" />);
    await waitFor(() => {
      expect(apiClient.getRepoStatus).toHaveBeenNthCalledWith(2, "proj-2");
      expect(screen.getByText("Modified (0)")).toBeTruthy();
    });

    deferredDiff.resolve({
      file_path: "src/main.ts",
      diff: "diff --git a/src/main.ts b/src/main.ts",
    });

    await waitFor(() => {
      expect(screen.queryByRole("button", { name: /src\/main\.ts/ })).toBeNull();
    });
  });
});
