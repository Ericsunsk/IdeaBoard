import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { buildLibraryAssetSavedOutcome, type LibraryMutationPublisher } from './folderFacts'
import { requireWorkspaceId, type LibrarySearchReference } from '../core/validators'
import { saveWorkspaceSearchReference } from './libraryUseCases'
import type { LibraryAssetView } from '../core/transfer'
import {
  toCanvasLibraryImportDescriptor,
  getLibraryDropEffect,
  readLibraryFolderMoveTransfer,
  readSearchReferenceTransfer,
  writeCanvasLibraryImportTransfer,
  writeLibraryFolderMoveTransfer,
} from '../core/transfer'
import { useAppToast } from '../../../core/composables/useAppToast'
import { userFacingErrorCopy } from '../../../core/errors/errorCatalog'
import {
  LIBRARY_ALL_FOLDER_KEY,
  type LibraryFolderTab,
  type useLibraryWorkspace,
} from './useLibraryWorkspace'

export interface LibraryIslandDnDOptions {
  workspace: ReturnType<typeof useLibraryWorkspace>
  mutationPublisher: LibraryMutationPublisher
}

export function useLibraryIslandDnD(options: LibraryIslandDnDOptions) {
  const { workspace, mutationPublisher } = options
  const { t } = useI18n()
  const toast = useAppToast()
  const hoveredPillKey = ref<string | null>(null)

  function handleAssetDragStart(e: DragEvent, asset: LibraryAssetView) {
    if (!e.dataTransfer) return
    const descriptor = toCanvasLibraryImportDescriptor(asset)
    writeLibraryFolderMoveTransfer(e.dataTransfer, asset.id)
    writeCanvasLibraryImportTransfer(e.dataTransfer, descriptor)

    // Micro Drag Ghost (58×44px 微缩投影圆角拖拽影子)
    const ghost = document.createElement('div')
    ghost.style.position = 'fixed'
    ghost.style.top = '-9999px'
    ghost.style.left = '-9999px'
    ghost.style.width = '58px'
    ghost.style.height = '44px'
    ghost.style.borderRadius = '8px'
    ghost.style.overflow = 'hidden'
    ghost.style.boxShadow = '0 12px 28px rgba(0,0,0,0.3)'
    ghost.style.transform = 'rotate(-3deg)'
    ghost.style.zIndex = '9999'

    const img = document.createElement('img')
    img.src = asset.previewUrl
    img.style.width = '100%'
    img.style.height = '100%'
    img.style.objectFit = 'cover'

    ghost.appendChild(img)
    document.body.appendChild(ghost)
    e.dataTransfer.setDragImage(ghost, 29, 22)

    setTimeout(() => {
      document.body.removeChild(ghost)
    }, 0)
  }

  function handleIslandDragOver(e: DragEvent) {
    e.dataTransfer!.dropEffect = getLibraryDropEffect(e.dataTransfer)
  }

  async function handleIslandDrop(e: DragEvent) {
    await handleDropIntoFolder(e, workspace.selectedFolder.value)
  }

  async function handleDropIntoFolder(e: DragEvent, folderKey: string) {
    const libraryAssetID = readLibraryFolderMoveTransfer(e.dataTransfer)
    if (libraryAssetID) {
      if (folderKey !== LIBRARY_ALL_FOLDER_KEY) {
        await workspace.moveSelectedAsset(libraryAssetID, folderKey)
      }
      return
    }

    const reference = readSearchReferenceTransfer(e.dataTransfer)
    if (!reference) return
    await saveReferenceToFolder(reference, folderKey)
  }

  function handlePillDragOver(e: DragEvent, key: string) {
    e.dataTransfer!.dropEffect = getLibraryDropEffect(e.dataTransfer)
    hoveredPillKey.value = key
  }

  function handlePillDragLeave() {
    hoveredPillKey.value = null
  }

  async function handlePillDrop(e: DragEvent, folder: LibraryFolderTab) {
    hoveredPillKey.value = null
    await handleDropIntoFolder(e, folder.folderKey)
  }

  async function saveReferenceToFolder(reference: LibrarySearchReference, folderKey: string) {
    const targetFolder = workspace.folders.value.find((folder) => folder.id === folderKey)
    const targetFolderId =
      targetFolder && folderKey !== LIBRARY_ALL_FOLDER_KEY ? targetFolder.id : undefined

    try {
      const workspaceId = requireWorkspaceId(workspace.workspaceId.value)
      const result = await saveWorkspaceSearchReference(workspaceId, reference, targetFolderId)
      // The committed save owns the target: publish its actual folders and
      // reload through the caller's own selection change.
      await mutationPublisher.publishMutation({
        ...buildLibraryAssetSavedOutcome(result.asset, 'selection-will-reload'),
        affectedFolderKeys: result.affectedFolderKeys,
      })
      workspace.selectFolder(result.asset.folderId ?? LIBRARY_ALL_FOLDER_KEY)
    } catch (error) {
      toast.error(userFacingErrorCopy(error, t('library.errors.saveAssetFailed')))
    }
  }

  return {
    hoveredPillKey,
    handleAssetDragStart,
    handleIslandDragOver,
    handleIslandDrop,
    handlePillDragOver,
    handlePillDragLeave,
    handlePillDrop,
  }
}
