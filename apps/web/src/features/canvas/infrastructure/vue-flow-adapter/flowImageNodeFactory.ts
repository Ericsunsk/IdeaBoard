import type { Node } from '@vue-flow/core'
import type { ImageNodeItem } from '../../../board'
import type { FlowNodeData } from '../../domain/nodes/flowTypes.ts'
import type { FlowAdapterNode } from './flowSnapshotAdapter'
import { resolveFlowNodeDragHandle } from './flowNodeDragHandle'

export function createImageFlowNode(item: ImageNodeItem): Node {
  const data: FlowNodeData = {
    kind: 'image',
    nodeType: 'image',
    title: `Image #${item.id}`,
    summary: 'Canvas image node',
    snapshotNode: item,
  }
  const node: FlowAdapterNode = {
    id: `image-${item.id}`,
    type: 'canvas',
    dragHandle: resolveFlowNodeDragHandle('image'),
    position: { x: item.x, y: item.y },
    data,
  }
  return node as Node
}
