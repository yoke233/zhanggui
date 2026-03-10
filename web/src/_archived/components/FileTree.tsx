import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ApiClient } from "../lib/apiClient";
import type { FileEntry, RepoStatusResponse } from "../types/api";

interface FileTreeProps {
  apiClient: ApiClient;
  projectId: string;
  selectedFiles: string[];
  onToggleFile: (filePath: string, selected: boolean) => void;
}

type EntriesByDir = Record<string, FileEntry[]>;
type BoolMap = Record<string, boolean>;
type StatusMap = Record<string, string>;

const ROOT_DIR = "";

const STATUS_BADGE_CLASS: Record<string, string> = {
  M: "border-amber-200 bg-amber-50 text-amber-700",
  A: "border-emerald-200 bg-emerald-50 text-emerald-700",
  D: "border-rose-200 bg-rose-50 text-rose-700",
  R: "border-violet-200 bg-violet-50 text-violet-700",
  "?": "border-slate-200 bg-slate-100 text-slate-700",
};

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error && error.message.trim().length > 0) {
    return error.message;
  }
  return "加载仓库文件树失败";
};

const normalizePath = (path: string): string => {
  const unified = path.replace(/\\/g, "/").trim();
  return unified.replace(/^\.\/+/, "");
};

const normalizeDir = (dir?: string): string => {
  if (!dir) {
    return ROOT_DIR;
  }
  return normalizePath(dir);
};

const normalizeGitStatus = (rawStatus: string | undefined): string => {
  const status = (rawStatus ?? "").trim().toUpperCase();
  if (status === "??" || status === "?") {
    return "?";
  }
  if (status.includes("M")) {
    return "M";
  }
  if (status.includes("A")) {
    return "A";
  }
  if (status.includes("D")) {
    return "D";
  }
  if (status.includes("R")) {
    return "R";
  }
  return "";
};

const inferName = (entry: FileEntry): string => {
  if (typeof entry.name === "string" && entry.name.trim().length > 0) {
    return entry.name.trim();
  }
  const normalizedPath = normalizePath(entry.path);
  const segments = normalizedPath.split("/");
  return segments[segments.length - 1] ?? normalizedPath;
};

const sortEntries = (entries: FileEntry[]): FileEntry[] => {
  return [...entries].sort((a, b) => {
    if (a.type !== b.type) {
      return a.type === "dir" ? -1 : 1;
    }
    return inferName(a).localeCompare(inferName(b), "zh-CN");
  });
};

const toStatusMap = (response: RepoStatusResponse): StatusMap => {
  const map: StatusMap = {};
  (Array.isArray(response.items) ? response.items : []).forEach((entry) => {
    if (entry.type !== "file") {
      return;
    }
    const status = normalizeGitStatus(entry.git_status);
    if (!status) {
      return;
    }
    map[normalizePath(entry.path)] = status;
  });
  return map;
};

