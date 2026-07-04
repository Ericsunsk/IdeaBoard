import type { Ref } from 'vue'
import type { Connection, Edge, Node } from '@vue-flow/core'
import { createCanvasNodeAt } from '../utils/canvasNodeFactory'
import { getDefaultCanvasNodeTitle } from '../config/canvasNodeRegistry'
import { isFlowConnectionAllowed, normalizeFlowConnection } from '../utils/flowConnectionAdapter'
import { resolveFlowNodeDragHandle } from '../utils/flowNodeDragHandle'
import {
  patchFlowSnapshotParam,
  type FlowSnapshotParamPatch,
} from '../utils/flowSnapshotParamMutations'
import { useFlowConnectionMutations } from './useFlowConnectionMutations'
import type { FlowNodeData } from '../types/flowTypes'
import type { CanvasVueFlowGraphAdapter } from './useCanvasVueFlowGraphAdapter'
import {
  createFlowGraphCommandOptions,
  type FlowGraphCommandSideEffectOptions,
} from './flowGraphAdapterCommandSideEffects'
import type { FlowHistorySnapshot } from './useFlowHistory'
import {
  getCanvasNodePortInputLimit,
  isCanvasExecutionOutputNodeType,
  type CanvasCreatableNodeKind,
  type CanvasNodeItem,
  type CanvasNodeItemByKind,
} from '../../canvas-board/domain'

type UseFlowNodeMutationsOptions = {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- vue-flow node/edge data is an open Record; narrowing here breaks vue-flow's internal type expectations
  nodes: Ref<Array<Record<string, any>>>
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- vue-flow node/edge data is an open Record; narrowing here breaks vue-flow's internal type expectations
  edges: Ref<Array<Record<string, any>>>
  createNodeKind: Ref<CanvasCreatableNodeKind>
  graphAdapter: CanvasVueFlowGraphAdapter
  onWarning: (message: string) => void
}

type UpdateSnapshotNodeOptions = {
  history?: boolean
  dirty?: boolean
  historyBefore?: FlowHistorySnapshot
}

type InitialSnapshotPatch<T extends CanvasCreatableNodeKind> = Partial<CanvasNodeItemByKind[T]>

type FlowNodeDataSource = {
  id: string
  data?: unknown
}

function buildOutgoingEdgesBySource(edges: Edge[]): Map<string, Edge[]> {
  const outgoingEdgesBySource = new Map<string, Edge[]>()
  for (const edge of edges) {
    const existingEdges = outgoingEdgesBySource.get(edge.source)
    if (existingEdges) {
      existingEdges.push(edge)
    } else {
      outgoingEdgesBySource.set(edge.source, [edge])
    }
  }
  return outgoingEdgesBySource
}

function buildNodeDataById(nodes: FlowNodeDataSource[]): Map<string, FlowNodeData> {
  const nodeDataById = new Map<string, FlowNodeData>()
  for (const node of nodes) {
    if (node.data) nodeDataById.set(node.id, node.data as FlowNodeData)
  }
  return nodeDataById
}

function isReadyExecutionOutputNode(data: FlowNodeData | undefined): boolean {
  if (!data || !isCanvasExecutionOutputNodeType(data.kind)) return false
  const snapshotNode = data.snapshotNode
  return (
    !!snapshotNode &&
    typeof snapshotNode === 'object' &&
    'outputStatus' in snapshotNode &&
    snapshotNode.outputStatus === 'ready'
  )
}

function collectReadyExecutionOutputNodeIds(params: {
  nodeId: string
  includeSelf: boolean
  outgoingEdgesBySource: Map<string, Edge[]>
  nodeDataById: Map<string, FlowNodeData>
}): Set<string> {
  const staleNodeIds = new Set<string>()
  const visited = new Set<string>()
  const queue: string[] = []

  const collectIfReady = (targetNodeId: string) => {
    if (isReadyExecutionOutputNode(params.nodeDataById.get(targetNodeId))) {
      staleNodeIds.add(targetNodeId)
    }
  }

  const enqueueOutgoingTargets = (sourceNodeId: string) => {
    for (const edge of params.outgoingEdgesBySource.get(sourceNodeId) ?? []) {
      if (visited.has(edge.target)) continue
      queue.push(edge.target)
      visited.add(edge.target)
    }
  }

  if (params.includeSelf) collectIfReady(params.nodeId)

  visited.add(params.nodeId)
  enqueueOutgoingTargets(params.nodeId)

  for (let queueIndex = 0; queueIndex < queue.length; queueIndex += 1) {
    const currentId = queue[queueIndex]!
    collectIfReady(currentId)
    enqueueOutgoingTargets(currentId)
  }

  return staleNodeIds
}

