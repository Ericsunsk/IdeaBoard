import { type ExternalImageProvider } from '../../../core/contracts/externalImageProviders'
import type { LibraryFolderFacts, LibraryMutationPublisher } from '../../library'
import type {
  WorkspaceLibraryAsset,
  WorkspaceLibraryFolder,
  WorkspaceLibrarySaveResult,
} from '../../library'
import type { LibrarySearchReference } from '../domain/authAction'

export type SearchLibraryCoordinator = LibraryMutationPublisher & {
  folderFacts: LibraryFolderFacts
  saveReference: (
    reference: LibrarySearchReference,
    folderID?: string,
    newFolderName?: string,
  ) => Promise<{ asset: WorkspaceLibraryAsset; createdFolder?: WorkspaceLibraryFolder | undefined }>
  saveReferences: (
    references: LibrarySearchReference[],
  ) => Promise<PromiseSettledResult<WorkspaceLibrarySaveResult>[]>
  removeReference: (
    provider: ExternalImageProvider,
    sourceID: string,
  ) => Promise<{ asset: WorkspaceLibraryAsset | null }>
  reset: () => void
  dispose: () => void
}
