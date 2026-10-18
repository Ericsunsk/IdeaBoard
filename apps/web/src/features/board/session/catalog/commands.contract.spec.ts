import { beforeEach, describe, expect, it, vi } from 'vitest'
import type {
  BoardCommandDependencies,
  BoardCreateResult,
  BoardDeleteResult,
  BoardDuplicateResult,
  BoardRenameResult,
} from './commands'
import { createBoardCommands } from './commands'
import type { WorkspaceBoardMeta } from '../adapters/boardsResource'
import { makeBoardMeta as makeBoard } from '../../testing'

function makeDeps(): BoardCommandDependencies {
  return {
    createBoard: vi.fn(() => makeBoard('created-board', 'Untitled')),
    getBoard: vi.fn((boardId: string) => makeBoard(boardId, 'Board')),
    renameBoard: vi.fn((boardId: string, nextName: string) => makeBoard(boardId, nextName.trim())),
    duplicateBoard: vi.fn(() => makeBoard('copy-board', 'Source Copy')),
    deleteBoard: vi.fn(() => true),
    isNameTaken: vi.fn(() => false),
    duplicateBoardState: vi.fn(() => true),
    deleteBoardState: vi.fn(() => true),
    syncBoard: vi.fn(async () => undefined),
    syncRenameBoard: vi.fn(async () => undefined),
    syncDeleteBoard: vi.fn(async () => undefined),
    hydrateBoards: vi.fn(async () => ({ isOk: true as const })),
    getSyncConflictKind: vi.fn((error: unknown) => {
      if (typeof error !== 'object' || !error) return null
      const record = error as { status?: unknown; code?: unknown }
      if (record.code === 'board_idempotency_conflict') return 'idempotency-conflict' as const
      if (record.status === 409 || record.code === 'board_revision_conflict') {
        return 'revision-conflict' as const
      }
      return null
    }),
    isNotFoundError: vi.fn((error: unknown) => {
      return typeof error === 'object' && !!error && 'status' in error && error.status === 404
    }),
    clearSyncRollback: vi.fn(),
    isMutationCurrent: vi.fn(() => true),
    resolveWorkspace: vi.fn(() => 'ws_test123'),
    fetchSourceDocument: vi.fn(async () => ({
      boardId: 'source-board',
      revision: 2,
      schemaVersion: 4,
      document: {},
      updatedAt: 300,
    })),
    createServerBoard: vi.fn(
      async (_workspaceId: string, name: string): Promise<WorkspaceBoardMeta> => ({
        id: 'board-server-1',
        name,
        revision: 1,
        schemaVersion: 4,
        createdAt: 100,
        updatedAt: 200,
        contentVersion: '',
        previewScene: null,
        // A minted board carries the server access fact (none, creator-only).
        capabilities: { read: true, write: true, manage: true, generate: true },
        accessState: 'private',
        workspaceAccess: 'none',
        isCreator: true,
        hasDirectGrant: false,
      }),
    ),
    archiveServerBoard: vi.fn(async (): Promise<WorkspaceBoardMeta> => ({
      id: 'board-server-1',
      name: 'Untitled',
      revision: 1,
      schemaVersion: 4,
      createdAt: 100,
      updatedAt: 200,
      contentVersion: '',
      previewScene: null,
      capabilities: { read: true, write: true, manage: true, generate: true },
      accessState: 'private',
      workspaceAccess: 'none',
      isCreator: true,
      hasDirectGrant: false,
    })),
    registerServerBoard: vi.fn((input: { id: string; name: string; revision?: number }) =>
      makeBoard(input.id, input.name),
    ),
  }
}

