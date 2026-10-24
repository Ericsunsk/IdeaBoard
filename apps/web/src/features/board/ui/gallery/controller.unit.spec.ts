import { computed, nextTick, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createBoardRepository, type BoardRepository } from '../../session/catalog/repository'
import { makeBoardMeta as makeBoard } from '../../testing'
import type { BoardMeta } from '../../session/catalog/boards'
import type { CatalogNavigation } from './contract'
import type { BoardFeedback } from './useBoardFeedback'
import { createGalleryController } from './controller'
import { createDeferred } from '../../../../test/deferred'

const disposeControllers: (() => void)[] = []

afterEach(() => {
  disposeControllers.splice(0).forEach((dispose) => dispose())
  vi.restoreAllMocks()
})

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
      hydrate: vi.fn(async () => ({ isOk: true as const })),
      create: vi.fn(async () => ({ isOk: true as const, board: makeBoard('created-board') })),
      rename: vi.fn(async ({ boardId, nextName }) => ({
        isOk: true as const,
        board: makeBoard(boardId, nextName),
      })),
      duplicate: vi.fn(async () => ({ isOk: true as const, board: makeBoard('copied-board') })),
      delete: vi.fn(async () => ({ isOk: true as const, isRemoteWasMissing: false })),
    },
    observeDocument: vi.fn(() => vi.fn()),
    dispose: vi.fn(),
  }
  const scenes = {
    sceneMap: ref({}),
    workspaceId: ref(null),
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
    scenes,
    navigation,
    feedback,
    warn,
  })
  disposeControllers.push(controller.dispose)

  return { boards, scope, catalog, scenes, navigation, feedback, warn, controller }
}

