import { NavLink, Outlet } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import {
  Bot,
  Sparkles,
  FileStack,
} from "lucide-react";

const tabs = [
  { to: "/runtime/agents", icon: Bot, labelKey: "nav.agents" },
  { to: "/runtime/skills", icon: Sparkles, labelKey: "nav.skills" },
  { to: "/runtime/templates", icon: FileStack, labelKey: "nav.templates" },
];

export function RuntimeLayout() {
  const { t } = useTranslation();

  return (
    <div className="flex h-full flex-col">
      <div className="shrink-0 border-b bg-background">
        <div className="flex items-center gap-1 overflow-x-auto px-8 pt-4">
          {tabs.map((tab) => (
            <NavLink
              key={tab.to}
              to={tab.to}
              className={({ isActive }) =>
                cn(
                  "flex items-center gap-2 border-b-2 px-4 pb-3 pt-1 text-sm font-medium transition-colors whitespace-nowrap",
                  isActive
                    ? "border-primary text-foreground"
                    : "border-transparent text-muted-foreground hover:border-muted-foreground/30 hover:text-foreground",
                )
              }
            >
              <tab.icon className="h-4 w-4" />
              {t(tab.labelKey)}
            </NavLink>
          ))}
        </div>
      </div>
      <div className="flex-1 overflow-auto">
        <Outlet />
      </div>
    </div>
  );
}
