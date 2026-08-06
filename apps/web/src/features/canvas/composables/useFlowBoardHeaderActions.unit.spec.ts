import { computed, ref } from 'vue'
import type { Router } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  createCanvasBoardCatalog,
  type CanvasBoardCatalog,
} from '../../canvas-board/catalog/canvasBoardCatalog'
import { replaceCanvasBoards } from '../../canvas-board/catalog/canvasBoards'
import { clearStoredAuth, setStoredAuth } from '../../../shared/utils/auth'
import { useFlowBoardHeaderActions } from './useFlowBoardHeaderActions'

function createCatalog(
  rename: CanvasBoardCatalog['commands']['rename'] = vi.fn(async () => ({
    ok: true as const,
    board: board('board-a', 'New Board'),
  })),
  hydrate: CanvasBoardCatalog['commands']['hydrate'] = vi.fn(async () => ({ ok: true as const })),
) {
  const boards = ref([board('board-a', 'Old Board'), board('board-b', 'Taken')])
  return {
    viewModel: {
      boards,
      byId: computed(() => new Map(boards.value.map((item) => [item.id, item] as const))),
    },
    commands: {
      rename,
      hydrate,
      create: vi.fn(),
      duplicate: vi.fn(),
      delete: vi.fn(),
    },
  } as unknown as Pick<CanvasBoardCatalog, 'viewModel' | 'commands'>
}

function board(id: string, name: string) {
  return { id, name, createdAt: 100, updatedAt: 200 }
}

function createHeader(catalog: Pick<CanvasBoardCatalog, 'viewModel' | 'commands'>) {
  const message = { warning: vi.fn(), success: vi.fn(), error: vi.fn() }
  const router = { push: vi.fn(async () => undefined) } as unknown as Router
  const header = useFlowBoardHeaderActions({
    currentBoardId: ref('board-a'),
    message,
    router,
    route: { query: {} },
    flushAutosave: vi.fn(async () => undefined),
    catalog,
  })
  return { header, message, router }
}

