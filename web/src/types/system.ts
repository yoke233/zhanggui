export interface SandboxProviderSupport {
  supported: boolean;
  reason?: string;
}

export interface SandboxSupportResponse {
  os: string;
  arch: string;
  enabled: boolean;
  current_provider: string;
  current_supported: boolean;
  providers: Record<string, SandboxProviderSupport>;
}
