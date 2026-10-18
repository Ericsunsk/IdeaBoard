import { computed, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import { setUserLocale } from '../../../../core/i18n'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import type { BoardAccessCapabilities, BoardMeta } from '../../session/catalog/boards'
import { makeBoardMeta, primeAuthenticatedWorkspace } from '../../testing'
import { createGalleryController } from './controller'
import BoardsPage from './BoardsPage.vue'

const { useCatalog } = vi.hoisted(() => ({ useCatalog: vi.fn() }))

vi.mock('./useCatalog', () => ({ useCatalog, formatCatalogDate: vi.fn(() => 'today') }))
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

const disposeCatalogs: Array<() => void> = []

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

function ownBoard(): BoardMeta {
  return makeBoardMeta('own', 'Own Board', 30, {
    revision: 1,
    accessState: 'private',
    workspaceAccess: 'none',
    isCreator: true,
    hasDirectGrant: false,
    capabilities: ownerCapabilities,
  })
}

function teamBoard(): BoardMeta {
  return makeBoardMeta('team', 'Team Board', 20, {
    revision: 1,
    accessState: 'workspace',
    workspaceAccess: 'viewer',
    isCreator: false,
    hasDirectGrant: true,
    capabilities: viewerCapabilities,
  })
}

async function renderRange(initialBoards: BoardMeta[] = [ownBoard(), teamBoard()]) {
  const hydrate = vi.fn(async () => ({ isOk: true as const }))
  const catalog = createGalleryController({
    catalog: {
      viewModel: {
        boards: ref(initialBoards),
        scope: ref('user-1'),
        byId: computed(() => new Map()),
      },
      commands: {
        hydrate,
        create: vi.fn(),
        rename: vi.fn(),
        duplicate: vi.fn(),
        delete: vi.fn(),
      },
      dispose: vi.fn(),
    },
    scenes: { sceneMap: ref({}), workspaceId: ref(null) },
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
  const view = renderWithUser(BoardsPage)
  await catalog.commands.initialize()
  return { ...view, catalog, hydrate }
}

beforeEach(async () => {
  localStorage.clear()
  primeAuthenticatedWorkspace('ws_range_page')
  await setUserLocale('en')
})

afterEach(async () => {
  disposeCatalogs.splice(0).forEach((dispose) => dispose())
  localStorage.clear()
  await setUserLocale('en')
})

describe('BoardsPage gallery range toggle', () => {
  test('exposes the current range with tooltip, accessible name, and explicit state', async () => {
    await renderRange()

    // Single circular toggle: default personal shows the User glyph.
    const toggle = screen.getByRole('button', { name: 'Personal boards' })
    expect(toggle).toHaveAttribute('title', 'Personal boards')
    // Explicit state beyond color: pressed semantics plus a state marker.
    expect(toggle).toHaveAttribute('aria-pressed', 'false')
    expect(toggle).toHaveAttribute('data-state', 'inactive')
    // Only the personal card renders; the team card stays out of the range.
    expect(screen.getByText('Own Board')).toBeVisible()
    expect(screen.queryByText('Team Board')).toBeNull()
  })

  test('flips ranges by pointer and keyboard without refetching', async () => {
    const view = await renderRange()

    await view.user.click(screen.getByRole('button', { name: 'Personal boards' }))
    const toggle = screen.getByRole('button', { name: 'Team boards' })
    expect(toggle).toHaveAttribute('aria-pressed', 'true')
    expect(toggle).toHaveAttribute('data-state', 'active')
    expect(screen.getByText('Team Board')).toBeVisible()
    expect(screen.queryByText('Own Board')).toBeNull()

    toggle.focus()
    expect(toggle).toHaveFocus()
    await view.user.keyboard('{Enter}')
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Personal boards' })).toHaveAttribute(
        'aria-pressed',
        'false',
      ),
    )
    expect(screen.getByText('Own Board')).toBeVisible()

    // Range switching is display-only: the catalog is never re-hydrated.
    expect(view.hydrate).toHaveBeenCalledTimes(1)
  })

  test('renders per-range empty states and search no-results distinctly', async () => {
    const view = await renderRange([ownBoard()])

    // Team empty renders no copy.
    await view.user.click(screen.getByRole('button', { name: 'Personal boards' }))
    expect(screen.queryByText('Own Board')).toBeNull()
    expect(screen.queryByText('No matching canvas')).toBeNull()

    await view.user.click(screen.getByRole('button', { name: 'Team boards' }))
    expect(screen.getByText('Own Board')).toBeVisible()

    view.catalog.commands.setSearchKeyword('nothing-matches')
    await waitFor(() => expect(screen.getByText('No matching canvas')).toBeVisible())
  })

  test('preserves the create control when the personal range holds no boards', async () => {
    await renderRange([teamBoard()])

    expect(screen.getByRole('button', { name: 'New Canvas' })).toBeVisible()
    expect(screen.queryByText('No matching canvas')).toBeNull()
  })

  test('localizes toggle names across locales', async () => {
    const view = await renderRange()

    await expect(setUserLocale('zh-CN')).resolves.toBe(true)
    await waitFor(() =>
      expect(screen.getByRole('button', { name: '个人画板' })).toHaveAttribute(
        'aria-pressed',
        'false',
      ),
    )
    await view.user.click(screen.getByRole('button', { name: '个人画板' }))
    expect(screen.getByRole('button', { name: '团队画板' })).toHaveAttribute('aria-pressed', 'true')
  })

  test('routes the share intent to the single Share boundary without a second panel', async () => {
    const view = await renderRange([ownBoard()])

    expect(useCatalog).toHaveBeenCalledWith(
      expect.objectContaining({
        navigation: expect.objectContaining({ shareBoard: expect.any(Function) }),
      }),
    )

    await view.user.click(screen.getByRole('button', { name: 'More actions' }))
    const share = await screen.findByRole('menuitem', { name: 'Share' })
    await view.user.click(share)
    // The intent resolves through the hook: the gallery opens no dialog of
    // its own and stays on the current range.
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(screen.getByRole('button', { name: 'Personal boards' })).toHaveAttribute(
      'aria-pressed',
      'false',
    )
  })
})
