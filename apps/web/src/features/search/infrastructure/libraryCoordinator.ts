import { type ExternalImageProvider } from '../../../shared/domain/externalImageProviders'
import type { SearchLibraryCoordinator } from '../ports/libraryCoordinatorPort'
import { inject, provide, toValue, type InjectionKey, type MaybeRefOrGetter } from 'vue'
import {
  createLibraryFolderFacts,
  createLibraryMutationPublisher,
  libraryAssetKey,
  listAllWorkspaceLibraryFolders,
  removeWorkspaceSearchReference,
  requireWorkspaceId,
  saveWorkspaceSearchReference,
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
  ) {
    const asset = await saveWorkspaceSearchReference(
      resolveWorkspaceId(),
      reference,
      folderID,
      newFolderName,
    )
    await mutationPublisher.publishMutation({
      kind: 'asset-saved',
      provider: asset.provider,
      sourceID: asset.sourceId,
      assetKey: libraryAssetKey(asset.provider, asset.sourceId),
      affectedFolderKeys: ['all', ...(folderID ? [folderID] : [])],
      galleryEffect: 'reload-current',
    })
    return asset
  }

  async function removeReference(provider: ExternalImageProvider, sourceID: string) {
    await removeWorkspaceSearchReference(resolveWorkspaceId(), provider, sourceID)
    await mutationPublisher.publishMutation({
      kind: 'asset-removed',
      provider,
      sourceID,
      assetKey: libraryAssetKey(provider, sourceID),
      affectedFolderKeys: ['all'],
      galleryEffect: 'reload-current',
    })
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
              kind: 'asset-saved' as const,
              provider: result.value.provider,
              sourceID: result.value.sourceId,
              assetKey: libraryAssetKey(result.value.provider, result.value.sourceId),
              affectedFolderKeys: ['all'],
              galleryEffect: 'reload-current' as const,
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
