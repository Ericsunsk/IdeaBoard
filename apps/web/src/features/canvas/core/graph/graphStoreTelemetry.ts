/**
 * Telemetry port for the canvas graph store.
 *
 * The domain owns this interface (pure types only) so the store can stay
 * free of infrastructure imports. The application layer provides the real
 * implementation built on the perf probe / interaction metrics; tests and
 * any headless consumer use {@link noopCanvasGraphStoreTelemetry}.
 */
export type CanvasGraphHydrateCounts = {
  nodes: number
  edges: number
  groups: number
}

export type CanvasGraphDraftCounts = {
  nodes: number
  edges: number
}

export type CanvasGraphCommitTelemetry = {
  shouldRecordHistory: boolean
  shouldMarkDirty: boolean
}

export type CanvasGraphStoreTelemetry = {
  measureHydrate<T>(counts: CanvasGraphHydrateCounts, task: () => T): T
  measureDraft<T>(counts: CanvasGraphDraftCounts, task: () => T): T
  recordCommit(sideEffects: CanvasGraphCommitTelemetry): void
}

export const noopCanvasGraphStoreTelemetry: CanvasGraphStoreTelemetry = {
  measureHydrate: (_counts, task) => task(),
  measureDraft: (_counts, task) => task(),
  recordCommit: () => {},
}
