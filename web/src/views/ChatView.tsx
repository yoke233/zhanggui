import { useEffect, useMemo, useRef, useState } from "react";
import type { ApiClient } from "../lib/apiClient";
import type { ChatMessage } from "../types/workflow";
import FileTree from "../components/FileTree";
import GitStatusPanel from "../components/GitStatusPanel";

interface ChatViewProps {
  apiClient: ApiClient;
  projectId: string;
}

const roleLabel: Record<ChatMessage["role"], string> = {
  user: "用户",
  assistant: "助手",
};

const roleStyle: Record<ChatMessage["role"], string> = {
  user: "bg-slate-900 text-white",
  assistant: "border border-slate-200 bg-white text-slate-900",
};

const formatTime = (time: string): string => {
  const date = new Date(time);
  if (Number.isNaN(date.getTime())) {
    return time;
  }
  return date.toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
};

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error && error.message.trim().length > 0) {
    return error.message;
  }
  return "请求失败，请稍后重试";
};

const parseFilePathsDraft = (raw: string): string[] => {
  const unique: string[] = [];
  const seen = new Set<string>();
  raw
    .split(",")
    .map((item) => item.trim())
    .filter((item) => item.length > 0)
    .forEach((item) => {
      if (!seen.has(item)) {
        seen.add(item);
        unique.push(item);
      }
    });
  return unique;
};

const parseInlineMarkdown = (text: string, keyPrefix: string) => {
  const nodes: Array<string | JSX.Element> = [];
  const pattern = /`([^`]+)`|\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)|\*\*([^*]+)\*\*|(\*[^*]+\*)/g;
  let lastIndex = 0;
  let matchIndex = 0;
  let match = pattern.exec(text);
  while (match) {
    const startIndex = match.index;
    if (startIndex > lastIndex) {
      nodes.push(text.slice(lastIndex, startIndex));
    }

    if (match[1]) {
      nodes.push(
        <code
          key={`${keyPrefix}-inline-code-${matchIndex}`}
          className="rounded bg-slate-100 px-1 py-0.5 font-mono text-[0.9em] text-slate-900"
        >
          {match[1]}
        </code>,
      );
    } else if (match[2] && match[3]) {
      nodes.push(
        <a
          key={`${keyPrefix}-link-${matchIndex}`}
          href={match[3]}
          target="_blank"
          rel="noreferrer"
          className="text-sky-700 underline"
        >
          {match[2]}
        </a>,
      );
    } else if (match[4]) {
      nodes.push(
        <strong key={`${keyPrefix}-strong-${matchIndex}`} className="font-semibold">
          {match[4]}
        </strong>,
      );
    } else if (match[5]) {
      nodes.push(
        <em key={`${keyPrefix}-em-${matchIndex}`} className="italic">
          {match[5].slice(1, -1)}
        </em>,
      );
    }

    lastIndex = startIndex + match[0].length;
    matchIndex += 1;
    match = pattern.exec(text);
  }

  if (lastIndex < text.length) {
    nodes.push(text.slice(lastIndex));
  }
  if (nodes.length === 0) {
    nodes.push(text);
  }
  return nodes;
};

const renderBasicMarkdown = (content: string, keyPrefix: string): JSX.Element[] => {
  const lines = content.replace(/\r\n/g, "\n").split("\n");
  const elements: JSX.Element[] = [];
  let index = 0;
  while (index < lines.length) {
    const rawLine = lines[index] ?? "";
    const line = rawLine.trim();

    if (!line) {
      index += 1;
      continue;
    }

    if (line.startsWith("```")) {
      const codeLines: string[] = [];
      index += 1;
      while (index < lines.length && !(lines[index] ?? "").trim().startsWith("```")) {
        codeLines.push(lines[index] ?? "");
        index += 1;
      }
      index += 1;
      elements.push(
        <pre
          key={`${keyPrefix}-code-block-${index}`}
          className="overflow-x-auto rounded-md bg-slate-900 p-2 text-xs text-slate-100"
        >
          <code>{codeLines.join("\n")}</code>
        </pre>,
      );
      continue;
    }

    const headingMatch = line.match(/^(#{1,6})\s+(.+)$/);
    if (headingMatch) {
      const level = headingMatch[1].length;
      const headingText = headingMatch[2];
      const HeadingTag = `h${level}` as keyof JSX.IntrinsicElements;
      elements.push(
        <HeadingTag key={`${keyPrefix}-heading-${index}`} className="font-semibold leading-snug">
          {parseInlineMarkdown(headingText, `${keyPrefix}-heading-${index}`)}
        </HeadingTag>,
      );
      index += 1;
      continue;
    }

    if (/^[-*]\s+/.test(line)) {
      const items: string[] = [];
      while (index < lines.length) {
        const candidate = (lines[index] ?? "").trim();
        const itemMatch = candidate.match(/^[-*]\s+(.+)$/);
        if (!itemMatch) {
          break;
        }
        items.push(itemMatch[1]);
        index += 1;
      }
      elements.push(
        <ul key={`${keyPrefix}-list-${index}`} className="list-disc space-y-1 pl-5">
          {items.map((item, itemIndex) => (
            <li key={`${keyPrefix}-item-${index}-${itemIndex}`}>
              {parseInlineMarkdown(item, `${keyPrefix}-item-${index}-${itemIndex}`)}
            </li>
          ))}
        </ul>,
      );
      continue;
    }

    const paragraphLines = [line];
    index += 1;
    while (index < lines.length) {
      const nextLine = (lines[index] ?? "").trim();
      if (!nextLine || /^#{1,6}\s+/.test(nextLine) || /^[-*]\s+/.test(nextLine) || nextLine.startsWith("```")) {
        break;
      }
      paragraphLines.push(nextLine);
      index += 1;
    }
    const paragraph = paragraphLines.join(" ");
    elements.push(
      <p key={`${keyPrefix}-paragraph-${index}`} className="whitespace-pre-wrap">
        {parseInlineMarkdown(paragraph, `${keyPrefix}-paragraph-${index}`)}
      </p>,
    );
  }

  if (elements.length === 0) {
    elements.push(
      <p key={`${keyPrefix}-empty`} className="whitespace-pre-wrap">
        {content}
      </p>,
    );
  }
  return elements;
};

