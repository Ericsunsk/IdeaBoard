import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { CanvasBoardSnapshot, CanvasBoardState } from '../../canvas-board/domain'
import { makeCanvasBoardSnapshot, makeCanvasBoardState } from '../../canvas-board/testing'
import type { BoardSessionPreviewRenderer } from './boardSessionPreview'
import {
  BoardSessionRemoteSyncError,
  type BoardSessionRepository,
} from './boardSessionRepository'
import { useCanvasBoardSession } from './useCanvasBoardSession'

function makeSnapshot(marker: string | number): CanvasBoardSnapshot {
  return makeCanvasBoardSnapshot({
    promptNodes: [
      {
        id: 1,
        type: 'prompt',
        x: 0,
        y: 0,
        width: 280,
        text: `snapshot-${marker}`,
        outputCount: 1,
        aspectRatio: 'auto',
        inputImages: [],
      },
    ],
  })
}

function makeState(snapshot: CanvasBoardSnapshot): CanvasBoardState {
  return makeCanvasBoardState({ ...snapshot, updatedAt: 1_000 })
}

function createDeferred<T = void>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

async function flushMicrotasks(times = 5) {
  for (let index = 0; index < times; index += 1) {
    await Promise.resolve()
  }
}

function createRepository(overrides: Partial<BoardSessionRepository> = {}): BoardSessionRepository {
  return {
    exists: vi.fn(() => true),
    load: vi.fn(async () => makeSnapshot(100)),
    save: vi.fn(async (_boardId: string, snapshot: CanvasBoardSnapshot) => makeState(snapshot)),
    setPreview: vi.fn(() => makeState(makeSnapshot(100))),
    sync: vi.fn(async () => undefined),
    ...overrides,
  }
}

