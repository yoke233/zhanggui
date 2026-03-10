/** @vitest-environment jsdom */

import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import GitHubStatusBadge from "./GitHubStatusBadge";

describe("GitHubStatusBadge", () => {
  it("renders connected/degraded/disconnected states", () => {
    const { rerender } = render(<GitHubStatusBadge status="connected" />);
    expect(screen.getByTestId("github-status-badge").textContent).toContain("Connected");

    rerender(<GitHubStatusBadge status="degraded" />);
    expect(screen.getByTestId("github-status-badge").textContent).toContain("Degraded");

    rerender(<GitHubStatusBadge status="disconnected" />);
    expect(screen.getByTestId("github-status-badge").textContent).toContain("Disconnected");
  });
});
