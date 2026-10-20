import { computed, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { setUserLocale } from '../../../../core/i18n'
import { renderWithUser, screen, waitFor, within } from '../../../../test/render'
import { createDeferred } from '../../../../test/deferred'
import { makeBoardMeta } from '../../testing'
import type { BoardMeta } from '../../session/catalog/boards'
import type { CatalogSortMode } from './contract'
import { createGalleryController } from './controller'
import BoardsPage from './BoardsPage.vue'

const { useCatalog } = vi.hoisted(() => ({ useCatalog: vi.fn() }))

vi.mock('./useCatalog', () => ({ useCatalog, formatCatalogDate: vi.fn() }))
vi.mock('../share/ShareDialog.vue', () => ({ default: { render: () => null } }))
vi.mock('../../../workspace', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../../workspace')>()
  return {
    ...actual,
    WorkspaceHeader: { render: () => null },
    useWorkspaceSelection: vi.fn(),
  }
})
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))

const disposeCatalogs: (() => void)[] = []

function renderCatalog(mode: CatalogSortMode) {
  const catalog = createGalleryController({
    catalog: {
      viewModel: { boards: ref([]), scope: ref(null), byId: computed(() => new Map()) },
      commands: {
        hydrate: vi.fn(),
        create: vi.fn(),
        rename: vi.fn(),
        duplicate: vi.fn(),
        delete: vi.fn(),
      },
      dispose: vi.fn(),
    },
    scenes: {
      sceneMap: ref({}),
      workspaceId: ref(null),
    },
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
    navigation: { openBoard: vi.fn() },
    warn: vi.fn(),
  })
  catalog.commands.setSortMode(mode)
  vi.spyOn(catalog.commands, 'setSortMode')
  disposeCatalogs.push(catalog.dispose)
  useCatalog.mockReturnValue(catalog)
  return { ...renderWithUser(BoardsPage), catalog }
}

// Recovery specs drive the real gallery controller instead of stubbing the
// viewModel: the HydrationFailed/RecoveryBoardId/ReloadPending states only
// exist as controller transitions, so the panel assertions stay coupled to
// the reloadCatalog semantics they guard.
function createRecoveryCatalog(initialBoards: BoardMeta[] = []) {
  const boards = ref<readonly BoardMeta[]>(initialBoards)
  const repository = {
    viewModel: {
      boards,
      scope: ref<string | null>('user-1'),
      byId: computed(() => new Map(boards.value.map((board) => [board.id, board]))),
    },
    commands: {
      hydrate: vi.fn(),
      create: vi.fn(),
      rename: vi.fn(),
      duplicate: vi.fn(),
      delete: vi.fn(),
    },
    dispose: vi.fn(),
  }
  const catalog = createGalleryController({
    catalog: repository,
    scenes: {
      sceneMap: ref({}),
      workspaceId: ref(null),
    },
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
    navigation: { openBoard: vi.fn() },
    warn: vi.fn(),
  })
  disposeCatalogs.push(catalog.dispose)
  useCatalog.mockReturnValue(catalog)
  return { catalog, repository }
}

beforeEach(async () => {
  await setUserLocale('en')
})

afterEach(async () => {
  disposeCatalogs.splice(0).forEach((dispose) => dispose())
  await setUserLocale('en')
})

describe('Boards sorting locale', () => {
  it.each([
    ['recent', 'Recent', '最近'],
    ['name', 'Name', '名称'],
  ] as const)(
    'updates the closed %s trigger when the locale changes',
    async (mode, english, chinese) => {
      const view = renderCatalog(mode)
      const trigger = screen.getByRole('combobox', { name: 'Sort canvases' })
      await waitFor(() => expect(trigger).toHaveTextContent(english))
      expect(trigger).toHaveAttribute('aria-expanded', 'false')

      await expect(setUserLocale('zh-CN')).resolves.toBe(true)

      await waitFor(() => expect(trigger).toHaveAccessibleName('排序画布'))
      await waitFor(() => expect(trigger).toHaveTextContent(chinese))
      expect(trigger).toHaveAttribute('aria-expanded', 'false')
      expect(view.catalog.commands.setSortMode).not.toHaveBeenCalled()

      await expect(setUserLocale('en')).resolves.toBe(true)
      await waitFor(() => expect(trigger).toHaveTextContent(english))
    },
  )
})

