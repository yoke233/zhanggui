import { useEffect, useRef, useState } from "react";
import type { WsClient } from "@/lib/wsClient";
import type { SystemEventPayload } from "@/types/ws";

interface PreflightStep {
  name: string;
  status: string;
  duration: string;
}

interface BannerState {
  visible: boolean;
  variant: "info" | "success" | "error" | "warning";
  title: string;
  steps: PreflightStep[];
  countdown: number | null;
}

const INITIAL_STATE: BannerState = {
  visible: false,
  variant: "info",
  title: "",
  steps: [],
  countdown: null,
};

const AUTO_HIDE_MS = 8000;

interface Props {
  wsClient: WsClient;
}

const SystemEventBanner = ({ wsClient }: Props) => {
  const [state, setState] = useState<BannerState>(INITIAL_STATE);
  const pendingReload = useRef(false);

  // After restart event, reload page when WS reconnects.
  useEffect(() => {
    const unsubStatus = wsClient.onStatusChange((status) => {
      if (status === "open" && pendingReload.current) {
        pendingReload.current = false;
        window.location.reload();
      }
    });
    return unsubStatus;
  }, [wsClient]);

  useEffect(() => {
    let hideTimer: ReturnType<typeof setTimeout> | null = null;

    const clearHideTimer = () => {
      if (hideTimer) {
        clearTimeout(hideTimer);
        hideTimer = null;
      }
    };

    const autoHide = () => {
      clearHideTimer();
      hideTimer = setTimeout(() => {
        setState((prev) => ({ ...prev, visible: false }));
      }, AUTO_HIDE_MS);
    };

    const unsub = wsClient.subscribe<SystemEventPayload>(
      "system_event",
      (payload) => {
        if (!payload || !payload.event) return;
        const { event, data } = payload;

        switch (event) {
          case "preflight_start":
            clearHideTimer();
            setState({
              visible: true,
              variant: "info",
              title: "Preflight quality gate running...",
              steps: [],
              countdown: null,
            });
            break;

          case "preflight_step":
            setState((prev) => ({
              ...prev,
              visible: true,
              variant: "info",
              title: data.message ?? "Preflight running...",
              steps: [
                ...prev.steps,
                {
                  name: data.name ?? "?",
                  status: data.status ?? "?",
                  duration: data.duration ?? "",
                },
              ],
            }));
            break;

          case "preflight_pass":
            setState((prev) => ({
              ...prev,
              visible: true,
              variant: "success",
              title: data.message ?? "Preflight passed",
            }));
            autoHide();
            break;

          case "preflight_fail":
            setState((prev) => ({
              ...prev,
              visible: true,
              variant: "error",
              title: data.message ?? "Preflight failed",
            }));
            autoHide();
            break;

          case "restart_countdown":
            clearHideTimer();
            setState((prev) => ({
              ...prev,
              visible: true,
              variant: "warning",
              title: data.message ?? "Server restarting...",
              countdown: data.seconds ?? null,
            }));
            break;

          case "restart":
            clearHideTimer();
            pendingReload.current = true;
            setState({
              visible: true,
              variant: "warning",
              title: "Server restarting now — reconnecting...",
              steps: [],
              countdown: 0,
            });
            break;
        }
      },
    );

    return () => {
      unsub();
      clearHideTimer();
    };
  }, [wsClient]);

  if (!state.visible) return null;

  const variantStyles: Record<string, string> = {
    info: "border-blue-300 bg-blue-50 text-blue-800",
    success: "border-emerald-300 bg-emerald-50 text-emerald-800",
    error: "border-rose-300 bg-rose-50 text-rose-800",
    warning: "border-amber-300 bg-amber-50 text-amber-800",
  };

  return (
    <div
      className={`fixed left-1/2 top-4 z-50 w-full max-w-lg -translate-x-1/2 rounded-xl border p-4 shadow-lg transition-all ${variantStyles[state.variant]}`}
    >
      <div className="flex items-center justify-between">
        <span className="text-sm font-semibold">{state.title}</span>
        <button
          type="button"
          className="ml-3 text-xs opacity-60 hover:opacity-100"
          onClick={() => setState((prev) => ({ ...prev, visible: false }))}
        >
          &times;
        </button>
      </div>

      {state.steps.length > 0 && (
        <div className="mt-2 space-y-1">
          {state.steps.map((step, i) => (
            <div key={i} className="flex items-center gap-2 text-xs">
              <span>{step.status === "PASS" ? "\u2705" : "\u274C"}</span>
              <span className="font-medium">{step.name}</span>
              {step.duration && (
                <span className="opacity-60">{step.duration}</span>
              )}
            </div>
          ))}
        </div>
      )}

      {state.countdown !== null && state.countdown > 0 && (
        <div className="mt-2 text-center text-2xl font-bold">
          {state.countdown}
        </div>
      )}
    </div>
  );
};

export default SystemEventBanner;
