import { useEffect, useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";
import { Textarea } from "@/components/ui/textarea";
import type { ApiClient } from "@/lib/apiClient";
import type { AdminAuditLogItem, ApiWorkflowProfile } from "@/types/api";

interface OpsViewProps {
  apiClient: ApiClient;
  projectId: string;
  refreshToken: number;
}

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

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error && error.message.trim().length > 0) {
    return error.message;
  }
  return "请求失败，请稍后重试";
};

const OpsView = ({ apiClient, projectId, refreshToken }: OpsViewProps) => {
  const [profiles, setProfiles] = useState<ApiWorkflowProfile[]>([]);
  const [auditItems, setAuditItems] = useState<AdminAuditLogItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [issueId, setIssueId] = useState("");
  const [reason, setReason] = useState("");
  const [systemEvent, setSystemEvent] = useState("ops_notice");
  const [systemMessage, setSystemMessage] = useState("");
  const [opFeedback, setOpFeedback] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState<"force-ready" | "force-unblock" | "system-event" | null>(null);

  const loadData = async () => {
    setLoading(true);
    setError(null);
    try {
      const [profileResponse, auditResponse] = await Promise.all([
        apiClient.listWorkflowProfiles(),
        apiClient.listAdminAuditLog?.({
          projectId,
          limit: 20,
          offset: 0,
        }),
      ]);
      setProfiles(Array.isArray(profileResponse.items) ? profileResponse.items : []);
      setAuditItems(Array.isArray(auditResponse?.items) ? auditResponse.items : []);
    } catch (requestError) {
      setError(getErrorMessage(requestError));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadData();
  }, [apiClient, projectId, refreshToken]);

  const auditStats = useMemo(() => {
    const dangerous = auditItems.filter((item) =>
      ["force_ready", "force_unblock", "replay_delivery", "restart"].includes(item.action),
    ).length;
    const today = auditItems.filter((item) => {
      const createdAt = new Date(item.created_at);
      const now = new Date();
      return (
        !Number.isNaN(createdAt.getTime()) &&
        createdAt.getFullYear() === now.getFullYear() &&
        createdAt.getMonth() === now.getMonth() &&
        createdAt.getDate() === now.getDate()
      );
    }).length;
    return {
      total: auditItems.length,
      dangerous,
      today,
    };
  }, [auditItems]);

  const runIssueOperation = async (action: "force-ready" | "force-unblock") => {
    const trimmedIssueId = issueId.trim();
    if (!trimmedIssueId) {
      setOpFeedback("请先输入 issue_id。");
      return;
    }
    setSubmitting(action);
    setOpFeedback(null);
    try {
      if (action === "force-ready") {
        await apiClient.forceIssueReady?.({
          issue_id: trimmedIssueId,
          reason: reason.trim() || undefined,
        });
      } else {
        await apiClient.forceIssueUnblock?.({
          issue_id: trimmedIssueId,
          reason: reason.trim() || undefined,
        });
      }
      setOpFeedback(`${trimmedIssueId} 已执行 ${action}。`);
      await loadData();
    } catch (requestError) {
      setOpFeedback(getErrorMessage(requestError));
    } finally {
      setSubmitting(null);
    }
  };

  const submitSystemEvent = async () => {
    const event = systemEvent.trim();
    if (!event) {
      setOpFeedback("事件名不能为空。");
      return;
    }
    setSubmitting("system-event");
    setOpFeedback(null);
    try {
      await apiClient.sendSystemEvent?.({
        event,
        data: systemMessage.trim() ? { message: systemMessage.trim() } : undefined,
      });
      setOpFeedback(`系统事件 ${event} 已发送。`);
      setSystemMessage("");
    } catch (requestError) {
      setOpFeedback(getErrorMessage(requestError));
    } finally {
      setSubmitting(null);
    }
  };

  return (
    <section className="flex flex-col gap-4">
      {error ? (
        <Card className="border-rose-200 bg-rose-50">
          <CardContent className="pt-6 text-sm text-rose-700">{error}</CardContent>
        </Card>
      ) : null}

      <div className="grid gap-4 xl:grid-cols-[1.1fr_0.9fr]">
        <Card className="rounded-2xl shadow-none">
          <CardHeader>
            <CardTitle>系统健康</CardTitle>
            <CardDescription>工作流配置和后台审计会优先展示在这里。</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4 md:grid-cols-3">
            <div className="rounded-[20px] border border-slate-200 bg-slate-50 p-5">
              <p className="text-xs font-semibold uppercase tracking-[0.16em] text-slate-400">审计总数</p>
              <p className="mt-3 text-3xl font-bold tracking-[-0.03em] text-slate-950">{auditStats.total}</p>
              <p className="mt-2 text-sm text-slate-500">当前项目已记录的所有审计动作</p>
            </div>
            <div className="rounded-[20px] border border-amber-200 bg-amber-50 p-5">
              <p className="text-xs font-semibold uppercase tracking-[0.16em] text-amber-500">危险操作</p>
              <p className="mt-3 text-3xl font-bold tracking-[-0.03em] text-amber-700">{auditStats.dangerous}</p>
              <p className="mt-2 text-sm text-amber-700/80">force ready / unblock / replay / restart</p>
            </div>
            <div className="rounded-[20px] border border-blue-200 bg-blue-50 p-5">
              <p className="text-xs font-semibold uppercase tracking-[0.16em] text-blue-500">今日动作</p>
              <p className="mt-3 text-3xl font-bold tracking-[-0.03em] text-blue-700">{auditStats.today}</p>
              <p className="mt-2 text-sm text-blue-700/80">便于快速判断是否有人工干预集中发生</p>
            </div>
          </CardContent>
        </Card>

        <Card className="rounded-2xl shadow-none">
          <CardHeader>
            <CardTitle>Workflow Profiles</CardTitle>
            <CardDescription>直接使用后端的 `/api/v1/workflow-profiles`。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {loading ? (
              <p className="text-sm text-slate-500">加载中...</p>
            ) : profiles.length === 0 ? (
              <p className="text-sm text-slate-500">暂无 profile。</p>
            ) : (
              profiles.map((profile) => (
                <div key={profile.type} className="rounded-2xl border border-slate-200 p-4">
                  <div className="flex items-center justify-between gap-3">
                    <p className="text-sm font-semibold text-slate-950">{profile.type}</p>
                    <Badge variant="secondary">{profile.sla_minutes} min SLA</Badge>
                  </div>
                  <p className="mt-2 text-sm text-slate-600">{profile.description}</p>
                  <p className="mt-2 text-xs text-slate-400">reviewer_count = {profile.reviewer_count}</p>
                </div>
              ))
            )}
          </CardContent>
        </Card>
      </div>

      <div className="grid gap-4 xl:grid-cols-[0.95fr_1.05fr]">
        <Card className="rounded-2xl shadow-none">
          <CardHeader>
            <CardTitle>危险操作</CardTitle>
            <CardDescription>仅在本页暴露 force ready / unblock 等操作。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <label htmlFor="ops-issue-id" className="text-xs font-semibold uppercase tracking-[0.14em] text-slate-400">
                Issue ID
              </label>
              <Input
                id="ops-issue-id"
                value={issueId}
                onChange={(event) => setIssueId(event.target.value)}
                placeholder="例如：issue-123 或 ISS-219"
              />
            </div>
            <div className="space-y-2">
              <label htmlFor="ops-reason" className="text-xs font-semibold uppercase tracking-[0.14em] text-slate-400">
                Reason
              </label>
              <Textarea
                id="ops-reason"
                value={reason}
                onChange={(event) => setReason(event.target.value)}
                className="min-h-[104px]"
                placeholder="记录人工干预原因，便于后续追溯。"
              />
            </div>
            <div className="flex flex-wrap gap-3">
              <Button
                variant="destructive"
                onClick={() => {
                  void runIssueOperation("force-ready");
                }}
                disabled={submitting !== null}
              >
                {submitting === "force-ready" ? "执行中..." : "Force Ready"}
              </Button>
              <Button
                variant="outline"
                onClick={() => {
                  void runIssueOperation("force-unblock");
                }}
                disabled={submitting !== null}
              >
                {submitting === "force-unblock" ? "执行中..." : "Force Unblock"}
              </Button>
            </div>
            <p className="text-xs leading-6 text-slate-500">
              这些操作直接调用后端 `/api/v1/admin/ops/*`，会写入审计日志，不建议在普通业务页暴露。
            </p>
          </CardContent>
        </Card>

        <Card className="rounded-2xl shadow-none">
          <CardHeader>
            <CardTitle>发送系统事件</CardTitle>
            <CardDescription>用于广播统一 banner 或系统级通知。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <label htmlFor="ops-event-name" className="text-xs font-semibold uppercase tracking-[0.14em] text-slate-400">
                Event
              </label>
              <Input
                id="ops-event-name"
                value={systemEvent}
                onChange={(event) => setSystemEvent(event.target.value)}
                placeholder="例如：ops_notice / maintenance / restart_countdown"
              />
            </div>
            <div className="space-y-2">
              <label htmlFor="ops-event-message" className="text-xs font-semibold uppercase tracking-[0.14em] text-slate-400">
                Message
              </label>
              <Textarea
                id="ops-event-message"
                value={systemMessage}
                onChange={(event) => setSystemMessage(event.target.value)}
                placeholder="将作为 data.message 发送给前端。"
              />
            </div>
            <Button
              variant="secondary"
              onClick={() => {
                void submitSystemEvent();
              }}
              disabled={submitting !== null}
            >
              {submitting === "system-event" ? "发送中..." : "发送系统事件"}
            </Button>
            {opFeedback ? (
              <>
                <Separator />
                <p className="text-sm text-slate-600">{opFeedback}</p>
              </>
            ) : null}
          </CardContent>
        </Card>
      </div>

      <Card className="rounded-2xl shadow-none">
        <CardHeader className="flex-row items-start justify-between gap-4">
          <div>
            <CardTitle>审计记录</CardTitle>
            <CardDescription>按时间倒序展示最近的项目级后台动作。</CardDescription>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              void loadData();
            }}
            disabled={loading}
          >
            {loading ? "刷新中..." : "刷新"}
          </Button>
        </CardHeader>
        <CardContent className="space-y-3">
          {auditItems.length === 0 ? (
            <p className="text-sm text-slate-500">暂无审计记录。</p>
          ) : (
            auditItems.map((item) => (
              <div
                key={`${item.id}-${item.created_at}`}
                className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3"
              >
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div className="flex items-center gap-2">
                    <p className="text-sm font-semibold text-slate-950">{item.action}</p>
                    <Badge
                      variant={
                        ["force_ready", "force_unblock", "restart"].includes(item.action)
                          ? "warning"
                          : "outline"
                      }
                    >
                      {item.user_id || "admin"}
                    </Badge>
                  </div>
                  <p className="text-xs text-slate-400">{formatTime(item.created_at)}</p>
                </div>
                <p className="mt-2 text-sm text-slate-600">{item.message}</p>
                <p className="mt-2 text-xs text-slate-400">
                  run={item.run_id || "-"} · issue={item.issue_id || "-"} · source={item.source || "-"}
                </p>
              </div>
            ))
          )}
        </CardContent>
      </Card>
    </section>
  );
};

export default OpsView;