describe('useCanvasBoardSession', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('stabilizes, saves, refreshes preview, recycles removed uploads, and returns the stabilized snapshot', async () => {
    const events: string[] = []
    const warning = vi.fn()
    const snapshot = makeSnapshot(100)
    const previousSnapshot = makeSnapshot(50)
    const stabilizedSnapshot = makeSnapshot(250)
    const lifecycle = {
      readPreviousSnapshot: vi.fn(() => {
        events.push('readPrevious')
        return previousSnapshot
      }),
      stabilizeSnapshotForSave: vi.fn(async (received: CanvasBoardSnapshot) => {
        events.push('stabilize')
        expect(received).toBe(snapshot)
        return stabilizedSnapshot
      }),
      recycleRemovedUploadsAfterSave: vi.fn(async () => {
        events.push('recycle')
      }),
    }
    const createUploadLifecycle = vi.fn(() => lifecycle)
    const repository = createRepository({
      save: vi.fn(async (boardId: string, received: CanvasBoardSnapshot) => {
        events.push('repository.save')
        expect(boardId).toBe('board-a')
        expect(received).toBe(stabilizedSnapshot)
        return makeState(received)
      }),
      setPreview: vi.fn(
        (
          boardId: string,
          previewSnapshotDataUrl: string | undefined,
          previewSnapshotUpdatedAt: number | undefined,
          previewSnapshotVersion: number | undefined,
        ) => {
          events.push('repository.setPreview')
          expect(boardId).toBe('board-a')
          expect(previewSnapshotDataUrl).toBe('data:image/webp;base64,preview')
          expect(previewSnapshotUpdatedAt).toEqual(expect.any(Number))
          expect(previewSnapshotVersion).toBe(42)
          return makeState(stabilizedSnapshot)
        },
      ),
      sync: vi.fn(async (boardId: string) => {
        events.push('repository.sync')
        expect(boardId).toBe('board-a')
      }),
    })
    const previewRenderer: BoardSessionPreviewRenderer = {
      rendererVersion: 42,
      render: vi.fn(async (received: CanvasBoardSnapshot) => {
        events.push('renderer.render')
        expect(received).toBe(stabilizedSnapshot)
        return 'data:image/webp;base64,preview'
      }),
    }
    const session = useCanvasBoardSession({
      getBoardIdQuery: () => '  board-a  ',
      getUserIdentity: () => ({ userId: 1, userUid: 'user-uid' }),
      repository,
      previewRenderer,
      message: { warning },
      createUploadLifecycle,
    })

    const result = await session.save(snapshot, { waitForPreview: true })

    expect(result.snapshot).toBe(stabilizedSnapshot)
    expect(result.remoteSync).toEqual({ ok: true })
    expect(createUploadLifecycle).toHaveBeenCalledWith({
      userId: 1,
      userUid: 'user-uid',
      boardId: 'board-a',
      warning,
    })
    expect(lifecycle.recycleRemovedUploadsAfterSave).toHaveBeenCalledWith(
      previousSnapshot,
      stabilizedSnapshot,
    )
    expect(events).toEqual([
      'readPrevious',
      'stabilize',
      'repository.save',
      'renderer.render',
      'repository.setPreview',
      'repository.sync',
      'recycle',
    ])
  })

  it('rejects with BOARD_NOT_FOUND before creating an upload lifecycle', async () => {
    const createUploadLifecycle = vi.fn()
    const repository = createRepository({
      exists: vi.fn(() => false),
      save: vi.fn(),
    })
    const session = useCanvasBoardSession({
      getBoardIdQuery: () => 'missing-board',
      getUserIdentity: () => ({ userId: 1, userUid: 'user-uid' }),
      repository,
      previewRenderer: {
        rendererVersion: 1,
        render: vi.fn(async () => 'data:image/webp;base64,preview'),
      },
      message: { warning: vi.fn() },
      createUploadLifecycle,
    })

    await expect(session.save(makeSnapshot(100))).rejects.toThrow('BOARD_NOT_FOUND')

    expect(createUploadLifecycle).not.toHaveBeenCalled()
    expect(repository.save).not.toHaveBeenCalled()
  })

  it('rejects when user identity is incomplete', async () => {
    const createUploadLifecycle = vi.fn()
    const repository = createRepository()
    const session = useCanvasBoardSession({
      getBoardIdQuery: () => 'board-a',
      getUserIdentity: () => ({ userId: 0, userUid: '' }),
      repository,
      previewRenderer: {
        rendererVersion: 1,
        render: vi.fn(async () => 'data:image/webp;base64,preview'),
      },
      message: { warning: vi.fn() },
      createUploadLifecycle,
    })

    await expect(session.save(makeSnapshot(100))).rejects.toThrow('缺少用户信息')

    expect(createUploadLifecycle).not.toHaveBeenCalled()
    expect(repository.save).not.toHaveBeenCalled()
  })

  it('waits for preview renderer and repository sync when waitForPreview is enabled', async () => {
    const syncGate = createDeferred<void>()
    const lifecycle = {
      readPreviousSnapshot: vi.fn(() => makeSnapshot(50)),
      stabilizeSnapshotForSave: vi.fn(async () => makeSnapshot(250)),
      recycleRemovedUploadsAfterSave: vi.fn(async () => undefined),
    }
    const createUploadLifecycle = vi.fn(() => lifecycle)
    const repository = createRepository({
      sync: vi.fn(() => syncGate.promise),
    })
    const previewRenderer: BoardSessionPreviewRenderer = {
      rendererVersion: 1,
      render: vi.fn(async () => 'data:image/webp;base64,preview'),
    }
    const session = useCanvasBoardSession({
      getBoardIdQuery: () => 'board-a',
      getUserIdentity: () => ({ userId: 1, userUid: 'user-uid' }),
      repository,
      previewRenderer,
      message: { warning: vi.fn() },
      createUploadLifecycle,
    })
    let isResolved = false

    const savePromise = session.save(makeSnapshot(100), { waitForPreview: true }).then(() => {
      isResolved = true
    })
    await flushMicrotasks()

    expect(previewRenderer.render).toHaveBeenCalledOnce()
    expect(repository.sync).toHaveBeenCalledOnce()
    expect(lifecycle.recycleRemovedUploadsAfterSave).not.toHaveBeenCalled()
    expect(isResolved).toBe(false)

    syncGate.resolve()
    await savePromise

    expect(lifecycle.recycleRemovedUploadsAfterSave).toHaveBeenCalledOnce()
    expect(isResolved).toBe(true)
  })

  it('returns a recoverable remote sync failure without recycling removed uploads', async () => {
    const snapshot = makeSnapshot(100)
    const stabilizedSnapshot = makeSnapshot(250)
    const savedState = makeState(stabilizedSnapshot)
    const remoteError = new Error('network down')
    const lifecycle = {
      readPreviousSnapshot: vi.fn(() => makeSnapshot(50)),
      stabilizeSnapshotForSave: vi.fn(async () => stabilizedSnapshot),
      recycleRemovedUploadsAfterSave: vi.fn(async () => undefined),
    }
    const repository = createRepository({
      save: vi.fn(async () => {
        throw new BoardSessionRemoteSyncError('board-a', savedState, remoteError)
      }),
      sync: vi.fn(async () => undefined),
    })
    const previewRenderer: BoardSessionPreviewRenderer = {
      rendererVersion: 1,
      render: vi.fn(async () => 'data:image/webp;base64,preview'),
    }
    const session = useCanvasBoardSession({
      getBoardIdQuery: () => 'board-a',
      getUserIdentity: () => ({ userId: 1, userUid: 'user-uid' }),
      repository,
      previewRenderer,
      message: { warning: vi.fn() },
      createUploadLifecycle: vi.fn(() => lifecycle),
    })

    const result = await session.save(snapshot, { waitForPreview: true })

    expect(result).toEqual({
      snapshot: stabilizedSnapshot,
      remoteSync: { ok: false, error: remoteError },
    })
    expect(previewRenderer.render).not.toHaveBeenCalled()
    expect(repository.sync).not.toHaveBeenCalled()
    expect(lifecycle.recycleRemovedUploadsAfterSave).not.toHaveBeenCalled()
  })

  it('normalizes board ids for load and exists', async () => {
    const loadedSnapshot = makeSnapshot(180)
    const exists = vi.fn((boardId: string) => boardId === 'route-board' || boardId === 'explicit')
    const load = vi.fn(async (_boardId: string) => loadedSnapshot)
    const repository = createRepository({ exists, load })
    const session = useCanvasBoardSession({
      getBoardIdQuery: () => ['  route-board  '],
      getUserIdentity: () => ({ userId: 1, userUid: 'user-uid' }),
      repository,
      previewRenderer: {
        rendererVersion: 1,
        render: vi.fn(async () => 'data:image/webp;base64,preview'),
      },
      message: { warning: vi.fn() },
      createUploadLifecycle: vi.fn(),
    })

    expect(session.currentBoardId.value).toBe('route-board')
    expect(session.exists()).toBe(true)
    expect(await session.load()).toBe(loadedSnapshot)
    expect(session.exists('  explicit  ')).toBe(true)
    expect(await session.load('  explicit  ')).toBe(loadedSnapshot)
    expect(exists).toHaveBeenNthCalledWith(1, 'route-board')
    expect(load).toHaveBeenNthCalledWith(1, 'route-board')
    expect(exists).toHaveBeenNthCalledWith(2, 'explicit')
    expect(load).toHaveBeenNthCalledWith(2, 'explicit')

    const blankRepository = createRepository({
      exists: vi.fn(),
      load: vi.fn(),
    })
    const blankSession = useCanvasBoardSession({
      getBoardIdQuery: () => '   ',
      getUserIdentity: () => ({ userId: 1, userUid: 'user-uid' }),
      repository: blankRepository,
      previewRenderer: {
        rendererVersion: 1,
        render: vi.fn(async () => 'data:image/webp;base64,preview'),
      },
      message: { warning: vi.fn() },
      createUploadLifecycle: vi.fn(),
    })

    expect(blankSession.currentBoardId.value).toBeNull()
    expect(blankSession.exists()).toBe(false)
    expect(await blankSession.load()).toBeNull()
    expect(blankSession.exists('   ')).toBe(false)
    expect(await blankSession.load('   ')).toBeNull()
    expect(blankRepository.exists).not.toHaveBeenCalled()
    expect(blankRepository.load).not.toHaveBeenCalled()
  })
})
