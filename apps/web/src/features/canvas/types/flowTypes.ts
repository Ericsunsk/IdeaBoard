import type { CanvasConnectionNodeType } from '../../canvas-board/domain'

export type FlowNodeKind = CanvasConnectionNodeType

export interface FlowNodeData {
  kind: FlowNodeKind
  nodeType?: CanvasConnectionNodeType
  title: string
  summary?: string
  snapshotNode?: unknown
  /** Runtime-only visual continuity while a persisted Library import is active. */
  libraryImportPreviewUrl?: string
}
