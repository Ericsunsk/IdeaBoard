import { computed, readonly, shallowRef, type ComputedRef, type Ref } from 'vue'
import {
  getAuthStorageUserScope,
  subscribeAuthStorageScopeChange,
} from '../../../../shared/utils/auth'
import { listBoards, subscribeBoardChanges, type BoardMeta } from './boards'
import {
  createBoardCommands,
  type BoardCommands,
  type BoardRenameRemoteResult,
  type BoardRollbackRenameResult,
  type BoardScopeChangedResult,
} from './commands'
import {
  hydrateBoardsFromServer,
  type HydrateBoardsFromServerOptions,
  type HydrateBoardsFromServerResult,
} from '../sync/boardSync'

type BoardRepositoryMutations = Pick<
  BoardCommands,
  'create' | 'renameOptimistic' | 'rollbackOptimisticRename' | 'duplicate' | 'delete'
>

export type BoardRepositoryRenameResult =
  | { ok: true; board: BoardMeta }
  | { ok: false; reason: 'local-name-conflict' | 'invalid-name'; error?: undefined }
  | {
      ok: false
      reason: 'remote-name-conflict' | 'sync-failed'
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
  hydrate(
    options?: HydrateBoardsFromServerOptions,
  ): Promise<HydrateBoardsFromServerResult>
  create: BoardCommands['create']
  rename(input: { boardId: string; nextName: string }): Promise<BoardRepositoryRenameResult>
  duplicate: BoardCommands['duplicate']
  delete: BoardCommands['delete']
}

export type BoardRepository = {
  viewModel: BoardRepositoryViewModel
  commands: BoardRepositoryCommands
  dispose(): void
}

export type BoardRepositoryDependencies = {
  listBoards(): BoardMeta[]
  subscribe(listener: () => void): () => void
  getScope(): string | null
  subscribeScope(listener: (scope: string | null) => void): () => void
  createCommands(scopeGuard?: { isCurrent: () => boolean }): BoardRepositoryMutations
  hydrate(
    options?: HydrateBoardsFromServerOptions,
  ): Promise<HydrateBoardsFromServerResult>
}

const defaultDependencies: BoardRepositoryDependencies = {
  listBoards,
  subscribe: subscribeBoardChanges,
  getScope: getAuthStorageUserScope,
  subscribeScope: subscribeAuthStorageScopeChange,
  createCommands: (scopeGuard) =>
    createBoardCommands({ isMutationCurrent: scopeGuard?.isCurrent ?? (() => true) }),
  hydrate: hydrateBoardsFromServer,
}

function cloneBoards(boards: BoardMeta[]): BoardMeta[] {
  return boards.map((board) => ({ ...board }))
}

function toRenameFailure(
  remote: Exclude<BoardRenameRemoteResult, { ok: true }>,
  rollback: BoardRollbackRenameResult,
  hydrate?: HydrateBoardsFromServerResult,
): BoardRepositoryRenameResult {
  return {
    ok: false,
    reason: remote.reason,
    boardId: remote.boardId,
    previousBoard: remote.previousBoard,
    error: remote.error,
    rolledBackBoard: rollback.ok ? rollback.board : undefined,
    rollback,
    hydrate,
  }
}

export function createBoardRepository(
  dependencyOverrides: Partial<BoardRepositoryDependencies> = {},
): BoardRepository {
  const deps = { ...defaultDependencies, ...dependencyOverrides }
  const scope = shallowRef(deps.getScope())
  const boards = shallowRef<BoardMeta[]>(scope.value ? cloneBoards(deps.listBoards()) : [])
  const byId = computed(() => new Map(boards.value.map((board) => [board.id, board] as const)))
  let mutationTail: Promise<void> = Promise.resolve()
  let scopeGeneration = 0

  function refresh() {
    boards.value = scope.value ? cloneBoards(deps.listBoards()) : []
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
    return { ok: false, reason: 'scope-changed' }
  }

  const unsubscribe = deps.subscribe(refresh)
  const unsubscribeScope = deps.subscribeScope((nextScope) => {
    if (scope.value === nextScope) return
    scopeGeneration += 1
    scope.value = nextScope
    refresh()
  })

  const commands: BoardRepositoryCommands = {
    hydrate(options = {}) {
      const token = captureScope()
      return enqueue(async () => {
        if (!isScopeCurrent(token)) {
          return { ok: false, error: new BoardRepositoryScopeChangedError() }
        }
        return deps.hydrate(options)
      })
    },

    create(input) {
      const token = captureScope()
      return enqueue(async () => {
        if (!isScopeCurrent(token)) return scopeChangedResult()
        const mutations = deps.createCommands({ isCurrent: () => isScopeCurrent(token) })
        return mutations.create(input)
      })
    },

    rename(input) {
      const token = captureScope()
      return enqueue(async () => {
        if (!isScopeCurrent(token)) return scopeChangedResult()
        const mutations = deps.createCommands({ isCurrent: () => isScopeCurrent(token) })
        const optimistic = mutations.renameOptimistic(input)
        if (!optimistic.ok) return optimistic

        const remote = await optimistic.confirm()
        if (!isScopeCurrent(token)) return scopeChangedResult()
        if (remote.ok) return remote

        const rollback = mutations.rollbackOptimisticRename({
          boardId: remote.boardId,
          previousName: remote.previousBoard.name,
        })
        const hydrate =
          remote.reason === 'remote-name-conflict'
            ? await deps.hydrate({
                preserveLocalNewer: false,
                rollbackBoardIds: [remote.boardId],
              })
            : undefined
        return toRenameFailure(remote, rollback, hydrate)
      })
    },

    duplicate(input) {
      const token = captureScope()
      return enqueue(async () => {
        if (!isScopeCurrent(token)) return scopeChangedResult()
        const mutations = deps.createCommands({ isCurrent: () => isScopeCurrent(token) })
        return mutations.duplicate(input)
      })
    },

    delete(input) {
      const token = captureScope()
      return enqueue(async () => {
        if (!isScopeCurrent(token)) return scopeChangedResult()
        const mutations = deps.createCommands({ isCurrent: () => isScopeCurrent(token) })
        return mutations.delete(input)
      })
    },
  }

  return {
    viewModel: {
      scope: readonly(scope),
      boards: readonly(boards),
      byId,
    },
    commands,
    dispose() {
      unsubscribe()
      unsubscribeScope()
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
