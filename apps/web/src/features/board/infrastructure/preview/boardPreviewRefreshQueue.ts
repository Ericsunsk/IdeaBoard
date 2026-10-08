import { ref, type Ref } from 'vue'
import { normalizeLatestBoardState, type BoardState } from '../../domain'
import {
  getBoardState,
  getBoardStates,
  setBoardPreviewSnapshot,
} from '../../infrastructure/persistence'
import { renderBoardPreviewDataUrl } from './boardPreviewSerializer'
import { BOARD_PREVIEW_RENDERER_VERSION } from './boardPreviewVersion'
import { isBoardSyncConflictError, syncBoardToServer } from '../../application/sync/boardSync'
import type { BoardMeta } from '../../application/catalog/boards'
import { isDarkModeActive } from '../../../../shared/theme/composables/useTheme'
import { getStoredPreviewTheme, setStoredPreviewTheme } from './boardPreviewThemeStorage'

export interface BoardPreviewRefreshQueueDependencies {
  getBoardState: (boardId: string) => BoardState | null
  getBoardStates: (boardIds: readonly string[]) => Record<string, BoardState | null>
  setPreviewSnapshot: (
    boardId: string,
    previewSnapshotDataUrl: string | undefined,
    previewSnapshotUpdatedAt: number | undefined,
    previewSnapshotVersion: number | undefined,
  ) => BoardState | null
  renderPreview: (state: BoardState, options?: { isDark?: boolean }) => Promise<string>
  syncBoard: (boardId: string) => Promise<unknown>
  isSyncConflictError: (error: unknown) => boolean
  rendererVersion: number
  now: () => number
  warn: (...data: unknown[]) => void
  getCurrentTheme?: () => 'dark' | 'light'
  getPreviewTheme?: (boardId: string) => 'dark' | 'light' | null
  setPreviewTheme?: (boardId: string, theme: 'dark' | 'light') => void
}

export interface BoardPreviewRefreshQueue {
  previewMap: Ref<Record<string, string>>
  reloadFromLocal: (boards: BoardMeta[]) => void
  schedule: (boards: BoardMeta[]) => void
  handleThemeChange?: (isDark: boolean, boards: BoardMeta[]) => void
  invalidate: () => void
  dispose: () => void
}

