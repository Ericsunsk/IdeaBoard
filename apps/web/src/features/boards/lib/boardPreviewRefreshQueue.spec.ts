import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import type { CanvasBoardState } from '../../canvas-board/domain'
import { createEmptyCanvasBoardState } from '../../canvas-board/domain'
import {
  createBoardPreviewRefreshQueue,
  type BoardPreviewRefreshQueueDependencies,
} from './boardPreviewRefreshQueue'
import type { CanvasBoardMeta } from '../../canvas-board/catalog/canvasBoards'

function makeBoard(id: string): CanvasBoardMeta {
  return {
    id,
    name: id,
    createdAt: 1,
    updatedAt: 100,
  }
}

function makeState(overrides: Partial<CanvasBoardState> = {}): CanvasBoardState {
  return {
    ...createEmptyCanvasBoardState(100),
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

function makeDeps(
  states: Record<string, CanvasBoardState | null>,
): BoardPreviewRefreshQueueDependencies {
  return {
    getBoardState: vi.fn((boardId: string) => states[boardId] ?? null),
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
    isNameConflictError: vi.fn((error: unknown) => {
      return typeof error === 'object' && !!error && 'status' in error && error.status === 409
    }),
    rendererVersion: 8,
    now: vi.fn(() => 500),
    warn: vi.fn(),
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
      invalidUpdatedAt: 'data:image/png;base64,preview',
      oldVersion: 'data:image/png;base64,preview',
      oldSnapshot: 'data:image/png;base64,preview',
    })
    expect(deps.renderPreview).toHaveBeenCalledTimes(4)
    expect(deps.setPreviewSnapshot).toHaveBeenCalledTimes(4)
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

  it('ignores name conflict errors from background sync', async () => {
    const states = {
      boardA: makeState({ updatedAt: 100 }),
    }
    const deps = makeDeps(states)
    vi.mocked(deps.syncBoard).mockRejectedValueOnce({ status: 409 })
    const queue = createBoardPreviewRefreshQueue(deps)

    queue.schedule([makeBoard('boardA')])
    await flushPromises()

    expect(deps.isNameConflictError).toHaveBeenCalledWith({ status: 409 })
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
})
