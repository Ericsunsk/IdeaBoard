import type {
  CanvasGraphStoreTelemetry,
} from '../../domain/graph/canvasGraphStoreTelemetry'
import { recordCanvasInteractionCount } from '../../infrastructure/browser/canvasInteractionMetrics'
import { measureCanvasPerf } from '../../infrastructure/browser/canvasPerfProbe'

/**
 * Application wiring for the graph store telemetry port.
 *
 * The domain store stays free of infrastructure imports; this probe restores
 * the exact perf-probe / interaction-counter behavior the store used to own
 * inline. Production session creation passes it explicitly (see
 * `createCanvasGraphSessionRuntime`).
 */
export const canvasGraphStoreTelemetryProbe: CanvasGraphStoreTelemetry = {
  measureHydrate: (counts, task) => measureCanvasPerf('graph-store.hydrate', () => counts, task),
  measureDraft: (counts, task) => measureCanvasPerf('graph-transaction.draft', () => counts, task),
  recordCommit: (sideEffects) => {
    recordCanvasInteractionCount('graphCommitCount')
    if (sideEffects.recordHistory) recordCanvasInteractionCount('graphHistoryCommitCount')
    if (sideEffects.markDirty) recordCanvasInteractionCount('graphDirtyCommitCount')
  },
}