const ChatView = ({ apiClient, projectId }: ChatViewProps) => {
  const [draft, setDraft] = useState("");
  const [filePathsDraft, setFilePathsDraft] = useState("");
  const [selectedFiles, setSelectedFiles] = useState<string[]>([]);
  const [leftPanelTab, setLeftPanelTab] = useState<"tree" | "git">("tree");
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [chatLoading, setChatLoading] = useState(false);
  const [planLoading, setPlanLoading] = useState(false);
  const [planFromFilesLoading, setPlanFromFilesLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [planNotice, setPlanNotice] = useState<string | null>(null);
  const chatRequestIdRef = useRef(0);
  const planRequestIdRef = useRef(0);
  const planFromFilesRequestIdRef = useRef(0);

  useEffect(() => {
    chatRequestIdRef.current += 1;
    planRequestIdRef.current += 1;
    planFromFilesRequestIdRef.current += 1;
    setDraft("");
    setFilePathsDraft("");
    setSelectedFiles([]);
    setLeftPanelTab("tree");
    setSessionId(null);
    setMessages([]);
    setError(null);
    setPlanNotice(null);
    setChatLoading(false);
    setPlanLoading(false);
    setPlanFromFilesLoading(false);
  }, [projectId]);

  const hasMessages = messages.length > 0;
  const canSubmit = draft.trim().length > 0 && !chatLoading;
  const canCreatePlan = !!sessionId && !planLoading;
  const filePaths = useMemo(() => parseFilePathsDraft(filePathsDraft), [filePathsDraft]);
  const canCreatePlanFromFiles =
    !!sessionId && filePaths.length > 0 && !planFromFilesLoading;

  const sortedMessages = useMemo(
    () =>
      [...messages].sort((a, b) => {
        return new Date(a.time).getTime() - new Date(b.time).getTime();
      }),
    [messages],
  );

  const handleStartChat = async () => {
    const message = draft.trim();
    if (!message) {
      return;
    }

    setChatLoading(true);
    setError(null);
    setPlanNotice(null);
    const requestId = chatRequestIdRef.current + 1;
    chatRequestIdRef.current = requestId;
    const targetProjectId = projectId;

    try {
      const payload = targetSessionIdRef.current
        ? { message, session_id: targetSessionIdRef.current }
        : { message };
      const created = await apiClient.createChat(targetProjectId, payload);
      if (chatRequestIdRef.current !== requestId) {
        return;
      }
      const session = await apiClient.getChat(targetProjectId, created.session_id);
      if (chatRequestIdRef.current !== requestId) {
        return;
      }
      setSessionId(created.session_id);
      setMessages(session.messages);
      setDraft("");
    } catch (requestError) {
      if (chatRequestIdRef.current !== requestId) {
        return;
      }
      setError(getErrorMessage(requestError));
    } finally {
      if (chatRequestIdRef.current === requestId) {
        setChatLoading(false);
      }
    }
  };

  const handleCreatePlan = async () => {
    if (!sessionId) {
      return;
    }

    setPlanLoading(true);
    setError(null);
    setPlanNotice(null);
    const requestId = planRequestIdRef.current + 1;
    planRequestIdRef.current = requestId;
    const targetProjectId = projectId;
    const targetSessionId = sessionId;
    try {
      const createdPlan = await apiClient.createPlan(targetProjectId, {
        session_id: targetSessionId,
      });
      if (planRequestIdRef.current !== requestId) {
        return;
      }
      setPlanNotice(`已创建计划：${createdPlan.id}`);
    } catch (requestError) {
      if (planRequestIdRef.current !== requestId) {
        return;
      }
      setError(getErrorMessage(requestError));
    } finally {
      if (planRequestIdRef.current === requestId) {
        setPlanLoading(false);
      }
    }
  };

  const handleCreatePlanFromFiles = async () => {
    if (!sessionId || filePaths.length === 0) {
      return;
    }

    setPlanFromFilesLoading(true);
    setError(null);
    setPlanNotice(null);
    const requestId = planFromFilesRequestIdRef.current + 1;
    planFromFilesRequestIdRef.current = requestId;
    const targetProjectId = projectId;
    const targetSessionId = sessionId;
    try {
      const createdPlan = await apiClient.createPlanFromFiles(targetProjectId, {
        session_id: targetSessionId,
        file_paths: filePaths,
      });
      if (planFromFilesRequestIdRef.current !== requestId) {
        return;
      }
      setPlanNotice(`已从文件创建计划：${createdPlan.id}`);
    } catch (requestError) {
      if (planFromFilesRequestIdRef.current !== requestId) {
        return;
      }
      setError(getErrorMessage(requestError));
    } finally {
      if (planFromFilesRequestIdRef.current === requestId) {
        setPlanFromFilesLoading(false);
      }
    }
  };

  const handleToggleFile = (filePath: string, selected: boolean) => {
    const normalizedPath = filePath.trim();
    if (!normalizedPath) {
      return;
    }

    setSelectedFiles((prev) => {
      const exists = prev.includes(normalizedPath);
      let next = prev;
      if (selected && !exists) {
        next = [...prev, normalizedPath];
      }
      if (!selected && exists) {
        next = prev.filter((item) => item !== normalizedPath);
      }
      setFilePathsDraft(next.join(", "));
      return next;
    });
  };

  const targetSessionIdRef = useRef<string | null>(sessionId);
  useEffect(() => {
    targetSessionIdRef.current = sessionId;
  }, [sessionId]);

  return (
    <section className="grid gap-4 lg:grid-cols-[280px_minmax(0,2fr)_320px]">
      <aside className="hidden rounded-xl border border-slate-200 bg-white p-4 shadow-sm lg:flex lg:min-h-[680px] lg:flex-col">
        <h3 className="text-base font-semibold text-slate-900">仓库视图</h3>
        <p className="mt-1 text-xs text-slate-600">
          在文件树中选择文件后，会自动同步到右侧“文件路径”输入框。
        </p>
        <div className="mt-3 grid grid-cols-2 rounded-md bg-slate-100 p-1 text-xs">
          <button
            type="button"
            className={`rounded px-2 py-1 font-medium ${
              leftPanelTab === "tree"
                ? "bg-white text-slate-900 shadow-sm"
                : "text-slate-600 hover:text-slate-900"
            }`}
            onClick={() => {
              setLeftPanelTab("tree");
            }}
          >
            文件树
          </button>
          <button
            type="button"
            className={`rounded px-2 py-1 font-medium ${
              leftPanelTab === "git"
                ? "bg-white text-slate-900 shadow-sm"
                : "text-slate-600 hover:text-slate-900"
            }`}
            onClick={() => {
              setLeftPanelTab("git");
            }}
          >
            Git Status
          </button>
        </div>
        <div className="mt-3 min-h-0 flex-1 overflow-y-auto">
          {leftPanelTab === "tree" ? (
            <FileTree
              apiClient={apiClient}
              projectId={projectId}
              selectedFiles={selectedFiles}
              onToggleFile={handleToggleFile}
            />
          ) : (
            <GitStatusPanel apiClient={apiClient} projectId={projectId} />
          )}
        </div>
      </aside>

      <div className="min-w-0 rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
        <h2 className="text-xl font-bold">Chat</h2>
        <p className="mt-1 text-sm text-slate-600">
          发送消息后调用 POST /chat 创建会话，再调用 GET /chat/:sid 获取完整历史。
        </p>

        <div className="mt-4 min-h-72 rounded-lg border border-slate-200 bg-slate-50 p-3">
          {hasMessages ? (
            <div className="flex flex-col gap-3">
              {sortedMessages.map((message, index) => (
                <article
                  key={`${message.time}-${index}`}
                  className={`max-w-[92%] rounded-lg px-3 py-2 text-sm ${
                    roleStyle[message.role]
                  } ${message.role === "user" ? "self-end" : "self-start"}`}
                >
                  <p className="mb-1 text-xs font-semibold opacity-80">
                    {roleLabel[message.role]} · {formatTime(message.time)}
                  </p>
                  <div className="space-y-2">
                    {renderBasicMarkdown(message.content, `${message.time}-${index}`)}
                  </div>
                </article>
              ))}
            </div>
          ) : (
            <p className="text-sm text-slate-500">当前会话暂无消息。</p>
          )}
        </div>

        <div className="mt-4">
          <label htmlFor="chat-message" className="mb-2 block text-sm font-medium">
            新消息
          </label>
          <textarea
            id="chat-message"
            rows={4}
            className="w-full resize-y rounded-lg border border-slate-300 px-3 py-2 text-sm"
            placeholder="请输入要拆分为计划的需求..."
            value={draft}
            onChange={(event) => {
              setDraft(event.target.value);
            }}
          />
          <div className="mt-3 flex justify-end">
            <button
              type="button"
              className="rounded-md bg-slate-900 px-4 py-2 text-sm font-semibold text-white disabled:cursor-not-allowed disabled:bg-slate-400"
              disabled={!canSubmit}
              onClick={() => {
                void handleStartChat();
              }}
            >
              {chatLoading ? "处理中..." : "发送并创建会话"}
            </button>
          </div>
        </div>
      </div>

      <aside className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
        <h3 className="text-lg font-semibold">会话与计划</h3>
        <p className="mt-2 break-all text-xs text-slate-600">
          Session ID: {sessionId ?? "未创建"}
        </p>
        <button
          type="button"
          className="mt-3 w-full rounded-md border border-slate-900 px-3 py-2 text-sm font-semibold text-slate-900 disabled:cursor-not-allowed disabled:border-slate-300 disabled:text-slate-400"
          disabled={!canCreatePlan}
          onClick={() => {
            void handleCreatePlan();
          }}
        >
          {planLoading ? "创建计划中..." : "基于当前会话创建计划"}
        </button>

        <label className="mt-3 block text-xs text-slate-700" htmlFor="plan-file-paths">
          文件路径（逗号分隔）
          <input
            id="plan-file-paths"
            className="mt-1 w-full rounded-md border border-slate-300 px-2 py-1 text-sm"
            placeholder="例如：cmd/app/main.go, internal/core/task.go"
            value={filePathsDraft}
            onChange={(event) => {
              const nextValue = event.target.value;
              setFilePathsDraft(nextValue);
              setSelectedFiles(parseFilePathsDraft(nextValue));
            }}
          />
        </label>
        <button
          type="button"
          className="mt-2 w-full rounded-md border border-sky-700 px-3 py-2 text-sm font-semibold text-sky-700 disabled:cursor-not-allowed disabled:border-slate-300 disabled:text-slate-400"
          disabled={!canCreatePlanFromFiles}
          onClick={() => {
            void handleCreatePlanFromFiles();
          }}
        >
          {planFromFilesLoading ? "从文件创建中..." : "从文件创建计划"}
        </button>

        {planNotice ? (
          <p className="mt-3 rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">
            {planNotice}
          </p>
        ) : null}
        {error ? (
          <p className="mt-3 rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">
            {error}
          </p>
        ) : null}
      </aside>
    </section>
  );
};

export default ChatView;
