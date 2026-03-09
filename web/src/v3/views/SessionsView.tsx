import { useEffect, useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import type { A2AClient } from "@/lib/a2aClient";
import type { ApiClient } from "@/lib/apiClient";
import type { WsClient } from "@/lib/wsClient";
import type { AgentInfo, ChatEventsPage, ChatRunEvent } from "@/types/api";
import type { ChatMessage, ChatSession } from "@/types/workflow";

interface SessionsViewProps {
  apiClient: ApiClient;
  a2aClient: A2AClient;
  wsClient: WsClient;
  projectId: string;
  a2aEnabled: boolean;
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

const previewFromSession = (session: ChatSession): string => {
  const lastMessage = session.messages.at(-1);
  if (!lastMessage?.content) {
    return "暂无消息";
  }
  return lastMessage.content;
};

const eventTone = (type: string): string => {
  if (type.includes("failed") || type.includes("error")) {
    return "border-rose-200 bg-rose-50";
  }
  if (type.includes("completed") || type.includes("done")) {
    return "border-emerald-200 bg-emerald-50";
  }
  if (type.includes("thought") || type.includes("started") || type.includes("update")) {
    return "border-blue-200 bg-blue-50";
  }
  return "border-slate-200 bg-slate-50";
};

const formatEventName = (type: string): string => {
  return type
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
};

const SessionsView = ({
  apiClient,
  projectId,
  a2aEnabled,
}: SessionsViewProps) => {
  const [sessions, setSessions] = useState<ChatSession[]>([]);
  const [sessionsLoading, setSessionsLoading] = useState(true);
  const [sessionsError, setSessionsError] = useState<string | null>(null);
  const [selectedSessionId, setSelectedSessionId] = useState("");
  const [sessionDetail, setSessionDetail] = useState<ChatEventsPage | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [draftMessage, setDraftMessage] = useState("");
  const [agentName, setAgentName] = useState("");
  const [agents, setAgents] = useState<AgentInfo[]>([]);
  const [createLoading, setCreateLoading] = useState(false);
  const [feedback, setFeedback] = useState<string | null>(null);
  const [search, setSearch] = useState("");

  useEffect(() => {
    let cancelled = false;
    const loadSessions = async () => {
      setSessionsLoading(true);
      setSessionsError(null);
      try {
        const [sessionResponse, agentResponse] = await Promise.all([
          apiClient.listChats(projectId),
          apiClient.listAgents().catch(() => ({ agents: [] })),
        ]);
        if (cancelled) {
          return;
        }
        const nextSessions = Array.isArray(sessionResponse) ? sessionResponse : [];
        setSessions(nextSessions);
        setAgents(Array.isArray(agentResponse.agents) ? agentResponse.agents : []);
        setAgentName((current) => current || agentResponse.agents?.[0]?.name || "");
        setSelectedSessionId((current) => {
          if (current && nextSessions.some((session) => session.id === current)) {
            return current;
          }
          return nextSessions[0]?.id ?? "";
        });
      } catch (error) {
        if (cancelled) {
          return;
        }
        setSessions([]);
        setSelectedSessionId("");
        setSessionsError(getErrorMessage(error));
      } finally {
        if (!cancelled) {
          setSessionsLoading(false);
        }
      }
    };

    void loadSessions();
    return () => {
      cancelled = true;
    };
  }, [apiClient, projectId]);

  useEffect(() => {
    if (!selectedSessionId) {
      setSessionDetail(null);
      return;
    }

    let cancelled = false;
    const loadSessionDetail = async () => {
      setDetailLoading(true);
      try {
        const response = await apiClient.listChatRunEvents(projectId, selectedSessionId, { limit: 40 });
        if (cancelled) {
          return;
        }
        setSessionDetail(response);
      } catch {
        if (cancelled) {
          return;
        }
        setSessionDetail(null);
      } finally {
        if (!cancelled) {
          setDetailLoading(false);
        }
      }
    };

    void loadSessionDetail();
    return () => {
      cancelled = true;
    };
  }, [apiClient, projectId, selectedSessionId]);

  const filteredSessions = useMemo(() => {
    const keyword = search.trim().toLowerCase();
    if (!keyword) {
      return sessions;
    }
    return sessions.filter((session) =>
      `${session.id} ${previewFromSession(session)}`.toLowerCase().includes(keyword),
    );
  }, [sessions, search]);

  const sessionStats = useMemo(() => {
    return {
      total: sessions.length,
      withMessages: sessions.filter((session) => session.messages.length > 0).length,
      activeEvents: sessionDetail?.events.length ?? 0,
    };
  }, [sessionDetail?.events.length, sessions]);

  const selectedSession = sessions.find((session) => session.id === selectedSessionId) ?? null;
  const extractedSummary = useMemo(() => {
    const messages = sessionDetail?.messages ?? [];
    const lastAssistant = [...messages].reverse().find((message) => message.role === "assistant");
    const summaryText = lastAssistant?.content || "当前会话尚未形成稳定的 assistant 摘要。";
    return {
      goal: summaryText.slice(0, 80) || "目标待提炼",
      scope: summaryText.slice(80, 180) || "范围待明确",
      acceptance: summaryText.slice(180, 260) || "验收条件待沉淀",
    };
  }, [sessionDetail?.messages]);

  const handleCreateChat = async () => {
    const message = draftMessage.trim();
    if (!message) {
      setFeedback("请先输入会话内容。");
      return;
    }
    setCreateLoading(true);
    setFeedback(null);
    try {
      const response = await apiClient.createChat(projectId, {
        message,
        agent_name: agentName || undefined,
      });
      setDraftMessage("");
      setFeedback(`已创建会话 ${response.session_id}。`);
      const refreshed = await apiClient.listChats(projectId);
      const nextSessions = Array.isArray(refreshed) ? refreshed : [];
      setSessions(nextSessions);
      setSelectedSessionId(response.session_id);
    } catch (error) {
      setFeedback(getErrorMessage(error));
    } finally {
      setCreateLoading(false);
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
                  Sessions & Threads
                </Badge>
                <Badge variant="outline" className={a2aEnabled ? "bg-emerald-50 text-emerald-700" : "bg-slate-50 text-slate-600"}>
                  {a2aEnabled ? "A2A 开启" : "标准会话"}
                </Badge>
              </div>
              <CardTitle className="mt-3 text-[24px] font-semibold tracking-[-0.02em]">
                会话 / 线程 工作区
              </CardTitle>
              <CardDescription className="mt-1">
                左侧收件箱，中间消息与事件流，右侧提炼结果与下游动作建议。
              </CardDescription>
            </div>
            <div className="grid gap-2 rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3">
              <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">当前会话</p>
              <p className="text-sm font-semibold text-slate-950">{selectedSession?.id || "尚未选择"}</p>
              <p className="text-xs text-slate-500">{sessionDetail?.agent_name || "等待载入 agent / 事件组"}</p>
            </div>
          </div>
        </CardHeader>
        <CardContent className="grid gap-3 px-5 pb-5 md:grid-cols-[1.2fr_auto_auto_auto]">
          <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
            <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">搜索会话</p>
            <Input
              className="mt-3 bg-white"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="搜索会话 ID、关键词、最近消息"
            />
          </div>
          <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
            <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">总会话</p>
            <p className="mt-2 text-2xl font-semibold text-slate-950">{sessionStats.total}</p>
          </div>
          <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
            <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">有消息</p>
            <p className="mt-2 text-2xl font-semibold text-slate-950">{sessionStats.withMessages}</p>
          </div>
          <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
            <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">事件数</p>
            <p className="mt-2 text-2xl font-semibold text-slate-950">{sessionStats.activeEvents}</p>
          </div>
        </CardContent>
      </Card>

      <div className="grid gap-4 xl:grid-cols-[0.74fr_1.08fr_0.82fr]">
        <div className="flex flex-col gap-4">
          <Card className="rounded-2xl border-slate-200 shadow-none">
            <CardHeader>
              <CardTitle className="text-base">会话收件箱</CardTitle>
              <CardDescription>按最近活动和可提炼性查看当前会话列表。</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              {sessionsLoading ? (
                <p className="text-sm text-slate-500">加载中...</p>
              ) : sessionsError ? (
                <p className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">
                  {sessionsError}
                </p>
              ) : filteredSessions.length === 0 ? (
                <p className="text-sm text-slate-500">当前没有会话。</p>
              ) : (
                filteredSessions.map((session) => (
                  <button
                    key={session.id}
                    type="button"
                    onClick={() => setSelectedSessionId(session.id)}
                    className={`w-full rounded-xl border px-4 py-3 text-left transition ${
                      selectedSessionId === session.id
                        ? "border-blue-300 bg-blue-50"
                        : "border-slate-200 bg-white hover:bg-slate-50"
                    }`}
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <p className="truncate text-sm font-semibold text-slate-950">{session.id}</p>
                        <p className="mt-1 line-clamp-2 text-xs leading-5 text-slate-500">{previewFromSession(session)}</p>
                      </div>
                      <Badge variant="outline">{session.messages.length}</Badge>
                    </div>
                    <p className="mt-2 text-[11px] text-slate-400">{formatTime(session.updated_at)}</p>
                  </button>
                ))
              )}
            </CardContent>
          </Card>

          <Card className="rounded-2xl border-slate-200 shadow-none">
            <CardHeader>
              <CardTitle className="text-base">新建会话</CardTitle>
              <CardDescription>从这里开始新的问题拆解、目标提炼或澄清线程。</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <label className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">Agent</label>
                <Select value={agentName} onChange={(event) => setAgentName(event.target.value)}>
                  <option value="">默认 Agent</option>
                  {agents.map((agent) => (
                    <option key={agent.name} value={agent.name}>
                      {agent.name}
                    </option>
                  ))}
                </Select>
              </div>
              <Textarea
                value={draftMessage}
                onChange={(event) => setDraftMessage(event.target.value)}
                className="min-h-[112px] bg-slate-50"
                placeholder="先描述目标、范围和验收条件，再决定直接建 Issue 还是进入 DAG 拆解。"
              />
              <Button variant="secondary" onClick={() => void handleCreateChat()} disabled={createLoading}>
                {createLoading ? "创建中..." : "创建会话"}
              </Button>
              {feedback ? (
                <p className="rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 text-sm text-slate-600">
                  {feedback}
                </p>
              ) : null}
            </CardContent>
          </Card>
        </div>

        <Card className="rounded-2xl border-slate-200 shadow-none">
          <CardHeader>
            <CardTitle className="text-base">当前对话 / 事件流</CardTitle>
            <CardDescription>以消息 + 事件组的混排方式查看当前会话，而不是原始聊天窗口。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {detailLoading ? (
              <p className="text-sm text-slate-500">加载中...</p>
            ) : !sessionDetail ? (
              <p className="text-sm text-slate-500">请选择一个会话查看内容。</p>
            ) : (
              <>
                {(sessionDetail.messages as ChatMessage[]).map((message, index) => (
                  <div
                    key={`${message.time}-${index}`}
                    className={`rounded-xl border px-4 py-3 ${
                      message.role === "assistant" ? "border-blue-200 bg-blue-50" : "border-slate-200 bg-slate-50"
                    }`}
                  >
                    <div className="flex items-center justify-between gap-3">
                      <p className="text-sm font-semibold text-slate-950">{message.role === "assistant" ? "Assistant" : "User"}</p>
                      <p className="text-[11px] text-slate-400">{message.time}</p>
                    </div>
                    <p className="mt-2 whitespace-pre-wrap text-sm leading-6 text-slate-700">{message.content}</p>
                  </div>
                ))}
                {(sessionDetail.events as ChatRunEvent[]).slice(0, 8).map((event) => (
                  <div key={event.id} className={`rounded-xl border px-4 py-3 ${eventTone(event.event_type)}`}>
                    <div className="flex items-start justify-between gap-3">
                      <div>
                        <p className="text-sm font-semibold text-slate-950">{formatEventName(event.event_type)}</p>
                        <p className="mt-1 text-xs text-slate-500">
                          {event.update_type} · {formatTime(event.created_at)}
                        </p>
                      </div>
                      <Badge variant="outline">{event.id}</Badge>
                    </div>
                  </div>
                ))}
              </>
            )}
          </CardContent>
        </Card>

        <div className="flex flex-col gap-4">
          <Card className="rounded-2xl border-slate-200 shadow-none">
            <CardHeader>
              <CardTitle className="text-base">提炼结果</CardTitle>
              <CardDescription>把对话先沉淀成目标、范围和验收，再决定是否转任务。</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
                <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">目标</p>
                <p className="mt-2 text-xs leading-5 text-slate-600">{extractedSummary.goal}</p>
              </div>
              <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
                <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">范围</p>
                <p className="mt-2 text-xs leading-5 text-slate-600">{extractedSummary.scope}</p>
              </div>
              <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
                <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">验收</p>
                <p className="mt-2 text-xs leading-5 text-slate-600">{extractedSummary.acceptance}</p>
              </div>
            </CardContent>
          </Card>

          <Card className="rounded-2xl border-slate-200 shadow-none">
            <CardHeader>
              <CardTitle className="text-base">转任务建议</CardTitle>
              <CardDescription>先从会话提炼出结构化结论，再进入 DAG 或 Issue 工作台。</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="rounded-2xl border border-slate-200 bg-indigo-50 p-4">
                <p className="text-sm font-semibold text-slate-950">建议路径</p>
                <p className="mt-1 text-xs leading-5 text-slate-600">
                  如果会话目标仍然模糊，先去 DAG 拆解；如果验收已经清晰，可直接进入 Issue 工作台。
                </p>
              </div>
              <div className="flex flex-wrap gap-2">
                <Button variant="secondary" size="sm">转 Issue 工作台</Button>
                <Button variant="outline" size="sm">转 DAG 拆解</Button>
              </div>
            </CardContent>
          </Card>

          <Card className="rounded-2xl border-slate-200 shadow-none">
            <CardHeader>
              <CardTitle className="text-base">高级控制 / 下游流转</CardTitle>
              <CardDescription>只放必要的低频动作，不让主工作区被控制项占满。</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
                <p className="text-sm font-semibold text-slate-950">下游流转</p>
                <p className="mt-1 text-xs leading-5 text-slate-600">
                  会话提炼之后，可继续进入 Proposal DAG，再在 Issue 工作台完成确认和推进。
                </p>
              </div>
              <Button variant="outline" size="sm">刷新当前会话</Button>
            </CardContent>
          </Card>
        </div>
      </div>
    </section>
  );
};

export default SessionsView;