describe('Boards catalog recovery', () => {
  it('hides the recovery panel when the catalog hydrates cleanly', () => {
    renderCatalog('recent')

    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('routes hydration failure through LoadErrorPanel with the retry copy', async () => {
    const { catalog, repository } = createRecoveryCatalog()
    vi.mocked(repository.commands.hydrate).mockResolvedValue({
      isOk: false as const,
      error: new Error('offline'),
    })
    vi.spyOn(catalog.commands, 'reloadCatalog')
    const view = renderWithUser(BoardsPage)
    // useCatalog is mocked, so the page never self-hydrates on mount:
    // drive the failing hydration explicitly like the range specs do.
    await catalog.commands.initialize()

    const alert = await screen.findByRole('alert')
    expect(
      within(alert).getByText('Could not load the canvas list. Please try again.'),
    ).toBeVisible()
    await view.user.click(within(alert).getByRole('button', { name: 'Retry' }))
    // The shared retry re-enters the catalog reload, not a local refresh.
    await waitFor(() => expect(repository.commands.hydrate).toHaveBeenCalledTimes(2))
    expect(catalog.commands.reloadCatalog).toHaveBeenCalledOnce()
  })

  it('routes rename-recovery through LoadErrorPanel with the reload copy', async () => {
    const board = makeBoardMeta('board-1', 'Original', 10)
    const { catalog, repository } = createRecoveryCatalog([board])
    const syncError = { status: 409, code: 'board_revision_conflict' }
    vi.mocked(repository.commands.rename).mockResolvedValueOnce({
      isOk: false,
      reason: 'revision-conflict',
      boardId: board.id,
      previousBoard: board,
      error: syncError,
      rollback: { isOk: true, board },
      hydrate: { isOk: false, error: new Error('hydrate failed') },
    })
    vi.mocked(repository.commands.hydrate).mockResolvedValue({ isOk: true })
    catalog.commands.startRename(board)
    catalog.commands.updateRenameDraft('Conflicting')
    await catalog.commands.commitRename()
    expect(catalog.viewModel.catalogRecoveryBoardId.value).toBe(board.id)

    const view = renderWithUser(BoardsPage)
    const alert = await screen.findByRole('alert')
    expect(within(alert).getByText('The canvas list needs to be reloaded')).toBeVisible()
    await view.user.click(within(alert).getByRole('button', { name: 'Reload' }))
    // Rename recovery reloads with the rollback scope for that board.
    await waitFor(() =>
      expect(repository.commands.hydrate).toHaveBeenCalledWith(
        expect.objectContaining({ rollbackBoardIds: [board.id] }),
      ),
    )
  })

  it('marks the shared retry button busy while a catalog reload is pending', async () => {
    const { catalog, repository } = createRecoveryCatalog()
    vi.mocked(repository.commands.hydrate).mockResolvedValue({
      isOk: false as const,
      error: new Error('offline'),
    })
    await catalog.commands.initialize()
    expect(catalog.viewModel.catalogHydrationFailed.value).toBe(true)

    const gated = createDeferred<{ isOk: true }>()
    vi.mocked(repository.commands.hydrate).mockReturnValueOnce(gated.promise)
    const reload = catalog.commands.reloadCatalog()
    expect(catalog.viewModel.isCatalogReloadPending.value).toBe(true)
    renderWithUser(BoardsPage)

    const retry = within(await screen.findByRole('alert')).getByRole('button', {
      name: 'Reloading…',
    })
    expect(retry).toBeDisabled()
    expect(retry).toHaveAttribute('aria-busy', 'true')
    gated.resolve({ isOk: true })
    await reload
  })
})
