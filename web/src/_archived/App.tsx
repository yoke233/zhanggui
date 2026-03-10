import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { SettingsPanel } from "@/components/SettingsPanel";
import SystemEventBanner from "@/components/SystemEventBanner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { createA2AClient, type A2AClient } from "@/lib/a2aClient";
import { createApiClient, type ApiClient } from "@/lib/apiClient";
import { cn } from "@/lib/utils";
import { createWsClient, type WsClient } from "@/lib/wsClient";
import type { Project } from "@/types/workflow";
import type { WsEnvelope } from "@/types/ws";
import V3IssuesView from "@/v3/views/IssuesView";
import V3OpsView from "@/v3/views/OpsView";
import V3OverviewView from "@/v3/views/OverviewView";
import V3RunsView from "@/v3/views/RunsView";
import V3SessionsView from "@/v3/views/SessionsView";
import AppV2 from "@/v2/AppV2";

type AppView = "overview" | "chat" | "board" | "runs" | "ops";

const VIEW_LABELS: Record<AppView, string> = {
  overview: "总览",
  chat: "会话",
  board: "项目 / Issue",
  runs: "Run",
  ops: "协议 / 运维",
};

const VIEW_DESCRIPTIONS: Record<AppView, string> = {
  overview: "将项目、Issue、Run 和协议健康放到同一视角。",
  chat: "从会话里提炼目标、范围和验收条件，再决定是否进入 DAG / Issue 流程。",
  board: "查看 Issue 队列、操作详情时间线，并继续推进 review / run 流程。",
  runs: "把阶段推进、运行事件流和 GitHub 关联状态集中到一个只读视图。",
  ops: "集中查看审计、workflow profile 和危险操作入口，避免在业务页散落高权限动作。",
};

const VIEW_EYEBROWS: Record<AppView, string> = {
  overview: "命令中心",
  chat: "会话 / 线程",
  board: "项目 / Issue 工作台",
  runs: "Run 详情与事件流",
  ops: "协议 / 审计 / 运维",
};

const ISSUE_RUN_EVENT_TYPES = new Set([
  "run_started",
  "run_update",
  "run_completed",
  "run_failed",
  "run_cancelled",
  "auto_merged",
  "issue_created",
  "issue_reviewing",
  "review_done",
  "issue_approved",
  "issue_queued",
  "issue_ready",
  "issue_executing",
  "issue_done",
  "issue_failed",
  "issue_dependency_changed",
]);

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || "/api/v1";
const TOKEN_STORAGE_KEY = "ai-workflow-api-token";

type TokenSource = "query" | "storage" | "missing";

interface ResolvedToken {
  token: string | null;
  source: TokenSource;
}

interface ViewProps {
  apiClient: ApiClient;
  a2aClient: A2AClient;
  wsClient: WsClient;
  projectId: string | null;
  refreshToken: number;
  a2aEnabled: boolean;
  projects: Project[];
  selectedProject: Project | null;
  onNavigate: (view: "chat" | "board" | "runs" | "ops") => void;
  onProjectCreated: (projectId?: string) => Promise<void>;
  wsStatus: ReturnType<WsClient["getStatus"]>;
}

const resolveA2AEnabledFromEnv = (): boolean => {
  const raw = String(import.meta.env.VITE_A2A_ENABLED ?? "").trim().toLowerCase();
  return raw === "true" || raw === "1" || raw === "on";
};

const parseViewFromLocation = (): AppView => {
  if (typeof window === "undefined") {
    return "overview";
  }
  const params = new URLSearchParams(window.location.search);
  const view = params.get("view");
  if (view === "overview" || view === "chat" || view === "board" || view === "runs" || view === "ops") {
    return view as AppView;
  }
  if (params.get("issue")) {
    return "board";
  }
  return "overview";
};

const readTokenFromStorage = (): string | null => {
  if (typeof window === "undefined") {
    return null;
  }
  const raw = window.localStorage.getItem(TOKEN_STORAGE_KEY);
  if (!raw) {
    return null;
  }
  const token = raw.trim();
  return token.length > 0 ? token : null;
};

const persistTokenToStorage = (token: string): void => {
  if (typeof window === "undefined") {
    return;
  }
  window.localStorage.setItem(TOKEN_STORAGE_KEY, token);
};

