import { afterEach, describe, expect, it, vi } from "vitest";
import { createApiClientV2 } from "./apiClientV2";

describe("apiClientV2", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("generateSteps 会命中 /flows/{id}/generate-steps 并 POST JSON body", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify([]), {
        status: 201,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClientV2({ baseUrl: "http://localhost:8080/api/v2" });
    await client.generateSteps(12, { description: "make a dag" });

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/v2/flows/12/generate-steps");
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({ description: "make a dag" });
  });

  it("updateStep 会命中 /steps/{id} 并 PUT JSON body", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          id: 1,
          flow_id: 2,
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

    const client = createApiClientV2({ baseUrl: "http://localhost:8080/api/v2" });
    await client.updateStep(99, { depends_on: [1, 2, 3] });

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/v2/steps/99");
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("PUT");
    expect(JSON.parse(String(init.body))).toEqual({ depends_on: [1, 2, 3] });
  });

  it("deleteStep 会命中 /steps/{id} 并 DELETE", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClientV2({ baseUrl: "http://localhost:8080/api/v2" });
    await client.deleteStep(7);

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/v2/steps/7");
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
          current_provider: "noop",
          current_supported: false,
          providers: {
            home_dir: { supported: true },
            litebox: { supported: true, reason: "ok" },
          },
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClientV2({ baseUrl: "http://localhost:8080/api/v2" });
    await client.getSandboxSupport();

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/v2/system/sandbox-support");
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("GET");
  });

  it("listFlows 会透传 archived 查询参数", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify([]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClientV2({ baseUrl: "http://localhost:8080/api/v2" });
    await client.listFlows({ project_id: 7, archived: false, limit: 20, offset: 10 });

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "http://localhost:8080/api/v2/flows?project_id=7&archived=false&limit=20&offset=10",
    );
  });

  it("listDrivers 会命中 /agents/drivers", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify([]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClientV2({ baseUrl: "http://localhost:8080/api/v2" });
    await client.listDrivers();

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/v2/agents/drivers");
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("GET");
  });
});
