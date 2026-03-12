import type { Issue, Step } from "@/types/apiV2";

export const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error && error.message.trim().length > 0) {
    return error.message;
  }
  return "请求失败，请稍后重试";
};

export const formatRelativeTime = (input?: string | null): string => {
  if (!input) {
    return "-";
  }
  const date = new Date(input);
  if (Number.isNaN(date.getTime())) {
    return input;
  }

  const diffMs = Date.now() - date.getTime();
  const diffMinutes = Math.max(0, Math.floor(diffMs / 60000));
  if (diffMinutes < 1) {
    return "刚刚";
  }
  if (diffMinutes < 60) {
    return `${diffMinutes} 分钟前`;
  }
  const diffHours = Math.floor(diffMinutes / 60);
  if (diffHours < 24) {
    return `${diffHours} 小时前`;
  }
  const diffDays = Math.floor(diffHours / 24);
  if (diffDays < 30) {
    return `${diffDays} 天前`;
  }
  return date.toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
};

export const formatIssueDuration = (issue: Pick<Issue, "created_at" | "updated_at" | "status">): string => {
  const created = new Date(issue.created_at);
  if (Number.isNaN(created.getTime())) {
    return "-";
  }
  const updated = new Date(issue.updated_at);
  const end =
    issue.status === "done" || issue.status === "failed" || issue.status === "cancelled"
      ? updated
      : new Date();
  const diffMs = Math.max(0, end.getTime() - created.getTime());
  const diffMinutes = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMinutes / 60);
  if (diffHours > 0) {
    return `${diffHours}h ${diffMinutes % 60}m`;
  }
  return `${diffMinutes}m`;
};

export const isActiveIssueStatus = (status: string): boolean =>
  status === "queued" || status === "running" || status === "blocked" || status === "pending";

export const normalizeStepTypeLabel = (type: Step["type"]): string => {
  switch (type) {
    case "exec":
      return "执行";
    case "gate":
      return "门禁";
    case "composite":
      return "复合";
    default:
      return type;
  }
};
