import { computed, readonly, shallowRef, type ComputedRef, type Ref } from 'vue'
import {
  boardScopeKey,
  getBoardScope,
  isCompleteBoardScope,
  subscribeBoardScopeChange,
} from './scope'
import { listBoards, subscribeBoardChanges, type BoardMeta } from './boards'
import { registerBoardDraftGuard } from '../sync/boardDraftState'
import {
  createBoardCommands,
  type BoardCommands,
  type BoardRenameRemoteResult,
  type BoardRollbackRenameResult,
  type BoardScopeChangedResult,
} from './commands'
import {
  hydrateBoardsFromServer,
  observeBoardServerSnapshots,
  type BoardSyncConflictKind,
  type HydrateBoardsFromServerOptions,
  type HydrateBoardsFromServerResult,
} from '../sync/boardSync'

type BoardRepositoryMutations = Pick<
  BoardCommands,
  | 'create'
  | 'createAndOpen'
  | 'renameOptimistic'
  | 'rollbackOptimisticRename'
  | 'duplicate'
  | 'delete'
>

export type BoardRepositoryRenameResult =
  | { isOk: true; board: BoardMeta }
  | { isOk: false; reason: 'local-name-conflict' | 'invalid-name'; error?: undefined }
  | {
      isOk: false
      reason: BoardSyncConflictKind | 'sync-failed'
      boardId: string
      previousBoard: BoardMeta
      error: unknown
      rolledBackBoard?: BoardMeta | undefined
      rollback: BoardRollbackRenameResult
      hydrate?: HydrateBoardsFromServerResult | undefined
    }
  | BoardScopeChangedResult

export type BoardRepositoryViewModel = {
  scope: Readonly<Ref<string | null>>
  boards: Readonly<Ref<readonly BoardMeta[]>>
  byId: ComputedRef<ReadonlyMap<string, BoardMeta>>
}

export type BoardRepositoryCommands = {
  hydrate(options?: HydrateBoardsFromServerOptions): Promise<HydrateBoardsFromServerResult>
  create: BoardCommands['create']
  createAndOpen: BoardCommands['createAndOpen']
  rename(input: { boardId: string; nextName: string }): Promise<BoardRepositoryRenameResult>
  duplicate: BoardCommands['duplicate']
  delete: BoardCommands['delete']
}

export type BoardRepository = {
  viewModel: BoardRepositoryViewModel
  commands: BoardRepositoryCommands
  observeDocument(boardId: string): () => void
  dispose(): void
  registerDraftGuard?: (guard: (boardId: string) => boolean) => () => void
}

export type BoardRepositoryDependencies = {
  listBoards(): BoardMeta[]
  subscribe(listener: () => void): () => void
  getScope(): string | null
  subscribeScope(listener: (scope: string | null) => void): () => void
  createCommands(scopeGuard?: { isCurrent: () => boolean }): BoardRepositoryMutations
  hydrate(options?: HydrateBoardsFromServerOptions): Promise<HydrateBoardsFromServerResult>
  observeServer?: typeof observeBoardServerSnapshots
}

function currentBoardScopeKey(): string | null {
  const scope = getBoardScope()
  return isCompleteBoardScope(scope) ? boardScopeKey(scope) : null
}

function subscribeBoardRepositoryScope(listener: (scope: string | null) => void): () => void {
  return subscribeBoardScopeChange(() => {
    listener(currentBoardScopeKey())
  })
}

const defaultDependencies: BoardRepositoryDependencies = {
  listBoards,
  subscribe: subscribeBoardChanges,
  getScope: currentBoardScopeKey,
  subscribeScope: subscribeBoardRepositoryScope,
  createCommands: (scopeGuard) =>
    createBoardCommands({ isMutationCurrent: scopeGuard?.isCurrent ?? (() => true) }),
  hydrate: hydrateBoardsFromServer,
  observeServer: observeBoardServerSnapshots,
}

function cloneBoards(boards: BoardMeta[]): BoardMeta[] {
  return boards.map((board) => ({ ...board }))
}

function toRenameFailure(
  remote: Exclude<BoardRenameRemoteResult, { isOk: true }>,
  rollback: BoardRollbackRenameResult,
  hydrate?: HydrateBoardsFromServerResult,
): BoardRepositoryRenameResult {
  return {
    isOk: false,
    reason: remote.reason,
    boardId: remote.boardId,
    previousBoard: remote.previousBoard,
    error: remote.error,
    rolledBackBoard: rollback.isOk ? rollback.board : undefined,
    rollback,
    hydrate,
  }
}

