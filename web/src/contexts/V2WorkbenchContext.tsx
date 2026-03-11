import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { createApiClientV2, type ApiClientV2 } from "@/lib/apiClientV2";
import { fetchDesktopBootstrap, isTauri } from "@/lib/desktopBridge";
import { getErrorMessage } from "@/lib/v2Workbench";
import { createWsClient, type WsClient } from "@/lib/wsClient";
import type { Project } from "@/types/apiV2";

const DEFAULT_API_BASE_URL =
  import.meta.env.VITE_API_V2_BASE_URL ||
  import.meta.env.VITE_API_BASE_URL ||
  "/api/v2";

const DEFAULT_WS_BASE_URL = import.meta.env.VITE_API_V2_BASE_URL || "/api/v2";
const TOKEN_STORAGE_KEY = "ai-workflow-api-token";
const PROJECT_STORAGE_KEY = "ai-workflow-selected-project-id";

type AuthStatus = "checking" | "ready" | "error";
type TokenSource = "query" | "storage" | "missing";

interface ResolvedToken {
  token: string | null;
  source: TokenSource;
}

interface V2WorkbenchContextValue {
  apiClient: ApiClientV2;
  wsClient: WsClient;
  authStatus: AuthStatus;
  authError: string | null;
  projects: Project[];
  projectsLoading: boolean;
  projectsError: string | null;
  selectedProjectId: number | null;
  selectedProject: Project | null;
  setSelectedProjectId: (projectId: number | null) => void;
  reloadProjects: (preferredProjectId?: number | null) => Promise<Project[]>;
}

const V2WorkbenchContext = createContext<V2WorkbenchContextValue | null>(null);

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

const readSelectedProjectId = (): number | null => {
  if (typeof window === "undefined") {
    return null;
  }
  const raw = window.localStorage.getItem(PROJECT_STORAGE_KEY);
  if (!raw) {
    return null;
  }
  const value = Number.parseInt(raw, 10);
  return Number.isFinite(value) ? value : null;
};

