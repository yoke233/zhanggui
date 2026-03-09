import { useEffect, useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import type { ApiClient } from "@/lib/apiClient";
import type { ApiIssue, DecomposeProposal, IssueTimelineEntry } from "@/types/api";

interface IssuesViewProps {
  apiClient: ApiClient;
  projectId: string;
  refreshToken: number;
}

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

const statusVariant = (status: string) => {
  switch (status.trim().toLowerCase()) {
    case "ready":
    case "reviewing":
    case "queued":
      return "warning" as const;
    case "done":
      return "success" as const;
    case "failed":
    case "abandoned":
    case "superseded":
      return "danger" as const;
    default:
      return "outline" as const;
  }
};

const toneClass = (status: string): string => {
  switch (status.trim().toLowerCase()) {
    case "success":
      return "border-emerald-200 bg-emerald-50";
    case "warning":
      return "border-amber-200 bg-amber-50";
    case "failed":
      return "border-rose-200 bg-rose-50";
    case "running":
      return "border-blue-200 bg-blue-50";
    default:
      return "border-slate-200 bg-slate-50";
  }
};

const IssuesView = ({ apiClient, projectId, refreshToken }: IssuesViewProps) => {
  const [issues, setIssues] = useState<ApiIssue[]>([]);
  const [issuesLoading, setIssuesLoading] = useState(true);
  const [issuesError, setIssuesError] = useState<string | null>(null);
  const [selectedIssueId, setSelectedIssueId] = useState<string | null>(null);
  const [timeline, setTimeline] = useState<IssueTimelineEntry[]>([]);
  const [timelineLoading, setTimelineLoading] = useState(false);
  const [dagSummary, setDagSummary] = useState<{
    nodes: number;
    edges: number;
    ready: number;
    running: number;
    failed: number;
  } | null>(null);
  const [proposalPrompt, setProposalPrompt] = useState("");
  const [proposal, setProposal] = useState<DecomposeProposal | null>(null);
  const [proposalLoading, setProposalLoading] = useState(false);
  const [proposalActionLoading, setProposalActionLoading] = useState(false);
  const [proposalFeedback, setProposalFeedback] = useState<string | null>(null);
  const [search, setSearch] = useState("");

  useEffect(() => {
    let cancelled = false;
    const loadIssues = async () => {
      setIssuesLoading(true);
      setIssuesError(null);
      try {
        const response = await apiClient.listIssues(projectId, { limit: 50, offset: 0 });
        if (cancelled) {
          return;
        }
        const nextItems = Array.isArray(response.items) ? response.items : [];
        setIssues(nextItems);
        setSelectedIssueId((current) => {
          if (current && nextItems.some((item) => item.id === current)) {
            return current;
          }
          return nextItems[0]?.id ?? null;
        });
      } catch (error) {
        if (cancelled) {
          return;
        }
        setIssues([]);
        setSelectedIssueId(null);
        setIssuesError(getErrorMessage(error));
      } finally {
        if (!cancelled) {
          setIssuesLoading(false);
        }
      }
    };

    void loadIssues();
    return () => {
      cancelled = true;
    };
  }, [apiClient, projectId, refreshToken]);

  useEffect(() => {
    if (!selectedIssueId) {
      setTimeline([]);
      setDagSummary(null);
      return;
    }

    let cancelled = false;
    const loadIssueDetail = async () => {
      setTimelineLoading(true);
      try {
        const [timelineResponse, dagResponse] = await Promise.all([
          apiClient.listIssueTimeline(projectId, selectedIssueId, { limit: 8, offset: 0 }),
          apiClient.getIssueDag(projectId, selectedIssueId),
        ]);
        if (cancelled) {
          return;
        }
        setTimeline(Array.isArray(timelineResponse.items) ? timelineResponse.items : []);
        setDagSummary({
          nodes: dagResponse.nodes.length,
          edges: dagResponse.edges.length,
          ready: dagResponse.stats.ready,
          running: dagResponse.stats.running,
          failed: dagResponse.stats.failed,
        });
      } catch {
        if (cancelled) {
          return;
        }
        setTimeline([]);
        setDagSummary(null);
      } finally {
        if (!cancelled) {
          setTimelineLoading(false);
        }
      }
    };

    void loadIssueDetail();
    return () => {
      cancelled = true;
    };
  }, [apiClient, projectId, selectedIssueId]);

  const filteredIssues = useMemo(() => {
    const keyword = search.trim().toLowerCase();
    if (!keyword) {
      return issues;
    }
    return issues.filter((issue) => {
      return [
        issue.title,
        issue.id,
        issue.run_id,
        issue.github?.issue_number ? String(issue.github.issue_number) : "",
      ]
        .join(" ")
        .toLowerCase()
        .includes(keyword);
    });
  }, [issues, search]);

  const issueStats = useMemo(() => {
    return issues.reduce(
      (summary, issue) => {
        const status = String(issue.status ?? "").trim().toLowerCase();
        summary.total += 1;
        if (status === "ready" || status === "reviewing" || status === "queued") {
          summary.review += 1;
        }
        if (status === "running" || status === "executing" || status === "merging") {
          summary.active += 1;
        }
        if (status === "done") {
          summary.done += 1;
        }
        return summary;
      },
      { total: 0, review: 0, active: 0, done: 0 },
    );
  }, [issues]);

  const selectedIssue = issues.find((issue) => issue.id === selectedIssueId) ?? null;

  const handleDecompose = async () => {
    const prompt = proposalPrompt.trim();
    if (!prompt) {
      setProposalFeedback("请先输入一句话拆解需求。");
      return;
    }
    setProposalLoading(true);
    setProposalFeedback(null);
    try {
      const nextProposal = await apiClient.decompose(projectId, { prompt });
      setProposal(nextProposal);
      setProposalFeedback(`已生成 ${nextProposal.issues.length} 个 DAG 节点草案。`);
    } catch (error) {
      setProposal(null);
      setProposalFeedback(getErrorMessage(error));
    } finally {
      setProposalLoading(false);
    }
  };

  const handleConfirmProposal = async () => {
    if (!proposal) {
      return;
    }
    setProposalActionLoading(true);
    setProposalFeedback(null);
    try {
      const response = await apiClient.confirmDecompose(projectId, {
        proposal_id: proposal.proposal_id,
        issues: proposal.issues,
      });
      setProposalFeedback(`已创建 ${response.created_issues.length} 个 Issue。`);
      setProposal(null);
      setProposalPrompt("");
      const refresh = await apiClient.listIssues(projectId, { limit: 50, offset: 0 });
      setIssues(Array.isArray(refresh.items) ? refresh.items : []);
    } catch (error) {
      setProposalFeedback(getErrorMessage(error));
    } finally {
      setProposalActionLoading(false);
    }
  };

  return (
    <section className="flex flex-col gap-4">
      <Card className="rounded-2xl border-slate-200 shadow-none">
        <CardHeader className="p-5">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div>
              <div className="flex items-center gap-2">
                <Badge variant="secondary" className="bg-indigo-50 text-indigo-600">
                  Projects & Issues
                </Badge>
                <Badge variant="outline" className="bg-amber-50 text-amber-700">
                  DAG 驱动
                </Badge>
              </div>
              <CardTitle className="mt-3 text-[24px] font-semibold tracking-[-0.02em]">
                项目 / Issue 工作台
              </CardTitle>
              <CardDescription className="mt-1">
                先生成 Proposal DAG，再预览编辑后批量创建 Issue；右侧只保留当前焦点与诊断。
              </CardDescription>
            </div>
            <div className="grid gap-2 rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3">
              <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">当前上下文</p>
              <p className="text-sm font-semibold text-slate-950">{projectId}</p>
              <p className="text-xs text-slate-500">先拆解，再确认，再批量推进。</p>
            </div>
          </div>
        </CardHeader>
        <CardContent className="grid gap-3 px-5 pb-5 md:grid-cols-[1.2fr_0.8fr_auto_auto]">
          <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
            <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">搜索 / 筛选</p>
            <Input
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              className="mt-3 bg-white"
              placeholder="搜索标题 / Issue ID / Run ID"
            />
          </div>
          <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
            <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">待审 / 执行</p>
            <p className="mt-2 text-2xl font-semibold text-slate-950">{issueStats.review}</p>
            <p className="mt-1 text-xs text-slate-500">reviewing / ready / queued</p>
          </div>
          <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
            <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">活动中</p>
            <p className="mt-2 text-2xl font-semibold text-slate-950">{issueStats.active}</p>
            <p className="mt-1 text-xs text-slate-500">executing / merging</p>
          </div>
          <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
            <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">已完成</p>
            <p className="mt-2 text-2xl font-semibold text-slate-950">{issueStats.done}</p>
            <p className="mt-1 text-xs text-slate-500">done</p>
          </div>
        </CardContent>
      </Card>

      <div className="grid gap-4 xl:grid-cols-[1.02fr_0.98fr_0.72fr]">
        <div className="flex flex-col gap-4">
          <Card className="rounded-2xl border-slate-200 shadow-none">
            <CardHeader>
              <CardTitle className="text-base">Proposal 草案</CardTitle>
              <CardDescription>一句话需求先拆成 DAG，再决定是否确认批量创建。</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <Textarea
                value={proposalPrompt}
                onChange={(event) => setProposalPrompt(event.target.value)}
                className="min-h-[112px] bg-slate-50"
                placeholder="例如：重做 v3 会话页，先提炼目标/范围/验收，再决定直接建 Issue 还是进入 DAG 拆解。"
              />
              <div className="flex flex-wrap gap-3">
                <Button variant="secondary" onClick={() => void handleDecompose()} disabled={proposalLoading}>
                  {proposalLoading ? "拆解中..." : "生成 DAG"}
                </Button>
                <Button
                  variant="outline"
                  onClick={() => void handleConfirmProposal()}
                  disabled={!proposal || proposalActionLoading}
                >
                  {proposalActionLoading ? "创建中..." : "确认并创建 Issue"}
                </Button>
              </div>
              {proposalFeedback ? (
                <p className="rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 text-sm text-slate-600">
                  {proposalFeedback}
                </p>
              ) : null}
              {proposal ? (
                <div className="grid gap-2">
                  {proposal.issues.slice(0, 4).map((item) => (
                    <div key={item.temp_id} className="rounded-xl border border-slate-200 bg-slate-50 px-3 py-3">
                      <div className="flex items-start justify-between gap-3">
                        <div>
                          <p className="text-sm font-semibold text-slate-950">{item.title}</p>
                          <p className="mt-1 text-xs leading-5 text-slate-500">{item.body}</p>
                        </div>
                        <Badge variant="outline">{item.depends_on.length} deps</Badge>
                      </div>
                    </div>
                  ))}
                </div>
              ) : null}
            </CardContent>
          </Card>

          <Card className="rounded-2xl border-slate-200 shadow-none">
            <CardHeader>
              <CardTitle className="text-base">Issue 队列</CardTitle>
              <CardDescription>列表只显示决策所需信息，进入右侧查看时间线和 DAG 摘要。</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              {issuesLoading ? (
                <p className="text-sm text-slate-500">加载中...</p>
              ) : issuesError ? (
                <p className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">
                  {issuesError}
                </p>
              ) : filteredIssues.length === 0 ? (
                <p className="text-sm text-slate-500">当前没有 Issue。</p>
              ) : (
                filteredIssues.map((issue) => (
                  <button
                    key={issue.id}
                    type="button"
                    onClick={() => setSelectedIssueId(issue.id)}
                    className={`w-full rounded-xl border px-4 py-3 text-left transition ${
                      selectedIssueId === issue.id
                        ? "border-blue-300 bg-blue-50"
                        : "border-slate-200 bg-white hover:bg-slate-50"
                    }`}
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <p className="truncate text-sm font-semibold text-slate-950">{issue.title || issue.id}</p>
                        <p className="mt-1 text-xs text-slate-500">
                          {issue.github?.issue_number ? `#${issue.github.issue_number}` : issue.id} · run {issue.run_id || "-"}
                        </p>
                      </div>
                      <Badge variant={statusVariant(String(issue.status ?? ""))}>{issue.status}</Badge>
                    </div>
                  </button>
                ))
              )}
            </CardContent>
          </Card>
        </div>

        <Card className="rounded-2xl border-slate-200 shadow-none">
          <CardHeader>
            <CardTitle className="text-base">当前详情 / 事件流</CardTitle>
            <CardDescription>围绕当前焦点 Issue 展示状态、依赖、时间线和最近动作。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {!selectedIssue ? (
              <p className="text-sm text-slate-500">请选择一个 Issue 查看详情。</p>
            ) : (
              <>
                <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <p className="text-sm font-semibold text-slate-950">{selectedIssue.title || selectedIssue.id}</p>
                      <p className="mt-1 text-xs text-slate-500">{selectedIssue.body || "暂无描述。"}</p>
                    </div>
                    <Badge variant={statusVariant(String(selectedIssue.status ?? ""))}>{selectedIssue.status}</Badge>
                  </div>
                  <div className="mt-3 grid gap-2 text-xs text-slate-500 md:grid-cols-2">
                    <p>更新时间：{formatTime(selectedIssue.updated_at)}</p>
                    <p>依赖：{selectedIssue.depends_on.length}</p>
                    <p>子模式：{selectedIssue.children_mode || "parallel"}</p>
                    <p>自动合并：{selectedIssue.auto_merge ? "已开启" : "关闭"}</p>
                  </div>
                </div>

                <div className="grid gap-3 md:grid-cols-3">
                  <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
                    <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">DAG 节点</p>
                    <p className="mt-2 text-2xl font-semibold text-slate-950">{dagSummary?.nodes ?? 0}</p>
                  </div>
                  <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
                    <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">Ready / Running</p>
                    <p className="mt-2 text-2xl font-semibold text-slate-950">
                      {(dagSummary?.ready ?? 0) + (dagSummary?.running ?? 0)}
                    </p>
                  </div>
                  <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
                    <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">失败节点</p>
                    <p className="mt-2 text-2xl font-semibold text-slate-950">{dagSummary?.failed ?? 0}</p>
                  </div>
                </div>

                <div className="space-y-3">
                  {timelineLoading ? (
                    <p className="text-sm text-slate-500">正在加载时间线...</p>
                  ) : timeline.length === 0 ? (
                    <p className="text-sm text-slate-500">暂无时间线记录。</p>
                  ) : (
                    timeline.map((entry) => (
                      <div key={entry.event_id} className={`rounded-xl border px-4 py-3 ${toneClass(entry.status)}`}>
                        <div className="flex items-start justify-between gap-3">
                          <div>
                            <p className="text-sm font-semibold text-slate-950">{entry.title}</p>
                            <p className="mt-1 text-xs leading-5 text-slate-600">{entry.body || "暂无补充说明。"}</p>
                          </div>
                          <Badge variant="outline">{entry.actor_name}</Badge>
                        </div>
                        <p className="mt-2 text-[11px] text-slate-500">
                          {formatTime(entry.created_at)} · {entry.kind} · {entry.refs.stage || "stage-unknown"}
                        </p>
                      </div>
                    ))
                  )}
                </div>
              </>
            )}
          </CardContent>
        </Card>

        <Card className="rounded-2xl border-slate-200 shadow-none">
          <CardHeader>
            <CardTitle className="text-base">侧栏摘要</CardTitle>
            <CardDescription>按设计稿保留焦点摘要、提炼提示和人工动作建议。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
              <p className="text-sm font-semibold text-slate-950">当前焦点</p>
              <p className="mt-1 text-xs leading-5 text-slate-500">
                {selectedIssue
                  ? `${selectedIssue.title || selectedIssue.id} 目前处于 ${selectedIssue.status}。`
                  : "从左侧选择一个 Issue，查看当前详情、依赖和时间线。"}
              </p>
            </div>
            <div className="rounded-2xl border border-slate-200 bg-amber-50 p-4">
              <p className="text-sm font-semibold text-slate-950">提炼建议</p>
              <p className="mt-1 text-xs leading-5 text-slate-600">
                先把一句话需求拆成 DAG，再确认是否真的需要批量建 Issue，避免任务颗粒度过粗。
              </p>
            </div>
            <div className="rounded-2xl border border-slate-200 bg-indigo-50 p-4">
              <p className="text-sm font-semibold text-slate-950">操作建议</p>
              <p className="mt-1 text-xs leading-5 text-slate-600">
                先处理 ready / review，再处理 running；失败节点优先排查依赖和检查点。
              </p>
            </div>
          </CardContent>
        </Card>
      </div>
    </section>
  );
};

export default IssuesView;
