import type { LibraryFolderFacts, LibraryMutationPublisher } from '../../library'
import type { WorkspaceLibraryAsset } from '../../library'
import type { LibrarySearchReference } from '../domain/authAction'

export type SearchLibraryCoordinator = LibraryMutationPublisher & {
  folderFacts: LibraryFolderFacts
  saveReference: (
    reference: LibrarySearchReference,
    folderID?: string,
    newFolderName?: string,
  ) => Promise<WorkspaceLibraryAsset>
  saveReferences: (
    references: LibrarySearchReference[],
  ) => Promise<PromiseSettledResult<WorkspaceLibraryAsset>[]>
  removeReference: (provider: 'unsplash' | 'pexels', sourceID: string) => Promise<void>
  reset: () => void
  dispose: () => void
}
