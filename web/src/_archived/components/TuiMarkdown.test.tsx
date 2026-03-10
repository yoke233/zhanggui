/** @vitest-environment jsdom */
import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { TuiMarkdown } from "./TuiMarkdown";

describe("TuiMarkdown", () => {
  afterEach(() => cleanup());

  it("renders plain text as paragraph", () => {
    render(<TuiMarkdown content="hello world" />);
    expect(screen.getByText("hello world")).toBeTruthy();
  });

  it("renders inline code", () => {
    render(<TuiMarkdown content="use `npm install` to install" />);
    expect(screen.getByText("npm install")).toBeTruthy();
    expect(screen.getByText("npm install").tagName).toBe("CODE");
  });

  it("renders bold text", () => {
    render(<TuiMarkdown content="this is **bold** text" />);
    expect(screen.getByText("bold").tagName).toBe("STRONG");
  });

  it("renders code blocks with syntax highlighter", () => {
    const content = "```javascript\nconst x = 1;\n```";
    render(<TuiMarkdown content={content} />);
    expect(screen.getByText("javascript")).toBeTruthy();
  });

  it("renders headings", () => {
    render(<TuiMarkdown content="## Hello" />);
    const heading = screen.getByText("Hello");
    expect(heading.tagName).toBe("H2");
  });

  it("renders unordered lists", () => {
    render(<TuiMarkdown content={"- item 1\n- item 2"} />);
    expect(screen.getByText("item 1")).toBeTruthy();
    expect(screen.getByText("item 2")).toBeTruthy();
  });

  it("renders links", () => {
    render(<TuiMarkdown content="[click](https://example.com)" />);
    const link = screen.getByText("click");
    expect(link.tagName).toBe("A");
    expect(link.getAttribute("href")).toBe("https://example.com");
  });
});
