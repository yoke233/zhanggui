import type React from "react";
import { useTranslation } from "react-i18next";
import { ChevronDown, Paperclip, Send, X } from "lucide-react";
import type { ConfigOption, SessionModeState, SlashCommand } from "@/types/apiV2";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import type { SessionRecord } from "./chatTypes";

interface ChatInputBarProps {
  messageInput: string;
  pendingFiles: File[];
  currentSession: SessionRecord | null;
  submitting: boolean;
  draftSessionReady: boolean;
  currentDriverLabel: string;
  currentProjectLabel: string;
  showCommandPalette: boolean;
  availableCommands: SlashCommand[];
  commandFilter: string;
  fileInputRef: React.RefObject<HTMLInputElement>;
  modes: SessionModeState | null;
  configOptions: ConfigOption[];
  onMessageChange: (value: string) => void;
  onPaste: (e: React.ClipboardEvent) => void;
  onKeyDown: (e: React.KeyboardEvent) => void;
  onSend: () => void;
  onCommandSelect: (name: string) => void;
  onRemovePendingFile: (index: number) => void;
  onCommandPaletteClose: () => void;
  onSetMode?: (modeId: string) => void;
  onSetConfigOption?: (configId: string, value: string) => void;
}

export function ChatInputBar(props: ChatInputBarProps) {
  const {
    messageInput,
    pendingFiles,
    currentSession,
    submitting,
    draftSessionReady,
    currentDriverLabel,
    currentProjectLabel,
    showCommandPalette,
    availableCommands,
    commandFilter,
    fileInputRef,
    onMessageChange,
    onPaste,
    onKeyDown,
    onSend,
    onCommandSelect,
    onRemovePendingFile,
    onCommandPaletteClose,
    modes,
    configOptions,
    onSetMode,
    onSetConfigOption,
  } = props;
  const { t } = useTranslation();

  const isDisabled = submitting || currentSession?.status === "running" || (!currentSession && !draftSessionReady);
  const filteredCommands = availableCommands.filter(
    (cmd) => !commandFilter || cmd.name.toLowerCase().includes(commandFilter.toLowerCase()),
  );

  return (
    <div className="space-y-2 border-t px-6 py-4">
      {pendingFiles.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {pendingFiles.map((file, idx) => (
            <Badge key={idx} variant="secondary" className="gap-1 text-xs">
              {file.name}
              <button type="button" onClick={() => onRemovePendingFile(idx)} className="ml-1 hover:text-red-500">
                <X className="h-3 w-3" />
              </button>
            </Badge>
          ))}
        </div>
      )}
      <div className="relative">
        {showCommandPalette && availableCommands.length > 0 && (
          <div className="absolute bottom-full left-0 z-50 mb-2 w-[580px] rounded-xl border bg-popover shadow-lg">
            <div className="border-b px-3 py-1.5">
              <span className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
                {t("chat.commands", { defaultValue: "命令" })}
              </span>
            </div>
            <div className="max-h-52 overflow-y-auto py-1">
              {filteredCommands.map((cmd) => (
                <button
                  key={cmd.name}
                  type="button"
                  className="flex w-full items-baseline gap-0 px-3 py-1.5 text-left transition-colors hover:bg-accent"
                  onClick={() => onCommandSelect(cmd.name)}
                >
                  <span className="w-36 shrink-0 font-mono text-xs font-semibold text-foreground">
                    /{cmd.name}
                  </span>
                  {cmd.description && (
                    <span className="min-w-0 truncate text-xs text-muted-foreground">
                      {cmd.description}
                    </span>
                  )}
                </button>
              ))}
              {filteredCommands.length === 0 && (
                <div className="px-3 py-2 text-xs text-muted-foreground">{t("chat.noCommandsMatch")}</div>
              )}
            </div>
          </div>
        )}
        <div className="flex items-center gap-2.5 rounded-lg border bg-background px-3.5 py-2.5">
          <Input
            placeholder={
              currentSession
                ? t("chat.inputPlaceholderActive")
                : t("chat.inputPlaceholderNew", { driver: currentDriverLabel, project: currentProjectLabel })
            }
            className="h-auto flex-1 border-0 p-0 text-sm shadow-none focus-visible:ring-0"
            value={messageInput}
            disabled={isDisabled}
            onChange={(event) => onMessageChange(event.target.value)}
            onPaste={onPaste}
            onKeyDown={onKeyDown}
            onBlur={() => {
              setTimeout(() => onCommandPaletteClose(), 150);
            }}
          />
          <div className="flex shrink-0 items-center gap-1.5">
            <button
              type="button"
              className="flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:text-foreground disabled:opacity-40"
              disabled={isDisabled}
              onClick={() => fileInputRef.current?.click()}
              title={t("chat.uploadFile")}
            >
              <Paperclip className="h-[18px] w-[18px]" />
            </button>
            <Button
              size="icon"
              className="h-8 w-8"
              disabled={isDisabled}
              onClick={onSend}
            >
              <Send className="h-4 w-4" />
            </Button>
          </div>
        </div>
        <div className="flex items-center justify-between pt-1 text-[11px] text-muted-foreground">
          <div className="flex items-center gap-1.5">
            {currentSession?.project_name && (
              <Badge variant="secondary" className="text-[10px]">
                {currentSession.project_name}
              </Badge>
            )}
            {currentSession?.branch && (
              <Badge variant="outline" className="font-mono text-[10px]">
                {currentSession.branch}
              </Badge>
            )}
            {modes && modes.available_modes.length > 0 ? (
              <>
                {(currentSession?.project_name || currentSession?.branch) && (
                  <span className="mx-0.5 text-border">·</span>
                )}
                {modes.available_modes.map((mode) => (
                  <button
                    key={mode.id}
                    type="button"
                    title={mode.description || mode.name}
                    className={cn(
                      "inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[11px] transition-colors",
                      modes.current_mode_id === mode.id
                        ? "bg-primary/10 font-medium text-primary"
                        : "text-muted-foreground hover:bg-muted hover:text-foreground",
                    )}
                    onClick={() => onSetMode?.(mode.id)}
                  >
                    {mode.name}
                    {modes.current_mode_id === mode.id && (
                      <ChevronDown className="h-2.5 w-2.5" />
                    )}
                  </button>
                ))}
              </>
            ) : null}
            {configOptions.length > 0 ? (
              <>
                {(currentSession?.project_name || currentSession?.branch || (modes && modes.available_modes.length > 0)) && (
                  <span className="mx-0.5 text-border">·</span>
                )}
                {configOptions.map((opt) => {
                  return (
                    <span key={opt.id} className="inline-flex items-center gap-0.5 text-[11px]">
                      <span className="text-muted-foreground">{opt.name}:</span>
                      <select
                        className="cursor-pointer appearance-none rounded bg-transparent px-0.5 py-0.5 text-[11px] font-medium text-foreground outline-none hover:bg-muted"
                        value={opt.current_value}
                        title={opt.description || opt.name}
                        onChange={(e) => onSetConfigOption?.(opt.id, e.target.value)}
                      >
                        {opt.options.map((v) => (
                          <option key={v.value} value={v.value}>
                            {v.name}
                          </option>
                        ))}
                      </select>
                    </span>
                  );
                })}
              </>
            ) : null}
          </div>
          <span>{t("chat.inputHint")}</span>
        </div>
      </div>
    </div>
  );
}
