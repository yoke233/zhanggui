import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
  ArrowUpRight,
  CheckCircle2,
  ChevronRight,
  Clock3,
  Loader2,
  MessageCircle,
  XCircle,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { cn } from "@/lib/utils";
import { formatRelativeTime, getErrorMessage } from "@/lib/v2Workbench";
import type { InitiativeDetail, Thread, WorkItem } from "@/types/apiV2";

const initiativeStatusStyle: Record<
  string,
  { label: string; text: string; bg: string }
> = {
  draft: { label: "草案", text: "text-slate-700", bg: "bg-slate-100" },
  proposed: { label: "待审批", text: "text-blue-700", bg: "bg-blue-50" },
  approved: { label: "已批准", text: "text-emerald-700", bg: "bg-emerald-50" },
  executing: { label: "执行中", text: "text-blue-700", bg: "bg-blue-50" },
  blocked: { label: "阻塞", text: "text-amber-700", bg: "bg-amber-50" },
  done: { label: "已完成", text: "text-emerald-700", bg: "bg-emerald-50" },
  failed: { label: "失败", text: "text-rose-700", bg: "bg-rose-50" },
  cancelled: { label: "已取消", text: "text-zinc-600", bg: "bg-zinc-100" },
};

const workItemStatusStyle: Record<
  string,
  { label: string; text: string; bg: string }
> = {
  open: { label: "待评估", text: "text-blue-600", bg: "bg-blue-50" },
  accepted: { label: "已确认", text: "text-amber-600", bg: "bg-amber-50" },
  queued: { label: "排队中", text: "text-zinc-600", bg: "bg-zinc-100" },
  running: { label: "运行中", text: "text-blue-600", bg: "bg-blue-50" },
  blocked: { label: "阻塞", text: "text-amber-600", bg: "bg-amber-50" },
  failed: { label: "失败", text: "text-red-600", bg: "bg-red-50" },
  done: { label: "已完成", text: "text-emerald-600", bg: "bg-emerald-50" },
  cancelled: { label: "已取消", text: "text-zinc-500", bg: "bg-zinc-100" },
  closed: { label: "已关闭", text: "text-zinc-500", bg: "bg-zinc-100" },
};

type ReviewFormState = {
  approvedBy: string;
  reviewNote: string;
};

function ProgressStat({
  label,
  value,
  tone,
}: {
  label: string;
  value: number;
  tone: "neutral" | "info" | "success" | "warning" | "danger";
}) {
  const style =
    tone === "success"
      ? "border-emerald-200 bg-emerald-50 text-emerald-700"
      : tone === "warning"
        ? "border-amber-200 bg-amber-50 text-amber-700"
        : tone === "danger"
          ? "border-rose-200 bg-rose-50 text-rose-700"
          : tone === "info"
            ? "border-blue-200 bg-blue-50 text-blue-700"
            : "border-border bg-muted/30 text-foreground";
  return (
    <div className={cn("rounded-lg border px-3 py-2", style)}>
      <div className="text-[10px] uppercase tracking-wider">{label}</div>
      <div className="mt-1 text-lg font-semibold">{value}</div>
    </div>
  );
}

