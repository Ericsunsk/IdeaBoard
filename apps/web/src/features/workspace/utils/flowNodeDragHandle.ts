import type { FlowNodeKind } from '../types/flowTypes'

export const FLOW_NODE_DRAG_HANDLE = [
  '.flow-canvas-node__drag-zone',
  '.flow-canvas-node__drag-surface',
].join(', ')

export function resolveFlowNodeDragHandle(_kind: FlowNodeKind): string {
  return FLOW_NODE_DRAG_HANDLE
}
