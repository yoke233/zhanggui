import { create } from "zustand";

export type Theme = "slate" | "ocean" | "forest" | "amber";
export type FontSize = "sm" | "md" | "lg";

const STORAGE_KEY = "ai-workflow-settings";

const loadFromStorage = (): { theme: Theme; fontSize: FontSize } => {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return { theme: "slate", fontSize: "md" };
    const parsed = JSON.parse(raw) as Partial<{ theme: Theme; fontSize: FontSize }>;
    return {
      theme: (["slate", "ocean", "forest", "amber"] as Theme[]).includes(parsed.theme as Theme)
        ? (parsed.theme as Theme)
        : "slate",
      fontSize: (["sm", "md", "lg"] as FontSize[]).includes(parsed.fontSize as FontSize)
        ? (parsed.fontSize as FontSize)
        : "md",
    };
  } catch {
    return { theme: "slate", fontSize: "md" };
  }
};

const saveToStorage = (theme: Theme, fontSize: FontSize) => {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ theme, fontSize }));
  } catch {
    // ignore
  }
};

interface SettingsState {
  theme: Theme;
  fontSize: FontSize;
  setTheme: (theme: Theme) => void;
  setFontSize: (size: FontSize) => void;
}

export const useSettingsStore = create<SettingsState>((set, get) => ({
  ...loadFromStorage(),
  setTheme: (theme) => {
    set({ theme });
    saveToStorage(theme, get().fontSize);
  },
  setFontSize: (fontSize) => {
    set({ fontSize });
    saveToStorage(get().theme, fontSize);
  },
}));
