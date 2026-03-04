/** @vitest-environment jsdom */

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import ChatView from "./ChatView";
import type { ApiClient } from "../lib/apiClient";
import type { WsClient } from "../lib/wsClient";
import type { ApiIssue } from "../types/api";
import type { WsEnvelope } from "../types/ws";

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

const buildIssue = (id: string): ApiIssue => ({
  id,
  project_id: "proj-1",
  session_id: "chat-1",
  title: "issue-title",
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
});

const buildChatSession = (overrides?: {
  id?: string;
  userContent?: string;
  assistantContent?: string;
  createdAt?: string;
  updatedAt?: string;
}) => ({
  id: overrides?.id ?? "chat-1",
  project_id: "proj-1",
  messages: [
    {
      role: "user" as const,
      content: overrides?.userContent ?? "请拆分任务",
      time: overrides?.createdAt ?? "2026-03-01T10:00:00.000Z",
    },
    {
      role: "assistant" as const,
      content: overrides?.assistantContent ?? "已完成拆分",
      time: overrides?.updatedAt ?? "2026-03-01T10:01:00.000Z",
    },
  ],
  created_at: overrides?.createdAt ?? "2026-03-01T10:00:00.000Z",
  updated_at: overrides?.updatedAt ?? "2026-03-01T10:01:00.000Z",
});

const createMockApiClient = (): ApiClient => {
  const createChat = vi.fn().mockResolvedValue({
    session_id: "chat-1",
    status: "accepted",
  });
  const cancelChat = vi.fn().mockResolvedValue({
    session_id: "chat-1",
    status: "cancelling",
  });
  const getChat = vi.fn().mockResolvedValue(buildChatSession());
  const createIssue = vi.fn().mockResolvedValue(buildIssue("plan-1"));
  const createIssueFromFiles = vi.fn().mockResolvedValue(buildIssue("plan-files-1"));
  const listChatRunEvents = vi.fn().mockResolvedValue([]);
  const listChats = vi.fn().mockResolvedValue([
    buildChatSession({
      id: "chat-1",
      updatedAt: "2026-03-01T10:01:00.000Z",
    }),
    buildChatSession({
      id: "chat-2",
      userContent: "第二个会话提问",
      assistantContent: "第二个会话回复",
      createdAt: "2026-03-01T11:00:00.000Z",
      updatedAt: "2026-03-01T11:02:00.000Z",
    }),
  ]);
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
    listRuns: vi.fn(),
    createRun: vi.fn(),
    createChat,
    cancelChat,
    getChat,
    listChats,
    listChatRunEvents,
    createIssue,
    createIssueFromFiles,
    getRepoTree,
    getRepoStatus,
    getRepoDiff,
    listPlans: vi.fn(),
    getPlanDag: vi.fn(),
  } as unknown as ApiClient;
};

const createMockWsHarness = (): {
  wsClient: WsClient;
  emit: (envelope: WsEnvelope<Record<string, unknown>>) => void;
} => {
  let wildcardHandler:
    | ((payload: WsEnvelope<Record<string, unknown>>) => void)
    | null = null;

  const wsClient = {
    connect: vi.fn(),
    disconnect: vi.fn(),
    send: vi.fn(),
    subscribe: vi.fn().mockImplementation((type, handler) => {
      if (type === "*") {
        wildcardHandler = handler as (payload: WsEnvelope<Record<string, unknown>>) => void;
      }
      return () => {
        if (wildcardHandler === handler) {
          wildcardHandler = null;
        }
      };
    }),
    onStatusChange: vi.fn().mockReturnValue(() => {}),
    getStatus: vi.fn().mockReturnValue("open"),
  } as unknown as WsClient;

  return {
    wsClient,
    emit: (envelope) => {
      wildcardHandler?.(envelope);
    },
  };
};

