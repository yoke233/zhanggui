import { useRef, useEffect, useCallback } from "react";
import { useSettingsStore, type Theme, type FontSize } from "../stores/settingsStore";

const THEMES: { id: Theme; label: string; color: string }[] = [
  { id: "slate",  label: "Slate",  color: "#475569" },
  { id: "ocean",  label: "Ocean",  color: "#0284c7" },
  { id: "forest", label: "Forest", color: "#059669" },
  { id: "amber",  label: "Amber",  color: "#d97706" },
];

const FONT_SIZES: { id: FontSize; label: string }[] = [
  { id: "sm", label: "S" },
  { id: "md", label: "M" },
  { id: "lg", label: "L" },
];

interface SettingsPanelProps {
  open: boolean;
  onClose: () => void;
}

export function SettingsPanel({ open, onClose }: SettingsPanelProps) {
  const theme = useSettingsStore((s) => s.theme);
  const fontSize = useSettingsStore((s) => s.fontSize);
  const setTheme = useSettingsStore((s) => s.setTheme);
  const setFontSize = useSettingsStore((s) => s.setFontSize);
  const panelRef = useRef<HTMLDivElement>(null);

  const handleOutsideClick = useCallback(
    (e: MouseEvent) => {
      if (panelRef.current && !panelRef.current.contains(e.target as Node)) {
        onClose();
      }
    },
    [onClose],
  );

  useEffect(() => {
    if (!open) return;
    document.addEventListener("mousedown", handleOutsideClick);
    return () => document.removeEventListener("mousedown", handleOutsideClick);
  }, [open, handleOutsideClick]);

  if (!open) return null;

  return (
    <div
      ref={panelRef}
      className="absolute right-0 top-full z-50 mt-2 w-72 rounded-xl border border-slate-200 bg-white p-4 shadow-xl"
    >
      <div className="flex items-center justify-between mb-3">
        <span className="text-sm font-semibold text-slate-700">外观设置</span>
        <button
          type="button"
          aria-label="关闭设置"
          onClick={onClose}
          className="text-slate-400 hover:text-slate-600"
        >
          ✕
        </button>
      </div>

      {/* Theme swatches */}
      <p className="mb-2 text-xs font-medium text-slate-500">主题颜色</p>
      <div className="mb-4 flex gap-3">
        {THEMES.map((t) => (
          <button
            key={t.id}
            type="button"
            title={t.id}
            aria-label={t.label}
            onClick={() => setTheme(t.id)}
            className="flex flex-col items-center gap-1"
          >
            <span
              className="block h-7 w-7 rounded-full border-2 transition-transform hover:scale-110"
              style={{
                backgroundColor: t.color,
                borderColor: theme === t.id ? t.color : "transparent",
                outline: theme === t.id ? `2px solid ${t.color}` : "2px solid transparent",
                outlineOffset: "2px",
              }}
            />
            <span className="text-[10px] text-slate-500">{t.label}</span>
          </button>
        ))}
      </div>

      {/* Font size */}
      <p className="mb-2 text-xs font-medium text-slate-500">聊天字号</p>
      <div className="flex gap-2">
        {FONT_SIZES.map((f) => (
          <button
            key={f.id}
            type="button"
            onClick={() => setFontSize(f.id)}
            className={`flex-1 rounded-lg border py-1.5 text-sm font-semibold transition ${
              fontSize === f.id
                ? "accent-bg border-transparent"
                : "border-slate-200 text-slate-600 hover:bg-slate-50"
            }`}
          >
            {f.label}
          </button>
        ))}
      </div>
    </div>
  );
}
