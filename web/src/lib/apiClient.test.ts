import { afterEach, describe, expect, it, vi } from "vitest";
import { createApiClient } from "./apiClient";

describe("apiClient", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("generateActions 会命中 /work-items/{id}/generate-actions 并 POST JSON body", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify([]), {
        status: 201,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    await client.generateActions(12, { description: "make a dag" });

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/work-items/12/generate-actions");
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({ description: "make a dag" });
  });

  it("updateAction 会命中 /actions/{id} 并 PUT JSON body", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          id: 1,
          work_item_id: 2,
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
    await client.updateAction(99, { position: 3 });

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/actions/99");
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("PUT");
    expect(JSON.parse(String(init.body))).toEqual({ position: 3 });
  });

  it("deleteAction 会命中 /actions/{id} 并 DELETE", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    await client.deleteAction(7);

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/actions/7");
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

  it("getLLMConfig 会命中 /admin/system/llm-config", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          default_config_id: "openai-response-default",
          configs: [
            {
              id: "openai-chat-default",
              type: "openai_chat_completion",
              base_url: "https://api.openai.com/v1",
              api_key: "",
              model: "gpt-4.1",
              temperature: 0,
            },
            {
              id: "openai-response-default",
              type: "openai_response",
              base_url: "https://api.openai.com/v1",
              api_key: "",
              model: "gpt-4.1-mini",
              max_output_tokens: 4096,
            },
            {
              id: "anthropic-default",
              type: "anthropic",
              base_url: "https://api.anthropic.com",
              api_key: "",
              model: "claude-3-7-sonnet-latest",
              thinking_budget_tokens: 2048,
            },
          ],
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    await client.getLLMConfig();

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/admin/system/llm-config");
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("GET");
  });

  it("updateLLMConfig 会命中 /admin/system/llm-config 并 PUT JSON body", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          default_config_id: "anthropic-default",
          configs: [
            {
              id: "anthropic-default",
              type: "anthropic",
              base_url: "https://api.anthropic.com",
              api_key: "sk-ant",
              model: "claude-sonnet-4-5",
              temperature: 0.2,
              max_output_tokens: 4096,
              reasoning_effort: "high",
              thinking_budget_tokens: 2048,
            },
          ],
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    await client.updateLLMConfig({
      default_config_id: "anthropic-default",
      configs: [
        {
          id: "anthropic-default",
          type: "anthropic",
          base_url: "https://api.anthropic.com",
          api_key: "sk-ant",
          model: "claude-sonnet-4-5",
          temperature: 0.2,
          max_output_tokens: 4096,
          reasoning_effort: "high",
          thinking_budget_tokens: 2048,
        },
      ],
    });

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/admin/system/llm-config");
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("PUT");
    expect(JSON.parse(String(init.body))).toEqual({
      default_config_id: "anthropic-default",
      configs: [
        {
          id: "anthropic-default",
          type: "anthropic",
          base_url: "https://api.anthropic.com",
          api_key: "sk-ant",
          model: "claude-sonnet-4-5",
          temperature: 0.2,
          max_output_tokens: 4096,
          reasoning_effort: "high",
          thinking_budget_tokens: 2048,
        },
      ],
    });
  });

  it("listWorkItems 会透传 archived 查询参数", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify([]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    await client.listWorkItems({ project_id: 7, archived: false, limit: 20, offset: 10 });

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "http://localhost:8080/api/work-items?project_id=7&archived=false&limit=20&offset=10",
    );
  });

  it("listCronWorkItems 会命中 /work-items/cron", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify([]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    await client.listCronWorkItems();

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/work-items/cron");
  });

  it("bootstrapPRWorkItem 会命中 /work-items/{id}/bootstrap-pr 并 POST JSON body", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          work_item_id: 12,
          implement_action_id: 101,
          commit_push_action_id: 102,
          open_pr_action_id: 103,
          gate_action_id: 104,
        }),
        {
          status: 201,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    await client.bootstrapPRWorkItem(12, { title: "demo", base_branch: "master" });

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/work-items/12/bootstrap-pr");
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({ title: "demo", base_branch: "master" });
  });

  it("createWorkItemFromTemplate 会命中 /templates/{id}/create-work-item 并 POST JSON body", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          work_item: { id: 12, title: "demo" },
          actions: [],
        }),
        {
          status: 201,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    await client.createWorkItemFromTemplate(12, { title: "demo", project_id: 7 });

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/templates/12/create-work-item");
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({ title: "demo", project_id: 7 });
  });

  it("listDrivers 会命中 /agents/drivers", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify([
        {
          id: "codex-cli",
          launch_command: "codex",
          launch_args: ["run"],
          capabilities_max: {
            fs_read: true,
            fs_write: true,
            terminal: true,
          },
        },
        {
          id: "claude-code",
          launch_command: "claude",
          launch_args: [],
          capabilities_max: {
            fs_read: true,
            fs_write: false,
            terminal: true,
          },
        },
      ]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    const drivers = await client.listDrivers();

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/agents/drivers");
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("GET");
    expect(drivers).toEqual([
      {
        id: "codex-cli",
        launch_command: "codex",
        launch_args: ["run"],
        capabilities_max: {
          fs_read: true,
          fs_write: true,
          terminal: true,
        },
      },
      {
        id: "claude-code",
        launch_command: "claude",
        launch_args: [],
        capabilities_max: {
          fs_read: true,
          fs_write: false,
          terminal: true,
        },
      },
    ]);
  });

  it("listThreads 会命中 /threads", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify([]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    await client.listThreads({ status: "active", limit: 10 });

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "http://localhost:8080/api/threads?status=active&limit=10",
    );
  });

  it("createThread 会命中 /threads 并 POST JSON body", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({ id: 1, title: "test", status: "active", created_at: "", updated_at: "" }),
        { status: 201, headers: { "Content-Type": "application/json" } },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    await client.createThread({ title: "test" });

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/threads");
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({ title: "test" });
  });

  it("createThreadMessage 会命中 /threads/{id}/messages 并 POST", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({ id: 1, thread_id: 5, sender_id: "u1", role: "human", content: "hi", created_at: "" }),
        { status: 201, headers: { "Content-Type": "application/json" } },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    await client.createThreadMessage(5, { content: "hi" });

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/threads/5/messages");
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({ content: "hi" });
  });

  it("addThreadParticipant 会命中 /threads/{id}/participants 并 POST", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({ id: 1, thread_id: 3, user_id: "u1", role: "member", joined_at: "" }),
        { status: 201, headers: { "Content-Type": "application/json" } },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    await client.addThreadParticipant(3, { user_id: "u1", role: "member" });

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/threads/3/participants");
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({ user_id: "u1", role: "member" });
  });

  it("createThreadWorkItemLink posts to correct URL", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({ id: 1, thread_id: 5, work_item_id: 10, relation_type: "related", is_primary: true, created_at: "" }),
        { status: 201, headers: { "Content-Type": "application/json" } },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    await client.createThreadWorkItemLink(5, { work_item_id: 10, relation_type: "related", is_primary: true });

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/threads/5/links/work-items");
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("POST");
  });

  it("listWorkItemsByThread gets correct URL", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    await client.listWorkItemsByThread(5);

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/threads/5/work-items");
  });

  it("listThreadsByWorkItem gets correct URL", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    await client.listThreadsByWorkItem(10);

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/work-items/10/threads");
  });

  it("proposal 相关方法会命中正确路由", async () => {
    const proposal = {
      id: 9,
      thread_id: 5,
      title: "提案 A",
      summary: "summary",
      content: "content",
      proposed_by: "lead",
      status: "draft",
      created_at: "2026-03-21T00:00:00Z",
      updated_at: "2026-03-21T00:00:00Z",
    };
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify([proposal]), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify(proposal), {
          status: 201,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify(proposal), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify(proposal), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(new Response("", { status: 200 }))
      .mockResolvedValueOnce(
        new Response(JSON.stringify(proposal), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ ...proposal, status: "open" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ ...proposal, status: "approved" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ ...proposal, status: "rejected" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ ...proposal, status: "revised" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    await client.listThreadProposals(5, { status: "draft" });
    await client.createThreadProposal(5, { title: "提案 A", proposed_by: "lead" });
    await client.getProposal(9);
    await client.updateProposal(9, { summary: "updated" });
    await client.deleteProposal(9);
    await client.replaceProposalDrafts(9, { work_item_drafts: [] });
    await client.submitProposal(9);
    await client.approveProposal(9, { reviewed_by: "reviewer", review_note: "ok" });
    await client.rejectProposal(9, { reviewed_by: "reviewer", review_note: "retry" });
    await client.reviseProposal(9, { reviewed_by: "reviewer", review_note: "revise" });

    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/api/threads/5/proposals?status=draft");
    expect((fetchMock.mock.calls[0]?.[1] as RequestInit).method).toBe("GET");

    expect(fetchMock.mock.calls[1]?.[0]).toBe("http://localhost:8080/api/threads/5/proposals");
    expect((fetchMock.mock.calls[1]?.[1] as RequestInit).method).toBe("POST");
    expect(JSON.parse(String((fetchMock.mock.calls[1]?.[1] as RequestInit).body))).toEqual({
      title: "提案 A",
      proposed_by: "lead",
    });

    expect(fetchMock.mock.calls[2]?.[0]).toBe("http://localhost:8080/api/proposals/9");
    expect((fetchMock.mock.calls[2]?.[1] as RequestInit).method).toBe("GET");

    expect(fetchMock.mock.calls[3]?.[0]).toBe("http://localhost:8080/api/proposals/9");
    expect((fetchMock.mock.calls[3]?.[1] as RequestInit).method).toBe("PUT");
    expect(JSON.parse(String((fetchMock.mock.calls[3]?.[1] as RequestInit).body))).toEqual({
      summary: "updated",
    });

    expect(fetchMock.mock.calls[4]?.[0]).toBe("http://localhost:8080/api/proposals/9");
    expect((fetchMock.mock.calls[4]?.[1] as RequestInit).method).toBe("DELETE");

    expect(fetchMock.mock.calls[5]?.[0]).toBe("http://localhost:8080/api/proposals/9/drafts");
    expect((fetchMock.mock.calls[5]?.[1] as RequestInit).method).toBe("PUT");
    expect(JSON.parse(String((fetchMock.mock.calls[5]?.[1] as RequestInit).body))).toEqual({
      work_item_drafts: [],
    });

    expect(fetchMock.mock.calls[6]?.[0]).toBe("http://localhost:8080/api/proposals/9/submit");
    expect((fetchMock.mock.calls[6]?.[1] as RequestInit).method).toBe("POST");

    expect(fetchMock.mock.calls[7]?.[0]).toBe("http://localhost:8080/api/proposals/9/approve");
    expect((fetchMock.mock.calls[7]?.[1] as RequestInit).method).toBe("POST");
    expect(JSON.parse(String((fetchMock.mock.calls[7]?.[1] as RequestInit).body))).toEqual({
      reviewed_by: "reviewer",
      review_note: "ok",
    });

    expect(fetchMock.mock.calls[8]?.[0]).toBe("http://localhost:8080/api/proposals/9/reject");
    expect((fetchMock.mock.calls[8]?.[1] as RequestInit).method).toBe("POST");
    expect(JSON.parse(String((fetchMock.mock.calls[8]?.[1] as RequestInit).body))).toEqual({
      reviewed_by: "reviewer",
      review_note: "retry",
    });

    expect(fetchMock.mock.calls[9]?.[0]).toBe("http://localhost:8080/api/proposals/9/revise");
    expect((fetchMock.mock.calls[9]?.[1] as RequestInit).method).toBe("POST");
    expect(JSON.parse(String((fetchMock.mock.calls[9]?.[1] as RequestInit).body))).toEqual({
      reviewed_by: "reviewer",
      review_note: "revise",
    });
  });

  it("initiative 相关方法会命中正确路由", async () => {
    const initiative = {
      id: 3,
      title: "计划 A",
      description: "desc",
      status: "draft",
      created_by: "lead",
      created_at: "2026-03-21T00:00:00Z",
      updated_at: "2026-03-21T00:00:00Z",
    };
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify([initiative]), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            initiative,
            items: [],
            work_items: [],
            threads: [],
            progress: {
              total: 0,
              pending: 0,
              running: 0,
              blocked: 0,
              done: 0,
              failed: 0,
              cancelled: 0,
            },
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        ),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ ...initiative, status: "proposed" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ ...initiative, status: "executing" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ ...initiative, status: "draft", review_note: "rework" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ ...initiative, status: "cancelled" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify([]), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    await client.listInitiatives({ status: "draft", limit: 10, offset: 5 });
    await client.getInitiative(3);
    await client.proposeInitiative(3);
    await client.approveInitiative(3, { approved_by: "approver" });
    await client.rejectInitiative(3, { review_note: "rework" });
    await client.cancelInitiative(3);
    await client.listInitiativeThreads(3);

    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "http://localhost:8080/api/initiatives?status=draft&limit=10&offset=5",
    );
    expect((fetchMock.mock.calls[0]?.[1] as RequestInit).method).toBe("GET");

    expect(fetchMock.mock.calls[1]?.[0]).toBe("http://localhost:8080/api/initiatives/3");
    expect((fetchMock.mock.calls[1]?.[1] as RequestInit).method).toBe("GET");

    expect(fetchMock.mock.calls[2]?.[0]).toBe("http://localhost:8080/api/initiatives/3/propose");
    expect((fetchMock.mock.calls[2]?.[1] as RequestInit).method).toBe("POST");

    expect(fetchMock.mock.calls[3]?.[0]).toBe("http://localhost:8080/api/initiatives/3/approve");
    expect((fetchMock.mock.calls[3]?.[1] as RequestInit).method).toBe("POST");
    expect(JSON.parse(String((fetchMock.mock.calls[3]?.[1] as RequestInit).body))).toEqual({
      approved_by: "approver",
    });

    expect(fetchMock.mock.calls[4]?.[0]).toBe("http://localhost:8080/api/initiatives/3/reject");
    expect((fetchMock.mock.calls[4]?.[1] as RequestInit).method).toBe("POST");
    expect(JSON.parse(String((fetchMock.mock.calls[4]?.[1] as RequestInit).body))).toEqual({
      review_note: "rework",
    });

    expect(fetchMock.mock.calls[5]?.[0]).toBe("http://localhost:8080/api/initiatives/3/cancel");
    expect((fetchMock.mock.calls[5]?.[1] as RequestInit).method).toBe("POST");

    expect(fetchMock.mock.calls[6]?.[0]).toBe("http://localhost:8080/api/initiatives/3/threads");
    expect((fetchMock.mock.calls[6]?.[1] as RequestInit).method).toBe("GET");
  });
});
