import { X } from "lucide-react";

interface ChatErrorBannerProps {
  error: string;
  onClose: () => void;
}

export function ChatErrorBanner({ error, onClose }: ChatErrorBannerProps) {
  return (
    <div className="mx-5 mt-4 flex items-center gap-2 rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
      <span className="min-w-0 flex-1">{error}</span>
      <button
        type="button"
        className="shrink-0 rounded p-0.5 text-rose-400 transition-colors hover:text-rose-600"
        onClick={onClose}
      >
        <X className="h-3.5 w-3.5" />
      </button>
    </div>
  );
}
