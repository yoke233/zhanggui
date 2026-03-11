import { NavLink } from "react-router-dom";
import { useMemo, useState } from "react";
import { cn } from "@/lib/utils";
import {
  LayoutDashboard,
  MessageSquare,
  GitBranch,
  BarChart3,
  Bot,
  FolderOpen,
  FileStack,
  ChevronsUpDown,
  Sparkles,
  Shield,
  LogOut,
} from "lucide-react";
import { useWorkbench } from "@/contexts/WorkbenchContext";

const navItems = [
  { to: "/", icon: LayoutDashboard, label: "仪表盘" },
  { to: "/chat", icon: MessageSquare, label: "对话" },
  { to: "/flows", icon: GitBranch, label: "流程" },
  { to: "/analytics", icon: BarChart3, label: "运行分析" },
  { to: "/sandbox", icon: Shield, label: "沙盒" },
  { to: "/templates", icon: FileStack, label: "模板" },
  { to: "/agents", icon: Bot, label: "代理" },
  { to: "/skills", icon: Sparkles, label: "技能" },
  { to: "/projects", icon: FolderOpen, label: "项目" },
];

export function AppSidebar() {
  const { projects, selectedProjectId, setSelectedProjectId, logout } = useWorkbench();
  const [showPicker, setShowPicker] = useState(false);
  const currentProject = useMemo(
    () => projects.find((project) => project.id === selectedProjectId) ?? projects[0] ?? null,
    [projects, selectedProjectId],
  );

  return (
    <aside className="flex h-screen w-56 flex-col border-r bg-sidebar">
      {/* Logo */}
      <div className="flex h-14 items-center gap-2.5 border-b px-5">
        <div className="flex h-7 w-7 items-center justify-center rounded-md bg-primary text-primary-foreground">
          <GitBranch className="h-4 w-4" />
        </div>
        <span className="text-sm font-semibold tracking-tight">AI Workflow</span>
      </div>

      {/* Project switcher */}
      <div className="px-3 pt-3">
        <div className="relative">
          <button
            onClick={() => setShowPicker(!showPicker)}
            className="flex w-full items-center gap-2 rounded-md border bg-background px-2.5 py-2 text-sm transition-colors hover:bg-muted"
          >
            <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded bg-indigo-50">
              <GitBranch className="h-3.5 w-3.5 text-indigo-500" />
            </div>
            <span className="flex-1 truncate text-left text-[13px] font-medium">
              {currentProject?.name ?? "未选择项目"}
            </span>
            <ChevronsUpDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          </button>

          {showPicker && projects.length > 0 ? (
            <div className="absolute left-0 right-0 top-full z-50 mt-1 rounded-md border bg-popover p-1 shadow-md">
              {projects.map((project) => (
                <button
                  key={project.id}
                  onClick={() => {
                    setSelectedProjectId(project.id);
                    setShowPicker(false);
                  }}
                  className={cn(
                    "flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-sm transition-colors hover:bg-accent",
                    project.id === currentProject?.id && "bg-accent",
                  )}
                >
                  <GitBranch className="h-3.5 w-3.5 text-muted-foreground" />
                  <span className="truncate">{project.name}</span>
                </button>
              ))}
            </div>
          ) : null}
        </div>
      </div>

      {/* Navigation */}
      <nav className="flex-1 space-y-1 px-3 py-3">
        <div className="mb-2 px-3 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
          导航
        </div>
        {navItems.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === "/"}
            className={({ isActive }) =>
              cn(
                "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                isActive
                  ? "bg-accent text-accent-foreground"
                  : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
              )
            }
          >
            <item.icon className="h-4 w-4" />
            {item.label}
          </NavLink>
        ))}
      </nav>

      {/* Logout */}
      <div className="border-t px-3 py-3">
        <button
          onClick={logout}
          className="flex w-full items-center gap-3 rounded-md px-3 py-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
        >
          <LogOut className="h-4 w-4" />
          Logout
        </button>
      </div>
    </aside>
  );
}

