import { ref } from 'vue'
import { isSupportedUploadImageFile } from '../../../core/utils/imageUploadSupport'
import { prepareImageAssetForUpload } from '../../../core/media/imageAssetPipeline'
import { i18n } from '../../../core/i18n'
import { queryLibraryDropHit } from './libraryDropHitTest'
import { resolveLibrarySaveFolderId, type LibrarySaveModule } from '../session/librarySaveModule'
import { LIBRARY_ALL_FOLDER_KEY } from '../session/folderFacts'

/**
 * Native file drop adapter for the Library island.
 *
 * Stays independent from the Pointer drag adapter (`useLibraryDropTargets`):
 * native `DataTransfer` files never ride the Pointer drop targets, and the
 * Pointer module never touches native events. Dropped images are prepared
 * (dimensions, normalization) here, then saved through the shared save
 * module — the single place that uploads, retries, publishes, and notifies.
 */

export type LibraryNativeFileDropOptions = {
  isOpen: () => boolean
  canReceive: () => boolean
  selectedFolder: () => string
  isFolderKnown: (key: string) => boolean
  selectFolder: (key: string) => void
  saveModule: LibrarySaveModule
  notifyWarning: (message: string) => void
}

function isFileDrag(event: DragEvent): boolean {
  return event.dataTransfer?.types.includes('Files') ?? false
}

export function useLibraryNativeFileDrop(options: LibraryNativeFileDropOptions) {
  const isNativeHovering = ref(false)
  const nativeHoveredFolderKey = ref<string | null>(null)

  function clearHover() {
    isNativeHovering.value = false
    nativeHoveredFolderKey.value = null
  }

  function onNativeDragOver(event: DragEvent) {
    if (!options.isOpen() || !options.canReceive() || !isFileDrag(event)) return
    event.preventDefault()
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'
    isNativeHovering.value = true
    const hit = queryLibraryDropHit(event.clientX, event.clientY)
    nativeHoveredFolderKey.value = hit?.kind === 'folder' ? hit.folderKey : null
  }

  function onNativeDragLeave(event: DragEvent) {
    if (event.relatedTarget) return
    clearHover()
  }

  function resolveDropTarget(event: DragEvent) {
    const hit = queryLibraryDropHit(event.clientX, event.clientY)
    return resolveLibrarySaveFolderId(
      hit?.kind === 'folder' ? hit.folderKey : undefined,
      options.selectedFolder(),
      options.isFolderKnown,
    )
  }

  function fileTitleFor(filename: string): string {
    return filename.replace(/\.[^.]+$/, '')
  }

  async function submitPreparedDrop(files: File[], folderId: string | undefined): Promise<void> {
    const result = await options.saveModule.submit(
      files.map((file) => ({
        source: 'media' as const,
        file: async () => {
          const prepared = await prepareImageAssetForUpload(file, {
            messages: {
              unknownError: i18n.global.t('common.errors.uploadFailed'),
              readImageSizeSkipCompression: i18n.global.t('common.errors.readImageSizeFailed'),
              compressFallbackOriginal: i18n.global.t('library.errors.saveAssetFailed'),
            },
          })
          for (const warning of prepared.warnings) options.notifyWarning(warning)
          return prepared.uploadFile
        },
        filename: file.name,
        ...(fileTitleFor(file.name).trim() ? { title: fileTitleFor(file.name) } : {}),
      })),
      {
        ...(folderId == null ? {} : { folderId }),
        galleryEffect: 'selection-will-reload',
      },
    )
    if (result.status === 'abandoned') return
    const saved = result.items.find((item) => item.status === 'saved')
    if (saved) options.selectFolder(saved.asset?.folderId ?? LIBRARY_ALL_FOLDER_KEY)
  }

  async function onNativeDrop(event: DragEvent) {
    const wasHovering = isNativeHovering.value
    clearHover()
    if (!options.isOpen() || !options.canReceive() || !isFileDrag(event)) return
    event.preventDefault()
    if (!wasHovering && !event.dataTransfer?.files.length) return
    const files = [...(event.dataTransfer?.files ?? [])].filter(isSupportedUploadImageFile)
    if (files.length === 0) return
    const resolved = resolveDropTarget(event)
    if (resolved.kind === 'invalid') return
    await submitPreparedDrop(files, resolved.folderId)
  }

  return {
    isNativeHovering,
    nativeHoveredFolderKey,
    onNativeDragOver,
    onNativeDragLeave,
    onNativeDrop,
  }
}
