import { afterEach, describe, expect, it, vi } from "vitest";
import { createApiClient } from "./apiClient";

describe("apiClient", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("generateSteps 会命中 /issues/{id}/generate-steps 并 POST JSON body", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify([]), {
        status: 201,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    await client.generateSteps(12, { description: "make a dag" });

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/issues/12/generate-steps");
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({ description: "make a dag" });
  });

  it("updateStep 会命中 /steps/{id} 并 PUT JSON body", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          id: 1,
          issue_id: 2,
          name: "x",
          type: "exec",
          status: "pending",
          max_retries: 0,
          retry_count: 0,
          created_at: "2026-03-10T00:00:00Z",
          updated_at: "2026-03-10T00:00:00Z",
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    await client.updateStep(99, { position: 3 });

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/steps/99");
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("PUT");
    expect(JSON.parse(String(init.body))).toEqual({ position: 3 });
  });

  it("deleteStep 会命中 /steps/{id} 并 DELETE", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    await client.deleteStep(7);

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/steps/7");
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("DELETE");
  });

  it("getSandboxSupport 会命中 /system/sandbox-support", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          os: "windows",
          arch: "amd64",
          enabled: false,
          configured_provider: "home_dir",
          current_provider: "noop",
          current_supported: false,
          providers: {
            home_dir: { supported: true, implemented: true },
            litebox: { supported: true, implemented: true, reason: "ok" },
            docker: { supported: false, implemented: false, reason: "missing" },
          },
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    await client.getSandboxSupport();

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/system/sandbox-support");
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("GET");
  });

  it("updateSandboxSupport 会命中 /admin/system/sandbox-support 并 PUT JSON body", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          os: "darwin",
          arch: "arm64",
          enabled: true,
          configured_provider: "home_dir",
          current_provider: "home_dir",
          current_supported: true,
          providers: {
            home_dir: { supported: true, implemented: true, reason: "ok" },
            litebox: { supported: false, implemented: true, reason: "windows only" },
          },
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    await client.updateSandboxSupport({ enabled: true, provider: "home_dir" });

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/admin/system/sandbox-support");
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("PUT");
    expect(JSON.parse(String(init.body))).toEqual({ enabled: true, provider: "home_dir" });
  });

  it("listIssues 会透传 archived 查询参数", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify([]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    await client.listIssues({ project_id: 7, archived: false, limit: 20, offset: 10 });

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "http://localhost:8080/api/issues?project_id=7&archived=false&limit=20&offset=10",
    );
  });

  it("bootstrapPRIssue 会命中 /issues/{id}/bootstrap-pr 并 POST JSON body", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          issue_id: 12,
          implement_step_id: 101,
          commit_push_step_id: 102,
          open_pr_step_id: 103,
          gate_step_id: 104,
        }),
        {
          status: 201,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    await client.bootstrapPRIssue(12, { title: "demo", base_branch: "master" });

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/issues/12/bootstrap-pr");
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({ title: "demo", base_branch: "master" });
  });

  it("listDrivers 会命中 /agents/drivers", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify([]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    await client.listDrivers();

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/agents/drivers");
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("GET");
  });
});

