import { useState, useCallback } from "react";
import { TuiMarkdown } from "./TuiMarkdown";

interface TuiActivityBlockProps {
  activityType: string;
  detail: string;
  time: string;
  groupId?: string;
  onExpandGroup?: (groupId: string) => void;
  groupChildren?: Array<{ id: string; type: string; detail: string; time: string }>;
  groupLoading?: boolean;
  groupError?: string;
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

const getCollapsedPreview = (content: string): string => {
  const firstLine = content.split("\n").map((l) => l.trim()).find((l) => l.length > 0) ?? "";
  return firstLine.length > 160 ? `${firstLine.slice(0, 160)}...` : firstLine;
};

const countExtraLines = (content: string): number => {
  const lines = content.split("\n").filter((l) => l.trim().length > 0);
  return Math.max(0, lines.length - 1);
};

const isCollapsibleType = (type: string): boolean =>
  type === "tool_call" || type === "tool_call_group";

export function TuiActivityBlock({
  activityType,
  detail,
  time,
  groupId,
  onExpandGroup,
  groupChildren,
  groupLoading,
  groupError,
}: TuiActivityBlockProps) {
  const collapsible = isCollapsibleType(activityType);
  const [expanded, setExpanded] = useState(!collapsible);
  const extraLines = countExtraLines(detail);

  const handleToggle = useCallback(() => {
    const willExpand = !expanded;
    setExpanded(willExpand);
    if (willExpand && activityType === "tool_call_group" && groupId && onExpandGroup) {
      onExpandGroup(groupId);
    }
  }, [expanded, activityType, groupId, onExpandGroup]);

  const isThought = activityType === "agent_thought";

  return (
    <div className={`ml-6 my-1 border-l-2 px-3 py-1 text-sm ${expanded ? "accent-border" : "border-slate-200"}`}>
      <div className="flex items-center gap-1.5">
        {collapsible ? (
          <>
            <button
              type="button"
              onClick={handleToggle}
              className="flex h-4 w-4 flex-shrink-0 items-center justify-center text-slate-400 hover:text-slate-700"
              aria-label={expanded ? "收起" : "展开"}
            >
              <svg viewBox="0 0 12 12" className="h-3 w-3 fill-current">
                {expanded
                  ? <path d="M6 8L1 3h10z"/>
                  : <path d="M8 6L3 1v10z"/>}
              </svg>
            </button>
            {!expanded && (
              <span className="truncate font-mono text-xs text-slate-500">
                {getCollapsedPreview(detail)}
                {extraLines > 0 && (
                  <span className="ml-1 text-slate-400">… +{extraLines} lines</span>
                )}
              </span>
            )}
          </>
        ) : (
          <span className="text-xs text-slate-500">{isThought ? "Thinking" : activityType}</span>
        )}
        <span className="ml-auto flex-shrink-0 text-[10px] text-slate-400">{formatTime(time)}</span>
      </div>

      {(expanded || !collapsible) && (
        <div className="relative mt-1 max-h-80 overflow-y-auto text-xs">
          {collapsible && (
            <div className="sticky top-0 flex justify-end">
              <button
                type="button"
                onClick={handleToggle}
                className="rounded bg-white/80 px-1.5 py-0.5 text-[10px] text-slate-400 hover:text-slate-700 backdrop-blur-sm"
                aria-label="收起"
              >
                ▲ 收起
              </button>
            </div>
          )}
          {activityType === "tool_call_group" && groupChildren ? (
            <div className="space-y-2">
              <div className="text-slate-600">{detail}</div>
              {groupLoading ? (
                <p className="text-slate-500">加载中...</p>
              ) : groupError ? (
                <p className="text-rose-600">加载失败：{groupError}</p>
              ) : groupChildren.length > 0 ? (
                groupChildren.map((child, idx) => (
                  <div key={`${child.id}-${idx}`} className="ml-2 border-l border-slate-200 px-2 py-1">
                    <p className="text-[10px] text-slate-500">{child.type} · {formatTime(child.time)}</p>
                    <div className="mt-1"><TuiMarkdown content={child.detail} /></div>
                  </div>
                ))
              ) : (
                <p className="text-slate-500">暂无详情</p>
              )}
            </div>
          ) : (
            <TuiMarkdown content={detail} />
          )}
        </div>
      )}
    </div>
  );
}
