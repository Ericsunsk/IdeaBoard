import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { BoardSnapshot, BoardState } from '../../../board'
import { userErrorCode } from '../../../../core/errors/errorCatalog'
import { makeBoardSnapshot, makeBoardState } from '../testing/boardFixtures'
import type { BoardSessionPreviewRenderer } from './preview'
import { BoardSessionRemoteSyncError, type BoardSessionRepository } from './repository'
import {
  BOARD_SESSION_AUTH_UNAVAILABLE_CODE,
  BOARD_SESSION_WORKSPACE_UNAVAILABLE_CODE,
  useBoardSession,
} from './useBoardSession'

function makeSnapshot(marker: string | number): BoardSnapshot {
  return makeBoardSnapshot({
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

function makeState(snapshot: BoardSnapshot): BoardState {
  return makeBoardState({ ...snapshot, updatedAt: 1_000 })
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
    saveLocal: vi.fn(async (_boardId: string, snapshot: BoardSnapshot) => makeState(snapshot)),
    save: vi.fn(async (_boardId: string, snapshot: BoardSnapshot) => makeState(snapshot)),
    setPreview: vi.fn(() => makeState(makeSnapshot(100))),
    sync: vi.fn(async () => undefined),
    ...overrides,
  }
}

describe('useBoardSession', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('stabilizes, saves, refreshes preview, and returns the stabilized snapshot', async () => {
    const events: string[] = []
    const warning = vi.fn()
    const snapshot = makeSnapshot(100)
    const stabilizedSnapshot = makeSnapshot(250)
    const lifecycle = {
      stabilizeSnapshotForSave: vi.fn(async (received: BoardSnapshot) => {
        events.push('stabilize')
        expect(received).toBe(snapshot)
        return stabilizedSnapshot
      }),
    }
    const createUploadLifecycle = vi.fn(() => lifecycle)
    const repository = createRepository({
      save: vi.fn(async (boardId: string, received: BoardSnapshot) => {
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
      render: vi.fn(async (received: BoardSnapshot) => {
        events.push('renderer.render')
        expect(received).toBe(stabilizedSnapshot)
        return 'data:image/webp;base64,preview'
      }),
    }
    const session = useBoardSession({
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
      userUid: 'user-uid',
      boardId: 'board-a',
      warning,
    })
    expect(events).toEqual([
      'stabilize',
      'repository.save',
      'renderer.render',
      'repository.setPreview',
      'repository.sync',
    ])
  })

  it('rejects with BOARD_NOT_FOUND before creating an upload lifecycle', async () => {
    const createUploadLifecycle = vi.fn()
    const repository = createRepository({
      exists: vi.fn(() => false),
      save: vi.fn(),
    })
    const session = useBoardSession({
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

  it('keeps a local draft and reports auth recovery when user identity is incomplete', async () => {
    const createUploadLifecycle = vi.fn()
    const repository = createRepository()
    const snapshot = makeSnapshot(100)
    const session = useBoardSession({
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

    const result = await session.save(snapshot)

    expect(result.snapshot).toBe(snapshot)
    expect(result.remoteSync.ok).toBe(false)
    if (!result.remoteSync.ok) {
      expect(result.remoteSync.reason).toBe('sync-failed')
      expect(result.remoteSync.error).toBeInstanceOf(Error)
      expect((result.remoteSync.error as Error).message).toContain('user session is missing')
      expect(userErrorCode(result.remoteSync.error)).toBe(BOARD_SESSION_AUTH_UNAVAILABLE_CODE)
    }
    expect(createUploadLifecycle).not.toHaveBeenCalled()
    expect(repository.saveLocal).toHaveBeenCalledWith('board-a', snapshot)
    expect(repository.save).not.toHaveBeenCalled()
  })

  it('tags workspace-missing sync failures so the toast names the workspace, not the network', async () => {
    const snapshot = makeSnapshot(100)
    const stabilizedSnapshot = makeSnapshot(250)
    const savedState = makeState(stabilizedSnapshot)
    const workspaceError = Object.assign(
      new Error('Board workspace is unavailable. Select a workspace before syncing boards.'),
      { name: 'BoardWorkspaceUnavailableError' },
    )
    const lifecycle = {
      stabilizeSnapshotForSave: vi.fn(async () => stabilizedSnapshot),
    }
    const repository = createRepository({
      save: vi.fn(async () => {
        throw new BoardSessionRemoteSyncError('board-a', savedState, workspaceError)
      }),
      sync: vi.fn(async () => undefined),
    })
    const session = useBoardSession({
      getBoardIdQuery: () => 'board-a',
      getUserIdentity: () => ({ userId: 1, userUid: 'user-uid' }),
      repository,
      previewRenderer: {
        rendererVersion: 1,
        render: vi.fn(async () => 'data:image/webp;base64,preview'),
      },
      message: { warning: vi.fn() },
      createUploadLifecycle: vi.fn(() => lifecycle),
    })

    const result = await session.save(snapshot)

    expect(result.snapshot).toBe(stabilizedSnapshot)
    expect(result.remoteSync.ok).toBe(false)
    if (!result.remoteSync.ok) {
      expect(result.remoteSync.reason).toBe('sync-failed')
      expect(userErrorCode(result.remoteSync.error)).toBe(BOARD_SESSION_WORKSPACE_UNAVAILABLE_CODE)
    }
    expect((workspaceError as { code?: unknown }).code).toBe(
      BOARD_SESSION_WORKSPACE_UNAVAILABLE_CODE,
    )
    expect(repository.sync).not.toHaveBeenCalled()
  })

  it('keeps a local draft before surfacing upload stabilization failures', async () => {
    const snapshot = makeSnapshot(100)
    const uploadError = new Error('upload auth expired')
    const lifecycle = {
      stabilizeSnapshotForSave: vi.fn(async () => {
        throw uploadError
      }),
    }
    const repository = createRepository()
    const session = useBoardSession({
      getBoardIdQuery: () => 'board-a',
      getUserIdentity: () => ({ userId: 1, userUid: 'user-uid' }),
      repository,
      previewRenderer: {
        rendererVersion: 1,
        render: vi.fn(async () => 'data:image/webp;base64,preview'),
      },
      message: { warning: vi.fn() },
      createUploadLifecycle: vi.fn(() => lifecycle),
    })

    await expect(session.save(snapshot)).rejects.toBe(uploadError)

    expect(repository.saveLocal).toHaveBeenCalledWith('board-a', snapshot)
    expect(repository.save).not.toHaveBeenCalled()
  })

  it('waits for preview renderer and repository sync when waitForPreview is enabled', async () => {
    const syncGate = createDeferred<void>()
    const lifecycle = {
      stabilizeSnapshotForSave: vi.fn(async () => makeSnapshot(250)),
    }
    const createUploadLifecycle = vi.fn(() => lifecycle)
    const repository = createRepository({
      sync: vi.fn(() => syncGate.promise),
    })
    const previewRenderer: BoardSessionPreviewRenderer = {
      rendererVersion: 1,
      render: vi.fn(async () => 'data:image/webp;base64,preview'),
    }
    const session = useBoardSession({
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
    expect(isResolved).toBe(false)

    syncGate.resolve()
    await savePromise

    expect(isResolved).toBe(true)
  })

  it('returns a recoverable remote sync failure without refreshing the preview', async () => {
    const snapshot = makeSnapshot(100)
    const stabilizedSnapshot = makeSnapshot(250)
    const savedState = makeState(stabilizedSnapshot)
    const remoteError = new Error('network down')
    const lifecycle = {
      stabilizeSnapshotForSave: vi.fn(async () => stabilizedSnapshot),
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
    const session = useBoardSession({
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
      remoteSync: { ok: false, reason: 'sync-failed', error: remoteError },
    })
    expect(previewRenderer.render).not.toHaveBeenCalled()
    expect(repository.sync).not.toHaveBeenCalled()
  })

  it('classifies 409 board_archived as archived instead of conflict', async () => {
    const snapshot = makeSnapshot(100)
    const stabilizedSnapshot = makeSnapshot(250)
    const savedState = makeState(stabilizedSnapshot)
    const archivedError = Object.assign(new Error('board is archived'), {
      status: 409,
      code: 'board_archived',
    })
    const lifecycle = {
      stabilizeSnapshotForSave: vi.fn(async () => stabilizedSnapshot),
    }
    const repository = createRepository({
      save: vi.fn(async () => {
        throw new BoardSessionRemoteSyncError('board-a', savedState, archivedError)
      }),
      sync: vi.fn(async () => undefined),
    })
    const previewRenderer: BoardSessionPreviewRenderer = {
      rendererVersion: 1,
      render: vi.fn(async () => 'data:image/webp;base64,preview'),
    }
    const session = useBoardSession({
      getBoardIdQuery: () => 'board-a',
      getUserIdentity: () => ({ userId: 1, userUid: 'user-uid' }),
      repository,
      previewRenderer,
      message: { warning: vi.fn() },
      createUploadLifecycle: vi.fn(() => lifecycle),
    })

    const result = await session.save(snapshot, { waitForPreview: true })

    expect(result.remoteSync).toEqual({ ok: false, reason: 'archived', error: archivedError })
    expect(previewRenderer.render).not.toHaveBeenCalled()
    expect(repository.sync).not.toHaveBeenCalled()
  })

  it.each([
    { code: 'board_revision_conflict', label: 'revision' },
    { code: 'board_idempotency_conflict', label: 'idempotency' },
  ])('classifies 409 $code as a sync conflict, never as archived', async ({ code }) => {
    const snapshot = makeSnapshot(100)
    const stabilizedSnapshot = makeSnapshot(250)
    const savedState = makeState(stabilizedSnapshot)
    const conflictError = Object.assign(new Error(`board ${code}`), {
      status: 409,
      code,
    })
    const lifecycle = {
      stabilizeSnapshotForSave: vi.fn(async () => stabilizedSnapshot),
    }
    const repository = createRepository({
      save: vi.fn(async () => {
        throw new BoardSessionRemoteSyncError('board-a', savedState, conflictError)
      }),
      sync: vi.fn(async () => undefined),
    })
    const previewRenderer: BoardSessionPreviewRenderer = {
      rendererVersion: 1,
      render: vi.fn(async () => 'data:image/webp;base64,preview'),
    }
    const session = useBoardSession({
      getBoardIdQuery: () => 'board-a',
      getUserIdentity: () => ({ userId: 1, userUid: 'user-uid' }),
      repository,
      previewRenderer,
      message: { warning: vi.fn() },
      createUploadLifecycle: vi.fn(() => lifecycle),
    })

    const result = await session.save(snapshot, { waitForPreview: true })

    expect(result.remoteSync).toEqual({ ok: false, reason: 'conflict', error: conflictError })
    expect(previewRenderer.render).not.toHaveBeenCalled()
    expect(repository.sync).not.toHaveBeenCalled()
  })

  it('normalizes board ids for load and exists', async () => {
    const loadedSnapshot = makeSnapshot(180)
    const exists = vi.fn((boardId: string) => boardId === 'route-board' || boardId === 'explicit')
    const load = vi.fn(async (_boardId: string) => loadedSnapshot)
    const repository = createRepository({ exists, load })
    const session = useBoardSession({
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
    const blankSession = useBoardSession({
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
