import { afterEach, describe, expect, it, vi } from "vitest";
import { createApiClient } from "./apiClient";

describe("apiClient cron normalization", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("兼容 issue_id 并归一化为 work_item_id", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify([
          {
            issue_id: 18,
            enabled: true,
            is_template: true,
            schedule: "0 * * * *",
            max_instances: 2,
            last_triggered: "2026-03-14T00:00:00Z",
          },
        ]),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = createApiClient({ baseUrl: "http://localhost:8080/api" });
    const result = await client.listCronWorkItems();

    expect(result).toEqual([
      {
        work_item_id: 18,
        enabled: true,
        is_template: true,
        schedule: "0 * * * *",
        max_instances: 2,
        last_triggered: "2026-03-14T00:00:00Z",
      },
    ]);
  });
});
