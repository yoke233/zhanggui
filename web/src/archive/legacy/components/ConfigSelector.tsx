import type { ConfigOption } from "@/types/ws";

interface ConfigSelectorProps {
  options: ConfigOption[];
  updatingConfigId: string | null;
  onChange: (configId: string, value: string) => void;
}

const ConfigSelector = ({
  options,
  updatingConfigId,
  onChange,
}: ConfigSelectorProps) => {
  if (options.length === 0) {
    return null;
  }

  return (
    <div className="mt-3 flex flex-wrap gap-3 rounded-lg border border-slate-200 bg-slate-50 p-3">
      {options.map((option) => (
        <label
          key={option.id}
          className="flex min-w-[12rem] flex-col gap-1 text-xs text-slate-600"
        >
          <span className="font-semibold text-slate-700">{option.name}</span>
          <select
            className="rounded-md border border-slate-300 bg-white px-2 py-1.5 text-sm text-slate-900"
            value={option.currentValue}
            disabled={updatingConfigId === option.id}
            onChange={(event) => {
              onChange(option.id, event.target.value);
            }}
          >
            {option.options.map((item) => (
              <option key={`${option.id}-${item.value}`} value={item.value}>
                {item.name}
              </option>
            ))}
          </select>
          {option.description ? (
            <span className="text-[11px] text-slate-500">
              {option.description}
            </span>
          ) : null}
        </label>
      ))}
    </div>
  );
};

export default ConfigSelector;
