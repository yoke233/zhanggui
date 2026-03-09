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
  const commandCenterProps: Array<Record<string, unknown>> = [];
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
    createChat: vi.fn(),
    getChat: vi.fn(),
    createIssue: vi.fn(),
    submitIssueReview: vi.fn(),
    applyIssueAction: vi.fn(),
    applyTaskAction: vi.fn(),
    listIssues: vi.fn().mockResolvedValue({ items: [], total: 0, offset: 0 }),
    getIssueDag: vi.fn().mockResolvedValue({
      nodes: [],
      edges: [],
      stats: { total: 0, pending: 0, ready: 0, running: 0, done: 0, failed: 0 },
    }),
    getRun: vi.fn(),
    listWorkflowProfiles: vi.fn().mockResolvedValue({ items: [], total: 0, offset: 0 }),
    listAdminAuditLog: vi.fn().mockResolvedValue({ items: [], total: 0, offset: 0 }),
    forceIssueReady: vi.fn(),
    forceIssueUnblock: vi.fn(),
    sendSystemEvent: vi.fn(),
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
    commandCenterProps.length = 0;
    chatViewProps.length = 0;
    a2aViewProps.length = 0;
    wsStatus = "open";
  };

  return {
    apiClient,
    wsClient,
    a2aClient,
    commandCenterProps,
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

vi.mock("./v3/views/OverviewView", () => ({
  default: (props: Record<string, unknown>) => {
    mocks.commandCenterProps.push(props);
    if (!props.projectId) {
      return <div>当前没有可展示的业务总览</div>;
    }
    return <div>Command Center Mock</div>;
  },
}));

vi.mock("./v3/views/SessionsView", () => ({
  default: (props: Record<string, unknown>) => {
    if (props.a2aEnabled) {
      mocks.a2aViewProps.push(props);
      return <div>A2A Chat View Mock</div>;
    }
    mocks.chatViewProps.push(props);
    return <div>Chat View Mock</div>;
  },
}));

vi.mock("./v3/views/IssuesView", () => ({
  default: () => <div>Board View Mock</div>,
}));

vi.mock("./v3/views/RunsView", () => ({
  default: () => <div>Run View Mock</div>,
}));

vi.mock("./v3/views/OpsView", () => ({
  default: ({ apiClient, onProjectCreated }: Record<string, any>) => {
    const React = require("react") as typeof import("react");
    const [sourceType, setSourceType] = React.useState("local_path");
    const [projectName, setProjectName] = React.useState("");
    const [repoPath, setRepoPath] = React.useState("");
    const [remoteURL, setRemoteURL] = React.useState("");
    const [message, setMessage] = React.useState("");

    return (
      <div>
        <label htmlFor="mock-project-name">项目名称</label>
        <input id="mock-project-name" value={projectName} onChange={(event) => setProjectName(event.target.value)} />
        <label htmlFor="mock-project-source">项目来源</label>
        <select id="mock-project-source" value={sourceType} onChange={(event) => setSourceType(event.target.value)}>
          <option value="local_path">本地已有仓库</option>
          <option value="local_new">新建本地仓库</option>
          <option value="github_clone">从 GitHub 克隆</option>
        </select>
        {sourceType === "github_clone" ? (
          <>
            <label htmlFor="mock-remote-url">Remote URL</label>
            <input id="mock-remote-url" value={remoteURL} onChange={(event) => setRemoteURL(event.target.value)} />
          </>
        ) : (
          <>
            <label htmlFor="mock-repo-path">仓库路径</label>
            <input id="mock-repo-path" value={repoPath} onChange={(event) => setRepoPath(event.target.value)} />
          </>
        )}
        <button
          type="button"
          onClick={async () => {
            const payload: Record<string, string> = {
              name: projectName,
              source_type: sourceType,
            };
            if (sourceType === "github_clone") {
              payload.remote_url = remoteURL;
            } else {
              payload.repo_path = repoPath;
            }
            const response = await apiClient.createProjectCreateRequest(payload);
            const status = await apiClient.getProjectCreateRequest(response.request_id);
            if (status.status === "failed") {
              setMessage(status.error || "创建失败");
              return;
            }
            await onProjectCreated(status.project_id);
            setMessage(status.message || "创建完成");
          }}
        >
          创建项目
        </button>
        {message ? <div>{message}</div> : null}
      </div>
    );
  },
}));

import App from "./App";

const TOKEN_STORAGE_KEY = "ai-workflow-api-token";

const baseProject = {
  id: "proj-1",
  name: "Alpha",
  repo_path: "D:/repo/alpha",
  created_at: "2026-03-01T10:00:00.000Z",
  updated_at: "2026-03-01T10:00:00.000Z",
};

describe("App", () => {
  beforeEach(() => {
    window.history.replaceState(null, "", "/?view=overview");
    localStorage.setItem(TOKEN_STORAGE_KEY, "local-token");
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
    vi.useRealTimers();
  });

  it("加载项目、支持项目切换与双视图切换", async () => {
    render(<App />);

    await waitFor(() => {
      expect(mocks.listProjects).toHaveBeenCalledTimes(1);
    });

    expect(screen.getByText("Command Center Mock")).toBeTruthy();

    const projectSelect = screen.getByLabelText("当前项目") as HTMLSelectElement;
    expect(projectSelect.value).toBe("proj-1");

    fireEvent.click(screen.getByRole("button", { name: "项目 / Issue" }));
    expect(screen.getByText("Board View Mock")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "会话" }));
    expect(screen.getByText("Chat View Mock")).toBeTruthy();

    fireEvent.change(projectSelect, { target: { value: "proj-2" } });
    expect(projectSelect.value).toBe("proj-2");
  });

  it("首访缺少 token 时会阻止进入并提示错误", async () => {
    localStorage.removeItem(TOKEN_STORAGE_KEY);
    window.history.replaceState(null, "", "/");

    render(<App />);

    expect(
      screen.getByText("缺少访问 token，请使用 ?token=xxxx 访问。"),
    ).toBeTruthy();
    expect(mocks.listProjects).not.toHaveBeenCalled();
    expect(screen.queryByText("Command Center Mock")).toBeNull();
  });

  it("URL token 无效时会报错且不会写入本地存储", async () => {
    localStorage.removeItem(TOKEN_STORAGE_KEY);
    window.history.replaceState(null, "", "/?token=bad-token&view=board");
    mocks.listProjects.mockRejectedValueOnce(new Error("unauthorized"));

    render(<App />);

    await waitFor(() => {
      expect(screen.getByText("Token 校验失败：unauthorized")).toBeTruthy();
    });
    expect(localStorage.getItem(TOKEN_STORAGE_KEY)).toBeNull();
    expect(window.location.search).toContain("token=bad-token");
    expect(screen.queryByText("Board View Mock")).toBeNull();
  });

  it("URL token 有效时会进入首页并缓存到本地存储", async () => {
    localStorage.removeItem(TOKEN_STORAGE_KEY);
    window.history.replaceState(null, "", "/?token=good-token&view=board");

    render(<App />);

    await waitFor(() => {
      expect(mocks.listProjects).toHaveBeenCalledTimes(1);
    });

    expect(screen.getByText("Command Center Mock")).toBeTruthy();
    expect(localStorage.getItem(TOKEN_STORAGE_KEY)).toBe("good-token");
    expect(window.location.search).toBe("?view=overview");
    expect(window.location.search).not.toContain("token=");
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
    mocks.getProjectCreateRequest.mockResolvedValueOnce({
      request_id: "req-9",
      status: "succeeded",
      project_id: "proj-9",
      message: "创建完成",
    });

    render(<App />);

    await waitFor(() => {
      expect(mocks.listProjects).toHaveBeenCalledTimes(1);
    });

    fireEvent.click(screen.getByRole("button", { name: "协议 / 运维" }));
    fireEvent.change(screen.getByLabelText("项目名称"), {
      target: { value: "Gamma" },
    });
    fireEvent.change(screen.getByLabelText("仓库路径"), {
      target: { value: "D:/repo/gamma" },
    });
    fireEvent.click(screen.getByRole("button", { name: "创建项目" }));

    await waitFor(() => {
      expect(mocks.createProjectCreateRequest).toHaveBeenCalledWith({
        name: "Gamma",
        source_type: "local_path",
        repo_path: "D:/repo/gamma",
      });
    });

    await waitFor(() => {
      expect(mocks.getProjectCreateRequest).toHaveBeenCalledWith("req-9");
    });

    await waitFor(() => {
      expect(mocks.listProjects).toHaveBeenCalledTimes(2);
      expect((screen.getByLabelText("当前项目") as HTMLSelectElement).value).toBe("proj-9");
    });
  });

  it("创建失败时会显示错误且不触发项目列表刷新", async () => {
    mocks.listProjects.mockResolvedValueOnce([baseProject]);
    mocks.createProjectCreateRequest.mockResolvedValueOnce({ request_id: "req-fail" });
    mocks.getProjectCreateRequest.mockResolvedValueOnce({
      request_id: "req-fail",
      status: "failed",
      error: "权限不足",
    });

    render(<App />);

    await waitFor(() => {
      expect(mocks.listProjects).toHaveBeenCalledTimes(1);
    });

    fireEvent.click(screen.getByRole("button", { name: "协议 / 运维" }));
    fireEvent.change(screen.getByLabelText("项目名称"), {
      target: { value: "Broken" },
    });
    fireEvent.change(screen.getByLabelText("仓库路径"), {
      target: { value: "D:/repo/broken" },
    });
    fireEvent.click(screen.getByRole("button", { name: "创建项目" }));

    await waitFor(() => {
      expect(mocks.createProjectCreateRequest).toHaveBeenCalledTimes(1);
    });

    await waitFor(() => {
      expect(mocks.getProjectCreateRequest).toHaveBeenCalledWith("req-fail");
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

    fireEvent.click(screen.getByRole("button", { name: "协议 / 运维" }));
    fireEvent.change(screen.getByLabelText("项目来源"), {
      target: { value: "local_new" },
    });
    expect(screen.getByLabelText("仓库路径")).toBeTruthy();

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
    expect(screen.getByText("当前没有可展示的业务总览")).toBeTruthy();
  });
  it("默认关闭 A2A 时走 legacy ChatView", async () => {
    render(<App />);

    await waitFor(() => {
      expect(mocks.listProjects).toHaveBeenCalledTimes(1);
    });

    fireEvent.click(screen.getByRole("button", { name: "会话" }));
    expect(screen.getByText("Chat View Mock")).toBeTruthy();
    expect(screen.queryByText("A2A Chat View Mock")).toBeNull();
    expect(mocks.chatViewProps.length).toBeGreaterThan(0);
    expect(mocks.a2aViewProps.length).toBe(0);
  });

  it("显式开启 A2A 时走 A2AChatView 入口", async () => {
    render(<App a2aEnabledOverride={true} />);

    await waitFor(() => {
      expect(mocks.listProjects).toHaveBeenCalledTimes(1);
    });

    fireEvent.click(screen.getByRole("button", { name: "会话" }));
    expect(screen.getByText("A2A Chat View Mock")).toBeTruthy();
    expect(screen.queryByText("Chat View Mock")).toBeNull();
    expect(mocks.a2aViewProps.length).toBeGreaterThan(0);
  });

});




