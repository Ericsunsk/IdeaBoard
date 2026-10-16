import { mediaDisplayFixture } from '../../../test/mediaDisplay'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { computed, nextTick, ref } from 'vue'
import { render, screen, userEvent, waitFor } from '../../../test/render'
import LibraryFloatingIsland from './LibraryFloatingIsland.vue'
import { createPointerDropTargets } from '../../../core/interaction/pointerDropTargets'
import type { LibraryDragPayload } from '../core/drag'
import { createLibraryFolderFacts, createLibraryMutationPublisher } from '../session/folderFacts'
import type { LibrarySearchReference } from '../core/validators'
import type { useLibraryWorkspace } from '../session/useLibraryWorkspace'
import type { LibrarySaveOperationState } from '../session/librarySaveModule'

type IslandProps = {
  -readonly [Key in keyof InstanceType<typeof LibraryFloatingIsland>['$props']]: InstanceType<
    typeof LibraryFloatingIsland
  >['$props'][Key]
}

const mocks = vi.hoisted(() => ({
  workspace: null as ReturnType<typeof workspaceState> | null,
  submit: vi.fn(),
}))

vi.mock('../session/useLibraryWorkspace', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../session/useLibraryWorkspace')>()),
  useLibraryWorkspace: () => mocks.workspace,
}))
vi.mock('../../../core/composables/useAppToast', () => ({
  useAppToast: () => ({ error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() }),
}))

const reference: LibrarySearchReference = {
  provider: 'unsplash',
  id: 'image-1',
  title: 'Image',
  author: 'Author',
  source_url: 'https://example.com/source',
  preview_url: 'https://example.com/preview',
  original_url: 'https://example.com/original',
  width: 100,
  height: 100,
}

function workspaceState() {
  const workspace: Partial<ReturnType<typeof useLibraryWorkspace>> = {
    workspaceId: computed(() => 'ws_test123'),
    folders: ref([
      {
        id: 'lfolder_folder0007',
        workspaceId: 'ws_test123',
        name: 'Destination',
        createdAt: '',
        display: mediaDisplayFixture(),
        updatedAt: '',
      },
    ]),
    folderTabs: computed(() => [
      { folderKey: 'all', name: 'All', isMutable: false },
      { folderKey: 'lfolder_folder0007', name: 'Destination', isMutable: true },
    ]),
    selectedFolder: ref('all'),
    hasMore: ref(false),
    errorMessage: ref(''),
    assets: ref([]),
    activeAsset: ref(null),
    initialize: vi.fn(),
    loadAssets: vi.fn(),
    selectFolder: vi.fn(),
    moveSelectedAsset: vi.fn(),
  }
  workspace.foldersLoading = ref(false)
  workspace.loading = ref(false)
  return workspace
}

function mountIsland(presentation: 'search' | 'canvas' = 'search', isTransitionEnabled = false) {
  const targets = createPointerDropTargets<LibraryDragPayload>()
  const folderFacts = createLibraryFolderFacts(async () => [])
  const mutationPublisher = createLibraryMutationPublisher(folderFacts)
  const saveModule = {
    operations: ref<LibrarySaveOperationState[]>([]),
    submit: mocks.submit,
    retryOperation: vi.fn(),
    clear: vi.fn(),
    dispose: vi.fn(),
  }
  const props: Partial<IslandProps> = {
    presentation,
    workspaceId: 'ws_test123',
    folderFacts,
    mutationPublisher,
    saveModule,
    dropTargets: targets,
  }
  props.open = true
  const view = render(LibraryFloatingIsland, {
    props: props as IslandProps,
    global: {
      stubs: {
        transition: !isTransitionEnabled,
        LibraryAssetGallery: true,
        LibraryAssetLightbox: true,
        LibraryFolderDialogs: true,
        LibraryFolderContextMenu: true,
        UserConfirmDialog: true,
      },
    },
  })
  return { ...view, targets, mutationPublisher, saveModule }
}

