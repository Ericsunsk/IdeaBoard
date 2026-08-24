import type { Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import { recordCanvasInteractionCount } from '../utils/canvasInteractionMetrics'
import { measureCanvasPerf } from '../utils/canvasPerfProbe'
import { toCanvasSemanticEdges } from './canvasEdgeProjection'
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

export type CanvasVueFlowGraphRemoveNodesOptions = CanvasVueFlowGraphCommandOptions & {
  removeConnectedEdges?: boolean
  removeChildren?: boolean
}

export type UseCanvasVueFlowGraphAdapterOptions = CanvasVueFlowGraphCommandOptions & {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  actions?: CanvasVueFlowGraphActions
  projectEdgesForActions?: (edges: Edge[]) => Edge[]
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
  return { nodes, edges: toCanvasSemanticEdges(edges) }
}

function readNodeTargetId(target: NodeTarget): string {
  return typeof target === 'string' ? target : target.id
}

function readEdgeTargetId(target: EdgeTarget): string {
  return typeof target === 'string' ? target : target.id
}

export function useCanvasVueFlowGraphAdapter(options: UseCanvasVueFlowGraphAdapterOptions) {
  const projectEdgesForActions = (edges: Edge[]) => options.projectEdgesForActions?.(edges) ?? edges

  function readGraph(): CanvasVueFlowGraph {
    return createGraph(options.nodes.value, options.edges.value)
  }

  function syncNodes(nextNodes: Node[]) {
    if (nextNodes !== options.nodes.value) recordCanvasInteractionCount('nodeArrayReplacements')
    options.nodes.value = nextNodes
  }

  function syncEdges(nextEdges: Edge[]) {
    const semanticEdges = toCanvasSemanticEdges(nextEdges)
    if (semanticEdges !== options.edges.value) recordCanvasInteractionCount('edgeArrayReplacements')
    options.edges.value = semanticEdges
  }

  function projectGraph<T>(nodes: number, edges: number, task: () => T): T {
    return measureCanvasPerf('vue-flow.projection', { nodes, edges }, task)
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
    const semanticGraph = createGraph(nextGraph.nodes, nextGraph.edges)
    return commandRunner.run({
      command: 'replaceGraph',
      nextGraph: semanticGraph,
      commit: () =>
        projectGraph(semanticGraph.nodes.length, semanticGraph.edges.length, () => {
          if (semanticGraph.nodes !== options.nodes.value) {
            options.actions?.setNodes?.(semanticGraph.nodes)
            syncNodes(semanticGraph.nodes)
          }
          if (semanticGraph.edges !== options.edges.value) {
            options.actions?.setEdges?.(projectEdgesForActions(semanticGraph.edges))
            syncEdges(semanticGraph.edges)
          }
        }),
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
      commit: () =>
        projectGraph(nextNodes.length, options.edges.value.length, () => {
          options.actions?.setNodes?.(nextNodes)
          syncNodes(nextNodes)
        }),
      commandOptions,
    })
  }

  function replaceEdges(
    edgesOrResolver: Edge[] | ((edges: Edge[]) => Edge[]),
    commandOptions?: CanvasVueFlowGraphCommandOptions,
  ): CanvasVueFlowGraphCommandResult {
    const nextEdges = toCanvasSemanticEdges(resolveValue(edgesOrResolver, options.edges.value))
    return commandRunner.run({
      command: 'replaceEdges',
      nextGraph: createGraph(options.nodes.value, nextEdges),
      commit: () =>
        projectGraph(options.nodes.value.length, nextEdges.length, () => {
          options.actions?.setEdges?.(projectEdgesForActions(nextEdges))
          syncEdges(nextEdges)
        }),
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
      commit: () =>
        projectGraph(nextNodes.length, options.edges.value.length, () => {
          options.actions?.addNodes?.(nodesToAdd.length === 1 ? nodesToAdd[0] : nodesToAdd)
          syncNodes(nextNodes)
        }),
      commandOptions,
    })
  }

  function addEdges(
    edgesOrResolver: EdgeInputResolver,
    commandOptions?: CanvasVueFlowGraphCommandOptions,
  ): CanvasVueFlowGraphCommandResult {
    const edgesToAdd = toCanvasSemanticEdges(
      toArray(resolveValue(edgesOrResolver, options.edges.value)),
    )
    if (edgesToAdd.length === 0) {
      return commandRunner.noOp('addEdges', commandOptions)
    }
    const nextEdges = [...options.edges.value, ...edgesToAdd]
    return commandRunner.run({
      command: 'addEdges',
      nextGraph: createGraph(options.nodes.value, nextEdges),
      commit: () =>
        projectGraph(options.nodes.value.length, nextEdges.length, () => {
          const projectedEdges = projectEdgesForActions(edgesToAdd)
          options.actions?.addEdges?.(
            projectedEdges.length === 1 ? projectedEdges[0] : projectedEdges,
          )
          syncEdges(nextEdges)
        }),
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
      commit: () =>
        projectGraph(nextNodes.length, nextEdges.length, () => {
          options.actions?.removeNodes?.(
            Array.from(targetIds),
            commandOptions.removeConnectedEdges ?? false,
            commandOptions.removeChildren,
          )
          syncNodes(nextNodes)
          if (nextEdges !== options.edges.value) syncEdges(nextEdges)
        }),
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
      commit: () =>
        projectGraph(options.nodes.value.length, nextEdges.length, () => {
          options.actions?.removeEdges?.(Array.from(targetIds))
          syncEdges(nextEdges)
        }),
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
      commit: () =>
        projectGraph(nextNodes.length, options.edges.value.length, () => {
          options.actions?.updateNode?.(nodeId, update, {
            replace: commandOptions.replace ?? false,
          })
          syncNodes(nextNodes)
        }),
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
    ...nativeChangeAdapter,
  }
}

export type CanvasVueFlowGraphAdapter = ReturnType<typeof useCanvasVueFlowGraphAdapter>