const FileTree = ({ apiClient, projectId, selectedFiles, onToggleFile }: FileTreeProps) => {
  const [entriesByDir, setEntriesByDir] = useState<EntriesByDir>({});
  const [expandedDirs, setExpandedDirs] = useState<BoolMap>({ [ROOT_DIR]: true });
  const [loadedDirs, setLoadedDirs] = useState<BoolMap>({});
  const [loadingDirs, setLoadingDirs] = useState<BoolMap>({});
  const [statusByPath, setStatusByPath] = useState<StatusMap>({});
  const [error, setError] = useState<string | null>(null);
  const requestIdRef = useRef(0);

  const selectedFileSet = useMemo(() => {
    return new Set(selectedFiles.map((filePath) => normalizePath(filePath)));
  }, [selectedFiles]);

  const loadDir = useCallback(
    async (dir?: string, requestId = requestIdRef.current) => {
      const dirKey = normalizeDir(dir);
      setLoadingDirs((prev) => ({ ...prev, [dirKey]: true }));
      setError(null);
      try {
        const response = await apiClient.getRepoTree(projectId, dirKey || undefined);
        if (requestIdRef.current !== requestId) {
          return;
        }
        const normalizedEntries = (Array.isArray(response.items) ? response.items : []).map(
          (entry) => {
            const path = normalizePath(entry.path);
            return {
              ...entry,
              path,
              name: inferName(entry),
            };
          },
        );
        setEntriesByDir((prev) => ({
          ...prev,
          [dirKey]: sortEntries(normalizedEntries),
        }));
        setLoadedDirs((prev) => ({ ...prev, [dirKey]: true }));
      } catch (loadError) {
        if (requestIdRef.current !== requestId) {
          return;
        }
        setError(getErrorMessage(loadError));
      } finally {
        if (requestIdRef.current === requestId) {
          setLoadingDirs((prev) => ({ ...prev, [dirKey]: false }));
        }
      }
    },
    [apiClient, projectId],
  );

  useEffect(() => {
    requestIdRef.current += 1;
    const requestId = requestIdRef.current;
    setEntriesByDir({});
    setExpandedDirs({ [ROOT_DIR]: true });
    setLoadedDirs({});
    setLoadingDirs({});
    setStatusByPath({});
    setError(null);
    void loadDir(ROOT_DIR, requestId);
    void (async () => {
      try {
        const response = await apiClient.getRepoStatus(projectId);
        if (requestIdRef.current !== requestId) {
          return;
        }
        setStatusByPath(toStatusMap(response));
      } catch {
        if (requestIdRef.current !== requestId) {
          return;
        }
        setStatusByPath({});
      }
    })();
  }, [apiClient, loadDir, projectId]);

  const toggleDir = useCallback(
    (dirPath: string) => {
      const normalizedPath = normalizeDir(dirPath);
      setExpandedDirs((prev) => {
        const nextExpanded = !prev[normalizedPath];
        if (nextExpanded && !loadedDirs[normalizedPath] && !loadingDirs[normalizedPath]) {
          void loadDir(normalizedPath);
        }
        return {
          ...prev,
          [normalizedPath]: nextExpanded,
        };
      });
    },
    [loadDir, loadedDirs, loadingDirs],
  );

  const renderEntries = useCallback(
    (dirPath: string, depth: number): JSX.Element => {
      const dirKey = normalizeDir(dirPath);
      const entries = entriesByDir[dirKey] ?? [];
      if (entries.length === 0 && loadedDirs[dirKey]) {
        return (
          <p className="py-2 text-xs text-slate-500" style={{ paddingLeft: `${depth * 14}px` }}>
            空目录
          </p>
        );
      }

      return (
        <ul className="space-y-1">
          {entries.map((entry) => {
            const entryPath = normalizePath(entry.path);
            const status = normalizeGitStatus(entry.git_status) || statusByPath[entryPath] || "";
            if (entry.type === "dir") {
              const expanded = !!expandedDirs[entryPath];
              const isLoading = !!loadingDirs[entryPath];
              return (
                <li key={`dir-${entryPath}`}>
                  <button
                    type="button"
                    className="flex w-full items-center gap-2 rounded px-1 py-1 text-left text-sm text-slate-700 hover:bg-slate-100"
                    style={{ paddingLeft: `${depth * 14 + 6}px` }}
                    aria-label={`${expanded ? "折叠目录" : "展开目录"} ${entryPath}`}
                    onClick={() => {
                      toggleDir(entryPath);
                    }}
                  >
                    <span className="font-mono text-xs">{expanded ? "▾" : "▸"}</span>
                    <span className="truncate">{inferName(entry)}</span>
                    {isLoading ? <span className="text-xs text-slate-500">加载中...</span> : null}
                  </button>
                  {expanded ? renderEntries(entryPath, depth + 1) : null}
                </li>
              );
            }

            return (
              <li
                key={`file-${entryPath}`}
                className="flex items-center gap-2 rounded px-1 py-1 text-sm hover:bg-slate-100"
                style={{ paddingLeft: `${depth * 14 + 22}px` }}
              >
                <input
                  type="checkbox"
                  className="h-4 w-4"
                  checked={selectedFileSet.has(entryPath)}
                  aria-label={`选择文件 ${entryPath}`}
                  onChange={(event) => {
                    onToggleFile(entryPath, event.target.checked);
                  }}
                />
                <span className="min-w-0 flex-1 truncate text-slate-700">{inferName(entry)}</span>
                {status ? (
                  <span
                    title={`状态 ${status}`}
                    className={`rounded border px-1.5 py-0.5 text-[10px] font-semibold ${
                      STATUS_BADGE_CLASS[status] ?? STATUS_BADGE_CLASS["?"]
                    }`}
                  >
                    {status}
                  </span>
                ) : null}
              </li>
            );
          })}
        </ul>
      );
    },
    [
      entriesByDir,
      expandedDirs,
      loadedDirs,
      loadingDirs,
      onToggleFile,
      selectedFileSet,
      statusByPath,
      toggleDir,
    ],
  );

  const loadingRoot = !!loadingDirs[ROOT_DIR] && !loadedDirs[ROOT_DIR];

  return (
    <div className="rounded-md border border-slate-200 bg-white p-2">
      {loadingRoot ? <p className="px-2 py-3 text-xs text-slate-500">加载文件树中...</p> : null}
      {error ? (
        <p className="mx-1 mb-2 rounded border border-rose-200 bg-rose-50 px-2 py-1 text-xs text-rose-700">
          {error}
        </p>
      ) : null}
      {renderEntries(ROOT_DIR, 0)}
    </div>
  );
};

export default FileTree;
