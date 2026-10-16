import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { prepareImageAssetForUpload } from '../../../core/media/imageAssetPipeline'
import { useLibraryNativeFileDrop } from './useLibraryNativeFileDrop'
import type { LibrarySaveModule } from '../session/librarySaveModule'

vi.mock('../../../core/media/imageAssetPipeline', () => ({
  prepareImageAssetForUpload: vi.fn(),
}))

const prepareMock = vi.mocked(prepareImageAssetForUpload)

function harness() {
  const selectedFolder = ref('all')
  const saveModule = {
    operations: ref([]),
    submit: vi.fn<LibrarySaveModule['submit']>(async () => ({
      operationId: 'op-1',
      status: 'succeeded',
      items: [],
    })),
    retryOperation: vi.fn(),
    clear: vi.fn(),
    dispose: vi.fn(),
  }
  const drop = useLibraryNativeFileDrop({
    isOpen: () => true,
    canReceive: () => true,
    selectedFolder: () => selectedFolder.value,
    isFolderKnown: (key) => key === 'lfolder_folder0007',
    selectFolder: (key: string) => {
      selectedFolder.value = key
    },
    saveModule: saveModule as never,
    notifyWarning: vi.fn(),
  })
  return { selectedFolder, saveModule, drop }
}

function fileDragEvent(type: string, files: File[], x = 10, y = 10): DragEvent {
  return {
    type,
    clientX: x,
    clientY: y,
    preventDefault: vi.fn(),
    dataTransfer: { types: ['Files'], files, dropEffect: '' },
  } as unknown as DragEvent
}

describe('useLibraryNativeFileDrop', () => {
  beforeEach(() => {
    prepareMock.mockReset()
    vi.restoreAllMocks()
  })

  it('submits before preparing dropped images so the module owns their scope and lifecycle', async () => {
    const uploadFile = new File(['pixels'], 'drop.png', { type: 'image/png' })
    prepareMock.mockResolvedValue({
      originalFile: uploadFile,
      uploadFile,
      width: 800,
      height: 600,
      ratio: 0.75,
      isDimensionsKnown: true,
      warnings: [],
      isCompressed: false,
    })
    const { drop, saveModule } = harness()
    const pill = document.createElement('div')
    pill.setAttribute('data-library-drop-folder', 'lfolder_folder0007')
    document.body.appendChild(pill)
    vi.spyOn(document, 'elementFromPoint').mockReturnValue(pill)
    try {
      const file = new File(['pixels'], 'drop.png', { type: 'image/png' })
      drop.onNativeDragOver(fileDragEvent('dragover', [file]))
      expect(drop.isNativeHovering.value).toBe(true)
      expect(drop.nativeHoveredFolderKey.value).toBe('lfolder_folder0007')

      await drop.onNativeDrop(fileDragEvent('drop', [file]))

      expect(prepareMock).not.toHaveBeenCalled()
      expect(saveModule.submit).toHaveBeenCalledExactlyOnceWith(
        [
          {
            source: 'media',
            file: expect.any(Function),
            filename: 'drop.png',
            title: 'drop',
          },
        ],
        { folderId: 'lfolder_folder0007', galleryEffect: 'selection-will-reload' },
      )
      const inputs = saveModule.submit.mock.calls[0]?.[0]
      const input = Array.isArray(inputs) ? inputs[0] : inputs
      if (input?.source !== 'media' || !('file' in input) || typeof input.file !== 'function') {
        throw new Error('Expected deferred image preparation')
      }
      expect(await input.file()).toBe(uploadFile)
      expect(prepareMock).toHaveBeenCalledOnce()
      expect(drop.isNativeHovering.value).toBe(false)
    } finally {
      pill.remove()
    }
  })

  it('ignores non-image files without touching the module', async () => {
    const { drop, saveModule } = harness()
    const file = new File(['text'], 'notes.txt', { type: 'text/plain' })
    await drop.onNativeDrop(fileDragEvent('drop', [file]))

    expect(prepareMock).not.toHaveBeenCalled()
    expect(saveModule.submit).not.toHaveBeenCalled()
  })
})
