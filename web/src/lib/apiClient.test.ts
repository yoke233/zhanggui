import { describe, expect, it, vi, afterEach } from "vitest";
import { ApiError, createApiClient } from "./apiClient";

describe("apiClient", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("会在请求头注入 Bearer token 并返回 JSON", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ ok: true }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({
      baseUrl: "http://localhost:8080/api/v1",
      getToken: () => "secret-token",
    });

    const result = await client.request<{ ok: boolean }>({
      path: "/projects",
    });

    expect(result.ok).toBe(true);
    expect(fetchMock).toHaveBeenCalledOnce();
    const call = fetchMock.mock.calls[0];
    expect(call?.[0]).toBe("http://localhost:8080/api/v1/projects");

    const requestInit = call?.[1];
    const headers = requestInit?.headers;
    expect(headers).toBeInstanceOf(Headers);
    expect((headers as Headers).get("Authorization")).toBe("Bearer secret-token");
  });

  it("当响应非 2xx 时抛出 ApiError", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ message: "bad request" }), {
          status: 400,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );

    const client = createApiClient({
      baseUrl: "http://localhost:8080/api/v1",
      getToken: () => "",
    });

    await expect(client.request({ path: "/projects" })).rejects.toBeInstanceOf(
      ApiError,
    );
  });

  it("listIssues/listRuns 会走 v2 路由并透传分页参数", async () => {
    const fetchMock = vi.fn().mockImplementation(async () => {
      return new Response(JSON.stringify({ items: [], total: 0, offset: 0 }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({
      baseUrl: "http://localhost:8080/api/v1",
    });

    await client.listIssues("proj-1", { limit: 50, offset: 100 });
    await client.listRuns("proj-1", { limit: 20, offset: 40 });

    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "http://localhost:8080/api/v2/issues?project_id=proj-1&limit=50&offset=100",
    );
    expect(fetchMock.mock.calls[1]?.[0]).toBe(
      "http://localhost:8080/api/v2/runs?project_id=proj-1&limit=20&offset=40",
    );
  });

  it("/api/* 路由会保留 baseUrl 的子路径前缀", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ items: [], total: 0, offset: 0 }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({
      baseUrl: "http://localhost:8080/console/api/v1",
    });

    await client.listRuns("proj-1", { limit: 1, offset: 2 });

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "http://localhost:8080/console/api/v2/runs?project_id=proj-1&limit=1&offset=2",
    );
  });

  it("workflow profile API 会走 v2 路由", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ items: [] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            type: "strict",
            sla_minutes: 60,
            reviewer_count: 3,
            description: "strict review flow",
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        ),
      );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({
      baseUrl: "http://localhost:8080/api/v1",
    });

    await client.listWorkflowProfiles();
    await client.getWorkflowProfile("strict");

    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "http://localhost:8080/api/v2/workflow-profiles",
    );
    expect(fetchMock.mock.calls[1]?.[0]).toBe(
      "http://localhost:8080/api/v2/workflow-profiles/strict",
    );
  });

  it("issue 动作接口会命中 issues 路由，不再访问 plans/tasks", async () => {
    const fetchMock = vi.fn().mockImplementation(async () => {
      return new Response(JSON.stringify({ status: "ok" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({
      baseUrl: "http://localhost:8080/api/v1",
    });

    await client.submitIssueReview("proj-1", "issue-1");
    await client.applyIssueAction("proj-1", "issue-1", {
      action: "reject",
      feedback: {
        category: "coverage_gap",
        detail: "补齐异常路径与失败回滚逻辑。",
      },
    });
    await client.setIssueAutoMerge("proj-1", "issue-1", {
      auto_merge: false,
    });

    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "http://localhost:8080/api/v1/projects/proj-1/issues/issue-1/review",
    );
    expect(fetchMock.mock.calls[1]?.[0]).toBe(
      "http://localhost:8080/api/v1/projects/proj-1/issues/issue-1/action",
    );
    expect(fetchMock.mock.calls[2]?.[0]).toBe(
      "http://localhost:8080/api/v1/projects/proj-1/issues/issue-1/auto-merge",
    );

    const actionBody = JSON.parse(String((fetchMock.mock.calls[1]?.[1] as RequestInit)?.body));
    expect(actionBody).toEqual({
      action: "reject",
      feedback: {
        category: "coverage_gap",
        detail: "补齐异常路径与失败回滚逻辑。",
      },
    });
  });

  it("issue 历史与审计接口会命中正确路由并透传查询参数", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify([]), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify([]), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            items: [],
            total: 0,
            offset: 0,
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        ),
      );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({
      baseUrl: "http://localhost:8080/api/v1",
    });

    await client.listIssueReviews?.("proj-1", "issue-1");
    await client.listIssueChanges?.("proj-1", "issue-1");
    await client.listAdminAuditLog?.({
      projectId: "proj-1",
      action: "force_ready",
      user: "admin",
      since: "2026-03-01T00:00:00Z",
      until: "2026-03-03T23:59:59Z",
      limit: 50,
      offset: 10,
    });

    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "http://localhost:8080/api/v1/projects/proj-1/issues/issue-1/reviews",
    );
    expect(fetchMock.mock.calls[1]?.[0]).toBe(
      "http://localhost:8080/api/v1/projects/proj-1/issues/issue-1/changes",
    );
    expect(fetchMock.mock.calls[2]?.[0]).toBe(
      "http://localhost:8080/api/v1/admin/audit-log?project_id=proj-1&action=force_ready&user=admin&since=2026-03-01T00%3A00%3A00Z&until=2026-03-03T23%3A59%3A59Z&limit=50&offset=10",
    );
  });

  it("Run logs 接口会命中正确路由并透传 stage/limit/offset", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          items: [
            {
              id: 2,
              run_id: "pipe-1",
              stage: "implement",
              type: "stdout",
              agent: "codex",
              content: "implement-log-2",
              timestamp: "2026-03-03T10:02:00Z",
            },
          ],
          total: 2,
          offset: 1,
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({
      baseUrl: "http://localhost:8080/api/v1",
    });

    const logs = await client.getRunLogs("proj-1", "pipe-1", {
      stage: "implement",
      limit: 1,
      offset: 1,
    });

    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "http://localhost:8080/api/v1/projects/proj-1/Runs/pipe-1/logs?stage=implement&limit=1&offset=1",
    );
    const requestInit = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(requestInit.method).toBe("GET");
    expect(logs.total).toBe(2);
    expect(logs.offset).toBe(1);
    expect(logs.items[0]?.content).toBe("implement-log-2");
  });

  it("issue timeline 接口会命中正确路由并返回分页事件列表", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          items: [
            {
              event_id: "change:chg-1",
              kind: "change",
              created_at: "2026-03-03T10:03:00Z",
              actor_type: "system",
              actor_name: "system",
              actor_avatar_seed: "system",
              title: "change · status",
              body: "draft -> reviewing · submit review",
              status: "info",
              refs: {
                issue_id: "issue-1",
                run_id: "pipe-1",
              },
              meta: { field: "status" },
            },
            {
              event_id: "review:7",
              kind: "review",
              created_at: "2026-03-03T10:04:00Z",
              actor_type: "agent",
              actor_name: "reviewer",
              actor_avatar_seed: "reviewer",
              title: "review · reviewer",
              body: "verdict=changes_requested · score=70",
              status: "warning",
              refs: {
                issue_id: "issue-1",
                run_id: "pipe-1",
              },
              meta: { verdict: "changes_requested", score: 70 },
            },
          ],
          total: 2,
          offset: 0,
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({
      baseUrl: "http://localhost:8080/api/v1",
    });

    const timeline = await client.listIssueTimeline("proj-1", "issue-1", {
      limit: 20,
      offset: 0,
    });

    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "http://localhost:8080/api/v1/projects/proj-1/issues/issue-1/timeline?limit=20&offset=0",
    );
    const requestInit = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(requestInit.method).toBe("GET");
    expect(timeline.total).toBe(2);
    expect(timeline.items).toHaveLength(2);
    expect(timeline.items[0]?.kind).toBe("change");
    expect(timeline.items[1]?.kind).toBe("review");
    expect(timeline.items[1]?.event_id).toBe("review:7");
  });

  it("createIssueFromFiles 命中 issues/from-files 路由", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          id: "issue-files-1",
          project_id: "proj-1",
          session_id: "chat-1",
          title: "Issue From Files",
          body: "",
          labels: [],
          milestone_id: "",
          attachments: [],
          depends_on: [],
          blocks: [],
          priority: 0,
          template: "standard",
          auto_merge: false,
          state: "open",
          status: "draft",
          run_id: "",
          version: 1,
          superseded_by: "",
          external_id: "",
          fail_policy: "block",
          created_at: "2026-03-01T00:00:00Z",
          updated_at: "2026-03-01T00:00:00Z",
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({
      baseUrl: "http://localhost:8080/api/v1",
    });

    const issue = await client.createIssueFromFiles("proj-1", {
      session_id: "chat-1",
      name: "Issue From Files",
      fail_policy: "block",
      file_paths: ["cmd/ai-flow/main.go", "internal/core/issue.go"],
    });

    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "http://localhost:8080/api/v1/projects/proj-1/issues/from-files",
    );
    const requestInit = fetchMock.mock.calls[0]?.[1] as RequestInit;
    const parsedBody = JSON.parse(String(requestInit.body)) as Record<string, unknown>;
    expect(parsedBody).toEqual({
      session_id: "chat-1",
      name: "Issue From Files",
      fail_policy: "block",
      file_paths: ["cmd/ai-flow/main.go", "internal/core/issue.go"],
    });
    expect(issue.id).toBe("issue-files-1");
    expect(issue.title).toBe("Issue From Files");
  });

  it("兼容别名 listPlans/createPlanFromFiles 也不再访问 /plans", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ items: [], total: 0, offset: 0 }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            id: "issue-files-2",
            project_id: "proj-1",
            session_id: "chat-1",
            title: "Issue",
            body: "",
            labels: [],
            milestone_id: "",
            attachments: [],
            depends_on: [],
            blocks: [],
            priority: 0,
            template: "standard",
            auto_merge: false,
            state: "open",
            status: "draft",
            run_id: "",
            version: 1,
            superseded_by: "",
            external_id: "",
            fail_policy: "block",
            created_at: "2026-03-01T00:00:00Z",
            updated_at: "2026-03-01T00:00:00Z",
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        ),
      );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({
      baseUrl: "http://localhost:8080/api/v1",
    });

    await client.listPlans("proj-1", { limit: 1, offset: 0 });
    await client.createPlanFromFiles("proj-1", {
      session_id: "chat-1",
      file_paths: ["README.md"],
    });

    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "http://localhost:8080/api/v2/issues?project_id=proj-1&limit=1&offset=0",
    );
    expect(fetchMock.mock.calls[1]?.[0]).toBe(
      "http://localhost:8080/api/v1/projects/proj-1/issues/from-files",
    );
    expect(String(fetchMock.mock.calls[0]?.[0])).not.toContain("/plans");
    expect(String(fetchMock.mock.calls[1]?.[0])).not.toContain("/plans");
  });

  it("仓库树/状态/diff 接口命中正确路由并透传查询参数", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            dir: "",
            items: [],
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            items: [],
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            file_path: "src/main.ts",
            diff: "diff --git a/src/main.ts b/src/main.ts",
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        ),
      );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({
      baseUrl: "http://localhost:8080/api/v1",
    });

    await client.getRepoTree("proj-1", "src");
    await client.getRepoStatus("proj-1");
    await client.getRepoDiff("proj-1", "src/main.ts");

    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "http://localhost:8080/api/v1/projects/proj-1/repo/tree?dir=src",
    );
    expect(fetchMock.mock.calls[1]?.[0]).toBe(
      "http://localhost:8080/api/v1/projects/proj-1/repo/status",
    );
    expect(fetchMock.mock.calls[2]?.[0]).toBe(
      "http://localhost:8080/api/v1/projects/proj-1/repo/diff?file=src%2Fmain.ts",
    );
  });
});
