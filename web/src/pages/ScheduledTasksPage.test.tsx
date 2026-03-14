// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nextProvider } from "react-i18next";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import i18n from "../i18n";
import { ScheduledTasksPage } from "./ScheduledTasksPage";

const { mockUseWorkbench } = vi.hoisted(() => ({
  mockUseWorkbench: vi.fn(),
}));

vi.mock("@/contexts/WorkbenchContext", () => ({
  useWorkbench: mockUseWorkbench,
}));

function renderPage() {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter>
        <ScheduledTasksPage />
      </MemoryRouter>
    </I18nextProvider>,
  );
}

describe("ScheduledTasksPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    void i18n.changeLanguage("zh-CN");
  });

  afterEach(() => {
    cleanup();
  });

  it("加载并展示定时任务列表，支持启停", async () => {
    const apiClient = {
      listCronWorkItems: vi.fn().mockResolvedValue([
        {
          work_item_id: 12,
          enabled: true,
          is_template: true,
          schedule: "0 * * * *",
          max_instances: 2,
          last_triggered: "2026-03-14T02:00:00Z",
        },
      ]),
      listWorkItems: vi.fn().mockResolvedValue([
        {
          id: 12,
          project_id: 9,
          title: "夜间巡检模板",
          body: "",
          priority: "medium",
          status: "open",
          created_at: "2026-03-14T00:00:00Z",
          updated_at: "2026-03-14T00:00:00Z",
        },
      ]),
      disableWorkItemCron: vi.fn().mockResolvedValue({}),
      setupWorkItemCron: vi.fn().mockResolvedValue({}),
    };

    mockUseWorkbench.mockReturnValue({
      apiClient,
      selectedProjectId: 9,
      selectedProject: {
        id: 9,
        name: "Alpha",
      },
      projects: [
        {
          id: 9,
          name: "Alpha",
        },
      ],
    });

    renderPage();

    expect(await screen.findByText("夜间巡检模板")).toBeTruthy();
    expect(apiClient.listCronWorkItems).toHaveBeenCalledTimes(1);
    expect(apiClient.listWorkItems).toHaveBeenCalledWith({
      project_id: 9,
      archived: false,
      limit: 200,
      offset: 0,
    });

    fireEvent.click(screen.getByRole("button", { name: "停用" }));

    await waitFor(() => {
      expect(apiClient.disableWorkItemCron).toHaveBeenCalledWith(12);
    });
  });

  it("新增弹窗只允许选择工作项并保存 cron", async () => {
    const apiClient = {
      listCronWorkItems: vi.fn().mockResolvedValue([]),
      listWorkItems: vi.fn().mockResolvedValue([
        {
          id: 8,
          project_id: 3,
          title: "每日汇总模板",
          body: "",
          priority: "medium",
          status: "open",
          created_at: "2026-03-14T00:00:00Z",
          updated_at: "2026-03-14T00:00:00Z",
        },
      ]),
      disableWorkItemCron: vi.fn(),
      setupWorkItemCron: vi.fn().mockResolvedValue({
        work_item_id: 8,
        enabled: true,
        is_template: true,
        schedule: "0 2 * * *",
        max_instances: 1,
      }),
    };

    mockUseWorkbench.mockReturnValue({
      apiClient,
      selectedProjectId: null,
      selectedProject: null,
      projects: [],
    });

    renderPage();

    await screen.findByRole("heading", { name: "定时任务控制台" });
    fireEvent.click(screen.getByRole("button", { name: "添加定时任务" }));

    fireEvent.change(screen.getByPlaceholderText("搜索工作项标题或编号"), { target: { value: "汇总" } });
    const comboBoxes = screen.getAllByRole("combobox");
    fireEvent.change(comboBoxes[comboBoxes.length - 1], { target: { value: "8" } });
    fireEvent.change(screen.getByPlaceholderText("例如: 0 */6 * * * (每6小时)"), { target: { value: "0 2 * * *" } });

    fireEvent.click(screen.getByRole("button", { name: "保存并启用" }));

    await waitFor(() => {
      expect(apiClient.setupWorkItemCron).toHaveBeenCalledWith(8, {
        schedule: "0 2 * * *",
        max_instances: 1,
      });
    });
  });
});
