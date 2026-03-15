import { useState } from "react";
import type { MutableRefObject } from "react";
import { useTranslation } from "react-i18next";
import { Bot, Check, Loader2, Plus, User, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { AgentProfile, ThreadAgentSession, ThreadParticipant } from "@/types/apiV2";

type AgentSessionWithProfileID = ThreadAgentSession & { agent_profile_id: string };

interface ThreadAgentsPanelProps {
  inviteableProfiles: AgentProfile[];
  selectedInviteIDs: Set<string>;
  invitingAgent: boolean;
  onToggleInviteSelection: (profileID: string) => void;
  onInviteAgent: () => void;
  onClearInviteSelection?: () => void;
  agentSessionsWithProfileID: AgentSessionWithProfileID[];
  profileByID: Map<string, AgentProfile>;
  highlightedAgentProfileID: string | null;
  agentCardRefs: MutableRefObject<Record<string, HTMLDivElement | null>>;
  removingAgentID: number | null;
  onRemoveAgent: (agentSessionID: number) => void;
  participants: ThreadParticipant[];
  agentStatusColor: (status: string) => string;
}

export function ThreadAgentsPanel({
  inviteableProfiles,
  selectedInviteIDs,
  invitingAgent,
  onToggleInviteSelection,
  onInviteAgent,
  onClearInviteSelection,
  agentSessionsWithProfileID,
  profileByID,
  highlightedAgentProfileID,
  agentCardRefs,
  removingAgentID,
  onRemoveAgent,
  participants,
  agentStatusColor,
}: ThreadAgentsPanelProps) {
  const { t } = useTranslation();
  const [showInvitePicker, setShowInvitePicker] = useState(false);

  const handleInvite = () => {
    onInviteAgent();
    setShowInvitePicker(false);
  };

  const handleCancel = () => {
    setShowInvitePicker(false);
    onClearInviteSelection?.();
  };

  return (
    <div className="space-y-4 p-4">
      {/* Active agents in conversation */}
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
            {t("threads.activeAgents", "Active Agents")} ({agentSessionsWithProfileID.length})
          </h3>
          {inviteableProfiles.length > 0 && !showInvitePicker && (
            <Button
              size="sm"
              variant="outline"
              className="h-7 text-xs"
              onClick={() => setShowInvitePicker(true)}
            >
              <Plus className="mr-1 h-3.5 w-3.5" />
              {t("threads.addAgent", "Add")}
            </Button>
          )}
        </div>

        {agentSessionsWithProfileID.length === 0 && !showInvitePicker ? (
          <div className="rounded-xl border border-dashed py-8 text-center">
            <Bot className="mx-auto h-8 w-8 text-muted-foreground/30" />
            <p className="mt-2 text-xs text-muted-foreground">{t("threads.noAgents", "No agents joined yet")}</p>
            {inviteableProfiles.length > 0 && (
              <Button
                size="sm"
                variant="outline"
                className="mt-3 h-7 text-xs"
                onClick={() => setShowInvitePicker(true)}
              >
                <Plus className="mr-1 h-3.5 w-3.5" />
                {t("threads.inviteAgent", "Invite Agent")}
              </Button>
            )}
          </div>
        ) : (
          <div className="space-y-2">
            {agentSessionsWithProfileID.map((session) => {
              const profile = profileByID.get(session.agent_profile_id);
              return (
                <div
                  key={session.id}
                  ref={(node) => {
                    agentCardRefs.current[session.agent_profile_id] = node;
                  }}
                  data-testid={`agent-card-${session.agent_profile_id}`}
                  className={cn(
                    "rounded-xl border p-3 transition-all",
                    highlightedAgentProfileID === session.agent_profile_id
                      ? "border-blue-300 bg-blue-50 shadow-md"
                      : "border-border/60 bg-background hover:border-border",
                  )}
                >
                  <div className="flex items-start gap-2.5">
                    <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-emerald-100 text-emerald-700">
                      <Bot className="h-4 w-4" />
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-1.5">
                        <span className="truncate text-sm font-medium">{profile?.name ?? session.agent_profile_id}</span>
                        <span className="flex items-center gap-1 rounded-full bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground">
                          <span className={cn("h-1.5 w-1.5 rounded-full", agentStatusColor(session.status ?? "unknown"))} />
                          {session.status ?? "unknown"}
                        </span>
                      </div>
                      {profile?.name && <p className="mt-0.5 truncate text-[11px] text-muted-foreground">@{session.agent_profile_id}</p>}
                      <div className="mt-1.5 flex items-center gap-3 text-[11px] text-muted-foreground">
                        <span>{t("threads.turns", "Turns")}: {session.turn_count ?? 0}</span>
                        <span>{(((session.total_input_tokens ?? 0) + (session.total_output_tokens ?? 0)) / 1000).toFixed(1)}k tokens</span>
                      </div>
                    </div>
                    <button
                      type="button"
                      className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
                      onClick={() => onRemoveAgent(session.id)}
                      disabled={removingAgentID === session.id}
                      aria-label={t("threads.removeAgentAria", { defaultValue: "Remove {{agent}}", agent: session.agent_profile_id })}
                    >
                      {removingAgentID === session.id ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <X className="h-3.5 w-3.5" />}
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Invite picker (shown on demand) */}
      {showInvitePicker && (
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              {t("threads.selectAgents", "Select Agents")}
            </h3>
            <div className="flex items-center gap-1.5">
              {selectedInviteIDs.size > 0 && (
                <Button size="sm" className="h-7 text-xs" onClick={handleInvite} disabled={invitingAgent}>
                  {invitingAgent ? <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" /> : <Check className="mr-1 h-3.5 w-3.5" />}
                  {t("threads.inviteSelected", "Add")} ({selectedInviteIDs.size})
                </Button>
              )}
              <Button size="sm" variant="ghost" className="h-7 text-xs" onClick={handleCancel} disabled={invitingAgent}>
                <X className="mr-1 h-3.5 w-3.5" />
                {t("common.cancel", "Cancel")}
              </Button>
            </div>
          </div>
          {inviteableProfiles.length === 0 ? (
            <p className="rounded-lg border border-dashed px-3 py-3 text-center text-[11px] text-muted-foreground">
              {t("threads.noInviteableAgents", "All available agents have been invited")}
            </p>
          ) : (
            <div className="space-y-1.5">
              {inviteableProfiles.map((profile) => {
                const isSelected = selectedInviteIDs.has(profile.id);
                return (
                  <button
                    key={profile.id}
                    type="button"
                    className={cn(
                      "flex w-full items-start gap-2.5 rounded-lg border p-2.5 text-left transition-all",
                      isSelected
                        ? "border-blue-300 bg-blue-50 shadow-sm"
                        : "border-border/60 bg-background hover:border-border hover:bg-muted/30",
                      invitingAgent && "pointer-events-none opacity-60",
                    )}
                    onClick={() => onToggleInviteSelection(profile.id)}
                    disabled={invitingAgent}
                  >
                    <div
                      className={cn(
                        "mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center rounded border transition-colors",
                        isSelected ? "border-blue-500 bg-blue-500 text-white" : "border-slate-300 bg-white",
                      )}
                    >
                      {isSelected && <Check className="h-3 w-3" />}
                    </div>
                    <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-emerald-100 text-emerald-700">
                      <Bot className="h-3.5 w-3.5" />
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-1.5">
                        <span className="truncate text-xs font-medium">{profile.name ?? profile.id}</span>
                        <Badge variant="outline" className="shrink-0 text-[9px]">{profile.role}</Badge>
                      </div>
                      {profile.name && <p className="mt-0.5 truncate text-[11px] text-muted-foreground">@{profile.id}</p>}
                      <p className="mt-0.5 truncate text-[10px] text-muted-foreground">
                        {t("threads.driver", "Driver")}: {profile.driver_id ?? profile.driver?.launch_command ?? "-"}
                        {profile.capabilities && profile.capabilities.length > 0 && (
                          <> | {profile.capabilities.slice(0, 3).join(", ")}{profile.capabilities.length > 3 ? "..." : ""}</>
                        )}
                      </p>
                    </div>
                  </button>
                );
              })}
            </div>
          )}
        </div>
      )}

      {/* Participants */}
      {participants.length > 0 && (
        <div className="space-y-2">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
            {t("threads.participants", "Participants")} ({participants.length})
          </h3>
          <div className="space-y-1.5">
            {participants.map((participant) => (
              <div key={participant.id} className="flex items-center gap-2 rounded-lg px-2 py-1.5 text-sm">
                <div className="flex h-6 w-6 items-center justify-center rounded-full bg-slate-100 text-slate-600">
                  <User className="h-3 w-3" />
                </div>
                <span className="truncate text-xs">{participant.user_id}</span>
                <Badge variant="outline" className="ml-auto text-[10px]">{participant.role}</Badge>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