describe("ChatView", () => {
  afterEach(() => {
    cleanup();
  });

  it("发送消息后使用 ACK + WS 流式渲染，并在 completed 后 refreshSession", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();

    render(<ChatView apiClient={apiClient} wsClient={wsHarness.wsClient} projectId="proj-1" />);

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "请拆分任务" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(apiClient.createChat).toHaveBeenCalledWith("proj-1", {
        message: "请拆分任务",
      });
    });
    expect(apiClient.getChat).not.toHaveBeenCalled();

    wsHarness.emit({
      type: "run_started",
      project_id: "proj-1",
      data: { session_id: "chat-1" },
    });
    wsHarness.emit({
      type: "run_update",
      project_id: "proj-1",
      data: {
        session_id: "chat-1",
        acp: {
          sessionUpdate: "agent_message_chunk",
          content: {
            text: "第一段",
          },
        },
      },
    });

    await waitFor(() => {
      expect(screen.getByText("助手 · 输入中...")).toBeTruthy();
      expect(screen.getByText("第一段")).toBeTruthy();
    });

    wsHarness.emit({
      type: "run_update",
      project_id: "proj-1",
      data: {
        session_id: "chat-1",
        acp: {
          sessionUpdate: "unknown_update_type",
          content: {
            text: "ignored",
          },
        },
      },
    });

    wsHarness.emit({
      type: "run_completed",
      project_id: "proj-1",
      data: {
        session_id: "chat-1",
        reply: "这条会被 refreshSession 覆盖",
      },
    });

    await waitFor(() => {
      expect(apiClient.getChat).toHaveBeenCalledWith("proj-1", "chat-1");
      expect(screen.getAllByText("已完成拆分").length).toBeGreaterThan(0);
    });
    expect(screen.queryByText("助手 · 输入中...")).toBeNull();
    expect(screen.getByRole("button", { name: "发送" })).toBeTruthy();
  });

  it("运行中点击停止会调用 cancelChat，并在 cancelled 后退出运行态", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();

    render(<ChatView apiClient={apiClient} wsClient={wsHarness.wsClient} projectId="proj-1" />);

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "请开始执行" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(apiClient.createChat).toHaveBeenCalledWith("proj-1", {
        message: "请开始执行",
      });
    });

    wsHarness.emit({
      type: "run_started",
      project_id: "proj-1",
      data: { session_id: "chat-1" },
    });

    fireEvent.click(screen.getByRole("button", { name: "停止" }));

    await waitFor(() => {
      expect(apiClient.cancelChat).toHaveBeenCalledWith("proj-1", "chat-1");
    });

    wsHarness.emit({
      type: "run_cancelled",
      project_id: "proj-1",
      data: { session_id: "chat-1" },
    });

    await waitFor(() => {
      expect(screen.getByText("当前请求已取消")).toBeTruthy();
      expect(screen.getByRole("button", { name: "发送" })).toBeTruthy();
    });
  });

  it("WS 事件会按 session_id 过滤，非当前会话更新会被忽略", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();

    render(<ChatView apiClient={apiClient} wsClient={wsHarness.wsClient} projectId="proj-1" />);

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "请拆分任务" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(screen.getByText("Session ID: chat-1")).toBeTruthy();
    });

    wsHarness.emit({
      type: "run_started",
      project_id: "proj-1",
      data: { session_id: "chat-1" },
    });
    wsHarness.emit({
      type: "run_update",
      project_id: "proj-1",
      data: {
        session_id: "chat-2",
        acp: {
          sessionUpdate: "agent_message_chunk",
          content: {
            text: "不应出现",
          },
        },
      },
    });
    wsHarness.emit({
      type: "run_update",
      project_id: "proj-1",
      data: {
        session_id: "chat-1",
        acp: {
          sessionUpdate: "agent_message_chunk",
          content: {
            text: "应出现",
          },
        },
      },
    });

    await waitFor(() => {
      expect(screen.getByText("应出现")).toBeTruthy();
    });
    expect(screen.queryByText("不应出现")).toBeNull();
  });

  it("忽略 started 之前或早于 started 时间戳的增量，避免串入上一轮内容", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();

    render(<ChatView apiClient={apiClient} wsClient={wsHarness.wsClient} projectId="proj-1" />);

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "第二轮开始" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(screen.getByText("Session ID: chat-1")).toBeTruthy();
    });

    wsHarness.emit({
      type: "run_update",
      project_id: "proj-1",
      data: {
        session_id: "chat-1",
        timestamp: "2026-03-03T01:39:59.000Z",
        acp: {
          sessionUpdate: "agent_message_chunk",
          content: {
            text: "旧轮残留",
          },
        },
      },
    });

    wsHarness.emit({
      type: "run_started",
      project_id: "proj-1",
      data: {
        session_id: "chat-1",
        timestamp: "2026-03-03T01:40:00.000Z",
      },
    });

    wsHarness.emit({
      type: "run_update",
      project_id: "proj-1",
      data: {
        session_id: "chat-1",
        timestamp: "2026-03-03T01:40:01.000Z",
        acp: {
          sessionUpdate: "agent_message_chunk",
          content: {
            text: "当前轮内容",
          },
        },
      },
    });

    await waitFor(() => {
      expect(screen.getByText("当前轮内容")).toBeTruthy();
    });
    expect(screen.queryByText("旧轮残留")).toBeNull();
  });

  it("已有 session 后按钮显示发送，并携带 session_id 继续对话", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();

    render(<ChatView apiClient={apiClient} wsClient={wsHarness.wsClient} projectId="proj-1" />);

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "第一轮" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(apiClient.createChat).toHaveBeenCalledWith("proj-1", {
        message: "第一轮",
      });
      expect(screen.getByText("Session ID: chat-1")).toBeTruthy();
    });

    wsHarness.emit({
      type: "run_started",
      project_id: "proj-1",
      data: { session_id: "chat-1" },
    });
    wsHarness.emit({
      type: "run_completed",
      project_id: "proj-1",
      data: { session_id: "chat-1", reply: "第一轮完成" },
    });

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "发送" })).toBeTruthy();
    });

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "第二轮" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送" }));

    await waitFor(() => {
      expect(apiClient.createChat).toHaveBeenNthCalledWith(2, "proj-1", {
        message: "第二轮",
        session_id: "chat-1",
      });
    });
  });

  it("会话完成后可触发 createIssueFromFiles", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();

    render(<ChatView apiClient={apiClient} wsClient={wsHarness.wsClient} projectId="proj-1" />);

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "请拆分任务" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(apiClient.createChat).toHaveBeenCalledWith("proj-1", {
        message: "请拆分任务",
      });
      expect(screen.getByText("Session ID: chat-1")).toBeTruthy();
    });

    wsHarness.emit({
      type: "run_started",
      project_id: "proj-1",
      data: { session_id: "chat-1" },
    });
    wsHarness.emit({
      type: "run_completed",
      project_id: "proj-1",
      data: { session_id: "chat-1", reply: "done" },
    });

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "从文件创建 issue" })).toBeTruthy();
    });

    fireEvent.change(screen.getByLabelText("文件路径（逗号分隔）"), {
      target: { value: "cmd/app/main.go, internal/core/task.go,  ,web/src/App.tsx" },
    });
    fireEvent.click(screen.getByRole("button", { name: "从文件创建 issue" }));

    await waitFor(() => {
      expect(apiClient.createIssueFromFiles).toHaveBeenCalledWith("proj-1", {
        session_id: "chat-1",
        file_paths: ["cmd/app/main.go", "internal/core/task.go", "web/src/App.tsx"],
      });
    });
    expect(screen.getByText("已从文件创建 issue：plan-files-1")).toBeTruthy();
  });

  it("支持展示会话列表并切换会话", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();

    vi.mocked(apiClient.getChat).mockImplementation(async (_projectID: string, sessionID: string) => {
      if (sessionID === "chat-2") {
        return buildChatSession({
          id: "chat-2",
          userContent: "第二个会话提问",
          assistantContent: "第二个会话回复",
          createdAt: "2026-03-01T11:00:00.000Z",
          updatedAt: "2026-03-01T11:02:00.000Z",
        });
      }
      return buildChatSession({
        id: "chat-1",
        assistantContent: "第一个会话回复",
      });
    });
    vi.mocked(apiClient.listChatRunEvents).mockImplementation(async (_projectID: string, sessionID: string) => {
      if (sessionID === "chat-2") {
        return [
          {
            id: 2,
            session_id: "chat-2",
            project_id: "proj-1",
            event_type: "run_update",
            update_type: "tool_call",
            payload: {
              session_id: "chat-2",
              acp: {
                sessionUpdate: "tool_call",
                title: "Terminal",
              },
            },
            created_at: "2026-03-01T11:01:00.000Z",
          },
        ];
      }
      return [];
    });

    render(<ChatView apiClient={apiClient} wsClient={wsHarness.wsClient} projectId="proj-1" />);

    await waitFor(() => {
      expect((apiClient as unknown as { listChats: ReturnType<typeof vi.fn> }).listChats).toHaveBeenCalledWith("proj-1");
    });

    fireEvent.click(screen.getByRole("button", { name: "chat-2" }));

    await waitFor(() => {
      expect(apiClient.getChat).toHaveBeenCalledWith("proj-1", "chat-2");
      expect(apiClient.listChatRunEvents).toHaveBeenCalledWith("proj-1", "chat-2");
      expect(screen.getAllByText("第二个会话回复").length).toBeGreaterThan(0);
      expect(screen.getByText(/Terminal/)).toBeTruthy();
    });
    expect(screen.getByText("Session ID: chat-2")).toBeTruthy();
  });

  it("可展示 tool_call/plan 等非文本运行事件", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();

    render(<ChatView apiClient={apiClient} wsClient={wsHarness.wsClient} projectId="proj-1" />);

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "执行任务" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(apiClient.createChat).toHaveBeenCalledWith("proj-1", {
        message: "执行任务",
      });
    });

    wsHarness.emit({
      type: "run_started",
      project_id: "proj-1",
      data: { session_id: "chat-1" },
    });
    wsHarness.emit({
      type: "run_update",
      project_id: "proj-1",
      data: {
        session_id: "chat-1",
        acp: {
          sessionUpdate: "tool_call",
          title: "读取文件",
          kind: "shell",
          status: "running",
        },
      },
    });
    wsHarness.emit({
      type: "run_update",
      project_id: "proj-1",
      data: {
        session_id: "chat-1",
        acp: {
          sessionUpdate: "plan",
          entries: [{ content: "步骤 1", status: "pending" }],
        },
      },
    });

    await waitFor(() => {
      expect(screen.getByText("运行事件")).toBeTruthy();
      expect(screen.getByText(/tool_call/)).toBeTruthy();
      expect(screen.getByText(/读取文件/)).toBeTruthy();
      expect(screen.getByText(/plan/)).toBeTruthy();
      expect(screen.getByText(/步骤 1/)).toBeTruthy();
    });
  });

  it("助手消息支持基础 Markdown 渲染", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();
    vi.mocked(apiClient.getChat).mockResolvedValue(
      buildChatSession({
        assistantContent: "# 计划概览\n- [文档](https://example.com)\n- `run test`",
      }),
    );

    render(<ChatView apiClient={apiClient} wsClient={wsHarness.wsClient} projectId="proj-1" />);

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "触发会话" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(apiClient.createChat).toHaveBeenCalledWith("proj-1", {
        message: "触发会话",
      });
      expect(screen.getByText("Session ID: chat-1")).toBeTruthy();
    });

    wsHarness.emit({
      type: "run_started",
      project_id: "proj-1",
      data: { session_id: "chat-1" },
    });
    wsHarness.emit({
      type: "run_completed",
      project_id: "proj-1",
      data: { session_id: "chat-1", reply: "最终回复" },
    });

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "计划概览" })).toBeTruthy();
    });
    const link = screen.getByRole("link", { name: "文档" }) as HTMLAnchorElement;
    expect(link.href).toBe("https://example.com/");
    expect(screen.getByText("run test")).toBeTruthy();
  });

  it("文件树选择会自动同步到文件路径输入框", () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();
    render(<ChatView apiClient={apiClient} wsClient={wsHarness.wsClient} projectId="proj-1" />);

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
    const wsHarness = createMockWsHarness();
    render(<ChatView apiClient={apiClient} wsClient={wsHarness.wsClient} projectId="proj-1" />);

    expect(screen.getByText("FileTreeMock")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Git Status" }));
    expect(screen.getByText("GitStatusPanelMock")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "文件树" }));
    expect(screen.getByText("FileTreeMock")).toBeTruthy();
  });

});

