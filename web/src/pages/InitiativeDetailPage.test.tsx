// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nextProvider } from "react-i18next";
import {
  createMemoryRouter,
  MemoryRouter,
  Route,
  RouterProvider,
  Routes,
} from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import i18n from "../i18n";
import { InitiativeDetailPage } from "./InitiativeDetailPage";

const { mockUseWorkbench } = vi.hoisted(() => ({
  mockUseWorkbench: vi.fn(),
}));

vi.mock("@/contexts/WorkbenchContext", () => ({
  useWorkbench: mockUseWorkbench,
}));

function renderPage(initialEntry = "/initiatives/9") {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <Routes>
          <Route
            path="/initiatives/:initiativeId"
            element={<InitiativeDetailPage />}
          />
        </Routes>
      </MemoryRouter>
    </I18nextProvider>,
  );
}

function buildDetail(id: number = 9, status: string = "proposed", overrides?: Partial<Record<string, unknown>>) {
  return {
    initiative: {
      id,
      title: `跨项目联调 ${id}`,
      description: "串联前后端 proposal 到 work item 的执行主链。",
      status,
      created_by: "owner-1",
      approved_by: status === "approved" ? "reviewer-1" : null,
      review_note: status === "approved" ? "可以执行" : "",
      created_at: "2026-03-21T00:00:00Z",
      updated_at: "2026-03-21T00:00:00Z",
    },
    items: [
      {
        id: 1,
        initiative_id: id,
        work_item_id: 101,
        role: "frontend",
        created_at: "2026-03-21T00:00:00Z",
      },
    ],
    work_items: [
      {
        id: 101,
        title: "补前端审批页",
        body: "提供 initiative 审批入口和状态展示",
        priority: "medium",
        status: "running",
        created_at: "2026-03-21T00:00:00Z",
        updated_at: "2026-03-21T00:00:00Z",
      },
    ],
    threads: [
      {
        id: 7,
        thread_id: 5,
        initiative_id: id,
        relation_type: "source",
        created_at: "2026-03-21T00:00:00Z",
      },
    ],
    progress: {
      total: 1,
      pending: 0,
      running: 1,
      blocked: 0,
      done: 0,
      failed: 0,
      cancelled: 0,
    },
    ...overrides,
  };
}

describe("InitiativeDetailPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    void i18n.changeLanguage("zh-CN");
  });

  afterEach(() => {
    cleanup();
  });

  it("展示 initiative 详情、work items 与关联 threads", async () => {
    const apiClient = {
      getInitiative: vi.fn().mockResolvedValue(buildDetail()),
      getThread: vi.fn().mockResolvedValue({
        id: 5,
        title: "Thread 讨论：跨项目联调",
        status: "active",
        created_at: "2026-03-21T00:00:00Z",
        updated_at: "2026-03-21T00:00:00Z",
      }),
    };
    mockUseWorkbench.mockReturnValue({ apiClient });

    renderPage();

    expect(await screen.findByText("跨项目联调 9")).toBeTruthy();
    expect(await screen.findByText("补前端审批页")).toBeTruthy();
    expect(await screen.findByText("Thread 讨论：跨项目联调")).toBeTruthy();

    await waitFor(() => {
      expect(screen.getByRole("link", { name: /补前端审批页/ }).getAttribute("href")).toBe(
        "/work-items/101",
      );
      expect(
        screen.getByRole("link", { name: /Thread 讨论：跨项目联调/ }).getAttribute("href"),
      ).toBe("/threads/5");
    });
  });

  it("支持审批 proposed initiative", async () => {
    const proposedDetail = buildDetail(9, "proposed");
    const approvedDetail = buildDetail(9, "approved");
    const apiClient = {
      getInitiative: vi
        .fn()
        .mockResolvedValueOnce(proposedDetail)
        .mockResolvedValueOnce(approvedDetail),
      getThread: vi.fn().mockResolvedValue({
        id: 5,
        title: "Thread 讨论：跨项目联调",
        status: "active",
        created_at: "2026-03-21T00:00:00Z",
        updated_at: "2026-03-21T00:00:00Z",
      }),
      approveInitiative: vi.fn().mockResolvedValue(approvedDetail.initiative),
    };
    mockUseWorkbench.mockReturnValue({ apiClient });

    renderPage();

    await screen.findByText("跨项目联调 9");
    fireEvent.change(screen.getByPlaceholderText("reviewer id"), {
      target: { value: "reviewer-a" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));

    await waitFor(() => {
      expect(apiClient.approveInitiative).toHaveBeenCalledWith(9, {
        approved_by: "reviewer-a",
      });
    });
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "Approve" })).toBeNull();
      expect(screen.getAllByText("已批准").length).toBeGreaterThan(0);
    });
  });

  it("切换 initiative 路由时会刷新审批表单默认值", async () => {
    const apiClient = {
      getInitiative: vi
        .fn()
        .mockResolvedValueOnce(buildDetail(9, "proposed"))
        .mockResolvedValueOnce(
          buildDetail(10, "approved", {
            initiative: {
              ...buildDetail(10, "approved").initiative,
              approved_by: "reviewer-2",
              review_note: "已审批通过",
            },
          }),
        ),
      getThread: vi.fn().mockResolvedValue({
        id: 5,
        title: "Thread 讨论：跨项目联调",
        status: "active",
        created_at: "2026-03-21T00:00:00Z",
        updated_at: "2026-03-21T00:00:00Z",
      }),
    };
    mockUseWorkbench.mockReturnValue({ apiClient });

    const router = createMemoryRouter(
      [
        {
          path: "/initiatives/:initiativeId",
          element: <InitiativeDetailPage />,
        },
      ],
      { initialEntries: ["/initiatives/9"] },
    );

    render(
      <I18nextProvider i18n={i18n}>
        <RouterProvider router={router} />
      </I18nextProvider>,
    );

    await screen.findByText("跨项目联调 9");
    fireEvent.change(screen.getByPlaceholderText("reviewer id"), {
      target: { value: "temporary-reviewer" },
    });
    fireEvent.change(screen.getByPlaceholderText("记录审批意见或返工说明"), {
      target: { value: "temporary-note" },
    });

    await router.navigate("/initiatives/10");

    await screen.findByText("跨项目联调 10");
    await waitFor(() => {
      expect(
        (screen.getByPlaceholderText("reviewer id") as HTMLInputElement).value,
      ).toBe("reviewer-2");
      expect(
        (screen.getByPlaceholderText("记录审批意见或返工说明") as HTMLTextAreaElement)
          .value,
      ).toBe("已审批通过");
    });
  });
});
