import { beforeEach, describe, expect, it } from "vitest";
import { useProjectsStore } from "./projectsStore";

describe("projectsStore", () => {
  const buildProject = (id: string, name: string, repoPath: string) => ({
    id,
    name,
    repo_path: repoPath,
    created_at: "2026-03-01T10:00:00.000Z",
    updated_at: "2026-03-01T10:00:00.000Z",
  });

  beforeEach(() => {
    useProjectsStore.setState({
      projects: [],
      selectedProjectId: null,
      loading: false,
      error: null,
    });
  });

  it("upsertProjects 会按 id 合并并覆盖旧值", () => {
    useProjectsStore.getState().upsertProjects([
      buildProject("p1", "Alpha", "D:/repo/a"),
    ]);
    useProjectsStore.getState().upsertProjects([
      buildProject("p1", "Alpha-2", "D:/repo/a2"),
      buildProject("p2", "Beta", "D:/repo/b"),
    ]);

    const state = useProjectsStore.getState();
    expect(state.projects).toHaveLength(2);
    expect(state.projects.find((project) => project.id === "p1")).toEqual({
      id: "p1",
      name: "Alpha-2",
      repo_path: "D:/repo/a2",
      created_at: "2026-03-01T10:00:00.000Z",
      updated_at: "2026-03-01T10:00:00.000Z",
    });
  });

  it("removeProject 会删除项目并清空已选中 id", () => {
    useProjectsStore.getState().upsertProjects([
      buildProject("p1", "Alpha", "D:/repo/a"),
    ]);
    useProjectsStore.getState().selectProject("p1");

    useProjectsStore.getState().removeProject("p1");

    const state = useProjectsStore.getState();
    expect(state.projects).toEqual([]);
    expect(state.selectedProjectId).toBeNull();
  });
});
