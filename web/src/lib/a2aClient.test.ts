import { afterEach, describe, expect, it, vi } from "vitest";
import { A2ARpcError, createA2AClient } from "./a2aClient";

const buildSendParams = () => ({
  message: {
    role: "user" as const,
    parts: [{ kind: "text" as const, text: "hello a2a" }],
    contextId: "chat-1",
  },
  metadata: {
    project_id: "proj-1",
  },
});

describe("a2aClient", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("sendMessage 会封装 JSON-RPC 请求并返回 task", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          jsonrpc: "2.0",
          id: "req-1",
          result: {
            id: "task-1",
            contextId: "chat-1",
            status: {
              state: "working",
            },
          },
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createA2AClient({
      baseUrl: "http://localhost:8080/api/v1",
      getToken: () => "",
    });

    const result = await client.sendMessage(buildSendParams());

    expect(result.id).toBe("task-1");
    expect(result.status.state).toBe("working");

    const call = fetchMock.mock.calls[0];
    expect(call?.[0]).toBe("http://localhost:8080/api/v1/a2a");
    const requestInit = call?.[1] as RequestInit;
    expect(requestInit.method).toBe("POST");
    const body = JSON.parse(String(requestInit.body)) as {
      jsonrpc: string;
      id: string;
      method: string;
      params: unknown;
    };
    expect(body.jsonrpc).toBe("2.0");
    expect(body.id.length).toBeGreaterThan(0);
    expect(body.method).toBe("message/send");
    expect(body.params).toEqual(buildSendParams());
  });

  it("getTask/cancelTask 会命中对应 method", async () => {
    const fetchMock = vi.fn().mockImplementation(async () => {
      return new Response(
        JSON.stringify({
          jsonrpc: "2.0",
          id: "req-2",
          result: {
            id: "task-1",
            status: {
              state: "completed",
            },
          },
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      );
    });
    vi.stubGlobal("fetch", fetchMock);

    const client = createA2AClient({
      baseUrl: "http://localhost:8080/api/v1",
    });

    await client.getTask({
      id: "task-1",
      metadata: { project_id: "proj-1" },
    });
    await client.cancelTask({
      id: "task-1",
      metadata: { project_id: "proj-1" },
    });

    const firstBody = JSON.parse(String((fetchMock.mock.calls[0]?.[1] as RequestInit).body)) as {
      method: string;
    };
    const secondBody = JSON.parse(String((fetchMock.mock.calls[1]?.[1] as RequestInit).body)) as {
      method: string;
    };
    expect(firstBody.method).toBe("tasks/get");
    expect(secondBody.method).toBe("tasks/cancel");
  });

  it("会通过 getToken 注入 Bearer Authorization", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          jsonrpc: "2.0",
          id: "req-3",
          result: {
            id: "task-1",
            status: { state: "submitted" },
          },
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createA2AClient({
      baseUrl: "http://localhost:8080/api/v1",
      getToken: () => "secret-token",
    });

    await client.sendMessage(buildSendParams());

    const requestInit = fetchMock.mock.calls[0]?.[1] as RequestInit;
    const headers = requestInit.headers as Headers;
    expect(headers.get("Authorization")).toBe("Bearer secret-token");
  });

  it("JSON-RPC error 会抛出 A2ARpcError 并携带 code/message", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            jsonrpc: "2.0",
            id: "req-4",
            error: {
              code: -32602,
              message: "invalid params",
            },
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        ),
      ),
    );

    const client = createA2AClient({
      baseUrl: "http://localhost:8080/api/v1",
    });

    await expect(
      client.sendMessage({
        message: {
          role: "user",
          parts: [],
        },
      }),
    ).rejects.toMatchObject({
      name: "A2ARpcError",
      code: -32602,
      message: "invalid params",
    });
  });

  it("streamMessage 会解析 SSE 并返回事件序列", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        [
          "event: delta",
          'data: {"text":"hello"}',
          "",
          "event: task",
          'data: {"id":"task-1","status":{"state":"working"}}',
          "",
          "event: done",
          'data: {"done":true,"id":"task-1"}',
          "",
        ].join("\n"),
        {
          status: 200,
          headers: { "Content-Type": "text/event-stream" },
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createA2AClient({
      baseUrl: "http://localhost:8080/api/v1",
    });

    const events = await client.streamMessage(buildSendParams());

    expect(events).toEqual([
      { event: "delta", data: { text: "hello" } },
      { event: "task", data: { id: "task-1", status: { state: "working" } } },
      { event: "done", data: { done: true, id: "task-1" } },
    ]);

    const body = JSON.parse(String((fetchMock.mock.calls[0]?.[1] as RequestInit).body)) as {
      method: string;
    };
    expect(body.method).toBe("message/stream");
  });

  it("streamMessage 返回 JSON-RPC error 时也会抛出 A2ARpcError", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            jsonrpc: "2.0",
            id: "req-5",
            error: {
              code: -32004,
              message: "task not found",
            },
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        ),
      ),
    );

    const client = createA2AClient({
      baseUrl: "http://localhost:8080/api/v1",
    });

    await expect(client.streamMessage(buildSendParams())).rejects.toBeInstanceOf(A2ARpcError);
  });
});

