import type { CanvasConnectionNodeType } from '../../../board/ports/canvas'

export type FlowNodeKind = CanvasConnectionNodeType

/**
 * Browser-only continuity for a local image while its managed Media upload is pending.
 * This deliberately stays outside the Canvas Board snapshot contract.
 */
export type FlowLocalUploadState = {
  previewUrl: string
  failureMessage?: string
}

export interface FlowNodeData {
  kind: FlowNodeKind
  nodeType?: CanvasConnectionNodeType
  title: string
  summary?: string
  snapshotNode?: unknown
  /** Runtime-only visual continuity while a persisted Library import is active. */
  libraryImportPreviewUrl?: string
  /** Runtime-only visual continuity while a local file is materialized into Media. */
  localUpload?: FlowLocalUploadState
}
