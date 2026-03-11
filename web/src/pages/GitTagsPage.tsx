import { useCallback, useEffect, useState } from "react";
import { useParams, Link } from "react-router-dom";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { GitCommitEntry, GitTagEntry } from "@/types/apiV2";
import { Tag, GitCommit, Upload, RefreshCw, Check, AlertCircle, ArrowLeft } from "lucide-react";

type TabType = "commits" | "tags";

export function GitTagsPage() {
  const { apiClient } = useWorkbench();
  const { projectId: projectIdParam } = useParams<{ projectId: string }>();
  const selectedProjectId = projectIdParam ? Number(projectIdParam) : null;

  const [activeTab, setActiveTab] = useState<TabType>("commits");
  const [commits, setCommits] = useState<GitCommitEntry[]>([]);
  const [tags, setTags] = useState<GitTagEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Create tag form
  const [tagName, setTagName] = useState("");
  const [tagRef, setTagRef] = useState("");
  const [tagMessage, setTagMessage] = useState("");
  const [pushAfterCreate, setPushAfterCreate] = useState(true);
  const [creating, setCreating] = useState(false);
  const [createResult, setCreateResult] = useState<string | null>(null);

  // Push state
  const [pushing, setPushing] = useState<string | null>(null);

  const loadCommits = useCallback(async () => {
    if (!selectedProjectId) return;
    setLoading(true);
    setError(null);
    try {
      const data = await apiClient.listGitCommits(selectedProjectId, { limit: 50 });
      setCommits(data);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [apiClient, selectedProjectId]);

  const loadTags = useCallback(async () => {
    if (!selectedProjectId) return;
    setLoading(true);
    setError(null);
    try {
      const data = await apiClient.listGitTags(selectedProjectId);
      setTags(data);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [apiClient, selectedProjectId]);

  useEffect(() => {
    if (activeTab === "commits") {
      void loadCommits();
    } else {
      void loadTags();
    }
  }, [activeTab, loadCommits, loadTags]);

  const handleCreateTag = async () => {
    if (!selectedProjectId || !tagName.trim()) return;
    setCreating(true);
    setCreateResult(null);
    try {
      const res = await apiClient.createGitTag(selectedProjectId, {
        name: tagName.trim(),
        ref: tagRef.trim() || undefined,
        message: tagMessage.trim() || undefined,
        push: pushAfterCreate,
      });
      if (res.push_error) {
        setCreateResult(`Tag ${res.name} 已创建 (${res.sha.slice(0, 7)})，但推送失败: ${res.push_error}`);
      } else if (res.pushed) {
        setCreateResult(`Tag ${res.name} 已创建并推送成功 (${res.sha.slice(0, 7)})`);
      } else {
        setCreateResult(`Tag ${res.name} 已创建 (${res.sha.slice(0, 7)})，尚未推送`);
      }
      setTagName("");
      setTagRef("");
      setTagMessage("");
      void loadTags();
    } catch (e) {
      setCreateResult(`创建失败: ${e instanceof Error ? e.message : String(e)}`);
    } finally {
      setCreating(false);
    }
  };

  const handlePushTag = async (name: string) => {
    if (!selectedProjectId) return;
    setPushing(name);
    try {
      await apiClient.pushGitTag(selectedProjectId, { name });
      setCreateResult(`Tag ${name} 推送成功`);
      void loadTags();
    } catch (e) {
      setCreateResult(`推送失败: ${e instanceof Error ? e.message : String(e)}`);
    } finally {
      setPushing(null);
    }
  };

  const formatTime = (ts: string) => {
    try {
      return new Date(ts).toLocaleString("zh-CN", {
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
      });
    } catch {
      return ts;
    }
  };

  if (!selectedProjectId) {
    return (
      <div className="px-6 py-8">
        <p className="text-sm text-muted-foreground">请先选择一个项目</p>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6 px-6 py-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <Link
            to="/projects"
            className="mb-2 inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
          >
            <ArrowLeft className="h-3 w-3" />
            返回项目
          </Link>
          <h1 className="text-2xl font-semibold tracking-tight">版本标签</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            查看提交记录、创建 Git Tag 并推送到远程触发 CI/CD
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => (activeTab === "commits" ? loadCommits() : loadTags())}
          disabled={loading}
        >
          <RefreshCw className={`mr-1.5 h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
          刷新
        </Button>
      </div>

      {/* Tabs */}
      <div className="flex gap-1 rounded-lg border bg-muted p-1">
        <button
          onClick={() => setActiveTab("commits")}
          className={`flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
            activeTab === "commits"
              ? "bg-background text-foreground shadow-sm"
              : "text-muted-foreground hover:text-foreground"
          }`}
        >
          <GitCommit className="h-3.5 w-3.5" />
          提交记录
        </button>
        <button
          onClick={() => setActiveTab("tags")}
          className={`flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
            activeTab === "tags"
              ? "bg-background text-foreground shadow-sm"
              : "text-muted-foreground hover:text-foreground"
          }`}
        >
          <Tag className="h-3.5 w-3.5" />
          标签列表
        </button>
      </div>

      {/* Error */}
      {error ? (
        <div className="flex items-center gap-2 rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
          <AlertCircle className="h-4 w-4 shrink-0" />
          {error}
        </div>
      ) : null}

      {/* Result message */}
      {createResult ? (
        <div className={`flex items-center gap-2 rounded-lg border px-4 py-3 text-sm ${
          createResult.includes("失败")
            ? "border-rose-200 bg-rose-50 text-rose-700"
            : "border-emerald-200 bg-emerald-50 text-emerald-700"
        }`}>
          {createResult.includes("失败") ? (
            <AlertCircle className="h-4 w-4 shrink-0" />
          ) : (
            <Check className="h-4 w-4 shrink-0" />
          )}
          {createResult}
        </div>
      ) : null}

      {/* Create Tag Form */}
      <div className="rounded-xl border bg-card p-5">
        <h2 className="mb-4 text-base font-medium">创建新标签</h2>
        <div className="flex flex-col gap-3">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <div>
              <label className="mb-1 block text-xs font-medium text-muted-foreground">
                标签名称 *
              </label>
              <Input
                placeholder="例如: v1.0.0"
                value={tagName}
                onChange={(e) => setTagName(e.target.value)}
              />
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-muted-foreground">
                目标 Commit (留空 = HEAD)
              </label>
              <Input
                placeholder="SHA 或分支名"
                value={tagRef}
                onChange={(e) => setTagRef(e.target.value)}
              />
            </div>
            <div className="flex items-end gap-2">
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={pushAfterCreate}
                  onChange={(e) => setPushAfterCreate(e.target.checked)}
                  className="rounded"
                />
                创建后自动推送
              </label>
            </div>
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-muted-foreground">
              标签描述 (可选，填写后创建 annotated tag)
            </label>
            <Textarea
              placeholder="版本说明..."
              value={tagMessage}
              onChange={(e) => setTagMessage(e.target.value)}
              rows={2}
            />
          </div>
          <div>
            <Button
              onClick={handleCreateTag}
              disabled={creating || !tagName.trim()}
              size="sm"
            >
              <Tag className="mr-1.5 h-3.5 w-3.5" />
              {creating ? "创建中..." : "创建标签"}
            </Button>
          </div>
        </div>
      </div>

      {/* Commits Table */}
      {activeTab === "commits" ? (
        <div className="rounded-xl border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-24">SHA</TableHead>
                <TableHead>提交信息</TableHead>
                <TableHead className="w-32">作者</TableHead>
                <TableHead className="w-40">时间</TableHead>
                <TableHead className="w-24">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading && commits.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className="py-8 text-center text-muted-foreground">
                    加载中...
                  </TableCell>
                </TableRow>
              ) : commits.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className="py-8 text-center text-muted-foreground">
                    暂无提交记录
                  </TableCell>
                </TableRow>
              ) : (
                commits.map((c) => (
                  <TableRow key={c.sha}>
                    <TableCell>
                      <Badge variant="secondary" className="font-mono text-xs">
                        {c.short}
                      </Badge>
                    </TableCell>
                    <TableCell className="max-w-md truncate text-sm">{c.message}</TableCell>
                    <TableCell className="text-sm text-muted-foreground">{c.author}</TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {formatTime(c.timestamp)}
                    </TableCell>
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-7 text-xs"
                        onClick={() => {
                          setTagRef(c.sha);
                          setActiveTab("commits");
                          window.scrollTo({ top: 0, behavior: "smooth" });
                        }}
                      >
                        <Tag className="mr-1 h-3 w-3" />
                        打标签
                      </Button>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
      ) : null}

      {/* Tags Table */}
      {activeTab === "tags" ? (
        <div className="rounded-xl border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-40">标签名</TableHead>
                <TableHead className="w-24">SHA</TableHead>
                <TableHead>描述</TableHead>
                <TableHead className="w-40">创建时间</TableHead>
                <TableHead className="w-24">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading && tags.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className="py-8 text-center text-muted-foreground">
                    加载中...
                  </TableCell>
                </TableRow>
              ) : tags.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className="py-8 text-center text-muted-foreground">
                    暂无标签
                  </TableCell>
                </TableRow>
              ) : (
                tags.map((t) => (
                  <TableRow key={t.name}>
                    <TableCell>
                      <div className="flex items-center gap-1.5">
                        <Tag className="h-3.5 w-3.5 text-indigo-500" />
                        <span className="font-medium text-sm">{t.name}</span>
                      </div>
                    </TableCell>
                    <TableCell>
                      <Badge variant="secondary" className="font-mono text-xs">
                        {t.sha.slice(0, 7)}
                      </Badge>
                    </TableCell>
                    <TableCell className="max-w-md truncate text-sm text-muted-foreground">
                      {t.message || "-"}
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {t.timestamp ? formatTime(t.timestamp) : "-"}
                    </TableCell>
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-7 text-xs"
                        disabled={pushing === t.name}
                        onClick={() => handlePushTag(t.name)}
                      >
                        <Upload className="mr-1 h-3 w-3" />
                        {pushing === t.name ? "推送中..." : "推送"}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
      ) : null}
    </div>
  );
}
