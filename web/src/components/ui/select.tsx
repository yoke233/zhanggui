import { useState, useRef, useEffect, type ReactNode, Children, isValidElement } from "react";
import { cn } from "@/lib/utils";
import { ChevronDown, Check } from "lucide-react";

interface OptionData {
  value: string;
  label: ReactNode;
  disabled?: boolean;
}

interface SelectProps {
  value?: string;
  onValueChange?: (value: string) => void;
  children?: ReactNode;
  className?: string;
  disabled?: boolean;
  placeholder?: string;
}

function extractOptions(children: ReactNode): OptionData[] {
  const options: OptionData[] = [];
  Children.forEach(children, (child) => {
    if (
      isValidElement<{ value?: string; children?: ReactNode; disabled?: boolean }>(child) &&
      (child.type === "option" || child.type === SelectItem)
    ) {
      options.push({
        value: String(child.props.value ?? ""),
        label: child.props.children,
        disabled: child.props.disabled,
      });
    }
  });
  return options;
}

export function Select({ value, onValueChange, children, className, disabled, placeholder }: SelectProps) {
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  const options = extractOptions(children);
  const selected = options.find((o) => o.value === value);
  const displayLabel = selected?.label ?? placeholder ?? "\u00A0";

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (e: PointerEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("pointerdown", onPointerDown);
    return () => document.removeEventListener("pointerdown", onPointerDown);
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open]);

  return (
    <div ref={containerRef} className="relative">
      <button
        type="button"
        disabled={disabled}
        className={cn(
          "flex h-10 w-full items-center justify-between gap-2 rounded-[10px] border border-slate-200 bg-white px-3 py-2 text-sm text-slate-950 transition",
          "hover:border-slate-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-slate-400",
          "disabled:cursor-not-allowed disabled:opacity-50",
          className,
        )}
        onClick={() => setOpen((prev) => !prev)}
      >
        <span className="truncate text-left">{displayLabel}</span>
        <ChevronDown className={cn("h-4 w-4 shrink-0 text-slate-400 transition-transform", open && "rotate-180")} />
      </button>

      {open && (
        <div className="absolute left-0 top-full z-50 mt-1 w-full min-w-[8rem] overflow-hidden rounded-xl border border-slate-200/80 bg-white py-1 shadow-[0_8px_30px_rgba(0,0,0,0.08),0_2px_8px_rgba(0,0,0,0.04)] animate-select-in">
          {options.map((opt) => (
            <button
              key={opt.value}
              type="button"
              disabled={opt.disabled}
              className={cn(
                "flex w-full items-center gap-2 px-3 py-2 text-sm transition-colors",
                "hover:bg-slate-50",
                value === opt.value ? "font-medium text-slate-950" : "text-slate-600",
                opt.disabled && "cursor-not-allowed opacity-50",
              )}
              onClick={() => {
                onValueChange?.(opt.value);
                setOpen(false);
              }}
            >
              <Check className={cn("h-3.5 w-3.5 shrink-0", value === opt.value ? "opacity-100" : "opacity-0")} />
              <span className="truncate">{opt.label}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

export function SelectItem(_props: { value: string; children?: ReactNode; disabled?: boolean }) {
  return null;
}

SelectItem.displayName = "SelectItem";
