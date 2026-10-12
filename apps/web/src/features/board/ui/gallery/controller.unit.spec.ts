import { computed, nextTick, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createBoardRepository, type BoardRepository } from '../../session/catalog/repository'
import type { BoardMeta } from '../../session/catalog/boards'
import type { BoardPreviewRefreshQueue } from '../../session/preview/boardPreviewRefreshQueue'
import type { CatalogNavigation } from './contract'
import type { BoardFeedback } from './useBoardFeedback'
import { createGalleryController } from './controller'

const disposeControllers: (() => void)[] = []

afterEach(() => {
  disposeControllers.splice(0).forEach((dispose) => dispose())
  vi.restoreAllMocks()
})

function makeBoard(id: string, name = id, updatedAt = 1): BoardMeta {
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

function createHarness(
  initialBoards: BoardMeta[] = [makeBoard('board-1')],
  repository?: BoardRepository,
) {
  const boards = ref<readonly BoardMeta[]>(initialBoards)
  const scope = ref<string | null>('user-1')
  const catalog: BoardRepository = {
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
  const navigation: CatalogNavigation = {
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
  const controller = createGalleryController({
    catalog: repository ?? catalog,
    preview,
    navigation,
    feedback,
    warn,
  })
  disposeControllers.push(controller.dispose)

  return { boards, scope, catalog, preview, navigation, feedback, warn, controller }
}

describe('createGalleryController', () => {
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
    const syncError = { status: 409, code: 'board_revision_conflict' }
    vi.mocked(harness.catalog.commands.rename).mockResolvedValueOnce({
      ok: false,
      reason: 'revision-conflict',
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
      includeDocuments: false,
      force: true,
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
    expect(harness.controller.viewModel.catalogHydrationFailed.value).toBe(true)
    expect(harness.feedback.catalogSyncFailed).toHaveBeenCalledOnce()

    vi.mocked(harness.catalog.commands.hydrate).mockResolvedValueOnce({ ok: true })
    await harness.controller.commands.reloadCatalog()
    expect(harness.controller.viewModel.catalogHydrationFailed.value).toBe(false)
    expect(harness.feedback.catalogReloaded).toHaveBeenCalledOnce()

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

  it('notifies preview of theme changes when theme dependency is provided', async () => {
    const isDark = ref(false)
    const boards = ref([makeBoard('board-1')])
    const handleThemeChange = vi.fn()
    const preview: BoardPreviewRefreshQueue = {
      previewMap: ref({}),
      reloadFromLocal: vi.fn(),
      schedule: vi.fn(),
      handleThemeChange,
      invalidate: vi.fn(),
      dispose: vi.fn(),
    }
    const catalog: BoardRepository = {
      viewModel: {
        scope: ref('user-1'),
        boards,
        byId: computed(() => new Map()),
      },
      commands: {
        hydrate: vi.fn(async () => ({ ok: true as const })),
        create: vi.fn(async () => ({ ok: true as const, board: makeBoard('b') })),
        rename: vi.fn(async () => ({ ok: true as const, board: makeBoard('b') })),
        duplicate: vi.fn(async () => ({ ok: true as const, board: makeBoard('b') })),
        delete: vi.fn(async () => ({ ok: true as const, remoteWasMissing: false })),
      },
      dispose: vi.fn(),
    }

    const controller = createGalleryController({
      catalog,
      preview,
      navigation: { openBoard: vi.fn() },
      feedback: {
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
      },
      theme: { isDark },
      warn: vi.fn(),
    })

    expect(handleThemeChange).not.toHaveBeenCalled()
    isDark.value = true
    await nextTick()

    expect(handleThemeChange).toHaveBeenCalledWith(true, [
      expect.objectContaining({ id: 'board-1' }),
    ])

    controller.dispose()
  })

  it('refreshes on window focus only when scope is stable, throttled, and reuses initialize', async () => {
    const harness = createHarness()
    await harness.controller.commands.initialize()
    expect(harness.catalog.commands.hydrate).toHaveBeenCalledTimes(1)

    window.dispatchEvent(new Event('focus'))
    await nextTick()
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(harness.catalog.commands.hydrate).toHaveBeenCalledTimes(2)

    // Immediate second focus is throttled (no new trigger, no loop).
    window.dispatchEvent(new Event('focus'))
    await nextTick()
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(harness.catalog.commands.hydrate).toHaveBeenCalledTimes(2)

    // Unstable scope never triggers a reload.
    harness.scope.value = null
    await nextTick()
    window.dispatchEvent(new Event('focus'))
    await nextTick()
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(harness.catalog.commands.hydrate).toHaveBeenCalledTimes(2)

    harness.controller.dispose()
    harness.scope.value = 'user-1'
    await nextTick()
    window.dispatchEvent(new Event('focus'))
    await nextTick()
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(harness.catalog.commands.hydrate).toHaveBeenCalledTimes(2)
  })

  it('skips visible reloads while the tab stays hidden', async () => {
    const harness = createHarness()
    await harness.controller.commands.initialize()
    expect(harness.catalog.commands.hydrate).toHaveBeenCalledTimes(1)

    const original = Object.getOwnPropertyDescriptor(document, 'visibilityState')
    Object.defineProperty(document, 'visibilityState', { value: 'hidden', configurable: true })
    try {
      window.dispatchEvent(new Event('focus'))
      await nextTick()
      await new Promise((resolve) => setTimeout(resolve, 0))
      expect(harness.catalog.commands.hydrate).toHaveBeenCalledTimes(1)
    } finally {
      if (original) Object.defineProperty(document, 'visibilityState', original)
      else
        Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true })
    }
    harness.controller.dispose()
  })

  it('auto-hydrates once when the scope becomes ready after a failure', async () => {
    const harness = createHarness()
    vi.mocked(harness.catalog.commands.hydrate).mockResolvedValueOnce({
      ok: false,
      error: new Error('no workspace'),
    })
    await harness.controller.commands.initialize()
    expect(harness.controller.viewModel.catalogHydrationFailed.value).toBe(true)
    expect(harness.feedback.catalogSyncFailed).toHaveBeenCalledTimes(1)
    expect(harness.catalog.commands.hydrate).toHaveBeenCalledTimes(1)

    harness.scope.value = null
    await nextTick()
    expect(harness.catalog.commands.hydrate).toHaveBeenCalledTimes(1)

    vi.mocked(harness.catalog.commands.hydrate).mockResolvedValueOnce({ ok: true })
    harness.scope.value = 'user-2'
    await nextTick()
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(harness.catalog.commands.hydrate).toHaveBeenCalledTimes(2)
    expect(harness.controller.viewModel.catalogHydrationFailed.value).toBe(false)

    harness.controller.dispose()
  })

  it('auto-hydrates on workspace switches without manual retry', async () => {
    const harness = createHarness()
    await harness.controller.commands.initialize()
    expect(harness.catalog.commands.hydrate).toHaveBeenCalledTimes(1)

    harness.scope.value = 'user-2'
    await nextTick()
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(harness.catalog.commands.hydrate).toHaveBeenCalledTimes(2)

    harness.controller.dispose()
  })

  it.each(['failure', 'rejection'] as const)(
    'ignores old workspace %s after focus refresh while the repository queues the new workspace hydrate',
    async (completion) => {
      const oldRefresh = deferred<Awaited<ReturnType<BoardRepository['commands']['hydrate']>>>()
      const newRefresh = deferred<Awaited<ReturnType<BoardRepository['commands']['hydrate']>>>()
      let currentScope = 'workspace-a'
      let changeScope!: (scope: string) => void
      const hydrate = vi
        .fn<BoardRepository['commands']['hydrate']>()
        .mockResolvedValueOnce({ ok: true })
        .mockReturnValueOnce(oldRefresh.promise)
        .mockReturnValueOnce(newRefresh.promise)
      const repository = createBoardRepository({
        getScope: () => currentScope,
        listBoards: () => [makeBoard(currentScope)],
        subscribe: () => () => {},
        subscribeScope: (listener) => {
          changeScope = (scope) => {
            currentScope = scope
            listener(scope)
          }
          return () => {}
        },
        hydrate,
      })
      disposeControllers.push(repository.dispose)
      const harness = createHarness(undefined, repository)
      await harness.controller.commands.initialize()

      window.dispatchEvent(new Event('focus'))
      await vi.waitFor(() => expect(hydrate).toHaveBeenCalledTimes(2))
      changeScope('workspace-b')
      await nextTick()
      expect(hydrate).toHaveBeenCalledTimes(2)

      const scopeChanged = new Error('BOARD_HYDRATE_SCOPE_CHANGED')
      if (completion === 'failure') oldRefresh.resolve({ ok: false, error: scopeChanged })
      else oldRefresh.reject(scopeChanged)
      await vi.waitFor(() => expect(hydrate).toHaveBeenCalledTimes(3))

      expect(harness.feedback.catalogSyncFailed).not.toHaveBeenCalled()
      expect(harness.warn).not.toHaveBeenCalled()
      expect(harness.controller.viewModel.catalogHydrationFailed.value).toBe(false)
      expect(harness.controller.viewModel.visibleBoards.value[0]?.id).toBe('workspace-b')

      newRefresh.resolve({ ok: true })
      await harness.controller.commands.initialize()
      expect(harness.controller.viewModel.catalogHydrationFailed.value).toBe(false)
      expect(harness.feedback.catalogSyncFailed).not.toHaveBeenCalled()
    },
  )

  it('keeps the initial loading state until the current workspace hydrate completes', async () => {
    const harness = createHarness()
    const oldRefresh = deferred<{ ok: true }>()
    const newRefresh = deferred<{ ok: true }>()
    vi.mocked(harness.catalog.commands.hydrate)
      .mockReturnValueOnce(oldRefresh.promise)
      .mockReturnValueOnce(newRefresh.promise)
    const initialLoad = harness.controller.commands.initialize()

    harness.scope.value = 'workspace-b'
    await nextTick()
    oldRefresh.resolve({ ok: true })
    await initialLoad

    expect(harness.controller.viewModel.isBoardsLoading.value).toBe(true)
    newRefresh.resolve({ ok: true })
    await harness.controller.commands.initialize()
    expect(harness.controller.viewModel.isBoardsLoading.value).toBe(false)
  })

  it('hydrates a workspace that becomes ready while an obsolete initial load is pending', async () => {
    const harness = createHarness()
    const oldRefresh = deferred<{ ok: true }>()
    const newRefresh = deferred<{ ok: true }>()
    vi.mocked(harness.catalog.commands.hydrate)
      .mockReturnValueOnce(oldRefresh.promise)
      .mockReturnValueOnce(newRefresh.promise)
    const initialLoad = harness.controller.commands.initialize()
    harness.scope.value = null
    await nextTick()
    harness.scope.value = 'workspace-b'
    await nextTick()

    expect(harness.catalog.commands.hydrate).toHaveBeenCalledTimes(2)
    oldRefresh.resolve({ ok: true })
    await initialLoad
    expect(harness.controller.viewModel.isBoardsLoading.value).toBe(true)
    newRefresh.resolve({ ok: true })
    await harness.controller.commands.initialize()
    expect(harness.controller.viewModel.isBoardsLoading.value).toBe(false)
  })

  it('preserves a current workspace failure and reports its exception after an old success', async () => {
    const harness = createHarness()
    const oldRefresh = deferred<{ ok: true }>()
    const newRefresh = deferred<{ ok: true }>()
    vi.mocked(harness.catalog.commands.hydrate)
      .mockReturnValueOnce(oldRefresh.promise)
      .mockReturnValueOnce(newRefresh.promise)
    const initialLoad = harness.controller.commands.initialize()
    harness.scope.value = 'workspace-b'
    await nextTick()
    const currentFailure = new Error('current workspace unavailable')
    newRefresh.reject(currentFailure)
    await harness.controller.commands.initialize()

    expect(harness.feedback.catalogSyncFailed).toHaveBeenCalledOnce()
    expect(harness.warn).toHaveBeenCalledWith('[boards] failed to hydrate catalog', currentFailure)
    oldRefresh.resolve({ ok: true })
    await initialLoad
    expect(harness.controller.viewModel.catalogHydrationFailed.value).toBe(true)
    expect(harness.feedback.catalogSyncFailed).toHaveBeenCalledOnce()
  })

  it('ignores a focus refresh superseded by another focus refresh in the same workspace', async () => {
    const harness = createHarness()
    await harness.controller.commands.initialize()
    const oldRefresh = deferred<{ ok: true }>()
    const latestRefresh = deferred<Awaited<ReturnType<BoardRepository['commands']['hydrate']>>>()
    vi.mocked(harness.catalog.commands.hydrate)
      .mockReturnValueOnce(oldRefresh.promise)
      .mockReturnValueOnce(latestRefresh.promise)
    const now = vi.spyOn(Date, 'now').mockReturnValue(10_000)
    window.dispatchEvent(new Event('focus'))
    await nextTick()
    const previous = harness.controller.commands.initialize()
    now.mockReturnValue(20_000)
    window.dispatchEvent(new Event('focus'))
    await nextTick()
    now.mockRestore()
    latestRefresh.resolve({ ok: false, error: new Error('latest failure') })
    await harness.controller.commands.initialize()
    oldRefresh.resolve({ ok: true })
    await previous

    expect(harness.controller.viewModel.catalogHydrationFailed.value).toBe(true)
    expect(harness.feedback.catalogSyncFailed).toHaveBeenCalledOnce()
  })

  it('does not release the current retry guard when an old workspace retry finishes', async () => {
    const harness = createHarness()
    vi.mocked(harness.catalog.commands.hydrate).mockResolvedValueOnce({
      ok: false,
      error: new Error('initial failure'),
    })
    await harness.controller.commands.initialize()
    const oldRetry = deferred<{ ok: true }>()
    vi.mocked(harness.catalog.commands.hydrate).mockReturnValueOnce(oldRetry.promise)
    const retry = harness.controller.commands.reloadCatalog()
    vi.mocked(harness.catalog.commands.hydrate).mockResolvedValueOnce({
      ok: false,
      error: new Error('new workspace failure'),
    })
    harness.scope.value = 'workspace-b'
    await nextTick()
    await harness.controller.commands.initialize()
    const newRetry = deferred<{ ok: true }>()
    vi.mocked(harness.catalog.commands.hydrate).mockReturnValueOnce(newRetry.promise)
    const currentRetry = harness.controller.commands.reloadCatalog()
    oldRetry.resolve({ ok: true })
    await retry

    expect(harness.controller.viewModel.isCatalogReloadPending.value).toBe(true)
    expect(harness.feedback.catalogReloaded).not.toHaveBeenCalled()
    newRetry.resolve({ ok: true })
    await currentRetry
    expect(harness.controller.viewModel.isCatalogReloadPending.value).toBe(false)
    expect(harness.feedback.catalogReloaded).toHaveBeenCalledOnce()
  })

  it('ignores hydration and retry completions after disposal', async () => {
    const harness = createHarness()
    vi.mocked(harness.catalog.commands.hydrate).mockResolvedValueOnce({
      ok: false,
      error: new Error('initial failure'),
    })
    await harness.controller.commands.initialize()
    const retryResult = deferred<{ ok: true }>()
    vi.mocked(harness.catalog.commands.hydrate).mockReturnValueOnce(retryResult.promise)
    const retry = harness.controller.commands.reloadCatalog()
    harness.controller.dispose()
    retryResult.resolve({ ok: true })
    await retry

    expect(harness.controller.viewModel.catalogHydrationFailed.value).toBe(true)
    expect(harness.controller.viewModel.isCatalogReloadPending.value).toBe(true)
    expect(harness.feedback.catalogReloaded).not.toHaveBeenCalled()
  })

  it.each(['success', 'failure', 'rejection'] as const)(
    'ignores an old workspace rename-recovery reload %s',
    async (completion) => {
      const harness = createHarness()
      const board = makeBoard('board-1')
      vi.mocked(harness.catalog.commands.rename).mockResolvedValueOnce({
        ok: false,
        reason: 'sync-failed',
        boardId: board.id,
        previousBoard: board,
        error: new Error('rename failed'),
        rollback: { ok: false, reason: 'invalid-name' },
      })
      harness.controller.commands.startRename(board)
      await harness.controller.commands.commitRename()
      harness.warn.mockClear()
      const oldReload = deferred<Awaited<ReturnType<BoardRepository['commands']['hydrate']>>>()
      const newRefresh = deferred<{ ok: true }>()
      vi.mocked(harness.catalog.commands.hydrate)
        .mockReturnValueOnce(oldReload.promise)
        .mockReturnValueOnce(newRefresh.promise)
      const reload = harness.controller.commands.reloadCatalog()
      harness.scope.value = 'workspace-b'
      await nextTick()
      const scopeChanged = new Error('BOARD_HYDRATE_SCOPE_CHANGED')
      if (completion === 'success') oldReload.resolve({ ok: true })
      else if (completion === 'failure') oldReload.resolve({ ok: false, error: scopeChanged })
      else oldReload.reject(scopeChanged)
      await reload

      expect(harness.feedback.catalogSyncFailed).not.toHaveBeenCalled()
      expect(harness.feedback.catalogReloaded).not.toHaveBeenCalled()
      expect(harness.warn).not.toHaveBeenCalled()
      expect(harness.controller.viewModel.catalogHydrationFailed.value).toBe(false)
      expect(harness.controller.viewModel.isBoardsLoading.value).toBe(true)
      newRefresh.resolve({ ok: true })
      await harness.controller.commands.initialize()
      expect(harness.controller.viewModel.isBoardsLoading.value).toBe(false)
    },
  )
})
