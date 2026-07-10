import type { Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import { recordCanvasInteractionCount } from '../utils/canvasInteractionMetrics'
import {
  createCanvasVueFlowNativeChangeAdapter,
  type CanvasVueFlowGraphActions,
} from './canvasVueFlowNativeChangeAdapter'
import { createGraphCommandRunner } from './canvasVueFlowGraphCommandRunner'
import type {
  CanvasVueFlowGraph,
  CanvasVueFlowGraphCommandOptions,
  CanvasVueFlowGraphCommandResult,
} from './canvasVueFlowGraphCommandRunner'

export type { CanvasVueFlowGraphActions } from './canvasVueFlowNativeChangeAdapter'
export type {
  CanvasVueFlowGraph,
  CanvasVueFlowGraphCommandContext,
  CanvasVueFlowGraphCommandOptions,
  CanvasVueFlowGraphCommandResult,
} from './canvasVueFlowGraphCommandRunner'

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

  const commandRunner = createGraphCommandRunner({
    beforeCommand: options.beforeCommand,
    afterCommand: options.afterCommand,
    readGraph,
  })

  const nativeChangeAdapter = createCanvasVueFlowNativeChangeAdapter({
    nodes: options.nodes,
    edges: options.edges,
    actions: options.actions,
    syncNodes,
    syncEdges,
    commandRunner,
  })

  function replaceGraph(
    nextGraph: CanvasVueFlowGraph,
    commandOptions?: CanvasVueFlowGraphCommandOptions,
  ): CanvasVueFlowGraphCommandResult {
    return commandRunner.run({
      command: 'replaceGraph',
      nextGraph,
      commit: () => {
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
    })
  }

  function replaceNodes(
    nodesOrResolver: Node[] | ((nodes: Node[]) => Node[]),
    commandOptions?: CanvasVueFlowGraphCommandOptions,
  ): CanvasVueFlowGraphCommandResult {
    const nextNodes = resolveValue(nodesOrResolver, options.nodes.value)
    return commandRunner.run({
      command: 'replaceNodes',
      nextGraph: createGraph(nextNodes, options.edges.value),
      commit: () => {
        options.actions?.setNodes?.(nextNodes)
        syncNodes(nextNodes)
      },
      commandOptions,
    })
  }

  function replaceEdges(
    edgesOrResolver: Edge[] | ((edges: Edge[]) => Edge[]),
    commandOptions?: CanvasVueFlowGraphCommandOptions,
  ): CanvasVueFlowGraphCommandResult {
    const nextEdges = resolveValue(edgesOrResolver, options.edges.value)
    return commandRunner.run({
      command: 'replaceEdges',
      nextGraph: createGraph(options.nodes.value, nextEdges),
      commit: () => {
        options.actions?.setEdges?.(nextEdges)
        syncEdges(nextEdges)
      },
      commandOptions,
    })
  }

  function addNodes(
    nodesOrResolver: NodeInputResolver,
    commandOptions?: CanvasVueFlowGraphCommandOptions,
  ): CanvasVueFlowGraphCommandResult {
    const nodesToAdd = toArray(resolveValue(nodesOrResolver, options.nodes.value))
    if (nodesToAdd.length === 0) {
      return commandRunner.noOp('addNodes', commandOptions)
    }
    const nextNodes = [...options.nodes.value, ...nodesToAdd]
    return commandRunner.run({
      command: 'addNodes',
      nextGraph: createGraph(nextNodes, options.edges.value),
      commit: () => {
        options.actions?.addNodes?.(nodesToAdd.length === 1 ? nodesToAdd[0] : nodesToAdd)
        syncNodes(nextNodes)
      },
      commandOptions,
    })
  }

  function addEdges(
    edgesOrResolver: EdgeInputResolver,
    commandOptions?: CanvasVueFlowGraphCommandOptions,
  ): CanvasVueFlowGraphCommandResult {
    const edgesToAdd = toArray(resolveValue(edgesOrResolver, options.edges.value))
    if (edgesToAdd.length === 0) {
      return commandRunner.noOp('addEdges', commandOptions)
    }
    const nextEdges = [...options.edges.value, ...edgesToAdd]
    return commandRunner.run({
      command: 'addEdges',
      nextGraph: createGraph(options.nodes.value, nextEdges),
      commit: () => {
        options.actions?.addEdges?.(edgesToAdd.length === 1 ? edgesToAdd[0] : edgesToAdd)
        syncEdges(nextEdges)
      },
      commandOptions,
    })
  }

  function removeNodes(
    targetsOrResolver: NodeTargetResolver,
    commandOptions: CanvasVueFlowGraphRemoveNodesOptions = {},
  ): CanvasVueFlowGraphCommandResult {
    const targetIds = new Set(
      toArray(resolveValue(targetsOrResolver, options.nodes.value)).map(readNodeTargetId),
    )
    if (targetIds.size === 0) {
      return commandRunner.noOp('removeNodes', commandOptions)
    }
    const nextNodes = options.nodes.value.filter((node) => !targetIds.has(node.id))
    if (nextNodes.length === options.nodes.value.length) {
      return commandRunner.noOp('removeNodes', commandOptions)
    }
    const nextEdges =
      commandOptions.removeConnectedEdges === true
        ? options.edges.value.filter(
            (edge) => !targetIds.has(edge.source) && !targetIds.has(edge.target),
          )
        : options.edges.value
    return commandRunner.run({
      command: 'removeNodes',
      nextGraph: createGraph(nextNodes, nextEdges),
      commit: () => {
        options.actions?.removeNodes?.(
          Array.from(targetIds),
          commandOptions.removeConnectedEdges ?? false,
          commandOptions.removeChildren,
        )
        syncNodes(nextNodes)
        if (nextEdges !== options.edges.value) syncEdges(nextEdges)
      },
      commandOptions,
    })
  }

  function removeEdges(
    targetsOrResolver: EdgeTargetResolver,
    commandOptions: CanvasVueFlowGraphCommandOptions = {},
  ): CanvasVueFlowGraphCommandResult {
    const targetIds = new Set(
      toArray(resolveValue(targetsOrResolver, options.edges.value)).map(readEdgeTargetId),
    )
    if (targetIds.size === 0) {
      return commandRunner.noOp('removeEdges', commandOptions)
    }
    const nextEdges = options.edges.value.filter((edge) => !targetIds.has(edge.id))
    if (nextEdges.length === options.edges.value.length) {
      return commandRunner.noOp('removeEdges', commandOptions)
    }
    return commandRunner.run({
      command: 'removeEdges',
      nextGraph: createGraph(options.nodes.value, nextEdges),
      commit: () => {
        options.actions?.removeEdges?.(Array.from(targetIds))
        syncEdges(nextEdges)
      },
      commandOptions,
    })
  }

  function updateNode(
    nodeId: string,
    update: NodeUpdate,
    commandOptions: CanvasVueFlowGraphNodeUpdateOptions = {},
  ): CanvasVueFlowGraphCommandResult {
    const nodeIndex = options.nodes.value.findIndex((node) => node.id === nodeId)
    if (nodeIndex < 0) {
      return commandRunner.noOp('updateNode', commandOptions)
    }
    const currentNode = options.nodes.value[nodeIndex]
    const patch = typeof update === 'function' ? update(currentNode) : update
    const nextNode = commandOptions.replace
      ? ({ id: currentNode.id, ...patch } as Node)
      : ({ ...currentNode, ...patch } as Node)
    const nextNodes = options.nodes.value.slice()
    nextNodes[nodeIndex] = nextNode
    return commandRunner.run({
      command: 'updateNode',
      nextGraph: createGraph(nextNodes, options.edges.value),
      commit: () => {
        options.actions?.updateNode?.(nodeId, update, { replace: commandOptions.replace ?? false })
        syncNodes(nextNodes)
      },
      commandOptions,
    })
  }

  function updateEdge(
    edgeId: string,
    update: EdgeUpdate,
    commandOptions: CanvasVueFlowGraphEdgeUpdateOptions = {},
  ): CanvasVueFlowGraphCommandResult {
    const edgeIndex = options.edges.value.findIndex((edge) => edge.id === edgeId)
    if (edgeIndex < 0) {
      return commandRunner.noOp('updateEdge', commandOptions)
    }
    const currentEdge = options.edges.value[edgeIndex]
    const patch = typeof update === 'function' ? update(currentEdge) : update
    const nextEdge = commandOptions.replace
      ? ({ id: currentEdge.id, ...patch } as Edge)
      : ({ ...currentEdge, ...patch } as Edge)
    const nextEdges = options.edges.value.slice()
    nextEdges[edgeIndex] = nextEdge
    return commandRunner.run({
      command: 'updateEdge',
      nextGraph: createGraph(options.nodes.value, nextEdges),
      commit: () => {
        options.actions?.setEdges?.(nextEdges)
        syncEdges(nextEdges)
      },
      commandOptions,
    })
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
    ...nativeChangeAdapter,
  }
}

export type CanvasVueFlowGraphAdapter = ReturnType<typeof useCanvasVueFlowGraphAdapter>
