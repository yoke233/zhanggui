/** @vitest-environment jsdom */
import { describe, it, expect, vi, beforeEach } from "vitest";

describe("settingsStore", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.resetModules();
  });

  it("defaults to slate theme and md font size", async () => {
    const { useSettingsStore } = await import("./settingsStore");
    const state = useSettingsStore.getState();
    expect(state.theme).toBe("slate");
    expect(state.fontSize).toBe("md");
  });

  it("setTheme updates theme and persists to localStorage", async () => {
    const { useSettingsStore } = await import("./settingsStore");
    useSettingsStore.getState().setTheme("ocean");
    expect(useSettingsStore.getState().theme).toBe("ocean");
    const saved = JSON.parse(localStorage.getItem("ai-workflow-settings") ?? "{}");
    expect(saved.theme).toBe("ocean");
  });

  it("setFontSize updates fontSize and persists to localStorage", async () => {
    const { useSettingsStore } = await import("./settingsStore");
    useSettingsStore.getState().setFontSize("lg");
    expect(useSettingsStore.getState().fontSize).toBe("lg");
    const saved = JSON.parse(localStorage.getItem("ai-workflow-settings") ?? "{}");
    expect(saved.fontSize).toBe("lg");
  });

  it("reads persisted values from localStorage on init", async () => {
    localStorage.setItem("ai-workflow-settings", JSON.stringify({ theme: "amber", fontSize: "sm" }));
    const { useSettingsStore } = await import("./settingsStore");
    expect(useSettingsStore.getState().theme).toBe("amber");
    expect(useSettingsStore.getState().fontSize).toBe("sm");
  });
});
