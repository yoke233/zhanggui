import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import ChatView from "./views/ChatView";
import PlanView from "./views/PlanView";
import BoardView from "./views/BoardView";
import PipelineView from "./views/PipelineView";
import ProjectAdminPanel from "./components/ProjectAdminPanel";
import { createApiClient, type ApiClient } from "./lib/apiClient";
import { createWsClient, type WsClient } from "./lib/wsClient";
import type { WsEnvelope } from "./types/ws";
import type { Project } from "./types/workflow";

type AppView = "chat" | "plan" | "board" | "pipeline";

const VIEW_LABELS: Record<AppView, string> = {
  chat: "Chat",
  plan: "Plan",
  board: "Board",
  pipeline: "Pipeline",
};

const PLAN_TASK_EVENT_TYPES = new Set([
  "plan_created",
  "plan_reviewing",
  "plan_approved",
  "plan_waiting_human",
  "plan_done",
  "plan_failed",
  "plan_partially_done",
  "task_ready",
  "task_running",
  "task_done",
  "task_failed",
]);

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || "/api/v1";
const API_TOKEN = import.meta.env.VITE_API_TOKEN || "";

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error && error.message.trim().length > 0) {
    return error.message;
  }
  return "请求失败，请稍后重试";
};

interface ViewProps {
  apiClient: ApiClient;
  wsClient: WsClient;
  projectId: string;
  refreshToken: number;
}

const renderView = ({ apiClient, wsClient, projectId, refreshToken }: ViewProps, view: AppView) => {
  switch (view) {
    case "chat":
      return <ChatView apiClient={apiClient} projectId={projectId} />;
    case "plan":
      return (
        <PlanView
          apiClient={apiClient}
          wsClient={wsClient}
          projectId={projectId}
          refreshToken={refreshToken}
        />
      );
    case "board":
      return (
        <BoardView
          apiClient={apiClient}
          projectId={projectId}
          refreshToken={refreshToken}
        />
      );
    case "pipeline":
      return (
        <PipelineView
          apiClient={apiClient}
          projectId={projectId}
          refreshToken={refreshToken}
        />
      );
    default:
      return null;
  }
};

