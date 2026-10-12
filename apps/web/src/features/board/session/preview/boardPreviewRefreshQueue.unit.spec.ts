import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import type { BoardState } from '../../core/document/nodes'
import { createEmptyBoardState } from '../../core/document/nodes'
import {
  createBoardPreviewRefreshQueue,
  type BoardPreviewRefreshQueueDependencies,
} from './boardPreviewRefreshQueue'
import type { BoardMeta } from '../catalog/boards'

function makeBoard(id: string): BoardMeta {
  return {
    id,
    name: id,
    createdAt: 1,
    updatedAt: 100,
  }
}

function makeState(overrides: Partial<BoardState> = {}): BoardState {
  return {
    ...createEmptyBoardState(100),
    ...overrides,
  }
}

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve
    reject = promiseReject
  })
  return { promise, resolve, reject }
}

function flushPromises() {
  return new Promise<void>((resolve) => setTimeout(resolve, 0))
}

function makeDeps(states: Record<string, BoardState | null>): BoardPreviewRefreshQueueDependencies {
  return {
    getBoardState: vi.fn((boardId: string) => states[boardId] ?? null),
    getBoardStates: vi.fn((boardIds: readonly string[]) =>
      Object.fromEntries(boardIds.map((boardId) => [boardId, states[boardId] ?? null])),
    ),
    setPreviewSnapshot: vi.fn(
      (
        boardId: string,
        previewSnapshotDataUrl: string | undefined,
        previewSnapshotUpdatedAt: number | undefined,
        previewSnapshotVersion: number | undefined,
      ) => {
        const existing = states[boardId]
        if (!existing) return null
        const updated = {
          ...existing,
          previewSnapshotDataUrl,
          previewSnapshotUpdatedAt,
          previewSnapshotVersion,
        }
        states[boardId] = updated
        return updated
      },
    ),
    renderPreview: vi.fn(async () => 'data:image/png;base64,preview'),
    syncBoard: vi.fn(async () => undefined),
    isSyncConflictError: vi.fn((error: unknown) => {
      if (typeof error !== 'object' || !error) return false
      const record = error as { status?: unknown; code?: unknown }
      return (
        record.status === 409 ||
        record.code === 'board_revision_conflict' ||
        record.code === 'board_idempotency_conflict'
      )
    }),
    rendererVersion: 8,
    now: vi.fn(() => 500),
    warn: vi.fn(),
    getCurrentTheme: () => 'light',
    getPreviewTheme: () => 'light',
    setPreviewTheme: vi.fn(),
  }
}

