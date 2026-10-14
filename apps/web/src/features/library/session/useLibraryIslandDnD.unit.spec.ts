import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useLibraryIslandDnD } from './useLibraryIslandDnD'

const mocks = vi.hoisted(() => ({
  saveReference: vi.fn(),
  toastError: vi.fn(),
}))

vi.mock('../../../core/composables/useAppToast', () => ({
  useAppToast: () => ({ success: vi.fn(), error: mocks.toastError }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

vi.mock('./libraryUseCases', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./libraryUseCases')>()
  return { ...actual, saveWorkspaceSearchReference: mocks.saveReference }
})

const reference = {
  provider: 'unsplash' as const,
  id: 'photo-1',
  source_url: 'https://example.com/source',
  preview_url: 'https://example.com/preview',
  original_url: 'https://example.com/original',
  title: 'Image',
  author: 'Author',
  width: 100,
  height: 100,
}

function workspaceStub() {
  return {
    folders: ref([{ id: 'lfolder_folder0007', name: 'Destination', workspaceId: 'ws_test123' }]),
    workspaceId: ref('ws_test123'),
    selectedFolder: ref('all'),
    selectFolder: vi.fn(),
    moveSelectedAsset: vi.fn(async () => true),
  }
}

describe('useLibraryIslandDnD', () => {
  beforeEach(() => vi.clearAllMocks())

  it('publishes the stored folders for a reference drop and selects the actual target', async () => {
    mocks.saveReference.mockResolvedValue({
      asset: { provider: 'unsplash', sourceId: 'photo-1', folderId: 'lfolder_folder0007' },
      assetKey: 'unsplash:photo-1',
      affectedFolderKeys: ['all', 'lfolder_folder0007'],
    })
    const outcomes: unknown[] = []
    const workspace = workspaceStub()
    const { receiveReference } = useLibraryIslandDnD({
      workspace: workspace as never,
      mutationPublisher: {
        publishMutation: vi.fn(async (outcome: unknown) => {
          outcomes.push(outcome)
        }),
        publishMutations: vi.fn(async () => {}),
        subscribe: vi.fn(() => () => {}),
      },
    })
    await receiveReference(reference, 'lfolder_folder0007')

    expect(mocks.saveReference).toHaveBeenCalledWith('ws_test123', reference, 'lfolder_folder0007')
    // Facts come from the committed save — never rebuilt from the pill key —
    // while the surface keeps its selection-will-reload policy.
    expect(outcomes).toEqual([
      {
        kind: 'asset-saved',
        provider: 'unsplash',
        sourceID: 'photo-1',
        assetKey: 'unsplash:photo-1',
        affectedFolderKeys: ['all', 'lfolder_folder0007'],
        galleryEffect: 'selection-will-reload',
      },
    ])
    expect(workspace.selectFolder).toHaveBeenCalledWith('lfolder_folder0007')
  })

  it('publishes only the all aggregate for an unfiled drop without duplicating keys', async () => {
    mocks.saveReference.mockResolvedValue({
      asset: { provider: 'unsplash', sourceId: 'photo-1', folderId: undefined },
      assetKey: 'unsplash:photo-1',
      affectedFolderKeys: ['all'],
    })
    const outcomes: unknown[] = []
    const workspace = workspaceStub()
    const { receiveReference } = useLibraryIslandDnD({
      workspace: workspace as never,
      mutationPublisher: {
        publishMutation: vi.fn(async (outcome: unknown) => {
          outcomes.push(outcome)
        }),
        publishMutations: vi.fn(async () => {}),
        subscribe: vi.fn(() => () => {}),
      },
    })
    await receiveReference(reference, 'all')

    expect(outcomes).toEqual([
      {
        kind: 'asset-saved',
        provider: 'unsplash',
        sourceID: 'photo-1',
        assetKey: 'unsplash:photo-1',
        affectedFolderKeys: ['all'],
        galleryEffect: 'selection-will-reload',
      },
    ])
    expect(workspace.selectFolder).toHaveBeenCalledWith('all')
  })

  it('delegates folder-move drops to the workspace move command', async () => {
    const workspace = workspaceStub()
    const { receiveDrop } = useLibraryIslandDnD({
      workspace: workspace as never,
      mutationPublisher: {
        publishMutation: vi.fn(async () => {}),
        publishMutations: vi.fn(async () => {}),
        subscribe: vi.fn(() => () => {}),
      },
    })
    await receiveDrop(
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
      'lfolder_folder0007',
    )

    expect(workspace.moveSelectedAsset).toHaveBeenCalledWith(
      'lasset_asset0009',
      'lfolder_folder0007',
    )
    expect(mocks.saveReference).not.toHaveBeenCalled()
  })

  it('does not publish or select a stale save after a workspace change', async () => {
    const workspace = workspaceStub()
    const publishMutation = vi.fn()
    const { receiveReference } = useLibraryIslandDnD({
      workspace: workspace as never,
      mutationPublisher: { publishMutation, publishMutations: vi.fn(), subscribe: vi.fn() },
    })
    let finish!: (value: unknown) => void
    mocks.saveReference.mockReturnValue(
      new Promise((resolve) => {
        finish = resolve
      }),
    )
    const pending = receiveReference(reference, 'all')
    workspace.workspaceId.value = 'ws_other123'
    finish({ asset: { folderId: undefined }, affectedFolderKeys: ['all'] })
    await pending

    expect(mocks.saveReference).toHaveBeenCalledWith('ws_test123', reference, undefined)
    expect(publishMutation).not.toHaveBeenCalled()
    expect(workspace.selectFolder).not.toHaveBeenCalled()
  })

  it('rejects a removed folder instead of saving an unfiled asset', async () => {
    const workspace = workspaceStub()
    const { receiveReference } = useLibraryIslandDnD({
      workspace: workspace as never,
      mutationPublisher: {
        publishMutation: vi.fn(),
        publishMutations: vi.fn(),
        subscribe: vi.fn(),
      },
    })
    await receiveReference(reference, 'lfolder_missing123')
    expect(mocks.saveReference).not.toHaveBeenCalled()
    expect(workspace.selectFolder).not.toHaveBeenCalled()
  })
})
