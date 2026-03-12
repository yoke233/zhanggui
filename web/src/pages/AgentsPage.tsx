import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Plus, Settings2, Bot, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { SandboxSupportPanel } from "@/components/SandboxSupportPanel";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { getErrorMessage } from "@/lib/v2Workbench";
import { CreateDriverDialog } from "@/components/agents/CreateDriverDialog";
import { CreateProfileDialog } from "@/components/agents/CreateProfileDialog";
import type { AgentDriver, AgentProfile } from "@/types/apiV2";
import type { SandboxSupportResponse } from "@/types/system";

const roleBadgeVariant: Record<string, "info" | "warning" | "default" | "secondary"> = {
  worker: "info",
  gate: "warning",
  lead: "default",
  support: "secondary",
};

const ALL_CAPS = ["fs_read", "fs_write", "terminal"] as const;

export function AgentsPage() {
  const { t } = useTranslation();
  const { apiClient } = useWorkbench();
  const [drivers, setDrivers] = useState<AgentDriver[]>([]);
  const [profiles, setProfiles] = useState<AgentProfile[]>([]);
  const [sandboxSupport, setSandboxSupport] = useState<SandboxSupportResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [sandboxLoading, setSandboxLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [sandboxError, setSandboxError] = useState<string | null>(null);
  const [driverDialogOpen, setDriverDialogOpen] = useState(false);
  const [profileDialogOpen, setProfileDialogOpen] = useState(false);

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      const [driverResp, profileResp] = await Promise.all([
        apiClient.listDrivers(),
        apiClient.listProfiles(),
      ]);
      setDrivers(driverResp);
      setProfiles(profileResp);
    } catch (e) {
      setError(getErrorMessage(e));
    } finally {
      setLoading(false);
    }
  };

  const loadSandboxSupport = async () => {
    setSandboxLoading(true);
    setSandboxError(null);
    try {
      setSandboxSupport(await apiClient.getSandboxSupport());
    } catch (e) {
      setSandboxError(getErrorMessage(e));
    } finally {
      setSandboxLoading(false);
    }
  };

  useEffect(() => {
    void Promise.all([load(), loadSandboxSupport()]);
  }, []);

  return (
    <div className="flex-1 space-y-6 p-8">
      <div className="flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-bold tracking-tight">{t("agents.title")}</h1>
            {loading ? <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /> : null}
          </div>
          <p className="text-sm text-muted-foreground">{t("agents.subtitle")}</p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" onClick={() => setDriverDialogOpen(true)}>
            <Settings2 className="mr-2 h-4 w-4" />
            {t("agents.newDriver")}
          </Button>
          <Button onClick={() => setProfileDialogOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            {t("agents.newProfile")}
          </Button>
        </div>
      </div>

      {error ? <p className="rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p> : null}

      <SandboxSupportPanel
        report={sandboxSupport}
        loading={sandboxLoading}
        error={sandboxError}
        onRefresh={() => void loadSandboxSupport()}
      />

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Bot className="h-5 w-5" />
            {t("agents.drivers")}
            <Badge variant="secondary" className="ml-1">{drivers.length}</Badge>
          </CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("agents.driverName")}</TableHead>
                <TableHead>{t("agents.launchCommand")}</TableHead>
                <TableHead>{t("agents.maxCapabilities")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {drivers.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={3} className="text-center text-muted-foreground">{t("agents.noDrivers")}</TableCell>
                </TableRow>
              ) : (
                drivers.map((driver) => (
                  <TableRow key={driver.id}>
                    <TableCell className="font-medium">{driver.id}</TableCell>
                    <TableCell>
                      <code className="rounded bg-muted px-1.5 py-0.5 text-xs font-mono">
                        {driver.launch_command} {(driver.launch_args ?? []).join(" ")}
                      </code>
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-wrap gap-1">
                        {ALL_CAPS.filter((cap) => driver.capabilities_max[cap]).map((cap) => (
                          <Badge key={cap} variant="outline" className="text-xs">{cap}</Badge>
                        ))}
                      </div>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            {t("agents.profiles")}
            <Badge variant="secondary" className="ml-1">{profiles.length}</Badge>
          </CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("agents.driverName")}</TableHead>
                <TableHead>{t("agents.role")}</TableHead>
                <TableHead>{t("agents.boundDriver")}</TableHead>
                <TableHead>{t("agents.capabilityTags")}</TableHead>
                <TableHead>{t("agents.actionPermissions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {profiles.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className="text-center text-muted-foreground">{t("agents.noProfiles")}</TableCell>
                </TableRow>
              ) : (
                profiles.map((profile) => (
                  <TableRow key={profile.id}>
                    <TableCell className="font-medium">{profile.name || profile.id}</TableCell>
                    <TableCell>
                      <Badge variant={roleBadgeVariant[profile.role] ?? "secondary"}>{profile.role}</Badge>
                    </TableCell>
                    <TableCell className="text-muted-foreground">{profile.driver_id}</TableCell>
                    <TableCell>
                      <div className="flex flex-wrap gap-1">
                        {(profile.capabilities ?? []).map((cap) => (
                          <Badge key={cap} variant="outline" className="text-xs">{cap}</Badge>
                        ))}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-wrap gap-1">
                        {(profile.actions_allowed ?? []).map((action) => (
                          <Badge key={action} variant="secondary" className="text-xs">{action}</Badge>
                        ))}
                      </div>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <CreateDriverDialog
        open={driverDialogOpen}
        onClose={() => setDriverDialogOpen(false)}
        onCreate={async (payload) => {
          await apiClient.createDriver(payload);
          await load();
        }}
      />

      <CreateProfileDialog
        open={profileDialogOpen}
        drivers={drivers}
        onClose={() => setProfileDialogOpen(false)}
        onCreate={async (payload) => {
          await apiClient.createProfile(payload);
          await load();
        }}
      />
    </div>
  );
}
