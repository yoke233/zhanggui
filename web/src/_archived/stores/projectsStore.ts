import { create } from "zustand";
import type { Project } from "../types/workflow";

const mergeProjects = (current: Project[], incoming: Project[]): Project[] => {
  const next = new Map(current.map((project) => [project.id, project]));
  incoming.forEach((project) => {
    const previous = next.get(project.id);
    next.set(project.id, previous ? { ...previous, ...project } : project);
  });
  return Array.from(next.values());
};

interface ProjectsState {
  projects: Project[];
  selectedProjectId: string | null;
  loading: boolean;
  error: string | null;
  setProjects: (projects: Project[]) => void;
  upsertProjects: (projects: Project[]) => void;
  removeProject: (projectId: string) => void;
  selectProject: (projectId: string | null) => void;
  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;
  reset: () => void;
}

const initialState = {
  projects: [] as Project[],
  selectedProjectId: null as string | null,
  loading: false,
  error: null as string | null,
};

export const useProjectsStore = create<ProjectsState>((set) => ({
  ...initialState,
  setProjects: (projects) => set({ projects }),
  upsertProjects: (projects) =>
    set((state) => ({ projects: mergeProjects(state.projects, projects) })),
  removeProject: (projectId) =>
    set((state) => ({
      projects: state.projects.filter((project) => project.id !== projectId),
      selectedProjectId:
        state.selectedProjectId === projectId ? null : state.selectedProjectId,
    })),
  selectProject: (projectId) => set({ selectedProjectId: projectId }),
  setLoading: (loading) => set({ loading }),
  setError: (error) => set({ error }),
  reset: () => set({ ...initialState }),
}));
