import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { BoardSnapshot, BoardState } from '../../../board'
import { userErrorCode } from '../../../../core/errors/errorCatalog'
import { makeBoardSnapshot, makeBoardState } from '../../../board/ports/testing'
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

function createRepository(overrides: Partial<BoardSessionRepository> = {}): BoardSessionRepository {
  return {
    exists: vi.fn(() => true),
    load: vi.fn(async () => makeSnapshot(100)),
    saveLocal: vi.fn(async (_boardId: string, snapshot: BoardSnapshot) => makeState(snapshot)),
    save: vi.fn(async (_boardId: string, snapshot: BoardSnapshot) => makeState(snapshot)),
    sync: vi.fn(async () => undefined),
    ...overrides,
  }
}

describe('useBoardSession', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('stabilizes, saves once, and returns the stabilized snapshot', async () => {
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
      sync: vi.fn(async (boardId: string) => {
        events.push('repository.sync')
        expect(boardId).toBe('board-a')
      }),
    })
    const session = useBoardSession({
      getBoardIdQuery: () => '  board-a  ',
      getUserIdentity: () => ({ userId: 1, userUid: 'user-uid' }),
      repository,
      message: { warning },
      createUploadLifecycle,
    })

    const result = await session.save(snapshot)

    expect(result.snapshot).toBe(stabilizedSnapshot)
    expect(result.remoteSync).toEqual({ isOk: true })
    expect(createUploadLifecycle).toHaveBeenCalledWith({
      userUid: 'user-uid',
      boardId: 'board-a',
      warning,
    })
    // Canvas only saves the document: no preview render, no preview write,
    // and no second sync ride the save.
    expect(events).toEqual(['stabilize', 'repository.save'])
    expect(repository.sync).not.toHaveBeenCalled()
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
      message: { warning: vi.fn() },
      createUploadLifecycle,
    })

    const result = await session.save(snapshot)

    expect(result.snapshot).toBe(snapshot)
    expect(result.remoteSync.isOk).toBe(false)
    if (!result.remoteSync.isOk) {
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
      message: { warning: vi.fn() },
      createUploadLifecycle: vi.fn(() => lifecycle),
    })

    const result = await session.save(snapshot)

    expect(result.snapshot).toBe(stabilizedSnapshot)
    expect(result.remoteSync.isOk).toBe(false)
    if (!result.remoteSync.isOk) {
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
      message: { warning: vi.fn() },
      createUploadLifecycle: vi.fn(() => lifecycle),
    })

    await expect(session.save(snapshot)).rejects.toBe(uploadError)

    expect(repository.saveLocal).toHaveBeenCalledWith('board-a', snapshot)
    expect(repository.save).not.toHaveBeenCalled()
  })

  it('resolves the save without a second sync once the document persists', async () => {
    const lifecycle = {
      stabilizeSnapshotForSave: vi.fn(async () => makeSnapshot(250)),
    }
    const createUploadLifecycle = vi.fn(() => lifecycle)
    const repository = createRepository()
    const session = useBoardSession({
      getBoardIdQuery: () => 'board-a',
      getUserIdentity: () => ({ userId: 1, userUid: 'user-uid' }),
      repository,
      message: { warning: vi.fn() },
      createUploadLifecycle,
    })

    const result = await session.save(makeSnapshot(100))

    expect(result.remoteSync).toEqual({ isOk: true })
    expect(repository.save).toHaveBeenCalledOnce()
    expect(repository.sync).not.toHaveBeenCalled()
  })

  it('returns a recoverable remote sync failure without a second sync', async () => {
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
    const session = useBoardSession({
      getBoardIdQuery: () => 'board-a',
      getUserIdentity: () => ({ userId: 1, userUid: 'user-uid' }),
      repository,
      message: { warning: vi.fn() },
      createUploadLifecycle: vi.fn(() => lifecycle),
    })

    const result = await session.save(snapshot)

    expect(result).toEqual({
      snapshot: stabilizedSnapshot,
      remoteSync: { isOk: false, reason: 'sync-failed', error: remoteError },
    })
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
    const session = useBoardSession({
      getBoardIdQuery: () => 'board-a',
      getUserIdentity: () => ({ userId: 1, userUid: 'user-uid' }),
      repository,
      message: { warning: vi.fn() },
      createUploadLifecycle: vi.fn(() => lifecycle),
    })

    const result = await session.save(snapshot)

    expect(result.remoteSync).toEqual({ isOk: false, reason: 'archived', error: archivedError })
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
    const session = useBoardSession({
      getBoardIdQuery: () => 'board-a',
      getUserIdentity: () => ({ userId: 1, userUid: 'user-uid' }),
      repository,
      message: { warning: vi.fn() },
      createUploadLifecycle: vi.fn(() => lifecycle),
    })

    const result = await session.save(snapshot)

    expect(result.remoteSync).toEqual({ isOk: false, reason: 'conflict', error: conflictError })
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
