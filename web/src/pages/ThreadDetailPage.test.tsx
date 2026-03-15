// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nextProvider } from "react-i18next";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import i18n from "../i18n";
import { ThreadDetailPage } from "./ThreadDetailPage";

const DEFAULT_MESSAGE_PLACEHOLDER = "Type @ to mention an agent, # to reference a file...";

const { mockUseWorkbench } = vi.hoisted(() => ({
  mockUseWorkbench: vi.fn(),
}));

vi.mock("@/contexts/WorkbenchContext", () => ({
  useWorkbench: mockUseWorkbench,
}));

function buildThread(summary = "已有摘要", metadata?: Record<string, unknown>) {
  return {
    id: 1,
    title: "讨论线程",
    status: "active",
    summary,
    metadata,
    created_at: "2026-03-13T00:00:00Z",
    updated_at: "2026-03-13T00:00:00Z",
  };
}

function buildProfile(id: string, role = "worker") {
  return {
    id,
    name: id,
    driver_id: "codex-cli",
    role,
    capabilities: [],
    actions_allowed: [],
  };
}

function buildAgentSession(id: number, profileID: string, status = "active") {
  return {
    id,
    thread_id: 1,
    agent_profile_id: profileID,
    acp_session_id: `acp-${id}`,
    status,
    turn_count: 0,
    total_input_tokens: 0,
    total_output_tokens: 0,
    joined_at: "2026-03-13T00:00:00Z",
    last_active_at: "2026-03-13T00:00:00Z",
  };
}

function createWsClientMock() {
  const subscriptions = new Map<string, Array<(payload: unknown) => void>>();
  const statusHandlers: Array<(status: "idle" | "connecting" | "open" | "closed") => void> = [];

  return {
    send: vi.fn(),
    getStatus: vi.fn(() => "open"),
    subscribe: vi.fn((type: string, handler: (payload: unknown) => void) => {
      const handlers = subscriptions.get(type) ?? [];
      handlers.push(handler);
      subscriptions.set(type, handlers);
      return () => {
        const current = subscriptions.get(type) ?? [];
        subscriptions.set(type, current.filter((item) => item !== handler));
      };
    }),
    onStatusChange: vi.fn((handler: (status: "idle" | "connecting" | "open" | "closed") => void) => {
      statusHandlers.push(handler);
      return () => {
        const idx = statusHandlers.indexOf(handler);
        if (idx >= 0) {
          statusHandlers.splice(idx, 1);
        }
      };
    }),
    emit(type: string, payload: unknown) {
      for (const handler of subscriptions.get(type) ?? []) {
        handler(payload);
      }
    },
    emitStatus(status: "idle" | "connecting" | "open" | "closed") {
      for (const handler of statusHandlers) {
        handler(status);
      }
    },
  };
}

function renderPage() {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter initialEntries={["/threads/1"]}>
        <Routes>
          <Route path="/threads/:threadId" element={<ThreadDetailPage />} />
        </Routes>
      </MemoryRouter>
    </I18nextProvider>,
  );
}


