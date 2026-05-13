import type { CanvasConnectionNodeType, CanvasConnectionPort } from '../types/canvasNodes'

export type FlowNodeKind = CanvasConnectionNodeType | 'message'

export interface FlowNodeData {
  kind: FlowNodeKind
  nodeType?: CanvasConnectionNodeType
  title: string
  summary?: string
  mode?: 'partial-edit' | 'bg-swap' | 'face-swap' | 'hd-upscale' | 'canvas-extend'
  inputPorts?: CanvasConnectionPort[]
  outputPorts?: CanvasConnectionPort[]
  snapshotNode?: unknown
}
