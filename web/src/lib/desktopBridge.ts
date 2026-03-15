export interface DesktopBootstrap {
  token: string;
  // Legacy compatibility base URL retained for legacy API integrations.
  api_v1_base_url: string;
  api_base_url: string;
  // Compatibility field: desktop websocket still points at the legacy v1 endpoint.
  ws_base_url: string;
}

export const isTauri = (): boolean => {
  if (typeof window === "undefined") {
    return false;
  }
  const w = window as unknown as {
    __TAURI__?: unknown;
    __TAURI_INTERNALS__?: unknown;
    __TAURI_IPC__?: unknown;
  };
  return Boolean(w.__TAURI__ || w.__TAURI_INTERNALS__ || w.__TAURI_IPC__);
};

const sleep = (ms: number): Promise<void> =>
  new Promise((resolve) => setTimeout(resolve, ms));

export const fetchDesktopBootstrap = async (options?: {
  timeoutMs?: number;
  retryIntervalMs?: number;
}): Promise<DesktopBootstrap> => {
  if (!isTauri()) {
    throw new Error("not running in Tauri");
  }

  const timeoutMs = options?.timeoutMs ?? 8000;
  const retryIntervalMs = options?.retryIntervalMs ?? 300;
  const startedAt = Date.now();
  let lastError: unknown = null;

  while (Date.now() - startedAt < timeoutMs) {
    try {
      const { invoke } = await import("@tauri-apps/api/core");
      const result = await invoke<DesktopBootstrap>("desktop_bootstrap");
      if (!result || typeof result.token !== "string" || result.token.trim().length === 0) {
        throw new Error("desktop_bootstrap returned empty token");
      }
      return result;
    } catch (err) {
      lastError = err;
      await sleep(retryIntervalMs);
    }
  }

  if (lastError instanceof Error) {
    throw lastError;
  }
  throw new Error("desktop bootstrap timed out");
};

