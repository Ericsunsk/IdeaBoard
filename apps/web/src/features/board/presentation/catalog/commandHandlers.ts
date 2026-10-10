import { readonly, ref, watch, type Ref } from 'vue'
import type { BoardRepository } from '../../../board/application/catalog/repository'
import type { HydrateBoardsFromServerOptions } from '../../application/sync/boardSync'
import type { CatalogEntry, CatalogNavigation } from './contract'
import type { BoardFeedback } from './useBoardFeedback'

type CatalogCommandState = {
  activeMenuBoardId: Ref<string | null>
  renamingBoardId: Ref<string | null>
  renameDraft: Ref<string>
  catalogRecoveryBoardId: Ref<string | null>
  isCatalogReloadPending: Ref<boolean>
  pendingDeleteBoard: Ref<CatalogEntry | null>
  isDeletePending: Ref<boolean>
}

type CatalogCommandDependencies = {
  catalog: BoardRepository
  navigation: CatalogNavigation
  feedback: BoardFeedback
  warn: (...data: unknown[]) => void
  state: CatalogCommandState
  clearRename(suppressNextOpen: boolean): void
  resetDeleteState(): void
}

async function runCatalogCommand<T>(
  operation: () => Promise<T>,
  onError: (error: unknown) => void,
): Promise<T | undefined> {
  try {
    return await operation()
  } catch (error) {
    onError(error)
    return undefined
  }
}

function createCatalogHydration({
  catalog,
  feedback,
  warn,
  state,
}: Pick<CatalogCommandDependencies, 'catalog' | 'feedback' | 'warn' | 'state'>) {
  const isBoardsLoading = ref(true)
  const catalogHydrationFailed = ref(false)
  let initializePromise: Promise<void> | null = null
  let generation = 0
  let disposed = false

  const stopScopeWatch = watch(
    catalog.viewModel.scope,
    () => {
      generation += 1
      initializePromise = null
      state.isCatalogReloadPending.value = false
    },
    { flush: 'sync' },
  )

  function capture() {
    return { scope: catalog.viewModel.scope.value, generation }
  }

  function isCurrent(token: ReturnType<typeof capture>): boolean {
    return (
      !disposed && token.scope === catalog.viewModel.scope.value && token.generation === generation
    )
  }

  function initialize(
    options: { force?: boolean; hydrateOptions?: HydrateBoardsFromServerOptions | undefined } = {},
  ): Promise<void> {
    if (disposed) return Promise.resolve()
    if (options.force) initializePromise = null
    if (initializePromise) return initializePromise
    generation += 1
    state.isCatalogReloadPending.value = false
    const token = capture()
    initializePromise = (async () => {
      try {
        const result = await (options.hydrateOptions
          ? catalog.commands.hydrate(options.hydrateOptions)
          : catalog.commands.hydrate())
        if (!isCurrent(token)) return
        if (!result.ok) {
          if (!options.hydrateOptions) catalogHydrationFailed.value = true
          feedback.catalogSyncFailed()
          return
        }
        catalogHydrationFailed.value = false
        if (options.hydrateOptions) state.catalogRecoveryBoardId.value = null
      } catch (error) {
        if (!isCurrent(token)) return
        if (!options.hydrateOptions) catalogHydrationFailed.value = true
        warn(
          options.hydrateOptions
            ? '[boards] failed to reload catalog'
            : '[boards] failed to hydrate catalog',
          error,
        )
        feedback.catalogSyncFailed()
      } finally {
        if (isCurrent(token)) isBoardsLoading.value = false
      }
    })()
    return initializePromise
  }

  async function reloadCatalog() {
    const boardId = state.catalogRecoveryBoardId.value
    if (disposed || state.isCatalogReloadPending.value) return
    if (!catalogHydrationFailed.value && !boardId) return
    const pending = initialize({
      force: true,
      hydrateOptions: boardId
        ? { preserveLocalNewer: false, rollbackBoardIds: [boardId] }
        : undefined,
    })
    const token = capture()
    state.isCatalogReloadPending.value = true
    try {
      await pending
      if (
        isCurrent(token) &&
        !catalogHydrationFailed.value &&
        !state.catalogRecoveryBoardId.value
      ) {
        feedback.catalogReloaded()
      }
    } finally {
      if (isCurrent(token)) state.isCatalogReloadPending.value = false
    }
  }

  return {
    viewModel: {
      isBoardsLoading: readonly(isBoardsLoading),
      catalogHydrationFailed: readonly(catalogHydrationFailed),
      catalogRecoveryBoardId: readonly(state.catalogRecoveryBoardId),
      isCatalogReloadPending: readonly(state.isCatalogReloadPending),
    },
    initialize,
    reloadCatalog,
    dispose() {
      disposed = true
      stopScopeWatch()
    },
  }
}

