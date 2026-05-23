import type { CanvasConnectionNodeType } from '../types/canvasNodes'

export type FlowNodeKind = CanvasConnectionNodeType

export interface FlowNodeData {
  kind: FlowNodeKind
  nodeType?: CanvasConnectionNodeType
  title: string
  summary?: string
  snapshotNode?: unknown
}
