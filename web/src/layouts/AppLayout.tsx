import { useState } from "react";
import { Outlet } from "react-router-dom";
import { Menu } from "lucide-react";
import { AppSidebar } from "@/components/app-sidebar";
import { useIsMobile } from "@/hooks/useIsMobile";

export function AppLayout() {
  const isMobile = useIsMobile();
  const [drawerOpen, setDrawerOpen] = useState(false);

  if (isMobile) {
    return (
      <div className="flex h-screen flex-col overflow-hidden">
        {/* Mobile top bar */}
        <header className="flex h-12 shrink-0 items-center gap-3 border-b bg-sidebar px-3">
          <button
            onClick={() => setDrawerOpen(true)}
            className="flex h-9 w-9 items-center justify-center rounded-md hover:bg-accent"
          >
            <Menu className="h-5 w-5" />
          </button>
          <span className="text-sm font-semibold">AI Workflow</span>
        </header>

        {/* Drawer sidebar */}
        <AppSidebar drawerOpen={drawerOpen} onClose={() => setDrawerOpen(false)} />

        {/* Page content */}
        <main className="flex-1 overflow-auto bg-background">
          <Outlet />
        </main>
      </div>
    );
  }

  return (
    <div className="flex h-screen overflow-hidden">
      <AppSidebar />
      <main className="flex-1 overflow-auto bg-background">
        <Outlet />
      </main>
    </div>
  );
}