describe("ThreadDetailPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    void i18n.changeLanguage("zh-CN");
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
      configurable: true,
      value: vi.fn(),
    });
  });

  afterEach(() => {
    cleanup();
  });


  it("进入页面订阅 thread，并通过 thread.send + 实时事件更新消息列表", async () => {
    const wsClient = createWsClientMock();
    const apiClient = {
      getThread: vi.fn().mockResolvedValue(buildThread("已有摘要")),
      listThreadMessages: vi.fn().mockResolvedValue([]),
      listThreadParticipants: vi.fn().mockResolvedValue([]),
      listWorkItemsByThread: vi.fn().mockResolvedValue([]),
      listThreadTaskGroups: vi.fn().mockResolvedValue([]),
      listThreadAgents: vi.fn().mockResolvedValue([]),
      listThreadAttachments: vi.fn().mockResolvedValue([]),
      listProfiles: vi.fn().mockResolvedValue([buildProfile("worker-a")]),
    };
    mockUseWorkbench.mockReturnValue({ apiClient, wsClient });

    renderPage();

    await waitFor(() => {
      expect(wsClient.send).toHaveBeenCalledWith({
        type: "subscribe_thread",
        data: { thread_id: 1 },
      });
    });

    const input = screen.getByPlaceholderText(DEFAULT_MESSAGE_PLACEHOLDER);
    fireEvent.change(input, { target: { value: "实时消息" } });
    fireEvent.keyDown(input, { key: "Enter" });

    expect(wsClient.send).toHaveBeenCalledWith(
      expect.objectContaining({
        type: "thread.send",
        data: expect.objectContaining({
          thread_id: 1,
          message: "实时消息",
        }),
      }),
    );

    const sendCall = wsClient.send.mock.calls.find((call) => call[0]?.type === "thread.send");
    const requestId = sendCall?.[0]?.data?.request_id;
    wsClient.emit("thread.ack", {
      request_id: requestId,
      thread_id: 1,
      status: "accepted",
    });
    wsClient.emit("thread.message", {
      thread_id: 1,
      message: "实时消息",
      sender_id: "human",
      role: "human",
    });
    wsClient.emit("thread.agent_output", {
      thread_id: 1,
      content: "agent reply",
      profile_id: "worker-a",
    });

    expect(await screen.findByText("实时消息")).toBeTruthy();
    expect(await screen.findByText("agent reply")).toBeTruthy();
  });

  it("支持邀请和移除 thread agent", async () => {
    const wsClient = createWsClientMock();
    const apiClient = {
      getThread: vi.fn().mockResolvedValue(buildThread("已有摘要")),
      listThreadMessages: vi.fn().mockResolvedValue([]),
      listThreadParticipants: vi.fn().mockResolvedValue([]),
      listWorkItemsByThread: vi.fn().mockResolvedValue([]),
      listThreadTaskGroups: vi.fn().mockResolvedValue([]),
      listThreadAgents: vi.fn()
        .mockResolvedValueOnce([])
        .mockResolvedValueOnce([buildAgentSession(11, "worker-a")])
        .mockResolvedValueOnce([]),
      listThreadAttachments: vi.fn().mockResolvedValue([]),
      listProfiles: vi.fn().mockResolvedValue([buildProfile("worker-a"), buildProfile("worker-b")]),
      inviteThreadAgent: vi.fn().mockResolvedValue(buildAgentSession(11, "worker-a", "joining")),
      removeThreadAgent: vi.fn().mockResolvedValue(undefined),
    };
    mockUseWorkbench.mockReturnValue({ apiClient, wsClient });

    renderPage();

    // Click "Add Agent" to reveal the invite picker, then select the agent.
    fireEvent.click(await screen.findByText("Add Agent"));
    fireEvent.click(await screen.findByText("worker-a"));
    fireEvent.click(await screen.findByText(/Add \(1\)/));

    await waitFor(() => {
      expect(apiClient.inviteThreadAgent).toHaveBeenCalledWith(1, { agent_profile_id: "worker-a" });
    });
    expect(await screen.findByText("worker-a")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Remove worker-a" }));

    await waitFor(() => {
      expect(apiClient.removeThreadAgent).toHaveBeenCalledWith(1, 11);
    });
    await waitFor(() => {
      expect(screen.queryByTestId("agent-card-worker-a")).toBeNull();
    });
  });

  it("支持用 @agent-id 定向发送消息", async () => {
    const wsClient = createWsClientMock();
    const apiClient = {
      getThread: vi.fn().mockResolvedValue(buildThread("已有摘要")),
      listThreadMessages: vi.fn().mockResolvedValue([]),
      listThreadParticipants: vi.fn().mockResolvedValue([]),
      listWorkItemsByThread: vi.fn().mockResolvedValue([]),
      listThreadTaskGroups: vi.fn().mockResolvedValue([]),
      listThreadAgents: vi.fn().mockResolvedValue([buildAgentSession(11, "worker-a")]),
      listThreadAttachments: vi.fn().mockResolvedValue([]),
      listProfiles: vi.fn().mockResolvedValue([buildProfile("worker-a")]),
    };
    mockUseWorkbench.mockReturnValue({ apiClient, wsClient });

    renderPage();

    await screen.findByText("worker-a");

    const input = screen.getByPlaceholderText(DEFAULT_MESSAGE_PLACEHOLDER);
    fireEvent.change(input, { target: { value: "@worker-a 请处理这个问题" } });
    fireEvent.keyDown(input, { key: "Enter" });

    expect(wsClient.send).toHaveBeenCalledWith(
      expect.objectContaining({
        type: "thread.send",
        data: expect.objectContaining({
          thread_id: 1,
          message: "@worker-a 请处理这个问题",
          target_agent_id: "worker-a",
        }),
      }),
    );

    const sendCall = wsClient.send.mock.calls.find((call) => call[0]?.type === "thread.send");
    const requestId = sendCall?.[0]?.data?.request_id;
    wsClient.emit("thread.ack", {
      request_id: requestId,
      thread_id: 1,
      status: "accepted",
    });
    wsClient.emit("thread.message", {
      thread_id: 1,
      message: "@worker-a 请处理这个问题",
      sender_id: "human",
      role: "human",
      target_agent_id: "worker-a",
    });

    expect(await screen.findByRole("button", { name: "@worker-a" })).toBeTruthy();
    expect(await screen.findByText("请处理这个问题")).toBeTruthy();
  });

  it("默认仅 @ 激活，普通消息不会携带 target_agent_id", async () => {
    const wsClient = createWsClientMock();
    const apiClient = {
      getThread: vi.fn().mockResolvedValue(buildThread("已有摘要")),
      listThreadMessages: vi.fn().mockResolvedValue([]),
      listThreadParticipants: vi.fn().mockResolvedValue([]),
      listWorkItemsByThread: vi.fn().mockResolvedValue([]),
      listThreadTaskGroups: vi.fn().mockResolvedValue([]),
      listThreadAgents: vi.fn().mockResolvedValue([buildAgentSession(11, "worker-a")]),
      listThreadAttachments: vi.fn().mockResolvedValue([]),
      listProfiles: vi.fn().mockResolvedValue([buildProfile("worker-a")]),
    };
    mockUseWorkbench.mockReturnValue({ apiClient, wsClient });

    renderPage();

    await screen.findByText("Mention-only mode: use @agent-id to direct messages to specific agents.");

    const input = screen.getByPlaceholderText(DEFAULT_MESSAGE_PLACEHOLDER);
    fireEvent.change(input, { target: { value: "普通讨论消息" } });
    fireEvent.keyDown(input, { key: "Enter" });

    expect(wsClient.send).toHaveBeenCalledWith(
      expect.objectContaining({
        type: "thread.send",
        data: expect.objectContaining({
          thread_id: 1,
          message: "普通讨论消息",
          target_agent_id: undefined,
        }),
      }),
    );
  });

  it("支持切换到广播模式", async () => {
    const wsClient = createWsClientMock();
    const apiClient = {
      getThread: vi.fn().mockResolvedValue(buildThread("已有摘要")),
      listThreadMessages: vi.fn().mockResolvedValue([]),
      listThreadParticipants: vi.fn().mockResolvedValue([]),
      listWorkItemsByThread: vi.fn().mockResolvedValue([]),
      listThreadTaskGroups: vi.fn().mockResolvedValue([]),
      listThreadAgents: vi.fn().mockResolvedValue([buildAgentSession(11, "worker-a")]),
      listThreadAttachments: vi.fn().mockResolvedValue([]),
      listProfiles: vi.fn().mockResolvedValue([buildProfile("worker-a")]),
      updateThread: vi.fn().mockResolvedValue(buildThread("已有摘要", { agent_routing_mode: "broadcast" })),
    };
    mockUseWorkbench.mockReturnValue({ apiClient, wsClient });

    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "Broadcast" }));

    await waitFor(() => {
      expect(apiClient.updateThread).toHaveBeenCalledWith(1, {
        metadata: { agent_routing_mode: "broadcast" },
      });
    });
    expect(await screen.findByText("Broadcast mode: messages go to all active agents. Use @agent-id for targeting.")).toBeTruthy();
  });

  it("输入 @ 时展示候选并插入 mention", async () => {
    const wsClient = createWsClientMock();
    const apiClient = {
      getThread: vi.fn().mockResolvedValue(buildThread("已有摘要")),
      listThreadMessages: vi.fn().mockResolvedValue([]),
      listThreadParticipants: vi.fn().mockResolvedValue([]),
      listWorkItemsByThread: vi.fn().mockResolvedValue([]),
      listThreadTaskGroups: vi.fn().mockResolvedValue([]),
      listThreadAgents: vi.fn().mockResolvedValue([buildAgentSession(11, "worker-a"), buildAgentSession(12, "worker-b")]),
      listThreadAttachments: vi.fn().mockResolvedValue([]),
      listProfiles: vi.fn().mockResolvedValue([buildProfile("worker-a"), buildProfile("worker-b")]),
    };
    mockUseWorkbench.mockReturnValue({ apiClient, wsClient });

    renderPage();

    const input = await screen.findByPlaceholderText(DEFAULT_MESSAGE_PLACEHOLDER);
    fireEvent.change(input, { target: { value: "@wor", selectionStart: 4 } });

    expect(await screen.findByRole("button", { name: /@worker-a/ })).toBeTruthy();

    fireEvent.mouseDown(screen.getByRole("button", { name: /@worker-a/ }));

    expect((input as HTMLInputElement).value).toBe("@worker-a ");
    expect(await screen.findByRole("button", { name: "@worker-a" })).toBeTruthy();
  });

  it("点击消息里的 mention 会高亮对应 agent 卡片", async () => {
    const wsClient = createWsClientMock();
    const apiClient = {
      getThread: vi.fn().mockResolvedValue(buildThread("已有摘要")),
      listThreadMessages: vi.fn().mockResolvedValue([
        {
          id: 101,
          thread_id: 1,
          sender_id: "human",
          role: "human",
          content: "@worker-a 请处理这个问题",
          metadata: { target_agent_id: "worker-a" },
          created_at: "2026-03-13T00:00:00Z",
        },
      ]),
      listThreadParticipants: vi.fn().mockResolvedValue([]),
      listWorkItemsByThread: vi.fn().mockResolvedValue([]),
      listThreadTaskGroups: vi.fn().mockResolvedValue([]),
      listThreadAgents: vi.fn().mockResolvedValue([buildAgentSession(11, "worker-a")]),
      listThreadAttachments: vi.fn().mockResolvedValue([]),
      listProfiles: vi.fn().mockResolvedValue([buildProfile("worker-a")]),
    };
    mockUseWorkbench.mockReturnValue({ apiClient, wsClient });

    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "@worker-a" }));

    await waitFor(() => {
      expect(screen.getByTestId("agent-card-worker-a").className).toContain("border-blue-300");
    });
  });

  it("展示消息归属的 Task Group 标记", async () => {
    const wsClient = createWsClientMock();
    const apiClient = {
      getThread: vi.fn().mockResolvedValue(buildThread("已有摘要")),
      listThreadMessages: vi.fn().mockResolvedValue([
        {
          id: 103,
          thread_id: 1,
          sender_id: "system",
          role: "system",
          content: "任务轨道已进入送审。",
          metadata: { task_group_id: 42 },
          created_at: "2026-03-13T00:00:00Z",
        },
      ]),
      listThreadParticipants: vi.fn().mockResolvedValue([]),
      listWorkItemsByThread: vi.fn().mockResolvedValue([]),
      listThreadTaskGroups: vi.fn().mockResolvedValue([]),
      listThreadAgents: vi.fn().mockResolvedValue([]),
      listThreadAttachments: vi.fn().mockResolvedValue([]),
      listProfiles: vi.fn().mockResolvedValue([buildProfile("worker-a")]),
    };
    mockUseWorkbench.mockReturnValue({ apiClient, wsClient });

    renderPage();

    expect(await screen.findByText("Group #42")).toBeTruthy();
    expect(await screen.findByText("任务轨道已进入送审。")).toBeTruthy();
  });

  it("hover 消息 mention 时展示 agent 信息卡", async () => {
    const wsClient = createWsClientMock();
    const apiClient = {
      getThread: vi.fn().mockResolvedValue(buildThread("已有摘要")),
      listThreadMessages: vi.fn().mockResolvedValue([
        {
          id: 102,
          thread_id: 1,
          sender_id: "human",
          role: "human",
          content: "@worker-a 请处理 hover",
          metadata: { target_agent_id: "worker-a" },
          created_at: "2026-03-13T00:00:00Z",
        },
      ]),
      listThreadParticipants: vi.fn().mockResolvedValue([]),
      listWorkItemsByThread: vi.fn().mockResolvedValue([]),
      listThreadTaskGroups: vi.fn().mockResolvedValue([]),
      listThreadAgents: vi.fn().mockResolvedValue([buildAgentSession(11, "worker-a")]),
      listThreadAttachments: vi.fn().mockResolvedValue([]),
      listProfiles: vi.fn().mockResolvedValue([buildProfile("worker-a")]),
    };
    mockUseWorkbench.mockReturnValue({ apiClient, wsClient });

    renderPage();

    fireEvent.mouseEnter(await screen.findByRole("button", { name: "@worker-a" }));

    const hoverCard = await screen.findByTestId("mention-hover-card-worker-a");
    expect(hoverCard.textContent).toContain("Turns: 0");
    expect(hoverCard.textContent).toContain("0.0k tokens");
  });
});
