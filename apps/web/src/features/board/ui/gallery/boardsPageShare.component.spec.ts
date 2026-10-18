import { computed, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import { setUserLocale } from '../../../../core/i18n'
import { renderWithUser, screen } from '../../../../test/render'
import { clearStoredAuth } from '../../../../core/utils/auth'
import { primeAuthenticatedWorkspace } from '../../testing'
import type { BoardAccessCapabilities } from '../../session/catalog/boards'
import type { CatalogEntry } from './contract'
import BoardCard from './BoardCard.vue'
import BoardsPage from './BoardsPage.vue'

const { useCatalog } = vi.hoisted(() => ({ useCatalog: vi.fn() }))

vi.mock('./useCatalog', () => ({ useCatalog, formatCatalogDate: vi.fn() }))
vi.mock('../../../workspace', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../../workspace')>()
  return {
    ...actual,
    WorkspaceHeader: { render: () => null },
    useWorkspaceSelection: vi.fn(),
  }
})
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))
vi.mock('../share/ShareDialog.vue', () => ({
  default: {
    name: 'ShareDialogStub',
    props: { panel: { type: Object } },
    template: '<div v-if="panel.isOpen.value" data-testid="share-stub">share open</div>',
  },
}))

const WORKSPACE = 'ws_share123'

function primeScope() {
  primeAuthenticatedWorkspace(WORKSPACE)
}

const manageCapabilities: BoardAccessCapabilities = {
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

function shareBoard(): CatalogEntry {
  return {
    id: 'board-1',
    name: 'Campaign',
    createdAt: 1,
    updatedAt: 2,
    capabilities: { ...manageCapabilities },
    accessState: 'workspace',
    workspaceAccess: 'none',
    isCreator: false,
  }
}

function renderPage(board: CatalogEntry) {
  const catalog = {
    viewModel: {
      visibleBoards: computed(() => [board]),
      sceneMap: ref({}),
      workspaceId: ref(WORKSPACE),
      searchKeyword: ref(''),
      sortMode: ref('recent'),
      range: ref('personal'),
      rangeCounts: computed(() => ({ personal: 1, team: 0 })),
      activeMenuBoardId: ref<string | null>(null),
      renamingBoardId: ref<string | null>(null),
      renameDraft: ref(''),
    },
    commands: {
      initialize: vi.fn(),
      setSearchKeyword: vi.fn(),
      setSortMode: vi.fn(),
      setRange: vi.fn(),
      toggleBoardMenu: vi.fn(),
      handleBoardMenuHidden: vi.fn(),
      createAndOpenBoard: vi.fn(),
      duplicateBoard: vi.fn(),
      startRename: vi.fn(),
      updateRenameDraft: vi.fn(),
      commitRename: vi.fn(),
      cancelRename: vi.fn(),
      reloadCatalog: vi.fn(),
      openBoardFromCard: vi.fn(),
      requestDelete: vi.fn(),
      closeDeleteDialog: vi.fn(),
      handleDeleteDialogHidden: vi.fn(),
      confirmDelete: vi.fn(),
    },
    dispose: vi.fn(),
  }
  useCatalog.mockReturnValue(catalog)
  return { ...renderWithUser(BoardsPage), catalog }
}

function renderCard(board: CatalogEntry) {
  return renderWithUser(BoardCard, {
    props: {
      board,
      sceneEntry: null,
      workspaceId: WORKSPACE,
      isRenaming: false,
      renameDraft: '',
      isMenuOpen: false,
      displayDate: 'today',
    },
  })
}

beforeEach(async () => {
  await setUserLocale('en')
  localStorage.clear()
  clearStoredAuth()
  primeScope()
})

afterEach(async () => {
  await setUserLocale('en')
})

describe('BoardsPage share intent', () => {
  test('opens the single Share panel from the card Share item', async () => {
    const view = renderPage(shareBoard())

    expect(screen.queryByTestId('share-stub')).toBeNull()
    await view.user.click(screen.getByRole('button', { name: 'More actions' }))
    await view.user.click(await screen.findByRole('menuitem', { name: 'Share' }))

    expect(await screen.findByTestId('share-stub')).toBeVisible()
  })
})

describe('BoardCard share menu', () => {
  test('managers see the Share item and emit the intent', async () => {
    const view = renderCard(shareBoard())

    await view.user.click(screen.getByRole('button', { name: 'More actions' }))
    await view.user.click(await screen.findByRole('menuitem', { name: 'Share' }))

    expect(view.emitted()['share-action']).toHaveLength(1)
  })

  test('viewers see no Share item', async () => {
    const view = renderCard({
      ...shareBoard(),
      capabilities: { ...viewerCapabilities },
      accessState: 'private',
    })

    await view.user.click(screen.getByRole('button', { name: 'More actions' }))
    expect(await screen.findByRole('menuitem', { name: 'Duplicate' })).toBeVisible()
    expect(screen.queryByRole('menuitem', { name: 'Share' })).toBeNull()
    expect(view.emitted()['share-action']).toBeUndefined()
  })
})