describe('boardCommands', () => {
  let deps: BoardCommandDependencies

  beforeEach(() => {
    deps = makeDeps()
  })

  it('creates the board on the server first, then syncs the minted identity', async () => {
    const commands = createBoardCommands(deps)

    const result = (await commands.create()) as BoardCreateResult

    expect(result).toMatchObject({ isOk: true, board: { id: 'board-server-1' } })
    expect(deps.createBoard).toHaveBeenCalledWith('Untitled')
    expect(deps.createServerBoard).toHaveBeenCalledWith('ws_test123', 'Untitled')
    expect(deps.syncBoard).toHaveBeenCalledWith('board-server-1')
    expect(deps.deleteBoard).toHaveBeenCalledWith('created-board')
  })

  it('cleans up local meta and state when create remote sync fails', async () => {
    const error = new Error('network failed')
    vi.mocked(deps.syncBoard).mockRejectedValueOnce(error)
    const commands = createBoardCommands(deps)

    const result = (await commands.create()) as BoardCreateResult

    expect(result).toMatchObject({ isOk: false, reason: 'sync-failed' })
    expect(deps.createBoard).toHaveBeenCalledWith('Untitled')
    expect(deps.syncBoard).toHaveBeenCalledWith('board-server-1')
    expect(deps.deleteBoard).toHaveBeenCalledWith('board-server-1')
    expect(deps.deleteBoardState).toHaveBeenCalledWith('board-server-1')
    expect(deps.hydrateBoards).not.toHaveBeenCalled()
  })

  it('fails create closed when no workspace is selected', async () => {
    const workspaceError = new Error('Board workspace is unavailable.')
    vi.mocked(deps.resolveWorkspace!).mockImplementationOnce(() => {
      throw workspaceError
    })
    const commands = createBoardCommands(deps)

    const result = (await commands.create()) as BoardCreateResult

    expect(result).toMatchObject({ isOk: false, reason: 'sync-failed' })
    expect(deps.createServerBoard).not.toHaveBeenCalled()
    expect(deps.syncBoard).not.toHaveBeenCalled()
    expect(deps.deleteBoard).toHaveBeenCalledWith('created-board')
  })

  it('archives the server board in the captured workspace when scope changes mid-create', async () => {
    vi.mocked(deps.isMutationCurrent).mockReturnValue(false)
    const commands = createBoardCommands(deps)

    const result = (await commands.create()) as BoardCreateResult

    expect(result).toEqual({ isOk: false, reason: 'scope-changed' })
    expect(deps.createServerBoard).toHaveBeenCalledWith('ws_test123', 'Untitled')
    expect(deps.archiveServerBoard).toHaveBeenCalledWith('ws_test123', 'board-server-1')
    expect(deps.registerServerBoard).not.toHaveBeenCalled()
    expect(deps.syncBoard).not.toHaveBeenCalled()
  })

  it('rolls back rename by hydrating the conflict target on revision conflicts', async () => {
    const error = { status: 409, code: 'board_revision_conflict', msg: '看板版本已变更' }
    vi.mocked(deps.syncRenameBoard).mockRejectedValueOnce(error)
    const commands = createBoardCommands(deps)

    const result = (await commands.rename({
      boardId: 'board-a',
      nextName: 'Taken',
    })) as BoardRenameResult

    expect(result).toMatchObject({ isOk: false, reason: 'revision-conflict' })
    expect(deps.isNameTaken).toHaveBeenCalledWith('Taken', 'board-a')
    expect(deps.renameBoard).toHaveBeenCalledWith('board-a', 'Taken')
    expect(deps.syncRenameBoard).toHaveBeenCalledWith('board-a', 'Taken')
    expect(deps.syncBoard).not.toHaveBeenCalled()
    expect(deps.hydrateBoards).toHaveBeenCalledWith({
      shouldPreserveLocalNewer: false,
      rollbackBoardIds: ['board-a'],
    })
  })

  it('returns an optimistic rename before remote confirmation resolves', async () => {
    const commands = createBoardCommands(deps)

    const result = commands.renameOptimistic({
      boardId: 'board-a',
      nextName: 'New Local',
    })

    expect(result).toMatchObject({
      isOk: true,
      board: { id: 'board-a', name: 'New Local' },
      previousBoard: { id: 'board-a', name: 'Board' },
    })
    expect(deps.renameBoard).toHaveBeenCalledWith('board-a', 'New Local')
    expect(deps.syncRenameBoard).not.toHaveBeenCalled()
    expect(deps.syncBoard).not.toHaveBeenCalled()

    if (!result.isOk) throw new Error('expected optimistic rename to succeed')
    await expect(result.confirm()).resolves.toMatchObject({
      isOk: true,
      board: { id: 'board-a', name: 'New Local' },
    })
    expect(deps.syncRenameBoard).toHaveBeenCalledWith('board-a', 'New Local')
    expect(deps.syncBoard).not.toHaveBeenCalled()
  })

  it('keeps optimistic rename local when a revision conflict needs caller recovery', async () => {
    const error = { status: 409, code: 'board_revision_conflict', msg: '看板版本已变更' }
    vi.mocked(deps.syncRenameBoard).mockRejectedValueOnce(error)
    const commands = createBoardCommands(deps)

    const result = commands.renameOptimistic({
      boardId: 'board-a',
      nextName: 'Taken Remote',
    })
    if (!result.isOk) throw new Error('expected optimistic rename to succeed')

    await expect(result.confirm()).resolves.toMatchObject({
      isOk: false,
      reason: 'revision-conflict',
      boardId: 'board-a',
      board: { id: 'board-a', name: 'Taken Remote' },
      previousBoard: { id: 'board-a', name: 'Board' },
      error,
    })
    expect(deps.hydrateBoards).not.toHaveBeenCalled()
  })

  it('rolls back an optimistic rename locally and clears the pending rollback marker', () => {
    const commands = createBoardCommands(deps)

    const result = commands.rollbackOptimisticRename({
      boardId: 'board-a',
      previousName: 'Board',
    })

    expect(result).toMatchObject({ isOk: true, board: { id: 'board-a', name: 'Board' } })
    expect(deps.renameBoard).toHaveBeenCalledWith('board-a', 'Board')
    expect(deps.clearSyncRollback).toHaveBeenCalledWith('board-a')
  })

  it('stops rename before local mutation when the name is already taken locally', async () => {
    vi.mocked(deps.isNameTaken).mockReturnValueOnce(true)
    const commands = createBoardCommands(deps)

    const result = (await commands.rename({
      boardId: 'board-a',
      nextName: 'Existing',
    })) as BoardRenameResult

    expect(result).toEqual({ isOk: false, reason: 'local-name-conflict' })
    expect(deps.renameBoard).not.toHaveBeenCalled()
    expect(deps.syncRenameBoard).not.toHaveBeenCalled()
    expect(deps.syncBoard).not.toHaveBeenCalled()
  })

  it('cleans up duplicate meta and state when duplicate sync fails', async () => {
    const error = new Error('network failed')
    vi.mocked(deps.syncBoard).mockRejectedValueOnce(error)
    const commands = createBoardCommands(deps)

    const result = (await commands.duplicate({
      sourceBoardId: 'source-board',
    })) as BoardDuplicateResult

    expect(result).toMatchObject({ isOk: false, reason: 'sync-failed' })
    expect(deps.duplicateBoard).toHaveBeenCalledWith('source-board')
    expect(deps.createServerBoard).toHaveBeenCalledWith('ws_test123', 'Source Copy')
    expect(deps.duplicateBoardState).toHaveBeenCalledWith('source-board', 'board-server-1')
    expect(deps.syncBoard).toHaveBeenCalledWith('board-server-1')
    expect(deps.deleteBoard).toHaveBeenCalledWith('board-server-1')
    expect(deps.deleteBoardState).toHaveBeenCalledWith('board-server-1')
  })

  it('allows duplicate to succeed when source state is missing', async () => {
    vi.mocked(deps.duplicateBoardState).mockReturnValueOnce(false)
    const commands = createBoardCommands(deps)

    const result = (await commands.duplicate({
      sourceBoardId: 'source-board',
    })) as BoardDuplicateResult

    expect(result).toMatchObject({ isOk: true, board: { id: 'board-server-1' } })
    expect(deps.syncBoard).toHaveBeenCalledWith('board-server-1')
    expect(deps.deleteBoard).toHaveBeenCalledWith('copy-board')
    expect(deps.deleteBoardState).not.toHaveBeenCalled()
  })

  it('registers the minted copy with the server access fact, never the source ACL', async () => {
    const commands = createBoardCommands(deps)

    await commands.duplicate({ sourceBoardId: 'source-board' })

    // The server mints workspace_access=none creator-only: registration
    // carries the minted capabilities verbatim so a copy never inherits
    // the source grants.
    expect(deps.registerServerBoard).toHaveBeenCalledWith({
      id: 'board-server-1',
      name: 'Source Copy',
      revision: 1,
      capabilities: { read: true, write: true, manage: true, generate: true },
      accessState: 'private',
      workspaceAccess: 'none',
      isCreator: true,
      hasDirectGrant: false,
    })
  })

  it('archives the duplicated server board in the captured workspace when scope changes', async () => {
    vi.mocked(deps.isMutationCurrent).mockReturnValue(false)
    const commands = createBoardCommands(deps)

    const result = (await commands.duplicate({
      sourceBoardId: 'source-board',
    })) as BoardDuplicateResult

    expect(result).toEqual({ isOk: false, reason: 'scope-changed' })
    expect(deps.createServerBoard).toHaveBeenCalledWith('ws_test123', 'Source Copy')
    expect(deps.archiveServerBoard).toHaveBeenCalledWith('ws_test123', 'board-server-1')
    expect(deps.registerServerBoard).not.toHaveBeenCalled()
    expect(deps.duplicateBoardState).not.toHaveBeenCalled()
    expect(deps.syncBoard).not.toHaveBeenCalled()
  })

  it('maps idempotency mismatches to idempotency-conflict on create', async () => {
    const error = { status: 409, code: 'board_idempotency_conflict', msg: '幂等冲突' }
    vi.mocked(deps.createServerBoard!).mockRejectedValueOnce(error)
    const commands = createBoardCommands(deps)

    const result = (await commands.create()) as BoardCreateResult

    expect(result).toMatchObject({ isOk: false, reason: 'idempotency-conflict' })
    expect(deps.hydrateBoards).toHaveBeenCalledWith()
  })

  it('cleans local duplicate state and hydrates on duplicate revision conflict', async () => {
    const error = { status: 409, code: 'board_revision_conflict', msg: '看板版本已变更' }
    vi.mocked(deps.syncBoard).mockRejectedValueOnce(error)
    const commands = createBoardCommands(deps)

    const result = (await commands.duplicate({
      sourceBoardId: 'source-board',
    })) as BoardDuplicateResult

    expect(result).toMatchObject({ isOk: false, reason: 'revision-conflict' })
    expect(deps.deleteBoard).toHaveBeenCalledWith('board-server-1')
    expect(deps.deleteBoardState).toHaveBeenCalledWith('board-server-1')
    expect(deps.hydrateBoards).toHaveBeenCalledWith()
  })

  it('proves source readability before minting the copy', async () => {
    const commands = createBoardCommands(deps)

    const result = (await commands.duplicate({
      sourceBoardId: 'source-board',
    })) as BoardDuplicateResult

    expect(result).toMatchObject({ isOk: true, board: { id: 'board-server-1' } })
    expect(deps.fetchSourceDocument).toHaveBeenCalledWith('ws_test123', 'source-board')
    expect(deps.createServerBoard).toHaveBeenCalledWith('ws_test123', 'Source Copy')
  })

  it('fails duplicate closed without minting when the source is unreadable', async () => {
    const error = { status: 404, code: 'board_not_found', msg: 'not found' }
    vi.mocked(deps.fetchSourceDocument!).mockRejectedValueOnce(error)
    const commands = createBoardCommands(deps)

    const result = (await commands.duplicate({
      sourceBoardId: 'source-board',
    })) as BoardDuplicateResult

    expect(result).toMatchObject({ isOk: false, reason: 'source-unreadable' })
    expect(deps.createServerBoard).not.toHaveBeenCalled()
    expect(deps.registerServerBoard).not.toHaveBeenCalled()
    expect(deps.syncBoard).not.toHaveBeenCalled()
    expect(deps.deleteBoard).toHaveBeenCalledWith('copy-board')
  })

  it('archives the minted server board when the copy document sync fails', async () => {
    const error = new Error('network failed')
    vi.mocked(deps.syncBoard).mockRejectedValueOnce(error)
    const commands = createBoardCommands(deps)

    const result = (await commands.duplicate({
      sourceBoardId: 'source-board',
    })) as BoardDuplicateResult

    expect(result).toMatchObject({ isOk: false, reason: 'sync-failed' })
    expect(deps.archiveServerBoard).toHaveBeenCalledWith('ws_test123', 'board-server-1')
  })

  it('skips the archive compensation when duplicate never minted', async () => {
    const error = { status: 404, code: 'board_not_found', msg: 'not found' }
    vi.mocked(deps.fetchSourceDocument!).mockRejectedValueOnce(error)
    const commands = createBoardCommands(deps)

    await commands.duplicate({ sourceBoardId: 'source-board' })

    expect(deps.archiveServerBoard).not.toHaveBeenCalled()
  })

  it('continues local delete cleanup when remote delete returns not found', async () => {
    vi.mocked(deps.syncDeleteBoard).mockRejectedValueOnce({ status: 404, msg: 'not found' })
    const commands = createBoardCommands(deps)

    const result = (await commands.delete({ boardId: 'board-a' })) as BoardDeleteResult

    expect(result).toEqual({ isOk: true, isRemoteWasMissing: true })
    expect(deps.deleteBoard).toHaveBeenCalledWith('board-a')
    expect(deps.deleteBoardState).toHaveBeenCalledWith('board-a')
  })

  it('does not delete local board when remote delete fails for a non-404 error', async () => {
    const error = new Error('network failed')
    vi.mocked(deps.syncDeleteBoard).mockRejectedValueOnce(error)
    const commands = createBoardCommands(deps)

    const result = (await commands.delete({ boardId: 'board-a' })) as BoardDeleteResult

    expect(result).toMatchObject({ isOk: false, reason: 'sync-failed' })
    expect(deps.deleteBoard).not.toHaveBeenCalled()
    expect(deps.deleteBoardState).not.toHaveBeenCalled()
  })
})
