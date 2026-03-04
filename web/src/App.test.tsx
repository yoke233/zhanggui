/** @vitest-environment jsdom */

import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WsEnvelope } from "./types/ws";

type AppWsStatus = "idle" | "connecting" | "open" | "closed";
type WsAnyHandler = (payload: unknown, raw: MessageEvent<string>) => void;
type WsStatusHandler = (status: AppWsStatus) => void;

const mocks = vi.hoisted(() => {
  const listProjects = vi.fn();
  const createProjectCreateRequest = vi.fn();
  const getProjectCreateRequest = vi.fn();
  const chatViewProps: Array<Record<string, unknown>> = [];
  const a2aViewProps: Array<Record<string, unknown>> = [];

  const a2aClient = {
    sendMessage: vi.fn(),
    getTask: vi.fn(),
    cancelTask: vi.fn(),
    streamMessage: vi.fn(),
  };

  const anyHandlers = new Set<WsAnyHandler>();
  const statusHandlers = new Set<WsStatusHandler>();
  let wsStatus: AppWsStatus = "open";

  const apiClient = {
    request: vi.fn(),
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    del: vi.fn(),
    getStats: vi.fn(),
    listProjects,
    createProject: vi.fn(),
    createProjectCreateRequest,
    getProjectCreateRequest,
    listRuns: vi.fn().mockResolvedValue({ items: [], total: 0, offset: 0 }),
    createRun: vi.fn(),
    createChat: vi.fn(),
    getChat: vi.fn(),
    createPlan: vi.fn(),
    submitPlanReview: vi.fn(),
    applyPlanAction: vi.fn(),
    applyTaskAction: vi.fn(),
    listPlans: vi.fn().mockResolvedValue({ items: [], total: 0, offset: 0 }),
    getPlanDag: vi.fn().mockResolvedValue({
      nodes: [],
      edges: [],
      stats: { total: 0, pending: 0, ready: 0, running: 0, done: 0, failed: 0 },
    }),
    getRun: vi.fn(),
    getRunCheckpoints: vi.fn(),
    applyRunAction: vi.fn(),
  };

  const wsClient = {
    connect: vi.fn(),
    disconnect: vi.fn(),
    send: vi.fn(),
    subscribe: vi.fn((type: string, handler: WsAnyHandler) => {
      if (type === "*") {
        anyHandlers.add(handler);
      }
      return () => {
        if (type === "*") {
          anyHandlers.delete(handler);
        }
      };
    }),
    onStatusChange: vi.fn((handler: WsStatusHandler) => {
      statusHandlers.add(handler);
      return () => {
        statusHandlers.delete(handler);
      };
    }),
    getStatus: vi.fn(() => wsStatus),
  };

  const emitEnvelope = (envelope: WsEnvelope): void => {
    anyHandlers.forEach((handler) => {
      handler(envelope, {} as MessageEvent<string>);
    });
  };

  const setWsStatus = (nextStatus: AppWsStatus): void => {
    wsStatus = nextStatus;
    statusHandlers.forEach((handler) => {
      handler(nextStatus);
    });
  };

  const resetState = (): void => {
    anyHandlers.clear();
    statusHandlers.clear();
    chatViewProps.length = 0;
    a2aViewProps.length = 0;
    wsStatus = "open";
  };

  return {
    apiClient,
    wsClient,
    a2aClient,
    chatViewProps,
    a2aViewProps,
    listProjects,
    createProjectCreateRequest,
    getProjectCreateRequest,
    emitEnvelope,
    setWsStatus,
    resetState,
  };
});

vi.mock("./lib/apiClient", () => {
  return {
    createApiClient: vi.fn(() => mocks.apiClient),
  };
});

vi.mock("./lib/wsClient", () => {
  return {
    createWsClient: vi.fn(() => mocks.wsClient),
  };
});

vi.mock("./lib/a2aClient", () => {
  return {
    createA2AClient: vi.fn(() => mocks.a2aClient),
  };
});

vi.mock("./views/ChatView", () => ({
  default: (props: Record<string, unknown>) => {
    mocks.chatViewProps.push(props);
    return <div>Chat View Mock</div>;
  },
}));

vi.mock("./views/A2AChatView", () => ({
  default: (props: Record<string, unknown>) => {
    mocks.a2aViewProps.push(props);
    return <div>A2A Chat View Mock</div>;
  },
}));

vi.mock("./views/BoardView", () => ({
  default: () => <div>Board View Mock</div>,
}));

import App from "./App";

const baseProject = {
  id: "proj-1",
  name: "Alpha",
  repo_path: "D:/repo/alpha",
  created_at: "2026-03-01T10:00:00.000Z",
  updated_at: "2026-03-01T10:00:00.000Z",
};

