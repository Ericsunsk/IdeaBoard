import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useLibraryIslandDnD } from './useLibraryIslandDnD'

const mocks = vi.hoisted(() => ({
  submit: vi.fn(),
}))

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

function saveModuleStub() {
  return {
    operations: ref([]),
    submit: mocks.submit,
    retryOperation: vi.fn(),
    clear: vi.fn(),
    dispose: vi.fn(),
  }
}

function savedResult(folderId: string | undefined) {
  return {
    operationId: 'op-1',
    status: 'succeeded',
    items: [
      {
        id: 'item-1',
        kind: 'external',
        title: 'Image',
        status: 'saved',
        attempts: 1,
        asset: { folderId },
        assetKey: 'unsplash:photo-1',
      },
    ],
  }
}

describe('useLibraryIslandDnD', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.submit.mockResolvedValue(savedResult('lfolder_folder0007'))
  })

  it('submits a reference drop to the shared save module and selects the stored target', async () => {
    const workspace = workspaceStub()
    const { receiveReference } = useLibraryIslandDnD({
      workspace: workspace as never,
      saveModule: saveModuleStub() as never,
    })
    await receiveReference(reference, 'lfolder_folder0007')

    // The island only routes: folder priority and save/notification stay in
    // the module. Selection follows the committed asset folder.
    expect(mocks.submit).toHaveBeenCalledExactlyOnceWith(
      { source: 'external', reference },
      { folderId: 'lfolder_folder0007', galleryEffect: 'selection-will-reload' },
    )
    expect(workspace.selectFolder).toHaveBeenCalledWith('lfolder_folder0007')
  })

  it('saves unfiled drops without a folder id', async () => {
    mocks.submit.mockResolvedValue(savedResult(undefined))
    const workspace = workspaceStub()
    const { receiveReference } = useLibraryIslandDnD({
      workspace: workspace as never,
      saveModule: saveModuleStub() as never,
    })
    await receiveReference(reference, 'all')

    expect(mocks.submit).toHaveBeenCalledExactlyOnceWith(
      { source: 'external', reference },
      { galleryEffect: 'selection-will-reload' },
    )
    expect(workspace.selectFolder).toHaveBeenCalledWith('all')
  })

  it('delegates folder-move drops to the workspace move command', async () => {
    const workspace = workspaceStub()
    const { receiveDrop } = useLibraryIslandDnD({
      workspace: workspace as never,
      saveModule: saveModuleStub() as never,
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
    expect(mocks.submit).not.toHaveBeenCalled()
  })

  it('does not select a stale save after a workspace change', async () => {
    const workspace = workspaceStub()
    const { receiveReference } = useLibraryIslandDnD({
      workspace: workspace as never,
      saveModule: saveModuleStub() as never,
    })
    let finish!: (value: unknown) => void
    mocks.submit.mockReturnValue(
      new Promise((resolve) => {
        finish = resolve
      }),
    )
    const pending = receiveReference(reference, 'all')
    workspace.workspaceId.value = 'ws_other123'
    finish(savedResult(undefined))
    await pending

    expect(mocks.submit).toHaveBeenCalledOnce()
    expect(workspace.selectFolder).not.toHaveBeenCalled()
  })

  it('rejects a removed folder instead of saving', async () => {
    const workspace = workspaceStub()
    const { receiveReference } = useLibraryIslandDnD({
      workspace: workspace as never,
      saveModule: saveModuleStub() as never,
    })
    await receiveReference(reference, 'lfolder_missing123')
    expect(mocks.submit).not.toHaveBeenCalled()
    expect(workspace.selectFolder).not.toHaveBeenCalled()
  })
})
