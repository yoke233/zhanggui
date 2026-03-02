/** @vitest-environment jsdom */

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import ChatView from "./ChatView";
import type { ApiClient } from "../lib/apiClient";
import type { ApiTaskPlan, CreateChatResponse } from "../types/api";

vi.mock("../components/FileTree", () => ({
  default: ({
    onToggleFile,
    selectedFiles,
  }: {
    onToggleFile: (filePath: string, selected: boolean) => void;
    selectedFiles: string[];
  }) => (
    <div>
      <p>FileTreeMock</p>
      <button
        type="button"
        onClick={() => {
          onToggleFile("cmd/app/main.go", true);
        }}
      >
        选择 main.go
      </button>
      <button
        type="button"
        onClick={() => {
          onToggleFile("internal/core/task.go", true);
        }}
      >
        选择 task.go
      </button>
      <button
        type="button"
        onClick={() => {
          onToggleFile("cmd/app/main.go", false);
        }}
      >
        取消 main.go
      </button>
      <p data-testid="selected-files-count">{selectedFiles.length}</p>
    </div>
  ),
}));

vi.mock("../components/GitStatusPanel", () => ({
  default: () => <div>GitStatusPanelMock</div>,
}));

const buildPlan = (id: string): ApiTaskPlan => ({
  id,
  project_id: "proj-1",
  session_id: "chat-1",
  name: "plan-name",
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
});

