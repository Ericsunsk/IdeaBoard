import type { SearchLibraryCoordinator } from '../ports/libraryCoordinatorPort'
import { inject, provide, type InjectionKey } from 'vue'
import {
  createLibraryFolderFacts,
  createLibraryMutationPublisher,
} from '../../library/libraryFolderFacts'
import {
  removeSearchReference,
  saveSearchReference,
  libraryAssetKey,
  type LibrarySearchReference,
} from '../../library/libraryResource'

/**
 * The Search screen's one coordination boundary for Library Folder facts and
 * mutations. Library still owns its resource operations and asset gallery.
 */
export function createSearchLibraryCoordinator(): SearchLibraryCoordinator {
  const folderFacts = createLibraryFolderFacts()
  const mutationPublisher = createLibraryMutationPublisher(folderFacts)

  async function saveReference(
    reference: LibrarySearchReference,
    folderID?: number,
    newFolderName?: string,
  ) {
    const asset = await saveSearchReference(reference, folderID, newFolderName)
    await mutationPublisher.publishMutation({
      kind: 'asset-saved',
      provider: asset.provider,
      sourceID: asset.source_id,
      assetKey: libraryAssetKey(asset.provider, asset.source_id),
      affectedFolderKeys: [
        'all',
        ...(folderID ? [String(folderID)] : newFolderName ? [] : ['quick-saves']),
      ],
      galleryEffect: 'reload-current',
    })
    return asset
  }

  async function removeReference(provider: 'unsplash' | 'pexels', sourceID: string) {
    await removeSearchReference(provider, sourceID)
    await mutationPublisher.publishMutation({
      kind: 'asset-removed',
      provider,
      sourceID,
      assetKey: libraryAssetKey(provider, sourceID),
      affectedFolderKeys: ['all', 'quick-saves'],
      galleryEffect: 'reload-current',
    })
  }

  async function saveReferences(references: LibrarySearchReference[]) {
    const results = await Promise.allSettled(
      references.map((reference) => saveSearchReference(reference)),
    )
    const outcomes = results.flatMap((result) =>
      result.status === 'fulfilled'
        ? [
            {
              kind: 'asset-saved' as const,
              provider: result.value.provider,
              sourceID: result.value.source_id,
              assetKey: libraryAssetKey(result.value.provider, result.value.source_id),
              affectedFolderKeys: ['all', 'quick-saves'],
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
