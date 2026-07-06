import {
  applyEdgeChanges as applyVueFlowEdgeChanges,
  applyNodeChanges as applyVueFlowNodeChanges,
  type Actions,
  type Edge,
  type EdgeChange,
  type Node,
  type NodeChange,
} from '@vue-flow/core'
import type { Ref } from 'vue'
import { recordCanvasInteractionCount } from '../utils/canvasInteractionMetrics'
import type {
  CanvasVueFlowGraphCommandOptions,
  CanvasVueFlowGraphCommandResult,
  CanvasVueFlowGraphCommandRunner,
} from './canvasVueFlowGraphCommandRunner'

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

type NativeNodeChangeState = Node & {
  selected?: boolean
  dragging?: boolean
}
type NativeEdgeChangeState = Edge & {
  selected?: boolean
}

type CanvasVueFlowNativeChangeAdapterOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  actions?: CanvasVueFlowGraphActions
  syncNodes: (nextNodes: Node[]) => void
  syncEdges: (nextEdges: Edge[]) => void
  commandRunner: CanvasVueFlowGraphCommandRunner
}

function createNodeLookup(nodes: Node[]): Map<string, Node> {
  return new Map(nodes.map((node) => [node.id, node]))
}

function createEdgeLookup(edges: Edge[]): Map<string, Edge> {
  return new Map(edges.map((edge) => [edge.id, edge]))
}

function isSamePosition(
  left: { x: number; y: number } | undefined,
  right: { x: number; y: number } | undefined,
): boolean {
  return left?.x === right?.x && left?.y === right?.y
}

function hasApplicableNodeChange(change: NodeChange, nodeLookup: Map<string, Node>): boolean {
  if (change.type === 'add') {
    return !nodeLookup.has(change.item.id)
  }
  if (change.type === 'remove') {
    return nodeLookup.has(change.id)
  }

  const node = nodeLookup.get(change.id)
  if (!node) return false

  if (change.type === 'select') {
    return (node as NativeNodeChangeState).selected !== change.selected
  }
  if (change.type === 'position') {
    return (
      (change.position !== undefined && !isSamePosition(node.position, change.position)) ||
      (change.dragging !== undefined &&
        (node as NativeNodeChangeState).dragging !== change.dragging)
    )
  }
  if (change.type === 'dimensions') {
    return (
      change.dimensions !== undefined ||
      change.handleBounds !== undefined ||
      change.resizing !== undefined ||
      change.updateStyle === true
    )
  }

  return true
}

function hasApplicableEdgeChange(change: EdgeChange, edgeLookup: Map<string, Edge>): boolean {
  if (change.type === 'add') {
    return !edgeLookup.has(change.item.id)
  }
  if (change.type === 'remove') {
    return edgeLookup.has(change.id)
  }

  const edge = edgeLookup.get(change.id)
  if (!edge) return false

  if (change.type === 'select') {
    return (edge as NativeEdgeChangeState).selected !== change.selected
  }

  return true
}

function hasApplicableNodeChanges(changes: NodeChange[], currentNodes: Node[]): boolean {
  const nodeLookup = createNodeLookup(currentNodes)
  return changes.some((change) => hasApplicableNodeChange(change, nodeLookup))
}

function hasApplicableEdgeChanges(changes: EdgeChange[], currentEdges: Edge[]): boolean {
  const edgeLookup = createEdgeLookup(currentEdges)
  return changes.some((change) => hasApplicableEdgeChange(change, edgeLookup))
}

