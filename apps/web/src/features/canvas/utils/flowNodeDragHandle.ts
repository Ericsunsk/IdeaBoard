import type { FlowNodeKind } from '../types/flowTypes'

export const FLOW_NODE_DRAG_HANDLE = [
  '.workflow-node-header',
  '.flow-canvas-node__drag-surface',
].join(', ')

export function resolveFlowNodeDragHandle(_kind: FlowNodeKind): string {
  void _kind
  return FLOW_NODE_DRAG_HANDLE
}
