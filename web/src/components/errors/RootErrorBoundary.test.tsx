// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RootErrorBoundary } from "./RootErrorBoundary";

function HealthyChild() {
  return <div>healthy tree</div>;
}

function BrokenChild(): never {
  throw new Error("boom from boundary test");
}

describe("RootErrorBoundary", () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("正常渲染子树", () => {
    render(
      <RootErrorBoundary>
        <HealthyChild />
      </RootErrorBoundary>,
    );

    expect(screen.getByText("healthy tree")).toBeTruthy();
  });

  it("子树抛错时展示根级兜底页", () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    render(
      <RootErrorBoundary>
        <BrokenChild />
      </RootErrorBoundary>,
    );

    expect(screen.getByRole("heading", { name: "页面遇到未捕获错误" })).toBeTruthy();
    expect(screen.getByText("boom from boundary test")).toBeTruthy();
    expect(screen.getByRole("button", { name: "刷新页面" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "返回首页" })).toBeTruthy();
    expect(errorSpy).toHaveBeenCalled();
  });
});
