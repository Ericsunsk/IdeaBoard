import type { Edge, EdgeChange, Node, NodeChange } from '@vue-flow/core'

export type CanvasVueFlowNativeChangeClassification = 'no-op' | 'visual-only' | 'graph-affecting'

type NativeNodeChangeState = Node &
  Partial<Pick<import('@vue-flow/core').GraphNode, 'selected' | 'dragging' | 'resizing'>> & {
    dimensions?: unknown
    handleBounds?: unknown
  }
type NativeEdgeChangeState = Edge & Partial<Pick<import('@vue-flow/core').GraphEdge, 'selected'>>

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

function isSameNativeValue(left: unknown, right: unknown): boolean {
  if (Object.is(left, right)) return true
  try {
    return JSON.stringify(left) === JSON.stringify(right)
  } catch {
    return false
  }
}

function mergeClassifications(
  current: CanvasVueFlowNativeChangeClassification,
  next: CanvasVueFlowNativeChangeClassification,
): CanvasVueFlowNativeChangeClassification {
  if (current === 'graph-affecting' || next === 'graph-affecting') return 'graph-affecting'
  if (current === 'visual-only' || next === 'visual-only') return 'visual-only'
  return 'no-op'
}

function classifyDimensionChange(
  change: Extract<NodeChange, { type: 'dimensions' }>,
  node: NativeNodeChangeState,
): CanvasVueFlowNativeChangeClassification {
  const didChange =
    (change.dimensions !== undefined && !isSameNativeValue(node.dimensions, change.dimensions)) ||
    (change.handleBounds !== undefined &&
      !isSameNativeValue(node.handleBounds, change.handleBounds)) ||
    (change.resizing !== undefined && node.resizing !== change.resizing) ||
    change.updateStyle === true
  return didChange ? 'visual-only' : 'no-op'
}

function classifyNodeChange(
  change: NodeChange,
  nodeLookup: Map<string, Node>,
): CanvasVueFlowNativeChangeClassification {
  if (change.type === 'add') {
    return nodeLookup.has(change.item.id) ? 'no-op' : 'graph-affecting'
  }
  if (change.type === 'remove') {
    return nodeLookup.has(change.id) ? 'graph-affecting' : 'no-op'
  }

  const node = nodeLookup.get(change.id)
  if (!node) return 'no-op'
  const nativeNode = node as NativeNodeChangeState

  if (change.type === 'select') {
    return nativeNode.selected === change.selected ? 'no-op' : 'visual-only'
  }
  if (change.type === 'position') {
    if (change.position !== undefined && !isSamePosition(node.position, change.position)) {
      return 'graph-affecting'
    }
    return change.dragging !== undefined && nativeNode.dragging !== change.dragging
      ? 'visual-only'
      : 'no-op'
  }
  if (change.type === 'dimensions') {
    return classifyDimensionChange(change, nativeNode)
  }

  return 'graph-affecting'
}

function classifyEdgeChange(
  change: EdgeChange,
  edgeLookup: Map<string, Edge>,
): CanvasVueFlowNativeChangeClassification {
  if (change.type === 'add') {
    return edgeLookup.has(change.item.id) ? 'no-op' : 'graph-affecting'
  }
  if (change.type === 'remove') {
    return edgeLookup.has(change.id) ? 'graph-affecting' : 'no-op'
  }

  const edge = edgeLookup.get(change.id)
  if (!edge) return 'no-op'

  if (change.type === 'select') {
    return (edge as NativeEdgeChangeState).selected === change.selected ? 'no-op' : 'visual-only'
  }

  return 'graph-affecting'
}

export function classifyVueFlowNativeNodeChanges(
  changes: NodeChange[],
  currentNodes: Node[],
): CanvasVueFlowNativeChangeClassification {
  const nodeLookup = createNodeLookup(currentNodes)
  return changes.reduce<CanvasVueFlowNativeChangeClassification>(
    (classification, change) =>
      mergeClassifications(classification, classifyNodeChange(change, nodeLookup)),
    'no-op',
  )
}

export function classifyVueFlowNativeEdgeChanges(
  changes: EdgeChange[],
  currentEdges: Edge[],
): CanvasVueFlowNativeChangeClassification {
  const edgeLookup = createEdgeLookup(currentEdges)
  return changes.reduce<CanvasVueFlowNativeChangeClassification>(
    (classification, change) =>
      mergeClassifications(classification, classifyEdgeChange(change, edgeLookup)),
    'no-op',
  )
}

export function mergeVueFlowNativeChangeClassifications(
  ...classifications: CanvasVueFlowNativeChangeClassification[]
): CanvasVueFlowNativeChangeClassification {
  return classifications.reduce<CanvasVueFlowNativeChangeClassification>(
    mergeClassifications,
    'no-op',
  )
}
