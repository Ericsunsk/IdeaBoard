import { ref, type Ref } from 'vue'
import { normalizeLatestBoardState, type BoardState } from '../../domain'
import {
  getBoardState,
  getBoardStates,
  setBoardPreviewSnapshot,
} from '../../infrastructure/persistence'
import { renderBoardPreviewDataUrl } from './boardPreviewSerializer'
import { BOARD_PREVIEW_RENDERER_VERSION } from './boardPreviewVersion'
import { isBoardNameConflictError, syncBoardToServer } from '../../application/sync/boardSync'
import type { BoardMeta } from '../../application/catalog/boards'

export interface BoardPreviewRefreshQueueDependencies {
  getBoardState: (boardId: string) => BoardState | null
  getBoardStates: (boardIds: readonly string[]) => Record<string, BoardState | null>
  setPreviewSnapshot: (
    boardId: string,
    previewSnapshotDataUrl: string | undefined,
    previewSnapshotUpdatedAt: number | undefined,
    previewSnapshotVersion: number | undefined,
  ) => BoardState | null
  renderPreview: (state: BoardState) => Promise<string>
  syncBoard: (boardId: string) => Promise<unknown>
  isNameConflictError: (error: unknown) => boolean
  rendererVersion: number
  now: () => number
  warn: (...data: unknown[]) => void
}

export interface BoardPreviewRefreshQueue {
  previewMap: Ref<Record<string, string>>
  reloadFromLocal: (boards: BoardMeta[]) => void
  schedule: (boards: BoardMeta[]) => void
  invalidate: () => void
  dispose: () => void
}

const defaultDependencies: BoardPreviewRefreshQueueDependencies = {
  getBoardState: getBoardState,
  getBoardStates: getBoardStates,
  setPreviewSnapshot: setBoardPreviewSnapshot,
  renderPreview: renderBoardPreviewDataUrl,
  syncBoard: syncBoardToServer,
  isNameConflictError: isBoardNameConflictError,
  rendererVersion: BOARD_PREVIEW_RENDERER_VERSION,
  now: Date.now,
  warn: console.warn,
}

export function createBoardPreviewRefreshQueue(
  dependencyOverrides: Partial<BoardPreviewRefreshQueueDependencies> = {},
): BoardPreviewRefreshQueue {
  const deps = { ...defaultDependencies, ...dependencyOverrides }
  const previewMap = ref<Record<string, string>>({})
  const queue: string[] = []
  const queuedBoardIds = new Set<string>()
  let isInFlight = false
  let isDisposed = false
  let generation = 0

  function readLatestBoardState(boardId: string, boundary: string): BoardState | null {
    const state = deps.getBoardState(boardId)
    const result = normalizeLatestBoardState(state, { mode: 'result', boundary })
    if (result.ok) return result.value
    if (state) {
      deps.warn('[boards] invalid board state for preview refresh', boardId, result.error)
    }
    return null
  }

  function readLatestBoardStates(
    boardIds: readonly string[],
    boundary: string,
  ): Map<string, BoardState | null> {
    // Bulk-read once per round: the local persistence layer keeps every board
    // in a single storage blob, so one parse serves the whole round instead
    // of re-parsing the blob once per board (O(n^2) total parsing).
    const rawMap = deps.getBoardStates(boardIds)
    const result = new Map<string, BoardState | null>()
    boardIds.forEach((boardId) => {
      const state = rawMap[boardId] ?? null
      const normalized = normalizeLatestBoardState(state, { mode: 'result', boundary })
      if (normalized.ok) {
        result.set(boardId, normalized.value)
        return
      }
      if (state) {
        deps.warn('[boards] invalid board state for preview refresh', boardId, normalized.error)
      }
      result.set(boardId, null)
    })
    return result
  }

  function reloadFromLocal(boards: BoardMeta[]) {
    const nextMap: Record<string, string> = {}
    const states = readLatestBoardStates(
      boards.map((board) => board.id),
      'preview-reload',
    )
    boards.forEach((board) => {
      nextMap[board.id] = states.get(board.id)?.previewSnapshotDataUrl || ''
    })
    previewMap.value = nextMap
  }

  function isPreviewStaleState(state: BoardState | null): boolean {
    if (!state) return false
    if (!state.previewSnapshotDataUrl) return true
    if (!Number.isFinite(state.previewSnapshotUpdatedAt)) return true
    if (state.previewSnapshotVersion !== deps.rendererVersion) return true
    return (state.previewSnapshotUpdatedAt || 0) < state.updatedAt
  }

  function isPreviewStale(boardId: string): boolean {
    return isPreviewStaleState(readLatestBoardState(boardId, 'preview-stale-check'))
  }

  function enqueue(boardId: string) {
    if (!boardId || queuedBoardIds.has(boardId)) return
    queuedBoardIds.add(boardId)
    queue.push(boardId)
  }

  async function refreshInBackground(boardId: string, activeGeneration: number) {
    const startState = readLatestBoardState(boardId, 'preview-render')
    if (!startState || !isPreviewStaleState(startState)) return
    const baseUpdatedAt = startState.updatedAt
    let snapshotDataUrl: string | undefined
    let snapshotUpdatedAt: number | undefined
    try {
      const renderedDataUrl = await deps.renderPreview(startState)
      if (isDisposed || generation !== activeGeneration) return
      snapshotDataUrl = renderedDataUrl || undefined
      snapshotUpdatedAt = renderedDataUrl ? deps.now() : undefined
    } catch (error) {
      deps.warn('[boards] failed to render board preview', boardId, error)
      return
    }

    const latestState = readLatestBoardState(boardId, 'preview-write')
    if (isDisposed || generation !== activeGeneration || !latestState) return
    if (latestState.updatedAt !== baseUpdatedAt) {
      enqueue(boardId)
      return
    }

    const updatedState = deps.setPreviewSnapshot(
      boardId,
      snapshotDataUrl,
      snapshotUpdatedAt,
      snapshotDataUrl ? deps.rendererVersion : undefined,
    )
    if (!updatedState) return
    previewMap.value = {
      ...previewMap.value,
      [boardId]: updatedState.previewSnapshotDataUrl || '',
    }
    void deps.syncBoard(boardId).catch((error) => {
      if (deps.isNameConflictError(error)) return
      deps.warn('[boards] failed to sync refreshed preview', boardId, error)
    })
  }

  async function runQueue() {
    if (isDisposed || isInFlight) return
    isInFlight = true
    try {
      while (!isDisposed && queue.length > 0) {
        const boardId = queue.shift()
        if (!boardId) continue
        queuedBoardIds.delete(boardId)
        if (!isPreviewStale(boardId)) continue
        await refreshInBackground(boardId, generation)
      }
    } finally {
      isInFlight = false
      if (!isDisposed && queue.length > 0) {
        void runQueue()
      }
    }
  }

  function schedule(boards: BoardMeta[]) {
    if (isDisposed) return
    if (boards.length === 0) return
    const states = readLatestBoardStates(
      boards.map((board) => board.id),
      'preview-stale-check',
    )
    boards.forEach((board) => {
      if (isPreviewStaleState(states.get(board.id) ?? null)) {
        enqueue(board.id)
      }
    })
    void runQueue()
  }

  function invalidate() {
    generation += 1
    queue.length = 0
    queuedBoardIds.clear()
  }

  function dispose() {
    isDisposed = true
    invalidate()
  }

  return {
    previewMap,
    reloadFromLocal,
    schedule,
    invalidate,
    dispose,
  }
}
