import { forwardRef, useState, useCallback } from "react";
import { TuiMarkdown } from "@/archive/legacy/components/TuiMarkdown";

interface TuiMessageProps {
  role: "user" | "assistant";
  content: string;
  time: string;
  id?: string;
}

const formatTime = (time: string): string => {
  const date = new Date(time);
  if (Number.isNaN(date.getTime())) return time;
  return date.toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
};

function CopyButton({ content }: { content: string }) {
  const [copied, setCopied] = useState(false);
  const handleCopy = useCallback(() => {
    void navigator.clipboard.writeText(content).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  }, [content]);
  return (
    <button
      type="button"
      onClick={handleCopy}
      className="rounded px-1.5 py-0.5 text-[10px] text-slate-400 hover:text-slate-600 transition-colors"
      aria-label="复制消息"
      title="复制"
    >
      {copied ? "✓" : "⎘"}
    </button>
  );
}

export const TuiMessage = forwardRef<HTMLDivElement, TuiMessageProps>(
  function TuiMessage({ role, content, time, id }, ref) {
    if (role === "user") {
      return (
        <div ref={ref} id={id} className="border-b border-slate-200 px-4 py-3">
          <div className="flex items-start gap-2">
            <span className="mt-0.5 select-none text-base" aria-hidden>🤣</span>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-1">
                <span className="text-xs text-slate-400">{formatTime(time)}</span>
                <span className="ml-auto"><CopyButton content={content} /></span>
              </div>
              <p className="mt-1 font-medium whitespace-pre-wrap" style={{ fontSize: "var(--font-chat)" }}>{content}</p>
            </div>
          </div>
        </div>
      );
    }

    return (
      <div ref={ref} id={id} className="border-b border-slate-200 px-4 py-3">
        <div className="flex items-start gap-2">
          <span className="mt-0.5 select-none text-base" aria-hidden>🤖</span>
          <div className="min-w-0 flex-1" style={{ fontSize: "var(--font-chat)" }}>
            <div className="flex items-center gap-1">
              <span className="text-xs text-slate-400">{formatTime(time)}</span>
              <span className="ml-auto"><CopyButton content={content} /></span>
            </div>
            <div className="mt-1 space-y-2">
              <TuiMarkdown content={content} />
            </div>
          </div>
        </div>
      </div>
    );
  },
);
