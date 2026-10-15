import { mediaDisplayFixture } from '../../../../test/mediaDisplay'
import { defineComponent, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import { mockAppToast, type AppToastMocks } from '../../../../test/toast'
import { i18n } from '../../../../core/i18n'
import SearchSavePopover from './SearchSavePopover.vue'
import type { SearchSavePopoverController } from '../browser/savePopoverContract'
import type { SearchResult } from '../../core/models'

const mocks = vi.hoisted(() => ({ saveReference: vi.fn() }))
vi.mock('../../../../core/composables/useAppToast')
vi.mock('../../session/adapters/libraryCoordinator', () => ({
  useSearchLibraryCoordinator: () => ({
    folderFacts: {
      folders: ref([{ id: 'folder-a', name: 'Folder A' }]),
      load: async () => ({ status: 'current' }),
    },
    saveReference: mocks.saveReference,
  }),
}))

const result: SearchResult = {
  id: 'photo',
  provider: 'unsplash',
  title: 'Photo',
  author: 'Author',
  author_url: 'https://example.com/author',
  source_url: 'https://example.com/photo',
  thumbnail_url: 'https://example.com/thumb',
  preview_url: 'https://example.com/preview',
  original_url: 'https://example.com/original',
  width: 100,
  height: 100,
  media_kind: 'image',
  display: mediaDisplayFixture(),
}
const Host = defineComponent({
  components: { SearchSavePopover },
  setup() {
    const popover = ref<SearchSavePopoverController>()
    function open(event: Event) {
      popover.value?.toggle(event, { result, isAuthenticated: true, isSaved: false })
    }
    return { popover, open }
  },
  template: '<button @click="open">Open save menu</button><SearchSavePopover ref="popover" />',
})

describe('Search save feedback', () => {
  let toast: AppToastMocks
  beforeEach(() => {
    vi.clearAllMocks()
    toast = mockAppToast()
  })

  it.each([
    ['folder-a', 'Folder A'],
    ['folder-unknown', undefined],
  ])(
    'reports the actual deduplicated folder %s after creating another folder',
    async (folderId, name) => {
      mocks.saveReference.mockResolvedValue({
        asset: { folderId },
        createdFolder: { id: 'folder-b', name: 'Folder B' },
      })
      const { user } = renderWithUser(Host, {
        global: {
          stubs: {
            Popover: { props: ['open'], template: '<div v-if="open"><slot /></div>' },
            PopoverContent: { template: '<div><slot /></div>' },
            PopoverAnchor: true,
          },
        },
      })
      await user.click(screen.getByRole('button', { name: 'Open save menu' }))
      const input = screen.getByRole('textbox')
      await user.type(input, 'Folder B')
      await waitFor(() => expect(screen.getByRole('button', { name: /Folder B/ })).toBeEnabled())
      await user.click(screen.getByRole('button', { name: /Folder B/ }))
      await waitFor(() =>
        expect(toast.success).toHaveBeenCalledWith(
          name
            ? i18n.global.t('search.savePopover.savedToast', { name })
            : i18n.global.t('search.savePopover.savedToLibraryToast'),
        ),
      )
      expect(mocks.saveReference).toHaveBeenCalledWith(result, undefined, 'Folder B')
    },
  )
})
