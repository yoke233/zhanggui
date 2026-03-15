import type { ResourceSpace } from "@/types/apiV2";

export type SupportedScmProvider = "github" | "codeup";

const normalizeProvider = (value: unknown): SupportedScmProvider | null => {
  if (typeof value !== "string") {
    return null;
  }
  const normalized = value.trim().toLowerCase();
  if (normalized === "github" || normalized === "codeup") {
    return normalized;
  }
  return null;
};

const parseHostFromUri = (uri: string): string => {
  const trimmed = uri.trim();
  if (!trimmed) {
    return "";
  }

  if (/^https?:\/\//i.test(trimmed)) {
    try {
      return new URL(trimmed).hostname.toLowerCase();
    } catch {
      return "";
    }
  }

  const sshMatch = trimmed.match(/^[^@]+@([^:]+):/);
  if (sshMatch?.[1]) {
    return sshMatch[1].trim().toLowerCase();
  }

  return "";
};

const normalizeBoolean = (value: unknown): boolean => {
  if (typeof value === "boolean") {
    return value;
  }
  if (typeof value === "string") {
    const normalized = value.trim().toLowerCase();
    return normalized === "true" || normalized === "1" || normalized === "yes" || normalized === "on";
  }
  return false;
};

type ScmBinding = Pick<ResourceSpace, "kind" | "config"> & { uri?: string; root_uri?: string };

export const detectScmProviderFromBinding = (binding: ScmBinding): SupportedScmProvider | null => {
  if (binding.kind.trim().toLowerCase() !== "git") {
    return null;
  }

  const fromConfig = normalizeProvider(binding.config?.provider);
  if (fromConfig) {
    return fromConfig;
  }

  const host = parseHostFromUri(binding.root_uri ?? binding.uri ?? "");
  if (host === "github.com") {
    return "github";
  }
  if (host.includes("codeup.aliyun.com") || host.includes("rdc.aliyuncs.com")) {
    return "codeup";
  }
  return null;
};

export const detectScmProviderFromBindings = (
  bindings: ScmBinding[],
): SupportedScmProvider | null => {
  for (const binding of bindings) {
    const provider = detectScmProviderFromBinding(binding);
    if (provider) {
      return provider;
    }
  }
  return null;
};

export const isScmFlowEnabledBinding = (binding: ScmBinding): boolean => {
  const provider = detectScmProviderFromBinding(binding);
  if (!provider) {
    return false;
  }
  return normalizeBoolean(binding.config?.enable_scm_flow);
};

export const getScmFlowProviderFromBindings = (
  bindings: ScmBinding[],
): SupportedScmProvider | null => {
  for (const binding of bindings) {
    if (isScmFlowEnabledBinding(binding)) {
      return detectScmProviderFromBinding(binding);
    }
  }
  return null;
};

export const detectEnabledScmFlowProviderFromBindings = getScmFlowProviderFromBindings;
