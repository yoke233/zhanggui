/** @vitest-environment jsdom */

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import A2AChatView from "./A2AChatView";
import type { A2AClient } from "../lib/a2aClient";

const createMockA2AClient = (): A2AClient => {
  return {
    sendMessage: vi.fn().mockResolvedValue({
      id: "task-1",
      contextId: "chat-1",
      status: {
        state: "working",
      },
    }),
    getTask: vi.fn(),
    cancelTask: vi.fn().mockResolvedValue({
      id: "task-1",
      contextId: "chat-1",
      status: {
        state: "canceled",
      },
    }),
    streamMessage: vi.fn().mockResolvedValue([]),
  };
};

describe("A2AChatView", () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("发送消息会调用 a2aClient.sendMessage 并展示 task 信息", async () => {
    const a2aClient = createMockA2AClient();
    render(<A2AChatView a2aClient={a2aClient} projectId="proj-1" />);

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "hello a2a" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(a2aClient.sendMessage).toHaveBeenCalledWith({
        message: {
          role: "user",
          parts: [{ kind: "text", text: "hello a2a" }],
        },
        metadata: {
          project_id: "proj-1",
        },
      });
    });

    await waitFor(() => {
      expect(screen.getByText("Session ID: chat-1")).toBeTruthy();
      expect(screen.getByText("A2A Task ID: task-1")).toBeTruthy();
      expect(screen.getByText("A2A Task State: working")).toBeTruthy();
    });
  });

  it("运行中点击停止会调用 cancelTask", async () => {
    const a2aClient = createMockA2AClient();
    render(<A2AChatView a2aClient={a2aClient} projectId="proj-1" />);

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "cancel me" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "停止" })).toBeTruthy();
    });

    fireEvent.click(screen.getByRole("button", { name: "停止" }));

    await waitFor(() => {
      expect(a2aClient.cancelTask).toHaveBeenCalledWith({
        id: "task-1",
        metadata: {
          project_id: "proj-1",
        },
      });
      expect(screen.getByText("A2A Task State: canceled")).toBeTruthy();
      expect(screen.getByText("当前请求已取消")).toBeTruthy();
    });
  });

  it("A2A 错误会反馈给用户", async () => {
    const a2aClient = createMockA2AClient();
    (a2aClient.sendMessage as unknown as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new Error("a2a send failed"),
    );

    render(<A2AChatView a2aClient={a2aClient} projectId="proj-1" />);

    fireEvent.change(screen.getByLabelText("新消息"), {
      target: { value: "will fail" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送并创建会话" }));

    await waitFor(() => {
      expect(screen.getByText("a2a send failed")).toBeTruthy();
    });
  });
});