const resolveTokenFromLocation = (): ResolvedToken => {
  if (typeof window === "undefined") {
    return { token: null, source: "missing" };
  }

  const params = new URLSearchParams(window.location.search);
  const queryToken = (params.get("token") ?? "").trim();
  if (queryToken.length > 0) {
    return { token: queryToken, source: "query" };
  }

  const storageToken = readTokenFromStorage();
  if (storageToken) {
    return { token: storageToken, source: "storage" };
  }

  return { token: null, source: "missing" };
};

const cleanupTokenFromUrlToHome = (): void => {
  if (typeof window === "undefined") {
    return;
  }

  const url = new URL(window.location.href);
  url.searchParams.delete("token");
  url.searchParams.delete("issue");
  url.searchParams.set("view", "overview");
  window.history.replaceState(null, "", `${url.pathname}${url.search}${url.hash}`);
};

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error && error.message.trim().length > 0) {
    return error.message;
  }
  return "请求失败，请稍后重试";
};

const renderView = (
  {
    apiClient,
    a2aClient,
    wsClient,
    projectId,
    refreshToken,
    a2aEnabled,
    projects,
    selectedProject,
    onNavigate,
    onProjectCreated,
    wsStatus,
  }: ViewProps,
  view: AppView,
) => {
  switch (view) {
    case "overview":
      return (
        <V3OverviewView
          apiClient={apiClient}
          projectId={projectId}
          projects={projects}
          selectedProject={selectedProject}
          refreshToken={refreshToken}
          onNavigate={onNavigate}
        />
      );
    case "chat":
      if (!projectId) {
        return null;
      }
      return (
        <V3SessionsView
          apiClient={apiClient}
          a2aClient={a2aClient}
          wsClient={wsClient}
          projectId={projectId}
          a2aEnabled={a2aEnabled}
        />
      );
    case "board":
      if (!projectId) {
        return null;
      }
      return <V3IssuesView apiClient={apiClient} projectId={projectId} refreshToken={refreshToken} />;
    case "runs":
      if (!projectId) {
        return null;
      }
      return <V3RunsView apiClient={apiClient} projectId={projectId} refreshToken={refreshToken} />;
    case "ops":
      return (
        <V3OpsView
          apiClient={apiClient}
          wsClient={wsClient}
          wsStatus={wsStatus}
          projectId={projectId}
          refreshToken={refreshToken}
          onProjectCreated={onProjectCreated}
        />
      );
    default:
      return null;
  }
};

interface AppProps {
  a2aEnabledOverride?: boolean;
  uiVersionOverride?: "v2" | "v3";
}

