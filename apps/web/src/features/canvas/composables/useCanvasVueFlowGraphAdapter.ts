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
import {
  recordCanvasGraphCommand,
  recordCanvasInteractionCount,
} from '../utils/canvasInteractionMetrics'

export type CanvasVueFlowGraph = {
  nodes: Node[]
  edges: Edge[]
}

export type CanvasVueFlowGraphCommandName =
  | 'replaceGraph'
  | 'replaceNodes'
  | 'replaceEdges'
  | 'addNodes'
  | 'addEdges'
  | 'removeNodes'
  | 'removeEdges'
  | 'updateNode'
  | 'updateEdge'
  | 'applyGraphChanges'
  | 'applyNodeChanges'
  | 'applyEdgeChanges'

export type CanvasVueFlowGraphCommandContext = {
  command: CanvasVueFlowGraphCommandName
  before: CanvasVueFlowGraph
  next: CanvasVueFlowGraph
  metadata?: Record<string, unknown>
}

export type CanvasVueFlowGraphCommandResult = {
  command: CanvasVueFlowGraphCommandName
  before: CanvasVueFlowGraph
  after: CanvasVueFlowGraph
  didChange: boolean
  metadata?: Record<string, unknown>
}

type BeforeCommandHook = (context: CanvasVueFlowGraphCommandContext) => unknown
type AfterCommandHook = (result: CanvasVueFlowGraphCommandResult, beforeHookValue: unknown) => void

export type CanvasVueFlowGraphCommandOptions = {
  metadata?: Record<string, unknown>
  beforeCommand?: BeforeCommandHook
  afterCommand?: AfterCommandHook
}

export type CanvasVueFlowGraphNodeUpdateOptions = CanvasVueFlowGraphCommandOptions & {
  replace?: boolean
}

export type CanvasVueFlowGraphEdgeUpdateOptions = CanvasVueFlowGraphCommandOptions & {
  replace?: boolean
}

