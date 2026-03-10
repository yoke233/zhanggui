import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import type { ApiClient } from "../lib/apiClient";
import type { WsClient } from "../lib/wsClient";
import type { CreateProjectCreateRequest, ProjectSourceType } from "../types/api";
import type {
  ProjectCreateEventEnvelope,
  ProjectCreateEventType,
  WsEnvelope,
} from "../types/ws";

type WsStatus = ReturnType<WsClient["getStatus"]>;
type CreatePhase = "idle" | "pending" | "succeeded" | "failed";

interface ProjectAdminPanelProps {
  apiClient: ApiClient;
  wsClient: WsClient;
  wsStatus: WsStatus;
  onProjectCreated: (projectId?: string) => Promise<void> | void;
  pollIntervalMs?: number;
}

const PROJECT_CREATE_EVENT_TYPES = new Set<ProjectCreateEventType>([
  "project_create_started",
  "project_create_progress",
  "project_create_succeeded",
  "project_create_failed",
]);

const DEFAULT_POLL_INTERVAL_MS = 1500;

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error && error.message.trim().length > 0) {
    return error.message;
  }
  return "请求失败，请稍后重试";
};

const readString = (value: unknown): string | undefined => {
  if (typeof value !== "string") {
    return undefined;
  }
  const trimmed = value.trim();
  return trimmed.length > 0 ? trimmed : undefined;
};

const readProgress = (value: unknown): number | undefined => {
  if (typeof value === "number" && Number.isFinite(value)) {
    return Math.max(0, Math.min(100, value));
  }
  if (typeof value === "string") {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) {
      return Math.max(0, Math.min(100, parsed));
    }
  }
  return undefined;
};

const readObject = (value: unknown): Record<string, unknown> | null => {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return null;
  }
  return value as Record<string, unknown>;
};

const getEnvelopePayload = (
  envelope: WsEnvelope,
): Record<string, unknown> | null => {
  return readObject(envelope.data ?? envelope.payload);
};

const extractCreateEventDetail = (envelope: WsEnvelope) => {
  const payload = getEnvelopePayload(envelope);
  const topLevel = readObject(envelope);

  return {
    requestId:
      readString(payload?.request_id) ?? readString(topLevel?.request_id),
    projectId:
      readString(payload?.project_id) ??
      readString(envelope.project_id) ??
      undefined,
    message: readString(payload?.message),
    error: readString(payload?.error),
    progress: readProgress(payload?.progress),
  };
};

