import type { Actions, Edge, EdgeChange, Node, NodeChange } from '@vue-flow/core'
import type { Ref } from 'vue'

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
type NodeChangeTarget = Node & {
  selected?: boolean
  dragging?: boolean
  dimensions?: unknown
  handleBounds?: unknown
  resizing?: boolean
}
type EdgeChangeTarget = Edge & {
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

function cloneNodesForChanges(nodes: Node[]): Node[] {
  return nodes.map((node) => ({ ...node }))
}

function cloneEdgesForChanges(edges: Edge[]): Edge[] {
  return edges.map((edge) => ({ ...edge }))
}

function applyNodeChangesToPlainNodes(changes: NodeChange[], currentNodes: Node[]): Node[] {
  const nextNodes = cloneNodesForChanges(currentNodes)

  for (const change of changes) {
    if (change.type === 'add') {
      if (!nextNodes.some((node) => node.id === change.item.id)) nextNodes.push(change.item as Node)
      continue
    }
    if (change.type === 'remove') {
      const index = nextNodes.findIndex((node) => node.id === change.id)
      if (index >= 0) nextNodes.splice(index, 1)
    }
  }

  for (const change of changes) {
    if (change.type === 'add' || change.type === 'remove') continue
    const node = nextNodes.find((item) => item.id === change.id)
    if (!node) continue
    const mutableNode = node as NodeChangeTarget
    if (change.type === 'select') {
      mutableNode.selected = change.selected
      continue
    }
    if (change.type === 'position') {
      mutableNode.position = change.position
      if (typeof change.dragging !== 'undefined') mutableNode.dragging = change.dragging
      continue
    }
    if (change.type === 'dimensions') {
      if (typeof change.dimensions !== 'undefined') mutableNode.dimensions = change.dimensions
      if (typeof change.handleBounds !== 'undefined') {
        mutableNode.handleBounds = change.handleBounds
      }
      if (typeof change.resizing !== 'undefined') mutableNode.resizing = change.resizing
      if (change.updateStyle && change.dimensions) {
        const dimensionsStyle = {
          width: `${change.dimensions.width}px`,
          height: `${change.dimensions.height}px`,
        }
        if (typeof node.style === 'function') {
          const style = node.style
          node.style = ((params) => ({
            ...style(params),
            ...dimensionsStyle,
          })) as Node['style']
        } else {
          node.style = {
            ...((node.style ?? {}) as Record<string, unknown>),
            ...dimensionsStyle,
          } as Node['style']
        }
      }
    }
  }

  return nextNodes
}

function applyEdgeChangesToPlainEdges(changes: EdgeChange[], currentEdges: Edge[]): Edge[] {
  const nextEdges = cloneEdgesForChanges(currentEdges)

  for (const change of changes) {
    if (change.type === 'add') {
      if (!nextEdges.some((edge) => edge.id === change.item.id)) nextEdges.push(change.item as Edge)
      continue
    }
    if (change.type === 'remove') {
      const index = nextEdges.findIndex((edge) => edge.id === change.id)
      if (index >= 0) nextEdges.splice(index, 1)
    }
  }

  for (const change of changes) {
    if (change.type === 'add' || change.type === 'remove') continue
    const edge = nextEdges.find((item) => item.id === change.id)
    if (!edge) continue
    if (change.type === 'select') {
      ;(edge as EdgeChangeTarget).selected = change.selected
    }
  }

  return nextEdges
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

export function useCanvasVueFlowGraphAdapter(options: UseCanvasVueFlowGraphAdapterOptions) {
  function readGraph(): CanvasVueFlowGraph {
    return createGraph(options.nodes.value, options.edges.value)
  }

  function syncNodes(nextNodes: Node[]) {
    options.nodes.value = nextNodes
  }

  function syncEdges(nextEdges: Edge[]) {
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
    if (changes.length === 0) {
      const before = readGraph()
      return {
        command: 'applyNodeChanges',
        before,
        after: before,
        didChange: false,
        metadata: commandOptions?.metadata,
      }
    }
    const nextNodes = applyNodeChangesToPlainNodes(changes, options.nodes.value)
    return runCommand(
      'applyNodeChanges',
      createGraph(nextNodes, options.edges.value),
      () => {
        options.actions?.applyNodeChanges?.(changes)
        syncNodes(nextNodes)
      },
      commandOptions,
    )
  }

  function applyEdgeChanges(
    changes: EdgeChange[],
    commandOptions?: CanvasVueFlowGraphCommandOptions,
  ): CanvasVueFlowGraphCommandResult {
    if (changes.length === 0) {
      const before = readGraph()
      return {
        command: 'applyEdgeChanges',
        before,
        after: before,
        didChange: false,
        metadata: commandOptions?.metadata,
      }
    }
    const nextEdges = applyEdgeChangesToPlainEdges(changes, options.edges.value)
    return runCommand(
      'applyEdgeChanges',
      createGraph(options.nodes.value, nextEdges),
      () => {
        options.actions?.applyEdgeChanges?.(changes)
        syncEdges(nextEdges)
      },
      commandOptions,
    )
  }

  function applyGraphChanges(
    changes: { nodes?: NodeChange[]; edges?: EdgeChange[] },
    commandOptions?: CanvasVueFlowGraphCommandOptions,
  ): CanvasVueFlowGraphCommandResult {
    const nodeChanges = changes.nodes ?? []
    const edgeChanges = changes.edges ?? []
    if (nodeChanges.length === 0 && edgeChanges.length === 0) {
      const before = readGraph()
      return {
        command: 'applyGraphChanges',
        before,
        after: before,
        didChange: false,
        metadata: commandOptions?.metadata,
      }
    }
    const nextNodes =
      nodeChanges.length > 0
        ? applyNodeChangesToPlainNodes(nodeChanges, options.nodes.value)
        : options.nodes.value
    const nextEdges =
      edgeChanges.length > 0
        ? applyEdgeChangesToPlainEdges(edgeChanges, options.edges.value)
        : options.edges.value

    return runCommand(
      'applyGraphChanges',
      createGraph(nextNodes, nextEdges),
      () => {
        if (nodeChanges.length > 0) {
          options.actions?.applyNodeChanges?.(nodeChanges)
          syncNodes(nextNodes)
        }
        if (edgeChanges.length > 0) {
          options.actions?.applyEdgeChanges?.(edgeChanges)
          syncEdges(nextEdges)
        }
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
