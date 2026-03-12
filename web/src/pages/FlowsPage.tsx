import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import {
  ChevronDown,
  Columns3,
  Folder,
  List,
  Loader2,
  Plus,
  Search,
  Tag,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { formatRelativeTime, getErrorMessage } from "@/lib/v2Workbench";
import { cn } from "@/lib/utils";
import type { Issue, IssuePriority, IssueStatus } from "@/types/apiV2";

/* ── Kanban column definitions ── */

interface KanbanColumn {
  key: string;
  statuses: IssueStatus[];
  color: string;        // dot color
  bgHover: string;      // header hover tint
}

const KANBAN_COLUMNS: KanbanColumn[] = [
  { key: "open",        statuses: ["open"],                       color: "bg-blue-500",    bgHover: "hover:bg-blue-50" },
  { key: "accepted",    statuses: ["accepted", "queued"],         color: "bg-amber-500",   bgHover: "hover:bg-amber-50" },
  { key: "in_progress", statuses: ["running", "blocked", "failed"], color: "bg-violet-500",  bgHover: "hover:bg-violet-50" },
  { key: "done",        statuses: ["done"],                       color: "bg-emerald-500", bgHover: "hover:bg-emerald-50" },
  { key: "closed",      statuses: ["closed", "cancelled"],        color: "bg-zinc-400",    bgHover: "hover:bg-zinc-50" },
];

/* ── Priority badge ── */

const priorityConfig: Record<IssuePriority, { label: string; text: string; bg: string }> = {
  urgent: { label: "紧急", text: "text-red-500",    bg: "bg-red-50" },
  high:   { label: "高",   text: "text-amber-500",  bg: "bg-amber-50" },
  medium: { label: "中",   text: "text-blue-500",   bg: "bg-blue-50" },
  low:    { label: "低",   text: "text-zinc-500",   bg: "bg-zinc-50" },
};

function PriorityBadge({ priority }: { priority: IssuePriority }) {
  const cfg = priorityConfig[priority] ?? priorityConfig.medium;
  return (
    <span className={cn("rounded px-1.5 py-0.5 text-[11px] font-medium", cfg.text, cfg.bg)}>
      {cfg.label}
    </span>
  );
}

function LabelBadge({ label }: { label: string }) {
  return (
    <span className="rounded bg-blue-50 px-1.5 py-0.5 text-[11px] font-medium text-blue-500">
      {label}
    </span>
  );
}

/* ── Issue Card ── */

function IssueCard({ issue, projectName }: { issue: Issue; projectName?: string }) {
  return (
    <Link
      to={`/work-items/${issue.id}`}
      className="block rounded-md border bg-white p-3 transition-shadow hover:shadow-sm"
    >
      {projectName && (
        <div className="mb-1 flex items-center gap-1 text-[11px] text-muted-foreground">
          <Folder className="h-3 w-3" />
          <span>{projectName}</span>
        </div>
      )}
      <p className="text-[13px] font-medium leading-snug text-foreground">{issue.title}</p>
      {issue.body && (
        <p className="mt-1 line-clamp-2 text-xs leading-relaxed text-muted-foreground">{issue.body}</p>
      )}
      <div className="mt-2 flex items-center justify-between">
        <PriorityBadge priority={issue.priority} />
        <div className="flex items-center gap-1">
          {issue.labels?.slice(0, 2).map((label) => (
            <LabelBadge key={label} label={label} />
          ))}
        </div>
      </div>
    </Link>
  );
}

/* ── Main page ── */

export function IssuesPage() {
  const { t } = useTranslation();
  const { apiClient, selectedProject, selectedProjectId, projects } = useWorkbench();
  const [search, setSearch] = useState("");
  const [issues, setIssues] = useState<Issue[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [viewMode, setViewMode] = useState<"kanban" | "list">("kanban");
  const [priorityFilter, setPriorityFilter] = useState<IssuePriority | "all">("all");
  const [labelFilter, setLabelFilter] = useState<string>("all");

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        const listed = await apiClient.listIssues({
          project_id: selectedProjectId ?? undefined,
          archived: false,
          limit: 200,
          offset: 0,
        });
        if (!cancelled) setIssues(listed);
      } catch (loadError) {
        if (!cancelled) setError(getErrorMessage(loadError));
      } finally {
        if (!cancelled) setLoading(false);
      }
    };
    void load();
    return () => { cancelled = true; };
  }, [apiClient, selectedProjectId]);

  const projectNameMap = useMemo(() => {
    const map = new Map<number, string>();
    for (const p of projects) map.set(p.id, p.name);
    return map;
  }, [projects]);

  const allLabels = useMemo(() => {
    const set = new Set<string>();
    for (const issue of issues) {
      for (const label of issue.labels ?? []) set.add(label);
    }
    return Array.from(set).sort();
  }, [issues]);

  const filtered = useMemo(() =>
    issues.filter((issue) => {
      if (search && !issue.title.toLowerCase().includes(search.toLowerCase()) && !String(issue.id).includes(search)) return false;
      if (priorityFilter !== "all" && issue.priority !== priorityFilter) return false;
      if (labelFilter !== "all" && !(issue.labels ?? []).includes(labelFilter)) return false;
      return true;
    }),
    [issues, search, priorityFilter, labelFilter],
  );

  const columnData = useMemo(() =>
    KANBAN_COLUMNS.map((col) => ({
      ...col,
      items: filtered.filter((issue) => col.statuses.includes(issue.status)),
    })),
    [filtered],
  );

  return (
    <div className="flex h-full flex-col overflow-hidden">
      {/* Header */}
      <div className="shrink-0 space-y-4 px-8 pt-8">
        <div className="flex items-center justify-between">
          <div>
            <div className="flex items-center gap-2">
              <h1 className="text-2xl font-bold tracking-tight">{t("workItems.title")}</h1>
              {loading && <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />}
            </div>
            <p className="text-sm text-muted-foreground">{t("workItems.subtitle")}</p>
          </div>
          <div className="flex items-center gap-2">
            {/* View toggle */}
            <div className="flex overflow-hidden rounded-md border">
              <button
                type="button"
                className={cn(
                  "flex items-center gap-1.5 px-3 py-1.5 text-[13px] font-medium transition-colors",
                  viewMode === "kanban" ? "bg-foreground text-background" : "text-muted-foreground hover:text-foreground",
                )}
                onClick={() => setViewMode("kanban")}
              >
                <Columns3 className="h-4 w-4" />
                {t("workItems.kanban")}
              </button>
              <button
                type="button"
                className={cn(
                  "flex items-center gap-1.5 px-3 py-1.5 text-[13px] font-medium transition-colors",
                  viewMode === "list" ? "bg-foreground text-background" : "text-muted-foreground hover:text-foreground",
                )}
                onClick={() => setViewMode("list")}
              >
                <List className="h-4 w-4" />
                {t("workItems.list")}
              </button>
            </div>
            <Link to="/work-items/new">
              <Button size="sm">
                <Plus className="mr-1.5 h-4 w-4" />
                {t("workItems.new")}
              </Button>
            </Link>
          </div>
        </div>

        {/* Filter bar */}
        <div className="flex items-center gap-3">
          <div className="relative w-60">
            <Search className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder={t("workItems.searchPlaceholder")}
              className="h-9 pl-8 text-[13px]"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
          {!selectedProject && (
            <div className="flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-[13px]">
              <Folder className="h-3.5 w-3.5 text-muted-foreground" />
              <span>{t("workItems.allProjects")}</span>
              <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
            </div>
          )}
          <select
            className="h-9 rounded-md border bg-background px-3 text-[13px]"
            value={priorityFilter}
            onChange={(e) => setPriorityFilter(e.target.value as IssuePriority | "all")}
          >
            <option value="all">{t("workItems.allPriorities")}</option>
            <option value="urgent">{t("workItems.priorityUrgent")}</option>
            <option value="high">{t("workItems.priorityHigh")}</option>
            <option value="medium">{t("workItems.priorityMedium")}</option>
            <option value="low">{t("workItems.priorityLow")}</option>
          </select>
          {allLabels.length > 0 && (
            <div className="flex items-center gap-1.5">
              <Tag className="h-3.5 w-3.5 text-muted-foreground" />
              <select
                className="h-9 rounded-md border bg-background px-3 text-[13px]"
                value={labelFilter}
                onChange={(e) => setLabelFilter(e.target.value)}
              >
                <option value="all">{t("workItems.allLabels")}</option>
                {allLabels.map((label) => (
                  <option key={label} value={label}>{label}</option>
                ))}
              </select>
            </div>
          )}
        </div>
      </div>

      {error && (
        <p className="mx-8 mt-4 rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p>
      )}

      {/* Content */}
      <div className="flex-1 overflow-auto px-8 pb-8 pt-4">
        {viewMode === "kanban" ? (
          /* ── Kanban Board ── */
          <div className="flex h-full gap-4">
            {columnData.map((col) => (
              <div key={col.key} className="flex min-w-[220px] flex-1 flex-col rounded-lg bg-muted/50 p-3">
                {/* Column header */}
                <div className="mb-3 flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <span className={cn("h-2 w-2 rounded-full", col.color)} />
                    <span className="text-[13px] font-semibold">{t(`workItems.col_${col.key}`)}</span>
                  </div>
                  <span className="rounded-full bg-background px-2 py-0.5 text-xs font-medium text-muted-foreground">
                    {col.items.length}
                  </span>
                </div>
                {/* Cards */}
                <div className="flex-1 space-y-2 overflow-y-auto">
                  {col.items.map((issue) => (
                    <IssueCard
                      key={issue.id}
                      issue={issue}
                      projectName={issue.project_id != null ? projectNameMap.get(issue.project_id) : undefined}
                    />
                  ))}
                  {col.items.length === 0 && (
                    <p className="py-6 text-center text-xs text-muted-foreground">{t("workItems.empty")}</p>
                  )}
                </div>
              </div>
            ))}
          </div>
        ) : (
          /* ── List View ── */
          <div className="rounded-lg border">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b bg-muted/30">
                  <th className="px-4 py-2.5 text-left text-xs font-medium text-muted-foreground">{t("workItems.titleCol")}</th>
                  <th className="px-4 py-2.5 text-left text-xs font-medium text-muted-foreground">{t("common.status")}</th>
                  <th className="px-4 py-2.5 text-left text-xs font-medium text-muted-foreground">{t("workItems.priority")}</th>
                  <th className="px-4 py-2.5 text-left text-xs font-medium text-muted-foreground">{t("workItems.labels")}</th>
                  <th className="px-4 py-2.5 text-left text-xs font-medium text-muted-foreground">{t("workItems.updated")}</th>
                </tr>
              </thead>
              <tbody>
                {filtered.length === 0 ? (
                  <tr>
                    <td colSpan={5} className="px-4 py-8 text-center text-muted-foreground">{t("workItems.empty")}</td>
                  </tr>
                ) : (
                  filtered.map((issue) => (
                    <tr key={issue.id} className="border-b transition-colors hover:bg-muted/30">
                      <td className="px-4 py-2.5">
                        <Link to={`/work-items/${issue.id}`} className="font-medium hover:underline">{issue.title}</Link>
                        {issue.project_id != null && projectNameMap.get(issue.project_id) && (
                          <span className="ml-2 text-xs text-muted-foreground">{projectNameMap.get(issue.project_id)}</span>
                        )}
                      </td>
                      <td className="px-4 py-2.5">
                        <span className="rounded-full bg-muted px-2 py-0.5 text-xs font-medium">{issue.status}</span>
                      </td>
                      <td className="px-4 py-2.5"><PriorityBadge priority={issue.priority} /></td>
                      <td className="px-4 py-2.5">
                        <div className="flex gap-1">
                          {(issue.labels ?? []).slice(0, 3).map((label) => <LabelBadge key={label} label={label} />)}
                        </div>
                      </td>
                      <td className="px-4 py-2.5 text-xs text-muted-foreground">{formatRelativeTime(issue.updated_at)}</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}

// Keep backward-compatible export
export { IssuesPage as FlowsPage };
