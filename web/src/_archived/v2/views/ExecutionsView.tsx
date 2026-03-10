import { useEffect, useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import type { ApiClientV2 } from "@/lib/apiClientV2";
import type { Execution } from "@/types/apiV2";
import { PageScaffold } from "@/v3/components/PageScaffold";

interface ExecutionsViewProps {
  apiClient: ApiClientV2;
  stepId: number;
  refreshToken: number;
  onSelectExecution?: (execId: number) => void;
}

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error && error.message.trim().length > 0) {
    return error.message;
  }
  return "请求失败，请稍后重试";
};

const formatTime = (value?: string | null) => {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString("zh-CN", { hour12: false });
};

const statusTone = (status: string): "secondary" | "warning" | "success" | "danger" => {
  switch (status.trim().toLowerCase()) {
    case "running":
      return "warning";
    case "succeeded":
      return "success";
    case "failed":
    case "cancelled":
      return "danger";
    default:
      return "secondary";
  }
};

const ExecutionsView = ({ apiClient, stepId, refreshToken, onSelectExecution }: ExecutionsViewProps) => {
  const [executions, setExecutions] = useState<Execution[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedExecId, setSelectedExecId] = useState<number | null>(null);
  const [selectedExec, setSelectedExec] = useState<Execution | null>(null);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        const listed = await apiClient.listExecutions(stepId);
        if (cancelled) {
          return;
        }
        setExecutions(listed);
        if (listed.length > 0 && selectedExecId == null) {
          setSelectedExecId(listed[0].id);
        }
      } catch (err) {
        if (cancelled) {
          return;
        }
        setError(getErrorMessage(err));
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
  }, [apiClient, stepId, refreshToken, selectedExecId]);

  useEffect(() => {
    let cancelled = false;
    const loadExec = async () => {
      if (selectedExecId == null) {
        setSelectedExec(null);
        return;
      }
      try {
        const loaded = await apiClient.getExecution(selectedExecId);
        if (!cancelled) {
          setSelectedExec(loaded);
        }
      } catch (err) {
        if (!cancelled) {
          setError(getErrorMessage(err));
        }
      }
    };
    void loadExec();
    return () => {
      cancelled = true;
    };
  }, [apiClient, selectedExecId]);

  const selectedTitle = useMemo(() => {
    if (!selectedExec) {
      return "未选择 Execution";
    }
    const status = String(selectedExec.status ?? "").trim() || "unknown";
    return `Execution #${selectedExec.id} · ${status}`;
  }, [selectedExec]);

  return (
    <PageScaffold
      eyebrow="Executions / Attempts"
      title={`Executions（Step #${stepId}）`}
      description="Execution 是 Step 的一次执行尝试（含重试）。左侧列表、右侧详情。"
      contextTitle={selectedExec ? `当前 Execution #${selectedExec.id}` : "当前 Execution：未选择"}
      contextMeta={selectedExec ? `status: ${selectedExec.status}` : "请选择一个 execution 查看详情"}
    >
      <Card className="rounded-2xl border-slate-200 shadow-none">
        <CardContent className="space-y-4 px-5 pb-5">
          {error ? <p className="text-sm text-red-600">{error}</p> : null}
          {loading ? <p className="text-sm text-slate-500">加载中...</p> : null}
          {!loading && executions.length === 0 ? (
            <p className="text-sm text-slate-500">暂无 Execution。</p>
          ) : null}

          <div className="grid gap-3 lg:grid-cols-[minmax(0,360px)_minmax(0,1fr)]">
            <div className="grid gap-2">
              {executions.map((exec) => (
                <button
                  key={exec.id}
                  type="button"
                  onClick={() => {
                    setSelectedExecId(exec.id);
                    onSelectExecution?.(exec.id);
                  }}
                  className={[
                    "flex w-full items-start justify-between gap-3 rounded-2xl border px-4 py-3 text-left transition",
                    exec.id === selectedExecId ? "border-indigo-200 bg-indigo-50/40" : "border-slate-200 bg-white hover:bg-slate-50",
                  ].join(" ")}
                >
                  <div>
                    <p className="text-sm font-semibold text-slate-900">#{exec.id}</p>
                    <p className="mt-1 text-[11px] text-slate-500">
                      attempt {exec.attempt} · {formatTime(exec.created_at)}
                    </p>
                  </div>
                  <Badge variant={statusTone(exec.status)}>{exec.status}</Badge>
                </button>
              ))}
            </div>

            <div className="rounded-2xl border border-slate-200 bg-white p-4">
              <p className="text-sm font-semibold text-slate-900">{selectedTitle}</p>
              {selectedExec ? (
                <div className="mt-3 grid gap-2 text-sm text-slate-700">
                  <p>
                    <span className="text-slate-500">状态：</span>
                    {selectedExec.status}
                  </p>
                  <p>
                    <span className="text-slate-500">开始：</span>
                    {formatTime(selectedExec.started_at)}
                  </p>
                  <p>
                    <span className="text-slate-500">结束：</span>
                    {formatTime(selectedExec.finished_at)}
                  </p>
                  <p>
                    <span className="text-slate-500">Agent：</span>
                    {selectedExec.agent_id || "-"}
                  </p>
                  {selectedExec.error_message ? (
                    <p className="rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700">
                      {selectedExec.error_kind ? `[${selectedExec.error_kind}] ` : null}
                      {selectedExec.error_message}
                    </p>
                  ) : null}
                </div>
              ) : (
                <p className="mt-2 text-sm text-slate-500">请选择一个 Execution 查看详情。</p>
              )}
            </div>
          </div>
        </CardContent>
      </Card>
    </PageScaffold>
  );
};

export default ExecutionsView;