const defaultDependencies: BoardPreviewRefreshQueueDependencies = {
  getBoardState: getBoardState,
  getBoardStates: getBoardStates,
  setPreviewSnapshot: setBoardPreviewSnapshot,
  renderPreview: renderBoardPreviewDataUrl,
  syncBoard: syncBoardToServer,
  isSyncConflictError: isBoardSyncConflictError,
  rendererVersion: BOARD_PREVIEW_RENDERER_VERSION,
  now: Date.now,
  warn: console.warn,
  getCurrentTheme: () => (isDarkModeActive() ? 'dark' : 'light'),
  getPreviewTheme: getStoredPreviewTheme,
  setPreviewTheme: setStoredPreviewTheme,
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

  let currentTheme: 'dark' | 'light' = deps.getCurrentTheme
    ? deps.getCurrentTheme()
    : isDarkModeActive()
      ? 'dark'
      : 'light'

  const themeSnapshotCache = new Map<
    'dark' | 'light',
    Map<string, { dataUrl: string; updatedAt: number }>
  >()

  function getThemeCache(theme: 'dark' | 'light') {
    let cache = themeSnapshotCache.get(theme)
    if (!cache) {
      cache = new Map()
      themeSnapshotCache.set(theme, cache)
    }
    return cache
  }

  function getCachedThemePreview(
    boardId: string,
    state: BoardState | null,
    theme: 'dark' | 'light',
  ): string | null {
    const cached = getThemeCache(theme).get(boardId)
    return cached && state && cached.updatedAt >= state.updatedAt ? cached.dataUrl : null
  }

  function normalizePreviewState(
    state: BoardState | null,
    boundary: string,
    boardId: string,
  ): BoardState | null {
    const normalized = normalizeLatestBoardState(state, { mode: 'result', boundary })
    if (normalized.ok) return normalized.value
    if (state) {
      deps.warn('[boards] invalid board state for preview refresh', boardId, normalized.error)
    }
    return null
  }

  function readLatestBoardState(boardId: string, boundary: string): BoardState | null {
    return normalizePreviewState(deps.getBoardState(boardId), boundary, boardId)
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
      result.set(boardId, normalizePreviewState(rawMap[boardId] ?? null, boundary, boardId))
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
      const state = states.get(board.id)
      const boardId = board.id
      const cachedPreview = getCachedThemePreview(boardId, state ?? null, currentTheme)
      if (cachedPreview !== null) {
        nextMap[boardId] = cachedPreview
        return
      }

      const snapshotUrl = state?.previewSnapshotDataUrl || ''
      if (snapshotUrl) {
        const recordedTheme = deps.getPreviewTheme ? deps.getPreviewTheme(boardId) : null
        if (recordedTheme === currentTheme) {
          nextMap[boardId] = snapshotUrl
          getThemeCache(currentTheme).set(boardId, {
            dataUrl: snapshotUrl,
            updatedAt: state?.updatedAt ?? 0,
          })
          return
        }
      }
      nextMap[boardId] = ''
    })
    previewMap.value = nextMap
  }

  function isPreviewStaleState(state: BoardState | null, boardId?: string): boolean {
    if (!state) return false
    if (!state.previewSnapshotDataUrl) return true
    if (!Number.isFinite(state.previewSnapshotUpdatedAt)) return true
    if (state.previewSnapshotVersion !== deps.rendererVersion) return true
    if ((state.previewSnapshotUpdatedAt || 0) < state.updatedAt) return true
    if (boardId && deps.getPreviewTheme) {
      const cached = getThemeCache(currentTheme).get(boardId)
      if (!cached || cached.updatedAt < state.updatedAt) {
        const recordedTheme = deps.getPreviewTheme(boardId)
        if (recordedTheme !== currentTheme) {
          return true
        }
      }
    }
    return false
  }

  function isPreviewStale(boardId: string): boolean {
    return isPreviewStaleState(readLatestBoardState(boardId, 'preview-stale-check'), boardId)
  }

  function enqueue(boardId: string) {
    if (!boardId || queuedBoardIds.has(boardId)) return
    queuedBoardIds.add(boardId)
    queue.push(boardId)
  }

  async function renderSnapshotDataUrl(
    boardId: string,
    state: BoardState,
    theme: 'dark' | 'light',
    activeGeneration: number,
  ): Promise<string | null> {
    try {
      const rendered = await deps.renderPreview(state, { isDark: theme === 'dark' })
      if (isDisposed || generation !== activeGeneration) return null
      return rendered || null
    } catch (error) {
      deps.warn('[boards] failed to render board preview', boardId, error)
      return null
    }
  }

  function commitRefreshedPreview(
    boardId: string,
    renderTheme: 'dark' | 'light',
    baseUpdatedAt: number,
    renderedDataUrl: string,
  ): BoardState | null {
    getThemeCache(renderTheme).set(boardId, {
      dataUrl: renderedDataUrl,
      updatedAt: baseUpdatedAt,
    })
    deps.setPreviewTheme?.(boardId, renderTheme)

    const updatedState = deps.setPreviewSnapshot(
      boardId,
      renderedDataUrl,
      deps.now(),
      deps.rendererVersion,
    )
    if (!updatedState) return null

    if (currentTheme === renderTheme) {
      previewMap.value = {
        ...previewMap.value,
        [boardId]: updatedState.previewSnapshotDataUrl || '',
      }
    }
    return updatedState
  }

  async function refreshInBackground(boardId: string, activeGeneration: number) {
    const startState = readLatestBoardState(boardId, 'preview-render')
    if (!startState || !isPreviewStaleState(startState, boardId)) return
    const baseUpdatedAt = startState.updatedAt
    const renderTheme = currentTheme

    const renderedDataUrl = await renderSnapshotDataUrl(
      boardId,
      startState,
      renderTheme,
      activeGeneration,
    )
    if (!renderedDataUrl) return

    const latestState = readLatestBoardState(boardId, 'preview-write')
    if (isDisposed || generation !== activeGeneration || !latestState) return
    if (latestState.updatedAt !== baseUpdatedAt) {
      enqueue(boardId)
      return
    }

    const updatedState = commitRefreshedPreview(
      boardId,
      renderTheme,
      baseUpdatedAt,
      renderedDataUrl,
    )
    if (!updatedState) return

    const isContentStale =
      !startState.previewSnapshotDataUrl ||
      !Number.isFinite(startState.previewSnapshotUpdatedAt) ||
      (startState.previewSnapshotUpdatedAt || 0) < baseUpdatedAt ||
      startState.previewSnapshotVersion !== deps.rendererVersion

    if (isContentStale) {
      void deps.syncBoard(boardId).catch((error) => {
        if (deps.isSyncConflictError(error)) return
        deps.warn('[boards] failed to sync refreshed preview', boardId, error)
      })
    }
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
      if (isPreviewStaleState(states.get(board.id) ?? null, board.id)) {
        enqueue(board.id)
      }
    })
    void runQueue()
  }

  function handleThemeChange(isDark: boolean, boards: BoardMeta[]) {
    const nextTheme: 'dark' | 'light' = isDark ? 'dark' : 'light'
    if (currentTheme === nextTheme) return
    currentTheme = nextTheme
    invalidate()

    const states = readLatestBoardStates(
      boards.map((board) => board.id),
      'preview-theme-change',
    )
    const nextMap = { ...previewMap.value }
    const staleBoardIds: string[] = []

    boards.forEach((board) => {
      const state = states.get(board.id)
      if (!state) return
      const cachedPreview = getCachedThemePreview(board.id, state, nextTheme)
      if (cachedPreview !== null) {
        nextMap[board.id] = cachedPreview
      } else {
        staleBoardIds.push(board.id)
      }
    })

    previewMap.value = nextMap

    if (staleBoardIds.length > 0) {
      staleBoardIds.forEach((id) => enqueue(id))
      void runQueue()
    }
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
    handleThemeChange,
    invalidate,
    dispose,
  }
}