describe('boardPreviewRefreshQueue', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('loads local preview snapshots and refreshes stale boards only', async () => {
    const states = {
      fresh: makeState({
        previewSnapshotDataUrl: 'fresh-url',
        previewSnapshotUpdatedAt: 150,
        previewSnapshotVersion: 8,
        updatedAt: 100,
      }),
      missingDataUrl: makeState({
        previewSnapshotUpdatedAt: 150,
        previewSnapshotVersion: 8,
        updatedAt: 100,
      }),
      invalidUpdatedAt: makeState({
        previewSnapshotDataUrl: 'invalid-url',
        previewSnapshotUpdatedAt: Number.NaN,
        previewSnapshotVersion: 8,
        updatedAt: 100,
      }),
      oldVersion: makeState({
        previewSnapshotDataUrl: 'old-version-url',
        previewSnapshotUpdatedAt: 150,
        previewSnapshotVersion: 7,
        updatedAt: 100,
      }),
      oldSnapshot: makeState({
        previewSnapshotDataUrl: 'old-snapshot-url',
        previewSnapshotUpdatedAt: 90,
        previewSnapshotVersion: 8,
        updatedAt: 100,
      }),
    }
    const deps = makeDeps(states)
    const queue = createBoardPreviewRefreshQueue(deps)
    const boards = Object.keys(states).map(makeBoard)

    queue.reloadFromLocal(boards)
    queue.schedule(boards)
    await flushPromises()

    expect(queue.previewMap.value).toMatchObject({
      fresh: 'fresh-url',
      missingDataUrl: 'data:image/png;base64,preview',
      invalidUpdatedAt: '',
      oldVersion: 'data:image/png;base64,preview',
      oldSnapshot: 'data:image/png;base64,preview',
    })
    expect(deps.renderPreview).toHaveBeenCalledTimes(3)
    expect(deps.setPreviewSnapshot).toHaveBeenCalledTimes(3)
    expect(deps.setPreviewSnapshot).not.toHaveBeenCalledWith(
      'fresh',
      expect.anything(),
      expect.anything(),
      expect.anything(),
    )
  })

  it('deduplicates queued boards while a refresh is in flight', async () => {
    const gate = deferred<string>()
    const states = {
      boardA: makeState({ updatedAt: 100 }),
    }
    const deps = makeDeps(states)
    vi.mocked(deps.renderPreview).mockReturnValue(gate.promise)
    const queue = createBoardPreviewRefreshQueue(deps)
    const boards = [makeBoard('boardA')]

    queue.schedule(boards)
    queue.schedule(boards)
    await nextTick()

    expect(deps.renderPreview).toHaveBeenCalledTimes(1)
    gate.resolve('deduped-preview')
    await flushPromises()

    expect(deps.setPreviewSnapshot).toHaveBeenCalledTimes(1)
    expect(queue.previewMap.value.boardA).toBe('deduped-preview')
  })

  it('refreshes a historical snapshot when its preview theme metadata is missing', async () => {
    const states = {
      boardA: makeState({
        previewSnapshotDataUrl: 'legacy-preview',
        previewSnapshotUpdatedAt: 150,
        previewSnapshotVersion: 8,
        updatedAt: 100,
      }),
    }
    const deps = makeDeps(states)
    deps.getPreviewTheme = vi.fn(() => null)
    const queue = createBoardPreviewRefreshQueue(deps)
    const board = makeBoard('boardA')

    queue.reloadFromLocal([board])
    expect(queue.previewMap.value.boardA).toBe('')
    queue.schedule([board])
    await flushPromises()

    expect(deps.renderPreview).toHaveBeenCalledTimes(1)
    expect(queue.previewMap.value.boardA).toBe('data:image/png;base64,preview')
  })

  it('drops an in-flight render from the previous theme after a theme change', async () => {
    const lightGate = deferred<string>()
    const states = { boardA: makeState({ updatedAt: 100 }) }
    const deps = makeDeps(states)
    vi.mocked(deps.renderPreview)
      .mockReturnValueOnce(lightGate.promise)
      .mockResolvedValueOnce('fresh-dark-preview')
    const queue = createBoardPreviewRefreshQueue(deps)
    const board = makeBoard('boardA')

    queue.schedule([board])
    await nextTick()
    queue.handleThemeChange?.(true, [board])
    lightGate.resolve('stale-light-preview')
    await flushPromises()

    expect(deps.renderPreview).toHaveBeenCalledTimes(2)
    expect(deps.renderPreview).toHaveBeenLastCalledWith(expect.anything(), { isDark: true })
    expect(deps.setPreviewSnapshot).toHaveBeenCalledTimes(1)
    expect(deps.setPreviewSnapshot).toHaveBeenCalledWith('boardA', 'fresh-dark-preview', 500, 8)
  })

  it('requeues every board after a theme change, including ids already queued', async () => {
    const lightGate = deferred<string>()
    const states = {
      boardA: makeState({ updatedAt: 100 }),
      boardB: makeState({ updatedAt: 100 }),
    }
    const deps = makeDeps(states)
    vi.mocked(deps.renderPreview)
      .mockReturnValueOnce(lightGate.promise)
      .mockResolvedValueOnce('dark-a')
      .mockResolvedValueOnce('dark-b')
    const queue = createBoardPreviewRefreshQueue(deps)
    const boards = [makeBoard('boardA'), makeBoard('boardB')]

    queue.schedule(boards)
    await nextTick()
    queue.handleThemeChange?.(true, boards)
    lightGate.resolve('stale-light-a')
    await flushPromises()
    await flushPromises()

    expect(deps.renderPreview).toHaveBeenCalledTimes(3)
    expect(deps.setPreviewSnapshot).toHaveBeenCalledWith('boardA', 'dark-a', 500, 8)
    expect(deps.setPreviewSnapshot).toHaveBeenCalledWith('boardB', 'dark-b', 500, 8)
  })

  it('drops a stale render after the Boards Catalog invalidates the prior scope', async () => {
    const gate = deferred<string>()
    const states = {
      sharedBoardId: makeState({ updatedAt: 100 }),
    }
    const deps = makeDeps(states)
    vi.mocked(deps.renderPreview).mockReturnValueOnce(gate.promise)
    const queue = createBoardPreviewRefreshQueue(deps)

    queue.schedule([makeBoard('sharedBoardId')])
    await nextTick()
    queue.invalidate()
    states.sharedBoardId = makeState({
      updatedAt: 200,
      previewSnapshotDataUrl: 'new-scope-preview',
      previewSnapshotUpdatedAt: 250,
      previewSnapshotVersion: 8,
    })
    queue.reloadFromLocal([makeBoard('sharedBoardId')])
    gate.resolve('old-scope-preview')
    await flushPromises()

    expect(deps.setPreviewSnapshot).not.toHaveBeenCalled()
    expect(deps.syncBoard).not.toHaveBeenCalled()
    expect(queue.previewMap.value.sharedBoardId).toBe('new-scope-preview')
  })

  it('re-enqueues when updatedAt changes during render and writes the latest render', async () => {
    const states = {
      boardA: makeState({ updatedAt: 100 }),
    }
    const deps = makeDeps(states)
    vi.mocked(deps.renderPreview)
      .mockImplementationOnce(async () => {
        states.boardA = makeState({ updatedAt: 200 })
        return 'stale-render'
      })
      .mockResolvedValueOnce('fresh-render')
    const queue = createBoardPreviewRefreshQueue(deps)

    queue.schedule([makeBoard('boardA')])
    await flushPromises()
    await flushPromises()

    expect(deps.renderPreview).toHaveBeenCalledTimes(2)
    expect(deps.setPreviewSnapshot).toHaveBeenCalledTimes(1)
    expect(deps.setPreviewSnapshot).toHaveBeenCalledWith('boardA', 'fresh-render', 500, 8)
    expect(queue.previewMap.value.boardA).toBe('fresh-render')
  })

  it('warns and skips preview refresh for non-latest board state', async () => {
    const states = {
      boardA: {
        schemaVersion: 1,
        imageNodes: [
          {
            id: 1,
            type: 'image-node',
            x: 0,
            y: 0,
            width: 280,
            sourceImageUrl: 'https://example.com/source.png',
            sourceImageRatio: 1,
            outputImages: [{ url: 'https://example.com/output.png', ratio: 1 }],
          },
        ],
        promptNodes: [
          {
            id: 2,
            type: 'prompt',
            x: 300,
            y: 0,
            width: 280,
            text: 'older prompt',
          },
        ],
        fusionNodes: [
          {
            id: 3,
            type: 'fusion',
            x: 600,
            y: 0,
            width: 286,
            outputImages: [],
          },
        ],
        connections: [],
        groups: [],
        updatedAt: 100,
      } as unknown as BoardState,
    }
    const deps = makeDeps(states)
    const queue = createBoardPreviewRefreshQueue(deps)

    queue.schedule([makeBoard('boardA')])
    await flushPromises()

    expect(deps.renderPreview).not.toHaveBeenCalled()
    expect(deps.setPreviewSnapshot).not.toHaveBeenCalled()
    expect(deps.syncBoard).not.toHaveBeenCalled()
    expect(deps.warn).toHaveBeenCalledWith(
      '[boards] invalid board state for preview refresh',
      'boardA',
      expect.objectContaining({
        code: 'INVALID_BOARD_STATE',
        boundary: 'preview-stale-check',
      }),
    )
    expect(queue.previewMap.value.boardA).toBeUndefined()
  })

  it('warns and skips snapshot writes when render fails', async () => {
    const error = new Error('render failed')
    const states = {
      boardA: makeState({ updatedAt: 100 }),
    }
    const deps = makeDeps(states)
    vi.mocked(deps.renderPreview).mockRejectedValueOnce(error)
    const queue = createBoardPreviewRefreshQueue(deps)

    queue.schedule([makeBoard('boardA')])
    await flushPromises()

    expect(deps.setPreviewSnapshot).not.toHaveBeenCalled()
    expect(deps.warn).toHaveBeenCalledWith(
      '[boards] failed to render board preview',
      'boardA',
      error,
    )
  })

  it('ignores sync-conflict errors from background sync', async () => {
    const states = {
      boardA: makeState({ updatedAt: 100 }),
    }
    const deps = makeDeps(states)
    vi.mocked(deps.syncBoard).mockRejectedValueOnce({
      status: 409,
      code: 'board_revision_conflict',
    })
    const queue = createBoardPreviewRefreshQueue(deps)

    queue.schedule([makeBoard('boardA')])
    await flushPromises()

    expect(deps.isSyncConflictError).toHaveBeenCalledWith({
      status: 409,
      code: 'board_revision_conflict',
    })
    expect(deps.warn).not.toHaveBeenCalledWith(
      '[boards] failed to sync refreshed preview',
      expect.anything(),
      expect.anything(),
    )
  })

  it('ignores idempotency conflicts from background sync', async () => {
    const states = {
      boardA: makeState({ updatedAt: 100 }),
    }
    const deps = makeDeps(states)
    vi.mocked(deps.syncBoard).mockRejectedValueOnce({
      status: 409,
      code: 'board_idempotency_conflict',
    })
    const queue = createBoardPreviewRefreshQueue(deps)

    queue.schedule([makeBoard('boardA')])
    await flushPromises()

    expect(deps.isSyncConflictError).toHaveBeenCalledWith({
      status: 409,
      code: 'board_idempotency_conflict',
    })
    expect(deps.warn).not.toHaveBeenCalledWith(
      '[boards] failed to sync refreshed preview',
      expect.anything(),
      expect.anything(),
    )
  })

  it('warns for non-conflict errors from background sync', async () => {
    const error = new Error('sync failed')
    const states = {
      boardA: makeState({ updatedAt: 100 }),
    }
    const deps = makeDeps(states)
    vi.mocked(deps.syncBoard).mockRejectedValueOnce(error)
    const queue = createBoardPreviewRefreshQueue(deps)

    queue.schedule([makeBoard('boardA')])
    await flushPromises()

    expect(deps.warn).toHaveBeenCalledWith(
      '[boards] failed to sync refreshed preview',
      'boardA',
      error,
    )
  })

  it('bulk-reads board states once per round instead of once per board', async () => {
    const states = {
      boardA: makeState({ updatedAt: 100 }),
      boardB: makeState({ updatedAt: 100 }),
      boardC: makeState({ updatedAt: 100 }),
    }
    const deps = makeDeps(states)
    const queue = createBoardPreviewRefreshQueue(deps)
    const boards = [makeBoard('boardA'), makeBoard('boardB'), makeBoard('boardC')]

    queue.reloadFromLocal(boards)
    expect(deps.getBoardStates).toHaveBeenCalledTimes(1)
    expect(deps.getBoardStates).toHaveBeenCalledWith(['boardA', 'boardB', 'boardC'])
    expect(vi.mocked(deps.getBoardState)).not.toHaveBeenCalled()

    vi.mocked(deps.getBoardStates).mockClear()
    vi.mocked(deps.getBoardState).mockClear()
    queue.schedule(boards)
    // One bulk read serves the whole scheduling round; the per-board
    // background refreshes that follow re-read only the stale boards they
    // actually render, never the full catalog per board.
    expect(deps.getBoardStates).toHaveBeenCalledTimes(1)
    expect(deps.getBoardStates).toHaveBeenCalledWith(['boardA', 'boardB', 'boardC'])
    await flushPromises()
    expect(queue.previewMap.value.boardA).toBe('data:image/png;base64,preview')
  })

  it('switches preview snapshots when theme changes and caches by theme', async () => {
    const states = {
      boardA: makeState({
        updatedAt: 100,
        previewSnapshotDataUrl: 'light-preview',
        previewSnapshotUpdatedAt: 150,
        previewSnapshotVersion: 8,
      }),
    }
    const deps = makeDeps(states)
    deps.getCurrentTheme = vi.fn((): 'light' => 'light')
    let currentMockTheme: 'dark' | 'light' = 'light'
    deps.getPreviewTheme = vi.fn(() => currentMockTheme)
    deps.setPreviewTheme = vi.fn((_id, theme) => {
      currentMockTheme = theme
    })
    vi.mocked(deps.renderPreview).mockImplementation(async (_state, options) => {
      return options?.isDark ? 'dark-preview-rendered' : 'light-preview-rendered'
    })

    const queue = createBoardPreviewRefreshQueue(deps)
    const boards = [makeBoard('boardA')]

    queue.reloadFromLocal(boards)
    expect(queue.previewMap.value.boardA).toBe('light-preview')

    // Switch theme to dark: since dark preview is not cached yet, queue refreshes in background with isDark: true
    queue.handleThemeChange?.(true, boards)
    await flushPromises()

    expect(deps.renderPreview).toHaveBeenCalledWith(expect.anything(), { isDark: true })
    expect(queue.previewMap.value.boardA).toBe('dark-preview-rendered')
    // Background theme switch preview re-renders must NOT trigger remote sync
    expect(deps.syncBoard).not.toHaveBeenCalled()

    // Switch theme back to light: immediately retrieves cached light preview without re-rendering
    vi.mocked(deps.renderPreview).mockClear()
    queue.handleThemeChange?.(false, boards)
    expect(queue.previewMap.value.boardA).toBe('light-preview')
    expect(deps.renderPreview).not.toHaveBeenCalled()
  })
})