describe("App", () => {
  beforeEach(() => {
    window.history.replaceState(null, "", "/?view=chat");
    mocks.resetState();
    mocks.listProjects.mockReset();
    mocks.createProjectCreateRequest.mockReset();
    mocks.getProjectCreateRequest.mockReset();
    mocks.listProjects.mockResolvedValue([
      baseProject,
      {
        id: "proj-2",
        name: "Beta",
        repo_path: "D:/repo/beta",
        created_at: "2026-03-01T10:00:00.000Z",
        updated_at: "2026-03-01T10:00:00.000Z",
      },
    ]);
    mocks.createProjectCreateRequest.mockResolvedValue({ request_id: "req-default" });
    mocks.getProjectCreateRequest.mockResolvedValue({
      request_id: "req-default",
      status: "running",
    });
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("加载项目、支持项目切换与双视图切换", async () => {
    render(<App />);

    await waitFor(() => {
      expect(mocks.listProjects).toHaveBeenCalledTimes(1);
    });

    expect(screen.getByText("Chat View Mock")).toBeTruthy();

    const projectSelect = screen.getByLabelText("当前项目") as HTMLSelectElement;
    expect(projectSelect.value).toBe("proj-1");

    fireEvent.click(screen.getByRole("button", { name: "Issues" }));
    expect(screen.getByText("Board View Mock")).toBeTruthy();

    fireEvent.change(projectSelect, { target: { value: "proj-2" } });
    expect(projectSelect.value).toBe("proj-2");
  });

  it("创建成功后会刷新项目列表并自动选中新项目", async () => {
    mocks.listProjects.mockResolvedValueOnce([baseProject]).mockResolvedValueOnce([
      baseProject,
      {
        id: "proj-9",
        name: "Gamma",
        repo_path: "D:/repo/gamma",
        created_at: "2026-03-02T08:00:00.000Z",
        updated_at: "2026-03-02T08:00:00.000Z",
      },
    ]);
    mocks.createProjectCreateRequest.mockResolvedValueOnce({ request_id: "req-9" });

    render(<App />);

    await waitFor(() => {
      expect(mocks.listProjects).toHaveBeenCalledTimes(1);
    });

    fireEvent.click(screen.getByRole("button", { name: "创建项目" }));
    fireEvent.change(screen.getByLabelText("项目名称"), {
      target: { value: "Gamma" },
    });
    fireEvent.change(screen.getByLabelText("仓库路径"), {
      target: { value: "D:/repo/gamma" },
    });
    fireEvent.click(screen.getByRole("button", { name: "提交创建请求" }));

    await waitFor(() => {
      expect(mocks.createProjectCreateRequest).toHaveBeenCalledWith({
        name: "Gamma",
        source_type: "local_path",
        repo_path: "D:/repo/gamma",
      });
    });

    await act(async () => {
      mocks.emitEnvelope({
        type: "project_create_succeeded",
        data: {
          request_id: "req-9",
          project_id: "proj-9",
          message: "创建完成",
        },
      });
    });

    await waitFor(() => {
      expect(mocks.listProjects).toHaveBeenCalledTimes(2);
      expect((screen.getByLabelText("当前项目") as HTMLSelectElement).value).toBe("proj-9");
    });
  });

  it("创建失败时会显示错误且不触发项目列表刷新", async () => {
    mocks.listProjects.mockResolvedValueOnce([baseProject]);
    mocks.createProjectCreateRequest.mockResolvedValueOnce({ request_id: "req-fail" });

    render(<App />);

    await waitFor(() => {
      expect(mocks.listProjects).toHaveBeenCalledTimes(1);
    });

    fireEvent.click(screen.getByRole("button", { name: "创建项目" }));
    fireEvent.change(screen.getByLabelText("项目名称"), {
      target: { value: "Broken" },
    });
    fireEvent.change(screen.getByLabelText("仓库路径"), {
      target: { value: "D:/repo/broken" },
    });
    fireEvent.click(screen.getByRole("button", { name: "提交创建请求" }));

    await waitFor(() => {
      expect(mocks.createProjectCreateRequest).toHaveBeenCalledTimes(1);
    });

    await act(async () => {
      mocks.emitEnvelope({
        type: "project_create_failed",
        data: {
          request_id: "req-fail",
          error: "权限不足",
        },
      });
    });

    await waitFor(() => {
      expect(screen.getByText("权限不足")).toBeTruthy();
    });
    expect(mocks.listProjects).toHaveBeenCalledTimes(1);
  });

  it("来源切换逻辑在 App 中可正常工作", async () => {
    render(<App />);

    await waitFor(() => {
      expect(mocks.listProjects).toHaveBeenCalledTimes(1);
    });

    fireEvent.click(screen.getByRole("button", { name: "创建项目" }));
    fireEvent.change(screen.getByLabelText("项目来源"), {
      target: { value: "local_new" },
    });
    expect(screen.queryByLabelText("仓库路径")).toBeNull();

    fireEvent.change(screen.getByLabelText("项目来源"), {
      target: { value: "github_clone" },
    });
    expect(screen.getByLabelText("Remote URL")).toBeTruthy();
    expect(screen.queryByLabelText("仓库路径")).toBeNull();
  });

  it("listProjects 返回 null 时会回退为空数组并保持可交互", async () => {
    mocks.listProjects.mockResolvedValueOnce(null);
    render(<App />);

    await waitFor(() => {
      expect(mocks.listProjects).toHaveBeenCalledTimes(1);
    });

    expect((screen.getByLabelText("当前项目") as HTMLSelectElement).value).toBe("");
    expect(screen.getByText("暂无可用项目。请先在后端创建项目，或点击“刷新项目”重试。")).toBeTruthy();
  });
  it("默认关闭 A2A 时走 legacy ChatView", async () => {
    render(<App />);

    await waitFor(() => {
      expect(mocks.listProjects).toHaveBeenCalledTimes(1);
    });

    expect(screen.getByText("Chat View Mock")).toBeTruthy();
    expect(screen.queryByText("A2A Chat View Mock")).toBeNull();
    expect(mocks.chatViewProps.length).toBeGreaterThan(0);
    expect(mocks.a2aViewProps.length).toBe(0);
  });

  it("开启 A2A 时切换到 A2AChatView 入口", async () => {
    render(<App a2aEnabledOverride />);

    await waitFor(() => {
      expect(mocks.listProjects).toHaveBeenCalledTimes(1);
    });

    expect(screen.getByText("A2A Chat View Mock")).toBeTruthy();
    expect(screen.queryByText("Chat View Mock")).toBeNull();
    expect(mocks.a2aViewProps.length).toBeGreaterThan(0);
  });

});




