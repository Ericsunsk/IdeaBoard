import { shallowRef } from 'vue'
import type { CanvasGraphCommitPatch, CanvasGraphPatchDirection } from '..'
import { recordCanvasInteractionCount } from '../../../infrastructure/browser/canvasInteractionMetrics.ts'
import { measureCanvasPerf } from '../../../infrastructure/browser/canvasPerfProbe.ts'

type UseCanvasGraphHistoryOptions = {
  maxSteps?: number
  applyPatch: (patch: CanvasGraphCommitPatch, direction: CanvasGraphPatchDirection) => boolean
}

export type CanvasGraphHistory = {
  record: (patch: CanvasGraphCommitPatch) => boolean
  undo: () => void
  redo: () => void
  clearBaseline: () => void
}

/**
 * Stores reversible graph commits, never pre/post Flow snapshots. The graph
 * session remains the sole source of truth and owns applying a patch back to
 * the domain; this module only manages bounded undo/redo stacks.
 */
export function useCanvasGraphHistory(options: UseCanvasGraphHistoryOptions): CanvasGraphHistory {
  const maxSteps = Math.max(1, options.maxSteps ?? 10)
  const past = shallowRef<CanvasGraphCommitPatch[]>([])
  const future = shallowRef<CanvasGraphCommitPatch[]>([])

  function pushBounded(current: CanvasGraphCommitPatch[], patch: CanvasGraphCommitPatch) {
    return [...current, patch].slice(-maxSteps)
  }

  function record(patch: CanvasGraphCommitPatch): boolean {
    const hasRecords =
      patch.nodes.records.length > 0 ||
      patch.edges.records.length > 0 ||
      patch.groups.records.length > 0
    const hasOrder =
      patch.nodes.order.length > 0 || patch.edges.order.length > 0 || patch.groups.order.length > 0
    if (!hasRecords && !hasOrder) return false
    recordCanvasInteractionCount('historyPushCount')
    measureCanvasPerf(
      'history.record',
      () => ({
        nodes: patch.nodes.records.length,
        edges: patch.edges.records.length,
        groups: patch.groups.records.length,
      }),
      () => {
        past.value = pushBounded(past.value, patch)
        future.value = []
      },
    )
    return true
  }

  function clearBaseline() {
    past.value = []
    future.value = []
  }

  function apply(direction: CanvasGraphPatchDirection) {
    const source = direction === 'backward' ? past : future
    const destination = direction === 'backward' ? future : past
    const patch = source.value[source.value.length - 1]
    if (!patch) return
    if (!options.applyPatch(patch, direction)) return
    source.value = source.value.slice(0, -1)
    destination.value = pushBounded(destination.value, patch)
  }

  function undo() {
    apply('backward')
  }

  function redo() {
    apply('forward')
  }

  return { record, undo, redo, clearBaseline }
}
