import { useEffect, useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import type { ApiClientV2 } from "@/lib/apiClientV2";
import type { Briefing, ContextRef } from "@/types/apiV2";
import { PageScaffold } from "@/v3/components/PageScaffold";

interface BriefingViewProps {
  apiClient: ApiClientV2;
  stepId: number | null;
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

const normalizeContextRefs = (refs?: ContextRef[]) => (Array.isArray(refs) ? refs : []);

const BriefingView = ({ apiClient, stepId }: BriefingViewProps) => {
  const [briefingId, setBriefingId] = useState("");
  const [briefing, setBriefing] = useState<Briefing | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setBriefing(null);
    setError(null);
  }, [stepId]);

  const loadById = async () => {
    const parsed = Number.parseInt(briefingId.trim(), 10);
    if (!Number.isFinite(parsed) || parsed <= 0) {
      setError("briefing_id 无效。");
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const loaded = await apiClient.getBriefing(parsed);
      setBriefing(loaded);
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  const loadByStep = async () => {
    if (stepId == null) {
      setError("请先选择 Step。");
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const loaded = await apiClient.getBriefingByStep(stepId);
      setBriefing(loaded);
      setBriefingId(String(loaded.id));
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  const rendered = useMemo(() => {
    if (!briefing) {
      return null;
    }
    return {
      refs: normalizeContextRefs(briefing.context_refs),
      constraints: Array.isArray(briefing.constraints) ? briefing.constraints : [],
    };
  }, [briefing]);

  return (
    <PageScaffold
      eyebrow="Briefing / Task Sheet"
      title="Briefing 查看器"
      description="查看 by-id / by-step 的 briefing，包含 objective、context_refs 与 constraints。"
      contextTitle={stepId != null ? `step ${stepId}` : "未指定 step"}
      contextMeta={briefing ? `briefing #${briefing.id}` : "尚未加载 briefing"}
    >
      <Card className="rounded-2xl border-slate-200 shadow-none">
        <CardContent className="space-y-4 px-5 pb-5">
          <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-end">
            <div className="grid gap-1">
              <label htmlFor="v2-briefing-id" className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
                briefing_id
              </label>
              <Input
                id="v2-briefing-id"
                value={briefingId}
                onChange={(event) => setBriefingId(event.target.value)}
                placeholder="例如：55"
              />
            </div>
            <div className="flex flex-wrap gap-2">
              <Button variant="outline" onClick={() => void loadById()} disabled={loading}>
                按 ID 加载
              </Button>
              <Button variant="outline" onClick={() => void loadByStep()} disabled={loading}>
                Step Briefing
              </Button>
            </div>
          </div>

          {error ? (
            <p className="rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700">
              {error}
            </p>
          ) : null}

          {briefing ? (
            <div className="grid gap-3">
              <div className="rounded-2xl border border-slate-200 bg-white p-4">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <p className="text-sm font-semibold text-slate-900">Briefing #{briefing.id}</p>
                    <p className="mt-1 text-[11px] text-slate-500">
                      step {briefing.step_id} · {formatTime(briefing.created_at)}
                    </p>
                  </div>
                </div>
                <p className="mt-3 text-sm font-semibold text-slate-900">objective</p>
                <pre className="mt-2 whitespace-pre-wrap text-sm leading-6 text-slate-900">
                  {briefing.objective}
                </pre>
              </div>

              <div className="grid gap-3 lg:grid-cols-2">
                <div className="rounded-2xl border border-slate-200 bg-white p-4">
                  <p className="text-sm font-semibold text-slate-900">constraints</p>
                  <div className="mt-2 grid gap-2">
                    {rendered?.constraints.length ? (
                      rendered.constraints.map((constraint, index) => (
                        <div key={`${constraint}-${index}`} className="rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 text-sm text-slate-800">
                          {constraint}
                        </div>
                      ))
                    ) : (
                      <p className="text-sm text-slate-500">无 constraints</p>
                    )}
                  </div>
                </div>

                <div className="rounded-2xl border border-slate-200 bg-white p-4">
                  <p className="text-sm font-semibold text-slate-900">context_refs</p>
                  <div className="mt-2 grid gap-2">
                    {rendered?.refs.length ? (
                      rendered.refs.map((ref, index) => (
                        <div key={`${ref.type}-${ref.ref_id}-${index}`} className="rounded-xl border border-slate-200 bg-slate-50 px-3 py-2">
                          <div className="flex flex-wrap items-center justify-between gap-2">
                            <Badge variant="outline" className="bg-white text-slate-700">
                              {ref.type}
                            </Badge>
                            <Badge variant="outline" className="bg-white text-slate-700">
                              ref {ref.ref_id}
                            </Badge>
                          </div>
                          {ref.label ? (
                            <p className="mt-2 text-sm font-semibold text-slate-800">
                              {ref.label}
                            </p>
                          ) : null}
                          {ref.inline ? (
                            <pre className="mt-2 max-h-[180px] overflow-auto whitespace-pre-wrap rounded-xl bg-slate-950 px-3 py-2 text-xs text-slate-100">
                              {ref.inline}
                            </pre>
                          ) : (
                            <p className="mt-2 text-sm text-slate-500">（无 inline 内容）</p>
                          )}
                        </div>
                      ))
                    ) : (
                      <p className="text-sm text-slate-500">无 context_refs</p>
                    )}
                  </div>
                </div>
              </div>
            </div>
          ) : null}
        </CardContent>
      </Card>
    </PageScaffold>
  );
};

export default BriefingView;
