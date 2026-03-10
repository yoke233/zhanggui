import { Badge, type BadgeProps } from "@/components/ui/badge";

type Status = "done" | "succeeded" | "running" | "in_progress" | "pending" | "queued" | "ready"
  | "failed" | "cancelled" | "blocked" | "waiting_gate" | "created" | string;

const statusConfig: Record<string, { variant: BadgeProps["variant"]; label: string }> = {
  done: { variant: "success", label: "已完成" },
  succeeded: { variant: "success", label: "成功" },
  running: { variant: "info", label: "运行中" },
  in_progress: { variant: "info", label: "进行中" },
  pending: { variant: "secondary", label: "待处理" },
  queued: { variant: "secondary", label: "排队中" },
  ready: { variant: "info", label: "就绪" },
  failed: { variant: "destructive", label: "失败" },
  cancelled: { variant: "secondary", label: "已取消" },
  blocked: { variant: "warning", label: "阻塞" },
  waiting_gate: { variant: "warning", label: "等待审核" },
  created: { variant: "secondary", label: "已创建" },
};

export function StatusBadge({ status }: { status: Status }) {
  const config = statusConfig[status] ?? { variant: "outline" as const, label: status };
  return <Badge variant={config.variant}>{config.label}</Badge>;
}
