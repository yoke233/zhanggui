/** @vitest-environment jsdom */
import { describe, it, expect, afterEach, vi } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { SettingsPanel } from "./SettingsPanel";
import { useSettingsStore } from "../stores/settingsStore";

describe("SettingsPanel", () => {
  afterEach(() => {
    cleanup();
    localStorage.clear();
    useSettingsStore.setState({ theme: "slate", fontSize: "md" });
  });

  it("renders theme swatches and font size buttons", () => {
    render(<SettingsPanel open onClose={() => {}} />);
    expect(screen.getByTitle("slate")).toBeTruthy();
    expect(screen.getByTitle("ocean")).toBeTruthy();
    expect(screen.getByTitle("forest")).toBeTruthy();
    expect(screen.getByTitle("amber")).toBeTruthy();
    expect(screen.getByText("S")).toBeTruthy();
    expect(screen.getByText("M")).toBeTruthy();
    expect(screen.getByText("L")).toBeTruthy();
  });

  it("clicking a theme swatch updates the store", () => {
    render(<SettingsPanel open onClose={() => {}} />);
    fireEvent.click(screen.getByTitle("ocean"));
    expect(useSettingsStore.getState().theme).toBe("ocean");
  });

  it("clicking a font size button updates the store", () => {
    render(<SettingsPanel open onClose={() => {}} />);
    fireEvent.click(screen.getByText("L"));
    expect(useSettingsStore.getState().fontSize).toBe("lg");
  });

  it("calls onClose when close button clicked", () => {
    const onClose = vi.fn();
    render(<SettingsPanel open onClose={onClose} />);
    fireEvent.click(screen.getByLabelText("关闭设置"));
    expect(onClose).toHaveBeenCalled();
  });

  it("renders nothing when open=false", () => {
    const { container } = render(<SettingsPanel open={false} onClose={() => {}} />);
    expect(container.firstChild).toBeNull();
  });
});
