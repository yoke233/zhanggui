import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import SystemEventBanner from "@/components/SystemEventBanner";
import { WorkbenchProvider, useWorkbench } from "@/contexts/WorkbenchContext";
import { AppLayout } from "@/layouts/AppLayout";
import { AgentsPage } from "@/pages/AgentsPage";
import { AnalyticsPage } from "@/pages/AnalyticsPage";
import { ChatPage } from "@/pages/ChatPage";
import { CreateIssuePage } from "@/pages/CreateFlowPage";
import { CreateProjectPage } from "@/pages/CreateProjectPage";
import { DashboardPage } from "@/pages/DashboardPage";
import { ExecutionDetailPage } from "@/pages/ExecutionDetailPage";
import { IssueDetailPage } from "@/pages/FlowDetailPage";
import { IssuesPage } from "@/pages/FlowsPage";
import { GitTagsPage } from "@/pages/GitTagsPage";
import { LoginPage } from "@/pages/LoginPage";
import { ProjectsPage } from "@/pages/ProjectsPage";
import { SandboxPage } from "@/pages/SandboxPage";
import { SkillsPage } from "@/pages/SkillsPage";
import { TemplatesPage } from "@/pages/TemplatesPage";
import { UsagePage } from "@/pages/UsagePage";

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
  const { authStatus, authError, wsClient, login } = useWorkbench();

  if (authStatus !== "ready") {
    return (
      <LoginPage
        onLogin={login}
        loading={authStatus === "checking"}
        error={authError}
      />
    );
  }

  return (
    <>
      <SystemEventBanner wsClient={wsClient} />
      <Routes>
        <Route element={<AppLayout />}>
          <Route path="/" element={<DashboardPage />} />
          <Route path="/chat" element={<ChatPage />} />
          <Route path="/issues" element={<IssuesPage />} />
          <Route path="/issues/new" element={<CreateIssuePage />} />
          <Route path="/issues/:flowId" element={<IssueDetailPage />} />
          {/* Legacy /flows routes redirect to /issues */}
          <Route path="/flows" element={<Navigate to="/issues" replace />} />
          <Route path="/flows/new" element={<Navigate to="/issues/new" replace />} />
          <Route path="/flows/:flowId" element={<Navigate to="/issues" replace />} />
          <Route path="/templates" element={<TemplatesPage />} />
          <Route path="/executions/:execId" element={<ExecutionDetailPage />} />
          <Route path="/analytics" element={<AnalyticsPage />} />
          <Route path="/usage" element={<UsagePage />} />
          <Route path="/sandbox" element={<SandboxPage />} />
          <Route path="/agents" element={<AgentsPage />} />
          <Route path="/skills" element={<SkillsPage />} />
          <Route path="/projects" element={<ProjectsPage />} />
          <Route path="/projects/:projectId/git-tags" element={<GitTagsPage />} />
          <Route path="/projects/new" element={<CreateProjectPage />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </>
  );
};

export default App;