const AppV3 = ({ a2aEnabledOverride }: AppProps = {}) => {
  const a2aEnabled = a2aEnabledOverride ?? resolveA2AEnabledFromEnv();
  const tokenRef = useRef<string | null>(null);
  const apiClient = useMemo(
    () =>
      createApiClient({
        baseUrl: API_BASE_URL,
        getToken: () => tokenRef.current,
      }),
    [],
  );
  const wsClient = useMemo(
    () =>
      createWsClient({
        baseUrl: API_BASE_URL,
        getToken: () => tokenRef.current,
      }),
    [],
  );
  const a2aClient = useMemo(
    () =>
      createA2AClient({
        baseUrl: import.meta.env.VITE_A2A_BASE_URL || "/api/v1",
        getToken: () => tokenRef.current,
      }),
    [],
  );

  const [authStatus, setAuthStatus] = useState<"checking" | "ready" | "error">("checking");
  const [authError, setAuthError] = useState<string | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);
  const [projectsLoading, setProjectsLoading] = useState(false);
  const [projectsError, setProjectsError] = useState<string | null>(null);
  const [selectedProjectId, setSelectedProjectId] = useState<string | null>(null);
  const [activeView, setActiveView] = useState<AppView>(() => parseViewFromLocation());
  const [refreshToken, setRefreshToken] = useState(0);
  const [wsStatus, setWsStatus] = useState(wsClient.getStatus());
  const [settingsOpen, setSettingsOpen] = useState(false);

  const selectedProjectIdRef = useRef<string | null>(selectedProjectId);
  useEffect(() => {
    selectedProjectIdRef.current = selectedProjectId;
  }, [selectedProjectId]);

  const applyProjects = useCallback((nextProjects: Project[], preferredProjectId?: string | null) => {
    setProjects(nextProjects);
    setSelectedProjectId((current) => {
      if (preferredProjectId && nextProjects.some((project) => project.id === preferredProjectId)) {
        return preferredProjectId;
      }
      if (current && nextProjects.some((project) => project.id === current)) {
        return current;
      }
      return nextProjects[0]?.id ?? null;
    });
  }, []);

  const fetchProjects = useCallback(async (): Promise<Project[]> => {
    const listedProjects = await apiClient.listProjects();
    return Array.isArray(listedProjects) ? listedProjects : [];
  }, [apiClient]);

  const loadProjects = useCallback(
    async (preferredProjectId?: string | null) => {
      if (authStatus !== "ready") {
        return;
      }

      setProjectsLoading(true);
      setProjectsError(null);

      try {
        const nextProjects = await fetchProjects();
        applyProjects(nextProjects, preferredProjectId);
      } catch (error) {
        setProjectsError(getErrorMessage(error));
      } finally {
        setProjectsLoading(false);
      }
    },
    [applyProjects, authStatus, fetchProjects],
  );

  useEffect(() => {
    const resolvedToken = resolveTokenFromLocation();
    if (!resolvedToken.token) {
      setAuthStatus("error");
      setAuthError("缺少访问 token，请使用 ?token=xxxx 访问。");
      return;
    }

    tokenRef.current = resolvedToken.token;

    let cancelled = false;
    const bootstrap = async (): Promise<void> => {
      setAuthStatus("checking");
      setAuthError(null);
      setProjectsLoading(true);
      setProjectsError(null);

      try {
        const nextProjects = await fetchProjects();
        if (cancelled) {
          return;
        }
        applyProjects(nextProjects);
        if (resolvedToken.source === "query" && resolvedToken.token) {
          persistTokenToStorage(resolvedToken.token);
          setActiveView("overview");
          cleanupTokenFromUrlToHome();
        }
        setAuthStatus("ready");
      } catch (error) {
        if (cancelled) {
          return;
        }
        setProjects([]);
        setSelectedProjectId(null);
        setAuthStatus("error");
        setAuthError(`Token 校验失败：${getErrorMessage(error)}`);
      } finally {
        if (!cancelled) {
          setProjectsLoading(false);
        }
      }
    };

    void bootstrap();

    return () => {
      cancelled = true;
    };
  }, [applyProjects, fetchProjects]);

  useEffect(() => {
    if (authStatus !== "ready") {
      return;
    }

    const unsubscribeStatus = wsClient.onStatusChange((status) => {
      setWsStatus(status);
    });
    const unsubscribeAll = wsClient.subscribe<WsEnvelope>("*", (payload) => {
      const envelope = payload as WsEnvelope;
      if (!ISSUE_RUN_EVENT_TYPES.has(envelope.type)) {
        return;
      }
      const projectID = selectedProjectIdRef.current;
      if (projectID && envelope.project_id && envelope.project_id.trim().length > 0 && envelope.project_id !== projectID) {
        return;
      }
      setRefreshToken((current) => current + 1);
    });

    wsClient.connect();

    return () => {
      unsubscribeAll();
      unsubscribeStatus();
      wsClient.disconnect(1000, "app_unmount");
    };
  }, [authStatus, wsClient]);

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }
    const onPopState = () => {
      setActiveView(parseViewFromLocation());
    };
    window.addEventListener("popstate", onPopState);
    return () => {
      window.removeEventListener("popstate", onPopState);
    };
  }, []);

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }
    const url = new URL(window.location.href);
    const current = url.searchParams.get("view");
    if (current === activeView) {
      return;
    }
    url.searchParams.set("view", activeView);
    if (activeView !== "board") {
      url.searchParams.delete("issue");
    }
    window.history.replaceState(null, "", `${url.pathname}${url.search}${url.hash}`);
  }, [activeView]);

  const selectedProject = selectedProjectId
    ? projects.find((project) => project.id === selectedProjectId) ?? null
    : null;

  if (authStatus !== "ready") {
    return (
      <main className="min-h-screen px-4 py-6 text-slate-900 md:px-6">
        <div className="mx-auto flex w-full max-w-3xl flex-col gap-4">
          <section className="rounded-2xl border border-slate-200 bg-white p-8 shadow-[0_24px_80px_rgba(15,23,42,0.08)]">
            <Badge variant="secondary">AI Workflow</Badge>
            <h1 className="mt-4 text-3xl font-semibold tracking-tight">AI Workflow Workbench</h1>
            <p className="mt-2 text-sm text-slate-600">
              {authStatus === "checking" ? "正在验证访问 token..." : authError ?? "Token 校验失败"}
            </p>
            {authStatus === "error" ? (
              <p className="mt-4 rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
                请使用带 token 的访问链接重新进入，例如：<code>?token=xxxx</code>
              </p>
            ) : null}
          </section>
        </div>
      </main>
    );
  }

  return (
    <main className="min-h-screen bg-slate-50 px-3 py-3 text-slate-900 md:px-4">
      <SystemEventBanner wsClient={wsClient} />
      <div className="mx-auto grid min-h-[calc(100vh-1.5rem)] w-full max-w-[1440px] gap-0 overflow-hidden rounded-2xl border border-slate-200 bg-slate-50 lg:grid-cols-[248px_minmax(0,1fr)]">
        <aside className="flex min-h-full flex-col bg-[#0b1730] px-5 py-5 text-white">
          <div className="flex items-center gap-3 rounded-xl px-1">
            <div className="flex h-8 w-8 items-center justify-center rounded-xl bg-[#2563eb] text-xs font-semibold">
              OS
            </div>
            <div className="min-w-0">
              <p className="text-sm font-semibold">AI Workflow</p>
              <p className="mt-0.5 text-[11px] text-slate-400">v3 操作台</p>
            </div>
          </div>

          <div className="mt-6">
            <p className="px-2 text-[11px] font-semibold uppercase tracking-[0.18em] text-slate-500">主导航</p>
            <nav className="mt-3 flex flex-col gap-1.5">
              {(Object.keys(VIEW_LABELS) as AppView[]).map((view) => (
                <button
                  key={view}
                  type="button"
                  aria-label={VIEW_LABELS[view]}
                  onClick={() => {
                    setActiveView(view);
                  }}
                  className={cn(
                    "flex items-center justify-between rounded-xl px-3 py-3 text-left transition",
                    activeView === view ? "bg-[#162b5b] text-white" : "bg-[#0f1a33] text-slate-300 hover:text-white",
                  )}
                >
                  <div>
                    <p className="text-sm font-medium">{VIEW_LABELS[view]}</p>
                    <p className="mt-0.5 text-[11px] text-slate-400">{VIEW_EYEBROWS[view]}</p>
                  </div>
                  {view === "board" ? (
                    <span className="rounded-full bg-blue-500/15 px-2 py-1 text-[10px] font-semibold uppercase tracking-[0.12em] text-blue-200">
                      进行中
                    </span>
                  ) : null}
                </button>
              ))}
            </nav>
          </div>

          <div className="mt-auto space-y-3">
            <div>
              <p className="px-2 text-[11px] font-semibold uppercase tracking-[0.18em] text-slate-500">当前上下文</p>
              <div className="mt-3 rounded-2xl bg-[#0f1a33] p-4">
                <p className="text-sm font-semibold text-slate-100">
                  {selectedProject ? `项目 ${selectedProject.name}` : "尚未选择项目"}
                </p>
                <p className="mt-1 text-[11px] text-slate-400">
                  {selectedProject ? selectedProject.repo_path : "请先在运维页创建或刷新项目"}
                </p>
              </div>
            </div>
            <div className="rounded-2xl bg-[#0f1a33] p-4">
              <p className="text-sm font-semibold text-slate-100">{activeView === "ops" ? "管理员注意" : "系统状态"}</p>
              <p className="mt-1 text-[11px] leading-5 text-slate-400">
                {activeView === "ops"
                  ? "仅在本页暴露高权限动作与审计操作。"
                  : "默认优先处理待审 Issue、阻塞 Run 和会话收敛。"}
              </p>
            </div>
          </div>
        </aside>

        <section className="flex min-h-0 flex-col bg-slate-50">
          <header className="border-b border-slate-200 bg-white px-7 py-5">
            <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
              <div className="space-y-2">
                <div className="flex items-center gap-2">
                  <Badge variant="secondary" className="bg-indigo-50 text-indigo-600">
                    {VIEW_EYEBROWS[activeView]}
                  </Badge>
                  {selectedProject ? (
                    <Badge variant="outline" className="bg-slate-50 text-slate-600">
                      {selectedProject.name}
                    </Badge>
                  ) : null}
                </div>
                <div>
                  <h1 className="text-[26px] font-semibold tracking-[-0.02em] text-slate-950">{VIEW_LABELS[activeView]}</h1>
                  <p className="mt-1 text-sm leading-6 text-slate-500">{VIEW_DESCRIPTIONS[activeView]}</p>
                </div>
              </div>

              <div className="grid gap-3 xl:min-w-[520px]">
                <div className="flex flex-wrap items-center justify-end gap-2">
                  <Badge variant="outline" className="bg-slate-50 text-slate-600">
                    API {API_BASE_URL}
                  </Badge>
                  <Badge
                    variant="secondary"
                    className={cn(
                      "uppercase",
                      wsStatus === "open" ? "bg-emerald-50 text-emerald-600" : "bg-amber-50 text-amber-600",
                    )}
                  >
                    WS {wsStatus}
                  </Badge>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => {
                      void loadProjects();
                    }}
                    disabled={projectsLoading}
                  >
                    刷新项目
                  </Button>
                  <div className="relative">
                    <Button
                      variant="outline"
                      size="sm"
                      aria-label="外观设置"
                      onClick={() => setSettingsOpen((value) => !value)}
                    >
                      设置
                    </Button>
                    <SettingsPanel open={settingsOpen} onClose={() => setSettingsOpen(false)} />
                  </div>
                </div>

                <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-end">
                  <div className="grid gap-1">
                    <label htmlFor="project-select" className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
                      当前项目
                    </label>
                    <Select
                      id="project-select"
                      aria-label="当前项目"
                      value={selectedProjectId ?? ""}
                      onChange={(event) => {
                        const value = event.target.value;
                        setSelectedProjectId(value.length > 0 ? value : null);
                      }}
                      disabled={projectsLoading}
                      className="h-11 rounded-xl border-slate-200 bg-slate-50"
                    >
                      {projects.length === 0 ? (
                        <option value="">暂无项目</option>
                      ) : (
                        projects.map((project) => (
                          <option key={project.id} value={project.id}>
                            {project.name}
                          </option>
                        ))
                      )}
                    </Select>
                  </div>
                  <div className="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 text-xs leading-5 text-slate-500">
                    项目创建、审计和危险操作已统一收口到“协议 / 运维”页面。
                  </div>
                </div>

                {projectsError ? (
                  <p className="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
                    加载项目失败：{projectsError}
                  </p>
                ) : null}
              </div>
            </div>
          </header>

          {!selectedProjectId && activeView !== "ops" && activeView !== "overview" ? (
            <section className="m-7 rounded-2xl border border-slate-200 bg-white p-8 text-sm text-slate-600 shadow-none">
              暂无可用项目。请先进入“协议 / 运维”创建项目，或点击“刷新项目”重试。
            </section>
          ) : (
            <div className="min-h-0 p-7">
              {renderView(
                {
                  apiClient,
                  a2aClient,
                  wsClient,
                  projectId: selectedProjectId,
                  refreshToken,
                  a2aEnabled,
                  projects,
                  selectedProject,
                  onNavigate: (view) => {
                    setActiveView(view);
                  },
                  onProjectCreated: async (projectId) => {
                    await loadProjects(projectId);
                    setActiveView("ops");
                  },
                  wsStatus,
                },
                activeView,
              )}
            </div>
          )}
        </section>
      </div>
    </main>
  );
};

const resolveUIVersion = (): "v2" | "v3" => {
  const raw = String(import.meta.env.VITE_UI_VERSION ?? "").trim().toLowerCase();
  return raw === "v3" ? "v3" : "v2";
};

const App = (props: AppProps = {}) => {
  const uiVersion = props.uiVersionOverride ?? resolveUIVersion();
  if (uiVersion === "v3") {
    return <AppV3 {...props} />;
  }
  return <AppV2 />;
};

export default App;
