import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { SettingsPanel } from "@/components/SettingsPanel";
import SystemEventBanner from "@/components/SystemEventBanner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { createApiClientV2, type ApiClientV2 } from "@/lib/apiClientV2";
import { cn } from "@/lib/utils";
import { createWsClient, type WsClient } from "@/lib/wsClient";
import type { Project, ResourceBinding } from "@/types/apiV2";
import V2ArtifactView from "@/v2/views/ArtifactView";
import V2BriefingView from "@/v2/views/BriefingView";
import V2ChatView from "@/v2/views/ChatView";
import V2EventsView from "@/v2/views/EventsView";
import V2ExecutionsView from "@/v2/views/ExecutionsView";
import V2FlowsView from "@/v2/views/FlowsView";
import V2OpsView from "@/v2/views/OpsView";
import V2StepsView from "@/v2/views/StepsView";
import V2OverviewView from "@/v2/views/OverviewView";

type AppView = "overview" | "chat" | "flows" | "steps" | "ops";

const VIEW_LABELS: Record<AppView, string> = {
  overview: "总览",
  chat: "会话",
  flows: "任务列表",
  steps: "步骤",
  ops: "协议 / 运维",
};

const VIEW_DESCRIPTIONS: Record<AppView, string> = {
  overview: "保持 v3 信息架构与视觉布局，在同一视角里看项目、Flow 进度与系统健康。",
  chat: "Lead Chat（WebSocket 流式），从对话里沉淀目标并驱动 Flow。",
  flows: "Flow 工作台：创建、筛选、选择，并进入 Step/Execution/事件流。",
  steps: "围绕选中的 Flow/Step 查看 Steps、Executions、Events、Artifact、Briefing。",
  ops: "项目、资源绑定、统计与控制性接口统一收口在本页。",
};

const VIEW_EYEBROWS: Record<AppView, string> = {
  overview: "命令中心",
  chat: "会话 / 线程",
  flows: "任务列表 / Flow",
  steps: "步骤 / 执行与事件流",
  ops: "协议 / 审计 / 运维",
};

const API_BASE_URL =
  import.meta.env.VITE_API_V2_BASE_URL ||
  import.meta.env.VITE_API_BASE_URL ||
  "/api/v2";

const WS_BASE_URL = import.meta.env.VITE_WS_BASE_URL || "/api/v1";
const TOKEN_STORAGE_KEY = "ai-workflow-api-token";

type TokenSource = "query" | "storage" | "missing";

