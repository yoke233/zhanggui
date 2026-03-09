import { useEffect, useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import type { ApiClient } from "@/lib/apiClient";
import type { WsClient } from "@/lib/wsClient";
import type { AdminAuditLogItem, ApiWorkflowProfile, CreateProjectCreateRequest, ProjectSourceType } from "@/types/api";

interface V3OpsViewProps {
  apiClient: ApiClient;
  wsClient: WsClient;
  wsStatus: ReturnType<WsClient["getStatus"]>;
  projectId: string | null;
  refreshToken: number;
  onProjectCreated: (projectId?: string) => Promise<void>;
}

const POLL_INTERVAL_MS = 1500;

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error && error.message.trim().length > 0) {
    return error.message;
  }
  return "请求失败，请稍后重试";
};

const formatTime = (value?: string): string => {
  if (!value) {
    return "-";
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toLocaleString("zh-CN", { hour12: false });
};

const V3OpsView = ({
  apiClient,
  wsStatus,
  projectId,
  refreshToken,
  onProjectCreated,
}: V3OpsViewProps) => {
  const [profiles, setProfiles] = useState<ApiWorkflowProfile[]>([]);
  const [auditItems, setAuditItems] = useState<AdminAuditLogItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [sourceType, setSourceType] = useState<ProjectSourceType>("local_path");
  const [projectName, setProjectName] = useState("");
  const [repoPath, setRepoPath] = useState("");
  const [remoteURL, setRemoteURL] = useState("");
  const [gitRef, setGitRef] = useState("");
  const [createFeedback, setCreateFeedback] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);

  const [issueId, setIssueId] = useState("");
  const [reason, setReason] = useState("");
  const [systemEvent, setSystemEvent] = useState("ops_notice");
  const [systemMessage, setSystemMessage] = useState("");
  const [actionFeedback, setActionFeedback] = useState<string | null>(null);
  const [actionLoading, setActionLoading] = useState<"force-ready" | "force-unblock" | "system-event" | null>(null);

  useEffect(() => {
    let cancelled = false;
    const loadData = async () => {
      setLoading(true);
      setError(null);
      try {
        const [profileResponse, auditResponse] = await Promise.all([
          apiClient.listWorkflowProfiles(),
          apiClient.listAdminAuditLog?.({
            projectId: projectId ?? undefined,
            limit: 12,
            offset: 0,
          }),
        ]);
        if (cancelled) {
          return;
        }
        setProfiles(Array.isArray(profileResponse.items) ? profileResponse.items : []);
        setAuditItems(Array.isArray(auditResponse?.items) ? auditResponse.items : []);
      } catch (requestError) {
        if (cancelled) {
          return;
        }
        setError(getErrorMessage(requestError));
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };

    void loadData();
    return () => {
      cancelled = true;
    };
  }, [apiClient, projectId, refreshToken]);

  const auditStats = useMemo(() => {
    return auditItems.reduce(
      (summary, item) => {
        summary.total += 1;
        if (["force_ready", "force_unblock", "restart", "replay_delivery"].includes(item.action)) {
          summary.dangerous += 1;
        }
        if (item.created_at.slice(0, 10) === new Date().toISOString().slice(0, 10)) {
          summary.today += 1;
        }
        return summary;
      },
      { total: 0, dangerous: 0, today: 0 },
    );
  }, [auditItems]);

  const handleCreateProject = async () => {
    const trimmedName = projectName.trim();
    if (!trimmedName) {
      setCreateFeedback("项目名称不能为空。");
      return;
    }

    const payload: CreateProjectCreateRequest = {
      name: trimmedName,
      source_type: sourceType,
    };

    if (sourceType === "local_path" && repoPath.trim()) {
      payload.repo_path = repoPath.trim();
    }
    if (sourceType === "local_new" && repoPath.trim()) {
      payload.repo_path = repoPath.trim();
    }
    if (sourceType === "github_clone") {
      payload.remote_url = remoteURL.trim();
      if (gitRef.trim()) {
        payload.ref = gitRef.trim();
      }
    }

    setCreating(true);
    setCreateFeedback(null);
    try {
      const response = await apiClient.createProjectCreateRequest(payload);
      setCreateFeedback(`创建请求已提交：${response.request_id}`);
      let nextStatus = "pending";
      let createdProjectId: string | undefined;

      while (nextStatus === "pending" || nextStatus === "running") {
        await new Promise((resolve) => setTimeout(resolve, POLL_INTERVAL_MS));
        const status = await apiClient.getProjectCreateRequest(response.request_id);
        nextStatus = status.status;
        createdProjectId = status.project_id;
        setCreateFeedback(status.message || `创建状态：${status.status}`);
        if (status.status === "failed") {
          throw new Error(status.error || status.message || "项目创建失败");
        }
      }

      setProjectName("");
      setRepoPath("");
      setRemoteURL("");
      setGitRef("");
      await onProjectCreated(createdProjectId);
    } catch (requestError) {
      setCreateFeedback(getErrorMessage(requestError));
    } finally {
      setCreating(false);
    }
  };

  const runIssueAction = async (action: "force-ready" | "force-unblock") => {
    if (!issueId.trim()) {
      setActionFeedback("请先输入 issue_id。");
      return;
    }
    setActionLoading(action);
    setActionFeedback(null);
    try {
      if (action === "force-ready") {
        await apiClient.forceIssueReady?.({
          issue_id: issueId.trim(),
          reason: reason.trim() || undefined,
        });
      } else {
        await apiClient.forceIssueUnblock?.({
          issue_id: issueId.trim(),
          reason: reason.trim() || undefined,
        });
      }
      setActionFeedback(`${issueId.trim()} 已执行 ${action}。`);
    } catch (requestError) {
      setActionFeedback(getErrorMessage(requestError));
    } finally {
      setActionLoading(null);
    }
  };

  const runSystemEvent = async () => {
    if (!systemEvent.trim()) {
      setActionFeedback("系统事件名不能为空。");
      return;
    }
    setActionLoading("system-event");
    setActionFeedback(null);
    try {
      await apiClient.sendSystemEvent?.({
        event: systemEvent.trim(),
        data: systemMessage.trim() ? { message: systemMessage.trim() } : undefined,
      });
      setSystemMessage("");
      setActionFeedback(`系统事件 ${systemEvent.trim()} 已发送。`);
    } catch (requestError) {
      setActionFeedback(getErrorMessage(requestError));
    } finally {
      setActionLoading(null);
    }
  };

  return (
    <section className="flex flex-col gap-4">
      <Card className="rounded-2xl border-slate-200 shadow-none">
        <CardHeader className="p-5">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div>
              <div className="flex items-center gap-2">
                <Badge variant="secondary" className="bg-emerald-50 text-emerald-600">
                  Integrations & Admin Ops
                </Badge>
                <Badge variant="outline" className={wsStatus === "open" ? "bg-emerald-50 text-emerald-700" : "bg-amber-50 text-amber-700"}>
                  WS {wsStatus}
                </Badge>
              </div>
              <CardTitle className="mt-3 text-[24px] font-semibold tracking-[-0.02em]">
                协议 / 审计 / 运维控制台
              </CardTitle>
              <CardDescription className="mt-1">
                默认先看健康、统计和审计，再进入系统级操作；危险动作集中保护，避免误触。
              </CardDescription>
            </div>
            <div className="grid gap-2 rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3">
              <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">当前上下文</p>
              <p className="text-sm font-semibold text-slate-950">{projectId ?? "系统级入口"}</p>
              <p className="text-xs text-slate-500">审计 {auditStats.total} 条 · 危险动作 {auditStats.dangerous} 次</p>
            </div>
          </div>
        </CardHeader>
        <CardContent className="grid gap-3 px-5 pb-5 md:grid-cols-3">
          <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
            <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">审计总数</p>
            <p className="mt-2 text-2xl font-semibold text-slate-950">{auditStats.total}</p>
          </div>
          <div className="rounded-2xl border border-slate-200 bg-amber-50 p-4">
            <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-amber-700">危险操作</p>
            <p className="mt-2 text-2xl font-semibold text-slate-950">{auditStats.dangerous}</p>
          </div>
          <div className="rounded-2xl border border-slate-200 bg-indigo-50 p-4">
            <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-indigo-700">今日动作</p>
            <p className="mt-2 text-2xl font-semibold text-slate-950">{auditStats.today}</p>
          </div>
        </CardContent>
      </Card>

      {error ? (
        <p className="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p>
      ) : null}

      <div className="grid gap-4 xl:grid-cols-[0.78fr_1fr_0.82fr]">
        <Card className="rounded-2xl border-slate-200 shadow-none">
          <CardHeader>
            <CardTitle className="text-base">项目管理</CardTitle>
            <CardDescription>旧项目管理面板已归档，这里是新的创建入口。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <label htmlFor="v3-project-name" className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
                项目名称
              </label>
              <Input
                id="v3-project-name"
                value={projectName}
                onChange={(event) => setProjectName(event.target.value)}
                placeholder="例如：proj-v3-dashboard"
              />
            </div>
            <div className="space-y-2">
              <label htmlFor="v3-project-source" className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
                项目来源
              </label>
              <Select id="v3-project-source" value={sourceType} onChange={(event) => setSourceType(event.target.value as ProjectSourceType)}>
                <option value="local_path">本地已有仓库</option>
                <option value="local_new">新建本地仓库</option>
                <option value="github_clone">从 GitHub 克隆</option>
              </Select>
            </div>
            {(sourceType === "local_path" || sourceType === "local_new") ? (
              <div className="space-y-2">
                <label htmlFor="v3-project-repo-path" className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
                  仓库路径
                </label>
                <Input
                  id="v3-project-repo-path"
                  value={repoPath}
                  onChange={(event) => setRepoPath(event.target.value)}
                  placeholder="D:/repo/project-v3"
                />
              </div>
            ) : (
              <>
                <div className="space-y-2">
                  <label htmlFor="v3-project-remote-url" className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
                    Remote URL
                  </label>
                  <Input
                    id="v3-project-remote-url"
                    value={remoteURL}
                    onChange={(event) => setRemoteURL(event.target.value)}
                    placeholder="https://github.com/org/repo.git"
                  />
                </div>
                <div className="space-y-2">
                  <label htmlFor="v3-project-ref" className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
                    Ref
                  </label>
                  <Input
                    id="v3-project-ref"
                    value={gitRef}
                    onChange={(event) => setGitRef(event.target.value)}
                    placeholder="main / release-v3"
                  />
                </div>
              </>
            )}
            <Button variant="secondary" onClick={() => void handleCreateProject()} disabled={creating}>
              {creating ? "创建中..." : "创建项目"}
            </Button>
            {createFeedback ? (
              <p className="rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 text-sm text-slate-600">{createFeedback}</p>
            ) : null}
          </CardContent>
        </Card>

        <div className="flex flex-col gap-4">
          <Card className="rounded-2xl border-slate-200 shadow-none">
            <CardHeader>
              <CardTitle className="text-base">Workflow Profiles</CardTitle>
              <CardDescription>用 profile 看系统健康和默认流程，不把诊断混进业务页。</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              {loading ? (
                <p className="text-sm text-slate-500">加载中...</p>
              ) : profiles.length === 0 ? (
                <p className="text-sm text-slate-500">暂无 profile。</p>
              ) : (
                profiles.map((profile) => (
                  <div key={profile.type} className="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3">
                    <div className="flex items-start justify-between gap-3">
                      <div>
                        <p className="text-sm font-semibold text-slate-950">{profile.type}</p>
                        <p className="mt-1 text-xs leading-5 text-slate-500">{profile.description}</p>
                      </div>
                      <Badge variant="outline">{profile.sla_minutes} min SLA</Badge>
                    </div>
                    <p className="mt-2 text-[11px] text-slate-400">reviewer_count = {profile.reviewer_count}</p>
                  </div>
                ))
              )}
            </CardContent>
          </Card>

          <Card className="rounded-2xl border-slate-200 shadow-none">
            <CardHeader>
              <CardTitle className="text-base">审计记录</CardTitle>
              <CardDescription>默认先看最近的后台动作，再决定是否进入危险操作。</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              {auditItems.length === 0 ? (
                <p className="text-sm text-slate-500">暂无审计记录。</p>
              ) : (
                auditItems.map((item) => (
                  <div key={`${item.id}-${item.created_at}`} className="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3">
                    <div className="flex items-start justify-between gap-3">
                      <div>
                        <p className="text-sm font-semibold text-slate-950">{item.action}</p>
                        <p className="mt-1 text-xs leading-5 text-slate-500">{item.message || "暂无描述。"}</p>
                      </div>
                      <Badge variant="outline">{item.user_id || "admin"}</Badge>
                    </div>
                    <p className="mt-2 text-[11px] text-slate-400">
                      {formatTime(item.created_at)} · issue {item.issue_id || "-"} · run {item.run_id || "-"}
                    </p>
                  </div>
                ))
              )}
            </CardContent>
          </Card>
        </div>

        <div className="flex flex-col gap-4">
          <Card className="rounded-2xl border-slate-200 shadow-none">
            <CardHeader>
              <CardTitle className="text-base">危险操作</CardTitle>
              <CardDescription>高权限动作只集中在这一区，不再散落到业务页。</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <label className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">Issue ID</label>
                <Input value={issueId} onChange={(event) => setIssueId(event.target.value)} placeholder="issue-123 / ISS-219" />
              </div>
              <div className="space-y-2">
                <label className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">Reason</label>
                <Textarea value={reason} onChange={(event) => setReason(event.target.value)} className="min-h-[96px]" placeholder="记录人工干预原因，便于审计回溯。" />
              </div>
              <div className="flex flex-wrap gap-2">
                <Button variant="destructive" size="sm" onClick={() => void runIssueAction("force-ready")} disabled={actionLoading !== null}>
                  {actionLoading === "force-ready" ? "执行中..." : "Force Ready"}
                </Button>
                <Button variant="outline" size="sm" onClick={() => void runIssueAction("force-unblock")} disabled={actionLoading !== null}>
                  {actionLoading === "force-unblock" ? "执行中..." : "Force Unblock"}
                </Button>
              </div>
            </CardContent>
          </Card>

          <Card className="rounded-2xl border-slate-200 shadow-none">
            <CardHeader>
              <CardTitle className="text-base">发送系统事件</CardTitle>
              <CardDescription>广播 banner 或系统级通知，不把此能力埋在别处。</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <label className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">Event</label>
                <Input value={systemEvent} onChange={(event) => setSystemEvent(event.target.value)} />
              </div>
              <div className="space-y-2">
                <label className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">Message</label>
                <Textarea value={systemMessage} onChange={(event) => setSystemMessage(event.target.value)} className="min-h-[96px]" placeholder="将作为 data.message 发送给前端。" />
              </div>
              <Button variant="secondary" size="sm" onClick={() => void runSystemEvent()} disabled={actionLoading !== null}>
                {actionLoading === "system-event" ? "发送中..." : "发送系统事件"}
              </Button>
              {actionFeedback ? (
                <p className="rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 text-sm text-slate-600">{actionFeedback}</p>
              ) : null}
            </CardContent>
          </Card>
        </div>
      </div>
    </section>
  );
};

export default V3OpsView;
