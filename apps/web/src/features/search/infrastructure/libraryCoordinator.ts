import { type ExternalImageProvider } from '../../../core/contracts/externalImageProviders'
import type { SearchLibraryCoordinator } from '../ports/libraryCoordinatorPort'
import { inject, provide, toValue, type InjectionKey, type MaybeRefOrGetter } from 'vue'
import {
  buildLibraryAssetRemovedOutcome,
  buildLibraryAssetSavedOutcome,
  createLibraryFolderFacts,
  createLibraryMutationPublisher,
  listAllWorkspaceLibraryFolders,
  removeWorkspaceSearchReference,
  requireWorkspaceId,
  saveWorkspaceSearchReference,
  type WorkspaceLibraryAsset,
  type WorkspaceLibraryFolder,
} from '../../library'
import type { LibrarySearchReference } from '../domain/authAction'

export type SearchLibraryCoordinatorOptions = {
  workspaceId: MaybeRefOrGetter<string | null | undefined>
}

/**
 * The Search screen's one coordination boundary for Library Folder facts and
 * mutations. Library still owns its resource operations and asset gallery.
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

  function resolveWorkspaceId(): string {
    return requireWorkspaceId(toValue(options.workspaceId))
  }

  async function saveReference(
    reference: LibrarySearchReference,
    folderID?: string,
    newFolderName?: string,
  ): Promise<{ asset: WorkspaceLibraryAsset; createdFolder?: WorkspaceLibraryFolder | undefined }> {
    // The committed save result owns the actual target: `affectedFolderKeys`
    // must come from `result` (asset + created folder), never from the input
    // `folderID`. `galleryEffect` stays a Search-surface policy.
    const result = await saveWorkspaceSearchReference(
      resolveWorkspaceId(),
      reference,
      folderID,
      newFolderName,
    )
    await mutationPublisher.publishMutation({
      ...buildLibraryAssetSavedOutcome(result.asset, 'reload-current'),
      affectedFolderKeys: result.affectedFolderKeys,
    })
    return {
      asset: result.asset,
      ...(result.createdFolder == null ? {} : { createdFolder: result.createdFolder }),
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
        { provider, sourceId: sourceID, folderId: undefined },
        'reload-current',
      ),
    )
    return { asset: null }
  }

  async function saveReferences(references: LibrarySearchReference[]) {
    const workspaceId = resolveWorkspaceId()
    const results = await Promise.allSettled(
      references.map((reference) => saveWorkspaceSearchReference(workspaceId, reference)),
    )
    const outcomes = results.flatMap((result) =>
      result.status === 'fulfilled'
        ? [
            {
              ...buildLibraryAssetSavedOutcome(result.value.asset, 'reload-current'),
              affectedFolderKeys: result.value.affectedFolderKeys,
            },
          ]
        : [],
    )
    await mutationPublisher.publishMutations(outcomes)
    return results
  }

  return {
    folderFacts,
    saveReference,
    saveReferences,
    removeReference,
    publishMutation: mutationPublisher.publishMutation,
    publishMutations: mutationPublisher.publishMutations,
    subscribe: mutationPublisher.subscribe,
    reset: folderFacts.reset,
    dispose: folderFacts.dispose,
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
