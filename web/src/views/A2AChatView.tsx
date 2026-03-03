import { useMemo, useState } from "react";
import type { A2AClient } from "../lib/a2aClient";
import type { A2ATask } from "../types/a2a";

interface A2AChatViewProps {
  a2aClient: A2AClient;
  projectId: string;
}

interface LocalMessage {
  id: string;
  role: "user";
  content: string;
}

const isTerminalState = (state: string): boolean => {
  const normalized = state.trim().toLowerCase();
  return normalized === "completed" || normalized === "failed" || normalized === "canceled";
};

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error && error.message.trim().length > 0) {
    return error.message;
  }
  return "请求失败，请稍后重试";
};

const toSafeTaskState = (task: A2ATask): string => {
  const state = task.status?.state;
  if (typeof state !== "string") {
    return "unknown";
  }
  const trimmed = state.trim();
  return trimmed.length > 0 ? trimmed : "unknown";
};

const A2AChatView = ({ a2aClient, projectId }: A2AChatViewProps) => {
  const [draft, setDraft] = useState("");
  const [messages, setMessages] = useState<LocalMessage[]>([]);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [taskId, setTaskId] = useState<string | null>(null);
  const [taskState, setTaskState] = useState<string>("unknown");
  const [loading, setLoading] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submitLabel = loading ? "停止" : sessionId ? "发送" : "发送并创建会话";
  const canSubmit = loading ? !cancelling && !!taskId : draft.trim().length > 0;

  const sortedMessages = useMemo(() => messages, [messages]);

  const handleSend = async () => {
    if (loading) {
      return;
    }
    const message = draft.trim();
    if (!message) {
      return;
    }

    setError(null);
    setLoading(true);
    setCancelling(false);
    setDraft("");
    setMessages((prev) => [
      ...prev,
      {
        id: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
        role: "user",
        content: message,
      },
    ]);

    try {
      const task = await a2aClient.sendMessage({
        message: {
          role: "user",
          parts: [{ kind: "text", text: message }],
          ...(sessionId ? { contextId: sessionId } : {}),
        },
        metadata: {
          project_id: projectId,
        },
      });

      const nextTaskId = typeof task.id === "string" ? task.id.trim() : "";
      const nextSessionId = typeof task.contextId === "string" ? task.contextId.trim() : "";
      const nextState = toSafeTaskState(task);

      setTaskId(nextTaskId || null);
      setTaskState(nextState);
      if (nextSessionId) {
        setSessionId(nextSessionId);
      }
      setLoading(!isTerminalState(nextState));
      setCancelling(false);
    } catch (requestError) {
      setLoading(false);
      setCancelling(false);
      setError(getErrorMessage(requestError));
    }
  };

  const handleCancel = async () => {
    if (!loading || cancelling || !taskId) {
      return;
    }

    setCancelling(true);
    setError(null);

    try {
      const task = await a2aClient.cancelTask({
        id: taskId,
        metadata: {
          project_id: projectId,
        },
      });
      const nextState = toSafeTaskState(task);
      setTaskState(nextState);
      setLoading(!isTerminalState(nextState));
      setCancelling(false);
      if (nextState === "canceled") {
        setError("当前请求已取消");
      }
    } catch (requestError) {
      setCancelling(false);
      setError(getErrorMessage(requestError));
    }
  };

  return (
    <section className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
      <h2 className="text-xl font-bold">A2A Chat</h2>
      <p className="mt-1 text-sm text-slate-600">通过 A2A JSON-RPC 直接发送与取消任务。</p>

      <div className="mt-3 rounded border border-sky-200 bg-sky-50 px-3 py-2 text-xs text-sky-800">
        <p className="break-all">Session ID: {sessionId ?? "未创建"}</p>
        <p className="mt-1 break-all">A2A Task ID: {taskId ?? "未创建"}</p>
        <p className="mt-1 break-all">A2A Task State: {taskState}</p>
      </div>

      <div className="mt-4 rounded-lg border border-slate-200 bg-slate-50 p-3">
        {sortedMessages.length > 0 ? (
          <ul className="space-y-2">
            {sortedMessages.map((message) => (
              <li key={message.id} className="rounded border border-slate-200 bg-white px-3 py-2 text-sm">
                <p className="mb-1 text-xs font-semibold text-slate-500">用户</p>
                <p className="whitespace-pre-wrap break-words text-slate-900">{message.content}</p>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-slate-500">当前会话暂无消息。</p>
        )}
      </div>

      <div className="mt-4">
        <label htmlFor="a2a-chat-message" className="mb-2 block text-sm font-medium">
          新消息
        </label>
        <textarea
          id="a2a-chat-message"
          rows={4}
          className="min-h-[7rem] w-full resize-y rounded-lg border border-slate-300 px-3 py-2 text-sm"
          placeholder="请输入要发送给 A2A agent 的内容..."
          value={draft}
          onChange={(event) => {
            setDraft(event.target.value);
          }}
        />
        <div className="mt-3 flex justify-end">
          <button
            type="button"
            className="w-36 rounded-md bg-slate-900 px-4 py-2 text-center text-sm font-semibold text-white disabled:cursor-not-allowed disabled:bg-slate-400"
            disabled={!canSubmit}
            onClick={() => {
              if (loading) {
                void handleCancel();
                return;
              }
              void handleSend();
            }}
          >
            {submitLabel}
          </button>
        </div>
      </div>

      {error ? (
        <p className="mt-3 rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">
          {error}
        </p>
      ) : null}
    </section>
  );
};

export default A2AChatView;
