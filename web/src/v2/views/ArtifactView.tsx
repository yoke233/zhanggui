import { useEffect, useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import type { ApiClientV2 } from "@/lib/apiClientV2";
import type { Artifact } from "@/types/apiV2";
import { PageScaffold } from "@/v3/components/PageScaffold";

interface ArtifactViewProps {
  apiClient: ApiClientV2;
  stepId: number | null;
  execId: number | null;
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

const ArtifactView = ({ apiClient, stepId, execId }: ArtifactViewProps) => {
  const [artifactId, setArtifactId] = useState("");
  const [artifact, setArtifact] = useState<Artifact | null>(null);
  const [artifactsByExec, setArtifactsByExec] = useState<Artifact[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const effectiveStepId = stepId ?? null;
  const effectiveExecId = execId ?? null;

  useEffect(() => {
    setArtifact(null);
    setArtifactsByExec([]);
    setError(null);
  }, [effectiveStepId, effectiveExecId]);

  const loadById = async () => {
    const parsed = Number.parseInt(artifactId.trim(), 10);
    if (!Number.isFinite(parsed) || parsed <= 0) {
      setError("artifact_id 无效。");
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const loaded = await apiClient.getArtifact(parsed);
      setArtifact(loaded);
      setArtifactsByExec([]);
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  const loadLatestForStep = async () => {
    if (effectiveStepId == null) {
      setError("请先选择 Step。");
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const loaded = await apiClient.getLatestArtifact(effectiveStepId);
      setArtifact(loaded);
      setArtifactsByExec([]);
      setArtifactId(String(loaded.id));
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  const loadByExecution = async () => {
    if (effectiveExecId == null) {
      setError("请先选择 Execution（在 Executions 页选择一个）。");
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const loaded = await apiClient.listArtifactsByExecution(effectiveExecId);
      setArtifactsByExec(loaded);
      setArtifact(null);
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  const renderedArtifact = useMemo(() => {
    if (!artifact) {
      return null;
    }
    const assets = Array.isArray(artifact.assets) ? artifact.assets : [];
    const metadata = artifact.metadata ?? {};
    return { assets, metadata };
  }, [artifact]);

  return (
    <PageScaffold
      eyebrow="Artifact / Deliverables"
      title="Artifact 查看器"
      description="查看 latest/by-id/by-execution 的交付物，并浏览 assets / metadata。"
      contextTitle={effectiveStepId != null ? `step ${effectiveStepId}` : effectiveExecId != null ? `exec ${effectiveExecId}` : "未指定上下文"}
      contextMeta={artifact ? `artifact #${artifact.id}` : "尚未加载 artifact"}
    >
      <Card className="rounded-2xl border-slate-200 shadow-none">
        <CardContent className="space-y-4 px-5 pb-5">
          <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-end">
            <div className="grid gap-1">
              <label htmlFor="v2-artifact-id" className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-400">
                artifact_id
              </label>
              <Input
                id="v2-artifact-id"
                value={artifactId}
                onChange={(event) => setArtifactId(event.target.value)}
                placeholder="例如：101"
              />
            </div>
            <div className="flex flex-wrap gap-2">
              <Button variant="outline" onClick={() => void loadById()} disabled={loading}>
                按 ID 加载
              </Button>
              <Button variant="outline" onClick={() => void loadLatestForStep()} disabled={loading}>
                Step Latest
              </Button>
              <Button variant="outline" onClick={() => void loadByExecution()} disabled={loading}>
                Exec Artifacts
              </Button>
            </div>
          </div>

          {error ? (
            <p className="rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700">
              {error}
            </p>
          ) : null}

          {artifact ? (
            <div className="grid gap-3">
              <div className="rounded-2xl border border-slate-200 bg-white p-4">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <p className="text-sm font-semibold text-slate-900">
                      Artifact #{artifact.id}
                    </p>
                    <p className="mt-1 text-[11px] text-slate-500">
                      exec {artifact.execution_id} · step {artifact.step_id} · flow {artifact.flow_id} · {formatTime(artifact.created_at)}
                    </p>
                  </div>
                </div>
                <pre className="mt-3 whitespace-pre-wrap text-sm leading-6 text-slate-900">
                  {artifact.result_markdown}
                </pre>
              </div>

              <div className="grid gap-3 lg:grid-cols-2">
                <div className="rounded-2xl border border-slate-200 bg-white p-4">
                  <p className="text-sm font-semibold text-slate-900">metadata</p>
                  <pre className="mt-2 overflow-auto rounded-xl bg-slate-950 px-3 py-2 text-xs text-slate-100">
                    {JSON.stringify(renderedArtifact?.metadata ?? {}, null, 2)}
                  </pre>
                </div>
                <div className="rounded-2xl border border-slate-200 bg-white p-4">
                  <p className="text-sm font-semibold text-slate-900">assets</p>
                  <div className="mt-2 grid gap-2">
                    {(renderedArtifact?.assets ?? []).length === 0 ? (
                      <p className="text-sm text-slate-500">无 assets</p>
                    ) : (
                      (renderedArtifact?.assets ?? []).map((asset, index) => (
                        <div key={`${asset.uri}-${index}`} className="rounded-xl border border-slate-200 bg-slate-50 px-3 py-2">
                          <p className="text-sm font-semibold text-slate-800">{asset.name || `asset-${index + 1}`}</p>
                          <p className="mt-1 text-[11px] text-slate-600">{asset.media_type || "-"}</p>
                          <p className="mt-1 break-all text-[11px] text-slate-600">{asset.uri}</p>
                        </div>
                      ))
                    )}
                  </div>
                </div>
              </div>
            </div>
          ) : null}

          {artifactsByExec.length > 0 ? (
            <div className="grid gap-2">
              <p className="text-sm font-semibold text-slate-900">Execution Artifacts</p>
              {artifactsByExec.map((item) => (
                <button
                  key={item.id}
                  type="button"
                  onClick={() => {
                    setArtifact(item);
                    setArtifactId(String(item.id));
                    setArtifactsByExec([]);
                  }}
                  className="flex items-start justify-between gap-3 rounded-2xl border border-slate-200 bg-white px-4 py-3 text-left hover:bg-slate-50"
                >
                  <div>
                    <p className="text-sm font-semibold text-slate-900">#{item.id}</p>
                    <p className="mt-1 text-[11px] text-slate-500">{formatTime(item.created_at)}</p>
                  </div>
                  <Badge variant="outline" className="bg-slate-50 text-slate-600">
                    step {item.step_id}
                  </Badge>
                </button>
              ))}
            </div>
          ) : null}
        </CardContent>
      </Card>
    </PageScaffold>
  );
};

export default ArtifactView;
