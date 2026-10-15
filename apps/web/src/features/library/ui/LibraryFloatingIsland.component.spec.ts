import { mediaDisplayFixture } from '../../../test/mediaDisplay'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { computed, nextTick, ref } from 'vue'
import { render, screen, waitFor } from '../../../test/render'
import LibraryFloatingIsland from './LibraryFloatingIsland.vue'
import { createPointerDropTargets } from '../../../core/interaction/pointerDropTargets'
import type { LibraryDragPayload } from '../core/drag'
import type { LibraryFolderFacts } from '../session/folderFacts'
import type { LibrarySearchReference } from '../core/validators'
import type { useLibraryWorkspace } from '../session/useLibraryWorkspace'

type IslandProps = {
  -readonly [Key in keyof InstanceType<typeof LibraryFloatingIsland>['$props']]: InstanceType<
    typeof LibraryFloatingIsland
  >['$props'][Key]
}

const mocks = vi.hoisted(() => ({
  workspace: null as ReturnType<typeof workspaceState> | null,
  saveReference: vi.fn(),
}))

vi.mock('../session/useLibraryWorkspace', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../session/useLibraryWorkspace')>()),
  useLibraryWorkspace: () => mocks.workspace,
}))
vi.mock('../session/libraryUseCases', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../session/libraryUseCases')>()),
  saveWorkspaceSearchReference: mocks.saveReference,
}))
vi.mock('../../../core/composables/useAppToast', () => ({
  useAppToast: () => ({ error: vi.fn(), success: vi.fn() }),
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
    selectFolder: vi.fn(),
    moveSelectedAsset: vi.fn(),
  }
  workspace.foldersLoading = ref(false)
  workspace.loading = ref(false)
  return workspace
}

function mountIsland() {
  const targets = createPointerDropTargets<LibraryDragPayload>()
  const publishMutation = vi.fn(async () => {})
  const props: Partial<IslandProps> = {
    presentation: 'search',
    workspaceId: 'ws_test123',
    folderFacts: {} as LibraryFolderFacts,
    mutationPublisher: {
      publishMutation,
      publishMutations: vi.fn(),
      subscribe: vi.fn(() => () => {}),
    },
    dropTargets: targets,
  }
  props.open = true
  const view = render(LibraryFloatingIsland, {
    props: props as IslandProps,
    global: {
      stubs: {
        LibraryAssetGallery: true,
        LibraryAssetLightbox: true,
        LibraryFolderDialogs: true,
        LibraryFolderContextMenu: true,
        UserConfirmDialog: true,
      },
    },
  })
  return { ...view, targets, publishMutation }
}

describe('Library pointer reference receiving', () => {
  beforeEach(() => {
    mocks.workspace = workspaceState()
    mocks.saveReference.mockReset().mockResolvedValue({
      asset: { provider: 'unsplash', sourceId: 'image-1', folderId: 'lfolder_folder0007' },
      affectedFolderKeys: ['all', 'lfolder_folder0007'],
    })
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue(
      new DOMRect(0, 0, 100, 100),
    )
  })
  afterEach(() => vi.restoreAllMocks())

  it('receives a folder drop through the existing Library receiver', async () => {
    const { targets, publishMutation } = mountIsland()
    const folder = screen.getByRole('tab', { name: 'Destination' })
    vi.spyOn(document, 'elementFromPoint').mockReturnValue(folder)
    targets.update(10, 10, { kind: 'reference', reference })
    await nextTick()

    targets.drop({ kind: 'reference', reference }, 10, 10)
    await waitFor(() =>
      expect(mocks.workspace?.selectFolder).toHaveBeenCalledWith('lfolder_folder0007'),
    )

    expect(mocks.saveReference).toHaveBeenCalledExactlyOnceWith(
      'ws_test123',
      reference,
      'lfolder_folder0007',
    )
    expect(publishMutation).toHaveBeenCalledOnce()
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

    expect(mocks.saveReference).toHaveBeenCalledExactlyOnceWith(
      'ws_test123',
      reference,
      'lfolder_folder0007',
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
    expect(mocks.saveReference).not.toHaveBeenCalled()
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
    expect(mocks.saveReference).not.toHaveBeenCalled()
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

    expect(mocks.saveReference).not.toHaveBeenCalled()
  })
})
