/** @vitest-environment jsdom */
import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { TuiMessage } from "./TuiMessage";

describe("TuiMessage", () => {
  afterEach(() => cleanup());

  it("renders user message with icon prefix and background", () => {
    const { container } = render(
      <TuiMessage role="user" content="hello" time="2026-01-01T00:00:00Z" />,
    );
    expect(screen.getByText("hello")).toBeTruthy();
    expect(container.querySelector(".bg-slate-50")).toBeTruthy();
  });

  it("renders assistant message with bullet prefix", () => {
    render(
      <TuiMessage role="assistant" content="world" time="2026-01-01T00:00:00Z" />,
    );
    expect(screen.getByText("world")).toBeTruthy();
    expect(screen.getByText("•")).toBeTruthy();
  });

  it("renders markdown content in assistant message", () => {
    render(
      <TuiMessage role="assistant" content="use `npm`" time="2026-01-01T00:00:00Z" />,
    );
    expect(screen.getByText("npm").tagName).toBe("CODE");
  });

  it("shows formatted time", () => {
    render(
      <TuiMessage role="user" content="hi" time="2026-03-07T15:30:00Z" />,
    );
    // 时间应该渲染出来（包含 15 或 23 取决于时区）
    const timeEl = screen.getByText(/\d{2}\/\d{2}/);
    expect(timeEl).toBeTruthy();
  });
});
