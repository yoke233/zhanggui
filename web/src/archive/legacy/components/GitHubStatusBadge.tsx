import type { GitHubConnectionStatus } from "@/types/workflow";

interface GitHubStatusBadgeProps {
  status?: GitHubConnectionStatus;
}

const STATUS_META: Record<GitHubConnectionStatus, { label: string; className: string }> = {
  connected: {
    label: "Connected",
    className: "border-emerald-200 bg-emerald-50 text-emerald-700",
  },
  degraded: {
    label: "Degraded",
    className: "border-amber-200 bg-amber-50 text-amber-700",
  },
  disconnected: {
    label: "Disconnected",
    className: "border-slate-200 bg-slate-100 text-slate-600",
  },
};

const GitHubStatusBadge = ({ status = "disconnected" }: GitHubStatusBadgeProps) => {
  const meta = STATUS_META[status] ?? STATUS_META.disconnected;
  return (
    <span
      data-testid="github-status-badge"
      className={`inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium ${meta.className}`}
    >
      GitHub: {meta.label}
    </span>
  );
};

export default GitHubStatusBadge;
