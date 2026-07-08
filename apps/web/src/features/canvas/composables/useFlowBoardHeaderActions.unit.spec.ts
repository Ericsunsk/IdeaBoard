import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import type { Router } from 'vue-router'
import { clearStoredAuth, setStoredAuth } from '../../../shared/utils/auth'
import {
  getCanvasBoardById,
  isCanvasBoardNameTaken,
  renameCanvasBoard,
  replaceCanvasBoards,
  type CanvasBoardMeta,
} from '../../canvas-board/catalog/canvasBoards'
import type {
  CanvasBoardCommands,
  CanvasBoardOptimisticRenameResult,
  CanvasBoardRenameRemoteResult,
  CanvasBoardRollbackRenameResult,
} from '../../canvas-board/catalog/canvasBoardCommands'
import { useFlowBoardHeaderActions } from './useFlowBoardHeaderActions'

function makeRouter(): Router {
  return {
    push: vi.fn(async () => undefined),
  } as unknown as Router
}

function makeMessage() {
  return {
    warning: vi.fn(),
    success: vi.fn(),
    error: vi.fn(),
  }
}

function setAuthScope() {
  setStoredAuth({
    id: 1,
    uid: 'ABC123',
    username: 'tester',
    avatar: '',
    role: 'user',
    status: 1,
    created_at: '2026-01-01T00:00:00.000Z',
    updated_at: '2026-01-01T00:00:00.000Z',
  })
}

function seedBoards() {
  replaceCanvasBoards([
    { id: 'board-a', name: 'Old Board', createdAt: 100, updatedAt: 200 },
    { id: 'board-b', name: 'Taken', createdAt: 100, updatedAt: 150 },
  ])
}

function cloneBoard(board: CanvasBoardMeta): CanvasBoardMeta {
  return {
    id: board.id,
    name: board.name,
    createdAt: board.createdAt,
    updatedAt: board.updatedAt,
  }
}

function createDeferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve
    reject = promiseReject
  })
  return { promise, resolve, reject }
}

function makeOptimisticBoardCommands(
  confirm: () => Promise<CanvasBoardRenameRemoteResult>,
): Pick<CanvasBoardCommands, 'renameOptimistic' | 'rollbackOptimisticRename'> {
  return {
    renameOptimistic: vi.fn(({ boardId, nextName }): CanvasBoardOptimisticRenameResult => {
      if (isCanvasBoardNameTaken(nextName, boardId)) {
        return { ok: false, reason: 'local-name-conflict' }
      }
      const previousBoard = getCanvasBoardById(boardId)
      if (!previousBoard) return { ok: false, reason: 'invalid-name' }
      const previous = cloneBoard(previousBoard)
      const board = renameCanvasBoard(boardId, nextName)
      if (!board) return { ok: false, reason: 'invalid-name' }
      return {
        ok: true,
        board: cloneBoard(board),
        previousBoard: previous,
        confirm,
      }
    }),
    rollbackOptimisticRename: vi.fn(
      ({ boardId, previousName }): CanvasBoardRollbackRenameResult => {
        if (isCanvasBoardNameTaken(previousName, boardId)) {
          return { ok: false, reason: 'local-name-conflict' }
        }
        const board = renameCanvasBoard(boardId, previousName)
        if (!board) return { ok: false, reason: 'invalid-name' }
        return { ok: true, board: cloneBoard(board) }
      },
    ),
  }
}

function createHeader(
  boardCommands: Pick<CanvasBoardCommands, 'renameOptimistic' | 'rollbackOptimisticRename'>,
) {
  const message = makeMessage()
  const header = useFlowBoardHeaderActions({
    currentBoardId: ref('board-a'),
    message,
    router: makeRouter(),
    route: { query: {} },
    flushAutosave: vi.fn(async () => undefined),
    boardCommands,
  })
  return { header, message }
}

