import { type ExternalImageProvider } from '../../../../core/contracts/externalImageProviders'
import type { SearchLibraryCoordinator } from '../../ports/libraryCoordinatorPort'
import { inject, provide, toValue, type InjectionKey, type MaybeRefOrGetter } from 'vue'
import {
  buildLibraryAssetRemovedOutcome,
  createLibraryFolderFacts,
  createLibraryMutationPublisher,
  createLibrarySaveModule,
  listAllWorkspaceLibraryFolders,
  removeWorkspaceSearchReference,
  requireWorkspaceId,
  type LibrarySaveModule,
  type LibrarySaveSubmitResult,
  type WorkspaceLibraryAsset,
  type WorkspaceLibraryFolder,
} from '../../../library'
import { useAppToast } from '../../../../core/composables/useAppToast'
import type { LibrarySearchReference } from '../../core/authAction'

export type SearchLibraryCoordinatorOptions = {
  workspaceId: MaybeRefOrGetter<string | null | undefined>
  /** Test seam: inject a stub save module; production always builds its own. */
  saveModule?: LibrarySaveModule | undefined
}

/**
 * The Search screen's one coordination boundary for Library saves, Folder
 * facts, and mutations. Saves delegate to the shared Library save module
 * (upload-once, idempotent retry, batch publication and notifications); this
 * adapter only shapes Search inputs and keeps the Search-facing interface.
 * The workspace id resolves per call from `features/workspace` selection so
 * workspace switches never reuse a stale id.
 */
export function createSearchLibraryCoordinator(
  options: SearchLibraryCoordinatorOptions,
): SearchLibraryCoordinator {
  const folderFacts = createLibraryFolderFacts(() =>
    listAllWorkspaceLibraryFolders(requireWorkspaceId(toValue(options.workspaceId))),
  )
  const mutationPublisher = createLibraryMutationPublisher(folderFacts)
  const toast = useAppToast()
  const saveModule =
    options.saveModule ??
    createLibrarySaveModule({
      workspaceId: options.workspaceId,
      mutationPublisher,
      notify: toast,
    })

  function resolveWorkspaceId(): string {
    return requireWorkspaceId(toValue(options.workspaceId))
  }

  async function saveReference(
    reference: LibrarySearchReference,
    folderID?: string,
    newFolderName?: string,
    successMessage?: string,
  ): Promise<{ asset: WorkspaceLibraryAsset; createdFolder?: WorkspaceLibraryFolder | undefined }> {
    // The committed save result owns the actual target: callers read the
    // stored asset (and created folder) back, never their input `folderID`.
    // `galleryEffect` stays a Search-surface policy.
    resolveWorkspaceId()
    const result = await saveModule.submit(
      {
        source: 'external',
        reference,
      },
      {
        ...(folderID == null ? {} : { folderId: folderID }),
        ...(newFolderName == null ? {} : { newFolderName }),
        galleryEffect: 'reload-current',
        ...(successMessage == null ? {} : { successMessage }),
      },
    )
    const saved = result.items[0]
    if (!saved || saved.status !== 'saved' || !saved.asset) {
      throw new Error(saved?.error ?? 'Could not save the asset')
    }
    const createdFolderId = result.createdFolderId
    const createdFolder =
      createdFolderId == null
        ? undefined
        : folderFacts.folders.value.find((folder) => folder.id === createdFolderId)
    return {
      asset: saved.asset,
      ...(createdFolder == null ? {} : { createdFolder }),
    }
  }

  async function removeReference(
    provider: ExternalImageProvider,
    sourceID: string,
  ): Promise<{ asset: WorkspaceLibraryAsset | null }> {
    // Removal resolves the owned asset first: the outcome carries the actual
    // deleted folders. An unresolved reference deletes nothing and affects
    // only the `all` aggregate.
    const removed = await removeWorkspaceSearchReference(resolveWorkspaceId(), provider, sourceID)
    if (removed) {
      await mutationPublisher.publishMutation({
        ...buildLibraryAssetRemovedOutcome(removed.asset, 'reload-current'),
        affectedFolderKeys: removed.affectedFolderKeys,
      })
      return { asset: removed.asset }
    }
    await mutationPublisher.publishMutation(
      buildLibraryAssetRemovedOutcome(
        { source: 'external', provider, sourceId: sourceID, folderId: undefined },
        'reload-current',
      ),
    )
    return { asset: null }
  }

  async function saveReferences(
    references: LibrarySearchReference[],
  ): Promise<LibrarySaveSubmitResult> {
    resolveWorkspaceId()
    return saveModule.submit(
      references.map((reference) => ({ source: 'external' as const, reference })),
      { galleryEffect: 'reload-current' },
    )
  }

  function reset(): void {
    saveModule.clear()
    folderFacts.reset()
  }

  function dispose(): void {
    saveModule.dispose()
    folderFacts.dispose()
  }

  return {
    folderFacts,
    saveModule,
    saveReference,
    saveReferences,
    removeReference,
    publishMutation: mutationPublisher.publishMutation,
    publishMutations: mutationPublisher.publishMutations,
    subscribe: mutationPublisher.subscribe,
    reset,
    dispose,
  }
}

const searchLibraryCoordinatorKey: InjectionKey<SearchLibraryCoordinator> = Symbol(
  'search:library-coordinator',
)

export function provideSearchLibraryCoordinator(coordinator: SearchLibraryCoordinator) {
  provide(searchLibraryCoordinatorKey, coordinator)
}

export function useSearchLibraryCoordinator(): SearchLibraryCoordinator {
  const coordinator = inject(searchLibraryCoordinatorKey)
  if (!coordinator) throw new Error('Search library coordinator is not installed')
  return coordinator
}
