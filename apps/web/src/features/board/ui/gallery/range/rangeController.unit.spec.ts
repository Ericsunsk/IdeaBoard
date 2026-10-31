import { computed, nextTick, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import { createBoardRepository, type BoardRepository } from '../../../session/catalog/repository'
import type { BoardAccessCapabilities, BoardMeta } from '../../../session/catalog/boards'
import { makeBoardMeta as makeBoard, makeBoardMeta, primeAuthenticatedWorkspace } from '../../../testing'
import { createDeferred } from '../../../../../test/deferred'
import { getWorkspaceSelectionStore } from '../../../../workspace'
import type { CatalogNavigation } from '../styling/contract'
import type { BoardFeedback } from '../controllers/useBoardFeedback'
import { createGalleryController } from '../controllers/galleryController'
import { readPersistedGalleryRange } from './range'

const disposeControllers: Array<() => void> = []

const ownerCapabilities: BoardAccessCapabilities = {
  read: true,
  write: true,
  manage: true,
  generate: true,
}
const viewerCapabilities: BoardAccessCapabilities = {
  read: true,
  write: false,
  manage: false,
  generate: false,
}

function rangeBoards(): BoardMeta[] {
  return [
    makeBoardMeta('own', 'Own', 30, {
      revision: 1,
      accessState: 'private',
      workspaceAccess: 'none',
      isCreator: true,
      hasDirectGrant: false,
      capabilities: ownerCapabilities,
    }),
    makeBoardMeta('selected', 'Selected', 20, {
      revision: 1,
      accessState: 'workspace',
      workspaceAccess: 'viewer',
      isCreator: false,
      hasDirectGrant: true,
      capabilities: viewerCapabilities,
    }),
    makeBoardMeta('team', 'Team', 10, {
      revision: 1,
      accessState: 'workspace',
      workspaceAccess: 'viewer',
      isCreator: false,
      hasDirectGrant: true,
      capabilities: viewerCapabilities,
    }),
    makeBoardMeta('both', 'Both', 40, {
      revision: 1,
      accessState: 'workspace',
      workspaceAccess: 'editor',
      isCreator: true,
      hasDirectGrant: false,
      capabilities: ownerCapabilities,
    }),
    makeBoardMeta('draft', 'Draft', 50),
  ]
}

function createHarness(initialBoards: BoardMeta[] = rangeBoards(), repository?: BoardRepository) {
  const boards = ref<readonly BoardMeta[]>(initialBoards)
  const scope = ref<string | null>('user-1')
  const catalog: BoardRepository = repository ?? {
    viewModel: {
      scope,
      boards,
      byId: computed(() => new Map(boards.value.map((board) => [board.id, board]))),
    },
    commands: {
      hydrate: vi.fn(async () => ({ isOk: true as const })),
      create: vi.fn(async () => ({ isOk: true as const, board: makeBoardMeta('created') })),
      rename: vi.fn(async ({ boardId, nextName }) => ({
        isOk: true as const,
        board: makeBoardMeta(boardId, nextName),
      })),
      duplicate: vi.fn(async () => ({ isOk: true as const, board: makeBoardMeta('copied') })),
      delete: vi.fn(async () => ({ isOk: true as const, isRemoteWasMissing: false })),
    },
    observeDocument: vi.fn(() => vi.fn()),
    dispose: vi.fn(),
  }
  const navigation: CatalogNavigation = { openBoard: vi.fn() }
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
  const warnMock = vi.fn()
  const controller = createGalleryController({
    catalog,
    scenes: { sceneMap: ref({}), workspaceId: ref(null) },
    navigation,
    feedback,
    warn: warnMock,
  })
  disposeControllers.push(controller.dispose)
  return { boards, scope, catalog, navigation, feedback, warn: warnMock, controller }
}

beforeEach(() => {
  localStorage.clear()
  primeAuthenticatedWorkspace('ws_first')
})

afterEach(() => {
  disposeControllers.splice(0).forEach((dispose) => dispose())
  vi.restoreAllMocks()
})

describe('gallery range controller', () => {
  test('defaults to personal and derives search and sort within each range', () => {
    const harness = createHarness()

    expect(harness.controller.viewModel.range.value).toBe('personal')
    // Ranges overlap and count independently (non-exclusive): personal is
    // ownership (own + creator-shared both + unsynced draft), team is
    // workspace-shared (selected + team + creator-shared both).
    expect(harness.controller.viewModel.visibleBoards.value.map((board) => board.id)).toEqual([
      'draft',
      'both',
      'own',
    ])
    expect(harness.controller.viewModel.rangeCounts.value).toEqual({ personal: 3, team: 3 })

    harness.controller.commands.setRange('team')
    expect(harness.controller.viewModel.visibleBoards.value.map((board) => board.id)).toEqual([
      'both',
      'selected',
      'team',
    ])

    harness.controller.commands.setSearchKeyword('selected')
    expect(harness.controller.viewModel.visibleBoards.value.map((board) => board.id)).toEqual([
      'selected',
    ])

    harness.controller.commands.setSearchKeyword('')
    harness.controller.commands.setSortMode('name')
    expect(harness.controller.viewModel.visibleBoards.value.map((board) => board.id)).toEqual([
      'both',
      'selected',
      'team',
    ])
  })

  test('switching ranges never hydrates and keeps the draft overlay intact', () => {
    const harness = createHarness()

    harness.controller.commands.setRange('team')
    harness.controller.commands.setRange('personal')
    harness.controller.commands.setRange('team')

    expect(harness.catalog.commands.hydrate).not.toHaveBeenCalled()
    expect(harness.boards.value.map((board) => board.id)).toEqual([
      'own',
      'selected',
      'team',
      'both',
      'draft',
    ])
    expect(harness.controller.viewModel.visibleBoards.value.map((board) => board.id)).toEqual([
      'both',
      'selected',
      'team',
    ])
  })

  test('persists per User + Workspace and reloads the saved range on scope switches', async () => {
    const harness = createHarness()

    harness.controller.commands.setRange('team')
    expect(readPersistedGalleryRange()).toBe('team')

    getWorkspaceSelectionStore().select('ws_second')
    harness.scope.value = 'ws_second'
    await nextTick()
    expect(harness.controller.viewModel.range.value).toBe('personal')

    getWorkspaceSelectionStore().select('ws_first')
    harness.scope.value = 'ws_first'
    await nextTick()
    expect(harness.controller.viewModel.range.value).toBe('team')
  })

  test('returns to personal after a successful creation without sending the range', async () => {
    const harness = createHarness()
    harness.controller.commands.setRange('team')

    await harness.controller.commands.createAndOpenBoard()

    expect(harness.catalog.commands.create).toHaveBeenCalledTimes(1)
    // The creation command carries no range or ACL fact: the server fixes
    // the none default whatever range the gallery showed.
    expect(harness.catalog.commands.create).toHaveBeenCalledWith()
    expect(harness.navigation.openBoard).toHaveBeenCalledWith('created')
    expect(harness.controller.viewModel.range.value).toBe('personal')
    expect(readPersistedGalleryRange()).toBe('personal')
  })

  test('keeps the current range when creation fails', async () => {
    const harness = createHarness()
    harness.controller.commands.setRange('team')
    vi.mocked(harness.catalog.commands.create).mockResolvedValueOnce({
      isOk: false,
      reason: 'sync-failed',
      board: makeBoardMeta('failed'),
      error: new Error('offline'),
    })

    await harness.controller.commands.createAndOpenBoard()

    expect(harness.navigation.openBoard).not.toHaveBeenCalled()
    expect(harness.controller.viewModel.range.value).toBe('team')
  })
})

// Ported from controller.unit.spec.ts L348-491 (deleted): focus throttling +
// hydrate race contract. The range controller shares createGalleryController,
// so the refresh guards live here after the consolidation.
describe('gallery focus/hydrate races (ported)', () => {
  test('refreshes on window focus only when scope is stable, throttled, and reuses initialize', async () => {
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

  test('skips visible reloads while the tab stays hidden', async () => {
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

  test('auto-hydrates once when the scope becomes ready after a failure', async () => {
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

  test('auto-hydrates on workspace switches without manual retry', async () => {
    const harness = createHarness()
    await harness.controller.commands.initialize()
    expect(harness.catalog.commands.hydrate).toHaveBeenCalledTimes(1)

    harness.scope.value = 'user-2'
    await nextTick()
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(harness.catalog.commands.hydrate).toHaveBeenCalledTimes(2)

    harness.controller.dispose()
  })

  test.each(['failure', 'rejection'] as const)(
    'ignores old workspace %s after focus refresh while the repository queues the new workspace hydrate',
    async (completion) => {
      const oldRefresh = createDeferred<Awaited<ReturnType<BoardRepository['commands']['hydrate']>>>()
      const newRefresh = createDeferred<Awaited<ReturnType<BoardRepository['commands']['hydrate']>>>()
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
})