export function createCommandHandlers({
  catalog,
  navigation,
  feedback,
  warn,
  state,
  clearRename,
  resetDeleteState,
}: CatalogCommandDependencies) {
  const hydration = createCatalogHydration({ catalog, feedback, warn, state })
  async function createAndOpenBoard() {
    state.activeMenuBoardId.value = null
    const result = await runCatalogCommand(
      () => catalog.commands.create(),
      (error) => {
        warn('[boards] failed to create board', error)
        feedback.catalogSyncFailed()
      },
    )
    if (!result) return
    if (!result.ok) {
      if (result.reason === 'scope-changed') return
      feedback.createFailed(result)
      warn('[boards] failed to sync new board', result.error)
      return
    }
    navigation.openBoard(result.board.id)
  }

  async function duplicateBoard(boardId: string) {
    state.activeMenuBoardId.value = null
    const result = await runCatalogCommand(
      () => catalog.commands.duplicate({ sourceBoardId: boardId }),
      (error) => {
        warn('[boards] failed to duplicate board', error)
        feedback.catalogSyncFailed()
      },
    )
    if (!result) return
    if (!result.ok) {
      if (result.reason === 'scope-changed') return
      feedback.copyFailed(result)
      if (result.reason !== 'source-missing') {
        warn('[boards] failed to sync duplicated board', result.error)
      }
      return
    }
    feedback.copied()
  }

  async function commitRename() {
    const boardId = state.renamingBoardId.value
    if (!boardId) return
    const result = await catalog.commands.rename({
      boardId,
      nextName: state.renameDraft.value,
    })
    if (!result.ok) {
      switch (result.reason) {
        case 'scope-changed':
          return
        case 'local-name-conflict':
          feedback.renameValidationFailed(result)
          return
        case 'invalid-name':
          clearRename(true)
          feedback.renameValidationFailed(result)
          return
        case 'revision-conflict':
        case 'idempotency-conflict':
        case 'sync-failed':
          clearRename(true)
          feedback.renameSyncFailed(result)
          state.catalogRecoveryBoardId.value =
            result.reason === 'revision-conflict' || result.reason === 'idempotency-conflict'
              ? result.hydrate?.ok === true
                ? null
                : boardId
              : result.rollback.ok
                ? null
                : boardId
          warn('[boards] failed to sync renamed board', result.error)
          return
      }
    }
    clearRename(true)
    feedback.renamed()
  }

  async function confirmDelete() {
    const board = state.pendingDeleteBoard.value
    if (!board || state.isDeletePending.value) return
    state.isDeletePending.value = true
    const result = await runCatalogCommand(
      () => catalog.commands.delete({ boardId: board.id }),
      (error) => {
        const failure = { ok: false as const, reason: 'sync-failed' as const, error }
        feedback.deleteFailed(failure)
        warn('[boards] failed to sync deleted board', error)
      },
    )
    try {
      if (!result) return
      if (!result.ok) {
        if (result.reason === 'scope-changed') {
          resetDeleteState()
          return
        }
        feedback.deleteFailed(result)
        if (result.reason === 'sync-failed') {
          warn('[boards] failed to sync deleted board', result.error)
        } else {
          resetDeleteState()
        }
        return
      }
      if (state.renamingBoardId.value === board.id) {
        clearRename(false)
      }
      resetDeleteState()
      feedback.deleted()
    } finally {
      state.isDeletePending.value = false
    }
  }

  return {
    createAndOpenBoard,
    duplicateBoard,
    commitRename,
    reloadCatalog: hydration.reloadCatalog,
    confirmDelete,
    hydration,
  }
}
