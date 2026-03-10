import { create } from "zustand";
import type { Run } from "../types/workflow";

const mergeRuns = (current: Run[], incoming: Run[]): Run[] => {
  const next = new Map(current.map((Run) => [Run.id, Run]));
  incoming.forEach((Run) => {
    const previous = next.get(Run.id);
    next.set(Run.id, previous ? { ...previous, ...Run } : Run);
  });
  return Array.from(next.values());
};

interface RunsState {
  RunsByProjectId: Record<string, Run[]>;
  selectedRunId: string | null;
  loading: boolean;
  error: string | null;
  setRuns: (projectId: string, Runs: Run[]) => void;
  upsertRuns: (projectId: string, Runs: Run[]) => void;
  removeRun: (projectId: string, RunId: string) => void;
  selectRun: (RunId: string | null) => void;
  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;
  reset: () => void;
}

const initialState = {
  RunsByProjectId: {} as Record<string, Run[]>,
  selectedRunId: null as string | null,
  loading: false,
  error: null as string | null,
};

export const useRunsStore = create<RunsState>((set) => ({
  ...initialState,
  setRuns: (projectId, Runs) =>
    set((state) => ({
      RunsByProjectId: {
        ...state.RunsByProjectId,
        [projectId]: Runs,
      },
    })),
  upsertRuns: (projectId, Runs) =>
    set((state) => ({
      RunsByProjectId: {
        ...state.RunsByProjectId,
        [projectId]: mergeRuns(
          state.RunsByProjectId[projectId] ?? [],
          Runs,
        ),
      },
    })),
  removeRun: (projectId, RunId) =>
    set((state) => ({
      RunsByProjectId: {
        ...state.RunsByProjectId,
        [projectId]: (state.RunsByProjectId[projectId] ?? []).filter(
          (Run) => Run.id !== RunId,
        ),
      },
      selectedRunId:
        state.selectedRunId === RunId ? null : state.selectedRunId,
    })),
  selectRun: (RunId) => set({ selectedRunId: RunId }),
  setLoading: (loading) => set({ loading }),
  setError: (error) => set({ error }),
  reset: () => set({ ...initialState }),
}));
