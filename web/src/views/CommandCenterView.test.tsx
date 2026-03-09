/** @vitest-environment jsdom */

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import CommandCenterView from "./CommandCenterView";
import type { ApiClient } from "../lib/apiClient";
import type { ApiIssue, ApiRun, ApiStatsResponse } from "../types/api";
import type { Project } from "../types/workflow";

const buildProject = (): Project => ({
  id: "proj-1",
  name: "Alpha",
  repo_path: "D:/repo/alpha",
  created_at: "2026-03-01T10:00:00.000Z",
  updated_at: "2026-03-01T10:00:00.000Z",
});

const buildIssue = (): ApiIssue => ({
  id: "issue-1",
  project_id: "proj-1",
  session_id: "chat-1",
  title: "梳理首页 / 指标",
  body: "",
  labels: [],
  milestone_id: "",
  attachments: [],
  depends_on: [],
  blocks: [],
  priority: 0,
  template: "standard",
  auto_merge: false,
  state: "open",
  status: "ready",
  run_id: "run-1",
  version: 1,
  superseded_by: "",
  parent_id: "",
  children_mode: "",
  external_id: "",
  submitted_by: "",
  merge_retries: 0,
  triage_instructions: "",
  fail_policy: "block",
  created_at: "2026-03-01T10:00:00.000Z",
  updated_at: "2026-03-01T11:00:00.000Z",
  github: {
    issue_number: 204,
  },
});

const buildRun = (): ApiRun => ({
  id: "run-1",
  project_id: "proj-1",
  profile: "strict",
  status: "in_progress",
  started_at: "2026-03-01T11:00:00.000Z",
  finished_at: "",
  created_at: "2026-03-01T10:00:00.000Z",
  updated_at: "2026-03-01T11:20:00.000Z",
});

const stats: ApiStatsResponse = {
  total_Runs: 12,
  active_Runs: 3,
  success_rate: 0.91,
  avg_duration: "6m",
  tokens_used: {
    claude: 2400,
    codex: 1800,
  },
};

const createMockApiClient = (): ApiClient =>
  ({
    request: vi.fn(),
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    del: vi.fn(),
    listAgents: vi.fn(),
    getStats: vi.fn().mockResolvedValue(stats),
    listProjects: vi.fn(),
    createProject: vi.fn(),
    createProjectCreateRequest: vi.fn(),
    getProjectCreateRequest: vi.fn(),
    listIssues: vi.fn().mockResolvedValue({
      items: [buildIssue()],
      total: 1,
      offset: 0,
    }),
    getIssue: vi.fn(),
    listWorkflowProfiles: vi.fn(),
    getWorkflowProfile: vi.fn(),
    listRuns: vi.fn().mockResolvedValue({
      items: [buildRun()],
      total: 1,
      offset: 0,
    }),
    getRun: vi.fn(),
    getRunCheckpoints: vi.fn(),
    getStageSessionStatus: vi.fn(),
    wakeStageSession: vi.fn(),
    promptStageSession: vi.fn(),
    createIssue: vi.fn(),
    decompose: vi.fn(),
    confirmDecompose: vi.fn(),
    createIssueFromFiles: vi.fn(),
    submitIssueReview: vi.fn(),
    applyIssueAction: vi.fn(),
    getIssueDag: vi.fn(),
    listIssueReviews: vi.fn(),
    listIssueChanges: vi.fn(),
    listChats: vi.fn(),
    listChatRunEvents: vi.fn(),
    getChatEventGroup: vi.fn(),
    createChat: vi.fn(),
    cancelChat: vi.fn(),
    getChatSessionStatus: vi.fn(),
    getChat: vi.fn(),
    getSessionCommands: vi.fn(),
    getSessionConfigOptions: vi.fn(),
    setSessionConfigOption: vi.fn(),
    setIssueAutoMerge: vi.fn(),
    applyTaskAction: vi.fn(),
    listIssueTimeline: vi.fn(),
    listIssueTaskSteps: vi.fn(),
    listAdminAuditLog: vi.fn(),
    getRepoTree: vi.fn(),
    getRepoStatus: vi.fn(),
    getRepoDiff: vi.fn(),
    listRunEvents: vi.fn(),
  }) as ApiClient;

describe("CommandCenterView", () => {
  it("对接统计、Issue 和 Run 接口并渲染 v3 首页", async () => {
    const apiClient = createMockApiClient();
    const onNavigate = vi.fn();
    const project = buildProject();

    render(
      <CommandCenterView
        apiClient={apiClient}
        projectId="proj-1"
        projects={[project]}
        selectedProject={project}
        refreshToken={0}
        onNavigate={onNavigate}
      />,
    );

    await waitFor(() => {
      expect(apiClient.getStats).toHaveBeenCalledTimes(1);
      expect(apiClient.listIssues).toHaveBeenCalledWith("proj-1", { limit: 12, offset: 0 });
      expect(apiClient.listRuns).toHaveBeenCalledWith("proj-1", { limit: 8, offset: 0 });
    });

    expect(screen.getByText(/将项目、Issue、Run 和协议健康放到同一视角/)).toBeTruthy();
    expect(screen.getByText("3")).toBeTruthy();
    expect(screen.getByText("91%")).toBeTruthy();
    expect(screen.getByText("4,200")).toBeTruthy();
    expect(screen.getByText("梳理首页 / 指标")).toBeTruthy();
    expect(screen.getAllByText("run-1").length).toBeGreaterThan(0);

    fireEvent.click(screen.getByRole("button", { name: "查看所有 Issue" }));
    expect(onNavigate).toHaveBeenCalledWith("board");

    fireEvent.click(screen.getByRole("button", { name: "进入会话工作区" }));
    expect(onNavigate).toHaveBeenCalledWith("chat");
  });
});
