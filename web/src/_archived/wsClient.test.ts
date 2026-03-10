import { describe, expect, test } from "vitest";
import { WsClient } from "./wsClient";

class FakeWebSocket {
  static readonly OPEN = 1;
  static readonly CONNECTING = 0;
  static readonly CLOSED = 3;

  static createdUrls: string[] = [];
  static instances: FakeWebSocket[] = [];

  url: string;
  readyState = FakeWebSocket.CONNECTING;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;

  constructor(url: string) {
    this.url = url;
    FakeWebSocket.createdUrls.push(url);
    FakeWebSocket.instances.push(this);
  }

  send(_data: string): void {}

  close(): void {
    this.readyState = FakeWebSocket.CLOSED;
    this.onclose?.();
  }

  open(): void {
    this.readyState = FakeWebSocket.OPEN;
    this.onopen?.();
  }

  emitMessage(data: string): void {
    this.onmessage?.({ data });
  }
}

describe("WsClient", () => {
  test("builds ws url with token and routes message from data field", () => {
    FakeWebSocket.createdUrls = [];
    FakeWebSocket.instances = [];

    const client = new WsClient(
      {
        baseUrl: "http://127.0.0.1:8080/api/v1",
        getToken: () => "secret-token",
      },
      FakeWebSocket as unknown as new (url: string) => WebSocket,
    );

    let received: unknown = null;
    client.subscribe("plan_done", (payload) => {
      received = payload;
    });

    client.connect();
    expect(FakeWebSocket.createdUrls).toHaveLength(1);
    expect(FakeWebSocket.createdUrls[0]).toContain("/api/v1/ws");
    expect(FakeWebSocket.createdUrls[0]).toContain("token=secret-token");

    const socket = FakeWebSocket.instances[0];
    socket.open();
    socket.emitMessage(
      JSON.stringify({
        type: "plan_done",
        data: { ok: true, count: 3 },
      }),
    );

    expect(received).toEqual({ ok: true, count: 3 });
  });
});