const App = () => {
  const apiClient = useMemo(
    () =>
      createApiClient({
        baseUrl: API_BASE_URL,
        getToken: () => API_TOKEN || null,
      }),
    [],
  );
  const wsClient = useMemo(
    () =>
      createWsClient({
        baseUrl: API_BASE_URL,
        getToken: () => API_TOKEN || null,
      }),
    [],
  );

  const [projects, setProjects] = useState<Project[]>([]);
  const [projectsLoading, setProjectsLoading] = useState(false);
  const [projectsError, setProjectsError] = useState<string | null>(null);
  const [selectedProjectId, setSelectedProjectId] = useState<string | null>(null);
  const [activeView, setActiveView] = useState<AppView>("chat");
  const [refreshToken, setRefreshToken] = useState(0);
  const [wsStatus, setWsStatus] = useState(wsClient.getStatus());

  const selectedProjectIdRef = useRef<string | null>(selectedProjectId);
  useEffect(() => {
    selectedProjectIdRef.current = selectedProjectId;
  }, [selectedProjectId]);

  const loadProjects = useCallback(async (preferredProjectId?: string | null) => {
    setProjectsLoading(true);
    setProjectsError(null);

    try {
      const listedProjects = await apiClient.listProjects();
      const nextProjects = Array.isArray(listedProjects) ? listedProjects : [];
      setProjects(nextProjects);
      setSelectedProjectId((current) => {
        if (
          preferredProjectId &&
          nextProjects.some((project) => project.id === preferredProjectId)
        ) {
          return preferredProjectId;
        }
        if (current && nextProjects.some((project) => project.id === current)) {
          return current;
        }
        return nextProjects[0]?.id ?? null;
      });
    } catch (error) {
      setProjectsError(getErrorMessage(error));
    } finally {
      setProjectsLoading(false);
    }
  }, [apiClient]);

  useEffect(() => {
    void loadProjects();
  }, [loadProjects]);

  useEffect(() => {
    const unsubscribeStatus = wsClient.onStatusChange((status) => {
      setWsStatus(status);
    });
    const unsubscribeAll = wsClient.subscribe<WsEnvelope>("*", (payload) => {
      const envelope = payload as WsEnvelope;
      if (!PLAN_TASK_EVENT_TYPES.has(envelope.type)) {
        return;
      }
      const projectID = selectedProjectIdRef.current;
      if (
        projectID &&
        envelope.project_id &&
        envelope.project_id.trim().length > 0 &&
        envelope.project_id !== projectID
      ) {
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
  }, [wsClient]);

  const selectedProject = selectedProjectId
    ? projects.find((project) => project.id === selectedProjectId) ?? null
    : null;

  return (
    <main className="min-h-screen bg-slate-100 px-4 py-6 text-slate-900 md:px-6">
      <div className="mx-auto flex w-full max-w-7xl flex-col gap-4">
        <header className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h1 className="text-2xl font-bold">AI Workflow Workbench</h1>
              <p className="mt-1 text-sm text-slate-600">
                API: <code>{API_BASE_URL}</code> · WS 状态:{" "}
                <span className="font-semibold">{wsStatus}</span>
              </p>
            </div>
            <div className="flex items-center gap-2">
              <label htmlFor="project-select" className="text-sm font-medium">
                当前项目
              </label>
              <select
                id="project-select"
                className="min-w-64 rounded-md border border-slate-300 bg-white px-3 py-2 text-sm"
                value={selectedProjectId ?? ""}
                onChange={(event) => {
                  const value = event.target.value;
                  setSelectedProjectId(value.length > 0 ? value : null);
                }}
                disabled={projectsLoading}
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
              </select>
              <button
                type="button"
                className="rounded-md border border-slate-300 bg-white px-3 py-2 text-sm hover:bg-slate-50"
                onClick={() => {
                  void loadProjects();
                }}
                disabled={projectsLoading}
              >
                刷新项目
              </button>
            </div>
          </div>

          {selectedProject ? (
            <p className="mt-2 text-xs text-slate-500">
              项目 ID: {selectedProject.id} · Repo: {selectedProject.repo_path}
            </p>
          ) : null}

          {projectsError ? (
            <p className="mt-2 rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">
              加载项目失败：{projectsError}
            </p>
          ) : null}

          <ProjectAdminPanel
            apiClient={apiClient}
            wsClient={wsClient}
            wsStatus={wsStatus}
            onProjectCreated={async (projectId) => {
              await loadProjects(projectId);
            }}
          />
        </header>

        <nav className="rounded-xl border border-slate-200 bg-white p-2 shadow-sm">
          <div className="flex flex-wrap gap-2">
            {(Object.keys(VIEW_LABELS) as AppView[]).map((view) => (
              <button
                key={view}
                type="button"
                onClick={() => {
                  setActiveView(view);
                }}
                className={`rounded-lg px-4 py-2 text-sm font-semibold transition ${
                  activeView === view
                    ? "bg-slate-900 text-white"
                    : "bg-slate-100 text-slate-700 hover:bg-slate-200"
                }`}
              >
                {VIEW_LABELS[view]}
              </button>
            ))}
          </div>
        </nav>

        {!selectedProjectId ? (
          <section className="rounded-xl border border-slate-200 bg-white p-6 text-sm text-slate-600 shadow-sm">
            暂无可用项目。请先在后端创建项目，或点击“刷新项目”重试。
          </section>
        ) : (
          renderView(
            {
              apiClient,
              wsClient,
              projectId: selectedProjectId,
              refreshToken,
            },
            activeView,
          )
        )}
      </div>
    </main>
  );
};

export default App;