interface ResolvedToken {
  token: string | null;
  source: TokenSource;
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
  if (view === "overview" || view === "chat" || view === "flows" || view === "steps" || view === "ops") {
    return view as AppView;
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

interface AppV2Props {
  apiBaseUrlOverride?: string;
  uiA2AEnabledOverride?: boolean;
}

const AppV2 = ({ apiBaseUrlOverride, uiA2AEnabledOverride }: AppV2Props = {}) => {
  const tokenRef = useRef<string | null>(null);
  const apiClient: ApiClientV2 = useMemo(
    () =>
      createApiClientV2({
        baseUrl: apiBaseUrlOverride ?? API_BASE_URL,
        getToken: () => tokenRef.current,
      }),
    [apiBaseUrlOverride],
  );

  const wsClient: WsClient = useMemo(
    () =>
      createWsClient({
        baseUrl: WS_BASE_URL,
        getToken: () => tokenRef.current,
      }),
    [],
  );

  const [authStatus, setAuthStatus] = useState<"checking" | "ready" | "error">("checking");
  const [authError, setAuthError] = useState<string | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);
  const [projectsLoading, setProjectsLoading] = useState(false);
  const [projectsError, setProjectsError] = useState<string | null>(null);
  const [selectedProjectId, setSelectedProjectId] = useState<number | null>(null);
  const [activeView, setActiveView] = useState<AppView>(() => parseViewFromLocation());
  const [refreshToken, setRefreshToken] = useState(0);
  const [wsStatus, setWsStatus] = useState(wsClient.getStatus());
  const [settingsOpen, setSettingsOpen] = useState(false);

  const [projectResources, setProjectResources] = useState<ResourceBinding[]>([]);
  const [projectResourcesLoading, setProjectResourcesLoading] = useState(false);
  const [projectResourcesError, setProjectResourcesError] = useState<string | null>(null);

  const [selectedFlowId, setSelectedFlowId] = useState<number | null>(null);
  const [selectedStepId, setSelectedStepId] = useState<number | null>(null);
  const [selectedExecId, setSelectedExecId] = useState<number | null>(null);

  const selectedProject = selectedProjectId
    ? projects.find((project) => project.id === selectedProjectId) ?? null
    : null;

  const selectedWorkspace = useMemo(() => {
    const local = projectResources.find((r) => r.kind === "local_fs") ?? null;
    return local ?? projectResources[0] ?? null;
  }, [projectResources]);

  const applyProjects = useCallback((nextProjects: Project[], preferredProjectId?: number | null) => {
    setProjects(nextProjects);
    setSelectedProjectId((current) => {
      if (preferredProjectId != null && nextProjects.some((project) => project.id === preferredProjectId)) {
        return preferredProjectId;
      }
      if (current != null && nextProjects.some((project) => project.id === current)) {
        return current;
      }
      return nextProjects[0]?.id ?? null;
    });
  }, []);

  const fetchProjects = useCallback(async (): Promise<Project[]> => {
    const listed = await apiClient.listProjects({ limit: 200, offset: 0 });
    return Array.isArray(listed) ? listed : [];
  }, [apiClient]);

  const loadProjects = useCallback(
    async (preferredProjectId?: number | null) => {
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
    const unsub = wsClient.onStatusChange((status) => {
      setWsStatus(status);
    });
    return unsub;
  }, [wsClient]);

  useEffect(() => {
    const resolvedToken = resolveTokenFromLocation();
    if (!resolvedToken.token) {
      setAuthStatus("error");
      setAuthError("缺少访问 token，请使用 ?token=xxxx 访问。");
      return;
    }

    tokenRef.current = resolvedToken.token;
    wsClient.connect();

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
      wsClient.disconnect();
    };
  }, [applyProjects, fetchProjects, wsClient]);

