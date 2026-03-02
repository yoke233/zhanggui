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

  it("listPlans/listPipelines 会透传 limit 与 offset 查询参数", async () => {
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

    await client.listPlans("proj-1", { limit: 50, offset: 100 });
    await client.listPipelines("proj-1", { limit: 20, offset: 40 });

    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "http://localhost:8080/api/v1/projects/proj-1/plans?limit=50&offset=100",
    );
    expect(fetchMock.mock.calls[1]?.[0]).toBe(
      "http://localhost:8080/api/v1/projects/proj-1/pipelines?limit=20&offset=40",
    );
  });

  it("createProject 支持 github 字段并不包含多余字段", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: "p1" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({
      baseUrl: "http://localhost:8080/api/v1",
    });

    await client.createProject({
      name: "proj",
      repo_path: "D:/repo/proj",
      github: {
        owner: "acme",
        repo: "repo",
      },
    });

    const requestInit = fetchMock.mock.calls[0]?.[1] as RequestInit;
    const parsedBody = JSON.parse(String(requestInit.body)) as Record<string, unknown>;
    expect(parsedBody).toEqual({
      name: "proj",
      repo_path: "D:/repo/proj",
      github: {
        owner: "acme",
        repo: "repo",
      },
    });
    expect(parsedBody).not.toHaveProperty("config");
  });

  it("计划/任务/Pipeline 动作接口会命中正确路由并透传请求体", async () => {
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

    await client.submitPlanReview("proj-1", "plan-1");
    await client.applyPlanAction("proj-1", "plan-1", {
      action: "reject",
      feedback: {
        category: "coverage_gap",
        detail: "补齐异常路径与失败回滚逻辑。",
      },
    });
    await client.applyTaskAction("proj-1", "plan-1", "task-1", {
      action: "retry",
    });
    await client.applyPipelineAction("proj-1", "pipe-1", {
      action: "abort",
    });
    await client.getPipelineCheckpoints("proj-1", "pipe-1");

    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "http://localhost:8080/api/v1/projects/proj-1/plans/plan-1/review",
    );
    expect(fetchMock.mock.calls[1]?.[0]).toBe(
      "http://localhost:8080/api/v1/projects/proj-1/plans/plan-1/action",
    );
    expect(fetchMock.mock.calls[2]?.[0]).toBe(
      "http://localhost:8080/api/v1/projects/proj-1/plans/plan-1/tasks/task-1/action",
    );
    expect(fetchMock.mock.calls[3]?.[0]).toBe(
      "http://localhost:8080/api/v1/projects/proj-1/pipelines/pipe-1/action",
    );
    expect(fetchMock.mock.calls[4]?.[0]).toBe(
      "http://localhost:8080/api/v1/projects/proj-1/pipelines/pipe-1/checkpoints",
    );

    const reviewBody = JSON.parse(String((fetchMock.mock.calls[1]?.[1] as RequestInit)?.body));
    expect(reviewBody).toEqual({
      action: "reject",
      feedback: {
        category: "coverage_gap",
        detail: "补齐异常路径与失败回滚逻辑。",
      },
    });

    const taskBody = JSON.parse(String((fetchMock.mock.calls[2]?.[1] as RequestInit)?.body));
    expect(taskBody).toEqual({
      action: "retry",
    });

    const pipelineBody = JSON.parse(String((fetchMock.mock.calls[3]?.[1] as RequestInit)?.body));
    expect(pipelineBody).toEqual({
      action: "abort",
    });
  });

  it("getPipeline/listPlans 能携带 task_item_id 与结构化任务字段", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            id: "pipe-1",
            project_id: "proj-1",
            name: "pipeline-one",
            description: "pipeline",
                    template: "standard",
                    status: "created",
                    current_stage: "implement",
                    artifacts: {},
                    config: {
                      issue_number: 201,
                      issue_url: "https://github.com/acme/ai-workflow/issues/201",
                      pr_number: 301,
                      pr_url: "https://github.com/acme/ai-workflow/pull/301",
                      github_connection_status: "connected",
                    },
            branch_name: "",
            worktree_path: "",
            max_total_retries: 5,
            total_retries: 0,
            task_item_id: "task-1",
            started_at: "",
            finished_at: "",
            created_at: "2026-03-01T00:00:00Z",
            updated_at: "2026-03-01T00:00:00Z",
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            items: [
              {
                id: "plan-1",
                project_id: "proj-1",
                session_id: "chat-1",
                name: "plan",
                status: "draft",
                wait_reason: "",
                fail_policy: "block",
                review_round: 0,
                tasks: [
                  {
                    id: "task-1",
                    plan_id: "plan-1",
                    title: "task",
                    description: "task description",
                    labels: [],
                    depends_on: [],
                    inputs: ["oauth_app_id"],
                    outputs: ["oauth_token"],
                    acceptance: ["callback returns 200"],
                    constraints: ["keep backward compatibility"],
                    template: "standard",
                    pipeline_id: "",
                    external_id: "https://github.com/acme/ai-workflow/issues/201",
                    status: "pending",
                    created_at: "2026-03-01T00:00:00Z",
                    updated_at: "2026-03-01T00:00:00Z",
                  },
                ],
                created_at: "2026-03-01T00:00:00Z",
                updated_at: "2026-03-01T00:00:00Z",
              },
            ],
            total: 1,
            offset: 0,
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({
      baseUrl: "http://localhost:8080/api/v1",
    });

    const pipeline = await client.getPipeline("proj-1", "pipe-1");
    const plans = await client.listPlans("proj-1");

    expect(pipeline.task_item_id).toBe("task-1");
    expect(plans.items[0]?.tasks[0]?.inputs[0]).toBe("oauth_app_id");
    expect(plans.items[0]?.tasks[0]?.outputs[0]).toBe("oauth_token");
    expect(plans.items[0]?.tasks[0]?.acceptance[0]).toBe("callback returns 200");
    expect(plans.items[0]?.tasks[0]?.constraints[0]).toBe("keep backward compatibility");
    expect(pipeline.github?.issue_number).toBe(201);
    expect(pipeline.github?.issue_url).toBe("https://github.com/acme/ai-workflow/issues/201");
    expect(pipeline.github?.pr_number).toBe(301);
    expect(pipeline.github?.pr_url).toBe("https://github.com/acme/ai-workflow/pull/301");
    expect(pipeline.github?.connection_status).toBe("connected");
    expect(plans.items[0]?.tasks[0]?.github?.issue_number).toBe(201);
    expect(plans.items[0]?.tasks[0]?.github?.issue_url).toBe(
      "https://github.com/acme/ai-workflow/issues/201",
    );
  });

  it("createProjectCreateRequest/getProjectCreateRequest 命中 create-requests 路由", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ request_id: "req-1" }), {
          status: 202,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            request_id: "req-1",
            status: "succeeded",
            project_id: "proj-9",
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

    const accepted = await client.createProjectCreateRequest({
      name: "demo",
      source_type: "github_clone",
      owner: "acme",
      repo: "demo",
      ref: "main",
    });
    const status = await client.getProjectCreateRequest("req-1");

    expect(accepted.request_id).toBe("req-1");
    expect(status.status).toBe("succeeded");
    expect(status.project_id).toBe("proj-9");
    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "http://localhost:8080/api/v1/projects/create-requests",
    );
    expect(fetchMock.mock.calls[1]?.[0]).toBe(
      "http://localhost:8080/api/v1/projects/create-requests/req-1",
    );

    const requestInit = fetchMock.mock.calls[0]?.[1] as RequestInit;
    const parsedBody = JSON.parse(String(requestInit.body)) as Record<string, unknown>;
    expect(parsedBody).toEqual({
      name: "demo",
      source_type: "github_clone",
      owner: "acme",
      repo: "demo",
      ref: "main",
    });
  });
});
