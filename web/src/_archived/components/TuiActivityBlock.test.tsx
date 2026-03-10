/** @vitest-environment jsdom */
import { describe, it, expect, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { TuiActivityBlock } from "./TuiActivityBlock";

describe("TuiActivityBlock", () => {
  afterEach(() => cleanup());

  it("renders collapsed by default for tool_call", () => {
    render(
      <TuiActivityBlock
        activityType="tool_call"
        detail={"Ran rg -n 'hello'\nline 1\nline 2\nline 3\nline 4"}
        time="2026-01-01T00:00:00Z"
      />,
    );
    expect(screen.getByText(/Ran rg/)).toBeTruthy();
    expect(screen.getByText(/\+\d+ lines/)).toBeTruthy();
  });

  it("expands on click", () => {
    render(
      <TuiActivityBlock
        activityType="tool_call"
        detail={"Ran rg -n 'hello'\nline 1\nline 2\nline 3"}
        time="2026-01-01T00:00:00Z"
      />,
    );
    fireEvent.click(screen.getByRole("button"));
    expect(screen.getByText(/line 3/)).toBeTruthy();
  });

  it("renders agent_thought with thinking label", () => {
    render(
      <TuiActivityBlock
        activityType="agent_thought"
        detail="I need to check the files"
        time="2026-01-01T00:00:00Z"
      />,
    );
    expect(screen.getByText(/I need to check/)).toBeTruthy();
    expect(screen.getByText("Thinking")).toBeTruthy();
  });

  it("renders plan entries expanded by default", () => {
    render(
      <TuiActivityBlock
        activityType="plan"
        detail="- Step 1\n- Step 2"
        time="2026-01-01T00:00:00Z"
      />,
    );
    expect(screen.getByText(/Step 1/)).toBeTruthy();
  });
});
