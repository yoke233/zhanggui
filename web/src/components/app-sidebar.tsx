import { NavLink, useLocation } from "react-router-dom";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { saveLanguage } from "@/i18n";
import {
  Home,
  MessageSquare,
  ClipboardList,
  GitBranch,
  ChevronDown,
  ChevronRight,
  ChevronsUpDown,
  FolderOpen,
  Activity,
  Cpu,
  LogOut,
  Globe,
  PanelLeftClose,
  PanelLeftOpen,
} from "lucide-react";
import { useWorkbench } from "@/contexts/WorkbenchContext";

const navSections = [
  {
    labelKey: "nav.sectionWorkspace",
    items: [
      { to: "/", icon: Home, labelKey: "nav.home" },
      { to: "/work-items", icon: ClipboardList, labelKey: "nav.workItems" },
      { to: "/chat", icon: MessageSquare, labelKey: "nav.chat" },
    ],
  },
  {
    labelKey: "nav.sectionManage",
    items: [
      { to: "/projects", icon: FolderOpen, labelKey: "nav.projects" },
      { to: "/monitoring", icon: Activity, labelKey: "nav.monitoring" },
      { to: "/runtime", icon: Cpu, labelKey: "nav.runtime" },
    ],
  },
];

export function AppSidebar() {
  const { t, i18n } = useTranslation();
  const { projects, selectedProjectId, setSelectedProjectId, logout } = useWorkbench();
  const location = useLocation();
  const [showPicker, setShowPicker] = useState(false);
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem("sidebar-collapsed") === "true");
  const [collapsedSections, setCollapsedSections] = useState<Record<string, boolean>>(() => {
    try {
      const raw = localStorage.getItem("sidebar-collapsed-sections");
      if (!raw) return {};
      const parsed = JSON.parse(raw);
      return parsed && typeof parsed === "object" ? parsed as Record<string, boolean> : {};
    } catch {
      return {};
    }
  });
  const currentProject = useMemo(
    () => projects.find((project) => project.id === selectedProjectId) ?? projects[0] ?? null,
    [projects, selectedProjectId],
  );
  const toggleSection = (labelKey: string) => {
    setCollapsedSections((current) => {
      const next = { ...current, [labelKey]: !current[labelKey] };
      localStorage.setItem("sidebar-collapsed-sections", JSON.stringify(next));
      return next;
    });
  };

  return (
    <aside
      className={cn(
        "flex h-screen flex-col border-r bg-sidebar transition-[width] duration-200",
        collapsed ? "w-14" : "w-56",
      )}
    >
      {/* Logo */}
      <div className="flex h-14 items-center gap-2.5 border-b px-5">
        <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-primary text-primary-foreground">
          <GitBranch className="h-4 w-4" />
        </div>
        {!collapsed && (
          <span className="text-sm font-semibold tracking-tight whitespace-nowrap">AI Workflow</span>
        )}
      </div>

      {/* Project switcher */}
      {!collapsed && (
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
                {currentProject?.name ?? t("nav.noProject")}
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
      )}

      {/* Navigation */}
      <nav className={cn("flex-1 overflow-y-auto py-3", collapsed ? "px-1.5" : "px-3")}>
        {!collapsed && (
          <div className="mb-2 px-3 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
            {t("nav.navigation")}
          </div>
        )}
        <div className="space-y-3">
          {navSections.map((section) => {
            const sectionActive = section.items.some((item) =>
              item.to === "/"
                ? location.pathname === "/"
                : location.pathname === item.to || location.pathname.startsWith(`${item.to}/`),
            );

            return (
            <div key={section.labelKey} className="space-y-1">
              {!collapsed && (
                <button
                  type="button"
                  onClick={() => toggleSection(section.labelKey)}
                  className={cn(
                    "flex w-full items-center justify-between rounded-md px-3 pb-1 pt-0.5 text-[10px] font-semibold uppercase tracking-[0.18em] transition-colors",
                    sectionActive
                      ? "bg-accent/80 text-accent-foreground"
                      : "text-muted-foreground hover:bg-accent/60 hover:text-accent-foreground",
                  )}
                >
                  <span>{t(section.labelKey)}</span>
                  {collapsedSections[section.labelKey] ? (
                    <ChevronRight className="h-3.5 w-3.5 shrink-0" />
                  ) : (
                    <ChevronDown className="h-3.5 w-3.5 shrink-0" />
                  )}
                </button>
              )}
              {(collapsed || !collapsedSections[section.labelKey]) && section.items.map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  end={item.to === "/"}
                  title={collapsed ? t(item.labelKey) : undefined}
                  className={({ isActive }) =>
                    cn(
                      "flex items-center rounded-md text-sm font-medium transition-colors",
                      collapsed ? "justify-center px-0 py-2" : "gap-3 px-3 py-2",
                      isActive
                        ? "bg-accent text-accent-foreground"
                        : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
                    )
                  }
                >
                  <item.icon className="h-4 w-4 shrink-0" />
                  {!collapsed && t(item.labelKey)}
                </NavLink>
              ))}
            </div>
            );
          })}
        </div>
      </nav>

      {/* Language switcher + Logout + Collapse toggle */}
      <div className={cn("border-t py-3 space-y-1", collapsed ? "px-1.5" : "px-3")}>
        <button
          onClick={() => {
            const next = i18n.language === "zh-CN" ? "en" : "zh-CN";
            void i18n.changeLanguage(next);
            saveLanguage(next);
          }}
          title={collapsed ? (i18n.language === "zh-CN" ? "English" : "中文") : undefined}
          className={cn(
            "flex w-full items-center rounded-md text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground",
            collapsed ? "justify-center px-0 py-2" : "gap-3 px-3 py-2",
          )}
        >
          <Globe className="h-4 w-4 shrink-0" />
          {!collapsed && (i18n.language === "zh-CN" ? "English" : "中文")}
        </button>
        <button
          onClick={logout}
          title={collapsed ? t("nav.logout") : undefined}
          className={cn(
            "flex w-full items-center rounded-md text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground",
            collapsed ? "justify-center px-0 py-2" : "gap-3 px-3 py-2",
          )}
        >
          <LogOut className="h-4 w-4 shrink-0" />
          {!collapsed && t("nav.logout")}
        </button>
        <button
          onClick={() => {
            const next = !collapsed;
            setCollapsed(next);
            localStorage.setItem("sidebar-collapsed", String(next));
          }}
          title={collapsed ? t("nav.expandSidebar", "Expand sidebar") : t("nav.collapseSidebar", "Collapse sidebar")}
          className={cn(
            "flex w-full items-center rounded-md text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground",
            collapsed ? "justify-center px-0 py-2" : "gap-3 px-3 py-2",
          )}
        >
          {collapsed ? (
            <PanelLeftOpen className="h-4 w-4 shrink-0" />
          ) : (
            <>
              <PanelLeftClose className="h-4 w-4 shrink-0" />
              {t("nav.collapseSidebar", "Collapse")}
            </>
          )}
        </button>
      </div>
    </aside>
  );
}