function markReadyExecutionOutputNodesStale(items: Node[], staleNodeIds: Set<string>): Node[] {
  let nextNodes: Node[] | null = null
  for (let index = 0; index < items.length; index += 1) {
    const node = items[index]!
    if (!staleNodeIds.has(node.id)) continue
    const data = (node.data ?? {}) as { snapshotNode?: unknown; title?: string }
    const snapshotNode = data.snapshotNode
    if (
      !snapshotNode ||
      typeof snapshotNode !== 'object' ||
      !('outputStatus' in snapshotNode) ||
      snapshotNode.outputStatus !== 'ready'
    ) {
      continue
    }
    const nextSnapshotNode = {
      ...(snapshotNode as CanvasNodeItem),
      outputStatus: 'stale',
    }
    if (!nextNodes) nextNodes = [...items]
    nextNodes[index] = {
      ...node,
      data: {
        ...data,
        title:
          typeof nextSnapshotNode.title === 'string' && nextSnapshotNode.title.trim()
            ? nextSnapshotNode.title.trim()
            : data.title,
        snapshotNode: nextSnapshotNode,
      },
    }
  }
  return nextNodes ?? items
}

function createSnapshotCommandSideEffects(
  mutationOptions: UpdateSnapshotNodeOptions,
): FlowGraphCommandSideEffectOptions {
  const sideEffects: FlowGraphCommandSideEffectOptions = {}
  if (mutationOptions.history !== undefined) sideEffects.recordHistory = mutationOptions.history
  if (mutationOptions.historyBefore) sideEffects.historyBefore = mutationOptions.historyBefore
  if (mutationOptions.dirty !== undefined) sideEffects.markDirty = mutationOptions.dirty
  return sideEffects
}

