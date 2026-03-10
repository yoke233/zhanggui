import { useState } from "react";
import { Plus, Settings2, Bot, X, Check } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import {
  Dialog, DialogHeader, DialogTitle, DialogDescription, DialogBody, DialogFooter,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";

interface Driver {
  id: string;
  name: string;
  command: string;
  capabilitiesMax: string[];
  profileCount: number;
}

interface Profile {
  id: string;
  name: string;
  role: string;
  driver: string;
  capabilities: string[];
  actions: string[];
  maxTurns: number;
}

const roleBadgeVariant: Record<string, "info" | "warning" | "default" | "secondary"> = {
  worker: "info",
  gate: "warning",
  lead: "default",
  support: "secondary",
};

const ALL_CAPS = ["fs_read", "fs_write", "terminal"];

export function AgentsPage() {
  const [drivers] = useState<Driver[]>([
    { id: "claude", name: "claude", command: "npx -y @anthropic-ai/claude-code-acp", capabilitiesMax: ["fs_read", "fs_write", "terminal"], profileCount: 3 },
    { id: "codex", name: "codex", command: "npx -y @anthropic/codex-acp", capabilitiesMax: ["fs_read", "fs_write"], profileCount: 2 },
  ]);

  const [profiles] = useState<Profile[]>([
    { id: "claude-worker", name: "claude-worker", role: "worker", driver: "claude", capabilities: ["backend", "frontend"], actions: ["implement", "test"], maxTurns: 12 },
    { id: "claude-reviewer", name: "claude-reviewer", role: "gate", driver: "claude", capabilities: ["backend", "frontend"], actions: ["review"], maxTurns: 5 },
    { id: "claude-lead", name: "claude-lead", role: "lead", driver: "claude", capabilities: ["backend", "frontend", "infra"], actions: ["plan", "delegate", "review"], maxTurns: 20 },
    { id: "codex-worker", name: "codex-worker", role: "worker", driver: "codex", capabilities: ["backend"], actions: ["implement"], maxTurns: 8 },
    { id: "codex-reviewer", name: "codex-reviewer", role: "gate", driver: "codex", capabilities: ["backend"], actions: ["review"], maxTurns: 5 },
  ]);

  /* ── Dialog state ── */
  const [driverDialogOpen, setDriverDialogOpen] = useState(false);
  const [profileDialogOpen, setProfileDialogOpen] = useState(false);

  // Driver form
  const [driverName, setDriverName] = useState("");
  const [driverCmd, setDriverCmd] = useState("");
  const [driverArgs, setDriverArgs] = useState("");
  const [driverCaps, setDriverCaps] = useState<string[]>(["fs_read", "fs_write", "terminal"]);

  // Profile form
  const [profileName, setProfileName] = useState("");
  const [profileRole, setProfileRole] = useState("worker");
  const [profileDriver, setProfileDriver] = useState("claude");
  const [profileCaps, setProfileCaps] = useState<string[]>(["backend", "frontend"]);
  const [profileActions, setProfileActions] = useState<string[]>(["implement", "test"]);
  const [profileMaxTurns, setProfileMaxTurns] = useState("12");
  const [newCap, setNewCap] = useState("");
  const [newAction, setNewAction] = useState("");

  const toggleCap = (cap: string) => {
    setDriverCaps((prev) =>
      prev.includes(cap) ? prev.filter((c) => c !== cap) : [...prev, cap],
    );
  };

  return (
    <div className="flex-1 space-y-6 p-8">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">代理管理</h1>
          <p className="text-sm text-muted-foreground">管理工作流中的代理配置和角色分配</p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" onClick={() => setDriverDialogOpen(true)}>
            <Settings2 className="mr-2 h-4 w-4" />
            新建驱动
          </Button>
          <Button onClick={() => setProfileDialogOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            新建配置
          </Button>
        </div>
      </div>

      {/* Drivers section */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Bot className="h-5 w-5" />
            驱动 <Badge variant="secondary" className="ml-1">{drivers.length}</Badge>
          </CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>名称</TableHead>
                <TableHead>启动命令</TableHead>
                <TableHead>最大能力</TableHead>
                <TableHead>配置数</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {drivers.map((d) => (
                <TableRow key={d.id}>
                  <TableCell className="font-medium">{d.name}</TableCell>
                  <TableCell>
                    <code className="rounded bg-muted px-1.5 py-0.5 text-xs font-mono">{d.command}</code>
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {d.capabilitiesMax.map((c) => (
                        <Badge key={c} variant="outline" className="text-xs">{c}</Badge>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell>{d.profileCount}</TableCell>
                  <TableCell>
                    <Button variant="ghost" size="icon" className="h-8 w-8">
                      <Settings2 className="h-4 w-4" />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {/* Profiles section */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            配置 <Badge variant="secondary" className="ml-1">{profiles.length}</Badge>
          </CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>名称</TableHead>
                <TableHead>角色</TableHead>
                <TableHead>驱动</TableHead>
                <TableHead>能力标签</TableHead>
                <TableHead>操作权限</TableHead>
                <TableHead>最大轮次</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {profiles.map((p) => (
                <TableRow key={p.id}>
                  <TableCell className="font-medium">{p.name}</TableCell>
                  <TableCell>
                    <Badge variant={roleBadgeVariant[p.role] ?? "secondary"}>{p.role}</Badge>
                  </TableCell>
                  <TableCell className="text-muted-foreground">{p.driver}</TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {p.capabilities.map((c) => (
                        <Badge key={c} variant="outline" className="text-xs">{c}</Badge>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {p.actions.map((a) => (
                        <Badge key={a} variant="secondary" className="text-xs">{a}</Badge>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell>{p.maxTurns}</TableCell>
                  <TableCell>
                    <Button variant="ghost" size="icon" className="h-8 w-8">
                      <Settings2 className="h-4 w-4" />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {/* ── Create Driver Dialog ── */}
      <Dialog open={driverDialogOpen} onClose={() => setDriverDialogOpen(false)} className="max-w-md">
        <DialogHeader>
          <DialogTitle>新建驱动</DialogTitle>
          <DialogDescription>添加一个新的 Agent 驱动进程配置</DialogDescription>
        </DialogHeader>
        <DialogBody>
          <div className="space-y-1.5">
            <label className="text-sm font-medium">驱动名称</label>
            <Input placeholder="例如：claude" value={driverName} onChange={(e) => setDriverName(e.target.value)} />
          </div>
          <div className="space-y-1.5">
            <label className="text-sm font-medium">启动命令</label>
            <Input placeholder="npx" className="font-mono" value={driverCmd} onChange={(e) => setDriverCmd(e.target.value)} />
          </div>
          <div className="space-y-1.5">
            <label className="text-sm font-medium">启动参数</label>
            <Input placeholder="-y @anthropic-ai/claude-code-acp" value={driverArgs} onChange={(e) => setDriverArgs(e.target.value)} />
          </div>
          <div className="space-y-2">
            <label className="text-sm font-medium">最大能力</label>
            <p className="text-xs text-muted-foreground">该驱动允许的最高权限上限</p>
            <div className="flex gap-4">
              {ALL_CAPS.map((cap) => (
                <label key={cap} className="flex items-center gap-2 cursor-pointer">
                  <button
                    type="button"
                    onClick={() => toggleCap(cap)}
                    className={cn(
                      "flex h-[18px] w-[18px] items-center justify-center rounded transition-colors",
                      driverCaps.includes(cap)
                        ? "bg-primary text-primary-foreground"
                        : "border border-input",
                    )}
                  >
                    {driverCaps.includes(cap) && <Check className="h-3 w-3" />}
                  </button>
                  <span className="text-sm">{cap}</span>
                </label>
              ))}
            </div>
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => setDriverDialogOpen(false)}>取消</Button>
          <Button>创建驱动</Button>
        </DialogFooter>
      </Dialog>

      {/* ── Create Profile Dialog ── */}
      <Dialog open={profileDialogOpen} onClose={() => setProfileDialogOpen(false)} className="max-w-lg">
        <DialogHeader>
          <DialogTitle>新建配置</DialogTitle>
          <DialogDescription>创建 Agent 角色配置，绑定驱动并设置能力</DialogDescription>
        </DialogHeader>
        <DialogBody>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-1.5">
              <label className="text-sm font-medium">配置名称</label>
              <Input placeholder="例如：claude-worker" value={profileName} onChange={(e) => setProfileName(e.target.value)} />
            </div>
            <div className="space-y-1.5">
              <label className="text-sm font-medium">角色</label>
              <select
                className="flex h-10 w-full rounded-md border bg-background px-3 text-sm"
                value={profileRole}
                onChange={(e) => setProfileRole(e.target.value)}
              >
                <option value="worker">worker</option>
                <option value="gate">gate</option>
                <option value="lead">lead</option>
                <option value="support">support</option>
              </select>
            </div>
          </div>
          <div className="space-y-1.5">
            <label className="text-sm font-medium">驱动</label>
            <select
              className="flex h-10 w-full rounded-md border bg-background px-3 text-sm"
              value={profileDriver}
              onChange={(e) => setProfileDriver(e.target.value)}
            >
              {drivers.map((d) => (
                <option key={d.id} value={d.id}>{d.name}</option>
              ))}
            </select>
          </div>
          <div className="space-y-2">
            <label className="text-sm font-medium">能力标签</label>
            <p className="text-xs text-muted-foreground">不能超出驱动的最大能力范围</p>
            <div className="flex flex-wrap gap-2">
              {profileCaps.map((c) => (
                <span key={c} className="inline-flex items-center gap-1.5 rounded-full bg-secondary px-3 py-1 text-xs font-medium">
                  {c}
                  <button onClick={() => setProfileCaps((prev) => prev.filter((x) => x !== c))}>
                    <X className="h-3 w-3 text-muted-foreground hover:text-foreground" />
                  </button>
                </span>
              ))}
              <form
                className="inline-flex"
                onSubmit={(e) => {
                  e.preventDefault();
                  if (newCap.trim() && !profileCaps.includes(newCap.trim())) {
                    setProfileCaps((prev) => [...prev, newCap.trim()]);
                    setNewCap("");
                  }
                }}
              >
                <input
                  className="w-16 rounded-full border px-3 py-1 text-xs outline-none focus:ring-1 focus:ring-ring"
                  placeholder="+ 添加"
                  value={newCap}
                  onChange={(e) => setNewCap(e.target.value)}
                />
              </form>
            </div>
          </div>
          <div className="space-y-2">
            <label className="text-sm font-medium">操作权限</label>
            <div className="flex flex-wrap gap-2">
              {profileActions.map((a) => (
                <span key={a} className="inline-flex items-center gap-1.5 rounded-full bg-indigo-50 px-3 py-1 text-xs font-medium text-indigo-600">
                  {a}
                  <button onClick={() => setProfileActions((prev) => prev.filter((x) => x !== a))}>
                    <X className="h-3 w-3 text-indigo-400 hover:text-indigo-600" />
                  </button>
                </span>
              ))}
              <form
                className="inline-flex"
                onSubmit={(e) => {
                  e.preventDefault();
                  if (newAction.trim() && !profileActions.includes(newAction.trim())) {
                    setProfileActions((prev) => [...prev, newAction.trim()]);
                    setNewAction("");
                  }
                }}
              >
                <input
                  className="w-16 rounded-full border px-3 py-1 text-xs outline-none focus:ring-1 focus:ring-ring"
                  placeholder="+ 添加"
                  value={newAction}
                  onChange={(e) => setNewAction(e.target.value)}
                />
              </form>
            </div>
          </div>
          <div className="w-28 space-y-1.5">
            <label className="text-sm font-medium">最大轮次</label>
            <Input type="number" value={profileMaxTurns} onChange={(e) => setProfileMaxTurns(e.target.value)} />
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => setProfileDialogOpen(false)}>取消</Button>
          <Button>创建配置</Button>
        </DialogFooter>
      </Dialog>
    </div>
  );
}
