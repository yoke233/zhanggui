// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nextProvider } from "react-i18next";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import i18n from "../i18n";
import { AgentsPage } from "./AgentsPage";

const { mockUseWorkbench } = vi.hoisted(() => ({
  mockUseWorkbench: vi.fn(),
}));

vi.mock("@/contexts/WorkbenchContext", () => ({
  useWorkbench: mockUseWorkbench,
}));

function createWsClientMock() {
  const handlers = new Map<string, (payload: unknown) => void>();
  return {
    subscribe: vi.fn((type: string, handler: (payload: unknown) => void) => {
      handlers.set(type, handler);
      return vi.fn();
    }),
    emit(type: string, payload: unknown) {
      const handler = handlers.get(type);
      if (handler) {
        handler(payload);
      }
    },
  };
}

function renderPage() {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter>
        <AgentsPage />
      </MemoryRouter>
    </I18nextProvider>,
  );
}

describe("AgentsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    void i18n.changeLanguage("zh-CN");
  });

  afterEach(() => {
    cleanup();
  });

  it("支持编辑并保存 LLM 配置", async () => {
    const wsClient = createWsClientMock();
    const apiClient = {
      listDrivers: vi.fn().mockResolvedValue([
        {
          id: "codex-cli",
          launch_command: "codex",
          launch_args: ["serve"],
          capabilities_max: { fs_read: true, fs_write: true, terminal: true },
        },
      ]),
      listProfiles: vi.fn().mockResolvedValue([
        {
          id: "lead",
          name: "Lead",
          role: "lead",
          driver_id: "codex-cli",
          skills: ["planner"],
          session: { reuse: true },
        },
      ]),
      getLLMConfig: vi.fn().mockResolvedValue({
        default_config_id: "openai-prod",
        configs: [
          {
            id: "openai-prod",
            type: "openai_response",
            model: "gpt-4.1-mini",
          },
        ],
      }),
      listSkills: vi.fn().mockResolvedValue([
        {
          name: "planner",
          has_skill_md: true,
          valid: true,
          metadata: { name: "planner", description: "Plan tasks" },
        },
      ]),
      getSandboxSupport: vi.fn().mockResolvedValue({
        os: "windows",
        arch: "amd64",
        enabled: true,
        configured_provider: "docker",
        current_provider: "docker",
        current_supported: true,
        providers: {},
      }),
      updateLLMConfig: vi.fn().mockResolvedValue({
        default_config_id: "openai-prod",
        configs: [
          {
            id: "openai-prod",
            type: "openai_response",
            model: "gpt-4.1",
          },
        ],
      }),
    };

    mockUseWorkbench.mockReturnValue({ apiClient, wsClient });

    renderPage();

    expect(await screen.findByText("代理管理")).toBeTruthy();

    fireEvent.change(await screen.findByDisplayValue("gpt-4.1-mini"), {
      target: { value: "gpt-4.1" },
    });
    fireEvent.click(screen.getByRole("button", { name: "保存配置" }));

    await waitFor(() => {
      expect(apiClient.updateLLMConfig).toHaveBeenCalledWith({
        default_config_id: "openai-prod",
        configs: [
          {
            id: "openai-prod",
            type: "openai_response",
            model: "gpt-4.1",
          },
        ],
      });
    });

    expect(await screen.findByDisplayValue("gpt-4.1")).toBeTruthy();
  });

  it("收到 runtime 配置重载事件后重新拉取页面数据", async () => {
    const wsClient = createWsClientMock();
    const apiClient = {
      listDrivers: vi.fn().mockResolvedValue([
        {
          id: "codex-cli",
          launch_command: "codex",
          launch_args: ["serve"],
          capabilities_max: { fs_read: true, fs_write: true, terminal: true },
        },
      ]),
      listProfiles: vi.fn().mockResolvedValue([
        {
          id: "worker-a",
          name: "Worker A",
          role: "worker",
          driver_id: "codex-cli",
          skills: [],
          session: { reuse: false },
        },
      ]),
      getLLMConfig: vi.fn().mockResolvedValue({
        default_config_id: "claude-backup",
        configs: [
          {
            id: "claude-backup",
            type: "anthropic",
            model: "claude-3-7-sonnet-latest",
          },
        ],
      }),
      listSkills: vi.fn().mockResolvedValue([]),
      getSandboxSupport: vi.fn().mockResolvedValue({
        os: "linux",
        arch: "amd64",
        enabled: false,
        configured_provider: "docker",
        current_provider: "",
        current_supported: false,
        providers: {},
      }),
      updateLLMConfig: vi.fn(),
    };

    mockUseWorkbench.mockReturnValue({ apiClient, wsClient });

    renderPage();

    expect(await screen.findByText("worker-a")).toBeTruthy();

    expect(apiClient.listDrivers).toHaveBeenCalledTimes(1);
    expect(apiClient.listProfiles).toHaveBeenCalledTimes(1);
    expect(apiClient.getLLMConfig).toHaveBeenCalledTimes(1);
    expect(apiClient.listSkills).toHaveBeenCalledTimes(1);
    expect(apiClient.getSandboxSupport).toHaveBeenCalledTimes(1);

    wsClient.emit("runtime.config_reloaded", {});

    await waitFor(() => {
      expect(apiClient.listDrivers).toHaveBeenCalledTimes(2);
      expect(apiClient.listProfiles).toHaveBeenCalledTimes(2);
      expect(apiClient.getLLMConfig).toHaveBeenCalledTimes(2);
      expect(apiClient.listSkills).toHaveBeenCalledTimes(2);
      expect(apiClient.getSandboxSupport).toHaveBeenCalledTimes(2);
    });
  });

  it("支持点击 profile 打开弹窗并从系统列表选择 skill", async () => {
    const wsClient = createWsClientMock();
    const apiClient = {
      listDrivers: vi.fn().mockResolvedValue([
        {
          id: "codex-acp",
          launch_command: "npx",
          launch_args: ["-y", "@zed-industries/codex-acp"],
          capabilities_max: { fs_read: true, fs_write: true, terminal: true },
        },
      ]),
      listProfiles: vi.fn().mockResolvedValue([
        {
          id: "lead",
          name: "Lead Agent",
          role: "lead",
          driver_id: "codex-acp",
          skills: ["plan-actions"],
          actions_allowed: ["read_context", "submit"],
          capabilities: ["planning"],
          session: { reuse: true, max_turns: 8 },
        },
      ]),
      getLLMConfig: vi.fn().mockResolvedValue({
        default_config_id: "openai-prod",
        configs: [
          {
            id: "openai-prod",
            type: "openai_response",
            model: "gpt-5.4",
          },
          {
            id: "anthropic-prod",
            type: "anthropic",
            model: "claude-opus-4-6",
          },
        ],
      }),
      listSkills: vi.fn().mockResolvedValue([
        {
          name: "plan-actions",
          has_skill_md: true,
          valid: true,
          metadata: { name: "plan-actions", description: "Planning helpers" },
        },
        {
          name: "strict-review",
          has_skill_md: true,
          valid: true,
          metadata: { name: "strict-review", description: "Review helpers" },
        },
      ]),
      getSandboxSupport: vi.fn().mockResolvedValue({
        os: "windows",
        arch: "amd64",
        enabled: true,
        configured_provider: "docker",
        current_provider: "docker",
        current_supported: true,
        providers: {},
      }),
      updateLLMConfig: vi.fn(),
      updateProfile: vi.fn().mockResolvedValue({}),
      createProfile: vi.fn(),
      createDriver: vi.fn(),
      updateDriver: vi.fn(),
      deleteDriver: vi.fn(),
      deleteProfile: vi.fn(),
    };

    mockUseWorkbench.mockReturnValue({ apiClient, wsClient });

    renderPage();

    fireEvent.click(await screen.findByText("Lead Agent"));

    expect(await screen.findByText("编辑配置")).toBeTruthy();
    fireEvent.click(screen.getByLabelText(/strict-review/i));
    fireEvent.click(screen.getAllByRole("button", { name: "保存配置" }).at(-1)!);

    await waitFor(() => {
      expect(apiClient.updateProfile).toHaveBeenCalledWith(
        "lead",
        expect.objectContaining({
          id: "lead",
          name: "Lead Agent",
          driver_id: "codex-acp",
          role: "lead",
          skills: ["plan-actions", "strict-review"],
          llm_config_id: "system",
          session: expect.objectContaining({
            reuse: true,
            max_turns: 8,
          }),
        }),
      );
    });
  });
});