export function useFlowNodeMutations(options: UseFlowNodeMutationsOptions) {
  function resolveNodePosition(baseX = 120, baseY = 100) {
    if (options.nodes.value.length === 0) return { x: baseX, y: baseY }
    let maxY = baseY
    for (const node of options.nodes.value) {
      if (node.position.y > maxY) maxY = node.position.y
    }
    return { x: baseX, y: maxY + 40 }
  }

  function createSnapshotNodeForKind<T extends CanvasCreatableNodeKind>(
    kind: T,
    x: number,
    y: number,
  ) {
    return createCanvasNodeAt(kind, { x, y })
  }

  function lookupData(nodeId: string) {
    const matched = (options.nodes.value as Array<{ id: string; data?: unknown }>).find(
      (node) => node.id === nodeId,
    )
    return matched?.data as FlowNodeData | undefined
  }

  function isValidConnection(connection: Connection) {
    return isFlowConnectionAllowed(connection, lookupData)
  }

  const connectionMutations = useFlowConnectionMutations({
    edges: options.edges,
    isValidConnection,
    normalizeConnection: (connection, existingEdges) =>
      normalizeFlowConnection(connection, lookupData, existingEdges),
    allowsMultipleTargetConnections: (connection) => {
      const targetKind = lookupData(connection.target ?? '')?.kind
      return (
        !!targetKind &&
        getCanvasNodePortInputLimit(targetKind, 'image') > 1 &&
        connection.targetHandle === 'in:image'
      )
    },
    validateConnectionMutation: (connection, existingEdges) => {
      const targetKind = lookupData(connection.target ?? '')?.kind
      if (!targetKind || connection.targetHandle !== 'in:image') {
        return null
      }
      const inputLimit = getCanvasNodePortInputLimit(targetKind, 'image')
      if (inputLimit <= 1) return null
      const imageInputCount = existingEdges.filter(
        (edge) => edge.target === connection.target && edge.targetHandle === 'in:image',
      ).length
      if (imageInputCount >= inputLimit) {
        return `${getDefaultCanvasNodeTitle(targetKind)} 最多接入 ${inputLimit} 张图片`
      }
      return null
    },
    graphAdapter: options.graphAdapter,
    onWarning: options.onWarning,
    markFlowExecutionNodeDirty,
  })

  function onAddNode() {
    const kind = options.createNodeKind.value
    const position = resolveNodePosition()
    appendNodeAtPosition(kind, position.x, position.y)
  }

  function appendNodeAtPosition<T extends CanvasCreatableNodeKind>(
    kind: T,
    x: number,
    y: number,
    initialSnapshotPatch?: InitialSnapshotPatch<T>,
  ) {
    const baseSnapshotNode = createSnapshotNodeForKind(kind, x, y)
    const snapshotNode = initialSnapshotPatch
      ? { ...baseSnapshotNode, ...initialSnapshotPatch }
      : baseSnapshotNode
    const snapshotId = Number(snapshotNode.id)
    if (!Number.isFinite(snapshotId) || snapshotId <= 0) return null
    const flowNodeId = `${kind}-${snapshotId}`
    const flowNodeTitle =
      typeof snapshotNode.title === 'string' && snapshotNode.title.trim()
        ? snapshotNode.title.trim()
        : getDefaultCanvasNodeTitle(kind)
    const flowNode: Node = {
      id: flowNodeId,
      type: 'canvas',
      dragHandle: resolveFlowNodeDragHandle(kind),
      position: { x, y },
      data: {
        kind,
        nodeType: kind,
        title: flowNodeTitle,
        snapshotNode,
      } satisfies FlowNodeData,
    }
    options.graphAdapter.addNodes(flowNode, createFlowGraphCommandOptions())
    return flowNodeId
  }

  function createNodeAtPosition<T extends CanvasCreatableNodeKind>(
    kind: T,
    x: number,
    y: number,
    initialSnapshotPatch?: InitialSnapshotPatch<T>,
  ) {
    return appendNodeAtPosition(kind, x, y, initialSnapshotPatch)
  }

  function onFlowDeleteNode(nodeId: string) {
    const flowNodes = options.nodes.value as Array<{ id: string; data?: unknown }>
    const target = flowNodes.find((node) => node.id === nodeId)
    if (!target) return
    options.graphAdapter.removeNodes(nodeId, {
      ...createFlowGraphCommandOptions(),
      removeConnectedEdges: true,
    })
  }

  function updateSnapshotNode(
    nodeId: string,
    patcher: (snapshotNode: CanvasNodeItem) => CanvasNodeItem,
    mutationOptions: UpdateSnapshotNodeOptions = {},
  ) {
    const node = (options.nodes.value as Node[]).find((item) => item.id === nodeId)
    const data = (node?.data ?? {}) as { snapshotNode?: unknown; title?: string }
    if (!node || !data.snapshotNode || typeof data.snapshotNode !== 'object') return
    const snapshotNode = data.snapshotNode as CanvasNodeItem
    const nextSnapshotNode = patcher(snapshotNode)
    if (nextSnapshotNode === snapshotNode) return
    options.graphAdapter.updateNode(
      nodeId,
      {
        data: {
          ...data,
          title:
            typeof nextSnapshotNode.title === 'string' && nextSnapshotNode.title.trim()
              ? nextSnapshotNode.title.trim()
              : data.title,
          snapshotNode: nextSnapshotNode,
        },
      },
      createFlowGraphCommandOptions(createSnapshotCommandSideEffects(mutationOptions)),
    )
  }

  function onFlowPromptTextUpdate(nodeId: string, value: string) {
    updateSnapshotNode(nodeId, (snapshotNode) => {
      if (snapshotNode.type !== 'prompt') return snapshotNode
      if (snapshotNode.text === value) return snapshotNode
      return { ...snapshotNode, text: value }
    })
  }

  function onFlowNodeParamUpdate(nodeId: string, payload: FlowSnapshotParamPatch) {
    updateSnapshotNode(nodeId, (snapshotNode) => patchFlowSnapshotParam(snapshotNode, payload))
  }

  function markFlowExecutionNodeDirty(
    nodeId: string,
    { includeSelf = false }: { includeSelf?: boolean } = {},
  ) {
    const staleNodeIds = collectReadyExecutionOutputNodeIds({
      nodeId,
      includeSelf,
      outgoingEdgesBySource: buildOutgoingEdgesBySource(options.edges.value as unknown as Edge[]),
      nodeDataById: buildNodeDataById(options.nodes.value as FlowNodeDataSource[]),
    })

    if (staleNodeIds.size === 0) return

    options.graphAdapter.replaceNodes(
      (items) => markReadyExecutionOutputNodesStale(items, staleNodeIds),
      createFlowGraphCommandOptions({ recordHistory: false }),
    )
  }

  return {
    isValidConnection,
    resolveNodePosition,
    createSnapshotNodeForKind,
    onConnect: connectionMutations.onConnect,
    onEdgeUpdate: connectionMutations.onEdgeUpdate,
    onAddNode,
    createNodeAtPosition,
    onFlowDeleteNode,
    updateSnapshotNode,
    onFlowPromptTextUpdate,
    onFlowNodeParamUpdate,
    markFlowExecutionNodeDirty,
  }
}
