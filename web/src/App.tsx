import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import SystemEventBanner from "@/components/SystemEventBanner";
import { Badge } from "@/components/ui/badge";
import { WorkbenchProvider, useWorkbench } from "@/contexts/WorkbenchContext";
import { AppLayout } from "@/layouts/AppLayout";
import { AgentsPage } from "@/pages/AgentsPage";
import { AnalyticsPage } from "@/pages/AnalyticsPage";
import { ChatPage } from "@/pages/ChatPage";
import { CreateFlowPage } from "@/pages/CreateFlowPage";
import { CreateProjectPage } from "@/pages/CreateProjectPage";
import { DashboardPage } from "@/pages/DashboardPage";
import { ExecutionDetailPage } from "@/pages/ExecutionDetailPage";
import { FlowDetailPage } from "@/pages/FlowDetailPage";
import { FlowsPage } from "@/pages/FlowsPage";
import { GitTagsPage } from "@/pages/GitTagsPage";
import { ProjectsPage } from "@/pages/ProjectsPage";
import { SandboxPage } from "@/pages/SandboxPage";
import { SkillsPage } from "@/pages/SkillsPage";
import { TemplatesPage } from "@/pages/TemplatesPage";

interface AppProps {
  a2aEnabledOverride?: boolean;
}

const App = (_props: AppProps = {}) => {
  return (
    <BrowserRouter>
      <WorkbenchProvider>
        <WorkbenchRoutes />
      </WorkbenchProvider>
    </BrowserRouter>
  );
};

const WorkbenchRoutes = () => {
  const { authStatus, authError, wsClient } = useWorkbench();

  if (authStatus !== "ready") {
    return (
      <main className="min-h-screen px-4 py-6 text-slate-900 md:px-6">
        <div className="mx-auto flex w-full max-w-3xl flex-col gap-4">
          <section className="rounded-2xl border border-slate-200 bg-white p-8 shadow-[0_24px_80px_rgba(15,23,42,0.08)]">
            <Badge variant="secondary">AI Workflow</Badge>
            <h1 className="mt-4 text-3xl font-semibold tracking-tight">AI Workflow Workbench</h1>
            <p className="mt-2 text-sm text-slate-600">
              {authStatus === "checking" ? "正在验证访问 token..." : authError ?? "Token 校验失败"}
            </p>
            {authStatus === "error" ? (
              <p className="mt-4 rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
                请使用带 token 的访问链接重新进入，例如：<code>?token=xxxx</code>
              </p>
            ) : null}
          </section>
        </div>
      </main>
    );
  }

  return (
    <>
      <SystemEventBanner wsClient={wsClient} />
      <Routes>
        <Route element={<AppLayout />}>
          <Route path="/" element={<DashboardPage />} />
          <Route path="/chat" element={<ChatPage />} />
          <Route path="/flows" element={<FlowsPage />} />
          <Route path="/flows/new" element={<CreateFlowPage />} />
          <Route path="/flows/:flowId" element={<FlowDetailPage />} />
          <Route path="/templates" element={<TemplatesPage />} />
          <Route path="/executions/:execId" element={<ExecutionDetailPage />} />
          <Route path="/analytics" element={<AnalyticsPage />} />
          <Route path="/sandbox" element={<SandboxPage />} />
          <Route path="/agents" element={<AgentsPage />} />
          <Route path="/skills" element={<SkillsPage />} />
          <Route path="/git-tags" element={<GitTagsPage />} />
          <Route path="/projects" element={<ProjectsPage />} />
          <Route path="/projects/new" element={<CreateProjectPage />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </>
  );
};

export default App;
