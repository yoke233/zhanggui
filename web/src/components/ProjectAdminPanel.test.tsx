/** @vitest-environment jsdom */

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ApiClient } from "../lib/apiClient";
import type { WsClient } from "../lib/wsClient";
import type { WsEnvelope } from "../types/ws";
import ProjectAdminPanel from "./ProjectAdminPanel";

type WsStatus = "idle" | "connecting" | "open" | "closed";
type WsWildcardHandler = (payload: unknown, raw: MessageEvent<string>) => void;

interface WsHarness {
  wsClient: WsClient;
  emitEnvelope: (envelope: WsEnvelope) => void;
}

const createApiClientMock = (): ApiClient =>
  ({
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
    listRuns: vi.fn(),
    createRun: vi.fn(),
    createChat: vi.fn(),
    getChat: vi.fn(),
    createPlan: vi.fn(),
    submitPlanReview: vi.fn(),
    applyPlanAction: vi.fn(),
    applyTaskAction: vi.fn(),
    listPlans: vi.fn(),
    getPlanDag: vi.fn(),
    getRun: vi.fn(),
    getRunCheckpoints: vi.fn(),
    applyRunAction: vi.fn(),
  }) as unknown as ApiClient;

const createWsHarness = (): WsHarness => {
  const wildcardHandlers = new Set<WsWildcardHandler>();

  const wsClient = {
    connect: vi.fn(),
    disconnect: vi.fn(),
    send: vi.fn(),
    subscribe: vi.fn((type: string, handler: WsWildcardHandler) => {
      if (type === "*") {
        wildcardHandlers.add(handler);
      }
      return () => {
        if (type === "*") {
          wildcardHandlers.delete(handler);
        }
      };
    }),
    onStatusChange: vi.fn(),
    getStatus: vi.fn().mockReturnValue("open"),
  } as unknown as WsClient;

  return {
    wsClient,
    emitEnvelope: (envelope) => {
      wildcardHandlers.forEach((handler) => {
        handler(envelope, {} as MessageEvent<string>);
      });
    },
  };
};

const fillLocalPathForm = (name: string, repoPath: string): void => {
  fireEvent.change(screen.getByLabelText("项目名称"), {
    target: { value: name },
  });
  fireEvent.change(screen.getByLabelText("仓库路径"), {
    target: { value: repoPath },
  });
};

const openCreatePanel = (): void => {
  fireEvent.click(screen.getByRole("button", { name: "创建项目" }));
};

