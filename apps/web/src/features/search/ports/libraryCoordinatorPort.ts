import type { LibraryFolderFacts, LibraryMutationPublisher } from '../../library'
import type { LibraryAssetView } from '../../library'
import type { LibrarySearchReference } from '../domain/authAction'

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
