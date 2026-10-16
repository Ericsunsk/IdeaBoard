import { type ExternalImageProvider } from '../../../core/contracts/externalImageProviders'
import type { LibraryFolderFacts, LibraryMutationPublisher } from '../../library'
import type {
  LibrarySaveModule,
  LibrarySaveSubmitResult,
  WorkspaceLibraryAsset,
  WorkspaceLibraryFolder,
} from '../../library'
import type { LibrarySearchReference } from '../core/authAction'

export type SearchLibraryCoordinator = LibraryMutationPublisher & {
  folderFacts: LibraryFolderFacts
  /** Shared save module: Search submits through it, never around it. */
  saveModule: LibrarySaveModule
  saveReference: (
    reference: LibrarySearchReference,
    folderID?: string,
    newFolderName?: string,
    successMessage?: string,
  ) => Promise<{ asset: WorkspaceLibraryAsset; createdFolder?: WorkspaceLibraryFolder | undefined }>
  saveReferences: (references: LibrarySearchReference[]) => Promise<LibrarySaveSubmitResult>
  removeReference: (
    provider: ExternalImageProvider,
    sourceID: string,
  ) => Promise<{ asset: WorkspaceLibraryAsset | null }>
  reset: () => void
  dispose: () => void
}
