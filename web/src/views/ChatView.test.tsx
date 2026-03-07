/** @vitest-environment jsdom */

import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import ChatView from "./ChatView";
import { ApiError } from "../lib/apiClient";
import type { ApiClient } from "../lib/apiClient";
import type { WsClient } from "../lib/wsClient";
import type { ApiIssue, ChatRunEvent } from "../types/api";
import type { ChatMessage } from "../types/workflow";
import type { WsEnvelope } from "../types/ws";
import { useChatStore } from "../stores/chatStore";

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
  parent_id: "",
  external_id: "",
  submitted_by: "",
  merge_retries: 0,
  triage_instructions: "",
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

const buildChatEventsPage = (overrides?: {
  sessionId?: string;
  projectId?: string;
  updatedAt?: string;
  messages?: ChatMessage[];
  events?: ChatRunEvent[];
  nextCursor?: string;
}) => ({
  session_id: overrides?.sessionId ?? "chat-1",
  project_id: overrides?.projectId ?? "proj-1",
  updated_at: overrides?.updatedAt ?? "2026-03-01T10:01:00.000Z",
  messages: overrides?.messages ?? buildChatSession().messages,
  events: overrides?.events ?? [],
  next_cursor: overrides?.nextCursor ?? "",
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
  const createIssueFromFiles = vi
    .fn()
    .mockResolvedValue(buildIssue("plan-files-1"));
  const listChatRunEvents = vi.fn().mockResolvedValue(buildChatEventsPage());
  const getChatEventGroup = vi.fn().mockResolvedValue({
    session_id: "chat-1",
    project_id: "proj-1",
    group_id: "tool-call-group:1:2",
    events: [],
  });
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
  const getSessionCommands = vi.fn().mockResolvedValue([]);
  const getSessionConfigOptions = vi.fn().mockResolvedValue([]);
  const setSessionConfigOption = vi.fn().mockResolvedValue([]);
  const listAgents = vi.fn().mockResolvedValue({ agents: [{ name: "claude" }, { name: "codex" }] });

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
    createChat,
    cancelChat,
    getChat,
    listChats,
    listChatRunEvents,
    getChatEventGroup,
    createIssue,
    createIssueFromFiles,
    getRepoTree,
    getRepoStatus,
    getRepoDiff,
    listIssues: vi.fn().mockResolvedValue({ items: [], total: 0 }),
    getIssueDag: vi.fn(),
    getRun: vi.fn(),
    getRunCheckpoints: vi.fn().mockResolvedValue({ items: [], total: 0 }),
    getChatSessionStatus: vi
      .fn()
      .mockResolvedValue({ alive: false, running: false }),
    getSessionCommands,
    getSessionConfigOptions,
    setSessionConfigOption,
    listAgents,
    getStageSessionStatus: vi
      .fn()
      .mockResolvedValue({ alive: false, session_id: "" }),
    wakeStageSession: vi.fn().mockResolvedValue({ session_id: "" }),
    promptStageSession: vi.fn().mockResolvedValue(undefined),
  } as unknown as ApiClient;
};

const createMockWsHarness = (): {
  wsClient: WsClient;
  emit: (envelope: WsEnvelope<Record<string, unknown>>) => void;
  emitStatus: (status: "idle" | "connecting" | "open" | "closed") => void;
} => {
  let wildcardHandler:
    | ((payload: WsEnvelope<Record<string, unknown>>) => void)
    | null = null;
  let statusHandler:
    | ((status: "idle" | "connecting" | "open" | "closed") => void)
    | null = null;

  const wsClient = {
    connect: vi.fn(),
    disconnect: vi.fn(),
    send: vi.fn(),
    subscribe: vi.fn().mockImplementation((type, handler) => {
      if (type === "*") {
        wildcardHandler = handler as (
          payload: WsEnvelope<Record<string, unknown>>,
        ) => void;
      }
      return () => {
        if (wildcardHandler === handler) {
          wildcardHandler = null;
        }
      };
    }),
    onStatusChange: vi.fn().mockImplementation((handler) => {
      statusHandler = handler as (
        status: "idle" | "connecting" | "open" | "closed",
      ) => void;
      return () => {
        if (statusHandler === handler) {
          statusHandler = null;
        }
      };
    }),
    getStatus: vi.fn().mockReturnValue("open"),
  } as unknown as WsClient;

  return {
    wsClient,
    emit: (envelope) => {
      wildcardHandler?.(envelope);
    },
    emitStatus: (status) => {
      statusHandler?.(status);
    },
  };
};