describe("ProjectAdminPanel", () => {
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  it("支持 source_type 切换并展示对应字段", () => {
    const apiClient = createApiClientMock();
    const wsHarness = createWsHarness();

    render(
      <ProjectAdminPanel
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        wsStatus={"open" as WsStatus}
        onProjectCreated={vi.fn()}
      />,
    );

    expect(screen.queryByLabelText("项目来源")).toBeNull();
    openCreatePanel();
    expect(screen.getByLabelText("项目来源")).toBeTruthy();
    expect(screen.getByLabelText("仓库路径")).toBeTruthy();
    expect(screen.queryByLabelText("Remote URL")).toBeNull();

    fireEvent.change(screen.getByLabelText("项目来源"), {
      target: { value: "local_new" },
    });
    expect(screen.queryByLabelText("仓库路径")).toBeNull();
    expect(screen.queryByLabelText("Remote URL")).toBeNull();

    fireEvent.change(screen.getByLabelText("项目来源"), {
      target: { value: "github_clone" },
    });
    expect(screen.queryByLabelText("仓库路径")).toBeNull();
    expect(screen.getByLabelText("Remote URL")).toBeTruthy();
    expect(screen.getByLabelText("Git Ref（可选）")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "关闭创建项目" }));
    expect(screen.queryByLabelText("项目来源")).toBeNull();
  });

  it("可通过 WS 事件更新创建进度并在成功后回调", async () => {
    const apiClient = createApiClientMock();
    const wsHarness = createWsHarness();
    const onProjectCreated = vi.fn().mockResolvedValue(undefined);
    (apiClient.createProjectCreateRequest as unknown as ReturnType<typeof vi.fn>).mockResolvedValue({
      request_id: "req-1",
    });

    render(
      <ProjectAdminPanel
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        wsStatus={"open" as WsStatus}
        onProjectCreated={onProjectCreated}
      />,
    );

    openCreatePanel();
    fillLocalPathForm("Alpha", "D:/repo/alpha");
    fireEvent.click(screen.getByRole("button", { name: "提交创建请求" }));

    await waitFor(() => {
      expect(apiClient.createProjectCreateRequest).toHaveBeenCalledWith({
        name: "Alpha",
        source_type: "local_path",
        repo_path: "D:/repo/alpha",
      });
    });

    wsHarness.emitEnvelope({
      type: "project_create_started",
      data: {
        request_id: "req-1",
        message: "开始创建",
      },
    });
    wsHarness.emitEnvelope({
      type: "project_create_progress",
      data: {
        request_id: "req-1",
        message: "克隆仓库中",
        progress: 60,
      },
    });
    wsHarness.emitEnvelope({
      type: "project_create_succeeded",
      data: {
        request_id: "req-1",
        project_id: "proj-9",
        message: "创建成功",
      },
    });

    await waitFor(() => {
      expect(onProjectCreated).toHaveBeenCalledWith("proj-9");
    });
    expect(screen.getByText(/创建成功/)).toBeTruthy();
  });

  it("可通过 WS 事件展示失败状态", async () => {
    const apiClient = createApiClientMock();
    const wsHarness = createWsHarness();
    const onProjectCreated = vi.fn().mockResolvedValue(undefined);
    (apiClient.createProjectCreateRequest as unknown as ReturnType<typeof vi.fn>).mockResolvedValue({
      request_id: "req-fail",
    });

    render(
      <ProjectAdminPanel
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        wsStatus={"open" as WsStatus}
        onProjectCreated={onProjectCreated}
      />,
    );

    openCreatePanel();
    fillLocalPathForm("Beta", "D:/repo/beta");
    fireEvent.click(screen.getByRole("button", { name: "提交创建请求" }));

    await waitFor(() => {
      expect(apiClient.createProjectCreateRequest).toHaveBeenCalledTimes(1);
    });

    wsHarness.emitEnvelope({
      type: "project_create_failed",
      data: {
        request_id: "req-fail",
        error: "GitHub clone failed",
      },
    });

    await waitFor(() => {
      expect(screen.getByText("GitHub clone failed")).toBeTruthy();
    });
    expect(onProjectCreated).not.toHaveBeenCalled();
  });

  it("WS 不可用时会轮询状态接口并在成功后回调", async () => {
    const apiClient = createApiClientMock();
    const wsHarness = createWsHarness();
    const onProjectCreated = vi.fn().mockResolvedValue(undefined);
    (apiClient.createProjectCreateRequest as unknown as ReturnType<typeof vi.fn>).mockResolvedValue({
      request_id: "req-poll",
    });
    (apiClient.getProjectCreateRequest as unknown as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce({
        request_id: "req-poll",
        status: "running",
        message: "准备仓库",
        progress: 20,
      })
      .mockResolvedValueOnce({
        request_id: "req-poll",
        status: "succeeded",
        project_id: "proj-poll",
        message: "完成",
      });

    render(
      <ProjectAdminPanel
        apiClient={apiClient}
        wsClient={wsHarness.wsClient}
        wsStatus={"closed" as WsStatus}
        onProjectCreated={onProjectCreated}
        pollIntervalMs={20}
      />,
    );

    openCreatePanel();
    fillLocalPathForm("Gamma", "D:/repo/gamma");
    fireEvent.click(screen.getByRole("button", { name: "提交创建请求" }));

    await waitFor(() => {
      expect(apiClient.createProjectCreateRequest).toHaveBeenCalledTimes(1);
    });

    await waitFor(() => {
      expect(apiClient.getProjectCreateRequest).toHaveBeenCalledTimes(2);
      expect(onProjectCreated).toHaveBeenCalledWith("proj-poll");
    }, {
      timeout: 3000,
    });
  });
});
