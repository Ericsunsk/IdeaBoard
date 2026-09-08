import type { LibraryFolderFacts, LibraryMutationPublisher } from '../../library/libraryFolderFacts'
import type { LibraryAssetView } from '../../library/libraryResource'
import type { LibrarySearchReference } from '../domain/searchAuthAction'

export type SearchLibraryCoordinator = LibraryMutationPublisher & {
  folderFacts: LibraryFolderFacts
  saveReference: (
    reference: LibrarySearchReference,
    folderID?: number,
    newFolderName?: string,
  ) => Promise<LibraryAssetView>
  saveReferences: (
    references: LibrarySearchReference[],
  ) => Promise<PromiseSettledResult<LibraryAssetView>[]>
  removeReference: (provider: 'unsplash' | 'pexels', sourceID: string) => Promise<void>
  reset: () => void
  dispose: () => void
}
