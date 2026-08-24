import type { Actions, Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import type { CanvasFlowViewport } from './useCanvasViewportRuntime'

type ValueRef<T> = {
  readonly value: T
}

export type CanvasVueFlowGraphActions = Partial<
  Pick<
    Actions,
    | 'setNodes'
    | 'setEdges'
    | 'addNodes'
    | 'addEdges'
    | 'removeNodes'
    | 'removeEdges'
    | 'updateNode'
    | 'applyNodeChanges'
    | 'applyEdgeChanges'
  >
>

export type CanvasVueFlowAdapter = {
  viewport: Readonly<Ref<CanvasFlowViewport>>
  getViewport: () => CanvasFlowViewport
  setViewport: (
    viewport: CanvasFlowViewport,
    options?: { duration?: number; interpolate?: 'smooth' },
  ) => Promise<unknown> | unknown
  setCenter: (
    x: number,
    y: number,
    options?: { duration?: number; zoom?: number; interpolate?: 'smooth' },
  ) => Promise<unknown> | unknown
  zoomIn: () => Promise<unknown> | unknown
  zoomOut: () => Promise<unknown> | unknown
  panBy: (delta: { x: number; y: number }) => boolean
  updateNodeInternals: (nodeIds: string[]) => void
  screenToFlowCoordinate: (point: { x: number; y: number }) => { x: number; y: number }
  getSelectedElements: ValueRef<Array<Node | Edge>>
  removeSelectedElements: (elements: Array<Node | Edge>) => void
  selectOnlyNode: (nodeId: string) => boolean
  vueFlowGraphActions: CanvasVueFlowGraphActions
}
