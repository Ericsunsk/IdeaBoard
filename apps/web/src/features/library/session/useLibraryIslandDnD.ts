import { resolveLibrarySaveFolderId, type LibrarySaveModule } from './librarySaveModule'
import type { LibrarySearchReference } from '../core/validators'
import type { LibraryDragPayload } from '../core/drag'
import { LIBRARY_ALL_FOLDER_KEY, type useLibraryWorkspace } from './useLibraryWorkspace'

export interface LibraryIslandDnDOptions {
  workspace: ReturnType<typeof useLibraryWorkspace>
  saveModule: LibrarySaveModule
}

/**
 * Island drop routing only. Reference drops resolve the target folder
 * (hovered pill > selected folder > unfiled) and submit to the shared save
 * module, which owns upload, idempotent retry, mutation publication, and
 * notifications. Asset drops stay folder moves through the workspace.
 */
export function useLibraryIslandDnD(options: LibraryIslandDnDOptions) {
  const { workspace, saveModule } = options

  async function receiveDrop(payload: LibraryDragPayload, folderKey: string) {
    if (payload.kind === 'reference') return receiveReference(payload.reference, folderKey)
    if (folderKey === LIBRARY_ALL_FOLDER_KEY) return
    if (!workspace.folders.value.some((folder) => folder.id === folderKey)) return
    await workspace.moveSelectedAsset(payload.asset.assetId, folderKey)
  }

  async function receiveReference(reference: LibrarySearchReference, folderKey?: string) {
    const resolved = resolveLibrarySaveFolderId(folderKey, workspace.selectedFolder.value, (key) =>
      workspace.folders.value.some((folder) => folder.id === key),
    )
    if (resolved.kind === 'invalid') return
    const workspaceId = workspace.workspaceId.value
    const result = await saveModule.submit(
      { source: 'external', reference },
      {
        ...(resolved.folderId == null ? {} : { folderId: resolved.folderId }),
        galleryEffect: 'selection-will-reload',
      },
    )
    if (result.status === 'abandoned') return
    const saved = result.items[0]
    // The committed save owns the target: select its actual folder through
    // the caller's own selection change. A workspace switch in flight skips
    // the selection instead of touching the new scope.
    if (saved?.status === 'saved' && workspace.workspaceId.value === workspaceId) {
      workspace.selectFolder(saved.asset?.folderId ?? LIBRARY_ALL_FOLDER_KEY)
    }
  }

  return { receiveDrop, receiveReference }
}