describe('createGalleryController', () => {
  it('closes the active menu before creating and navigates only after the catalog command succeeds', async () => {
    const harness = createHarness()
    harness.controller.commands.toggleBoardMenu('board-1')
    vi.mocked(harness.catalog.commands.create).mockImplementation(async () => {
      expect(harness.controller.viewModel.activeMenuBoardId.value).toBeNull()
      expect(harness.navigation.openBoard).not.toHaveBeenCalled()
      return { isOk: true, board: makeBoard('created-board') }
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
      isOk: false,
      reason: 'revision-conflict',
      boardId: board.id,
      previousBoard: board,
      error: syncError,
      rollback: { isOk: true, board },
      hydrate: { isOk: false, error: new Error('hydrate failed') },
    })
    harness.controller.commands.startRename(board)
    harness.controller.commands.updateRenameDraft('Conflicting')

    await harness.controller.commands.commitRename()

    expect(harness.feedback.renameSyncFailed).toHaveBeenCalledOnce()
    expect(harness.controller.viewModel.renamingBoardId.value).toBeNull()
    expect(harness.controller.viewModel.catalogRecoveryBoardId.value).toBe(board.id)
    expect(harness.warn).toHaveBeenCalledWith('[boards] failed to sync renamed board', syncError)

    vi.mocked(harness.catalog.commands.hydrate)
      .mockResolvedValueOnce({ isOk: false, error: new Error('still unavailable') })
      .mockResolvedValueOnce({ isOk: true })

    await harness.controller.commands.reloadCatalog()
    expect(harness.feedback.catalogSyncFailed).toHaveBeenCalledOnce()
    expect(harness.controller.viewModel.catalogRecoveryBoardId.value).toBe(board.id)
    expect(harness.controller.viewModel.isCatalogReloadPending.value).toBe(false)

    await harness.controller.commands.reloadCatalog()
    expect(harness.catalog.commands.hydrate).toHaveBeenLastCalledWith({
      shouldIncludeDocuments: false,
      shouldForce: true,
      shouldPreserveLocalNewer: false,
      rollbackBoardIds: [board.id],
    })
    expect(harness.controller.viewModel.catalogRecoveryBoardId.value).toBeNull()
    expect(harness.feedback.catalogReloaded).toHaveBeenCalledOnce()
  })

  it('keeps the delete target recoverable after failure and always releases the pending guard', async () => {
    const harness = createHarness()
    const board = makeBoard('board-1')
    const firstDelete = createDeferred<
      | { isOk: true; isRemoteWasMissing: boolean }
      | { isOk: false; reason: 'sync-failed'; error: unknown }
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
    firstDelete.resolve({ isOk: false, reason: 'sync-failed', error })
    await pendingConfirmation

    expect(harness.controller.viewModel.isDeletePending.value).toBe(false)
    expect(harness.controller.viewModel.deleteDialogVisible.value).toBe(true)
    expect(harness.controller.viewModel.pendingDeleteBoard.value).toEqual(board)
    expect(harness.feedback.deleteFailed).toHaveBeenCalledOnce()
    expect(harness.warn).toHaveBeenCalledWith('[boards] failed to sync deleted board', error)

    vi.mocked(harness.catalog.commands.delete).mockResolvedValueOnce({
      isOk: true,
      isRemoteWasMissing: false,
    })
    await harness.controller.commands.confirmDelete()

    expect(harness.controller.viewModel.isDeletePending.value).toBe(false)
    expect(harness.controller.viewModel.deleteDialogVisible.value).toBe(false)
    expect(harness.controller.viewModel.pendingDeleteBoard.value).toBeNull()
    expect(harness.feedback.deleted).toHaveBeenCalledOnce()
  })

  it('owns loading, filtering, and sorting', async () => {
    const harness = createHarness([
      makeBoard('older', 'Alpha', 10),
      makeBoard('newer', 'Beta Canvas', 20),
    ])

    await nextTick()
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
      isOk: false,
      error: hydrateError,
    })
    await harness.controller.commands.initialize()
    expect(harness.controller.viewModel.isBoardsLoading.value).toBe(false)
    expect(harness.controller.viewModel.catalogHydrationFailed.value).toBe(true)
    expect(harness.feedback.catalogSyncFailed).toHaveBeenCalledOnce()

    vi.mocked(harness.catalog.commands.hydrate).mockResolvedValueOnce({ isOk: true })
    await harness.controller.commands.reloadCatalog()
    expect(harness.controller.viewModel.catalogHydrationFailed.value).toBe(false)
    expect(harness.feedback.catalogReloaded).toHaveBeenCalledOnce()
  })

  it('keeps create and duplicate failure feedback behind catalog command results', async () => {
    const harness = createHarness()
    const createError = new Error('create sync failed')
    vi.mocked(harness.catalog.commands.create).mockResolvedValueOnce({
      isOk: false,
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
      return { isOk: true, board: makeBoard('copy') }
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
      isOk: false,
      reason: 'sync-failed',
      boardId: board.id,
      previousBoard: board,
      error: new Error('failed'),
      rollback: { isOk: false, reason: 'invalid-name' },
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
  })

  it('exposes scene state without any theme-driven reprojection', async () => {
    const boards = ref([makeBoard('board-1')])
    const sceneMap = ref({ 'board-1': { contentVersion: 'cv-1', scene: null } })
    const workspaceId = ref<string | null>('ws_1')
    const scenes = {
      sceneMap,
      workspaceId,
    }
    const catalog: BoardRepository = {
      viewModel: {
        scope: ref('user-1'),
        boards,
        byId: computed(() => new Map()),
      },
      commands: {
        hydrate: vi.fn(async () => ({ isOk: true as const })),
        create: vi.fn(async () => ({ isOk: true as const, board: makeBoard('b') })),
        rename: vi.fn(async () => ({ isOk: true as const, board: makeBoard('b') })),
        duplicate: vi.fn(async () => ({ isOk: true as const, board: makeBoard('b') })),
        delete: vi.fn(async () => ({ isOk: true as const, isRemoteWasMissing: false })),
      },
      observeDocument: vi.fn(() => vi.fn()),
      dispose: vi.fn(),
    }

    // Theme switches only re-map tokens in CSS: the controller takes no theme
    // input and never re-indexes scenes for it.
    const controller = createGalleryController({
      catalog,
      scenes,
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
      warn: vi.fn(),
    })

    expect(controller.viewModel.sceneMap.value).toEqual(sceneMap.value)
    expect(controller.viewModel.workspaceId.value).toBe('ws_1')

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
      isOk: false,
      error: new Error('no workspace'),
    })
    await harness.controller.commands.initialize()
    expect(harness.controller.viewModel.catalogHydrationFailed.value).toBe(true)
    expect(harness.feedback.catalogSyncFailed).toHaveBeenCalledTimes(1)
    expect(harness.catalog.commands.hydrate).toHaveBeenCalledTimes(1)

    harness.scope.value = null
    await nextTick()
    expect(harness.catalog.commands.hydrate).toHaveBeenCalledTimes(1)

    vi.mocked(harness.catalog.commands.hydrate).mockResolvedValueOnce({ isOk: true })
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
      const oldRefresh =
        createDeferred<Awaited<ReturnType<BoardRepository['commands']['hydrate']>>>()
      const newRefresh =
        createDeferred<Awaited<ReturnType<BoardRepository['commands']['hydrate']>>>()
      let currentScope = 'workspace-a'
      let changeScope!: (scope: string) => void
      const hydrate = vi
        .fn<BoardRepository['commands']['hydrate']>()
        .mockResolvedValueOnce({ isOk: true })
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
      if (completion === 'failure') oldRefresh.resolve({ isOk: false, error: scopeChanged })
      else oldRefresh.reject(scopeChanged)
      await vi.waitFor(() => expect(hydrate).toHaveBeenCalledTimes(3))

      expect(harness.feedback.catalogSyncFailed).not.toHaveBeenCalled()
      expect(harness.warn).not.toHaveBeenCalled()
      expect(harness.controller.viewModel.catalogHydrationFailed.value).toBe(false)
      expect(harness.controller.viewModel.visibleBoards.value[0]?.id).toBe('workspace-b')

      newRefresh.resolve({ isOk: true })
      await harness.controller.commands.initialize()
      expect(harness.controller.viewModel.catalogHydrationFailed.value).toBe(false)
      expect(harness.feedback.catalogSyncFailed).not.toHaveBeenCalled()
    },
  )

  it('keeps the initial loading state until the current workspace hydrate completes', async () => {
    const harness = createHarness()
    const oldRefresh = createDeferred<{ isOk: true }>()
    const newRefresh = createDeferred<{ isOk: true }>()
    vi.mocked(harness.catalog.commands.hydrate)
      .mockReturnValueOnce(oldRefresh.promise)
      .mockReturnValueOnce(newRefresh.promise)
    const initialLoad = harness.controller.commands.initialize()

    harness.scope.value = 'workspace-b'
    await nextTick()
    oldRefresh.resolve({ isOk: true })
    await initialLoad

    expect(harness.controller.viewModel.isBoardsLoading.value).toBe(true)
    newRefresh.resolve({ isOk: true })
    await harness.controller.commands.initialize()
    expect(harness.controller.viewModel.isBoardsLoading.value).toBe(false)
  })

  it('hydrates a workspace that becomes ready while an obsolete initial load is pending', async () => {
    const harness = createHarness()
    const oldRefresh = createDeferred<{ isOk: true }>()
    const newRefresh = createDeferred<{ isOk: true }>()
    vi.mocked(harness.catalog.commands.hydrate)
      .mockReturnValueOnce(oldRefresh.promise)
      .mockReturnValueOnce(newRefresh.promise)
    const initialLoad = harness.controller.commands.initialize()
    harness.scope.value = null
    await nextTick()
    harness.scope.value = 'workspace-b'
    await nextTick()

    expect(harness.catalog.commands.hydrate).toHaveBeenCalledTimes(2)
    oldRefresh.resolve({ isOk: true })
    await initialLoad
    expect(harness.controller.viewModel.isBoardsLoading.value).toBe(true)
    newRefresh.resolve({ isOk: true })
    await harness.controller.commands.initialize()
    expect(harness.controller.viewModel.isBoardsLoading.value).toBe(false)
  })

  it('preserves a current workspace failure and reports its exception after an old success', async () => {
    const harness = createHarness()
    const oldRefresh = createDeferred<{ isOk: true }>()
    const newRefresh = createDeferred<{ isOk: true }>()
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
    oldRefresh.resolve({ isOk: true })
    await initialLoad
    expect(harness.controller.viewModel.catalogHydrationFailed.value).toBe(true)
    expect(harness.feedback.catalogSyncFailed).toHaveBeenCalledOnce()
  })

  it('ignores a focus refresh superseded by another focus refresh in the same workspace', async () => {
    const harness = createHarness()
    await harness.controller.commands.initialize()
    const oldRefresh = createDeferred<{ isOk: true }>()
    const latestRefresh =
      createDeferred<Awaited<ReturnType<BoardRepository['commands']['hydrate']>>>()
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
    latestRefresh.resolve({ isOk: false, error: new Error('latest failure') })
    await harness.controller.commands.initialize()
    oldRefresh.resolve({ isOk: true })
    await previous

    expect(harness.controller.viewModel.catalogHydrationFailed.value).toBe(true)
    expect(harness.feedback.catalogSyncFailed).toHaveBeenCalledOnce()
  })

  it('does not release the current retry guard when an old workspace retry finishes', async () => {
    const harness = createHarness()
    vi.mocked(harness.catalog.commands.hydrate).mockResolvedValueOnce({
      isOk: false,
      error: new Error('initial failure'),
    })
    await harness.controller.commands.initialize()
    const oldRetry = createDeferred<{ isOk: true }>()
    vi.mocked(harness.catalog.commands.hydrate).mockReturnValueOnce(oldRetry.promise)
    const retry = harness.controller.commands.reloadCatalog()
    vi.mocked(harness.catalog.commands.hydrate).mockResolvedValueOnce({
      isOk: false,
      error: new Error('new workspace failure'),
    })
    harness.scope.value = 'workspace-b'
    await nextTick()
    await harness.controller.commands.initialize()
    const newRetry = createDeferred<{ isOk: true }>()
    vi.mocked(harness.catalog.commands.hydrate).mockReturnValueOnce(newRetry.promise)
    const currentRetry = harness.controller.commands.reloadCatalog()
    oldRetry.resolve({ isOk: true })
    await retry

    expect(harness.controller.viewModel.isCatalogReloadPending.value).toBe(true)
    expect(harness.feedback.catalogReloaded).not.toHaveBeenCalled()
    newRetry.resolve({ isOk: true })
    await currentRetry
    expect(harness.controller.viewModel.isCatalogReloadPending.value).toBe(false)
    expect(harness.feedback.catalogReloaded).toHaveBeenCalledOnce()
  })

  it('ignores hydration and retry completions after disposal', async () => {
    const harness = createHarness()
    vi.mocked(harness.catalog.commands.hydrate).mockResolvedValueOnce({
      isOk: false,
      error: new Error('initial failure'),
    })
    await harness.controller.commands.initialize()
    const retryResult = createDeferred<{ isOk: true }>()
    vi.mocked(harness.catalog.commands.hydrate).mockReturnValueOnce(retryResult.promise)
    const retry = harness.controller.commands.reloadCatalog()
    harness.controller.dispose()
    retryResult.resolve({ isOk: true })
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
        isOk: false,
        reason: 'sync-failed',
        boardId: board.id,
        previousBoard: board,
        error: new Error('rename failed'),
        rollback: { isOk: false, reason: 'invalid-name' },
      })
      harness.controller.commands.startRename(board)
      await harness.controller.commands.commitRename()
      harness.warn.mockClear()
      const oldReload =
        createDeferred<Awaited<ReturnType<BoardRepository['commands']['hydrate']>>>()
      const newRefresh = createDeferred<{ isOk: true }>()
      vi.mocked(harness.catalog.commands.hydrate)
        .mockReturnValueOnce(oldReload.promise)
        .mockReturnValueOnce(newRefresh.promise)
      const reload = harness.controller.commands.reloadCatalog()
      harness.scope.value = 'workspace-b'
      await nextTick()
      const scopeChanged = new Error('BOARD_HYDRATE_SCOPE_CHANGED')
      if (completion === 'success') oldReload.resolve({ isOk: true })
      else if (completion === 'failure') oldReload.resolve({ isOk: false, error: scopeChanged })
      else oldReload.reject(scopeChanged)
      await reload

      expect(harness.feedback.catalogSyncFailed).not.toHaveBeenCalled()
      expect(harness.feedback.catalogReloaded).not.toHaveBeenCalled()
      expect(harness.warn).not.toHaveBeenCalled()
      expect(harness.controller.viewModel.catalogHydrationFailed.value).toBe(false)
      expect(harness.controller.viewModel.isBoardsLoading.value).toBe(true)
      newRefresh.resolve({ isOk: true })
      await harness.controller.commands.initialize()
      expect(harness.controller.viewModel.isBoardsLoading.value).toBe(false)
    },
  )
})
