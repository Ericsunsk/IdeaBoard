import { ref, type Ref } from 'vue'
import { normalizeLatestCanvasBoardState, type CanvasBoardState } from '../domain'
import { getCanvasBoardState, setCanvasBoardPreviewSnapshot } from '../persistence'
import { renderBoardPreviewDataUrl } from './boardPreviewSerializer'
import { BOARD_PREVIEW_RENDERER_VERSION } from './boardPreviewVersion'
import { isBoardNameConflictError, syncCanvasBoardToServer } from '../catalog/boardSync'
import type { CanvasBoardMeta } from '../catalog/boards'

export interface BoardPreviewRefreshQueueDependencies {
  getBoardState: (boardId: string) => CanvasBoardState | null
  setPreviewSnapshot: (
    boardId: string,
    previewSnapshotDataUrl: string | undefined,
    previewSnapshotUpdatedAt: number | undefined,
    previewSnapshotVersion: number | undefined,
  ) => CanvasBoardState | null
  renderPreview: (state: CanvasBoardState) => Promise<string>
  syncBoard: (boardId: string) => Promise<unknown>
  isNameConflictError: (error: unknown) => boolean
  rendererVersion: number
  now: () => number
  warn: (...data: unknown[]) => void
}

export interface BoardPreviewRefreshQueue {
  previewMap: Ref<Record<string, string>>
  reloadFromLocal: (boards: CanvasBoardMeta[]) => void
  schedule: (boards: CanvasBoardMeta[]) => void
  invalidate: () => void
  dispose: () => void
}

const defaultDependencies: BoardPreviewRefreshQueueDependencies = {
  getBoardState: getCanvasBoardState,
  setPreviewSnapshot: setCanvasBoardPreviewSnapshot,
  renderPreview: renderBoardPreviewDataUrl,
  syncBoard: syncCanvasBoardToServer,
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

  function readLatestBoardState(boardId: string, boundary: string): CanvasBoardState | null {
    const state = deps.getBoardState(boardId)
    const result = normalizeLatestCanvasBoardState(state, { mode: 'result', boundary })
    if (result.ok) return result.value
    if (state) {
      deps.warn('[boards] invalid board state for preview refresh', boardId, result.error)
    }
    return null
  }

  function reloadFromLocal(boards: CanvasBoardMeta[]) {
    const nextMap: Record<string, string> = {}
    boards.forEach((board) => {
      nextMap[board.id] =
        readLatestBoardState(board.id, 'preview-reload')?.previewSnapshotDataUrl || ''
    })
    previewMap.value = nextMap
  }

  function isPreviewStaleState(state: CanvasBoardState | null): boolean {
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

  function schedule(boards: CanvasBoardMeta[]) {
    if (isDisposed) return
    boards.forEach((board) => {
      if (isPreviewStale(board.id)) {
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
