import { forwardRef } from "react";
import { TuiMarkdown } from "./TuiMarkdown";

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

export const TuiMessage = forwardRef<HTMLDivElement, TuiMessageProps>(
  function TuiMessage({ role, content, time, id }, ref) {
    if (role === "user") {
      return (
        <div ref={ref} id={id} className="border-b border-slate-200 bg-slate-50 px-4 py-3">
          <div className="flex items-start gap-2">
            <span className="mt-0.5 select-none text-base" aria-hidden>👤</span>
            <div className="min-w-0 flex-1">
              <span className="text-xs text-slate-400">{formatTime(time)}</span>
              <p className="mt-1 text-sm font-medium whitespace-pre-wrap">{content}</p>
            </div>
          </div>
        </div>
      );
    }

    return (
      <div ref={ref} id={id} className="border-b border-slate-200 px-4 py-3">
        <div className="flex items-start gap-2">
          <span className="mt-0.5 select-none text-sm font-bold text-slate-400" aria-hidden>•</span>
          <div className="min-w-0 flex-1 text-sm">
            <span className="text-xs text-slate-400">{formatTime(time)}</span>
            <div className="mt-1 space-y-2">
              <TuiMarkdown content={content} />
            </div>
          </div>
        </div>
      </div>
    );
  },
);
