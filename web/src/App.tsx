import { Suspense, lazy } from "react";
import { BrowserRouter, Navigate, Route, Routes, useParams } from "react-router-dom";
import SystemEventBanner from "@/components/SystemEventBanner";
import { WorkbenchProvider, useWorkbench } from "@/contexts/WorkbenchContext";
import { AppLayout } from "@/layouts/AppLayout";
import { MonitoringLayout } from "@/layouts/MonitoringLayout";
import { RuntimeLayout } from "@/layouts/RuntimeLayout";
import { LoginPage } from "@/pages/LoginPage";

const AgentsPage = lazy(() => import("@/pages/AgentsPage").then((module) => ({ default: module.AgentsPage })));
const AnalyticsPage = lazy(() => import("@/pages/AnalyticsPage").then((module) => ({ default: module.AnalyticsPage })));
const ChatPage = lazy(() => import("@/pages/ChatPage").then((module) => ({ default: module.ChatPage })));
const CreateProjectPage = lazy(() => import("@/pages/CreateProjectPage").then((module) => ({ default: module.CreateProjectPage })));
const CreateWorkItemPage = lazy(() => import("@/pages/CreateWorkItemPage").then((module) => ({ default: module.CreateWorkItemPage })));
const DashboardPage = lazy(() => import("@/pages/DashboardPage").then((module) => ({ default: module.DashboardPage })));
const ExecutionDetailPage = lazy(() => import("@/pages/ExecutionDetailPage").then((module) => ({ default: module.ExecutionDetailPage })));
const FeatureManifestPage = lazy(() => import("@/pages/FeatureManifestPage").then((module) => ({ default: module.FeatureManifestPage })));
const GitTagsPage = lazy(() => import("@/pages/GitTagsPage").then((module) => ({ default: module.GitTagsPage })));
const InspectionPage = lazy(() => import("@/pages/InspectionPage").then((module) => ({ default: module.InspectionPage })));
const MobileHomePage = lazy(() => import("@/pages/MobileHomePage").then((module) => ({ default: module.MobileHomePage })));
const ProjectsPage = lazy(() => import("@/pages/ProjectsPage").then((module) => ({ default: module.ProjectsPage })));
const SandboxPage = lazy(() => import("@/pages/SandboxPage").then((module) => ({ default: module.SandboxPage })));
const ScheduledTasksPage = lazy(() => import("@/pages/ScheduledTasksPage").then((module) => ({ default: module.ScheduledTasksPage })));
const SkillsPage = lazy(() => import("@/pages/SkillsPage").then((module) => ({ default: module.SkillsPage })));
const TemplatesPage = lazy(() => import("@/pages/TemplatesPage").then((module) => ({ default: module.TemplatesPage })));
const ThreadDetailPage = lazy(() => import("@/pages/ThreadDetailPage").then((module) => ({ default: module.ThreadDetailPage })));
const ThreadsPage = lazy(() => import("@/pages/ThreadsPage").then((module) => ({ default: module.ThreadsPage })));
const UsagePage = lazy(() => import("@/pages/UsagePage").then((module) => ({ default: module.UsagePage })));
const WorkItemDetailPage = lazy(() => import("@/pages/WorkItemDetailPage").then((module) => ({ default: module.WorkItemDetailPage })));
const WorkItemsPage = lazy(() => import("@/pages/WorkItemsPage").then((module) => ({ default: module.WorkItemsPage })));

const RouteLoadingFallback = () => (
  <div className="flex min-h-[40vh] items-center justify-center">
    <div className="h-9 w-9 animate-spin rounded-full border-2 border-slate-200 border-t-slate-900" />
  </div>
);

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
      <Suspense fallback={<RouteLoadingFallback />}>
        <Routes>
          <Route element={<AppLayout />}>
            <Route path="/" element={<MobileHomePage />} />
            <Route path="/chat" element={<ChatPage />} />
            {/* Work Items */}
            <Route path="/work-items" element={<WorkItemsPage />} />
            <Route path="/work-items/new" element={<CreateWorkItemPage />} />
            <Route path="/work-items/:workItemId" element={<WorkItemDetailPage />} />
            {/* Threads (accessible via URL, linked from work items) */}
            <Route path="/threads" element={<ThreadsPage />} />
            <Route path="/threads/:threadId" element={<ThreadDetailPage />} />
            {/* Projects */}
            <Route path="/projects" element={<ProjectsPage />} />
            <Route path="/projects/new" element={<CreateProjectPage />} />
            <Route path="/projects/:projectId/git-tags" element={<GitTagsPage />} />
            <Route path="/projects/:projectId/manifest" element={<FeatureManifestPage />} />
            {/* Monitoring hub */}
            <Route path="/monitoring" element={<MonitoringLayout />}>
              <Route index element={<Navigate to="dashboard" replace />} />
              <Route path="dashboard" element={<DashboardPage />} />
              <Route path="analytics" element={<AnalyticsPage />} />
              <Route path="usage" element={<UsagePage />} />
              <Route path="inspections" element={<InspectionPage />} />
              <Route path="scheduled-tasks" element={<ScheduledTasksPage />} />
            </Route>
            {/* Runtime hub */}
            <Route path="/runtime" element={<RuntimeLayout />}>
              <Route index element={<Navigate to="agents" replace />} />
              <Route path="agents" element={<AgentsPage />} />
              <Route path="skills" element={<SkillsPage />} />
              <Route path="templates" element={<TemplatesPage />} />
            </Route>
            {/* Standalone pages */}
            <Route path="/executions/:execId" element={<ExecutionDetailPage />} />
            <Route path="/sandbox" element={<SandboxPage />} />
            {/* Legacy redirects */}
            <Route path="/issues" element={<LegacyWorkItemRedirect target="list" />} />
            <Route path="/issues/new" element={<LegacyWorkItemRedirect target="new" />} />
            <Route path="/issues/:workItemId" element={<LegacyWorkItemRedirect target="detail" />} />
            <Route path="/flows" element={<LegacyWorkItemRedirect target="list" />} />
            <Route path="/flows/new" element={<LegacyWorkItemRedirect target="new" />} />
            <Route path="/flows/:workItemId" element={<LegacyWorkItemRedirect target="detail" />} />
            <Route path="/dashboard" element={<Navigate to="/monitoring/dashboard" replace />} />
            <Route path="/analytics" element={<Navigate to="/monitoring/analytics" replace />} />
            <Route path="/usage" element={<Navigate to="/monitoring/usage" replace />} />
            <Route path="/inspections" element={<Navigate to="/monitoring/inspections" replace />} />
            <Route path="/scheduled-tasks" element={<Navigate to="/monitoring/scheduled-tasks" replace />} />
            <Route path="/agents" element={<Navigate to="/runtime/agents" replace />} />
            <Route path="/skills" element={<Navigate to="/runtime/skills" replace />} />
            <Route path="/templates" element={<Navigate to="/runtime/templates" replace />} />
            <Route path="/llm-api" element={<Navigate to="/runtime/agents" replace />} />
          </Route>
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </Suspense>
    </>
  );
};

export default App;
