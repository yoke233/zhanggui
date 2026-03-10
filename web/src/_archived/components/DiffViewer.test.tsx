/** @vitest-environment jsdom */

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import DiffViewer from "./DiffViewer";

const SAMPLE_DIFF = `diff --git a/src/main.ts b/src/main.ts
index 1111111..2222222 100644
--- a/src/main.ts
+++ b/src/main.ts
@@ -1,2 +1,2 @@
-console.log("old line");
+console.log("new line");
 console.log("keep line");
`;

describe("DiffViewer", () => {
  afterEach(() => {
    cleanup();
  });

  it("能渲染 unified diff 内容", () => {
    render(<DiffViewer diff={SAMPLE_DIFF} filePath="src/main.ts" />);

    expect(screen.getAllByText("src/main.ts").length).toBeGreaterThan(0);
    expect(screen.getByText("CHANGED")).toBeTruthy();
    expect(document.querySelector(".d2h-wrapper")).toBeTruthy();
  });

  it("空 diff 时显示兜底提示", () => {
    render(<DiffViewer diff="" filePath="src/main.ts" />);
    expect(screen.getByText("暂无 diff 内容。")).toBeTruthy();
  });
});
