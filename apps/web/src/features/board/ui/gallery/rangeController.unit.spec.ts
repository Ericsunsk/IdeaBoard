import { computed, nextTick, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import type { BoardRepository } from '../../session/catalog/repository'
import type { BoardAccessCapabilities, BoardMeta } from '../../session/catalog/boards'
import { makeBoardMeta, primeAuthenticatedWorkspace } from '../../testing'
import { getWorkspaceSelectionStore } from '../../../workspace'
import type { CatalogNavigation } from './contract'
import type { BoardFeedback } from './useBoardFeedback'
import { createGalleryController } from './controller'
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

function createHarness(initialBoards: BoardMeta[] = rangeBoards()) {
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
      create: vi.fn(async () => ({ isOk: true as const, board: makeBoardMeta('created') })),
      rename: vi.fn(async ({ boardId, nextName }) => ({
        isOk: true as const,
        board: makeBoardMeta(boardId, nextName),
      })),
      duplicate: vi.fn(async () => ({ isOk: true as const, board: makeBoardMeta('copied') })),
      delete: vi.fn(async () => ({ isOk: true as const, isRemoteWasMissing: false })),
    },
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
  const controller = createGalleryController({
    catalog,
    scenes: { sceneMap: ref({}), workspaceId: ref(null) },
    navigation,
    feedback,
    warn: vi.fn(),
  })
  disposeControllers.push(controller.dispose)
  return { boards, scope, catalog, navigation, controller }
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
