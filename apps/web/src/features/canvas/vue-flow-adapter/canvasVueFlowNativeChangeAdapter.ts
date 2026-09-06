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
import { measureCanvasPerf } from '../utils/canvasPerfProbe'
import { toCanvasSemanticEdges } from './canvasEdgeProjection'
import type {
  CanvasVueFlowGraphCommandOptions,
  CanvasVueFlowGraphCommandResult,
  CanvasVueFlowGraphCommandRunner,
} from './canvasVueFlowGraphCommandRunner'
import {
  classifyVueFlowNativeEdgeChanges,
  classifyVueFlowNativeNodeChanges,
  mergeVueFlowNativeChangeClassifications,
} from './canvasVueFlowNativeChangePolicy'

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

type CanvasVueFlowNativeChangeAdapterOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  actions?: CanvasVueFlowGraphActions
  syncNodes: (nextNodes: Node[]) => void
  syncEdges: (nextEdges: Edge[]) => void
  commandRunner: CanvasVueFlowGraphCommandRunner
}

type CanvasControlledNodeChange =
  Extract<NodeChange, { type: 'position' }> | Extract<NodeChange, { type: 'select' }>

function isCanvasControlledNodeChange(change: NodeChange): change is CanvasControlledNodeChange {
  return change.type === 'position' || change.type === 'select'
}

function createVisualOnlyResult(
  command: 'applyGraphChanges' | 'applyNodeChanges' | 'applyEdgeChanges',
  before: { nodes: Node[]; edges: Edge[] },
  after: { nodes: Node[]; edges: Edge[] },
  commandOptions: CanvasVueFlowGraphCommandOptions | undefined,
): CanvasVueFlowGraphCommandResult {
  return {
    command,
    before,
    after,
    didChange: false,
    metadata: commandOptions?.metadata,
  }
}

export function createCanvasVueFlowNativeChangeAdapter(
  options: CanvasVueFlowNativeChangeAdapterOptions,
) {
  function applyNativeNodeChanges(changes: NodeChange[]) {
    measureCanvasPerf(
      'vue-flow.projection',
      () => ({ nodes: options.nodes.value.length, changes: changes.length }),
      () => {
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
      },
    )
  }

  /**
   * Mirrors an event already applied by Vue Flow into the controlled prop.
   * Active drags must not re-enter Vue Flow through its action API.
   */
  function syncControlledNodeChanges(changes: NodeChange[]) {
    const changesByNodeId = new Map<string, CanvasControlledNodeChange[]>()
    changes.filter(isCanvasControlledNodeChange).forEach((change) => {
      const nodeChanges = changesByNodeId.get(change.id) ?? []
      nodeChanges.push(change)
      changesByNodeId.set(change.id, nodeChanges)
    })
    if (changesByNodeId.size === 0) return

    let nextNodes: Node[] | null = null
    options.nodes.value.forEach((node, index) => {
      const nodeChanges = changesByNodeId.get(node.id)
      if (!nodeChanges) return
      let nextNode = node
      nodeChanges.forEach((change) => {
        if (change.type === 'select') {
          nextNode = { ...nextNode, selected: change.selected } as Node
          return
        }
        nextNode = {
          ...nextNode,
          ...(change.position ? { position: change.position } : {}),
        } as Node
      })
      if (nextNode === node) return
      if (!nextNodes) nextNodes = [...options.nodes.value]
      nextNodes[index] = nextNode
    })
    if (nextNodes) {
      options.syncNodes(nextNodes)
    }
  }

  function applyNativeEdgeChanges(changes: EdgeChange[]) {
    measureCanvasPerf(
      'vue-flow.projection',
      () => ({ edges: options.edges.value.length, changes: changes.length }),
      () => {
        recordCanvasInteractionCount('nativeEdgeChangeBatches')
        recordCanvasInteractionCount('nativeEdgeChangeCount', changes.length)
        const actionResult = options.actions?.applyEdgeChanges?.(changes)
        if (Array.isArray(actionResult)) {
          options.syncEdges(toCanvasSemanticEdges(actionResult as Edge[]))
          return
        }
        const nextEdges = applyVueFlowEdgeChanges(
          changes,
          options.edges.value as Parameters<typeof applyVueFlowEdgeChanges>[1],
        ) as unknown as Edge[]
        if (nextEdges !== options.edges.value) options.syncEdges(nextEdges)
      },
    )
  }

  function applyNodeChanges(
    changes: NodeChange[],
    commandOptions?: CanvasVueFlowGraphCommandOptions,
  ): CanvasVueFlowGraphCommandResult {
    const classification = classifyVueFlowNativeNodeChanges(changes, options.nodes.value)
    if (classification === 'no-op') {
      return options.commandRunner.noOp('applyNodeChanges', commandOptions)
    }
    if (classification === 'visual-only') {
      const before = { nodes: options.nodes.value, edges: options.edges.value }
      applyNativeNodeChanges(changes)
      return createVisualOnlyResult(
        'applyNodeChanges',
        before,
        { nodes: options.nodes.value, edges: options.edges.value },
        commandOptions,
      )
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
    const classification = classifyVueFlowNativeEdgeChanges(changes, options.edges.value)
    if (classification === 'no-op') {
      return options.commandRunner.noOp('applyEdgeChanges', commandOptions)
    }
    if (classification === 'visual-only') {
      const before = { nodes: options.nodes.value, edges: options.edges.value }
      applyNativeEdgeChanges(changes)
      return createVisualOnlyResult(
        'applyEdgeChanges',
        before,
        { nodes: options.nodes.value, edges: options.edges.value },
        commandOptions,
      )
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
    const nodeClassification = classifyVueFlowNativeNodeChanges(nodeChanges, options.nodes.value)
    const edgeClassification = classifyVueFlowNativeEdgeChanges(edgeChanges, options.edges.value)
    const classification = mergeVueFlowNativeChangeClassifications(
      nodeClassification,
      edgeClassification,
    )
    if (classification === 'no-op') {
      return options.commandRunner.noOp('applyGraphChanges', commandOptions)
    }
    const shouldApplyNodeChanges = nodeClassification !== 'no-op'
    const shouldApplyEdgeChanges = edgeClassification !== 'no-op'
    if (classification === 'visual-only') {
      const before = { nodes: options.nodes.value, edges: options.edges.value }
      if (shouldApplyNodeChanges) applyNativeNodeChanges(nodeChanges)
      if (shouldApplyEdgeChanges) applyNativeEdgeChanges(edgeChanges)
      return createVisualOnlyResult(
        'applyGraphChanges',
        before,
        { nodes: options.nodes.value, edges: options.edges.value },
        commandOptions,
      )
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
    syncControlledNodeChanges,
    applyNodeChanges,
    applyEdgeChanges,
  }
}
