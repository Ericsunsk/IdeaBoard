import type { Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import { recordCanvasInteractionCount } from '../../browser/canvasInteractionMetrics.ts'
import { measureCanvasPerf } from '../../browser/canvasPerfProbe.ts'
import { toCanvasSemanticEdges } from './canvasEdgeProjection'
import {
  createCanvasVueFlowNativeChangeAdapter,
  type CanvasVueFlowGraphActions,
} from '../viewport/canvasVueFlowNativeChangeAdapter'
import { createGraphCommandRunner } from './canvasVueFlowGraphCommandRunner'
import type {
  CanvasVueFlowGraph,
  CanvasVueFlowGraphCommandOptions,
  CanvasVueFlowGraphCommandResult,
} from './canvasVueFlowGraphCommandRunner'

export type { CanvasVueFlowGraphActions } from '../viewport/canvasVueFlowNativeChangeAdapter'
export type {
  CanvasVueFlowGraph,
  CanvasVueFlowGraphCommandOptions,
  CanvasVueFlowGraphCommandResult,
} from './canvasVueFlowGraphCommandRunner'

export type CanvasVueFlowGraphNodeUpdateOptions = CanvasVueFlowGraphCommandOptions & {
  shouldReplace?: boolean
}

export type CanvasVueFlowGraphRemoveNodesOptions = CanvasVueFlowGraphCommandOptions & {
  shouldRemoveConnectedEdges?: boolean
  shouldRemoveChildren?: boolean
}

export type CanvasVueFlowIncrementalProjection = Readonly<{
  nodes?: Node[]
  edges?: Edge[]
  changedNodeIds?: readonly string[]
  changedEdgeIds?: readonly string[]
  nodeMutations?: Readonly<{
    added: readonly Node[]
    updated: readonly Node[]
    removedIds: readonly string[]
  }>
  edgeMutations?: Readonly<{
    added: readonly Edge[]
    updated: readonly Edge[]
    removedIds: readonly string[]
  }>
}>

export type UseCanvasVueFlowGraphAdapterOptions = CanvasVueFlowGraphCommandOptions & {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  actions?: CanvasVueFlowGraphActions | undefined
  projectEdgesForActions?: ((edges: Edge[]) => Edge[]) | undefined
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

/** @externalContract Vue Flow node metadata; initialized is present at runtime but omitted from GraphNode. */
type VueFlowRuntimeNode = Node &
  Partial<Pick<import('@vue-flow/core').GraphNode, 'dragging' | 'resizing'>> & {
    computedPosition?: unknown
    handleBounds?: unknown
    isParent?: boolean
    initialized?: boolean
    events?: unknown
  }

/**
 * Canvas owns semantic graph state; Vue Flow owns measurements and interaction
 * state. Feeding renderer fields back through the controlled `nodes` prop can
 * overwrite a newer internal geometry update with an older snapshot.
 */
function toControlledNode(node: Node): Node {
  const runtimeNode = node as VueFlowRuntimeNode
  if (
    runtimeNode.computedPosition === undefined &&
    runtimeNode.handleBounds === undefined &&
    runtimeNode.isParent === undefined &&
    runtimeNode.resizing === undefined &&
    runtimeNode.dragging === undefined &&
    runtimeNode.initialized === undefined &&
    runtimeNode.events === undefined
  ) {
    return node
  }
  const {
    computedPosition: _computedPosition,
    handleBounds: _handleBounds,
    isParent: _isParent,
    resizing: _resizing,
    dragging: _dragging,
    initialized: _initialized,
    events: _events,
    ...controlledNode
  } = runtimeNode
  return controlledNode as Node
}

function toControlledNodes(nodes: Node[]): Node[] {
  let nextNodes: Node[] | null = null
  nodes.forEach((node, index) => {
    const controlledNode = toControlledNode(node)
    if (controlledNode === node) return
    if (!nextNodes) nextNodes = [...nodes]
    nextNodes[index] = controlledNode
  })
  return nextNodes ?? nodes
}

function readNodeTargetId(target: NodeTarget): string {
  return typeof target === 'string' ? target : target.id
}

function readEdgeTargetId(target: EdgeTarget): string {
  return typeof target === 'string' ? target : target.id
}

type IncrementalProjectionPlan = {
  nodeMutations: CanvasVueFlowIncrementalProjection['nodeMutations']
  hasAddedNodes: boolean
  hasEdgeChanges: boolean
}

export function useCanvasVueFlowGraphAdapter(options: UseCanvasVueFlowGraphAdapterOptions) {
  const projectEdgesForActions = (edges: Edge[]) => options.projectEdgesForActions?.(edges) ?? edges

  function readGraph(): CanvasVueFlowGraph {
    return createGraph(options.nodes.value, options.edges.value)
  }

  function syncNodes(nextNodes: Node[]) {
    const controlledNodes = toControlledNodes(nextNodes)
    if (controlledNodes !== options.nodes.value)
      recordCanvasInteractionCount('nodeArrayReplacements')
    options.nodes.value = controlledNodes
  }

  function syncEdges(nextEdges: Edge[]) {
    const semanticEdges = toCanvasSemanticEdges(nextEdges)
    if (semanticEdges !== options.edges.value) recordCanvasInteractionCount('edgeArrayReplacements')
    options.edges.value = semanticEdges
  }

  function projectGraph<T>(nodes: number, edges: number, task: () => T): T {
    return measureCanvasPerf('vue-flow.projection', { nodes, edges }, task)
  }

  /**
   * Vue Flow nodes contain renderer-owned state (`computedPosition`, handle
   * bounds, drag state, and measurement flags). The canonical Canvas node is
   * only a partial projection of that object, so replacing the runtime node
   * would corrupt Vue Flow's drag/connection state. `updateNode` merges this
   * projection into the existing runtime node and lets Vue Flow's position
   * watcher recompute derived coordinates.
   */
  function updateRendererNodeProjection(node: Node) {
    const controlledNode = toControlledNode(node)
    const isSelected =
      (controlledNode as Node & Partial<Pick<import('@vue-flow/core').GraphNode, 'selected'>>)
        .selected ?? false
    options.actions?.updateNode?.(
      controlledNode.id,
      () =>
        ({
          ...controlledNode,
          selected: isSelected,
        }) satisfies Partial<import('@vue-flow/core').GraphNode>,
      { replace: false },
    )
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

  function fallbackNodeMutations(
    changedNodeIds: readonly string[],
    nextNodes: Node[],
  ): CanvasVueFlowIncrementalProjection['nodeMutations'] {
    return {
      added: [],
      updated: changedNodeIds
        .map((id) => nextNodes.find((node) => node.id === id))
        .filter((node): node is Node => !!node),
      removedIds: [],
    }
  }

  function fallbackEdgeMutations(
    changedEdgeIds: readonly string[],
    nextEdges: Edge[],
  ): CanvasVueFlowIncrementalProjection['edgeMutations'] {
    return {
      added: [],
      updated: changedEdgeIds
        .map((id) => nextEdges.find((edge) => edge.id === id))
        .filter((edge): edge is Edge => !!edge),
      removedIds: [],
    }
  }

  function planProjectionMutations(
    projection: CanvasVueFlowIncrementalProjection,
    nextNodes: Node[] | undefined,
    nextEdges: Edge[] | undefined,
    changedNodeIds: readonly string[],
    changedEdgeIds: readonly string[],
  ): IncrementalProjectionPlan {
    const nodeMutations =
      nextNodes && nextNodes !== options.nodes.value
        ? (projection.nodeMutations ?? fallbackNodeMutations(changedNodeIds, nextNodes))
        : undefined
    const edgeMutations = nextEdges
      ? (projection.edgeMutations ?? fallbackEdgeMutations(changedEdgeIds, nextEdges))
      : undefined
    return {
      nodeMutations,
      hasAddedNodes: (nodeMutations?.added.length ?? 0) > 0,
      hasEdgeChanges:
        !!edgeMutations &&
        (edgeMutations.added.length > 0 ||
          edgeMutations.updated.length > 0 ||
          edgeMutations.removedIds.length > 0 ||
          nextEdges !== options.edges.value),
    }
  }

  function applyProjectedEdgesFirst(
    plan: IncrementalProjectionPlan,
    nextEdges: Edge[] | undefined,
  ): boolean {
    // A renderer projection must not emit edgesChange. Apply an edge update
    // before node removal so edges never reference a missing runtime node.
    // If new nodes are being added, defer the set until those nodes exist.
    if (nextEdges && plan.hasEdgeChanges && !plan.hasAddedNodes) {
      options.actions?.setEdges?.(projectEdgesForActions(nextEdges))
      return true
    }
    return false
  }

  function applyProjectedNodes(
    plan: IncrementalProjectionPlan,
    nextNodes: Node[] | undefined,
    changedNodeIds: readonly string[],
  ): void {
    if (!nextNodes || nextNodes === options.nodes.value || !plan.nodeMutations) return
    const {
      added: addedNodes,
      updated: updatedNodes,
      removedIds: removedNodeIds,
    } = plan.nodeMutations
    if (addedNodes.length > 0 || removedNodeIds.length > 0) {
      // addNodes/removeNodes emit nodesChange and would re-enter the
      // canonical graph command path. setNodes parses the full structural
      // projection without emitting a second command and reuses existing
      // runtime nodes by id.
      options.actions?.setNodes?.(toControlledNodes(nextNodes))
    } else {
      updatedNodes.forEach(updateRendererNodeProjection)
    }
    recordCanvasInteractionCount('incrementalNodeProjectionCount', changedNodeIds.length)
    syncNodes(nextNodes)
  }

  function applyProjectedEdgesFinally(
    plan: IncrementalProjectionPlan,
    nextEdges: Edge[] | undefined,
    changedEdgeIds: readonly string[],
    isEdgesApplied: boolean,
  ): void {
    if (!nextEdges || nextEdges === options.edges.value) return
    if (!isEdgesApplied && plan.hasEdgeChanges) {
      // New nodes must be present before Vue Flow validates their edges.
      options.actions?.setEdges?.(projectEdgesForActions(nextEdges))
    }
    recordCanvasInteractionCount('incrementalEdgeProjectionCount', changedEdgeIds.length)
    syncEdges(nextEdges)
  }

  /**
   * Applies an already-planned canonical graph commit without creating a Flow
   * command. History and persistence consume the graph commit itself; Flow is
   * only its renderer and therefore must not introduce a second side-effect
   * path.
   */
  function applyIncrementalProjection(projection: CanvasVueFlowIncrementalProjection) {
    const nextNodes = projection.nodes
    const nextEdges = projection.edges ? toCanvasSemanticEdges(projection.edges) : undefined
    const changedNodeIds = projection.changedNodeIds ?? []
    const changedEdgeIds = projection.changedEdgeIds ?? []
    projectGraph(changedNodeIds.length, changedEdgeIds.length, () => {
      const plan = planProjectionMutations(
        projection,
        nextNodes,
        nextEdges,
        changedNodeIds,
        changedEdgeIds,
      )
      const isEdgesApplied = applyProjectedEdgesFirst(plan, nextEdges)
      applyProjectedNodes(plan, nextNodes, changedNodeIds)
      applyProjectedEdgesFinally(plan, nextEdges, changedEdgeIds, isEdgesApplied)
    })
  }

  /**
   * Replaces the renderer projection from canonical facts after a recovery.
   * This is intentionally separate from command-based graph edits so recovery
   * cannot create a second history or persistence boundary.
   */
  function replaceFullProjection(graph: CanvasVueFlowGraph) {
    const semanticGraph = createGraph(graph.nodes, graph.edges)
    projectGraph(semanticGraph.nodes.length, semanticGraph.edges.length, () => {
      options.actions?.setNodes?.(semanticGraph.nodes)
      syncNodes(semanticGraph.nodes)
      const projectedEdges = projectEdgesForActions(semanticGraph.edges)
      options.actions?.setEdges?.(projectedEdges)
      syncEdges(semanticGraph.edges)
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
          const singleNodeToAdd = nodesToAdd[0]
          options.actions?.addNodes?.(
            nodesToAdd.length === 1 && singleNodeToAdd ? singleNodeToAdd : nodesToAdd,
          )
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
          const singleEdgeToAdd = projectedEdges[0]
          options.actions?.addEdges?.(
            projectedEdges.length === 1 && singleEdgeToAdd ? singleEdgeToAdd : projectedEdges,
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
      commandOptions.shouldRemoveConnectedEdges === true
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
            commandOptions.shouldRemoveConnectedEdges ?? false,
            commandOptions.shouldRemoveChildren,
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
    if (!currentNode) {
      return commandRunner.noOp('updateNode', commandOptions)
    }
    const patch = typeof update === 'function' ? update(currentNode) : update
    const nextNode = commandOptions.shouldReplace
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
            replace: commandOptions.shouldReplace ?? false,
          })
          syncNodes(nextNodes)
        }),
      commandOptions,
    })
  }

  return {
    replaceGraph,
    applyIncrementalProjection,
    replaceFullProjection,
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