describe("ChatView", () => {
  afterEach(() => {
    useChatStore.getState().reset();
    cleanup();
  });

  it("发送消息后使用 ACK + WS 流式渲染，并在 completed 后通过 events 刷新", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();

    render(
      <ChatView
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        projectId="proj-1"
      />,
    );

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "请拆分任务" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(apiClient.createChat).toHaveBeenCalledWith("proj-1", expect.objectContaining({
        message: "请拆分任务",
      }));
    });
    expect(apiClient.listChatRunEvents).not.toHaveBeenCalled();

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
      expect(screen.getByText("输入中...")).toBeTruthy();
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
        reply: "这条会被 events 刷新覆盖",
      },
    });

    await waitFor(() => {
      expect(apiClient.listChatRunEvents).toHaveBeenCalledWith(
        "proj-1",
        "chat-1",
        {
          limit: 50,
          cursor: undefined,
        },
      );
      expect(screen.getAllByText("已完成拆分").length).toBeGreaterThan(0);
    });
    expect(screen.queryByText("输入中...")).toBeNull();
    expect(screen.getByRole("button", { name: "发送" })).toBeTruthy();
  });

  it("运行中点击停止会调用 cancelChat，并在 cancelled 后退出运行态", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();

    render(
      <ChatView
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        projectId="proj-1"
      />,
    );

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "请开始执行" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(apiClient.createChat).toHaveBeenCalledWith("proj-1", expect.objectContaining({
        message: "请开始执行",
      }));
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

    render(
      <ChatView
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        projectId="proj-1"
      />,
    );

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

  it("创建会话后会发送 subscribe_chat_session，并在切换会话时先退订旧会话", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();

    render(
      <ChatView
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        projectId="proj-1"
      />,
    );

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "请创建会话" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(screen.getByText("Session ID: chat-1")).toBeTruthy();
    });
    await waitFor(() => {
      expect(wsHarness.wsClient.send).toHaveBeenCalledWith({
        type: "subscribe_chat_session",
        session_id: "chat-1",
      });
    });

    wsHarness.emit({
      type: "run_completed",
      project_id: "proj-1",
      data: { session_id: "chat-1" },
    });
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "发送" })).toBeTruthy();
    });

    fireEvent.click(screen.getByRole("button", { name: "chat-2" }));
    await waitFor(() => {
      expect(wsHarness.wsClient.send).toHaveBeenCalledWith({
        type: "unsubscribe_chat_session",
        session_id: "chat-1",
      });
      expect(wsHarness.wsClient.send).toHaveBeenCalledWith({
        type: "subscribe_chat_session",
        session_id: "chat-2",
      });
    });
  });

  it("WS 重连后会自动补发当前会话的 subscribe_chat_session", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();

    render(
      <ChatView
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        projectId="proj-1"
      />,
    );

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "请创建会话" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(screen.getByText("Session ID: chat-1")).toBeTruthy();
    });

    const sendMock = wsHarness.wsClient.send as unknown as {
      mockClear: () => void;
    };
    sendMock.mockClear();
    wsHarness.emitStatus("open");

    await waitFor(() => {
      expect(wsHarness.wsClient.send).toHaveBeenCalledWith({
        type: "subscribe_chat_session",
        session_id: "chat-1",
      });
    });
  });

  it("收到同会话 started 且本端未发起时，显示跨端运行提示", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();

    render(
      <ChatView
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        projectId="proj-1"
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "chat-1" })).toBeTruthy();
    });
    fireEvent.click(screen.getByRole("button", { name: "chat-1" }));

    await waitFor(() => {
      expect(screen.getByText("Session ID: chat-1")).toBeTruthy();
    });

    wsHarness.emit({
      type: "run_started",
      project_id: "proj-1",
      data: { session_id: "chat-1" },
    });

    await waitFor(() => {
      expect(
        screen.getByText("当前会话正在其他终端运行，界面已进入同步监听。"),
      ).toBeTruthy();
    });

    wsHarness.emit({
      type: "run_completed",
      project_id: "proj-1",
      data: { session_id: "chat-1" },
    });

    await waitFor(() => {
      expect(
        screen.queryByText("当前会话正在其他终端运行，界面已进入同步监听。"),
      ).toBeNull();
    });
  });

  it("跨端运行同步时按钮显示同步中且不可取消", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();

    render(
      <ChatView
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        projectId="proj-1"
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "chat-1" })).toBeTruthy();
    });
    fireEvent.click(screen.getByRole("button", { name: "chat-1" }));

    await waitFor(() => {
      expect(screen.getByText("Session ID: chat-1")).toBeTruthy();
    });

    wsHarness.emit({
      type: "run_started",
      project_id: "proj-1",
      data: { session_id: "chat-1" },
    });

    const syncButton = await screen.findByRole("button", { name: "同步中" });
    expect(syncButton.getAttribute("disabled")).not.toBeNull();
    fireEvent.click(syncButton);
    expect(apiClient.cancelChat).not.toHaveBeenCalled();

    wsHarness.emit({
      type: "run_completed",
      project_id: "proj-1",
      data: { session_id: "chat-1" },
    });

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "发送" })).toBeTruthy();
    });
  });

  it("createChat 返回 409 busy 时会自动刷新会话与运行事件", async () => {
    const apiClient = createMockApiClient();
    (apiClient.createChat as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ApiError(409, "chat session is already running", {
        code: "CHAT_SESSION_BUSY",
      }),
    );
    const wsHarness = createMockWsHarness();

    render(
      <ChatView
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        projectId="proj-1"
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "chat-1" })).toBeTruthy();
    });
    fireEvent.click(screen.getByRole("button", { name: "chat-1" }));
    await waitFor(() => {
      expect(screen.getByText("Session ID: chat-1")).toBeTruthy();
    });

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "继续执行" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送" }));

    await waitFor(() => {
      expect(apiClient.createChat).toHaveBeenCalledWith("proj-1", {
        message: "继续执行",
        session_id: "chat-1",
      });
    });
    await waitFor(() => {
      expect(
        screen.getByText("该会话正在其他终端运行，已自动同步最新状态。"),
      ).toBeTruthy();
    });
    await waitFor(() => {
      expect(apiClient.listChatRunEvents).toHaveBeenCalledWith(
        "proj-1",
        "chat-1",
        {
          limit: 50,
          cursor: undefined,
        },
      );
      expect(apiClient.listChats).toHaveBeenCalledWith("proj-1");
    });
  });

  it("忽略 started 之前或早于 started 时间戳的增量，避免串入上一轮内容", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();

    render(
      <ChatView
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        projectId="proj-1"
      />,
    );

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

    render(
      <ChatView
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        projectId="proj-1"
      />,
    );

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "第一轮" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(apiClient.createChat).toHaveBeenCalledWith("proj-1", expect.objectContaining({
        message: "第一轮",
      }));
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

    render(
      <ChatView
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        projectId="proj-1"
      />,
    );

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "请拆分任务" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(apiClient.createChat).toHaveBeenCalledWith("proj-1", expect.objectContaining({
        message: "请拆分任务",
      }));
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

    // 打开左侧文件树面板
    fireEvent.click(screen.getByTitle("展开仓库视图"));

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "从选中文件创建 issue" }),
      ).toBeTruthy();
    });

    // 通过 FileTree 选择文件
    fireEvent.click(screen.getByRole("button", { name: "选择 main.go" }));
    fireEvent.click(screen.getByRole("button", { name: "选择 task.go" }));

    fireEvent.click(screen.getByRole("button", { name: "从选中文件创建 issue" }));

    await waitFor(() => {
      expect(apiClient.createIssueFromFiles).toHaveBeenCalledWith("proj-1", {
        session_id: "chat-1",
        file_paths: ["cmd/app/main.go", "internal/core/task.go"],
      });
    });
    expect(screen.getByText("已从文件创建 issue：plan-files-1")).toBeTruthy();
  });

  it("支持展示会话列表并切换会话", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();

    vi.mocked(apiClient.listChatRunEvents).mockImplementation(
      async (_projectID: string, sessionID: string) => {
        if (sessionID === "chat-2") {
          return buildChatEventsPage({
            sessionId: "chat-2",
            updatedAt: "2026-03-01T11:02:00.000Z",
            messages: buildChatSession({
              id: "chat-2",
              userContent: "第二个会话提问",
              assistantContent: "第二个会话回复",
              createdAt: "2026-03-01T11:00:00.000Z",
              updatedAt: "2026-03-01T11:02:00.000Z",
            }).messages,
            events: [
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
            ],
          });
        }
        return buildChatEventsPage({
          messages: buildChatSession({
            id: "chat-1",
            assistantContent: "第一个会话回复",
          }).messages,
        });
      },
    );

    render(
      <ChatView
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        projectId="proj-1"
      />,
    );

    await waitFor(() => {
      expect(
        (apiClient as unknown as { listChats: ReturnType<typeof vi.fn> })
          .listChats,
      ).toHaveBeenCalledWith("proj-1");
    });

    fireEvent.click(screen.getByRole("button", { name: "chat-2" }));

    await waitFor(() => {
      expect(apiClient.listChatRunEvents).toHaveBeenCalledWith(
        "proj-1",
        "chat-2",
        {
          limit: 50,
          cursor: undefined,
        },
      );
      expect(screen.getAllByText("第二个会话回复").length).toBeGreaterThan(0);
      expect(screen.getAllByText(/Terminal/).length).toBeGreaterThanOrEqual(1);
    });
    expect(screen.getByText("Session ID: chat-2")).toBeTruthy();
  });

  it("会话消息区向上滚动时会使用 cursor 加载更早记录", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();

    vi.mocked(apiClient.listChatRunEvents).mockImplementation(
      async (
        _projectID: string,
        sessionID: string,
        query?: { cursor?: string; limit?: number },
      ) => {
        if (sessionID !== "chat-2") {
          return buildChatEventsPage();
        }
        if (query?.cursor === "cursor-older") {
          return buildChatEventsPage({
            sessionId: "chat-2",
            updatedAt: "2026-03-01T11:02:00.000Z",
            messages: [
              {
                role: "user",
                content: "更早的一条提问",
                time: "2026-03-01T10:59:00.000Z",
              },
            ],
            events: [],
            nextCursor: "",
          });
        }
        return buildChatEventsPage({
          sessionId: "chat-2",
          updatedAt: "2026-03-01T11:02:00.000Z",
          messages: buildChatSession({
            id: "chat-2",
            userContent: "第二个会话提问",
            assistantContent: "第二个会话回复",
            createdAt: "2026-03-01T11:00:00.000Z",
            updatedAt: "2026-03-01T11:02:00.000Z",
          }).messages,
          events: [],
          nextCursor: "cursor-older",
        });
      },
    );

    render(
      <ChatView
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        projectId="proj-1"
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "chat-2" })).toBeTruthy();
    });

    fireEvent.click(screen.getByRole("button", { name: "chat-2" }));

    await waitFor(() => {
      expect(screen.getByText("向上滚动可加载更早记录")).toBeTruthy();
      expect(
        screen.getAllByText("第二个会话回复").length,
      ).toBeGreaterThanOrEqual(1);
    });

    const scrollContainer = screen.getByText("向上滚动可加载更早记录")
      .parentElement as HTMLDivElement;
    Object.defineProperty(scrollContainer, "scrollTop", {
      configurable: true,
      value: 0,
      writable: true,
    });
    Object.defineProperty(scrollContainer, "scrollHeight", {
      configurable: true,
      value: 600,
      writable: true,
    });

    fireEvent.scroll(scrollContainer);

    await waitFor(() => {
      expect(apiClient.listChatRunEvents).toHaveBeenCalledWith(
        "proj-1",
        "chat-2",
        {
          limit: 50,
          cursor: "cursor-older",
        },
      );
      expect(
        screen.getAllByText("更早的一条提问").length,
      ).toBeGreaterThanOrEqual(1);
    });
  });

  it("跨分页返回的同一 tool_call 会继续合并为一张卡", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();

    vi.mocked(apiClient.listChatRunEvents).mockImplementation(
      async (
        _projectID: string,
        sessionID: string,
        query?: { cursor?: string; limit?: number },
      ) => {
        if (sessionID !== "chat-2") {
          return buildChatEventsPage();
        }
        if (query?.cursor === "cursor-tool-older") {
          return buildChatEventsPage({
            sessionId: "chat-2",
            updatedAt: "2026-03-01T11:02:00.000Z",
            messages: [],
            events: [
              {
                id: 20,
                session_id: "chat-2",
                project_id: "proj-1",
                event_type: "run_update",
                update_type: "tool_call",
                payload: {
                  session_id: "chat-2",
                  acp: {
                    sessionUpdate: "tool_call",
                    toolCallId: "call-merged",
                    title: "读取 README",
                    status: "pending",
                  },
                },
                created_at: "2026-03-01T11:00:30.000Z",
              },
            ],
            nextCursor: "",
          });
        }
        return buildChatEventsPage({
          sessionId: "chat-2",
          updatedAt: "2026-03-01T11:02:00.000Z",
          messages: buildChatSession({
            id: "chat-2",
            userContent: "第二个会话提问",
            assistantContent: "第二个会话回复",
            createdAt: "2026-03-01T11:00:00.000Z",
            updatedAt: "2026-03-01T11:02:00.000Z",
          }).messages,
          events: [
            {
              id: 21,
              session_id: "chat-2",
              project_id: "proj-1",
              event_type: "run_update",
              update_type: "tool_call_update",
              payload: {
                session_id: "chat-2",
                acp: {
                  sessionUpdate: "tool_call_update",
                  toolCallId: "call-merged",
                  status: "completed",
                  rawOutput: {
                    stdout: "done",
                  },
                },
              },
              created_at: "2026-03-01T11:01:30.000Z",
            },
          ],
          nextCursor: "cursor-tool-older",
        });
      },
    );

    render(
      <ChatView
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        projectId="proj-1"
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "chat-2" })).toBeTruthy();
    });

    fireEvent.click(screen.getByRole("button", { name: "chat-2" }));

    await waitFor(() => {
      expect(
        screen.getAllByRole("button", { name: "展开" }),
      ).toHaveLength(1);
    });

    const scrollContainer = screen.getByText("向上滚动可加载更早记录")
      .parentElement as HTMLDivElement;
    Object.defineProperty(scrollContainer, "scrollTop", {
      configurable: true,
      value: 0,
      writable: true,
    });
    Object.defineProperty(scrollContainer, "scrollHeight", {
      configurable: true,
      value: 600,
      writable: true,
    });

    fireEvent.scroll(scrollContainer);

    await waitFor(() => {
      expect(apiClient.listChatRunEvents).toHaveBeenCalledWith(
        "proj-1",
        "chat-2",
        {
          limit: 50,
          cursor: "cursor-tool-older",
        },
      );
      expect(
        screen.getAllByRole("button", { name: "展开" }),
      ).toHaveLength(1);
      expect(screen.getAllByText(/读取 README/).length).toBeGreaterThanOrEqual(
        1,
      );
    });
  });

  it("tool_call_group 展开时会按需请求 group 详情并显示子项", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();

    vi.mocked(apiClient.listChatRunEvents).mockResolvedValue(
      buildChatEventsPage({
        sessionId: "chat-2",
        updatedAt: "2026-03-01T11:02:00.000Z",
        messages: buildChatSession({
          id: "chat-2",
          userContent: "第二个会话提问",
          assistantContent: "第二个会话回复",
          createdAt: "2026-03-01T11:00:00.000Z",
          updatedAt: "2026-03-01T11:02:00.000Z",
        }).messages,
        events: [
          {
            id: 31,
            session_id: "chat-2",
            project_id: "proj-1",
            event_type: "run_update",
            update_type: "tool_call_group",
            payload: {
              session_id: "chat-2",
              group_id: "tool-call-group:30:31",
              item_count: 2,
              preview: "读取 README 等 2 个工具调用",
            },
            created_at: "2026-03-01T11:01:30.000Z",
          },
        ],
      }),
    );
    vi.mocked(apiClient.getChatEventGroup).mockResolvedValue({
      session_id: "chat-2",
      project_id: "proj-1",
      group_id: "tool-call-group:30:31",
      events: [
        {
          id: 30,
          session_id: "chat-2",
          project_id: "proj-1",
          event_type: "run_update",
          update_type: "tool_call",
          payload: {
            session_id: "chat-2",
            acp: {
              sessionUpdate: "tool_call",
              toolCallId: "call-a",
              title: "读取 README",
              status: "pending",
            },
          },
          created_at: "2026-03-01T11:01:00.000Z",
        },
        {
          id: 31,
          session_id: "chat-2",
          project_id: "proj-1",
          event_type: "run_update",
          update_type: "tool_call_update",
          payload: {
            session_id: "chat-2",
            acp: {
              sessionUpdate: "tool_call_update",
              toolCallId: "call-b",
              status: "completed",
              rawOutput: {
                stdout: "done",
              },
            },
          },
          created_at: "2026-03-01T11:01:30.000Z",
        },
      ],
    });

    render(
      <ChatView
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        projectId="proj-1"
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "chat-2" })).toBeTruthy();
    });

    fireEvent.click(screen.getByRole("button", { name: "chat-2" }));

    await waitFor(() => {
      expect(
        screen.getAllByRole("button", { name: "展开" }),
      ).toHaveLength(1);
    });

    fireEvent.click(screen.getByRole("button", { name: "展开" }));

    await waitFor(() => {
      expect(apiClient.getChatEventGroup).toHaveBeenCalledWith(
        "proj-1",
        "chat-2",
        "tool-call-group:30:31",
      );
      expect(screen.getAllByText(/读取 README/).length).toBeGreaterThanOrEqual(
        1,
      );
      expect(screen.getAllByText(/tool_call/).length).toBeGreaterThanOrEqual(1);
    });
  });

  it("可展示 tool_call/plan 等非文本运行事件", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();

    render(
      <ChatView
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        projectId="proj-1"
      />,
    );

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "执行任务" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(apiClient.createChat).toHaveBeenCalledWith("proj-1", expect.objectContaining({
        message: "执行任务",
      }));
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
      expect(screen.getAllByText(/读取文件/).length).toBeGreaterThanOrEqual(1);
      expect(screen.getAllByText(/步骤 1/).length).toBeGreaterThanOrEqual(1);
    });
  });

  it("agent_thought_chunk 会聚合显示，tool_call 也会进入聊天框", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();

    render(
      <ChatView
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        projectId="proj-1"
      />,
    );

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "执行并展示过程" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(apiClient.createChat).toHaveBeenCalledWith("proj-1", expect.objectContaining({
        message: "执行并展示过程",
      }));
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
          title: "读取 README",
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
          sessionUpdate: "agent_thought_chunk",
          content: {
            text: "thinking",
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
          sessionUpdate: "agent_thought_chunk",
          content: {
            text: " in detail",
          },
        },
      },
    });

    await waitFor(() => {
      expect(
        screen.getAllByText(/Thinking/).length,
      ).toBeGreaterThanOrEqual(1);
      expect(
        screen.getAllByText("thinking in detail").length,
      ).toBeGreaterThanOrEqual(1);
      expect(screen.getAllByText(/读取 README/).length).toBeGreaterThanOrEqual(
        1,
      );
    });
  });

  it("tool_call 在聊天框默认收起，展开后可再次收起", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();

    render(
      <ChatView
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        projectId="proj-1"
      />,
    );

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "执行超长 tool call" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(apiClient.createChat).toHaveBeenCalledWith("proj-1", expect.objectContaining({
        message: "执行超长 tool call",
      }));
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
          toolCallId: "call-1",
          title: "长输出命令",
          kind: "execute",
          status: "pending",
        },
      },
    });
    wsHarness.emit({
      type: "run_update",
      project_id: "proj-1",
      data: {
        session_id: "chat-1",
        acp: {
          sessionUpdate: "tool_call_update",
          toolCallId: "call-1",
          status: "completed",
          rawOutput: {
            stdout: "line-1 ".repeat(80),
          },
        },
      },
    });

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "展开" })).toBeTruthy();
    });

    // tool_call 默认收起，rawOutput 不可见
    expect(screen.queryByText(/rawOutput:/)).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "展开" }));

    await waitFor(() => {
      expect(
        screen.getAllByRole("button", { name: "收起" }).length,
      ).toBeGreaterThanOrEqual(1);
      expect(screen.getByText(/rawOutput:/)).toBeTruthy();
    });

    fireEvent.click(
      screen.getAllByRole("button", { name: "收起" })[0]!,
    );

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "展开" })).toBeTruthy();
    });
  });

  it("历史缺失正文的 agent_thought_chunk 不应污染后续 thought 文本", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();

    vi.mocked(apiClient.listChatRunEvents).mockImplementation(
      async (_projectID: string, sessionID: string) => {
        if (sessionID === "chat-2") {
          return buildChatEventsPage({
            sessionId: "chat-2",
            updatedAt: "2026-03-01T11:02:00.000Z",
            messages: buildChatSession({
              id: "chat-2",
              userContent: "继续执行",
              assistantContent: "开始处理中",
              createdAt: "2026-03-01T11:00:00.000Z",
              updatedAt: "2026-03-01T11:02:00.000Z",
            }).messages,
            events: [
              {
                id: 12,
                session_id: "chat-2",
                project_id: "proj-1",
                event_type: "run_update",
                update_type: "agent_thought_chunk",
                payload: {
                  session_id: "chat-2",
                },
                created_at: "2026-03-01T11:01:00.000Z",
              },
            ],
          });
        }
        return buildChatEventsPage();
      },
    );

    render(
      <ChatView
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        projectId="proj-1"
      />,
    );

    await waitFor(() => {
      expect(
        (apiClient as unknown as { listChats: ReturnType<typeof vi.fn> })
          .listChats,
      ).toHaveBeenCalledWith("proj-1");
    });

    fireEvent.click(screen.getByRole("button", { name: "chat-2" }));

    await waitFor(() => {
      expect(apiClient.listChatRunEvents).toHaveBeenCalledWith(
        "proj-1",
        "chat-2",
        {
          limit: 50,
          cursor: undefined,
        },
      );
    });

    wsHarness.emit({
      type: "run_started",
      project_id: "proj-1",
      data: { session_id: "chat-2" },
    });
    wsHarness.emit({
      type: "run_update",
      project_id: "proj-1",
      data: {
        session_id: "chat-2",
        acp: {
          sessionUpdate: "agent_thought_chunk",
          content: {
            text: "The user wants to debug streaming rendering.",
          },
        },
      },
    });

    await waitFor(() => {
      expect(
        screen.getAllByText("The user wants to debug streaming rendering.")
          .length,
      ).toBeGreaterThanOrEqual(1);
    });

    expect(
      screen.queryByText(
        /agent_thought_chunkThe user wants to debug streaming rendering\./,
      ),
    ).toBeNull();
    expect(screen.queryByText(/^agent_thought_chunk$/)).toBeNull();
  });

  it("右侧运行事件列表会截断超长 tool_call 详情", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();

    render(
      <ChatView
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        projectId="proj-1"
      />,
    );

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "执行侧栏截断" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(apiClient.createChat).toHaveBeenCalledWith("proj-1", expect.objectContaining({
        message: "执行侧栏截断",
      }));
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
          toolCallId: "call-sidebar",
          title: "超长侧栏命令",
          status: "pending",
        },
      },
    });
    wsHarness.emit({
      type: "run_update",
      project_id: "proj-1",
      data: {
        session_id: "chat-1",
        acp: {
          sessionUpdate: "tool_call_update",
          toolCallId: "call-sidebar",
          status: "completed",
          rawOutput: {
            stdout: "side-output ".repeat(80),
          },
        },
      },
    });

    await waitFor(() => {
      expect(screen.getAllByText(/超长侧栏命令/).length).toBeGreaterThanOrEqual(1);
    });
  });

  it("助手消息支持基础 Markdown 渲染", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();
    vi.mocked(apiClient.listChatRunEvents).mockResolvedValue(
      buildChatEventsPage({
        messages: buildChatSession({
          assistantContent:
            "# 计划概览\n- [文档](https://example.com)\n- `run test`",
        }).messages,
      }),
    );

    render(
      <ChatView
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        projectId="proj-1"
      />,
    );

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "触发会话" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(apiClient.createChat).toHaveBeenCalledWith("proj-1", expect.objectContaining({
        message: "触发会话",
      }));
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
    const link = screen.getByRole("link", {
      name: "文档",
    }) as HTMLAnchorElement;
    expect(link.href).toBe("https://example.com/");
    expect(screen.getByText("run test")).toBeTruthy();
  });

  it("文件树选择后已选文件数显示在创建按钮上方", () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();
    render(
      <ChatView
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        projectId="proj-1"
      />,
    );

    fireEvent.click(screen.getByTitle("展开仓库视图"));

    // 未选择时无已选提示
    expect(screen.queryByText(/已选 \d+ 个文件/)).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "选择 main.go" }));
    expect(screen.getByText("已选 1 个文件")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "选择 task.go" }));
    expect(screen.getByText("已选 2 个文件")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "取消 main.go" }));
    expect(screen.getByText("已选 1 个文件")).toBeTruthy();
  });

  it("左侧面板支持在文件树与 Git Status 之间切换", () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();
    render(
      <ChatView
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        projectId="proj-1"
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "文件" }));
    expect(screen.getByText("FileTreeMock")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Git Status" }));
    expect(screen.getByText("GitStatusPanelMock")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "文件树" }));
    expect(screen.getByText("FileTreeMock")).toBeTruthy();
  });

  it("available_commands_update 会展示命令面板并回填 slash command", async () => {
    const apiClient = createMockApiClient();
    const wsHarness = createMockWsHarness();

    render(
      <ChatView
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        projectId="proj-1"
      />,
    );

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "请先创建会话" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(apiClient.getSessionCommands).toHaveBeenCalledWith(
        "proj-1",
        "chat-1",
      );
    });

    wsHarness.emit({
      type: "run_update",
      project_id: "proj-1",
      data: {
        session_id: "chat-1",
        acp: {
          sessionUpdate: "available_commands_update",
          availableCommands: [
            {
              name: "review",
              description: "Review current changes",
              input: {
                hint: "optional custom review instructions",
              },
            },
          ],
        },
      },
    });

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "/" },
    });

    await waitFor(() => {
      expect(screen.getByText("/review")).toBeTruthy();
    });

    const reviewButton = screen.getByText("/review").closest("button");
    expect(reviewButton).not.toBeNull();
    fireEvent.click(reviewButton!);

    expect(
      (screen.getByLabelText("新消息") as HTMLTextAreaElement).value,
    ).toBe("/review [optional custom review instructions]");
  });

  it("创建会话后会加载 config options，并支持切换当前值", async () => {
    const apiClient = createMockApiClient();
    apiClient.getSessionConfigOptions = vi.fn().mockResolvedValue([
      {
        id: "model",
        name: "Model",
        type: "select",
        currentValue: "model-1",
        options: [
          { value: "model-1", name: "Model 1" },
          { value: "model-2", name: "Model 2" },
        ],
      },
    ]);
    apiClient.setSessionConfigOption = vi.fn().mockResolvedValue([
      {
        id: "model",
        name: "Model",
        type: "select",
        currentValue: "model-2",
        options: [
          { value: "model-1", name: "Model 1" },
          { value: "model-2", name: "Model 2" },
        ],
      },
    ]);
    const wsHarness = createMockWsHarness();

    render(
      <ChatView
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        projectId="proj-1"
      />,
    );

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "请先创建会话" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(apiClient.getSessionConfigOptions).toHaveBeenCalledWith(
        "proj-1",
        "chat-1",
      );
    });

    const select = (await screen.findByLabelText("Model")) as HTMLSelectElement;
    expect(select.value).toBe("model-1");

    fireEvent.change(select, {
      target: { value: "model-2" },
    });

    await waitFor(() => {
      expect(apiClient.setSessionConfigOption).toHaveBeenCalledWith(
        "proj-1",
        "chat-1",
        "model",
        "model-2",
      );
    });
    expect((screen.getByLabelText("Model") as HTMLSelectElement).value).toBe(
      "model-2",
    );
  });
});
