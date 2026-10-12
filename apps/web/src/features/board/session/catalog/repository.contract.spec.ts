import { computed, nextTick, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { createBoardRepository, type BoardRepositoryDependencies } from './repository'
import type {
  BoardCommands,
  BoardOptimisticRenameResult,
  BoardRenameRemoteResult,
} from './commands'
import type { BoardMeta } from './boards'

function board(id: string, name: string): BoardMeta {
  return { id, name, createdAt: 100, updatedAt: 200 }
}

function createHarness() {
  const boards = ref<BoardMeta[]>([board('board-1', 'Original')])
  const listeners = new Set<() => void>()
  const scopeListeners = new Set<(scope: string | null) => void>()
  let currentScope: string | null = 'scope-a'
  const emit = () => listeners.forEach((listener) => listener())
  const listBoards = vi.fn(() => boards.value.map((item) => ({ ...item })))
  const renameOptimistic = vi.fn<BoardCommands['renameOptimistic']>(
    ({ boardId, nextName }): BoardOptimisticRenameResult => {
      const previousBoard = boards.value.find((item) => item.id === boardId)
      if (!previousBoard) return { isOk: false as const, reason: 'invalid-name' as const }
      const previous = { ...previousBoard }
      previousBoard.name = nextName
      emit()
      return {
        isOk: true,
        board: { ...previousBoard },
        previousBoard: previous,
        confirm: vi.fn<() => Promise<BoardRenameRemoteResult>>(async () => ({
          isOk: true,
          board: { ...previousBoard },
        })),
      }
    },
  )
  const rollbackOptimisticRename = vi.fn(
    ({ boardId, previousName }: { boardId: string; previousName: string }) => {
      const current = boards.value.find((item) => item.id === boardId)
      if (!current) return { isOk: false as const, reason: 'invalid-name' as const }
      current.name = previousName
      emit()
      return { isOk: true as const, board: { ...current } }
    },
  )
  const create = vi.fn(async () => {
    const created = board('board-2', 'Untitled')
    boards.value = [created, ...boards.value]
    emit()
    return { isOk: true as const, board: created }
  })
  const duplicate = vi.fn(async ({ sourceBoardId }: { sourceBoardId: string }) => {
    const source = boards.value.find((item) => item.id === sourceBoardId)
    if (!source) return { isOk: false as const, reason: 'source-missing' as const }
    const copy = board('board-copy', `${source.name} Copy`)
    boards.value = [copy, ...boards.value]
    emit()
    return { isOk: true as const, board: copy }
  })
  const deleteBoard = vi.fn(async ({ boardId }: { boardId: string }) => {
    boards.value = boards.value.filter((item) => item.id !== boardId)
    emit()
    return { isOk: true as const, isRemoteWasMissing: false }
  })
  const hydrate = vi.fn(async () => {
    emit()
    return { isOk: true as const }
  })
  const dependencies: BoardRepositoryDependencies = {
    listBoards,
    subscribe: (listener) => {
      listeners.add(listener)
      return () => listeners.delete(listener)
    },
    getScope: () => currentScope,
    subscribeScope: (listener) => {
      scopeListeners.add(listener)
      return () => scopeListeners.delete(listener)
    },
    createCommands: () => ({
      create,
      renameOptimistic,
      rollbackOptimisticRename,
      duplicate,
      delete: deleteBoard,
    }),
    hydrate,
  }
  return {
    boards,
    emit,
    dependencies,
    listBoards,
    hydrate,
    renameOptimistic,
    rollbackOptimisticRename,
    changeScope(nextScope: string | null) {
      currentScope = nextScope
      scopeListeners.forEach((listener) => listener(nextScope))
    },
  }
}

describe('BoardRepository contract', () => {
  it('keeps Boards and Canvas consumers synchronized from one reactive view model', async () => {
    const harness = createHarness()
    const catalog = createBoardRepository(harness.dependencies)
    const boardsConsumer = computed(() => catalog.viewModel.boards.value.map((item) => item.name))
    const canvasConsumer = computed(() => catalog.viewModel.byId.value.get('board-1')?.name)

    const result = await catalog.commands.rename({ boardId: 'board-1', nextName: 'Renamed' })
    await nextTick()

    expect(result).toMatchObject({ isOk: true, board: { name: 'Renamed' } })
    expect(boardsConsumer.value).toEqual(['Renamed'])
    expect(canvasConsumer.value).toBe('Renamed')
  })

  it('updates both consumers after local change notifications and remote hydrate', async () => {
    const harness = createHarness()
    const catalog = createBoardRepository(harness.dependencies)
    const boardsConsumer = computed(() => catalog.viewModel.boards.value.map((item) => item.name))
    const canvasConsumer = computed(() => catalog.viewModel.byId.value.get('board-1')?.name)

    harness.boards.value[0]!.name = 'Saved locally'
    harness.emit()
    await nextTick()
    expect(boardsConsumer.value).toEqual(['Saved locally'])
    expect(canvasConsumer.value).toBe('Saved locally')

    harness.boards.value = [board('board-1', 'Remote latest'), board('board-2', 'Shared board')]
    await catalog.commands.hydrate()
    await nextTick()
    expect(boardsConsumer.value).toEqual(['Remote latest', 'Shared board'])
    expect(canvasConsumer.value).toBe('Remote latest')
  })

  it('keeps both consumers synchronized after create duplicate and delete', async () => {
    const harness = createHarness()
    const catalog = createBoardRepository(harness.dependencies)
    const boardsConsumer = computed(() => catalog.viewModel.boards.value.map((item) => item.name))
    const canvasConsumer = computed(() =>
      [...catalog.viewModel.byId.value.values()].map((item) => item.name),
    )

    await catalog.commands.create()
    expect(boardsConsumer.value).toEqual(['Untitled', 'Original'])
    expect(canvasConsumer.value).toEqual(['Untitled', 'Original'])

    await catalog.commands.duplicate({ sourceBoardId: 'board-1' })
    expect(boardsConsumer.value).toEqual(['Original Copy', 'Untitled', 'Original'])
    expect(canvasConsumer.value).toEqual(['Original Copy', 'Untitled', 'Original'])

    await catalog.commands.delete({ boardId: 'board-1' })
    expect(boardsConsumer.value).toEqual(['Original Copy', 'Untitled'])
    expect(canvasConsumer.value).toEqual(['Original Copy', 'Untitled'])
  })

  it('uses storage change notification as the single invalidation path for mutations', async () => {
    const harness = createHarness()
    const catalog = createBoardRepository(harness.dependencies)
    harness.listBoards.mockClear()

    await catalog.commands.create()

    expect(harness.listBoards).toHaveBeenCalledOnce()
  })

  it('invalidates in-flight and queued mutations when auth scope changes', async () => {
    const harness = createHarness()
    let releaseCreate!: () => void
    const createStarted = vi.fn()
    const duplicate = vi.fn(async () => ({
      isOk: true as const,
      board: board('board-copy', 'Copy'),
    }))
    harness.dependencies.createCommands = ({ isCurrent } = { isCurrent: () => true }) => ({
      create: vi.fn(async () => {
        createStarted()
        await new Promise<void>((resolve) => {
          releaseCreate = resolve
        })
        return isCurrent()
          ? { isOk: true as const, board: board('board-2', 'Untitled') }
          : { isOk: false as const, reason: 'scope-changed' as const }
      }),
      renameOptimistic: harness.renameOptimistic,
      rollbackOptimisticRename: harness.rollbackOptimisticRename,
      duplicate,
      delete: vi.fn(async () => ({ isOk: true as const, isRemoteWasMissing: false })),
    })
    const catalog = createBoardRepository(harness.dependencies)

    const inFlight = catalog.commands.create()
    await vi.waitFor(() => expect(createStarted).toHaveBeenCalledOnce())
    const queued = catalog.commands.duplicate({ sourceBoardId: 'board-1' })
    harness.changeScope('scope-b')
    releaseCreate()

    await expect(inFlight).resolves.toEqual({ isOk: false, reason: 'scope-changed' })
    await expect(queued).resolves.toEqual({ isOk: false, reason: 'scope-changed' })
    expect(duplicate).not.toHaveBeenCalled()
    expect(catalog.viewModel.scope.value).toBe('scope-b')
  })

  it('rolls back an optimistic rename when remote confirmation fails and returns the typed failure', async () => {
    const harness = createHarness()
    const catalog = createBoardRepository(harness.dependencies)
    harness.renameOptimistic.mockImplementationOnce(
      ({ boardId, nextName }): BoardOptimisticRenameResult => {
        const current = harness.boards.value.find((item) => item.id === boardId)!
        const previousBoard = { ...current }
        current.name = nextName
        harness.emit()
        return {
          isOk: true as const,
          board: { ...current },
          previousBoard,
          confirm: vi.fn<() => Promise<BoardRenameRemoteResult>>(async () => ({
            isOk: false as const,
            reason: 'sync-failed' as const,
            boardId,
            board: { ...current },
            previousBoard,
            error: new Error('offline'),
          })),
        }
      },
    )

    const pending = catalog.commands.rename({ boardId: 'board-1', nextName: 'Unconfirmed' })
    await nextTick()
    expect(catalog.viewModel.byId.value.get('board-1')?.name).toBe('Unconfirmed')

    await expect(pending).resolves.toMatchObject({
      isOk: false,
      reason: 'sync-failed',
      boardId: 'board-1',
      previousBoard: { name: 'Original' },
      rolledBackBoard: { name: 'Original' },
    })
    expect(harness.rollbackOptimisticRename).toHaveBeenCalledWith({
      boardId: 'board-1',
      previousName: 'Original',
    })
    expect(catalog.viewModel.byId.value.get('board-1')?.name).toBe('Original')
  })

  it('hydrates the remote winner after a revision conflict and exposes typed rollback and hydrate results', async () => {
    const harness = createHarness()
    const catalog = createBoardRepository(harness.dependencies)
    const conflict = { status: 409, code: 'board_revision_conflict', msg: '看板版本已变更' }
    harness.renameOptimistic.mockImplementationOnce(
      ({ boardId, nextName }): BoardOptimisticRenameResult => {
        const current = harness.boards.value.find((item) => item.id === boardId)!
        const previousBoard = { ...current }
        current.name = nextName
        harness.emit()
        return {
          isOk: true,
          board: { ...current },
          previousBoard,
          confirm: vi.fn(async () => ({
            isOk: false as const,
            reason: 'revision-conflict' as const,
            boardId,
            board: { ...current },
            previousBoard,
            error: conflict,
          })),
        }
      },
    )
    harness.hydrate.mockImplementationOnce(async () => {
      harness.boards.value = [board('board-1', 'Remote Winner')]
      harness.emit()
      return { isOk: true as const }
    })

    const result = await catalog.commands.rename({ boardId: 'board-1', nextName: 'Taken' })

    expect(result).toMatchObject({
      isOk: false,
      reason: 'revision-conflict',
      rollback: { isOk: true, board: { name: 'Original' } },
      hydrate: { isOk: true },
    })
    expect(harness.hydrate).toHaveBeenCalledWith({
      shouldPreserveLocalNewer: false,
      rollbackBoardIds: ['board-1'],
    })
    expect(catalog.viewModel.byId.value.get('board-1')?.name).toBe('Remote Winner')
  })

  it('hydrates after an idempotency conflict with the same rollback path', async () => {
    const harness = createHarness()
    const catalog = createBoardRepository(harness.dependencies)
    const conflict = { status: 409, code: 'board_idempotency_conflict', msg: '幂等冲突' }
    harness.renameOptimistic.mockImplementationOnce(
      ({ boardId, nextName }): BoardOptimisticRenameResult => {
        const current = harness.boards.value.find((item) => item.id === boardId)!
        const previousBoard = { ...current }
        current.name = nextName
        harness.emit()
        return {
          isOk: true,
          board: { ...current },
          previousBoard,
          confirm: vi.fn(async () => ({
            isOk: false as const,
            reason: 'idempotency-conflict' as const,
            boardId,
            board: { ...current },
            previousBoard,
            error: conflict,
          })),
        }
      },
    )

    const result = await catalog.commands.rename({ boardId: 'board-1', nextName: 'Taken' })

    expect(result).toMatchObject({
      isOk: false,
      reason: 'idempotency-conflict',
      rollback: { isOk: true, board: { name: 'Original' } },
    })
    expect(harness.hydrate).toHaveBeenCalledWith({
      shouldPreserveLocalNewer: false,
      rollbackBoardIds: ['board-1'],
    })
  })
})
