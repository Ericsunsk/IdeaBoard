import { computed, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { WorkspaceBoardMeta } from '../../session/adapters/boardsResource'
import { makeBoardMeta as makeBoard } from '../../testing'

describe('Boards Catalog public interface', () => {
  afterEach(() => {
    vi.doUnmock('../../session/catalog/repository')
    vi.doUnmock('./useBoardFeedback')
    vi.resetModules()
  })

  it('owns the complete gallery workflow behind one page-facing contract', async () => {
    const boards = ref([makeBoard('board-1', 'Original', 10)])
    const scope = ref<string | null>('user-1')
    const hydrate = vi.fn().mockResolvedValue({ isOk: true as const })
    const create = vi.fn().mockResolvedValue({
      isOk: true as const,
      board: makeBoard('created-board', 'Created', 20),
    })
    const duplicate = vi.fn().mockResolvedValue({
      isOk: true as const,
      board: makeBoard('copied-board', 'Original Copy', 30),
    })
    const rename = vi
      .fn()
      .mockResolvedValueOnce({
        isOk: true as const,
        board: makeBoard('board-1', 'Renamed', 40),
      })
      .mockResolvedValueOnce({
        isOk: false as const,
        reason: 'revision-conflict' as const,
        boardId: 'board-1',
        previousBoard: makeBoard('board-1', 'Renamed', 40),
        error: { status: 409, code: 'board_revision_conflict' },
        rollback: { isOk: true as const, board: makeBoard('board-1', 'Renamed', 40) },
        hydrate: { isOk: false as const, error: new Error('recovery failed') },
      })
    const deleteBoard = vi.fn().mockResolvedValue({
      isOk: true as const,
      isRemoteWasMissing: false,
    })
    const feedback = {
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
    const navigation = { openBoard: vi.fn() }

    vi.doMock('../../session/catalog/repository', () => ({
      useBoardRepository: () => ({
        viewModel: {
          boards,
          byId: computed(() => new Map(boards.value.map((board) => [board.id, board]))),
          scope,
        },
        commands: { create, delete: deleteBoard, duplicate, hydrate, rename },
        dispose: vi.fn(),
      }),
    }))
    vi.doMock('./useBoardFeedback', () => ({ useBoardFeedback: () => feedback }))

    const { createCatalog } = await import('./useCatalog')
    const catalog = createCatalog({ navigation })

    await catalog.commands.initialize()
    expect(hydrate).toHaveBeenCalledOnce()

    catalog.commands.setSearchKeyword('original')
    expect(catalog.viewModel.visibleBoards.value.map((board) => board.id)).toEqual(['board-1'])

    await catalog.commands.createAndOpenBoard()
    expect(navigation.openBoard).toHaveBeenCalledWith('created-board')

    await catalog.commands.duplicateBoard('board-1')
    expect(duplicate).toHaveBeenCalledWith({ sourceBoardId: 'board-1' })
    expect(feedback.copied).toHaveBeenCalledOnce()

    const board = catalog.viewModel.visibleBoards.value[0]!
    catalog.commands.startRename(board)
    catalog.commands.updateRenameDraft('Renamed')
    await catalog.commands.commitRename()
    expect(rename).toHaveBeenCalledWith({ boardId: 'board-1', nextName: 'Renamed' })
    expect(feedback.renamed).toHaveBeenCalledOnce()

    catalog.commands.startRename(board)
    catalog.commands.updateRenameDraft('Conflicting')
    await catalog.commands.commitRename()
    expect(catalog.viewModel.catalogRecoveryBoardId.value).toBe('board-1')
    await catalog.commands.reloadCatalog()
    expect(catalog.viewModel.catalogRecoveryBoardId.value).toBeNull()
    expect(feedback.catalogReloaded).toHaveBeenCalledOnce()

    catalog.commands.requestDelete(board)
    await catalog.commands.confirmDelete()
    expect(deleteBoard).toHaveBeenCalledWith({ boardId: 'board-1' })
    expect(feedback.deleted).toHaveBeenCalledOnce()

    catalog.dispose()
  })
})

async function sceneCatalogFixture(): Promise<WorkspaceBoardMeta> {
  const { parsePreviewScene } = await import('../../core/preview/scene')
  const fixture =
    await import('../../../../../../api/internal/boards/previewscene/testdata/mixed.json')
  const scene = parsePreviewScene(fixture.scene)!
  return {
    id: 'board-1',
    name: 'Board',
    revision: 1,
    schemaVersion: 4,
    createdAt: 1,
    updatedAt: 1,
    contentVersion: 'cv-1',
    previewScene: scene,
    capabilities: { read: true, write: true, manage: true, generate: true },
    accessState: 'private' as const,
    workspaceAccess: 'none' as const,
    isCreator: true,
    hasDirectGrant: false,
  }
}

describe('Catalog scene reads', () => {
  it('reacts to Query-only scene changes, preserves identities, and isolates scope', async () => {
    const { shallowRef, nextTick } = await import('vue')
    const { queryClient } = await import('../../../../core/query/queryClient')
    const { seedBoardListQuery } = await import('../../session/sync/boardQueries')
    const { primeAuthenticatedWorkspace } = await import('../../testing')
    const { getBoardScope, boardScopeKey, isCompleteBoardScope, subscribeBoardScopeChange } =
      await import('../../session/catalog/scope')
    const { getWorkspaceSelectionStore } = await import('../../../workspace')
    const { setStoredAuth } = await import('../../../../core/utils/auth')
    const meta = await sceneCatalogFixture()
    const scene = meta.previewScene!
    primeAuthenticatedWorkspace('ws_first')
    const boards = shallowRef([makeBoard('board-1'), makeBoard('offline')])
    const captured = getBoardScope()
    const scope = ref<string | null>(
      isCompleteBoardScope(captured) ? boardScopeKey(captured) : null,
    )
    const stopScope = subscribeBoardScopeChange((next) => {
      scope.value = isCompleteBoardScope(next) ? boardScopeKey(next) : null
    })
    seedBoardListQuery('ws_first', [meta, { ...meta, id: 'invisible' }])
    vi.doMock('../../session/catalog/repository', () => ({
      useBoardRepository: () => ({
        viewModel: { boards, scope, byId: computed(() => new Map()) },
        commands: { hydrate: vi.fn(async () => ({ isOk: true })) },
      }),
    }))
    vi.doMock('./useBoardFeedback', () => ({ useBoardFeedback: () => ({}) }))
    const read = vi.spyOn(queryClient, 'fetchQuery')
    const { createCatalog } = await import('./useCatalog')
    const catalog = createCatalog({ navigation: { openBoard: vi.fn() } })
    try {
      expect(catalog.viewModel.workspaceId.value).toBe('ws_first')
      const first = catalog.viewModel.sceneMap.value['board-1']
      expect(first?.scene).toEqual(scene)
      expect(catalog.viewModel.sceneMap.value['offline']).toEqual({
        contentVersion: '',
        scene: null,
      })
      expect(catalog.viewModel.sceneMap.value['invisible']).toBeUndefined()
      seedBoardListQuery('ws_first', [{ ...meta, updatedAt: 2, previewScene: { ...scene } }])
      await nextTick()
      expect(catalog.viewModel.sceneMap.value['board-1']).toBe(first)
      seedBoardListQuery('ws_first', [
        { ...meta, contentVersion: 'cv-2', previewScene: { ...scene, contentVersion: 'cv-2' } },
      ])
      await nextTick()
      expect(catalog.viewModel.sceneMap.value['board-1']?.contentVersion).toBe('cv-2')
      expect(catalog.viewModel.sceneMap.value['board-1']).not.toBe(first)
      seedBoardListQuery('ws_first', [{ ...meta, contentVersion: 'cv-2', previewScene: null }])
      await nextTick()
      expect(catalog.viewModel.sceneMap.value['board-1']?.scene).toBeNull()
      seedBoardListQuery('ws_first', [
        { ...meta, contentVersion: 'cv-2', previewScene: { ...scene, contentVersion: 'cv-2' } },
      ])
      await nextTick()
      expect(catalog.viewModel.sceneMap.value['board-1']?.scene).not.toBeNull()
      expect(boards.value).toHaveLength(2)
      expect(read).not.toHaveBeenCalled()
      getWorkspaceSelectionStore().select('ws_second')
      expect(catalog.viewModel.workspaceId.value).toBe('ws_second')
      expect(catalog.viewModel.sceneMap.value['board-1']?.scene).toBeNull()
      seedBoardListQuery('ws_first', [{ ...meta, contentVersion: 'late' }])
      await nextTick()
      expect(catalog.viewModel.sceneMap.value['board-1']?.scene).toBeNull()
      seedBoardListQuery('ws_second', [meta])
      await nextTick()
      expect(catalog.viewModel.sceneMap.value['board-1']).not.toBe(first)
      expect(catalog.viewModel.sceneMap.value['board-1']?.scene).toEqual(scene)
      const second = catalog.viewModel.sceneMap.value['board-1']
      setStoredAuth({
        public_id: '00000000-0000-0000-0000-000000000002',
        display_name: 'other',
        avatar: '',
        role: 'user',
      })
      expect(catalog.viewModel.sceneMap.value['board-1']).not.toBe(second)
      expect(catalog.viewModel.sceneMap.value['board-1']?.scene).toEqual(scene)
      getWorkspaceSelectionStore().clear()
      expect(catalog.viewModel.workspaceId.value).toBeNull()
      expect(catalog.viewModel.sceneMap.value).toEqual({})
    } finally {
      catalog.dispose()
      expect(
        queryClient
          .getQueryCache()
          .find({ queryKey: ['boards', 'ws_second', 'list'] })
          ?.getObserversCount(),
      ).toBe(0)
      stopScope()
      queryClient.clear()
      read.mockRestore()
      vi.doUnmock('../../session/catalog/repository')
      vi.doUnmock('./useBoardFeedback')
      vi.resetModules()
    }
  })
})
