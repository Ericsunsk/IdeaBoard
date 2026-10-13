import { computed, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { setUserLocale } from '../../../../core/i18n'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import type { CatalogSortMode } from './contract'
import { createGalleryController } from './controller'
import BoardsPage from './BoardsPage.vue'

const { useCatalog } = vi.hoisted(() => ({ useCatalog: vi.fn() }))

vi.mock('./useCatalog', () => ({ useCatalog, formatCatalogDate: vi.fn() }))
vi.mock('../../../workspace', () => ({
  WorkspaceHeader: { render: () => null },
  useWorkspaceSelection: vi.fn(),
}))
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
    preview: {
      previewMap: ref({}),
      reloadFromLocal: vi.fn(),
      schedule: vi.fn(),
      invalidate: vi.fn(),
      dispose: vi.fn(),
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
