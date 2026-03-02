/** @vitest-environment jsdom */

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import FileTree from "./FileTree";
import type { ApiClient } from "../lib/apiClient";

const createDeferred = <T,>() => {
  let resolve: (value: T | PromiseLike<T>) => void = () => {};
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
};

const createMockApiClient = (): ApiClient => {
  const getRepoTree = vi
    .fn()
    .mockResolvedValueOnce({
      dir: "",
      items: [
        { path: "src", name: "src", type: "dir", git_status: "" },
        { path: "README.md", name: "README.md", type: "file", git_status: "" },
      ],
    })
    .mockResolvedValueOnce({
      dir: "src",
      items: [{ path: "src/main.ts", name: "main.ts", type: "file", git_status: "" }],
    });
  const getRepoStatus = vi.fn().mockResolvedValue({
    items: [
      { path: "README.md", name: "README.md", type: "file", git_status: "M" },
      { path: "src/main.ts", name: "main.ts", type: "file", git_status: "A" },
    ],
  });
  const getRepoDiff = vi.fn();

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
    getRepoTree,
    getRepoStatus,
    getRepoDiff,
  } as unknown as ApiClient;
};

describe("FileTree", () => {
  afterEach(() => {
    cleanup();
  });

  it("首次渲染会加载根目录与状态，并可懒加载子目录", async () => {
    const apiClient = createMockApiClient();
    const onToggleFile = vi.fn();

    render(
      <FileTree
        apiClient={apiClient}
        projectId="proj-1"
        selectedFiles={[]}
        onToggleFile={onToggleFile}
      />,
    );

    await waitFor(() => {
      expect(apiClient.getRepoTree).toHaveBeenCalledWith("proj-1", undefined);
      expect(apiClient.getRepoStatus).toHaveBeenCalledWith("proj-1");
    });

    expect(screen.getByText("README.md")).toBeTruthy();
    expect(screen.getByTitle("状态 M")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "展开目录 src" }));
    await waitFor(() => {
      expect(apiClient.getRepoTree).toHaveBeenCalledWith("proj-1", "src");
    });
    expect(screen.getByText("main.ts")).toBeTruthy();
  });

  it("文件 checkbox 切换时会触发 onToggleFile", async () => {
    const apiClient = createMockApiClient();
    const onToggleFile = vi.fn();

    render(
      <FileTree
        apiClient={apiClient}
        projectId="proj-1"
        selectedFiles={[]}
        onToggleFile={onToggleFile}
      />,
    );

    await waitFor(() => {
      expect(screen.getByLabelText("选择文件 README.md")).toBeTruthy();
    });

    const checkboxes = screen.getAllByLabelText("选择文件 README.md");
    fireEvent.click(checkboxes[0]);
    expect(onToggleFile).toHaveBeenCalledWith("README.md", true);
  });

  it("项目切换后会忽略旧请求结果，避免旧树数据回写", async () => {
    const deferredTree = createDeferred<{
      dir: string;
      items: Array<{ path: string; name: string; type: "file" | "dir"; git_status: string }>;
    }>();
    const deferredStatus = createDeferred<{
      items: Array<{ path: string; name: string; type: "file" | "dir"; git_status: string }>;
    }>();

    const apiClient = {
      ...createMockApiClient(),
      getRepoTree: vi
        .fn()
        .mockImplementationOnce(() => deferredTree.promise)
        .mockResolvedValueOnce({
          dir: "",
          items: [{ path: "new.txt", name: "new.txt", type: "file", git_status: "" }],
        }),
      getRepoStatus: vi
        .fn()
        .mockImplementationOnce(() => deferredStatus.promise)
        .mockResolvedValueOnce({ items: [] }),
    } as unknown as ApiClient;

    const { rerender } = render(
      <FileTree
        apiClient={apiClient}
        projectId="proj-1"
        selectedFiles={[]}
        onToggleFile={vi.fn()}
      />,
    );

    rerender(
      <FileTree
        apiClient={apiClient}
        projectId="proj-2"
        selectedFiles={[]}
        onToggleFile={vi.fn()}
      />,
    );

    await waitFor(() => {
      expect(apiClient.getRepoTree).toHaveBeenNthCalledWith(2, "proj-2", undefined);
      expect(apiClient.getRepoStatus).toHaveBeenNthCalledWith(2, "proj-2");
    });

    deferredTree.resolve({
      dir: "",
      items: [{ path: "old.txt", name: "old.txt", type: "file", git_status: "" }],
    });
    deferredStatus.resolve({ items: [] });

    await waitFor(() => {
      expect(screen.getByText("new.txt")).toBeTruthy();
    });
    expect(screen.queryByText("old.txt")).toBeNull();
  });
});