const ProjectAdminPanel = ({
  apiClient,
  wsClient,
  wsStatus,
  onProjectCreated,
  pollIntervalMs = DEFAULT_POLL_INTERVAL_MS,
}: ProjectAdminPanelProps) => {
  const [sourceType, setSourceType] = useState<ProjectSourceType>("local_path");
  const [name, setName] = useState("");
  const [repoPath, setRepoPath] = useState("");
  const [remoteUrl, setRemoteUrl] = useState("");
  const [ref, setRef] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [phase, setPhase] = useState<CreatePhase>("idle");
  const [requestId, setRequestId] = useState<string | null>(null);
  const [statusMessage, setStatusMessage] = useState<string | null>(null);
  const [failureMessage, setFailureMessage] = useState<string | null>(null);
  const [progress, setProgress] = useState<number | null>(null);
  const [createPanelOpen, setCreatePanelOpen] = useState(false);

  const activeRequestIdRef = useRef<string | null>(null);
  const doneNotifiedRef = useRef(false);

  useEffect(() => {
    activeRequestIdRef.current = requestId;
  }, [requestId]);

  const isBusy = submitting || phase === "pending";

  const canSubmit = useMemo(() => {
    if (isBusy || name.trim().length === 0) {
      return false;
    }
    if (sourceType === "local_path") {
      return repoPath.trim().length > 0;
    }
    if (sourceType === "github_clone") {
      return remoteUrl.trim().length > 0;
    }
    return true;
  }, [isBusy, name, remoteUrl, repoPath, sourceType]);

  const resolvePayload = useCallback((): CreateProjectCreateRequest | null => {
    const nextName = name.trim();
    if (nextName.length === 0) {
      setFailureMessage("请输入项目名称");
      return null;
    }

    if (sourceType === "local_path") {
      const nextRepoPath = repoPath.trim();
      if (nextRepoPath.length === 0) {
        setFailureMessage("请输入仓库路径");
        return null;
      }
      return {
        name: nextName,
        source_type: "local_path",
        repo_path: nextRepoPath,
      };
    }

    if (sourceType === "github_clone") {
      const nextRemoteURL = remoteUrl.trim();
      if (nextRemoteURL.length === 0) {
        setFailureMessage("请输入远程仓库地址");
        return null;
      }
      const payload: CreateProjectCreateRequest = {
        name: nextName,
        source_type: "github_clone",
        remote_url: nextRemoteURL,
      };
      const nextRef = ref.trim();
      if (nextRef.length > 0) {
        payload.ref = nextRef;
      }
      return payload;
    }

    return {
      name: nextName,
      source_type: "local_new",
    };
  }, [name, ref, remoteUrl, repoPath, sourceType]);

  const notifySucceeded = useCallback(
    async (projectId?: string, message?: string) => {
      setPhase("succeeded");
      setFailureMessage(null);
      setStatusMessage(message ?? "项目创建成功");
      if (doneNotifiedRef.current) {
        return;
      }
      doneNotifiedRef.current = true;
      try {
        await onProjectCreated(projectId);
      } catch (error) {
        setFailureMessage(`项目已创建，但刷新列表失败：${getErrorMessage(error)}`);
      }
    },
    [onProjectCreated],
  );

  const markFailed = useCallback((message: string) => {
    setPhase("failed");
    setFailureMessage(message);
  }, []);

  const handleSubmit = useCallback(
    async (event: FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (isBusy) {
        return;
      }

      const payload = resolvePayload();
      if (!payload) {
        return;
      }

      doneNotifiedRef.current = false;
      setSubmitting(true);
      setFailureMessage(null);
      setStatusMessage("正在提交创建请求...");
      setProgress(null);

      try {
        const response = await apiClient.createProjectCreateRequest(payload);
        const nextRequestId = response.request_id?.trim();
        if (!nextRequestId) {
          throw new Error("服务端未返回 request_id");
        }
        setRequestId(nextRequestId);
        setPhase("pending");
        setStatusMessage("创建请求已提交，等待执行...");
      } catch (error) {
        markFailed(getErrorMessage(error));
      } finally {
        setSubmitting(false);
      }
    },
    [apiClient, isBusy, markFailed, resolvePayload],
  );

  useEffect(() => {
    const unsubscribe = wsClient.subscribe<WsEnvelope>("*", (payload) => {
      const envelope = payload as ProjectCreateEventEnvelope;
      if (!PROJECT_CREATE_EVENT_TYPES.has(envelope.type)) {
        return;
      }

      const currentRequestId = activeRequestIdRef.current;
      if (!currentRequestId) {
        return;
      }

      const detail = extractCreateEventDetail(envelope);
      if (!detail.requestId || detail.requestId !== currentRequestId) {
        return;
      }

      if (detail.progress !== undefined) {
        setProgress(detail.progress);
      }

      if (envelope.type === "project_create_started") {
        setPhase("pending");
        setStatusMessage(detail.message ?? "开始创建项目...");
        return;
      }

      if (envelope.type === "project_create_progress") {
        setPhase("pending");
        setStatusMessage(detail.message ?? "项目创建中...");
        return;
      }

      if (envelope.type === "project_create_succeeded") {
        void notifySucceeded(detail.projectId, detail.message);
        return;
      }

      markFailed(detail.error ?? detail.message ?? "项目创建失败");
    });

    return () => {
      unsubscribe();
    };
  }, [markFailed, notifySucceeded, wsClient]);

  useEffect(() => {
    if (!requestId || phase !== "pending" || wsStatus === "open") {
      return;
    }

    let disposed = false;

    const pollOnce = async (): Promise<void> => {
      try {
        const result = await apiClient.getProjectCreateRequest(requestId);
        if (disposed) {
          return;
        }

        if (result.progress !== undefined) {
          const parsedProgress = readProgress(result.progress);
          if (parsedProgress !== undefined) {
            setProgress(parsedProgress);
          }
        }
        if (result.message) {
          setStatusMessage(result.message);
        }

        if (result.status === "succeeded") {
          await notifySucceeded(result.project_id, result.message);
          return;
        }
        if (result.status === "failed") {
          markFailed(result.error ?? result.message ?? "项目创建失败");
          return;
        }
        setPhase("pending");
      } catch (error) {
        if (disposed) {
          return;
        }
        setStatusMessage(`轮询状态失败：${getErrorMessage(error)}`);
      }
    };

    void pollOnce();

    const timer = window.setInterval(() => {
      void pollOnce();
    }, pollIntervalMs);

    return () => {
      disposed = true;
      window.clearInterval(timer);
    };
  }, [apiClient, markFailed, notifySucceeded, phase, pollIntervalMs, requestId, wsStatus]);

  return (
    <section className="mt-4 rounded-lg border border-slate-200 bg-slate-50 p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-sm font-semibold text-slate-900">项目管理</h2>
        {createPanelOpen ? (
          <button
            type="button"
            className="rounded-md border border-slate-300 bg-white px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-100"
            onClick={() => {
              setCreatePanelOpen(false);
            }}
          >
            关闭创建项目
          </button>
        ) : (
          <button
            type="button"
            className="rounded-md bg-slate-900 px-3 py-1.5 text-sm font-semibold text-white hover:bg-slate-800"
            onClick={() => {
              setCreatePanelOpen(true);
            }}
          >
            创建项目
          </button>
        )}
      </div>

      {createPanelOpen ? (
        <form className="mt-3 grid gap-3 md:grid-cols-2" onSubmit={handleSubmit}>
          <div className="flex flex-col gap-1">
            <label htmlFor="create-source-type" className="text-xs font-medium text-slate-700">
              项目来源
            </label>
            <select
              id="create-source-type"
              className="rounded-md border border-slate-300 bg-white px-3 py-2 text-sm"
              value={sourceType}
              onChange={(event) => {
                setSourceType(event.target.value as ProjectSourceType);
                setFailureMessage(null);
              }}
              disabled={isBusy}
            >
              <option value="local_path">本地已有仓库（local_path）</option>
              <option value="local_new">创建本地新仓库（local_new）</option>
              <option value="github_clone">从 GitHub 克隆（github_clone）</option>
            </select>
          </div>

          <div className="flex flex-col gap-1">
            <label htmlFor="create-project-name" className="text-xs font-medium text-slate-700">
              项目名称
            </label>
            <input
              id="create-project-name"
              className="rounded-md border border-slate-300 bg-white px-3 py-2 text-sm"
              value={name}
              onChange={(event) => {
                setName(event.target.value);
                setFailureMessage(null);
              }}
              placeholder="例如：demo-project"
              disabled={isBusy}
            />
          </div>

          {sourceType === "local_path" ? (
            <div className="flex flex-col gap-1 md:col-span-2">
              <label htmlFor="create-repo-path" className="text-xs font-medium text-slate-700">
                仓库路径
              </label>
              <input
                id="create-repo-path"
                className="rounded-md border border-slate-300 bg-white px-3 py-2 text-sm"
                value={repoPath}
                onChange={(event) => {
                  setRepoPath(event.target.value);
                  setFailureMessage(null);
                }}
                placeholder="例如：D:/repo/demo-project"
                disabled={isBusy}
              />
            </div>
          ) : null}

          {sourceType === "github_clone" ? (
            <>
              <div className="flex flex-col gap-1">
                <label htmlFor="create-remote-url" className="text-xs font-medium text-slate-700">
                  Remote URL
                </label>
                <input
                  id="create-remote-url"
                  className="rounded-md border border-slate-300 bg-white px-3 py-2 text-sm"
                  value={remoteUrl}
                  onChange={(event) => {
                    setRemoteUrl(event.target.value);
                    setFailureMessage(null);
                  }}
                  placeholder="例如：https://github.com/octocat/hello-world.git 或 git@github.com:octocat/hello-world.git"
                  disabled={isBusy}
                />
              </div>
              <div className="flex flex-col gap-1 md:col-span-2">
                <label htmlFor="create-github-ref" className="text-xs font-medium text-slate-700">
                  Git Ref（可选）
                </label>
                <input
                  id="create-github-ref"
                  className="rounded-md border border-slate-300 bg-white px-3 py-2 text-sm"
                  value={ref}
                  onChange={(event) => {
                    setRef(event.target.value);
                    setFailureMessage(null);
                  }}
                  placeholder="例如：main / v1.2.0 / 8f2a3bc"
                  disabled={isBusy}
                />
              </div>
            </>
          ) : null}

          <div className="flex flex-wrap items-center gap-3 md:col-span-2">
            <button
              type="submit"
              className="rounded-md bg-slate-900 px-4 py-2 text-sm font-semibold text-white disabled:cursor-not-allowed disabled:opacity-60"
              disabled={!canSubmit}
            >
              提交创建请求
            </button>
            {requestId ? (
              <span className="text-xs text-slate-600">请求 ID: {requestId}</span>
            ) : null}
            {phase === "pending" ? (
              <span className="text-xs text-slate-600">等待中（WS: {wsStatus}）</span>
            ) : null}
          </div>
        </form>
      ) : null}

      {statusMessage ? (
        <p className="mt-3 rounded-md border border-sky-200 bg-sky-50 px-3 py-2 text-sm text-sky-800">
          {statusMessage}
          {progress !== null ? `（${Math.round(progress)}%）` : ""}
        </p>
      ) : null}

      {failureMessage ? (
        <p className="mt-3 rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">
          {failureMessage}
        </p>
      ) : null}
    </section>
  );
};

export default ProjectAdminPanel;
