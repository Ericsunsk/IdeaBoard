import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { LibraryMutationPublisher } from '../folderFacts'
import {
  libraryAssetKey,
  saveSearchReference,
  type LibraryAssetView,
} from '../resource'
import {
  toCanvasLibraryImportDescriptor,
  getLibraryDropEffect,
  readLibraryFolderMoveTransfer,
  readSearchReferenceTransfer,
  writeCanvasLibraryImportTransfer,
  writeLibraryFolderMoveTransfer,
} from '../transfer'
import { useAppToast } from '../../../shared/composables/useAppToast'
import { userFacingErrorCopy } from '../../../shared/errors/errorCatalog'
import type { LibraryFolderTab, useLibraryWorkspace } from '../useLibraryWorkspace'

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
    img.src = asset.preview_url
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
      if (folderKey !== 'all') {
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
    await handleDropIntoFolder(e, folder.queryKey)
  }

  async function saveReferenceToFolder(
    reference: Parameters<typeof saveSearchReference>[0],
    folderKey: string,
  ) {
    const targetFolder = workspace.folders.value.find((folder) => folder.query_key === folderKey)
    const targetFolderId =
      targetFolder && !targetFolder.is_virtual && !targetFolder.is_system
        ? targetFolder.id
        : undefined

    try {
      const asset = await saveSearchReference(reference, targetFolderId)
      await mutationPublisher.publishMutation({
        kind: 'asset-saved',
        provider: asset.provider,
        sourceID: asset.source_id,
        assetKey: libraryAssetKey(asset.provider, asset.source_id),
        affectedFolderKeys: ['all', folderKey],
        galleryEffect: 'selection-will-reload',
      })
      workspace.selectFolder(folderKey)
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
