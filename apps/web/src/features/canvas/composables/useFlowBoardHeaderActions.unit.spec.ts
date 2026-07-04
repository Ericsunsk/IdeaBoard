import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import type { Router } from 'vue-router'
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

describe('useFlowBoardHeaderActions', () => {
  it('renames through the canvas-board command seam with a normalized name', async () => {
    const rename = vi.fn(async () => ({
      ok: true as const,
      board: { id: 'board-a', name: 'New Board', createdAt: 100, updatedAt: 200 },
    }))
    const message = makeMessage()
    const header = useFlowBoardHeaderActions({
      currentBoardId: ref('board-a'),
      message,
      router: makeRouter(),
      route: { query: {} },
      flushAutosave: vi.fn(async () => undefined),
      boardCommands: { rename },
    })

    header.boardNameDraft.value = '  New   Board  '
    await header.onConfirmBoardRename()

    expect(rename).toHaveBeenCalledWith({ boardId: 'board-a', nextName: 'New Board' })
    expect(message.success).toHaveBeenCalledWith('已重命名画布')
    expect(message.warning).not.toHaveBeenCalled()
    expect(message.error).not.toHaveBeenCalled()
  })

  it('reports local name conflicts from the shared rename command', async () => {
    const rename = vi.fn(async () => ({
      ok: false as const,
      reason: 'local-name-conflict' as const,
    }))
    const message = makeMessage()
    const header = useFlowBoardHeaderActions({
      currentBoardId: ref('board-a'),
      message,
      router: makeRouter(),
      route: { query: {} },
      flushAutosave: vi.fn(async () => undefined),
      boardCommands: { rename },
    })

    header.boardNameDraft.value = 'Taken'
    await header.onConfirmBoardRename()

    expect(rename).toHaveBeenCalledWith({ boardId: 'board-a', nextName: 'Taken' })
    expect(message.warning).toHaveBeenCalledWith('名称已存在')
    expect(message.success).not.toHaveBeenCalled()
  })

  it('reports remote sync conflicts after the command hydrates rollback state', async () => {
    const error = { status: 409, msg: '画布名称已存在' }
    const rename = vi.fn(async () => ({
      ok: false as const,
      reason: 'remote-name-conflict' as const,
      boardId: 'board-a',
      error,
    }))
    const consoleWarn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const message = makeMessage()
    const header = useFlowBoardHeaderActions({
      currentBoardId: ref('board-a'),
      message,
      router: makeRouter(),
      route: { query: {} },
      flushAutosave: vi.fn(async () => undefined),
      boardCommands: { rename },
    })

    header.boardNameDraft.value = 'Taken Remote'
    await header.onConfirmBoardRename()

    expect(message.error).toHaveBeenCalledWith('名称已存在，请更换后再试')
    expect(consoleWarn).toHaveBeenCalledWith('[canvas] failed to sync renamed board', error)
    expect(message.success).not.toHaveBeenCalled()
    consoleWarn.mockRestore()
  })
})
