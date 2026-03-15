// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { I18nextProvider } from "react-i18next";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import i18n from "../i18n";
import { ExecutionDetailPage } from "./ExecutionDetailPage";

const { mockUseWorkbench } = vi.hoisted(() => ({
  mockUseWorkbench: vi.fn(),
}));

vi.mock("@/contexts/WorkbenchContext", () => ({
  useWorkbench: mockUseWorkbench,
}));

function renderPage(initialEntry = "/executions/77") {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <Routes>
          <Route path="/executions/:execId" element={<ExecutionDetailPage />} />
        </Routes>
      </MemoryRouter>
    </I18nextProvider>,
  );
}

describe("ExecutionDetailPage", () => {
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

  it("展示执行详情、运行结果资源与事件日志", async () => {
    const apiClient = {
      getRun: vi.fn().mockResolvedValue({
        id: 77,
        action_id: 12,
        work_item_id: 42,
        attempt: 2,
        status: "running",
        started_at: "2026-03-15T00:00:00Z",
        finished_at: null,
        agent_id: "agent-1",
        result_markdown: "已提交补丁",
        created_at: "2026-03-15T00:10:00Z",
      }),
      getAction: vi.fn().mockResolvedValue({
        id: 12,
        work_item_id: 42,
        name: "实现支付重试",
        description: "补充支付超时重试",
        type: "exec",
        agent_role: "developer",
        acceptance_criteria: ["支付重试可配置"],
      }),
      getWorkItem: vi.fn().mockResolvedValue({
        id: 42,
        title: "支付链路优化",
      }),
      listRunResources: vi.fn().mockResolvedValue([
        {
          id: 9,
          run_id: 77,
          file_name: "patch.diff",
          created_at: "2026-03-15T00:10:00Z",
        },
      ]),
      listEvents: vi.fn().mockResolvedValue([
        {
          type: "exec.agent_output",
          timestamp: "2026-03-15T00:01:00Z",
          data: { type: "agent_thought", content: "先看日志" },
        },
        {
          type: "exec.agent_output",
          timestamp: "2026-03-15T00:02:00Z",
          data: { type: "tool_call", content: "rg -n payment" },
        },
        {
          type: "chat.output",
          timestamp: "2026-03-15T00:03:00Z",
          data: { type: "tool_call_completed", content: "命令执行完成" },
        },
        {
          type: "exec.status",
          timestamp: "2026-03-15T00:04:00Z",
          data: { content: "done" },
        },
      ]),
    };

    mockUseWorkbench.mockReturnValue({ apiClient });

    renderPage();

    expect(await screen.findByText("执行详情")).toBeTruthy();
    expect(screen.getByText("实现支付重试")).toBeTruthy();
    expect(screen.getByText("支付链路优化")).toBeTruthy();
    expect(screen.getByText("已提交补丁")).toBeTruthy();
    expect(screen.getByText("patch.diff")).toBeTruthy();
    expect(screen.getByText("支付重试可配置")).toBeTruthy();
    expect(screen.getByText("先看日志")).toBeTruthy();
    expect(screen.getByText("rg -n payment")).toBeTruthy();
    expect(screen.getByText("命令执行完成")).toBeTruthy();
    expect(screen.getByText("exec.status: done")).toBeTruthy();
  });

  it("加载失败时展示错误信息", async () => {
    const apiClient = {
      getRun: vi.fn().mockRejectedValue(new Error("execution unavailable")),
      getAction: vi.fn(),
      getWorkItem: vi.fn(),
      listRunResources: vi.fn(),
      listEvents: vi.fn(),
    };

    mockUseWorkbench.mockReturnValue({ apiClient });

    renderPage();

    expect(await screen.findByText("execution unavailable")).toBeTruthy();
  });
});
