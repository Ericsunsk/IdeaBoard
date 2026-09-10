import { beforeEach, describe, expect, it, vi } from 'vitest'
import type {
  BoardCommandDependencies,
  BoardCreateResult,
  BoardDeleteResult,
  BoardDuplicateResult,
  BoardRenameResult,
} from './commands'
import { createBoardCommands } from './commands'
import type { BoardMeta } from './boards'

function makeBoard(id: string, name = 'Board'): BoardMeta {
  return {
    id,
    name,
    createdAt: 100,
    updatedAt: 200,
  }
}

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
    syncDeleteBoard: vi.fn(async () => undefined),
    hydrateBoards: vi.fn(async () => ({ ok: true as const })),
    isNameConflictError: vi.fn((error: unknown) => {
      return typeof error === 'object' && !!error && 'status' in error && error.status === 409
    }),
    isNotFoundError: vi.fn((error: unknown) => {
      return typeof error === 'object' && !!error && 'status' in error && error.status === 404
    }),
    clearSyncRollback: vi.fn(),
    isMutationCurrent: vi.fn(() => true),
  }
}

describe('boardCommands', () => {
  let deps: BoardCommandDependencies

  beforeEach(() => {
    deps = makeDeps()
  })

  it('cleans up local meta and state when create remote sync fails', async () => {
    const error = new Error('network failed')
    vi.mocked(deps.syncBoard).mockRejectedValueOnce(error)
    const commands = createBoardCommands(deps)

    const result = (await commands.create()) as BoardCreateResult

    expect(result).toMatchObject({ ok: false, reason: 'sync-failed' })
    expect(deps.createBoard).toHaveBeenCalledWith('Untitled')
    expect(deps.syncBoard).toHaveBeenCalledWith('created-board')
    expect(deps.deleteBoard).toHaveBeenCalledWith('created-board')
    expect(deps.deleteBoardState).toHaveBeenCalledWith('created-board')
    expect(deps.hydrateBoards).not.toHaveBeenCalled()
  })

  it('rolls back rename by hydrating the conflict target when remote reports a name conflict', async () => {
    const error = { status: 409, msg: '画布名称已存在' }
    vi.mocked(deps.syncBoard).mockRejectedValueOnce(error)
    const commands = createBoardCommands(deps)

    const result = (await commands.rename({
      boardId: 'board-a',
      nextName: 'Taken',
    })) as BoardRenameResult

    expect(result).toMatchObject({ ok: false, reason: 'remote-name-conflict' })
    expect(deps.isNameTaken).toHaveBeenCalledWith('Taken', 'board-a')
    expect(deps.renameBoard).toHaveBeenCalledWith('board-a', 'Taken')
    expect(deps.hydrateBoards).toHaveBeenCalledWith({
      preserveLocalNewer: false,
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
      ok: true,
      board: { id: 'board-a', name: 'New Local' },
      previousBoard: { id: 'board-a', name: 'Board' },
    })
    expect(deps.renameBoard).toHaveBeenCalledWith('board-a', 'New Local')
    expect(deps.syncBoard).not.toHaveBeenCalled()

    if (!result.ok) throw new Error('expected optimistic rename to succeed')
    await expect(result.confirm()).resolves.toMatchObject({
      ok: true,
      board: { id: 'board-a', name: 'New Local' },
    })
    expect(deps.syncBoard).toHaveBeenCalledWith('board-a')
  })

  it('keeps optimistic rename local when remote conflict needs caller recovery', async () => {
    const error = { status: 409, msg: '画布名称已存在' }
    vi.mocked(deps.syncBoard).mockRejectedValueOnce(error)
    const commands = createBoardCommands(deps)

    const result = commands.renameOptimistic({
      boardId: 'board-a',
      nextName: 'Taken Remote',
    })
    if (!result.ok) throw new Error('expected optimistic rename to succeed')

    await expect(result.confirm()).resolves.toMatchObject({
      ok: false,
      reason: 'remote-name-conflict',
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

    expect(result).toMatchObject({ ok: true, board: { id: 'board-a', name: 'Board' } })
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

    expect(result).toEqual({ ok: false, reason: 'local-name-conflict' })
    expect(deps.renameBoard).not.toHaveBeenCalled()
    expect(deps.syncBoard).not.toHaveBeenCalled()
  })

  it('cleans up duplicate meta and state when duplicate sync fails', async () => {
    const error = new Error('network failed')
    vi.mocked(deps.syncBoard).mockRejectedValueOnce(error)
    const commands = createBoardCommands(deps)

    const result = (await commands.duplicate({
      sourceBoardId: 'source-board',
    })) as BoardDuplicateResult

    expect(result).toMatchObject({ ok: false, reason: 'sync-failed' })
    expect(deps.duplicateBoard).toHaveBeenCalledWith('source-board')
    expect(deps.duplicateBoardState).toHaveBeenCalledWith('source-board', 'copy-board')
    expect(deps.syncBoard).toHaveBeenCalledWith('copy-board')
    expect(deps.deleteBoard).toHaveBeenCalledWith('copy-board')
    expect(deps.deleteBoardState).toHaveBeenCalledWith('copy-board')
  })

  it('allows duplicate to succeed when source state is missing', async () => {
    vi.mocked(deps.duplicateBoardState).mockReturnValueOnce(false)
    const commands = createBoardCommands(deps)

    const result = (await commands.duplicate({
      sourceBoardId: 'source-board',
    })) as BoardDuplicateResult

    expect(result).toMatchObject({ ok: true, board: { id: 'copy-board' } })
    expect(deps.syncBoard).toHaveBeenCalledWith('copy-board')
    expect(deps.deleteBoard).not.toHaveBeenCalled()
    expect(deps.deleteBoardState).not.toHaveBeenCalled()
  })

  it('cleans local duplicate state and hydrates on duplicate remote name conflict', async () => {
    const error = { status: 409, msg: '画布名称已存在' }
    vi.mocked(deps.syncBoard).mockRejectedValueOnce(error)
    const commands = createBoardCommands(deps)

    const result = (await commands.duplicate({
      sourceBoardId: 'source-board',
    })) as BoardDuplicateResult

    expect(result).toMatchObject({ ok: false, reason: 'name-conflict' })
    expect(deps.deleteBoard).toHaveBeenCalledWith('copy-board')
    expect(deps.deleteBoardState).toHaveBeenCalledWith('copy-board')
    expect(deps.hydrateBoards).toHaveBeenCalledWith()
  })

  it('continues local delete cleanup when remote delete returns not found', async () => {
    vi.mocked(deps.syncDeleteBoard).mockRejectedValueOnce({ status: 404, msg: 'not found' })
    const commands = createBoardCommands(deps)

    const result = (await commands.delete({ boardId: 'board-a' })) as BoardDeleteResult

    expect(result).toEqual({ ok: true, remoteWasMissing: true })
    expect(deps.deleteBoard).toHaveBeenCalledWith('board-a')
    expect(deps.deleteBoardState).toHaveBeenCalledWith('board-a')
  })

  it('does not delete local board when remote delete fails for a non-404 error', async () => {
    const error = new Error('network failed')
    vi.mocked(deps.syncDeleteBoard).mockRejectedValueOnce(error)
    const commands = createBoardCommands(deps)

    const result = (await commands.delete({ boardId: 'board-a' })) as BoardDeleteResult

    expect(result).toMatchObject({ ok: false, reason: 'sync-failed' })
    expect(deps.deleteBoard).not.toHaveBeenCalled()
    expect(deps.deleteBoardState).not.toHaveBeenCalled()
  })
})