describe('Library pointer reference receiving', () => {
  beforeEach(() => {
    mocks.workspace = workspaceState()
    mocks.submit.mockReset().mockResolvedValue({
      operationId: 'op-1',
      status: 'succeeded',
      items: [
        {
          id: 'item-1',
          kind: 'external',
          title: 'Image',
          status: 'saved',
          attempts: 1,
          asset: { folderId: 'lfolder_folder0007' },
          assetKey: 'unsplash:image-1',
        },
      ],
    })
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue(
      new DOMRect(0, 0, 100, 100),
    )
  })
  afterEach(() => vi.restoreAllMocks())

  it.each(['search', 'canvas'] as const)(
    'shows save progress and retries failures in the %s island',
    async (presentation) => {
      const { saveModule } = mountIsland(presentation)
      saveModule.operations.value = [
        {
          id: 'op-retry',
          status: 'running',
          items: [
            {
              id: 'item-1',
              kind: 'media',
              title: 'Dropped image',
              status: 'preparing',
              attempts: 1,
            },
          ],
        },
      ]
      await nextTick()
      expect(screen.getByText(/Dropped image/)).toBeVisible()
      expect(screen.getByText('0/1')).toBeVisible()
      expect(screen.queryByRole('button', { name: 'Retry' })).not.toBeInTheDocument()

      saveModule.operations.value = [
        {
          id: 'op-retry',
          status: 'failed',
          items: [
            {
              id: 'item-1',
              kind: 'media',
              title: 'Dropped image',
              status: 'failed',
              attempts: 1,
              error: 'Upload failed',
            },
          ],
        },
      ]
      await nextTick()
      await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
      expect(saveModule.retryOperation).toHaveBeenCalledExactlyOnceWith('op-retry')
    },
  )

  it('reloads the current gallery once for a committed batch and ignores unrelated folders', async () => {
    const { mutationPublisher } = mountIsland()
    mocks.workspace!.selectedFolder!.value = 'lfolder_folder0007'
    await mutationPublisher.publishMutations([
      {
        kind: 'asset-moved',
        affectedFolderKeys: ['all', 'lfolder_folder0007'],
        galleryEffect: 'reload-current',
      },
      {
        kind: 'asset-moved',
        affectedFolderKeys: ['all', 'lfolder_folder0007'],
        galleryEffect: 'reload-current',
      },
    ])
    expect(mocks.workspace?.loadAssets).toHaveBeenCalledExactlyOnceWith(true)

    await mutationPublisher.publishMutations([
      {
        kind: 'asset-moved',
        affectedFolderKeys: ['all', 'lfolder_other0001'],
        galleryEffect: 'reload-current',
      },
      {
        kind: 'asset-moved',
        affectedFolderKeys: ['all', 'lfolder_folder0007'],
        galleryEffect: 'selection-will-reload',
      },
    ])
    expect(mocks.workspace?.loadAssets).toHaveBeenCalledOnce()
  })

  it('keeps the expanded Search layout until its closing transition finishes', async () => {
    const { container, rerender } = mountIsland('search', true)
    const panel = container.querySelector('aside')!
    panel.style.transition = 'transform 0.22s'
    await userEvent.click(screen.getByRole('button', { name: 'Expand library' }))
    expect(panel.classList.contains('sm:w-[calc(100vw-48px)]')).toBe(true)

    const props: Partial<IslandProps> = {}
    props.open = false
    await rerender(props)
    expect(panel.classList.contains('sm:w-[calc(100vw-48px)]')).toBe(true)
    expect(panel).not.toHaveStyle({ display: 'none' })

    await waitFor(() => expect(panel).toHaveStyle({ display: 'none' }))
    await waitFor(() => expect(panel.classList.contains('sm:w-[350px]')).toBe(true))

    props.open = true
    await rerender(props)
    expect(panel.classList.contains('sm:w-[350px]')).toBe(true)
  })

  it('preserves the expanded layout when reopening during its closing transition', async () => {
    const { container, rerender } = mountIsland('search', true)
    const panel = container.querySelector('aside')!
    panel.style.transition = 'transform 0.22s'
    await userEvent.click(screen.getByRole('button', { name: 'Expand library' }))

    const props: Partial<IslandProps> = {}
    props.open = false
    await rerender(props)
    props.open = true
    await rerender(props)

    await waitFor(() =>
      expect(panel.classList.contains('chrome-presence-leave-active')).toBe(false),
    )
    expect(panel.classList.contains('sm:w-[calc(100vw-48px)]')).toBe(true)
    expect(panel).not.toHaveStyle({ display: 'none' })
  })

  it.each(['search', 'canvas'] as const)(
    'retains the %s gallery when closing and reopening',
    async (presentation) => {
      const { container, rerender } = mountIsland(presentation)
      const panel = container.querySelector('aside')!
      const gallery = panel.querySelector('library-asset-gallery-stub')!
      expect(gallery).not.toBeNull()

      const props: Partial<IslandProps> = {}
      props.open = false
      await rerender(props)
      expect(panel).toHaveStyle({ display: 'none' })
      expect(panel.contains(gallery)).toBe(true)

      props.open = true
      await rerender(props)
      expect(container.querySelector('aside')).toBe(panel)
      expect(panel.querySelector('library-asset-gallery-stub')).toBe(gallery)
      expect(panel).not.toHaveStyle({ display: 'none' })
      expect(mocks.workspace?.initialize).toHaveBeenCalledOnce()
    },
  )

  it('receives a folder drop through the existing Library receiver', async () => {
    const { targets } = mountIsland()
    const folder = screen.getByRole('tab', { name: 'Destination' })
    vi.spyOn(document, 'elementFromPoint').mockReturnValue(folder)
    targets.update(10, 10, { kind: 'reference', reference })
    await nextTick()

    targets.drop({ kind: 'reference', reference }, 10, 10)
    await waitFor(() =>
      expect(mocks.workspace?.selectFolder).toHaveBeenCalledWith('lfolder_folder0007'),
    )

    // Drops delegate to the shared save module with the pill target; the
    // module owns upload, publication, and notification.
    expect(mocks.submit).toHaveBeenCalledExactlyOnceWith(
      { source: 'external', reference },
      { folderId: 'lfolder_folder0007', galleryEffect: 'selection-will-reload' },
    )
    expect(mocks.workspace?.selectFolder).toHaveBeenCalledWith('lfolder_folder0007')
  })

  it('receives on the panel into the currently selected folder', async () => {
    const { targets, container } = mountIsland()
    const panel = container.querySelector('aside')!
    vi.spyOn(document, 'elementFromPoint').mockReturnValue(panel)
    targets.update(10, 10, { kind: 'reference', reference })
    await nextTick()
    mocks.workspace!.selectedFolder!.value = 'lfolder_folder0007'
    targets.drop({ kind: 'reference', reference }, 10, 10)
    await waitFor(() =>
      expect(mocks.workspace?.selectFolder).toHaveBeenCalledWith('lfolder_folder0007'),
    )

    expect(mocks.submit).toHaveBeenCalledExactlyOnceWith(
      { source: 'external', reference },
      { folderId: 'lfolder_folder0007', galleryEffect: 'selection-will-reload' },
    )
  })

  it('moves an existing asset to a folder without saving a reference', async () => {
    const { targets } = mountIsland()
    const folder = screen.getByRole('tab', { name: 'Destination' })
    vi.spyOn(document, 'elementFromPoint').mockReturnValue(folder)
    targets.drop(
      {
        kind: 'asset',
        asset: {
          assetId: 'lasset_asset0009',
          title: '',
          previewUrl: '',
          width: 100,
          height: 100,
        },
      },
      10,
      10,
    )
    await waitFor(() =>
      expect(mocks.workspace?.moveSelectedAsset).toHaveBeenCalledWith(
        'lasset_asset0009',
        'lfolder_folder0007',
      ),
    )
    expect(mocks.submit).not.toHaveBeenCalled()
  })

  it('keeps drops on All as no-ops even when the panel selects a concrete folder', async () => {
    const { targets } = mountIsland()
    mocks.workspace!.selectedFolder!.value = 'lfolder_folder0007'
    const all = screen.getByRole('tab', { name: 'All' })
    vi.spyOn(document, 'elementFromPoint').mockReturnValue(all)
    targets.drop(
      {
        kind: 'asset',
        asset: {
          assetId: 'lasset_asset0009',
          title: '',
          previewUrl: '',
          width: 100,
          height: 100,
        },
      },
      10,
      10,
    )
    await nextTick()
    expect(mocks.workspace?.moveSelectedAsset).not.toHaveBeenCalled()
    expect(mocks.submit).not.toHaveBeenCalled()
  })

  it('unregisters receiving regions when the island closes or unmounts', async () => {
    const { targets, container, rerender, unmount } = mountIsland()
    const panel = container.querySelector('aside')!
    vi.spyOn(document, 'elementFromPoint').mockReturnValue(panel)
    targets.update(10, 10, { kind: 'reference', reference })
    const props: Partial<IslandProps> = {}
    props.open = false
    await rerender(props)
    targets.drop({ kind: 'reference', reference }, 10, 10)
    unmount()
    targets.drop({ kind: 'reference', reference }, 10, 10)

    expect(mocks.submit).not.toHaveBeenCalled()
  })
})
