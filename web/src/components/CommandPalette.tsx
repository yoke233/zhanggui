import type { AvailableCommand } from "../types/ws";

interface CommandPaletteProps {
  commands: AvailableCommand[];
  selectedIndex: number;
  onHover: (index: number) => void;
  onSelect: (command: AvailableCommand) => void;
}

const CommandPalette = ({
  commands,
  selectedIndex,
  onHover,
  onSelect,
}: CommandPaletteProps) => {
  if (commands.length === 0) {
    return null;
  }

  return (
    <div className="mt-2 rounded-lg border border-slate-200 bg-white shadow-sm">
      <div className="border-b border-slate-100 px-3 py-2 text-xs font-medium text-slate-500">
        可用命令
      </div>
      <ul className="max-h-64 overflow-y-auto py-1">
        {commands.map((command, index) => {
          const hint = command.input?.hint?.trim();
          const isActive = index === selectedIndex;
          return (
            <li key={command.name}>
              <button
                type="button"
                className={`flex w-full flex-col items-start gap-1 px-3 py-2 text-left ${
                  isActive
                    ? "bg-sky-50 text-sky-900"
                    : "text-slate-700 hover:bg-slate-50"
                }`}
                onMouseEnter={() => {
                  onHover(index);
                }}
                onClick={() => {
                  onSelect(command);
                }}
              >
                <div className="flex items-center gap-2">
                  <span className="font-semibold">/{command.name}</span>
                  {hint ? (
                    <span className="rounded bg-slate-100 px-1.5 py-0.5 text-[11px] text-slate-500">
                      {hint}
                    </span>
                  ) : null}
                </div>
                {command.description ? (
                  <span className="text-xs text-slate-500">
                    {command.description}
                  </span>
                ) : null}
              </button>
            </li>
          );
        })}
      </ul>
    </div>
  );
};

export default CommandPalette;
