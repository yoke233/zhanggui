import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import "./index.css";
import { useSettingsStore } from "./stores/settingsStore";

// Sync settings store → <html> data attributes immediately and on every change
const applySettings = (state: { theme: string; fontSize: string }) => {
  document.documentElement.dataset.theme = state.theme;
  document.documentElement.dataset.fontSize = state.fontSize;
};

applySettings(useSettingsStore.getState());
useSettingsStore.subscribe(applySettings);

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
