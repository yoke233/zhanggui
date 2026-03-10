/** @vitest-environment jsdom */

import { afterEach, describe, it, expect, vi } from "vitest";
import { cleanup, render, screen, fireEvent } from "@testing-library/react";
import { ScrollNavBar } from "./ScrollNavBar";

describe("ScrollNavBar", () => {
  afterEach(() => {
    cleanup();
  });

  const markers = [
    { id: "msg-1", label: "你好世界", position: 0.1 },
    { id: "msg-2", label: "第二条消息内容比较长一些截断", position: 0.5 },
    { id: "msg-3", label: "最后一条", position: 0.9 },
  ];

  it("renders correct number of markers", () => {
    render(<ScrollNavBar markers={markers} onMarkerClick={() => {}} />);
    const buttons = screen.getAllByRole("button");
    expect(buttons.length).toBe(3);
  });

  it("calls onMarkerClick with correct id", () => {
    const onClick = vi.fn();
    render(<ScrollNavBar markers={markers} onMarkerClick={onClick} />);
    const btn = screen.getByLabelText("第二条消息内容比较长一些截断");
    btn.click();
    expect(onClick).toHaveBeenCalledWith("msg-2");
  });

  it("shows tooltip on hover", async () => {
    render(<ScrollNavBar markers={markers} onMarkerClick={() => {}} />);
    fireEvent.mouseEnter(screen.getAllByRole("button")[0]);
    expect(screen.getByText("你好世界")).toBeTruthy();
  });
});
