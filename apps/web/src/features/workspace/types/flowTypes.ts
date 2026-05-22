import type { CanvasConnectionNodeType } from '../types/canvasNodes'
import type { EditNodeMode } from './editNodeModes'

export type FlowNodeKind = CanvasConnectionNodeType

export interface FlowNodeData {
  kind: FlowNodeKind
  nodeType?: CanvasConnectionNodeType
  title: string
  summary?: string
  mode?: EditNodeMode
  snapshotNode?: unknown
}
