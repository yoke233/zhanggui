import { useEffect, useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import type { ApiClientV2 } from "@/lib/apiClientV2";
import { PageScaffold } from "@/v3/components/PageScaffold";
import type { Event, Project, ResourceBinding, StatsResponse } from "@/types/apiV2";

interface OpsViewProps {
  apiClient: ApiClientV2;
  projects: Project[];
  projectsLoading: boolean;
  projectsError: string | null;
  selectedProjectId: number | null;
  onSelectProject: (projectId: number | null) => void;
  onRefreshProjects: () => void;
  onProjectCreated: (projectId?: number) => Promise<void> | void;
  resources: ResourceBinding[];
  resourcesLoading: boolean;
  resourcesError: string | null;
  onRefreshResources: () => Promise<void> | void;
}

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error && error.message.trim().length > 0) {
    return error.message;
  }
  return "请求失败，请稍后重试";
};

const formatTime = (value?: string) => {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString("zh-CN", { hour12: false });
};

const OpsView = ({
  apiClient,
  projects,
  projectsLoading,
  projectsError,
  selectedProjectId,
  onSelectProject,
  onRefreshProjects,
  onProjectCreated,
  resources,
  resourcesLoading,
  resourcesError,
  onRefreshResources,
}: OpsViewProps) => {
  const [stats, setStats] = useState<StatsResponse | null>(null);
  const [events, setEvents] = useState<Event[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [systemEvent, setSystemEvent] = useState("ops_notice");
  const [systemMessage, setSystemMessage] = useState("");
  const [sending, setSending] = useState(false);
  const [feedback, setFeedback] = useState<string | null>(null);

  const [newProjectName, setNewProjectName] = useState("");
  const [newProjectKind, setNewProjectKind] = useState<"dev" | "general" | string>("general");
  const [newProjectDescription, setNewProjectDescription] = useState("");
  const [creatingProject, setCreatingProject] = useState(false);
  const [projectFeedback, setProjectFeedback] = useState<string | null>(null);

  const [newResourceKind, setNewResourceKind] = useState("local_fs");
  const [newResourceURI, setNewResourceURI] = useState("");
  const [creatingResource, setCreatingResource] = useState(false);
  const [resourceFeedback, setResourceFeedback] = useState<string | null>(null);

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      const [nextStats, nextEvents] = await Promise.all([
        apiClient.getStats(),
        apiClient.listEvents({ types: ["admin.system_event"], limit: 50, offset: 0 }),
      ]);
      setStats(nextStats);
      setEvents(Array.isArray(nextEvents) ? nextEvents : []);
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiClient]);

  const statsCards = useMemo(() => {
    if (!stats) {
      return null;
    }
    const successRate = typeof stats.success_rate === "number" ? `${Math.round(stats.success_rate * 100)}%` : "--";
    return [
      { label: "Total Flows", value: String(stats.total_flows ?? 0) },
      { label: "Active Flows", value: String(stats.active_flows ?? 0) },
      { label: "Success Rate", value: successRate },
      { label: "Avg Duration", value: String(stats.avg_duration ?? "-") },
    ];
  }, [stats]);

  const handleSend = async () => {
    if (!systemEvent.trim()) {
      setFeedback("系统事件名不能为空。");
      return;
    }
    setSending(true);
    setFeedback(null);
    setError(null);
    try {
      await apiClient.sendSystemEvent({
        event: systemEvent.trim(),
        data: systemMessage.trim() ? { message: systemMessage.trim() } : undefined,
      });
      setSystemMessage("");
      setFeedback(`系统事件 ${systemEvent.trim()} 已发送。`);
      const nextEvents = await apiClient.listEvents({ types: ["admin.system_event"], limit: 50, offset: 0 });
      setEvents(Array.isArray(nextEvents) ? nextEvents : []);
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setSending(false);
    }
  };

  const handleCreateProject = async () => {
    const name = newProjectName.trim();
    if (!name) {
      setProjectFeedback("项目名称不能为空。");
      return;
    }
    setCreatingProject(true);
    setProjectFeedback(null);
    try {
      const created = await apiClient.createProject({
        name,
        kind: String(newProjectKind || "").trim() || "general",
        description: newProjectDescription.trim() || undefined,
      });
      setNewProjectName("");
      setNewProjectKind("general");
      setNewProjectDescription("");
      setProjectFeedback(`已创建项目 #${created.id}`);
      onSelectProject(created.id);
      await onProjectCreated(created.id);
    } catch (err) {
      setProjectFeedback(getErrorMessage(err));
    } finally {
      setCreatingProject(false);
    }
  };

  const selectedProject = selectedProjectId != null ? projects.find((p) => p.id === selectedProjectId) ?? null : null;

  const handleCreateResource = async () => {
    if (selectedProjectId == null) {
      setResourceFeedback("请先选择一个项目。");
      return;
    }
    const kind = newResourceKind.trim();
    const uri = newResourceURI.trim();
    if (!kind) {
      setResourceFeedback("kind 不能为空。");
      return;
    }
    if (!uri) {
      setResourceFeedback("uri 不能为空。");
      return;
    }
    setCreatingResource(true);
    setResourceFeedback(null);
    try {
      await apiClient.createProjectResource(selectedProjectId, { kind, uri });
      setNewResourceURI("");
      setResourceFeedback("已添加资源绑定。");
      await onRefreshResources();
    } catch (err) {
      setResourceFeedback(getErrorMessage(err));
    } finally {
      setCreatingResource(false);
    }
  };

  const handleDeleteResource = async (resourceId: number) => {
    setError(null);
    try {
      await apiClient.deleteResource(resourceId);
      await onRefreshResources();
    } catch (err) {
      setError(getErrorMessage(err));
    }
  };

  return (
    <PageScaffold
      eyebrow="Ops / 运维"
      title="统计 / 控制 / 项目"
      description="把项目创建、统计与控制性入口集中在这里，避免在业务页散落高权限动作。"
      contextTitle={selectedProject ? `项目：${selectedProject.name}` : "项目：未选择"}
      contextMeta={selectedProject ? `kind=${selectedProject.kind}${selectedProject.description ? ` · ${selectedProject.description}` : ""}` : "可在下方创建或切换项目"}
      actions={[
        { label: "刷新统计", onClick: () => void load(), variant: "outline" },
        { label: "刷新项目", onClick: onRefreshProjects, variant: "outline" },
        { label: "刷新资源", onClick: () => void onRefreshResources(), variant: "outline" },
      ]}
      stats={
        statsCards
          ? statsCards.map((card) => ({
              label: card.label,
              value: card.value,
              helper: "",
            }))
          : undefined
      }
    >
      {error ? (
        <p className="rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700">
          {error}
        </p>
      ) : null}

      <Card className="rounded-2xl border-slate-200 shadow-none">
        <CardHeader className="p-5">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div>
              <Badge variant="secondary" className="bg-indigo-50 text-indigo-600">
                Projects
              </Badge>
              <CardTitle className="mt-3 text-[18px] font-semibold tracking-[-0.02em]">
                项目上下文
              </CardTitle>
              <CardDescription className="mt-2 text-slate-600">
                Flow/Chat 会围绕当前项目绑定的 resource 工作（例如 `local_fs` 的 uri 会作为 Chat 的默认 work_dir）。
              </CardDescription>
            </div>
            <Button variant="outline" size="sm" onClick={onRefreshProjects} disabled={projectsLoading}>
              {projectsLoading ? "刷新中..." : "刷新"}
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-4 px-5 pb-5">
          {projectsError ? (
            <p className="rounded-xl border border-rose-200 bg-rose-50 p-3 text-sm text-rose-700">
              {projectsError}
            </p>
          ) : null}

          <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-end">
            <div className="grid gap-1">
              <label
                htmlFor="v2-ops-project-select"
                className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400"
              >
                当前项目
              </label>
              <Select
                id="v2-ops-project-select"
                value={selectedProjectId != null ? String(selectedProjectId) : ""}
                onChange={(e) => {
                  const next = Number.parseInt(e.target.value, 10);
                  onSelectProject(Number.isFinite(next) ? next : null);
                }}
                className="h-11 rounded-xl border-slate-200 bg-slate-50"
                disabled={projectsLoading}
              >
                {projects.length === 0 ? (
                  <option value="">暂无项目</option>
                ) : (
                  projects.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))
                )}
              </Select>
            </div>
            <div className="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 text-xs leading-5 text-slate-500">
              建议每个 repo checkout 对应一个项目。
            </div>
          </div>

          <div className="rounded-2xl border border-slate-200 bg-white p-4">
            <p className="text-sm font-semibold text-slate-900">创建项目</p>
            <div className="mt-3 grid gap-3 md:grid-cols-2">
              <div className="grid gap-1">
                <label
                  htmlFor="v2-project-name"
                  className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400"
                >
                  name
                </label>
                <Input
                  id="v2-project-name"
                  value={newProjectName}
                  onChange={(e) => setNewProjectName(e.target.value)}
                  placeholder="例如：ai-workflow"
                />
              </div>
              <div className="grid gap-1">
                <label
                  htmlFor="v2-project-kind"
                  className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400"
                >
                  kind
                </label>
                <Select
                  id="v2-project-kind"
                  value={newProjectKind}
                  onChange={(e) => setNewProjectKind(e.target.value)}
                  className="h-11 rounded-xl border-slate-200 bg-slate-50"
                >
                  <option value="general">general</option>
                  <option value="dev">dev</option>
                </Select>
              </div>
            </div>
            <div className="mt-3 grid gap-1">
              <label
                htmlFor="v2-project-desc"
                className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400"
              >
                description（可选）
              </label>
              <Input
                id="v2-project-desc"
                value={newProjectDescription}
                onChange={(e) => setNewProjectDescription(e.target.value)}
                placeholder="例如：用于 v2 Flow 编排的项目容器"
              />
            </div>
            <div className="mt-3 flex justify-end">
              <Button onClick={() => void handleCreateProject()} disabled={creatingProject}>
                {creatingProject ? "创建中..." : "创建"}
              </Button>
            </div>
            {projectFeedback ? <p className="mt-2 text-sm text-slate-600">{projectFeedback}</p> : null}
          </div>

          <div className="rounded-2xl border border-slate-200 bg-white p-4">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <p className="text-sm font-semibold text-slate-900">资源绑定（Resources）</p>
                <p className="mt-1 text-xs leading-5 text-slate-500">
                  Project 不再直接存 repo/workspace 信息；需要在这里绑定资源，例如本地目录 `local_fs`。
                </p>
              </div>
              <Button variant="outline" size="sm" onClick={() => void onRefreshResources()} disabled={resourcesLoading}>
                {resourcesLoading ? "刷新中..." : "刷新"}
              </Button>
            </div>

            {resourcesError ? (
              <p className="mt-3 rounded-xl border border-rose-200 bg-rose-50 p-3 text-sm text-rose-700">
                {resourcesError}
              </p>
            ) : null}

            <div className="mt-3 grid gap-3 md:grid-cols-[minmax(0,200px)_minmax(0,1fr)_auto] md:items-end">
              <div className="grid gap-1">
                <label
                  htmlFor="v2-resource-kind"
                  className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400"
                >
                  kind
                </label>
                <Input
                  id="v2-resource-kind"
                  value={newResourceKind}
                  onChange={(e) => setNewResourceKind(e.target.value)}
                  placeholder="例如：local_fs"
                />
              </div>
              <div className="grid gap-1">
                <label
                  htmlFor="v2-resource-uri"
                  className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400"
                >
                  uri
                </label>
                <Input
                  id="v2-resource-uri"
                  value={newResourceURI}
                  onChange={(e) => setNewResourceURI(e.target.value)}
                  placeholder="例如：D:\\project\\ai-workflow"
                />
              </div>
              <Button onClick={() => void handleCreateResource()} disabled={creatingResource || selectedProjectId == null}>
                {creatingResource ? "添加中..." : "添加"}
              </Button>
            </div>
            {resourceFeedback ? <p className="mt-2 text-sm text-slate-600">{resourceFeedback}</p> : null}

            <div className="mt-4 grid gap-2">
              {resources.length === 0 ? (
                <p className="text-sm text-slate-500">暂无资源绑定。</p>
              ) : (
                resources
                  .slice()
                  .sort((a, b) => (b.id ?? 0) - (a.id ?? 0))
                  .map((rb) => (
                    <div key={rb.id} className="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3">
                      <div className="min-w-0">
                        <p className="text-sm font-semibold text-slate-900">
                          #{rb.id} · {rb.kind}
                        </p>
                        <p className="mt-1 break-all text-xs text-slate-600">{rb.uri}</p>
                      </div>
                      <div className="flex items-center gap-2">
                        <Badge variant="outline" className="bg-white text-slate-700">
                          project {rb.project_id}
                        </Badge>
                        <Button variant="outline" size="sm" onClick={() => void handleDeleteResource(rb.id)}>
                          删除
                        </Button>
                      </div>
                    </div>
                  ))
              )}
            </div>
          </div>
        </CardContent>
      </Card>

      <Card className="rounded-2xl border-slate-200 shadow-none">
        <CardHeader className="p-5">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div>
              <Badge variant="secondary" className="bg-indigo-50 text-indigo-600">
                Controls
              </Badge>
              <CardTitle className="mt-3 text-[18px] font-semibold tracking-[-0.02em]">
                系统事件
              </CardTitle>
              <CardDescription className="mt-2 text-slate-600">
                对接 `/api/v2/admin/system-event`，并展示最近的系统事件（从 events 里读取）。
              </CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-4 px-5 pb-5">
          {loading ? <p className="text-sm text-slate-500">加载中...</p> : null}
          <div className="rounded-2xl border border-slate-200 bg-white p-4">
            <p className="text-sm font-semibold text-slate-900">发送系统事件</p>
            <div className="mt-3 grid gap-3 md:grid-cols-2">
              <div className="grid gap-1">
                <label
                  htmlFor="v2-op-event"
                  className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400"
                >
                  event
                </label>
                <Input id="v2-op-event" value={systemEvent} onChange={(e) => setSystemEvent(e.target.value)} />
              </div>
              <div className="grid gap-1">
                <label
                  htmlFor="v2-op-message"
                  className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400"
                >
                  message（可选）
                </label>
                <Textarea
                  id="v2-op-message"
                  value={systemMessage}
                  onChange={(e) => setSystemMessage(e.target.value)}
                  className="min-h-[80px]"
                />
              </div>
            </div>
            <div className="mt-3 flex flex-wrap items-center justify-end gap-2">
              <Button onClick={() => void handleSend()} disabled={sending}>
                {sending ? "发送中..." : "发送"}
              </Button>
            </div>
            {feedback ? <p className="mt-2 text-sm text-slate-600">{feedback}</p> : null}
          </div>

          <div className="rounded-2xl border border-slate-200 bg-white p-4">
            <p className="text-sm font-semibold text-slate-900">最近系统事件</p>
            <div className="mt-3 grid gap-2">
              {events.length === 0 ? (
                <p className="text-sm text-slate-500">暂无事件。</p>
              ) : (
                events
                  .slice()
                  .sort((a, b) => (b.id ?? 0) - (a.id ?? 0))
                  .slice(0, 30)
                  .map((ev) => (
                    <div key={ev.id} className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3">
                      <div className="flex flex-wrap items-start justify-between gap-3">
                        <div>
                          <p className="text-sm font-semibold text-slate-900">
                            #{ev.id} · {ev.type}
                          </p>
                          <p className="mt-1 text-[11px] text-slate-500">{formatTime(ev.timestamp)}</p>
                        </div>
                        <Badge variant="outline" className="bg-white text-slate-700">
                          {String(ev.data?.event ?? "-")}
                        </Badge>
                      </div>
                      {ev.data ? (
                        <pre className="mt-2 overflow-auto rounded-xl bg-slate-950 px-3 py-2 text-xs text-slate-100">
                          {JSON.stringify(ev.data, null, 2)}
                        </pre>
                      ) : null}
                    </div>
                  ))
              )}
            </div>
          </div>
        </CardContent>
      </Card>
    </PageScaffold>
  );
};

export default OpsView;
