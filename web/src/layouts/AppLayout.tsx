import { Outlet } from "react-router-dom";
import { AppSidebar } from "@/components/app-sidebar";

export function AppLayout() {
  return (
    <div className="flex h-screen overflow-hidden">
      <AppSidebar />
      <main className="flex-1 overflow-auto bg-background">
        <Outlet />
      </main>
    </div>
  );
}
