/** @vitest-environment jsdom */

import { afterEach, describe, it, expect, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { TuiCodeBlock } from "./TuiCodeBlock";

describe("TuiCodeBlock", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders code content", () => {
    render(<TuiCodeBlock code={'const x = 1;'} language="javascript" />);
    expect(screen.getByText(/const/)).toBeTruthy();
  });

  it("shows language label when provided", () => {
    render(<TuiCodeBlock code="fmt.Println()" language="go" />);
    expect(screen.getByText("go")).toBeTruthy();
  });

  it("renders copy button", () => {
    render(<TuiCodeBlock code="hello" />);
    expect(screen.getByRole("button", { name: /复制/i })).toBeTruthy();
  });

  it("copies code to clipboard on click", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });
    render(<TuiCodeBlock code="hello world" />);
    screen.getByRole("button", { name: /复制/i }).click();
    expect(writeText).toHaveBeenCalledWith("hello world");
  });
});
