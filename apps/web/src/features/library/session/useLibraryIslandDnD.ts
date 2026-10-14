import { useI18n } from 'vue-i18n'
import { buildLibraryAssetSavedOutcome, type LibraryMutationPublisher } from './folderFacts'
import { requireWorkspaceId, type LibrarySearchReference } from '../core/validators'
import { saveWorkspaceSearchReference } from './libraryUseCases'
import type { LibraryDragPayload } from '../core/drag'
import { useAppToast } from '../../../core/composables/useAppToast'
import { userFacingErrorCopy } from '../../../core/errors/errorCatalog'
import { LIBRARY_ALL_FOLDER_KEY, type useLibraryWorkspace } from './useLibraryWorkspace'

export interface LibraryIslandDnDOptions {
  workspace: ReturnType<typeof useLibraryWorkspace>
  mutationPublisher: LibraryMutationPublisher
}

export function useLibraryIslandDnD(options: LibraryIslandDnDOptions) {
  const { workspace, mutationPublisher } = options
  const { t } = useI18n()
  const toast = useAppToast()
  async function receiveDrop(payload: LibraryDragPayload, folderKey: string) {
    if (payload.kind === 'reference') return receiveReference(payload.reference, folderKey)
    if (folderKey === LIBRARY_ALL_FOLDER_KEY) return
    if (!workspace.folders.value.some((folder) => folder.id === folderKey)) return
    await workspace.moveSelectedAsset(payload.asset.assetId, folderKey)
  }

  async function receiveReference(reference: LibrarySearchReference, folderKey: string) {
    const targetFolder = workspace.folders.value.find((folder) => folder.id === folderKey)
    const targetFolderId =
      targetFolder && folderKey !== LIBRARY_ALL_FOLDER_KEY ? targetFolder.id : undefined
    if (folderKey !== LIBRARY_ALL_FOLDER_KEY && !targetFolder) return

    try {
      const workspaceId = requireWorkspaceId(workspace.workspaceId.value)
      const result = await saveWorkspaceSearchReference(workspaceId, reference, targetFolderId)
      if (workspace.workspaceId.value !== workspaceId) return
      // The committed save owns the target: publish its actual folders and
      // reload through the caller's own selection change.
      await mutationPublisher.publishMutation({
        ...buildLibraryAssetSavedOutcome(result.asset, 'selection-will-reload'),
        affectedFolderKeys: result.affectedFolderKeys,
      })
      if (workspace.workspaceId.value === workspaceId) {
        workspace.selectFolder(result.asset.folderId ?? LIBRARY_ALL_FOLDER_KEY)
      }
    } catch (error) {
      toast.error(userFacingErrorCopy(error, t('library.errors.saveAssetFailed')))
    }
  }

  return { receiveDrop, receiveReference }
}