const createMockApiClient = (): ApiClient => {
  const createChat = vi.fn().mockResolvedValue({
    session_id: "chat-1",
    reply: "ok",
  });
  const getChat = vi.fn().mockResolvedValue({
    id: "chat-1",
    project_id: "proj-1",
    messages: [
      {
        role: "user",
        content: "请拆分任务",
        time: "2026-03-01T10:00:00.000Z",
      },
      {
        role: "assistant",
        content: "已完成拆分",
        time: "2026-03-01T10:01:00.000Z",
      },
    ],
    created_at: "2026-03-01T10:00:00.000Z",
    updated_at: "2026-03-01T10:01:00.000Z",
  });
  const createPlan = vi.fn().mockResolvedValue(buildPlan("plan-1"));
  const createPlanFromFiles = vi.fn().mockResolvedValue(buildPlan("plan-files-1"));
  const getRepoTree = vi.fn().mockResolvedValue({ dir: "", items: [] });
  const getRepoStatus = vi.fn().mockResolvedValue({ items: [] });
  const getRepoDiff = vi.fn().mockResolvedValue({ file_path: "", diff: "" });

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
    createChat,
    getChat,
    createPlan,
    createPlanFromFiles,
    getRepoTree,
    getRepoStatus,
    getRepoDiff,
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

describe("ChatView", () => {
  afterEach(() => {
    cleanup();
  });

  it("发送消息后调用 createChat + getChat 并渲染会话消息", async () => {
    const apiClient = createMockApiClient();

    render(<ChatView apiClient={apiClient} projectId="proj-1" />);

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "请拆分任务" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(apiClient.createChat).toHaveBeenCalledWith("proj-1", {
        message: "请拆分任务",
      });
      expect(apiClient.getChat).toHaveBeenCalledWith("proj-1", "chat-1");
    });

    expect(screen.getByText(/助手/)).toBeTruthy();
    expect(screen.getByText("已完成拆分")).toBeTruthy();
  });

  it("已有会话时发送消息会携带 session_id 继续对话", async () => {
    const apiClient = createMockApiClient();

    render(<ChatView apiClient={apiClient} projectId="proj-1" />);

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "第一轮" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(apiClient.createChat).toHaveBeenNthCalledWith(1, "proj-1", {
        message: "第一轮",
      });
    });
    await waitFor(() => {
      expect(screen.getByText("Session ID: chat-1")).toBeTruthy();
    });

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "第二轮" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(apiClient.createChat).toHaveBeenNthCalledWith(2, "proj-1", {
        message: "第二轮",
        session_id: "chat-1",
      });
    });
  });

  it("会话创建后可触发 createPlan", async () => {
    const apiClient = createMockApiClient();

    render(<ChatView apiClient={apiClient} projectId="proj-1" />);

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "请拆分任务" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(apiClient.getChat).toHaveBeenCalled();
    });

    fireEvent.click(screen.getByRole("button", { name: "基于当前会话创建计划" }));

    await waitFor(() => {
      expect(apiClient.createPlan).toHaveBeenCalledWith("proj-1", {
        session_id: "chat-1",
      });
    });

    expect(screen.getByText("已创建计划：plan-1")).toBeTruthy();
  });

  it("项目切换后会忽略旧会话请求，避免回写到新项目", async () => {
    const deferredChat = createDeferred<CreateChatResponse>();
    const apiClient = createMockApiClient();
    vi.mocked(apiClient.createChat).mockImplementation(() => deferredChat.promise);

    const { rerender } = render(<ChatView apiClient={apiClient} projectId="proj-1" />);

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "旧项目请求" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    rerender(<ChatView apiClient={apiClient} projectId="proj-2" />);
    deferredChat.resolve({
      session_id: "chat-stale",
      reply: "stale",
    });

    await waitFor(() => {
      expect(apiClient.createChat).toHaveBeenCalledWith("proj-1", {
        message: "旧项目请求",
      });
    });
    await waitFor(() => {
      expect(apiClient.getChat).not.toHaveBeenCalled();
    });

    expect(screen.getByText("Session ID: 未创建")).toBeTruthy();
    expect(screen.queryByText("已完成拆分")).toBeNull();
  });

  it("createPlan 请求在项目切换后晚到时不会回写新项目状态", async () => {
    const deferredPlan = createDeferred<ApiTaskPlan>();
    const apiClient = createMockApiClient();
    vi.mocked(apiClient.createPlan).mockImplementation(() => deferredPlan.promise);

    const { rerender } = render(<ChatView apiClient={apiClient} projectId="proj-1" />);

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "请拆分任务" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(apiClient.getChat).toHaveBeenCalledWith("proj-1", "chat-1");
    });

    fireEvent.click(screen.getByRole("button", { name: "基于当前会话创建计划" }));

    await waitFor(() => {
      expect(apiClient.createPlan).toHaveBeenCalledWith("proj-1", {
        session_id: "chat-1",
      });
    });

    rerender(<ChatView apiClient={apiClient} projectId="proj-2" />);
    deferredPlan.resolve(buildPlan("plan-stale"));

    await waitFor(() => {
      expect(screen.getByText("Session ID: 未创建")).toBeTruthy();
    });
    expect(screen.queryByText("已创建计划：plan-stale")).toBeNull();
  });

  it("输入文件路径后可调用 createPlanFromFiles", async () => {
    const apiClient = createMockApiClient();

    render(<ChatView apiClient={apiClient} projectId="proj-1" />);

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "请拆分任务" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(apiClient.getChat).toHaveBeenCalledWith("proj-1", "chat-1");
    });

    fireEvent.change(screen.getByLabelText("文件路径（逗号分隔）"), {
      target: { value: "cmd/app/main.go, internal/core/task.go,  ,web/src/App.tsx" },
    });
    fireEvent.click(screen.getByRole("button", { name: "从文件创建计划" }));

    await waitFor(() => {
      expect(apiClient.createPlanFromFiles).toHaveBeenCalledWith("proj-1", {
        session_id: "chat-1",
        file_paths: ["cmd/app/main.go", "internal/core/task.go", "web/src/App.tsx"],
      });
    });
    expect(screen.getByText("已从文件创建计划：plan-files-1")).toBeTruthy();
  });

  it("助手消息支持基础 Markdown 渲染", async () => {
    const apiClient = createMockApiClient();
    vi.mocked(apiClient.getChat).mockResolvedValue({
      id: "chat-1",
      project_id: "proj-1",
      messages: [
        {
          role: "assistant",
          content: "# 计划概览\n- [文档](https://example.com)\n- `run test`",
          time: "2026-03-01T10:01:00.000Z",
        },
      ],
      created_at: "2026-03-01T10:00:00.000Z",
      updated_at: "2026-03-01T10:01:00.000Z",
    });

    render(<ChatView apiClient={apiClient} projectId="proj-1" />);

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "触发会话" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "计划概览" })).toBeTruthy();
    });
    const link = screen.getByRole("link", { name: "文档" }) as HTMLAnchorElement;
    expect(link.href).toBe("https://example.com/");
    expect(screen.getByText("run test")).toBeTruthy();
  });

  it("文件树选择会自动同步到文件路径输入框", () => {
    const apiClient = createMockApiClient();
    render(<ChatView apiClient={apiClient} projectId="proj-1" />);

    const input = screen.getByLabelText("文件路径（逗号分隔）") as HTMLInputElement;
    expect(input.value).toBe("");

    fireEvent.click(screen.getByRole("button", { name: "选择 main.go" }));
    expect(input.value).toBe("cmd/app/main.go");

    fireEvent.click(screen.getByRole("button", { name: "选择 task.go" }));
    expect(input.value).toBe("cmd/app/main.go, internal/core/task.go");

    fireEvent.click(screen.getByRole("button", { name: "取消 main.go" }));
    expect(input.value).toBe("internal/core/task.go");
  });

  it("左侧面板支持在文件树与 Git Status 之间切换", () => {
    const apiClient = createMockApiClient();
    render(<ChatView apiClient={apiClient} projectId="proj-1" />);

    expect(screen.getByText("FileTreeMock")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Git Status" }));
    expect(screen.getByText("GitStatusPanelMock")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "文件树" }));
    expect(screen.getByText("FileTreeMock")).toBeTruthy();
  });

});
