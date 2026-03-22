// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { I18nextProvider } from "react-i18next";
import type React from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import i18n from "@/i18n";
import { ChatInputBar } from "./ChatInputBar";

function renderBar(props?: Partial<React.ComponentProps<typeof ChatInputBar>>) {
  const fileClick = vi.fn();
  const baseProps: React.ComponentProps<typeof ChatInputBar> = {
    messageInput: "/dep",
    pendingFiles: [new File(["hello"], "spec.md", { type: "text/markdown" })],
    currentSession: {
      session_id: "session-1",
      title: "发布流程",
      project_name: "Alpha",
      status: "alive",
      branch: "feature/release",
      message_count: 4,
      updated_at: "2026-03-14T00:00:00Z",
      created_at: "2026-03-14T00:00:00Z",
      project_id: 1,
      profile_id: "lead-1",
      profile_name: "Lead",
      driver_id: "codex-cli",
    },
    submitting: false,
    draftSessionReady: true,
    currentDriverLabel: "Codex",
    currentProjectLabel: "Alpha",
    showCommandPalette: true,
    availableCommands: [
      { name: "deploy", description: "执行部署" },
      { name: "debug", description: "排查问题" },
    ],
    commandFilter: "dep",
    fileInputRef: { current: { click: fileClick } as unknown as HTMLInputElement },
    modes: {
      current_mode_id: "auto",
      available_modes: [
        { id: "auto", name: "自动", description: "自动模式" },
        { id: "review", name: "审阅", description: "审阅模式" },
      ],
    },
    configOptions: [
      {
        id: "risk",
        name: "风险级别",
        type: "select",
        current_value: "high",
        options: [
          { value: "high", name: "高" },
          { value: "low", name: "低" },
        ],
      },
    ],
    onMessageChange: vi.fn(),
    onPaste: vi.fn(),
    onKeyDown: vi.fn(),
    sessionRunning: false,
    onSend: vi.fn(),
    onCancel: vi.fn(),
    onCommandSelect: vi.fn(),
    onRemovePendingFile: vi.fn(),
    onCommandPaletteClose: vi.fn(),
    onSetMode: vi.fn(),
    onSetConfigOption: vi.fn(),
    pendingMessage: null,
    onCancelPending: vi.fn(),
  };

  const mergedProps = { ...baseProps, ...props };
  const result = render(
    <I18nextProvider i18n={i18n}>
      <ChatInputBar {...mergedProps} />
    </I18nextProvider>,
  );

  return { ...result, props: mergedProps, fileClick };
}

describe("ChatInputBar", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    void i18n.changeLanguage("zh-CN");
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it("支持命令选择、模式切换、配置变更和移除待上传文件", async () => {
    vi.useFakeTimers();
    const { props, fileClick } = renderBar();

    fireEvent.click(screen.getByRole("button", { name: /deploy/i }));
    expect(props.onCommandSelect).toHaveBeenCalledWith("deploy");

    fireEvent.click(screen.getByRole("button", { name: "审阅" }));
    expect(props.onSetMode).toHaveBeenCalledWith("review");

    fireEvent.click(screen.getByRole("button", { name: "高" }));
    fireEvent.click(screen.getByRole("button", { name: "低" }));
    expect(props.onSetConfigOption).toHaveBeenCalledWith("risk", "low");

    const pendingBadge = screen.getByText("spec.md").parentElement as HTMLElement;
    fireEvent.click(within(pendingBadge).getByRole("button"));
    expect(props.onRemovePendingFile).toHaveBeenCalledWith(0);

    fireEvent.click(screen.getByTitle("上传文件或图片"));
    expect(fileClick).toHaveBeenCalledTimes(1);

    fireEvent.blur(screen.getByRole("textbox"));
    await vi.advanceTimersByTimeAsync(160);

    expect(props.onCommandPaletteClose).toHaveBeenCalledTimes(1);
  });

  it("没有可用 draft session 时禁用输入和上传", () => {
    renderBar({
      currentSession: null,
      draftSessionReady: false,
      showCommandPalette: false,
      pendingFiles: [],
    });

    const input = screen.getByRole("textbox") as HTMLInputElement;
    expect(input.disabled).toBe(true);
    expect((screen.getByTitle("上传文件或图片") as HTMLButtonElement).disabled).toBe(true);
  });

  it("running 会话下显示取消按钮并允许继续输入", () => {
    renderBar({
      currentSession: {
        session_id: "session-1",
        title: "发布流程",
        project_name: "Alpha",
        status: "running",
        branch: "feature/release",
        message_count: 4,
        updated_at: "2026-03-14T00:00:00Z",
        created_at: "2026-03-14T00:00:00Z",
        project_id: 1,
        profile_id: "lead-1",
        profile_name: "Lead",
        driver_id: "codex-cli",
      },
      showCommandPalette: false,
      pendingFiles: [],
      modes: null,
      configOptions: [],
      sessionRunning: true,
    });

    expect((screen.getByRole("textbox") as HTMLInputElement).disabled).toBe(false);
    expect((screen.getByTitle("上传文件或图片") as HTMLButtonElement).disabled).toBe(false);
    const buttons = screen.getAllByRole("button");
    expect(buttons).toHaveLength(2);
    expect((buttons[1] as HTMLButtonElement).disabled).toBe(false);
  });
});
