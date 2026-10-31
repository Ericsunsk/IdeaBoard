import { shallowRef } from 'vue'
import type {
  CanvasGraphCommitPatch,
  CanvasGraphNodeId,
  CanvasGraphNodeRecord,
  CanvasGraphPatchDirection,
} from '../../../core/graph'
import { isSameJsonValue } from '../../../core/graph/graphValueEquality.ts'
import { recordCanvasInteractionCount } from '../../../session/browser/canvasInteractionMetrics.ts'
import { measureCanvasPerf } from '../../../session/browser/canvasPerfProbe.ts'

const DEFAULT_TEXT_COALESCE_MS = 1500

type HistoryEntry = {
  patch: CanvasGraphCommitPatch
  boardId: string | null
  recordedAt: number
}

type UseCanvasGraphHistoryOptions = {
  maxSteps?: number
  applyPatch: (patch: CanvasGraphCommitPatch, direction: CanvasGraphPatchDirection) => boolean
  /** Guard against undo/redo while no Graph Session is attached (board switch). */
  isAttached?: (() => boolean) | undefined
  /** Board epoch stamp: entries recorded for another board are never applied. */
  getBoardId?: (() => string | null | undefined) | undefined
  /** Time window inside which consecutive single-node text edits merge. */
  coalesceTextMs?: number | undefined
  now?: (() => number) | undefined
}

export type CanvasGraphHistory = {
  record: (patch: CanvasGraphCommitPatch) => boolean
  undo: () => void
  redo: () => void
  clearBaseline: () => void
}

type SingleNodeTextEdit = {
  id: CanvasGraphNodeId
  before: CanvasGraphNodeRecord
  after: CanvasGraphNodeRecord
}

function isSingleNodeRecordsOnly(patch: CanvasGraphCommitPatch): boolean {
  return (
    patch.nodes.records.length === 1 &&
    patch.edges.records.length === 0 &&
    patch.groups.records.length === 0 &&
    patch.nodes.order.length === 0 &&
    patch.edges.order.length === 0 &&
    patch.groups.order.length === 0
  )
}

function isTextOnlySnapshotChange(
  beforeSnapshot: CanvasGraphNodeRecord['snapshot'],
  afterSnapshot: CanvasGraphNodeRecord['snapshot'],
): boolean {
  if (beforeSnapshot.type !== 'prompt' || afterSnapshot.type !== 'prompt') return false
  if (beforeSnapshot.text === afterSnapshot.text) return false
  const { text: _beforeText, ...beforeRest } = beforeSnapshot
  const { text: _afterText, ...afterRest } = afterSnapshot
  return isSameJsonValue(beforeRest, afterRest)
}

function readSingleNodeTextEdit(patch: CanvasGraphCommitPatch): SingleNodeTextEdit | null {
  if (!isSingleNodeRecordsOnly(patch)) return null
  const record = patch.nodes.records[0]
  if (!record?.before || !record?.after) return null
  if (record.before.isSelected !== record.after.isSelected) return null
  if (!isTextOnlySnapshotChange(record.before.snapshot, record.after.snapshot)) return null
  return { id: record.id, before: record.before, after: record.after }
}

function tryMergeTextEdit(
  previous: SingleNodeTextEdit,
  next: SingleNodeTextEdit,
  nextPatch: CanvasGraphCommitPatch,
): CanvasGraphCommitPatch | null {
  if (previous.id !== next.id) return null
  return {
    ...nextPatch,
    nodes: {
      ...nextPatch.nodes,
      records: [{ id: previous.id, before: previous.before, after: next.after }],
    },
  }
}

/**
 * Stores reversible graph commits, never pre/post Flow snapshots. The graph
 * session remains the sole source of truth and owns applying a patch back to
 * the domain; this module only manages bounded undo/redo stacks.
 */
export function useCanvasGraphHistory(options: UseCanvasGraphHistoryOptions): CanvasGraphHistory {
  const maxSteps = Math.max(1, options.maxSteps ?? 10)
  const coalesceTextMs = Math.max(0, options.coalesceTextMs ?? DEFAULT_TEXT_COALESCE_MS)
  const now = options.now ?? (() => Date.now())
  const past = shallowRef<HistoryEntry[]>([])
  const future = shallowRef<HistoryEntry[]>([])

  function pushBounded(current: HistoryEntry[], entry: HistoryEntry) {
    return [...current, entry].slice(-maxSteps)
  }

  function record(patch: CanvasGraphCommitPatch): boolean {
    const hasRecords =
      patch.nodes.records.length > 0 ||
      patch.edges.records.length > 0 ||
      patch.groups.records.length > 0
    const hasOrder =
      patch.nodes.order.length > 0 || patch.edges.order.length > 0 || patch.groups.order.length > 0
    if (!hasRecords && !hasOrder) return false
    const boardId = options.getBoardId?.() ?? null
    const recordedAt = now()
    recordCanvasInteractionCount('historyPushCount')
    measureCanvasPerf(
      'history.record',
      () => ({
        nodes: patch.nodes.records.length,
        edges: patch.edges.records.length,
        groups: patch.groups.records.length,
      }),
      () => {
        // Keystroke-level prompt edits would otherwise push one entry per
        // character and evict structural history (maxSteps). Consecutive
        // text-only edits to the same node inside the coalescing window merge
        // into one entry, so undo restores the pre-typing text in one step.
        const previous = past.value[past.value.length - 1]
        let mergedPatch: CanvasGraphCommitPatch | null = null
        let mergedRecordedAt = recordedAt
        if (
          previous &&
          previous.boardId === boardId &&
          recordedAt - previous.recordedAt <= coalesceTextMs
        ) {
          const previousEdit = readSingleNodeTextEdit(previous.patch)
          const nextEdit = previousEdit ? readSingleNodeTextEdit(patch) : null
          mergedPatch =
            previousEdit && nextEdit ? tryMergeTextEdit(previousEdit, nextEdit, patch) : null
          if (mergedPatch) mergedRecordedAt = previous.recordedAt
        }
        const base = mergedPatch ? past.value.slice(0, -1) : past.value
        past.value = pushBounded(base, {
          patch: mergedPatch ?? patch,
          boardId,
          recordedAt: mergedRecordedAt,
        })
        future.value = []
      },
    )
    return true
  }

  function clearBaseline() {
    past.value = []
    future.value = []
  }

  function pruneForeignBoardEntries() {
    const currentBoardId = options.getBoardId?.() ?? null
    if (currentBoardId == null) return
    past.value = past.value.filter(
      (entry) => entry.boardId == null || entry.boardId === currentBoardId,
    )
    future.value = future.value.filter(
      (entry) => entry.boardId == null || entry.boardId === currentBoardId,
    )
  }

  function apply(direction: CanvasGraphPatchDirection) {
    // Undo/redo during a board switch would otherwise call
    // readCanvasGraphStore on a disposed session and throw, or apply a
    // cross-board patch. Bail while detached and never cross the board epoch.
    if (options.isAttached && !options.isAttached()) return
    pruneForeignBoardEntries()
    const source = direction === 'backward' ? past : future
    const destination = direction === 'backward' ? future : past
    const entry = source.value[source.value.length - 1]
    if (!entry) return
    if (!options.applyPatch(entry.patch, direction)) return
    source.value = source.value.slice(0, -1)
    destination.value = pushBounded(destination.value, entry)
  }

  function undo() {
    apply('backward')
  }

  function redo() {
    apply('forward')
  }

  return { record, undo, redo, clearBaseline }
}