export function InitiativeDetailPage() {
  const { initiativeId: initiativeIdParam } = useParams();
  const { apiClient } = useWorkbench();
  const initiativeId = Number.parseInt(initiativeIdParam ?? "", 10);

  const [detail, setDetail] = useState<InitiativeDetail | null>(null);
  const [linkedThreads, setLinkedThreads] = useState<Record<number, Thread>>({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [actionLoading, setActionLoading] = useState<
    "propose" | "approve" | "reject" | "cancel" | null
  >(null);
  const [reviewForm, setReviewForm] = useState<ReviewFormState>({
    approvedBy: "human",
    reviewNote: "",
  });

  useEffect(() => {
    setReviewForm({
      approvedBy: "human",
      reviewNote: "",
    });
  }, [initiativeId]);

  const loadDetail = useCallback(async () => {
    if (!Number.isFinite(initiativeId)) {
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const nextDetail = await apiClient.getInitiative(initiativeId);
      setDetail(nextDetail);
      setReviewForm({
        approvedBy:
          nextDetail.initiative.approved_by ||
          nextDetail.initiative.created_by ||
          "human",
        reviewNote: nextDetail.initiative.review_note || "",
      });
      if (nextDetail.threads.length > 0) {
        const threads = await Promise.allSettled(
          nextDetail.threads.map((link) => apiClient.getThread(link.thread_id)),
        );
        const threadMap: Record<number, Thread> = {};
        threads.forEach((result, index) => {
          if (result.status === "fulfilled") {
            threadMap[nextDetail.threads[index].thread_id] = result.value;
          }
        });
        setLinkedThreads(threadMap);
      } else {
        setLinkedThreads({});
      }
    } catch (loadError) {
      setError(getErrorMessage(loadError));
    } finally {
      setLoading(false);
    }
  }, [apiClient, initiativeId]);

  useEffect(() => {
    void loadDetail();
  }, [loadDetail]);

  const initiative = detail?.initiative ?? null;
  const statusStyle = initiative
    ? initiativeStatusStyle[initiative.status] ?? initiativeStatusStyle.draft
    : initiativeStatusStyle.draft;

  const canPropose = initiative?.status === "draft";
  const canApprove = initiative?.status === "proposed";
  const canReject = initiative?.status === "proposed";
  const canCancel =
    initiative != null &&
    !["done", "failed", "cancelled"].includes(initiative.status);

  const sortedWorkItems = useMemo(() => {
    return [...(detail?.work_items ?? [])].sort((left, right) => left.id - right.id);
  }, [detail?.work_items]);

  const runAction = async (
    action: "propose" | "approve" | "reject" | "cancel",
  ) => {
    if (!initiative) return;
    setActionLoading(action);
    setError(null);
    try {
      if (action === "propose") {
        await apiClient.proposeInitiative(initiative.id);
      } else if (action === "approve") {
        await apiClient.approveInitiative(initiative.id, {
          approved_by: reviewForm.approvedBy.trim() || initiative.created_by,
        });
      } else if (action === "reject") {
        await apiClient.rejectInitiative(initiative.id, {
          review_note: reviewForm.reviewNote.trim(),
        });
      } else {
        await apiClient.cancelInitiative(initiative.id);
      }
      await loadDetail();
    } catch (actionError) {
      setError(getErrorMessage(actionError));
    } finally {
      setActionLoading(null);
    }
  };

  if (!Number.isFinite(initiativeId)) {
    return (
      <div className="flex h-full items-center justify-center px-6">
        <div className="rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
          Initiative ID 无效。
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <div className="shrink-0 border-b px-8 pb-5 pt-6">
        <div className="mb-2 flex items-center gap-1.5 text-[13px]">
          <Link to="/threads" className="text-blue-600 hover:underline">
            Threads
          </Link>
          <ChevronRight className="h-3 w-3 text-muted-foreground" />
          <span className="font-medium text-foreground">
            Initiative #{initiativeId}
          </span>
        </div>
        <div className="flex items-center justify-between gap-4">
          <div className="min-w-0">
            <div className="flex items-center gap-3">
              <h1 className="truncate text-xl font-bold tracking-tight">
                {initiative?.title ?? `Initiative #${initiativeId}`}
              </h1>
              <span
                className={cn(
                  "inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium",
                  statusStyle.text,
                  statusStyle.bg,
                )}
              >
                {statusStyle.label}
              </span>
              {loading ? (
                <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
              ) : null}
            </div>
            {initiative?.description ? (
              <p className="mt-2 max-w-3xl text-sm text-muted-foreground">
                {initiative.description}
              </p>
            ) : null}
          </div>
          <div className="flex shrink-0 items-center gap-2">
            {canPropose ? (
              <Button
                size="sm"
                onClick={() => void runAction("propose")}
                disabled={actionLoading != null}
              >
                {actionLoading === "propose" ? (
                  <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" />
                ) : (
                  <ArrowUpRight className="mr-1 h-3.5 w-3.5" />
                )}
                Propose
              </Button>
            ) : null}
            {canApprove ? (
              <Button
                size="sm"
                onClick={() => void runAction("approve")}
                disabled={actionLoading != null}
              >
                {actionLoading === "approve" ? (
                  <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" />
                ) : (
                  <CheckCircle2 className="mr-1 h-3.5 w-3.5" />
                )}
                Approve
              </Button>
            ) : null}
            {canReject ? (
              <Button
                variant="outline"
                size="sm"
                onClick={() => void runAction("reject")}
                disabled={actionLoading != null}
              >
                {actionLoading === "reject" ? (
                  <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" />
                ) : (
                  <XCircle className="mr-1 h-3.5 w-3.5" />
                )}
                Reject
              </Button>
            ) : null}
            {canCancel ? (
              <Button
                variant="outline"
                size="sm"
                onClick={() => void runAction("cancel")}
                disabled={actionLoading != null}
              >
                Cancel
              </Button>
            ) : null}
          </div>
        </div>
      </div>

      {error ? (
        <p className="mx-8 mt-4 rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
          {error}
        </p>
      ) : null}

      <div className="flex flex-1 overflow-hidden">
        <div className="flex-1 overflow-y-auto px-8 py-6">
          {detail ? (
            <div className="space-y-6">
              <section>
                <div className="mb-3 flex items-center gap-2">
                  <h3 className="text-sm font-semibold">进度</h3>
                  <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] text-muted-foreground">
                    {detail.progress.total} work items
                  </span>
                </div>
                <div className="grid gap-3 md:grid-cols-3 xl:grid-cols-6">
                  <ProgressStat label="Pending" value={detail.progress.pending} tone="neutral" />
                  <ProgressStat label="Running" value={detail.progress.running} tone="info" />
                  <ProgressStat label="Blocked" value={detail.progress.blocked} tone="warning" />
                  <ProgressStat label="Done" value={detail.progress.done} tone="success" />
                  <ProgressStat label="Failed" value={detail.progress.failed} tone="danger" />
                  <ProgressStat label="Cancelled" value={detail.progress.cancelled} tone="neutral" />
                </div>
              </section>

              <section>
                <div className="mb-3 flex items-center justify-between">
                  <h3 className="text-sm font-semibold">Work Items</h3>
                  <span className="text-xs text-muted-foreground">
                    {sortedWorkItems.length} items
                  </span>
                </div>
                {sortedWorkItems.length > 0 ? (
                  <div className="space-y-2">
                    {sortedWorkItems.map((workItem: WorkItem) => {
                      const style =
                        workItemStatusStyle[workItem.status] ??
                        workItemStatusStyle.open;
                      const initiativeItem = detail.items.find(
                        (item) => item.work_item_id === workItem.id,
                      );
                      return (
                        <Link
                          key={workItem.id}
                          to={`/work-items/${workItem.id}`}
                          className="flex items-start gap-3 rounded-lg border px-3 py-3 transition-colors hover:bg-muted/40"
                        >
                          <div
                            className={cn(
                              "mt-0.5 rounded-full p-1.5",
                              style.bg,
                              style.text,
                            )}
                          >
                            <Clock3 className="h-3.5 w-3.5" />
                          </div>
                          <div className="min-w-0 flex-1">
                            <div className="flex items-center gap-2">
                              <span className="truncate text-sm font-medium">
                                {workItem.title}
                              </span>
                              <span
                                className={cn(
                                  "rounded-full px-2 py-0.5 text-[10px] font-medium",
                                  style.text,
                                  style.bg,
                                )}
                              >
                                {style.label}
                              </span>
                              {initiativeItem?.role ? (
                                <span className="rounded-full bg-muted px-2 py-0.5 text-[10px] text-muted-foreground">
                                  {initiativeItem.role}
                                </span>
                              ) : null}
                            </div>
                            {workItem.body ? (
                              <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                                {workItem.body}
                              </p>
                            ) : null}
                          </div>
                          <ArrowUpRight className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
                        </Link>
                      );
                    })}
                  </div>
                ) : (
                  <div className="rounded-lg border px-4 py-8 text-center text-sm text-muted-foreground">
                    暂无挂载的 work items
                  </div>
                )}
              </section>

              <section>
                <div className="mb-3 flex items-center justify-between">
                  <h3 className="text-sm font-semibold">关联 Threads</h3>
                  <span className="text-xs text-muted-foreground">
                    {detail.threads.length} threads
                  </span>
                </div>
                {detail.threads.length > 0 ? (
                  <div className="space-y-2">
                    {detail.threads.map((link) => {
                      const thread = linkedThreads[link.thread_id];
                      return (
                        <Link
                          key={link.id}
                          to={`/threads/${link.thread_id}`}
                          className="flex items-center gap-3 rounded-lg border px-3 py-3 transition-colors hover:bg-muted/40"
                        >
                          <MessageCircle className="h-4 w-4 shrink-0 text-blue-600" />
                          <div className="min-w-0 flex-1">
                            <div className="truncate text-sm font-medium">
                              {thread?.title ?? `Thread #${link.thread_id}`}
                            </div>
                            <div className="mt-1 flex items-center gap-2 text-[11px] text-muted-foreground">
                              <span>{link.relation_type || "related"}</span>
                              {thread?.updated_at ? (
                                <span>{formatRelativeTime(thread.updated_at)}</span>
                              ) : null}
                            </div>
                          </div>
                          <ArrowUpRight className="h-4 w-4 shrink-0 text-muted-foreground" />
                        </Link>
                      );
                    })}
                  </div>
                ) : (
                  <div className="rounded-lg border px-4 py-8 text-center text-sm text-muted-foreground">
                    暂无关联 threads
                  </div>
                )}
              </section>
            </div>
          ) : (
            <div className="rounded-lg border px-4 py-8 text-center text-sm text-muted-foreground">
              {loading ? "正在加载 initiative..." : "未找到 initiative。"}
            </div>
          )}
        </div>

        <div className="w-80 shrink-0 overflow-y-auto border-l px-5 py-6">
          <div className="space-y-5">
            <section className="space-y-3">
              <h4 className="text-sm font-semibold">审批入口</h4>
              <div className="space-y-2">
                <label className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                  Approved By
                </label>
                <Input
                  value={reviewForm.approvedBy}
                  onChange={(event) =>
                    setReviewForm((prev) => ({
                      ...prev,
                      approvedBy: event.target.value,
                    }))
                  }
                  placeholder="reviewer id"
                />
              </div>
              <div className="space-y-2">
                <label className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                  Review Note
                </label>
                <Textarea
                  value={reviewForm.reviewNote}
                  onChange={(event) =>
                    setReviewForm((prev) => ({
                      ...prev,
                      reviewNote: event.target.value,
                    }))
                  }
                  placeholder="记录审批意见或返工说明"
                  className="min-h-[96px] resize-y text-sm"
                />
              </div>
            </section>

            <hr className="border-border" />

            <section className="space-y-2 text-xs">
              <h4 className="text-sm font-semibold">基本信息</h4>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Initiative ID</span>
                <span>#{initiative?.id ?? initiativeId}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Status</span>
                <span>{statusStyle.label}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Created By</span>
                <span>{initiative?.created_by || "—"}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Approved By</span>
                <span>{initiative?.approved_by || "—"}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Updated</span>
                <span>
                  {initiative?.updated_at
                    ? formatRelativeTime(initiative.updated_at)
                    : "—"}
                </span>
              </div>
            </section>
          </div>
        </div>
      </div>
    </div>
  );
}
