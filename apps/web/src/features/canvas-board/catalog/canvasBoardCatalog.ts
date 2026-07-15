import { computed, readonly, shallowRef, type ComputedRef, type Ref } from 'vue'
import {
  getAuthStorageUserScope,
  subscribeAuthStorageScopeChange,
} from '../../../shared/utils/auth'
import { listCanvasBoards, subscribeCanvasBoardChanges, type CanvasBoardMeta } from './canvasBoards'
import {
  createCanvasBoardCommands,
  type CanvasBoardCommands,
  type CanvasBoardRenameRemoteResult,
  type CanvasBoardRollbackRenameResult,
  type CanvasBoardScopeChangedResult,
} from './canvasBoardCommands'
import {
  hydrateCanvasBoardsFromServer,
  type HydrateCanvasBoardsFromServerOptions,
  type HydrateCanvasBoardsFromServerResult,
} from './boardSync'

type CanvasBoardCatalogMutations = Pick<
  CanvasBoardCommands,
  'create' | 'renameOptimistic' | 'rollbackOptimisticRename' | 'duplicate' | 'delete'
>

export type CanvasBoardCatalogRenameResult =
  | { ok: true; board: CanvasBoardMeta }
  | { ok: false; reason: 'local-name-conflict' | 'invalid-name'; error?: undefined }
  | {
      ok: false
      reason: 'remote-name-conflict' | 'sync-failed'
      boardId: string
      previousBoard: CanvasBoardMeta
      error: unknown
      rolledBackBoard?: CanvasBoardMeta
      rollback: CanvasBoardRollbackRenameResult
      hydrate?: HydrateCanvasBoardsFromServerResult
    }
  | CanvasBoardScopeChangedResult

export type CanvasBoardCatalogViewModel = {
  scope: Readonly<Ref<string | null>>
  boards: Readonly<Ref<readonly CanvasBoardMeta[]>>
  byId: ComputedRef<ReadonlyMap<string, CanvasBoardMeta>>
}

export type CanvasBoardCatalogCommands = {
  hydrate(
    options?: HydrateCanvasBoardsFromServerOptions,
  ): Promise<HydrateCanvasBoardsFromServerResult>
  create: CanvasBoardCommands['create']
  rename(input: { boardId: string; nextName: string }): Promise<CanvasBoardCatalogRenameResult>
  duplicate: CanvasBoardCommands['duplicate']
  delete: CanvasBoardCommands['delete']
}

export type CanvasBoardCatalog = {
  viewModel: CanvasBoardCatalogViewModel
  commands: CanvasBoardCatalogCommands
  dispose(): void
}

export type CanvasBoardCatalogDependencies = {
  listBoards(): CanvasBoardMeta[]
  subscribe(listener: () => void): () => void
  getScope(): string | null
  subscribeScope(listener: (scope: string | null) => void): () => void
  createCommands(scopeGuard?: { isCurrent: () => boolean }): CanvasBoardCatalogMutations
  hydrate(
    options?: HydrateCanvasBoardsFromServerOptions,
  ): Promise<HydrateCanvasBoardsFromServerResult>
}

const defaultDependencies: CanvasBoardCatalogDependencies = {
  listBoards: listCanvasBoards,
  subscribe: subscribeCanvasBoardChanges,
  getScope: getAuthStorageUserScope,
  subscribeScope: subscribeAuthStorageScopeChange,
  createCommands: (scopeGuard) =>
    createCanvasBoardCommands({ isMutationCurrent: scopeGuard?.isCurrent ?? (() => true) }),
  hydrate: hydrateCanvasBoardsFromServer,
}

function cloneBoards(boards: CanvasBoardMeta[]): CanvasBoardMeta[] {
  return boards.map((board) => ({ ...board }))
}

function toRenameFailure(
  remote: Exclude<CanvasBoardRenameRemoteResult, { ok: true }>,
  rollback: CanvasBoardRollbackRenameResult,
  hydrate?: HydrateCanvasBoardsFromServerResult,
): CanvasBoardCatalogRenameResult {
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

export function createCanvasBoardCatalog(
  dependencyOverrides: Partial<CanvasBoardCatalogDependencies> = {},
): CanvasBoardCatalog {
  const deps = { ...defaultDependencies, ...dependencyOverrides }
  const scope = shallowRef(deps.getScope())
  const boards = shallowRef<CanvasBoardMeta[]>(scope.value ? cloneBoards(deps.listBoards()) : [])
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

  function scopeChangedResult(): CanvasBoardScopeChangedResult {
    return { ok: false, reason: 'scope-changed' }
  }

  const unsubscribe = deps.subscribe(refresh)
  const unsubscribeScope = deps.subscribeScope((nextScope) => {
    if (scope.value === nextScope) return
    scopeGeneration += 1
    scope.value = nextScope
    refresh()
  })

  const commands: CanvasBoardCatalogCommands = {
    hydrate(options = {}) {
      const token = captureScope()
      return enqueue(async () => {
        if (!isScopeCurrent(token)) {
          return { ok: false, error: new CanvasBoardCatalogScopeChangedError() }
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

export class CanvasBoardCatalogScopeChangedError extends Error {
  constructor() {
    super('CANVAS_BOARD_CATALOG_SCOPE_CHANGED')
    this.name = 'CanvasBoardCatalogScopeChangedError'
  }
}

const canvasBoardCatalog = createCanvasBoardCatalog()

export function useCanvasBoardCatalog(): CanvasBoardCatalog {
  return canvasBoardCatalog
}
