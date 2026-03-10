import { useCallback, useEffect, useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import type { ApiClientV2 } from "@/lib/apiClientV2";
import type { Event } from "@/types/apiV2";
import { PageScaffold } from "@/v3/components/PageScaffold";

interface EventsViewProps {
  apiClient: ApiClientV2;
  apiBaseUrl: string;
  getToken: () => string | null;
  flowId: number;
  refreshToken: number;
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

const POLL_INTERVAL_MS = 1500;

const EventsView = ({ apiClient, apiBaseUrl, getToken, flowId, refreshToken }: EventsViewProps) => {
  const [events, setEvents] = useState<Event[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [polling, setPolling] = useState(true);
  const [realtime, setRealtime] = useState(true);

  const buildWsUrl = (baseUrl: string, token: string, flowId: number): string => {
    const url = (() => {
      if (/^wss?:\/\//.test(baseUrl) || /^https?:\/\//.test(baseUrl)) {
        return new URL(baseUrl);
      }
      if (typeof window !== "undefined" && window.location?.origin) {
        return new URL(baseUrl, window.location.origin);
      }
      return new URL(baseUrl, "http://localhost");
    })();

    if (url.protocol === "http:") {
      url.protocol = "ws:";
    } else if (url.protocol === "https:") {
      url.protocol = "wss:";
    }

    const normalizedPath = url.pathname.replace(/\/+$/, "");
    url.pathname = normalizedPath.endsWith("/ws") ? normalizedPath : `${normalizedPath}/ws`;
    url.searchParams.set("token", token);
    url.searchParams.set("flow_id", String(flowId));
    return url.toString();
  };

  const reload = useCallback(async () => {
    setError(null);
    const listed = await apiClient.listFlowEvents(flowId, { limit: 200, offset: 0 });
    setEvents(listed);
  }, [apiClient, flowId]);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        const listed = await apiClient.listFlowEvents(flowId, { limit: 200, offset: 0 });
        if (!cancelled) {
          setEvents(listed);
        }
      } catch (err) {
        if (!cancelled) {
          setError(getErrorMessage(err));
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };
    void load();
    return () => {
      cancelled = true;
    };
  }, [apiClient, flowId, refreshToken]);

  useEffect(() => {
    if (!polling || realtime) {
      return;
    }
    let cancelled = false;
    const handle = window.setInterval(() => {
      if (cancelled) {
        return;
      }
      void reload().catch((err) => {
        if (!cancelled) {
          setError(getErrorMessage(err));
        }
      });
    }, POLL_INTERVAL_MS);
    return () => {
      cancelled = true;
      window.clearInterval(handle);
    };
  }, [polling, reload]);

  useEffect(() => {
    if (!realtime) {
      return;
    }
    const token = getToken();
    if (!token) {
      return;
    }

    let cancelled = false;
    let socket: WebSocket | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

    const connect = () => {
      if (cancelled) return;
      const wsUrl = buildWsUrl(apiBaseUrl, token, flowId);
      socket = new WebSocket(wsUrl);

      socket.onmessage = (ev) => {
        try {
          const parsed = JSON.parse(String(ev.data)) as Event;
          if (!parsed || typeof parsed !== "object") return;
          setEvents((current) => {
            const next = [parsed, ...current];
            return next.slice(0, 400);
          });
        } catch {
          // ignore
        }
      };

      socket.onclose = () => {
        socket = null;
        if (cancelled) return;
        reconnectTimer = setTimeout(connect, 1000);
      };

      socket.onerror = () => {
        // onclose will handle reconnect
      };
    };

    connect();

    return () => {
      cancelled = true;
      if (reconnectTimer) {
        clearTimeout(reconnectTimer);
      }
      if (socket) {
        socket.close();
      }
    };
  }, [apiBaseUrl, flowId, getToken, realtime]);

  const rendered = useMemo(() => {
    const sorted = [...events].sort((a, b) => b.id - a.id);
    return sorted.slice(0, 200);
  }, [events]);

  return (
    <PageScaffold
      eyebrow="事件流"
      title={`Events（Flow #${flowId}）`}
      description="v2 事件流（Flow/Step/Exec）。默认开启 WebSocket 实时；也可切换为轮询刷新。"
      contextTitle={realtime ? "实时：WebSocket" : polling ? "模式：轮询" : "模式：手动刷新"}
      contextMeta={`数量：${rendered.length} · flow_id=${flowId}`}
      actions={[
        { label: "刷新", onClick: () => void reload(), variant: "outline" },
        {
          label: realtime ? "关闭实时" : "开启实时",
          onClick: () => {
            setRealtime((v) => !v);
            setPolling(false);
          },
          variant: "outline",
        },
        {
          label: polling ? "停止轮询" : "开始轮询",
          onClick: () => {
            setPolling((v) => !v);
            setRealtime(false);
          },
          variant: "outline",
        },
      ]}
    >
      <Card className="rounded-2xl border-slate-200 shadow-none">
        <CardContent className="space-y-3 px-5 pb-5">
          {error ? <p className="text-sm text-red-600">{error}</p> : null}
          {loading ? <p className="text-sm text-slate-500">加载中...</p> : null}
          {!loading && rendered.length === 0 ? (
            <p className="text-sm text-slate-500">暂无事件。</p>
          ) : null}
          <div className="grid gap-2">
            {rendered.map((event) => (
              <div key={event.id} className="rounded-2xl border border-slate-200 bg-white px-4 py-3">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <p className="text-sm font-semibold text-slate-900">
                      #{event.id} · {event.type}
                    </p>
                    <p className="mt-1 text-[11px] text-slate-500">{formatTime(event.timestamp)}</p>
                  </div>
                  <div className="flex flex-wrap items-center gap-2">
                    {typeof event.step_id === "number" ? (
                      <Badge variant="outline" className="bg-slate-50 text-slate-600">
                        step {event.step_id}
                      </Badge>
                    ) : null}
                    {typeof event.exec_id === "number" ? (
                      <Badge variant="outline" className="bg-slate-50 text-slate-600">
                        exec {event.exec_id}
                      </Badge>
                    ) : null}
                  </div>
                </div>
                {event.data && Object.keys(event.data).length > 0 ? (
                  <pre className="mt-2 overflow-auto rounded-xl bg-slate-950 px-3 py-2 text-xs text-slate-100">
                    {JSON.stringify(event.data, null, 2)}
                  </pre>
                ) : null}
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </PageScaffold>
  );
};

export default EventsView;