describe('useFlowBoardHeaderActions', () => {
  beforeEach(() => {
    localStorage.clear()
    clearStoredAuth()
  })

  it('reads the current board and menu options from the shared catalog view model', () => {
    const { header } = createHeader(createCatalog())

    expect(header.currentBoardName.value).toBe('Old Board')
    expect(header.boardOptions.value).toEqual([
      { id: 'board-a', name: 'Old Board' },
      { id: 'board-b', name: 'Taken' },
    ])
  })

  it('delegates rename to the shared catalog and reports its typed failure after rollback', async () => {
    const error = new Error('offline')
    const rename = vi.fn(async () => ({
      ok: false as const,
      reason: 'sync-failed' as const,
      boardId: 'board-a',
      previousBoard: board('board-a', 'Old Board'),
      rolledBackBoard: board('board-a', 'Old Board'),
      rollback: { ok: true as const, board: board('board-a', 'Old Board') },
      error,
    }))
    const { header, message } = createHeader(createCatalog(rename))

    header.boardNameDraft.value = '  New   Board '
    await header.onConfirmBoardRename()

    expect(rename).toHaveBeenCalledWith({ boardId: 'board-a', nextName: 'New Board' })
    expect(message.error).toHaveBeenCalledWith('重命名同步失败，已恢复原名称')
  })

  it('offers reload recovery when rename rollback fails', async () => {
    const rollback = { ok: false as const, reason: 'invalid-name' as const }
    const rename = vi.fn(async () => ({
      ok: false as const,
      reason: 'sync-failed' as const,
      boardId: 'board-a',
      previousBoard: board('board-a', 'Old Board'),
      rollback,
      error: new Error('offline'),
    }))
    const hydrate = vi.fn(async () => ({ ok: true as const }))
    const { header, message } = createHeader(createCatalog(rename, hydrate))

    header.boardNameDraft.value = 'New Board'
    await header.onConfirmBoardRename()

    expect(message.error).toHaveBeenCalledWith('重命名同步失败，无法恢复原名称，请重新加载画布列表')
    expect(header.catalogRecovery.value).toEqual({
      status: 'reload-required',
      boardId: 'board-a',
    })

    await header.reloadCatalog()

    expect(hydrate).toHaveBeenCalledWith({
      preserveLocalNewer: false,
      rollbackBoardIds: ['board-a'],
    })
    expect(header.catalogRecovery.value).toEqual({ status: 'idle' })
    expect(message.success).toHaveBeenCalledWith('画布列表已重新加载')
  })

  it('reports remote conflict hydrate success without claiming the old name was restored', async () => {
    const rename = vi.fn(async () => ({
      ok: false as const,
      reason: 'remote-name-conflict' as const,
      boardId: 'board-a',
      previousBoard: board('board-a', 'Old Board'),
      rollback: { ok: true as const, board: board('board-a', 'Old Board') },
      hydrate: { ok: true as const },
      error: { status: 409 },
    }))
    const { header, message } = createHeader(createCatalog(rename))

    header.boardNameDraft.value = 'Taken'
    await header.onConfirmBoardRename()

    expect(message.error).toHaveBeenCalledWith('名称冲突，已重新加载画布列表')
    expect(header.catalogRecovery.value).toEqual({ status: 'idle' })
  })

  it('keeps rename visibly pending and blocks another rename until sync completes', async () => {
    let resolveRename!: (
      value: Awaited<ReturnType<CanvasBoardCatalog['commands']['rename']>>,
    ) => void
    const rename = vi.fn(
      () =>
        new Promise<Awaited<ReturnType<CanvasBoardCatalog['commands']['rename']>>>((resolve) => {
          resolveRename = resolve
        }),
    )
    const { header, message } = createHeader(createCatalog(rename))

    header.boardNameDraft.value = 'New Board'
    const task = header.onConfirmBoardRename()
    await vi.waitFor(() => expect(header.isRenamePending.value).toBe(true))
    header.startBoardRename()

    expect(message.warning).toHaveBeenCalledWith('画布名称正在同步，请稍后再改名')
    resolveRename({ ok: true, board: board('board-a', 'New Board') })
    await task
    expect(header.isRenamePending.value).toBe(false)
  })

  it('keeps reload recovery available when remote conflict hydration fails', async () => {
    const rename = vi.fn(async () => ({
      ok: false as const,
      reason: 'remote-name-conflict' as const,
      boardId: 'board-a',
      previousBoard: board('board-a', 'Old Board'),
      rollback: { ok: true as const, board: board('board-a', 'Old Board') },
      hydrate: { ok: false as const, error: new Error('hydrate failed') },
      error: { status: 409 },
    }))
    const { header, message } = createHeader(createCatalog(rename))

    header.boardNameDraft.value = 'Taken'
    await header.onConfirmBoardRename()

    expect(message.error).toHaveBeenCalledWith(
      '名称冲突，已恢复原名称，但远端列表刷新失败，请重新加载',
    )
    expect(header.catalogRecovery.value).toEqual({
      status: 'reload-required',
      boardId: 'board-a',
    })
  })

  it('navigates after flushing the current board session', async () => {
    const catalog = createCatalog()
    const { header, router } = createHeader(catalog)

    header.onSelectBoard('board-b')
    await vi.waitFor(() => expect(router.push).toHaveBeenCalled())

    expect(router.push).toHaveBeenCalledWith({
      path: '/user/canvas',
      query: { boardId: 'board-b' },
    })
  })

  it('resets the shared catalog across A logout B before Canvas Header is opened directly', async () => {
    setStoredAuth(authUser(1, 'USERA1'))
    replaceCanvasBoards([board('board-a', 'User A Board')])
    const catalog = createCanvasBoardCatalog()
    const currentBoardId = ref<string | null>('board-a')
    const message = { warning: vi.fn(), success: vi.fn(), error: vi.fn() }
    const header = useFlowBoardHeaderActions({
      currentBoardId,
      message,
      router: { push: vi.fn(async () => undefined) } as unknown as Router,
      route: { query: {} },
      flushAutosave: vi.fn(async () => undefined),
      catalog,
    })
    expect(header.boardOptions.value).toEqual([{ id: 'board-a', name: 'User A Board' }])

    clearStoredAuth()
    expect(header.boardOptions.value).toEqual([])

    setStoredAuth(authUser(2, 'USERB2'))
    replaceCanvasBoards([board('board-b', 'User B Board')])
    currentBoardId.value = 'board-b'

    expect(header.currentBoardName.value).toBe('User B Board')
    expect(header.boardOptions.value).toEqual([{ id: 'board-b', name: 'User B Board' }])
    catalog.dispose()
  })
})

function authUser(id: number, uid: string) {
  return {
    public_id: `00000000-0000-0000-0000-${String(id).padStart(12, '0')}`,
    display_name: `user-${id}`,
    storage_namespace: uid,
    avatar: '',
    role: 'user' as const,
    status: 1,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  }
}
