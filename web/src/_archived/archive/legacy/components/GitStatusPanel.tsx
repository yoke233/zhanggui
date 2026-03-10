import { useEffect, useMemo, useRef, useState } from "react";
import DiffViewer from "@/components/DiffViewer";
import type { ApiClient } from "@/lib/apiClient";
import type { FileEntry, RepoStatusResponse } from "@/types/api";

interface GitStatusPanelProps {
  apiClient: ApiClient;
  projectId: string;
}

interface StatusGroup {
  key: "modified" | "added" | "deleted" | "renamed" | "untracked";
  label: string;
  badge: string;
  files: string[];
}

const EMPTY_STATUS: RepoStatusResponse = {
  items: [],
};

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error && error.message.trim().length > 0) {
    return error.message;
  }
  return "加载 Git 状态失败";
};

const normalizeGitStatus = (rawStatus: string | undefined): "M" | "A" | "D" | "R" | "?" | "" => {
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

const groupStatusEntries = (entries: FileEntry[]): Record<StatusGroup["key"], string[]> => {
  const grouped: Record<StatusGroup["key"], string[]> = {
    modified: [],
    added: [],
    deleted: [],
    renamed: [],
    untracked: [],
  };

  entries.forEach((entry) => {
    if (entry.type !== "file") {
      return;
    }
    const status = normalizeGitStatus(entry.git_status);
    switch (status) {
      case "M":
        grouped.modified.push(entry.path);
        break;
      case "A":
        grouped.added.push(entry.path);
        break;
      case "D":
        grouped.deleted.push(entry.path);
        break;
      case "R":
        grouped.renamed.push(entry.path);
        break;
      case "?":
        grouped.untracked.push(entry.path);
        break;
      default:
        break;
    }
  });

  (Object.keys(grouped) as Array<StatusGroup["key"]>).forEach((key) => {
    grouped[key].sort((a, b) => a.localeCompare(b, "zh-CN"));
  });
  return grouped;
};

const GitStatusPanel = ({ apiClient, projectId }: GitStatusPanelProps) => {
  const [status, setStatus] = useState<RepoStatusResponse>(EMPTY_STATUS);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [activeFile, setActiveFile] = useState<string | null>(null);
  const [diffByFile, setDiffByFile] = useState<Record<string, string>>({});
  const [diffLoadedByFile, setDiffLoadedByFile] = useState<Record<string, boolean>>({});
  const [diffLoadingByFile, setDiffLoadingByFile] = useState<Record<string, boolean>>({});
  const [diffError, setDiffError] = useState<string | null>(null);
  const requestIdRef = useRef(0);
  const diffLoadingByFileRef = useRef<Record<string, boolean>>({});

  useEffect(() => {
    requestIdRef.current += 1;
    const requestId = requestIdRef.current;
    setLoading(true);
    setError(null);
    setActiveFile(null);
    setDiffByFile({});
    setDiffLoadedByFile({});
    setDiffLoadingByFile({});
    diffLoadingByFileRef.current = {};
    setDiffError(null);

    void (async () => {
      try {
        const response = await apiClient.getRepoStatus(projectId);
        if (requestIdRef.current !== requestId) {
          return;
        }
        setStatus({
          items: Array.isArray(response.items) ? response.items : [],
        });
      } catch (loadError) {
        if (requestIdRef.current !== requestId) {
          return;
        }
        setStatus(EMPTY_STATUS);
        setError(getErrorMessage(loadError));
      } finally {
        if (requestIdRef.current === requestId) {
          setLoading(false);
        }
      }
    })();
  }, [apiClient, projectId]);

  const groups = useMemo<StatusGroup[]>(
    () => {
      const grouped = groupStatusEntries(Array.isArray(status.items) ? status.items : []);
      return [
        {
          key: "modified",
          label: "Modified",
          badge: "M",
          files: grouped.modified,
        },
        {
          key: "added",
          label: "Added",
          badge: "A",
          files: grouped.added,
        },
        {
          key: "deleted",
          label: "Deleted",
          badge: "D",
          files: grouped.deleted,
        },
        {
          key: "renamed",
          label: "Renamed",
          badge: "R",
          files: grouped.renamed,
        },
        {
          key: "untracked",
          label: "Untracked",
          badge: "?",
          files: grouped.untracked,
        },
      ];
    },
    [status],
  );

  const handleFileClick = async (filePath: string) => {
    setActiveFile(filePath);
    setDiffError(null);
    if (diffLoadedByFile[filePath] || diffLoadingByFileRef.current[filePath]) {
      return;
    }

    const requestId = requestIdRef.current;
    const nextLoading = {
      ...diffLoadingByFileRef.current,
      [filePath]: true,
    };
    diffLoadingByFileRef.current = nextLoading;
    setDiffLoadingByFile(nextLoading);
    try {
      const response = await apiClient.getRepoDiff(projectId, filePath);
      if (requestIdRef.current !== requestId) {
        return;
      }
      setDiffByFile((prev) => ({
        ...prev,
        [filePath]: response.diff ?? "",
      }));
      setDiffLoadedByFile((prev) => ({
        ...prev,
        [filePath]: true,
      }));
    } catch (loadError) {
      if (requestIdRef.current !== requestId) {
        return;
      }
      setDiffError(getErrorMessage(loadError));
    } finally {
      if (requestIdRef.current === requestId) {
        setDiffLoadingByFile((prev) => {
          if (!prev[filePath]) {
            diffLoadingByFileRef.current = prev;
            return prev;
          }
          const updated = { ...prev };
          delete updated[filePath];
          diffLoadingByFileRef.current = updated;
          return updated;
        });
      }
    }
  };

  return (
    <div className="rounded-md border border-slate-200 bg-white p-3">
      {loading ? <p className="text-xs text-slate-500">加载 Git 状态中...</p> : null}
      {error ? (
        <p className="mb-2 rounded border border-rose-200 bg-rose-50 px-2 py-1 text-xs text-rose-700">
          {error}
        </p>
      ) : null}

      <div className="space-y-3">
        {groups.map((group) => (
          <section key={group.key}>
            <h4 className="mb-1 text-xs font-semibold text-slate-700">
              {group.label} ({group.files.length})
            </h4>
            {group.files.length === 0 ? (
              <p className="text-xs text-slate-400">暂无文件</p>
            ) : (
              <ul className="space-y-1">
                {group.files.map((filePath) => {
                  const isActive = activeFile === filePath;
                  return (
                    <li key={`${group.key}-${filePath}`}>
                      <button
                        type="button"
                        className={`flex w-full items-center gap-2 rounded px-2 py-1 text-left text-sm ${
                          isActive
                            ? "bg-slate-100 text-slate-900"
                            : "text-slate-700 hover:bg-slate-50"
                        }`}
                        onClick={() => {
                          void handleFileClick(filePath);
                        }}
                      >
                        <span className="rounded border border-slate-200 bg-slate-100 px-1.5 py-0.5 text-[10px] font-semibold text-slate-700">
                          {group.badge}
                        </span>
                        <span className="truncate">{filePath}</span>
                      </button>
                      {isActive ? (
                        <div className="mt-2 pl-2">
                          {diffLoadingByFile[filePath] ? (
                            <p className="text-xs text-slate-500">加载 diff 中...</p>
                          ) : null}
                          {diffError ? (
                            <p className="mb-2 rounded border border-rose-200 bg-rose-50 px-2 py-1 text-xs text-rose-700">
                              {diffError}
                            </p>
                          ) : null}
                          {diffLoadedByFile[filePath] ? (
                            <DiffViewer diff={diffByFile[filePath] ?? ""} filePath={filePath} />
                          ) : null}
                        </div>
                      ) : null}
                    </li>
                  );
                })}
              </ul>
            )}
          </section>
        ))}
      </div>
    </div>
  );
};

export default GitStatusPanel;