  useEffect(() => {
    if (selectedProjectId == null) {
      setProjectResources([]);
      setProjectResourcesError(null);
      return;
    }

    let cancelled = false;
    const load = async () => {
      setProjectResourcesLoading(true);
      setProjectResourcesError(null);
      try {
        const listed = await apiClient.listProjectResources(selectedProjectId);
        if (!cancelled) {
          setProjectResources(Array.isArray(listed) ? listed : []);
        }
      } catch (err) {
        if (!cancelled) {
          setProjectResources([]);
          setProjectResourcesError(getErrorMessage(err));
        }
      } finally {
        if (!cancelled) {
          setProjectResourcesLoading(false);
        }
      }
    };

    void load();
    return () => {
      cancelled = true;
    };
  }, [apiClient, selectedProjectId]);

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
    window.history.replaceState(null, "", `${url.pathname}${url.search}${url.hash}`);
  }, [activeView]);

  const a2aEnabled = uiA2AEnabledOverride ?? resolveA2AEnabledFromEnv();

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
    <main className="min-h-screen bg-slate-50 text-slate-900">
      <SystemEventBanner wsClient={wsClient} />
      <div className="grid min-h-screen w-full gap-0 overflow-hidden bg-slate-50 lg:grid-cols-[248px_minmax(0,1fr)]">
        <aside className="flex min-h-full flex-col bg-[#13284a] px-5 py-5 text-white">
          <div className="flex items-center gap-3 rounded-xl px-1">
            <div className="flex h-8 w-8 items-center justify-center rounded-xl bg-[#2563eb] text-xs font-semibold">
              OS
            </div>
            <div className="min-w-0">
              <p className="text-sm font-semibold">AI Workflow</p>
              <p className="mt-0.5 text-[11px] text-slate-400">v3 风格 · v2 模型</p>
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
                    activeView === view ? "bg-[#24437a] text-white" : "bg-[#162a4d] text-slate-200 hover:text-white",
                  )}
                >
                  <div>
                    <p className="text-sm font-medium">{VIEW_LABELS[view]}</p>
                    <p className="mt-0.5 text-[11px] text-slate-400">{VIEW_EYEBROWS[view]}</p>
                  </div>
                  {view === "flows" ? (
                    <span className="rounded-full bg-[#1d4ed8] px-2 py-1 text-[10px] font-semibold uppercase tracking-[0.12em] text-white">
                      Flow
                    </span>
                  ) : null}
                </button>
              ))}
            </nav>
          </div>

          <div className="mt-auto space-y-3">
            <div>
              <p className="px-2 text-[11px] font-semibold uppercase tracking-[0.18em] text-slate-500">当前上下文</p>
              <div className="mt-3 rounded-2xl bg-[#162a4d] p-4">
                <p className="text-sm font-semibold text-slate-100">
                  {selectedProject ? `项目 ${selectedProject.name}` : "尚未选择项目"}
                </p>
                <p className="mt-1 text-[11px] text-slate-400">
                  {selectedProject
                    ? `kind=${selectedProject.kind}${selectedProject.description ? ` · ${selectedProject.description}` : ""}`
                    : "请先在运维页创建或刷新项目"}
                </p>
                <p className="mt-1 text-[11px] text-slate-400">
                  {selectedWorkspace
                    ? `workspace: ${selectedWorkspace.kind} · ${selectedWorkspace.uri}`
                    : projectResourcesLoading
                      ? "workspace: 加载中..."
                      : projectResourcesError
                        ? `workspace: 加载失败：${projectResourcesError}`
                        : "workspace: 未绑定（请在 Ops 添加 resources）"}
                </p>
              </div>
            </div>
            <div className="rounded-2xl bg-[#162a4d] p-4">
              <p className="text-sm font-semibold text-slate-100">{activeView === "ops" ? "管理员注意" : "系统状态"}</p>
              <p className="mt-1 text-[11px] leading-5 text-slate-400">
                {activeView === "ops"
                  ? "仅在本页暴露高权限动作与资源绑定入口。"
                  : a2aEnabled
                    ? "A2A 已启用（仅影响 v1 会话）。v2 Lead Chat 与 Flow 模型在当前工作台内。"
                    : "默认优先处理待运行 Flow、阻塞 Step 和会话收敛。"}
              </p>
            </div>
          </div>
        </aside>

        <section className="flex min-h-0 flex-1 flex-col bg-slate-50">
          <header className="border-b border-slate-200 bg-white px-7 py-5">
            <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
              <div className="space-y-2">
                <div className="flex items-center gap-2">
                  <Badge variant="secondary" className="bg-indigo-50 text-indigo-600">
                    {VIEW_EYEBROWS[activeView]}
                  </Badge>
                  <Badge
                    variant="outline"
                    className={wsStatus === "open" ? "bg-emerald-50 text-emerald-700" : "bg-amber-50 text-amber-700"}
                  >
                    WS {wsStatus}
                  </Badge>
                </div>
                <div>
                  <h1 className="text-[26px] font-semibold tracking-[-0.02em] text-slate-950">
                    {VIEW_LABELS[activeView]}
                  </h1>
                  <p className="mt-1 text-sm leading-6 text-slate-500">{VIEW_DESCRIPTIONS[activeView]}</p>
                </div>
              </div>

              <div className="grid gap-3 xl:min-w-[520px]">
                <div className="flex flex-wrap items-center justify-end gap-2">
                  <Badge variant="outline" className="bg-slate-50 text-slate-600">
                    API {apiBaseUrlOverride ?? API_BASE_URL}
                  </Badge>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={async () => {
                      await loadProjects();
                    }}
                    disabled={projectsLoading}
                  >
                    刷新项目
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setRefreshToken((current) => current + 1)}
                  >
                    全局刷新
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
                      value={selectedProjectId != null ? String(selectedProjectId) : ""}
                      onChange={(event) => {
                        const next = Number.parseInt(event.target.value, 10);
                        setSelectedProjectId(Number.isFinite(next) ? next : null);
                        setSelectedFlowId(null);
                        setSelectedStepId(null);
                        setSelectedExecId(null);
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
                    项目创建/资源绑定/控制性入口已收口到“协议 / 运维”页面。
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
              暂无可用项目。请先进入“协议 / 运维”创建项目并绑定资源，或点击“刷新项目”重试。
            </section>
          ) : (
            <div className="min-h-0 p-7">
              {activeView === "overview" ? (
                <V2OverviewView
                  apiClient={apiClient}
                  projectId={selectedProjectId}
                  projects={projects}
                  selectedProject={selectedProject}
                  refreshToken={refreshToken}
                  onNavigate={(view) => setActiveView(view)}
                />
              ) : null}

              {activeView === "chat" ? (
                <V2ChatView
                  apiClient={apiClient}
                  apiBaseUrl={apiBaseUrlOverride ?? API_BASE_URL}
                  getToken={() => tokenRef.current}
                  defaultWorkDir={selectedWorkspace?.kind === "local_fs" ? selectedWorkspace.uri : undefined}
                />
              ) : null}

              {activeView === "flows" ? (
                <V2FlowsView
                  apiClient={apiClient}
                  projectId={selectedProjectId}
                  project={selectedProject}
                  selectedFlowId={selectedFlowId}
                  refreshToken={refreshToken}
                  onSelectFlow={(flowId) => {
                    setSelectedFlowId(flowId);
                    setSelectedStepId(null);
                    setSelectedExecId(null);
                    setActiveView("steps");
                  }}
                />
              ) : null}

              {activeView === "steps" ? (
                <div className="grid gap-4">
                  {selectedFlowId != null ? (
                    <V2StepsView
                      apiClient={apiClient}
                      flowId={selectedFlowId}
                      selectedStepId={selectedStepId}
                      refreshToken={refreshToken}
                      onSelectStep={(stepId) => {
                        setSelectedStepId(stepId);
                        setSelectedExecId(null);
                      }}
                    />
                  ) : (
                    <section className="rounded-2xl border border-slate-200 bg-white p-8 text-sm text-slate-600 shadow-none">
                      请先在“任务列表”中选择一个 Flow。
                    </section>
                  )}

                  {selectedStepId != null ? (
                    <V2ExecutionsView
                      apiClient={apiClient}
                      stepId={selectedStepId}
                      refreshToken={refreshToken}
                      onSelectExecution={(execId) => setSelectedExecId(execId)}
                    />
                  ) : null}

                  {selectedFlowId != null ? (
                    <V2EventsView
                      apiClient={apiClient}
                      apiBaseUrl={apiBaseUrlOverride ?? API_BASE_URL}
                      getToken={() => tokenRef.current}
                      flowId={selectedFlowId}
                      refreshToken={refreshToken}
                    />
                  ) : null}

                  <div className="grid gap-4 lg:grid-cols-2">
                    <V2ArtifactView apiClient={apiClient} stepId={selectedStepId} execId={selectedExecId} />
                    <V2BriefingView apiClient={apiClient} stepId={selectedStepId} />
                  </div>
                </div>
              ) : null}

              {activeView === "ops" ? (
                <V2OpsView
                  apiClient={apiClient}
                  projects={projects}
                  projectsLoading={projectsLoading}
                  projectsError={projectsError}
                  selectedProjectId={selectedProjectId}
                  onSelectProject={setSelectedProjectId}
                  onRefreshProjects={() => void loadProjects()}
                  onProjectCreated={async (projectId) => {
                    await loadProjects(projectId ?? null);
                    setActiveView("ops");
                  }}
                  resources={projectResources}
                  resourcesLoading={projectResourcesLoading}
                  resourcesError={projectResourcesError}
                  onRefreshResources={async () => {
                    if (selectedProjectId == null) return;
                    setProjectResourcesLoading(true);
                    setProjectResourcesError(null);
                    try {
                      const listed = await apiClient.listProjectResources(selectedProjectId);
                      setProjectResources(Array.isArray(listed) ? listed : []);
                    } catch (err) {
                      setProjectResources([]);
                      setProjectResourcesError(getErrorMessage(err));
                    } finally {
                      setProjectResourcesLoading(false);
                    }
                  }}
                />
              ) : null}
            </div>
          )}
        </section>
      </div>
    </main>
  );
};

export default AppV2;