export function createCanvasVueFlowNativeChangeAdapter(
  options: CanvasVueFlowNativeChangeAdapterOptions,
) {
  function applyNativeNodeChanges(changes: NodeChange[]) {
    recordCanvasInteractionCount('nativeNodeChangeBatches')
    recordCanvasInteractionCount('nativeNodeChangeCount', changes.length)
    const actionResult = options.actions?.applyNodeChanges?.(changes)
    if (Array.isArray(actionResult)) {
      options.syncNodes(actionResult as Node[])
      return
    }
    const nextNodes = applyVueFlowNodeChanges(
      changes,
      options.nodes.value as Parameters<typeof applyVueFlowNodeChanges>[1],
    ) as unknown as Node[]
    if (nextNodes !== options.nodes.value) options.syncNodes(nextNodes)
  }

  function projectNodeChanges(changes: NodeChange[]) {
    recordCanvasInteractionCount('nativeNodeChangeBatches')
    recordCanvasInteractionCount('nativeNodeChangeCount', changes.length)
    options.actions?.applyNodeChanges?.(changes)
  }

  function syncProjectedNodeChanges(
    changes: NodeChange[],
    commandOptions?: CanvasVueFlowGraphCommandOptions,
  ): CanvasVueFlowGraphCommandResult {
    if (changes.length === 0 || !hasApplicableNodeChanges(changes, options.nodes.value)) {
      return options.commandRunner.noOp('applyNodeChanges', commandOptions)
    }
    return options.commandRunner.runNative(
      'applyNodeChanges',
      () => {
        const nextNodes = applyVueFlowNodeChanges(
          changes,
          options.nodes.value as Parameters<typeof applyVueFlowNodeChanges>[1],
        ) as unknown as Node[]
        if (nextNodes !== options.nodes.value) options.syncNodes(nextNodes)
      },
      commandOptions,
    )
  }

  function applyNativeEdgeChanges(changes: EdgeChange[]) {
    recordCanvasInteractionCount('nativeEdgeChangeBatches')
    recordCanvasInteractionCount('nativeEdgeChangeCount', changes.length)
    const actionResult = options.actions?.applyEdgeChanges?.(changes)
    if (Array.isArray(actionResult)) {
      options.syncEdges(actionResult as Edge[])
      return
    }
    const nextEdges = applyVueFlowEdgeChanges(
      changes,
      options.edges.value as Parameters<typeof applyVueFlowEdgeChanges>[1],
    ) as unknown as Edge[]
    if (nextEdges !== options.edges.value) options.syncEdges(nextEdges)
  }

  function applyNodeChanges(
    changes: NodeChange[],
    commandOptions?: CanvasVueFlowGraphCommandOptions,
  ): CanvasVueFlowGraphCommandResult {
    if (changes.length === 0 || !hasApplicableNodeChanges(changes, options.nodes.value)) {
      return options.commandRunner.noOp('applyNodeChanges', commandOptions)
    }
    return options.commandRunner.runNative(
      'applyNodeChanges',
      () => applyNativeNodeChanges(changes),
      commandOptions,
    )
  }

  function applyEdgeChanges(
    changes: EdgeChange[],
    commandOptions?: CanvasVueFlowGraphCommandOptions,
  ): CanvasVueFlowGraphCommandResult {
    if (changes.length === 0 || !hasApplicableEdgeChanges(changes, options.edges.value)) {
      return options.commandRunner.noOp('applyEdgeChanges', commandOptions)
    }
    return options.commandRunner.runNative(
      'applyEdgeChanges',
      () => applyNativeEdgeChanges(changes),
      commandOptions,
    )
  }

  function applyGraphChanges(
    changes: { nodes?: NodeChange[]; edges?: EdgeChange[] },
    commandOptions?: CanvasVueFlowGraphCommandOptions,
  ): CanvasVueFlowGraphCommandResult {
    const nodeChanges = changes.nodes ?? []
    const edgeChanges = changes.edges ?? []
    const shouldApplyNodeChanges =
      nodeChanges.length > 0 && hasApplicableNodeChanges(nodeChanges, options.nodes.value)
    const shouldApplyEdgeChanges =
      edgeChanges.length > 0 && hasApplicableEdgeChanges(edgeChanges, options.edges.value)
    if (!shouldApplyNodeChanges && !shouldApplyEdgeChanges) {
      return options.commandRunner.noOp('applyGraphChanges', commandOptions)
    }

    return options.commandRunner.runNative(
      'applyGraphChanges',
      () => {
        if (shouldApplyNodeChanges) applyNativeNodeChanges(nodeChanges)
        if (shouldApplyEdgeChanges) applyNativeEdgeChanges(edgeChanges)
      },
      commandOptions,
    )
  }

  return {
    applyGraphChanges,
    projectNodeChanges,
    syncProjectedNodeChanges,
    applyNodeChanges,
    applyEdgeChanges,
  }
}