export type CanvasVueFlowGraphRemoveNodesOptions = CanvasVueFlowGraphCommandOptions & {
  removeConnectedEdges?: boolean
  removeChildren?: boolean
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

export type UseCanvasVueFlowGraphAdapterOptions = CanvasVueFlowGraphCommandOptions & {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  actions?: CanvasVueFlowGraphActions
}

type NodeInput = Node | Node[]
type EdgeInput = Edge | Edge[]
type NodeInputResolver = NodeInput | ((nodes: Node[]) => NodeInput)
type EdgeInputResolver = EdgeInput | ((edges: Edge[]) => EdgeInput)
type NodeTarget = string | Node
type EdgeTarget = string | Edge
type NodeTargetInput = NodeTarget | NodeTarget[]
type EdgeTargetInput = EdgeTarget | EdgeTarget[]
type NodeTargetResolver = NodeTargetInput | ((nodes: Node[]) => NodeTargetInput)
type EdgeTargetResolver = EdgeTargetInput | ((edges: Edge[]) => EdgeTargetInput)
type NodeUpdate = Partial<Node> | ((node: Node) => Partial<Node>)
type EdgeUpdate = Partial<Edge> | ((edge: Edge) => Partial<Edge>)
type NativeNodeChangeState = Node & {
  selected?: boolean
  dragging?: boolean
}
type NativeEdgeChangeState = Edge & {
  selected?: boolean
}

function resolveValue<TCurrent, TResult>(
  valueOrResolver: TResult | ((current: TCurrent) => TResult),
  current: TCurrent,
): TResult {
  if (typeof valueOrResolver === 'function') {
    return (valueOrResolver as (current: TCurrent) => TResult)(current)
  }
  return valueOrResolver
}

function toArray<T>(value: T | T[]): T[] {
  return Array.isArray(value) ? value : [value]
}

function createGraph(nodes: Node[], edges: Edge[]): CanvasVueFlowGraph {
  return { nodes, edges }
}

function readNodeTargetId(target: NodeTarget): string {
  return typeof target === 'string' ? target : target.id
}

function readEdgeTargetId(target: EdgeTarget): string {
  return typeof target === 'string' ? target : target.id
}

function isSameGraph(left: CanvasVueFlowGraph, right: CanvasVueFlowGraph): boolean {
  return left.nodes === right.nodes && left.edges === right.edges
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

export function useCanvasVueFlowGraphAdapter(options: UseCanvasVueFlowGraphAdapterOptions) {
  function readGraph(): CanvasVueFlowGraph {
    return createGraph(options.nodes.value, options.edges.value)
  }

  function syncNodes(nextNodes: Node[]) {
    if (nextNodes !== options.nodes.value) recordCanvasInteractionCount('nodeArrayReplacements')
    options.nodes.value = nextNodes
  }

  function syncEdges(nextEdges: Edge[]) {
    if (nextEdges !== options.edges.value) recordCanvasInteractionCount('edgeArrayReplacements')
    options.edges.value = nextEdges
  }

  function runCommand(
    command: CanvasVueFlowGraphCommandName,
    nextGraph: CanvasVueFlowGraph,
    commit: () => void,
    commandOptions: CanvasVueFlowGraphCommandOptions = {},
  ): CanvasVueFlowGraphCommandResult {
    const before = readGraph()
    if (isSameGraph(before, nextGraph)) {
      return {
        command,
        before,
        after: before,
        didChange: false,
        metadata: commandOptions.metadata,
      }
    }

    const context: CanvasVueFlowGraphCommandContext = {
      command,
      before,
      next: nextGraph,
      metadata: commandOptions.metadata,
    }
    const globalBeforeValue = options.beforeCommand?.(context)
    const commandBeforeValue = commandOptions.beforeCommand?.(context)

    commit()
    recordCanvasGraphCommand(command)

    const result: CanvasVueFlowGraphCommandResult = {
      command,
      before,
      after: readGraph(),
      didChange: true,
      metadata: commandOptions.metadata,
    }
    options.afterCommand?.(result, globalBeforeValue)
    commandOptions.afterCommand?.(result, commandBeforeValue)
    return result
  }

  function createNoOpCommandResult(
    command: CanvasVueFlowGraphCommandName,
    commandOptions: CanvasVueFlowGraphCommandOptions = {},
  ): CanvasVueFlowGraphCommandResult {
    const before = readGraph()
    return {
      command,
      before,
      after: before,
      didChange: false,
      metadata: commandOptions.metadata,
    }
  }

  function runNativeChangeCommand(
    command: CanvasVueFlowGraphCommandName,
    commit: () => void,
    commandOptions: CanvasVueFlowGraphCommandOptions = {},
  ): CanvasVueFlowGraphCommandResult {
    const before = readGraph()
    const context: CanvasVueFlowGraphCommandContext = {
      command,
      before,
      next: before,
      metadata: commandOptions.metadata,
    }
    const globalBeforeValue = options.beforeCommand?.(context)
    const commandBeforeValue = commandOptions.beforeCommand?.(context)

    commit()
    recordCanvasGraphCommand(command)

    const result: CanvasVueFlowGraphCommandResult = {
      command,
      before,
      after: readGraph(),
      didChange: true,
      metadata: commandOptions.metadata,
    }
    options.afterCommand?.(result, globalBeforeValue)
    commandOptions.afterCommand?.(result, commandBeforeValue)
    return result
  }

  function applyNativeNodeChanges(changes: NodeChange[]) {
    recordCanvasInteractionCount('nativeNodeChangeBatches')
    recordCanvasInteractionCount('nativeNodeChangeCount', changes.length)
    const actionResult = options.actions?.applyNodeChanges?.(changes)
    if (Array.isArray(actionResult)) {
      syncNodes(actionResult as Node[])
      return
    }
    const nextNodes = applyVueFlowNodeChanges(
      changes,
      options.nodes.value as Parameters<typeof applyVueFlowNodeChanges>[1],
    ) as unknown as Node[]
    if (nextNodes !== options.nodes.value) syncNodes(nextNodes)
  }

  function applyNativeEdgeChanges(changes: EdgeChange[]) {
    recordCanvasInteractionCount('nativeEdgeChangeBatches')
    recordCanvasInteractionCount('nativeEdgeChangeCount', changes.length)
    const actionResult = options.actions?.applyEdgeChanges?.(changes)
    if (Array.isArray(actionResult)) {
      syncEdges(actionResult as Edge[])
      return
    }
    const nextEdges = applyVueFlowEdgeChanges(
      changes,
      options.edges.value as Parameters<typeof applyVueFlowEdgeChanges>[1],
    ) as unknown as Edge[]
    if (nextEdges !== options.edges.value) syncEdges(nextEdges)
  }

  function replaceGraph(
    nextGraph: CanvasVueFlowGraph,
    commandOptions?: CanvasVueFlowGraphCommandOptions,
  ): CanvasVueFlowGraphCommandResult {
    return runCommand(
      'replaceGraph',
      nextGraph,
      () => {
        if (nextGraph.nodes !== options.nodes.value) {
          options.actions?.setNodes?.(nextGraph.nodes)
          syncNodes(nextGraph.nodes)
        }
        if (nextGraph.edges !== options.edges.value) {
          options.actions?.setEdges?.(nextGraph.edges)
          syncEdges(nextGraph.edges)
        }
      },
      commandOptions,
    )
  }

  function replaceNodes(
    nodesOrResolver: Node[] | ((nodes: Node[]) => Node[]),
    commandOptions?: CanvasVueFlowGraphCommandOptions,
  ): CanvasVueFlowGraphCommandResult {
    const nextNodes = resolveValue(nodesOrResolver, options.nodes.value)
    return runCommand(
      'replaceNodes',
      createGraph(nextNodes, options.edges.value),
      () => {
        options.actions?.setNodes?.(nextNodes)
        syncNodes(nextNodes)
      },
      commandOptions,
    )
  }

  function replaceEdges(
    edgesOrResolver: Edge[] | ((edges: Edge[]) => Edge[]),
    commandOptions?: CanvasVueFlowGraphCommandOptions,
  ): CanvasVueFlowGraphCommandResult {
    const nextEdges = resolveValue(edgesOrResolver, options.edges.value)
    return runCommand(
      'replaceEdges',
      createGraph(options.nodes.value, nextEdges),
      () => {
        options.actions?.setEdges?.(nextEdges)
        syncEdges(nextEdges)
      },
      commandOptions,
    )
  }

  function addNodes(
    nodesOrResolver: NodeInputResolver,
    commandOptions?: CanvasVueFlowGraphCommandOptions,
  ): CanvasVueFlowGraphCommandResult {
    const nodesToAdd = toArray(resolveValue(nodesOrResolver, options.nodes.value))
    if (nodesToAdd.length === 0) {
      const before = readGraph()
      return {
        command: 'addNodes',
        before,
        after: before,
        didChange: false,
        metadata: commandOptions?.metadata,
      }
    }
    const nextNodes = [...options.nodes.value, ...nodesToAdd]
    return runCommand(
      'addNodes',
      createGraph(nextNodes, options.edges.value),
      () => {
        options.actions?.addNodes?.(nodesToAdd.length === 1 ? nodesToAdd[0] : nodesToAdd)
        syncNodes(nextNodes)
      },
      commandOptions,
    )
  }

  function addEdges(
    edgesOrResolver: EdgeInputResolver,
    commandOptions?: CanvasVueFlowGraphCommandOptions,
  ): CanvasVueFlowGraphCommandResult {
    const edgesToAdd = toArray(resolveValue(edgesOrResolver, options.edges.value))
    if (edgesToAdd.length === 0) {
      const before = readGraph()
      return {
        command: 'addEdges',
        before,
        after: before,
        didChange: false,
        metadata: commandOptions?.metadata,
      }
    }
    const nextEdges = [...options.edges.value, ...edgesToAdd]
    return runCommand(
      'addEdges',
      createGraph(options.nodes.value, nextEdges),
      () => {
        options.actions?.addEdges?.(edgesToAdd.length === 1 ? edgesToAdd[0] : edgesToAdd)
        syncEdges(nextEdges)
      },
      commandOptions,
    )
  }

  function removeNodes(
    targetsOrResolver: NodeTargetResolver,
    commandOptions: CanvasVueFlowGraphRemoveNodesOptions = {},
  ): CanvasVueFlowGraphCommandResult {
    const targetIds = new Set(
      toArray(resolveValue(targetsOrResolver, options.nodes.value)).map(readNodeTargetId),
    )
    if (targetIds.size === 0) {
      const before = readGraph()
      return {
        command: 'removeNodes',
        before,
        after: before,
        didChange: false,
        metadata: commandOptions.metadata,
      }
    }
    const nextNodes = options.nodes.value.filter((node) => !targetIds.has(node.id))
    if (nextNodes.length === options.nodes.value.length) {
      const before = readGraph()
      return {
        command: 'removeNodes',
        before,
        after: before,
        didChange: false,
        metadata: commandOptions.metadata,
      }
    }
    const nextEdges =
      commandOptions.removeConnectedEdges === true
        ? options.edges.value.filter(
            (edge) => !targetIds.has(edge.source) && !targetIds.has(edge.target),
          )
        : options.edges.value
    return runCommand(
      'removeNodes',
      createGraph(nextNodes, nextEdges),
      () => {
        options.actions?.removeNodes?.(
          Array.from(targetIds),
          commandOptions.removeConnectedEdges ?? false,
          commandOptions.removeChildren,
        )
        syncNodes(nextNodes)
        if (nextEdges !== options.edges.value) syncEdges(nextEdges)
      },
      commandOptions,
    )
  }

  function removeEdges(
    targetsOrResolver: EdgeTargetResolver,
    commandOptions: CanvasVueFlowGraphCommandOptions = {},
  ): CanvasVueFlowGraphCommandResult {
    const targetIds = new Set(
      toArray(resolveValue(targetsOrResolver, options.edges.value)).map(readEdgeTargetId),
    )
    if (targetIds.size === 0) {
      const before = readGraph()
      return {
        command: 'removeEdges',
        before,
        after: before,
        didChange: false,
        metadata: commandOptions.metadata,
      }
    }
    const nextEdges = options.edges.value.filter((edge) => !targetIds.has(edge.id))
    if (nextEdges.length === options.edges.value.length) {
      const before = readGraph()
      return {
        command: 'removeEdges',
        before,
        after: before,
        didChange: false,
        metadata: commandOptions.metadata,
      }
    }
    return runCommand(
      'removeEdges',
      createGraph(options.nodes.value, nextEdges),
      () => {
        options.actions?.removeEdges?.(Array.from(targetIds))
        syncEdges(nextEdges)
      },
      commandOptions,
    )
  }

  function updateNode(
    nodeId: string,
    update: NodeUpdate,
    commandOptions: CanvasVueFlowGraphNodeUpdateOptions = {},
  ): CanvasVueFlowGraphCommandResult {
    const nodeIndex = options.nodes.value.findIndex((node) => node.id === nodeId)
    if (nodeIndex < 0) {
      const before = readGraph()
      return {
        command: 'updateNode',
        before,
        after: before,
        didChange: false,
        metadata: commandOptions.metadata,
      }
    }
    const currentNode = options.nodes.value[nodeIndex]
    const patch = typeof update === 'function' ? update(currentNode) : update
    const nextNode = commandOptions.replace
      ? ({ id: currentNode.id, ...patch } as Node)
      : ({ ...currentNode, ...patch } as Node)
    const nextNodes = options.nodes.value.slice()
    nextNodes[nodeIndex] = nextNode
    return runCommand(
      'updateNode',
      createGraph(nextNodes, options.edges.value),
      () => {
        options.actions?.updateNode?.(nodeId, update, { replace: commandOptions.replace ?? false })
        syncNodes(nextNodes)
      },
      commandOptions,
    )
  }

  function updateEdge(
    edgeId: string,
    update: EdgeUpdate,
    commandOptions: CanvasVueFlowGraphEdgeUpdateOptions = {},
  ): CanvasVueFlowGraphCommandResult {
    const edgeIndex = options.edges.value.findIndex((edge) => edge.id === edgeId)
    if (edgeIndex < 0) {
      const before = readGraph()
      return {
        command: 'updateEdge',
        before,
        after: before,
        didChange: false,
        metadata: commandOptions.metadata,
      }
    }
    const currentEdge = options.edges.value[edgeIndex]
    const patch = typeof update === 'function' ? update(currentEdge) : update
    const nextEdge = commandOptions.replace
      ? ({ id: currentEdge.id, ...patch } as Edge)
      : ({ ...currentEdge, ...patch } as Edge)
    const nextEdges = options.edges.value.slice()
    nextEdges[edgeIndex] = nextEdge
    return runCommand(
      'updateEdge',
      createGraph(options.nodes.value, nextEdges),
      () => {
        options.actions?.setEdges?.(nextEdges)
        syncEdges(nextEdges)
      },
      commandOptions,
    )
  }

  function applyNodeChanges(
    changes: NodeChange[],
    commandOptions?: CanvasVueFlowGraphCommandOptions,
  ): CanvasVueFlowGraphCommandResult {
    if (changes.length === 0 || !hasApplicableNodeChanges(changes, options.nodes.value)) {
      return createNoOpCommandResult('applyNodeChanges', commandOptions)
    }
    return runNativeChangeCommand(
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
      return createNoOpCommandResult('applyEdgeChanges', commandOptions)
    }
    return runNativeChangeCommand(
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
      return createNoOpCommandResult('applyGraphChanges', commandOptions)
    }

    return runNativeChangeCommand(
      'applyGraphChanges',
      () => {
        if (shouldApplyNodeChanges) applyNativeNodeChanges(nodeChanges)
        if (shouldApplyEdgeChanges) applyNativeEdgeChanges(edgeChanges)
      },
      commandOptions,
    )
  }

  return {
    replaceGraph,
    replaceNodes,
    replaceEdges,
    addNodes,
    addEdges,
    removeNodes,
    removeEdges,
    updateNode,
    updateEdge,
    applyGraphChanges,
    applyNodeChanges,
    applyEdgeChanges,
  }
}

export type CanvasVueFlowGraphAdapter = ReturnType<typeof useCanvasVueFlowGraphAdapter>
