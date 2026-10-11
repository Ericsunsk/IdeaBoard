import { computed, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

function makeBoard(id: string, name = id, updatedAt = 1) {
  return { id, name, createdAt: 1, updatedAt }
}

describe('Boards Catalog public interface', () => {
  afterEach(() => {
    vi.doUnmock('../../session/catalog/repository')
    vi.doUnmock('../../session/preview')
    vi.doUnmock('./feedback/useFeedback')
    vi.resetModules()
  })

  it('owns the complete gallery workflow behind one page-facing contract', async () => {
    const boards = ref([makeBoard('board-1', 'Original', 10)])
    const scope = ref<string | null>('user-1')
    const hydrate = vi.fn().mockResolvedValue({ ok: true as const })
    const create = vi.fn().mockResolvedValue({
      ok: true as const,
      board: makeBoard('created-board', 'Created', 20),
    })
    const duplicate = vi.fn().mockResolvedValue({
      ok: true as const,
      board: makeBoard('copied-board', 'Original Copy', 30),
    })
    const rename = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true as const,
        board: makeBoard('board-1', 'Renamed', 40),
      })
      .mockResolvedValueOnce({
        ok: false as const,
        reason: 'revision-conflict' as const,
        boardId: 'board-1',
        previousBoard: makeBoard('board-1', 'Renamed', 40),
        error: { status: 409, code: 'board_revision_conflict' },
        rollback: { ok: true as const, board: makeBoard('board-1', 'Renamed', 40) },
        hydrate: { ok: false as const, error: new Error('recovery failed') },
      })
    const deleteBoard = vi.fn().mockResolvedValue({
      ok: true as const,
      remoteWasMissing: false,
    })
    const preview = {
      previewMap: ref<Record<string, string>>({}),
      reloadFromLocal: vi.fn(),
      schedule: vi.fn(),
      invalidate: vi.fn(),
      dispose: vi.fn(),
    }
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
    vi.doMock('../../session/preview', () => ({
      createBoardPreviewRefreshQueue: () => preview,
    }))
    vi.doMock('./useBoardFeedback', () => ({ useBoardFeedback: () => feedback }))

    const { createCatalog } = await import('./useCatalog')
    const catalog = createCatalog({ navigation })

    await catalog.commands.initialize()
    expect(hydrate).toHaveBeenCalledOnce()
    expect(preview.reloadFromLocal).toHaveBeenCalledWith(boards.value)

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
    expect(preview.dispose).toHaveBeenCalledOnce()
  })
})
