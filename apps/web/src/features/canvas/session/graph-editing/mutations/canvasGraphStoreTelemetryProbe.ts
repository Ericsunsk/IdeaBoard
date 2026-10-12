import type { CanvasGraphStoreTelemetry } from '../../../core/graph'
import { recordCanvasInteractionCount } from '../../../session/browser/canvasInteractionMetrics'
import { measureCanvasPerf } from '../../../session/browser/canvasPerfProbe'

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
    if (sideEffects.shouldRecordHistory) recordCanvasInteractionCount('graphHistoryCommitCount')
    if (sideEffects.shouldMarkDirty) recordCanvasInteractionCount('graphDirtyCommitCount')
  },
}
