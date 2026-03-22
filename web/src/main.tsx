import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import { RootErrorBoundary } from "./components/errors/RootErrorBoundary";
import "./i18n";
import "./index.css";
import { useSettingsStore } from "./stores/settingsStore";
import { applyVscodeTheme, clearVscodeTheme } from "./lib/vscodeTheme";

// Sync settings store → <html> data attributes + custom theme CSS vars
const applySettings = (state: ReturnType<typeof useSettingsStore.getState>) => {
  const { theme, fontSize, userThemeCache, bundledThemeCache } = state;

  document.documentElement.dataset.fontSize = fontSize;

  // Check user-imported themes and bundled theme cache
  const vscTheme = userThemeCache[theme] ?? bundledThemeCache[theme];
  if (vscTheme) {
    document.documentElement.dataset.theme = "custom";
    applyVscodeTheme(vscTheme);
  } else {
    clearVscodeTheme();
    document.documentElement.dataset.theme = theme;
  }
};

applySettings(useSettingsStore.getState());
useSettingsStore.subscribe(applySettings);

// If active theme is not a CSS-only builtin, try to restore it:
// 1. Try bundled manifest first
// 2. Then try user themes from backend API
{
  const { theme } = useSettingsStore.getState();
  const isBuiltin = ["slate", "ocean", "forest", "amber"].includes(theme);
  if (!isBuiltin && theme) {
    const store = useSettingsStore.getState();
    // Try bundled first, then user themes
    void store.loadBundledManifest().then(() => {
      const { bundledThemes } = useSettingsStore.getState();
      if (bundledThemes.some((t) => t.id === theme)) {
        void useSettingsStore.getState().activateBundledTheme(theme);
      } else {
        // Must be a user theme — load from backend
        void useSettingsStore.getState().loadUserThemes().then(() => {
          void useSettingsStore.getState().activateUserTheme(theme);
        });
      }
    });
  }
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <RootErrorBoundary>
      <App />
    </RootErrorBoundary>
  </React.StrictMode>,
);