const persistSelectedProjectId = (projectId: number | null): void => {
  if (typeof window === "undefined") {
    return;
  }
  if (projectId == null) {
    window.localStorage.removeItem(PROJECT_STORAGE_KEY);
    return;
  }
  window.localStorage.setItem(PROJECT_STORAGE_KEY, String(projectId));
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

const cleanupTokenFromUrl = (): void => {
  if (typeof window === "undefined") {
    return;
  }
  const url = new URL(window.location.href);
  url.searchParams.delete("token");
  window.history.replaceState(null, "", `${url.pathname}${url.search}${url.hash}`);
};

interface ProviderProps {
  children: ReactNode;
}

export function V2WorkbenchProvider({ children }: ProviderProps) {
  const tokenRef = useRef<string | null>(null);
  const [apiBaseUrl, setApiBaseUrl] = useState(DEFAULT_API_BASE_URL);
  const [wsBaseUrl, setWsBaseUrl] = useState(DEFAULT_WS_BASE_URL);

  const apiClient = useMemo(
    () =>
      createApiClientV2({
        baseUrl: apiBaseUrl,
        getToken: () => tokenRef.current,
      }),
    [apiBaseUrl],
  );

  const wsClient = useMemo(
    () =>
      createWsClient({
        baseUrl: wsBaseUrl,
        getToken: () => tokenRef.current,
      }),
    [wsBaseUrl],
  );

  const [authStatus, setAuthStatus] = useState<AuthStatus>("checking");
  const [authError, setAuthError] = useState<string | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);
  const [projectsLoading, setProjectsLoading] = useState(false);
  const [projectsError, setProjectsError] = useState<string | null>(null);
  const [selectedProjectId, setSelectedProjectIdState] = useState<number | null>(() => readSelectedProjectId());

  const applyProjects = useCallback((nextProjects: Project[], preferredProjectId?: number | null) => {
    setProjects(nextProjects);
    setSelectedProjectIdState((current) => {
      const preferred = preferredProjectId ?? current ?? readSelectedProjectId();
      const matched = preferred != null && nextProjects.some((project) => project.id === preferred)
        ? preferred
        : nextProjects[0]?.id ?? null;
      persistSelectedProjectId(matched);
      return matched;
    });
  }, []);

  const reloadProjects = useCallback(
    async (preferredProjectId?: number | null): Promise<Project[]> => {
      setProjectsLoading(true);
      setProjectsError(null);
      try {
        const listed = await apiClient.listProjects({ limit: 200, offset: 0 });
        const nextProjects = Array.isArray(listed) ? listed : [];
        applyProjects(nextProjects, preferredProjectId);
        return nextProjects;
      } catch (error) {
        setProjectsError(getErrorMessage(error));
        throw error;
      } finally {
        setProjectsLoading(false);
      }
    },
    [apiClient, applyProjects],
  );

  useEffect(() => {
    const resolvedToken = resolveTokenFromLocation();
    let cancelled = false;

    const bootstrap = async (): Promise<void> => {
      let token = resolvedToken.token;
      let tokenSource = resolvedToken.source;
      let effectiveApiBaseUrl = apiBaseUrl;
      let effectiveWsBaseUrl = wsBaseUrl;

      if (!token && isTauri()) {
        try {
          const desktop = await fetchDesktopBootstrap();
          if (cancelled) {
            return;
          }
          token = desktop.token;
          tokenSource = "storage";
          effectiveApiBaseUrl = desktop.api_v2_base_url;
          effectiveWsBaseUrl = desktop.api_v2_base_url;
          setApiBaseUrl(desktop.api_v2_base_url);
          setWsBaseUrl(desktop.api_v2_base_url);
          persistTokenToStorage(desktop.token);
        } catch (error) {
          if (!cancelled) {
            setAuthStatus("error");
            setAuthError(`桌面版启动失败：${getErrorMessage(error)}`);
          }
          return;
        }
      }

      if (!token) {
        if (!cancelled) {
          setAuthStatus("error");
          setAuthError("缺少访问 token，请使用 ?token=xxxx 访问。");
        }
        return;
      }

      tokenRef.current = token;
      setAuthStatus("checking");
      setAuthError(null);

      try {
        const bootstrapClient = createApiClientV2({
          baseUrl: effectiveApiBaseUrl,
          getToken: () => token,
        });
        const listed = await bootstrapClient.listProjects({ limit: 200, offset: 0 });
        if (cancelled) {
          return;
        }
        applyProjects(Array.isArray(listed) ? listed : []);
        if (tokenSource === "query") {
          persistTokenToStorage(token);
          cleanupTokenFromUrl();
        }
        setApiBaseUrl(effectiveApiBaseUrl);
        setWsBaseUrl(effectiveWsBaseUrl);
        setAuthStatus("ready");
      } catch (error) {
        if (!cancelled) {
          setProjects([]);
          setAuthStatus("error");
          setAuthError(`Token 校验失败：${getErrorMessage(error)}`);
        }
      }
    };

    void bootstrap();
    return () => {
      cancelled = true;
    };
  }, [apiBaseUrl, applyProjects, wsBaseUrl]);

  useEffect(() => {
    if (authStatus !== "ready") {
      return;
    }
    wsClient.connect();
    return () => {
      wsClient.disconnect();
    };
  }, [authStatus, wsClient]);

  const setSelectedProjectId = useCallback((projectId: number | null) => {
    persistSelectedProjectId(projectId);
    setSelectedProjectIdState(projectId);
  }, []);

  const value = useMemo<V2WorkbenchContextValue>(() => {
    const selectedProject =
      selectedProjectId == null
        ? null
        : projects.find((project) => project.id === selectedProjectId) ?? null;

    return {
      apiClient,
      wsClient,
      authStatus,
      authError,
      projects,
      projectsLoading,
      projectsError,
      selectedProjectId,
      selectedProject,
      setSelectedProjectId,
      reloadProjects,
    };
  }, [
    apiClient,
    wsClient,
    authStatus,
    authError,
    projects,
    projectsLoading,
    projectsError,
    selectedProjectId,
    setSelectedProjectId,
    reloadProjects,
  ]);

  return <V2WorkbenchContext.Provider value={value}>{children}</V2WorkbenchContext.Provider>;
}

export const useV2Workbench = (): V2WorkbenchContextValue => {
  const value = useContext(V2WorkbenchContext);
  if (!value) {
    throw new Error("useV2Workbench must be used within V2WorkbenchProvider");
  }
  return value;
};

