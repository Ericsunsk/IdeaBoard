import { computed, nextTick, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import type { CanvasBoardCatalog } from '../../canvas-board/catalog/catalog'
import type { CanvasBoardMeta } from '../../canvas-board/catalog/boards'
import type { BoardPreviewRefreshQueue } from '../../canvas-board/preview'
import type { BoardCatalogNavigation } from '../contract'
import type { BoardFeedback } from '../feedback/useFeedback'
import { createController } from './controller'

function makeBoard(id: string, name = id, updatedAt = 1): CanvasBoardMeta {
  return { id, name, createdAt: 1, updatedAt }
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

function createHarness(initialBoards: CanvasBoardMeta[] = [makeBoard('board-1')]) {
  const boards = ref<readonly CanvasBoardMeta[]>(initialBoards)
  const scope = ref<string | null>('user-1')
  const catalog: CanvasBoardCatalog = {
    viewModel: {
      scope,
      boards,
      byId: computed(() => new Map(boards.value.map((board) => [board.id, board]))),
    },
    commands: {
      hydrate: vi.fn(async () => ({ ok: true as const })),
      create: vi.fn(async () => ({ ok: true as const, board: makeBoard('created-board') })),
      rename: vi.fn(async ({ boardId, nextName }) => ({
        ok: true as const,
        board: makeBoard(boardId, nextName),
      })),
      duplicate: vi.fn(async () => ({ ok: true as const, board: makeBoard('copied-board') })),
      delete: vi.fn(async () => ({ ok: true as const, remoteWasMissing: false })),
    },
    dispose: vi.fn(),
  }
  const preview: BoardPreviewRefreshQueue = {
    previewMap: ref({}),
    reloadFromLocal: vi.fn(),
    schedule: vi.fn(),
    invalidate: vi.fn(),
    dispose: vi.fn(),
  }
  const navigation: BoardCatalogNavigation = {
    openBoard: vi.fn(),
  }
  const feedback: BoardFeedback = {
    createFailed: vi.fn(),
    renameValidationFailed: vi.fn(),
    renameSyncFailed: vi.fn(),
    renamed: vi.fn(),
    copyFailed: vi.fn(),
    copied: vi.fn(),
    deleteFailed: vi.fn(),
    deleted: vi.fn(),
    catalogSyncFailed: vi.fn(),
    catalogReloaded: vi.fn(),
  }
  const warn = vi.fn()
  const controller = createController({ catalog, preview, navigation, feedback, warn })

  return { boards, scope, catalog, preview, navigation, feedback, warn, controller }
}

describe('createController', () => {
  it('closes the active menu before creating and navigates only after the catalog command succeeds', async () => {
    const harness = createHarness()
    harness.controller.commands.toggleBoardMenu('board-1')
    vi.mocked(harness.catalog.commands.create).mockImplementation(async () => {
      expect(harness.controller.viewModel.activeMenuBoardId.value).toBeNull()
      expect(harness.navigation.openBoard).not.toHaveBeenCalled()
      return { ok: true, board: makeBoard('created-board') }
    })

    await harness.controller.commands.createAndOpenBoard()

    expect(harness.catalog.commands.create).toHaveBeenCalledOnce()
    expect(harness.navigation.openBoard).toHaveBeenCalledWith('created-board')
  })

  it('closes the menu before rename and suppresses exactly the click caused by committing the input', async () => {
    const harness = createHarness()
    const board = makeBoard('board-1', 'Original')
    harness.controller.commands.toggleBoardMenu(board.id)

    harness.controller.commands.startRename(board)

    expect(harness.controller.viewModel.activeMenuBoardId.value).toBeNull()
    expect(harness.controller.viewModel.renamingBoardId.value).toBe(board.id)
    expect(harness.controller.viewModel.renameDraft.value).toBe('Original')

    harness.controller.commands.updateRenameDraft('Renamed')
    await harness.controller.commands.commitRename()
    expect(harness.catalog.commands.rename).toHaveBeenCalledWith({
      boardId: board.id,
      nextName: 'Renamed',
    })
    expect(harness.feedback.renamed).toHaveBeenCalledOnce()

    harness.controller.commands.openBoardFromCard(board.id)
    expect(harness.navigation.openBoard).not.toHaveBeenCalled()

    harness.controller.commands.openBoardFromCard(board.id)
    expect(harness.navigation.openBoard).toHaveBeenCalledWith(board.id)
  })

  it('exposes catalog recovery only when rename recovery failed and clears it after a successful reload', async () => {
    const harness = createHarness()
    const board = makeBoard('board-1', 'Original')
    const syncError = new Error('conflict')
    vi.mocked(harness.catalog.commands.rename).mockResolvedValueOnce({
      ok: false,
      reason: 'remote-name-conflict',
      boardId: board.id,
      previousBoard: board,
      error: syncError,
      rollback: { ok: true, board },
      hydrate: { ok: false, error: new Error('hydrate failed') },
    })
    harness.controller.commands.startRename(board)
    harness.controller.commands.updateRenameDraft('Conflicting')

    await harness.controller.commands.commitRename()

    expect(harness.feedback.renameSyncFailed).toHaveBeenCalledOnce()
    expect(harness.controller.viewModel.renamingBoardId.value).toBeNull()
    expect(harness.controller.viewModel.catalogRecoveryBoardId.value).toBe(board.id)
    expect(harness.warn).toHaveBeenCalledWith('[boards] failed to sync renamed board', syncError)

    vi.mocked(harness.catalog.commands.hydrate)
      .mockResolvedValueOnce({ ok: false, error: new Error('still unavailable') })
      .mockResolvedValueOnce({ ok: true })

    await harness.controller.commands.reloadCatalog()
    expect(harness.feedback.catalogSyncFailed).toHaveBeenCalledOnce()
    expect(harness.controller.viewModel.catalogRecoveryBoardId.value).toBe(board.id)
    expect(harness.controller.viewModel.isCatalogReloadPending.value).toBe(false)

    await harness.controller.commands.reloadCatalog()
    expect(harness.catalog.commands.hydrate).toHaveBeenLastCalledWith({
      preserveLocalNewer: false,
      rollbackBoardIds: [board.id],
    })
    expect(harness.controller.viewModel.catalogRecoveryBoardId.value).toBeNull()
    expect(harness.feedback.catalogReloaded).toHaveBeenCalledOnce()
  })

  it('keeps the delete target recoverable after failure and always releases the pending guard', async () => {
    const harness = createHarness()
    const board = makeBoard('board-1')
    const firstDelete = deferred<
      { ok: true; remoteWasMissing: boolean } | { ok: false; reason: 'sync-failed'; error: unknown }
    >()
    vi.mocked(harness.catalog.commands.delete).mockReturnValueOnce(firstDelete.promise)
    harness.controller.commands.toggleBoardMenu(board.id)
    harness.controller.commands.requestDelete(board)

    expect(harness.controller.viewModel.activeMenuBoardId.value).toBeNull()
    expect(harness.controller.viewModel.deleteDialogVisible.value).toBe(true)
    expect(harness.controller.viewModel.pendingDeleteBoard.value).toEqual(board)

    const pendingConfirmation = harness.controller.commands.confirmDelete()
    expect(harness.controller.viewModel.isDeletePending.value).toBe(true)

    const error = new Error('delete failed')
    firstDelete.resolve({ ok: false, reason: 'sync-failed', error })
    await pendingConfirmation

    expect(harness.controller.viewModel.isDeletePending.value).toBe(false)
    expect(harness.controller.viewModel.deleteDialogVisible.value).toBe(true)
    expect(harness.controller.viewModel.pendingDeleteBoard.value).toEqual(board)
    expect(harness.feedback.deleteFailed).toHaveBeenCalledOnce()
    expect(harness.warn).toHaveBeenCalledWith('[boards] failed to sync deleted board', error)

    vi.mocked(harness.catalog.commands.delete).mockResolvedValueOnce({
      ok: true,
      remoteWasMissing: false,
    })
    await harness.controller.commands.confirmDelete()

    expect(harness.controller.viewModel.isDeletePending.value).toBe(false)
    expect(harness.controller.viewModel.deleteDialogVisible.value).toBe(false)
    expect(harness.controller.viewModel.pendingDeleteBoard.value).toBeNull()
    expect(harness.feedback.deleted).toHaveBeenCalledOnce()
  })

  it('owns loading, filtering, sorting, and the preview queue lifecycle', async () => {
    const harness = createHarness([
      makeBoard('older', 'Alpha', 10),
      makeBoard('newer', 'Beta Canvas', 20),
    ])

    await nextTick()
    expect(harness.preview.reloadFromLocal).toHaveBeenLastCalledWith(harness.boards.value)
    expect(harness.preview.schedule).toHaveBeenLastCalledWith(harness.boards.value)
    expect(harness.controller.viewModel.visibleBoards.value.map((board) => board.id)).toEqual([
      'newer',
      'older',
    ])

    harness.controller.commands.setSearchKeyword('  beta  ')
    expect(harness.controller.viewModel.searchKeyword.value).toBe('beta')
    expect(harness.controller.viewModel.visibleBoards.value.map((board) => board.id)).toEqual([
      'newer',
    ])

    harness.controller.commands.setSearchKeyword('')
    harness.controller.commands.setSortMode('name')
    expect(harness.controller.viewModel.visibleBoards.value.map((board) => board.id)).toEqual([
      'older',
      'newer',
    ])

    const hydrateError = new Error('hydrate failed')
    vi.mocked(harness.catalog.commands.hydrate).mockResolvedValueOnce({
      ok: false,
      error: hydrateError,
    })
    await harness.controller.commands.initialize()
    expect(harness.controller.viewModel.isBoardsLoading.value).toBe(false)
    expect(harness.feedback.catalogSyncFailed).toHaveBeenCalledOnce()

    harness.controller.dispose()
    expect(harness.preview.dispose).toHaveBeenCalledOnce()
    vi.mocked(harness.preview.schedule).mockClear()
    harness.boards.value = [makeBoard('after-dispose')]
    await nextTick()
    expect(harness.preview.schedule).not.toHaveBeenCalled()
  })

  it('keeps create and duplicate failure feedback behind catalog command results', async () => {
    const harness = createHarness()
    const createError = new Error('create sync failed')
    vi.mocked(harness.catalog.commands.create).mockResolvedValueOnce({
      ok: false,
      reason: 'sync-failed',
      board: makeBoard('failed-create'),
      error: createError,
    })

    await harness.controller.commands.createAndOpenBoard()

    expect(harness.feedback.createFailed).toHaveBeenCalledOnce()
    expect(harness.warn).toHaveBeenCalledWith('[boards] failed to sync new board', createError)
    expect(harness.navigation.openBoard).not.toHaveBeenCalled()

    const source = makeBoard('board-1')
    harness.controller.commands.toggleBoardMenu(source.id)
    vi.mocked(harness.catalog.commands.duplicate).mockImplementationOnce(async () => {
      expect(harness.controller.viewModel.activeMenuBoardId.value).toBeNull()
      return { ok: true, board: makeBoard('copy') }
    })

    await harness.controller.commands.duplicateBoard(source.id)

    expect(harness.catalog.commands.duplicate).toHaveBeenCalledWith({
      sourceBoardId: source.id,
    })
    expect(harness.feedback.copied).toHaveBeenCalledOnce()
  })

  it('clears account-scoped transient UI when the catalog scope changes', async () => {
    const harness = createHarness()
    const board = makeBoard('board-1')
    vi.mocked(harness.catalog.commands.rename).mockResolvedValueOnce({
      ok: false,
      reason: 'sync-failed',
      boardId: board.id,
      previousBoard: board,
      error: new Error('failed'),
      rollback: { ok: false, reason: 'invalid-name' },
    })
    harness.controller.commands.startRename(board)
    await harness.controller.commands.commitRename()
    harness.controller.commands.toggleBoardMenu(board.id)
    harness.controller.commands.requestDelete(board)

    harness.scope.value = 'user-2'
    await nextTick()

    expect(harness.controller.viewModel.activeMenuBoardId.value).toBeNull()
    expect(harness.controller.viewModel.renamingBoardId.value).toBeNull()
    expect(harness.controller.viewModel.renameDraft.value).toBe('')
    expect(harness.controller.viewModel.catalogRecoveryBoardId.value).toBeNull()
    expect(harness.controller.viewModel.deleteDialogVisible.value).toBe(false)
    expect(harness.controller.viewModel.pendingDeleteBoard.value).toBeNull()
    expect(harness.preview.invalidate).toHaveBeenCalledOnce()
  })
})