describe('useFlowBoardHeaderActions', () => {
  beforeEach(() => {
    localStorage.clear()
    clearStoredAuth()
    setAuthScope()
    seedBoards()
    vi.clearAllMocks()
  })

  it('updates the board name locally before remote rename confirmation resolves', async () => {
    const remote = createDeferred<CanvasBoardRenameRemoteResult>()
    const boardCommands = makeOptimisticBoardCommands(() => remote.promise)
    const { header, message } = createHeader(boardCommands)

    header.boardNameDraft.value = '  New   Board  '
    const confirmTask = header.onConfirmBoardRename()

    expect(boardCommands.renameOptimistic).toHaveBeenCalledWith({
      boardId: 'board-a',
      nextName: 'New Board',
    })
    expect(header.currentBoardName.value).toBe('New Board')
    expect(header.boardRenameRecovery.value).toMatchObject({
      status: 'pending',
      previousName: 'Old Board',
      nextName: 'New Board',
    })
    expect(message.success).not.toHaveBeenCalled()

    remote.resolve({ ok: true, board: getCanvasBoardById('board-a')! })
    await confirmTask

    expect(header.boardRenameRecovery.value).toEqual({ status: 'idle' })
    expect(header.currentBoardName.value).toBe('New Board')
    expect(message.success).toHaveBeenCalledWith('已重命名画布')
    expect(message.warning).not.toHaveBeenCalled()
    expect(message.error).not.toHaveBeenCalled()
  })

  it('keeps failed optimistic rename visible and retries the same remote confirmation', async () => {
    const error = new Error('network failed')
    const confirm = vi
      .fn<() => Promise<CanvasBoardRenameRemoteResult>>()
      .mockResolvedValueOnce({
        ok: false,
        reason: 'sync-failed',
        boardId: 'board-a',
        board: { id: 'board-a', name: 'New Board', createdAt: 100, updatedAt: 201 },
        previousBoard: { id: 'board-a', name: 'Old Board', createdAt: 100, updatedAt: 200 },
        error,
      })
      .mockResolvedValueOnce({
        ok: true,
        board: { id: 'board-a', name: 'New Board', createdAt: 100, updatedAt: 201 },
      })
    const consoleWarn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const { header, message } = createHeader(makeOptimisticBoardCommands(confirm))

    header.boardNameDraft.value = 'New Board'
    await header.onConfirmBoardRename()

    expect(header.currentBoardName.value).toBe('New Board')
    expect(header.boardRenameRecovery.value).toMatchObject({ status: 'failed' })
    expect(message.error).toHaveBeenCalledWith('重命名未同步，可重试或撤回')

    await header.retryBoardRename()

    expect(confirm).toHaveBeenCalledTimes(2)
    expect(header.boardRenameRecovery.value).toEqual({ status: 'idle' })
    expect(message.success).toHaveBeenCalledWith('已重命名画布')
    expect(consoleWarn).toHaveBeenCalledWith('[canvas] failed to sync renamed board', error)
    consoleWarn.mockRestore()
  })

  it('rolls back a failed optimistic rename to the previous local name', async () => {
    const confirm = vi.fn<() => Promise<CanvasBoardRenameRemoteResult>>().mockResolvedValue({
      ok: false,
      reason: 'sync-failed',
      boardId: 'board-a',
      board: { id: 'board-a', name: 'New Board', createdAt: 100, updatedAt: 201 },
      previousBoard: { id: 'board-a', name: 'Old Board', createdAt: 100, updatedAt: 200 },
      error: new Error('network failed'),
    })
    const consoleWarn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const boardCommands = makeOptimisticBoardCommands(confirm)
    const { header, message } = createHeader(boardCommands)

    header.boardNameDraft.value = 'New Board'
    await header.onConfirmBoardRename()
    header.rollbackBoardRename()

    expect(boardCommands.rollbackOptimisticRename).toHaveBeenCalledWith({
      boardId: 'board-a',
      previousName: 'Old Board',
    })
    expect(header.currentBoardName.value).toBe('Old Board')
    expect(header.boardRenameRecovery.value).toEqual({ status: 'idle' })
    expect(message.success).toHaveBeenCalledWith('已撤回重命名')
    consoleWarn.mockRestore()
  })

  it('keeps conflict recovery explicit without silently overwriting the optimistic name', async () => {
    const error = { status: 409, msg: '画布名称已存在' }
    const confirm = vi.fn<() => Promise<CanvasBoardRenameRemoteResult>>().mockResolvedValue({
      ok: false,
      reason: 'remote-name-conflict',
      boardId: 'board-a',
      board: { id: 'board-a', name: 'Remote Taken', createdAt: 100, updatedAt: 201 },
      previousBoard: { id: 'board-a', name: 'Old Board', createdAt: 100, updatedAt: 200 },
      error,
    })
    const consoleWarn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const { header, message } = createHeader(makeOptimisticBoardCommands(confirm))

    header.boardNameDraft.value = 'Remote Taken'
    await header.onConfirmBoardRename()

    expect(header.currentBoardName.value).toBe('Remote Taken')
    expect(header.boardRenameRecovery.value).toMatchObject({
      status: 'conflict',
      previousName: 'Old Board',
      nextName: 'Remote Taken',
    })
    expect(message.error).toHaveBeenCalledWith('名称冲突，请更换名称或撤回')

    header.startBoardRename()

    expect(header.isRenamingBoard.value).toBe(true)
    expect(header.boardNameDraft.value).toBe('Remote Taken')
    expect(consoleWarn).toHaveBeenCalledWith('[canvas] failed to sync renamed board', error)
    consoleWarn.mockRestore()
  })

  it('reports local name conflicts before any optimistic mutation', async () => {
    const confirm = vi.fn<() => Promise<CanvasBoardRenameRemoteResult>>()
    const boardCommands = makeOptimisticBoardCommands(confirm)
    const { header, message } = createHeader(boardCommands)

    header.boardNameDraft.value = 'Taken'
    await header.onConfirmBoardRename()

    expect(boardCommands.renameOptimistic).toHaveBeenCalledWith({
      boardId: 'board-a',
      nextName: 'Taken',
    })
    expect(confirm).not.toHaveBeenCalled()
    expect(header.currentBoardName.value).toBe('Old Board')
    expect(message.warning).toHaveBeenCalledWith('名称已存在')
    expect(message.success).not.toHaveBeenCalled()
  })
})