export function createBoardRepository(
  dependencyOverrides: Partial<BoardRepositoryDependencies> = {},
): BoardRepository {
  const deps = { ...defaultDependencies, ...dependencyOverrides }
  // Custom repository ports own their own observation lifecycle.
  if (Object.keys(dependencyOverrides).length && !dependencyOverrides.observeServer)
    delete deps.observeServer
  const scope = shallowRef(deps.getScope())
  const boards = shallowRef<BoardMeta[]>(scope.value ? cloneBoards(deps.listBoards()) : [])
  const byId = computed(() => new Map(boards.value.map((board) => [board.id, board] as const)))
  let mutationTail: Promise<void> = Promise.resolve()
  let scopeGeneration = 0
  let serverBoards: BoardMeta[] | null = null
  let stopServer: (() => void) | undefined
  let isObservingServer = false
  let isHydrating = false
  let isDisposed = false
  const documentObservers = new Map<symbol, string>()
  const draftGuardStops = new Set<() => void>()

  function refresh() {
    if (!scope.value) {
      boards.value = []
      return
    }
    const local = deps.listBoards()
    // Synced catalog rows come from Query; offline creations remain a local
    // overlay. Canvas reads its draft metadata through the store boundary.
    boards.value = serverBoards
      ? cloneBoards([
          ...serverBoards,
          ...local.filter(
            (board) =>
              board.revision === undefined &&
              !serverBoards!.some((remote) => remote.id === board.id),
          ),
        ])
      : cloneBoards(local)
  }

  function observeServer() {
    if (!isObservingServer || isHydrating || isDisposed) return
    stopServer?.()
    stopServer = deps.observeServer?.(
      (next) => {
        serverBoards = next
        refresh()
      },
      [...new Set(documentObservers.values())],
    )
  }

  function enqueue<T>(operation: () => Promise<T>): Promise<T> {
    const task = mutationTail.then(operation, operation)
    mutationTail = task.then(
      () => undefined,
      () => undefined,
    )
    return task
  }

  function captureScope() {
    return { scope: scope.value, generation: scopeGeneration }
  }

  function isScopeCurrent(token: ReturnType<typeof captureScope>): boolean {
    return !!token.scope && token.scope === scope.value && token.generation === scopeGeneration
  }

  function scopeChangedResult(): BoardScopeChangedResult {
    return { isOk: false, reason: 'scope-changed' }
  }

  function scopedMutations(token: ReturnType<typeof captureScope>): BoardRepositoryMutations {
    return deps.createCommands({ isCurrent: () => isScopeCurrent(token) })
  }

  function enqueueScopedMutation<T>(
    operation: (mutations: BoardRepositoryMutations) => Promise<T>,
  ): Promise<T | BoardScopeChangedResult> {
    const token = captureScope()
    return enqueue(async () => {
      if (!isScopeCurrent(token)) return scopeChangedResult()
      return operation(scopedMutations(token))
    })
  }

  const unsubscribe = deps.subscribe(refresh)
  const unsubscribeScope = deps.subscribeScope((nextScope) => {
    if (scope.value === nextScope) return
    scopeGeneration += 1
    // Navigation must not queue the new scope behind an obsolete mutation.
    if (deps.observeServer) mutationTail = Promise.resolve()
    stopServer?.()
    stopServer = undefined
    isObservingServer = false
    isHydrating = false
    documentObservers.clear()
    serverBoards = null
    scope.value = nextScope
    refresh()
  })

  const commands: BoardRepositoryCommands = {
    hydrate(options = {}) {
      const token = captureScope()
      return enqueue(async () => {
        if (!isScopeCurrent(token)) {
          return { isOk: false, error: new BoardRepositoryScopeChangedError() }
        }
        isHydrating = true
        stopServer?.()
        stopServer = undefined
        const observeOptions = deps.observeServer
          ? {
              ...options,
              // Lazy default: Gallery/Canvas start list-only; documents load
              // per boardId single pull (onlyBoardIds M→1), never full walks.
              shouldIncludeDocuments: options.shouldIncludeDocuments ?? false,
              shouldForce: options.shouldForce ?? true,
            }
          : options
        try {
          const result = await deps.hydrate(observeOptions)
          if (result.isOk && isScopeCurrent(token)) isObservingServer = true
          return result
        } finally {
          if (isScopeCurrent(token)) {
            isHydrating = false
            observeServer()
          }
        }
      })
    },

    create(input) {
      return enqueueScopedMutation((mutations) => mutations.create(input))
    },

    createAndOpen(input) {
      return enqueueScopedMutation((mutations) => mutations.createAndOpen(input))
    },

    rename(input) {
      const token = captureScope()
      return enqueue(async () => {
        if (!isScopeCurrent(token)) return scopeChangedResult()
        const mutations = scopedMutations(token)
        const optimistic = mutations.renameOptimistic(input)
        if (!optimistic.isOk) return optimistic

        const remote = await optimistic.confirm()
        if (!isScopeCurrent(token)) return scopeChangedResult()
        if (remote.isOk) return remote

        const rollback = mutations.rollbackOptimisticRename({
          boardId: remote.boardId,
          previousName: remote.previousBoard.name,
        })
        const hydrate =
          remote.reason === 'revision-conflict' || remote.reason === 'idempotency-conflict'
            ? await deps.hydrate({
                shouldPreserveLocalNewer: false,
                rollbackBoardIds: [remote.boardId],
              })
            : undefined
        return toRenameFailure(remote, rollback, hydrate)
      })
    },

    duplicate(input) {
      return enqueueScopedMutation((mutations) => mutations.duplicate(input))
    },

    delete(input) {
      return enqueueScopedMutation((mutations) => mutations.delete(input))
    },
  }

  return {
    viewModel: {
      scope: readonly(scope),
      boards: readonly(boards),
      byId,
    },
    commands,
    observeDocument(boardId) {
      const normalized = boardId.trim()
      if (!normalized || !scope.value || isDisposed) return () => {}
      const token = Symbol(normalized)
      documentObservers.set(token, normalized)
      observeServer()
      return () => {
        if (documentObservers.delete(token)) observeServer()
      }
    },
    dispose() {
      isDisposed = true
      documentObservers.clear()
      draftGuardStops.forEach((stop) => stop())
      draftGuardStops.clear()
      stopServer?.()
      unsubscribe()
      unsubscribeScope()
    },
    registerDraftGuard(guard) {
      if (isDisposed) return () => {}
      const unregister = registerBoardDraftGuard(guard)
      const stop = () => {
        unregister()
        draftGuardStops.delete(stop)
      }
      draftGuardStops.add(stop)
      return stop
    },
  }
}

class BoardRepositoryScopeChangedError extends Error {
  constructor() {
    super('BOARD_REPOSITORY_SCOPE_CHANGED')
    this.name = 'BoardRepositoryScopeChangedError'
  }
}

const boardRepository = createBoardRepository()

export function useBoardRepository(): BoardRepository {
  return boardRepository
}
