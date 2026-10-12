import type { CanvasGraphGroupId, CanvasGraphNodeId } from '../../core/graph'

export type CanvasSelectionGroupAction =
  | {
      kind: 'group'
      label: string
      nodeIds: CanvasGraphNodeId[]
    }
  | {
      kind: 'ungroup'
      label: string
      groupId: CanvasGraphGroupId
      nodeIds: CanvasGraphNodeId[]
    }
