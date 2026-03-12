// @vitest-environment jsdom
import { fireEvent, render, screen } from "@testing-library/react";
import { I18nextProvider } from "react-i18next";
import { describe, expect, it, vi } from "vitest";
import i18n from "@/i18n";
import { SandboxSupportPanel } from "./SandboxSupportPanel";

describe("SandboxSupportPanel", () => {
  it("展示当前沙盒状态和 provider 列表", () => {
    const onRefresh = vi.fn();
    void i18n.changeLanguage("zh-CN");
    render(
      <I18nextProvider i18n={i18n}>
        <SandboxSupportPanel
          report={{
            os: "darwin",
            arch: "arm64",
            enabled: true,
            configured_provider: "boxlite",
            current_provider: "boxlite",
            current_supported: false,
            providers: {
              boxlite: { supported: true, implemented: false, reason: "尚未接入" },
              docker: { supported: false, implemented: false, reason: "未发现 docker" },
              home_dir: { supported: true, implemented: true, reason: "基础隔离" },
            },
          }}
          loading={false}
          error={null}
          onRefresh={onRefresh}
        />
      </I18nextProvider>,
    );

    expect(screen.getByText("沙盒状态")).toBeTruthy();
    expect(screen.getByText("darwin / arm64")).toBeTruthy();
    expect(screen.getByText("已开启")).toBeTruthy();
    expect(screen.getAllByText("boxlite").length).toBeGreaterThan(0);
    expect(screen.getByText("基础隔离")).toBeTruthy();
    expect(screen.getAllByText("未接入").length).toBeGreaterThan(0);

    fireEvent.click(screen.getByRole("button", { name: "刷新" }));
    expect(onRefresh).toHaveBeenCalledTimes(1);
  });
});
