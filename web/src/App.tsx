import { BrowserRouter, Navigate, Route, Routes, useParams } from "react-router-dom";
import SystemEventBanner from "@/components/SystemEventBanner";
import { WorkbenchProvider, useWorkbench } from "@/contexts/WorkbenchContext";
import { AppLayout } from "@/layouts/AppLayout";
import { AgentsPage } from "@/pages/AgentsPage";
import { AnalyticsPage } from "@/pages/AnalyticsPage";
import { ChatPage } from "@/pages/ChatPage";
import { CreateWorkItemPage } from "@/pages/CreateWorkItemPage";
import { CreateProjectPage } from "@/pages/CreateProjectPage";
import { DashboardPage } from "@/pages/DashboardPage";
import { MobileHomePage } from "@/pages/MobileHomePage";
import { ExecutionDetailPage } from "@/pages/ExecutionDetailPage";
import { WorkItemDetailPage } from "@/pages/WorkItemDetailPage";
import { WorkItemsPage } from "@/pages/WorkItemsPage";
import { FeatureManifestPage } from "@/pages/FeatureManifestPage";
import { GitTagsPage } from "@/pages/GitTagsPage";
import { LoginPage } from "@/pages/LoginPage";
import { ProjectsPage } from "@/pages/ProjectsPage";
import { SandboxPage } from "@/pages/SandboxPage";
import { SkillsPage } from "@/pages/SkillsPage";
import { TemplatesPage } from "@/pages/TemplatesPage";
import { ThreadsPage } from "@/pages/ThreadsPage";
import { ThreadDetailPage } from "@/pages/ThreadDetailPage";
import { InspectionPage } from "@/pages/InspectionPage";
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

const LegacyWorkItemRedirect = ({ target }: { target: "detail" | "list" | "new" }) => {
  const { workItemId } = useParams();

  if (target === "new") {
    return <Navigate to="/work-items/new" replace />;
  }
  if (target === "detail" && workItemId) {
    return <Navigate to={`/work-items/${workItemId}`} replace />;
  }
  return <Navigate to="/work-items" replace />;
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
          <Route path="/" element={<MobileHomePage />} />
          <Route path="/dashboard" element={<DashboardPage />} />
          <Route path="/threads" element={<ThreadsPage />} />
          <Route path="/threads/:threadId" element={<ThreadDetailPage />} />
          <Route path="/chat" element={<ChatPage />} />
          {/* Work Items (primary entry) */}
          <Route path="/work-items" element={<WorkItemsPage />} />
          <Route path="/work-items/new" element={<CreateWorkItemPage />} />
          <Route path="/work-items/:workItemId" element={<WorkItemDetailPage />} />
          {/* Legacy /issues routes redirect to /work-items */}
          <Route path="/issues" element={<LegacyWorkItemRedirect target="list" />} />
          <Route path="/issues/new" element={<LegacyWorkItemRedirect target="new" />} />
          <Route path="/issues/:workItemId" element={<LegacyWorkItemRedirect target="detail" />} />
          {/* Legacy /flows routes redirect to /work-items */}
          <Route path="/flows" element={<LegacyWorkItemRedirect target="list" />} />
          <Route path="/flows/new" element={<LegacyWorkItemRedirect target="new" />} />
          <Route path="/flows/:workItemId" element={<LegacyWorkItemRedirect target="detail" />} />
          <Route path="/templates" element={<TemplatesPage />} />
          <Route path="/executions/:execId" element={<ExecutionDetailPage />} />
          <Route path="/analytics" element={<AnalyticsPage />} />
          <Route path="/inspections" element={<InspectionPage />} />
          <Route path="/usage" element={<UsagePage />} />
          <Route path="/llm-api" element={<Navigate to="/agents" replace />} />
          <Route path="/sandbox" element={<SandboxPage />} />
          <Route path="/agents" element={<AgentsPage />} />
          <Route path="/skills" element={<SkillsPage />} />
          <Route path="/projects" element={<ProjectsPage />} />
          <Route path="/projects/:projectId/git-tags" element={<GitTagsPage />} />
          <Route path="/projects/:projectId/manifest" element={<FeatureManifestPage />} />
          <Route path="/projects/new" element={<CreateProjectPage />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </>
  );
};

export default App;
