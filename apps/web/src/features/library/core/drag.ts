import type { PointerDropTargets } from '../../../core/interaction/pointerDropTargets'
import type { CanvasLibraryImportDescriptor } from './transfer'
import type { LibrarySearchReference } from './validators'

export type LibraryDragPayload =
  | { kind: 'reference'; reference: LibrarySearchReference }
  | { kind: 'asset'; asset: CanvasLibraryImportDescriptor }

export type LibraryDropTargets = PointerDropTargets<LibraryDragPayload>
