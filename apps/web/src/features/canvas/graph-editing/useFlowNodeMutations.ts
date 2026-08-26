import type { Ref } from 'vue'
import type { Connection, Edge, Node } from '@vue-flow/core'
import { i18n } from '../../../shared/i18n'
import { createCanvasNodeAt } from '../utils/canvasNodeFactory'
import { getDefaultCanvasNodeTitle } from '../config/canvasNodeRegistry'
import { isFlowConnectionAllowed, normalizeFlowConnection } from '../utils/flowConnectionAdapter'
import {
  patchFlowSnapshotParam,
  type FlowSnapshotParamPatch,
} from '../utils/flowSnapshotParamMutations'
import { createCanvasConnectionCommandLifecycle } from '../vue-flow-adapter'
import type { FlowNodeData } from '../types/flowTypes'
import type { CanvasVueFlowGraphAdapter } from '../vue-flow-adapter/useCanvasVueFlowGraphAdapter'
import {
  isCanvasGraphNodeId,
  type CanvasGraphNodeId,
  type CanvasGraphNodeIdForKind,
  type CanvasGraphStore,
} from '../graph-runtime'
import {
  getCanvasNodePortInputLimit,
  isCanvasExecutionOutputNodeType,
  type CanvasBoardSnapshot,
  type CanvasCreatableNodeKind,
  type CanvasNodeItem,
  type CanvasNodeItemByKind,
} from '../../canvas-board/domain'
type UseFlowNodeMutationsOptions = {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- vue-flow node/edge data is an open Record; narrowing here breaks vue-flow's internal type expectations
  nodes: Ref<Array<Record<string, any>>>
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- vue-flow node/edge data is an open Record; narrowing here breaks vue-flow's internal type expectations
  edges: Ref<Array<Record<string, any>>>
  boardBaseSnapshot?: Ref<CanvasBoardSnapshot>
  readCanvasGraphStore: () => CanvasGraphStore
  graphAdapter: CanvasVueFlowGraphAdapter
  onWarning: (message: string) => void
}

type UpdateSnapshotNodeOptions = {
  history?: boolean
  dirty?: boolean
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

export function useFlowNodeMutations(options: UseFlowNodeMutationsOptions) {
  function lookupData(nodeId: string) {
    const matched = (options.nodes.value as Array<{ id: string; data?: unknown }>).find(
      (node) => node.id === nodeId,
    )
    return matched?.data as FlowNodeData | undefined
  }

  function isValidConnection(connection: Connection) {
    return isFlowConnectionAllowed(connection, lookupData)
  }

  const connectionAdapter = createCanvasConnectionCommandLifecycle({
    readNodes: () => options.nodes.value as unknown as Node[],
    readEdges: () => options.edges.value as unknown as Edge[],
    readBoardBaseSnapshot: () => options.boardBaseSnapshot?.value,
    readGraphStore: options.readCanvasGraphStore,
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
        return i18n.global.t('canvas.session.inputLimitExceeded', {
          label: getDefaultCanvasNodeTitle(targetKind),
          count: inputLimit,
        })
      }
      return null
    },
    graphAdapter: options.graphAdapter,
    onWarning: options.onWarning,
    markFlowExecutionNodeDirty,
  })

  function appendNodeAtPosition<T extends CanvasCreatableNodeKind>(
    kind: T,
    x: number,
    y: number,
    initialSnapshotPatch?: InitialSnapshotPatch<T>,
  ): CanvasGraphNodeIdForKind<T> | null {
    const baseSnapshotNode = createCanvasNodeAt(kind, { x, y })
    const snapshotNode = initialSnapshotPatch
      ? { ...baseSnapshotNode, ...initialSnapshotPatch }
      : baseSnapshotNode
    const snapshotId = Number(snapshotNode.id)
    if (!Number.isFinite(snapshotId) || snapshotId <= 0) return null
    const flowNodeId = `${kind}-${snapshotId}` as CanvasGraphNodeIdForKind<T>
    const store = options.readCanvasGraphStore()
    const transaction = store.beginTransaction('node-update')
    try {
      transaction.addNode(snapshotNode).commit()
    } catch (error) {
      transaction.cancel()
      console.error('[Canvas] Failed to create graph node', error)
      options.onWarning(i18n.global.t('canvas.session.createNodeFailed'))
      return null
    }
    return flowNodeId
  }

  function createNodeAtPosition<T extends CanvasCreatableNodeKind>(
    kind: T,
    x: number,
    y: number,
    initialSnapshotPatch?: InitialSnapshotPatch<T>,
  ): CanvasGraphNodeIdForKind<T> | null {
    return appendNodeAtPosition(kind, x, y, initialSnapshotPatch)
  }

  function updateSnapshotNode(
    nodeId: string,
    patcher: (snapshotNode: CanvasNodeItem) => CanvasNodeItem,
    mutationOptions: UpdateSnapshotNodeOptions = {},
  ) {
    const node = (options.nodes.value as Node[]).find((item) => item.id === nodeId)
    const graphNodeId = isCanvasGraphNodeId(nodeId) ? nodeId : null
    if (!node || !graphNodeId) return
    const store = options.readCanvasGraphStore()
    const record = store.getNode(graphNodeId)
    if (!record) return
    const snapshotNode = record.snapshot
    const nextSnapshotNode = patcher(snapshotNode)
    if (nextSnapshotNode === snapshotNode) return
    const transaction = store.beginTransaction('node-update', {
      recordHistory: mutationOptions.history ?? true,
      markDirty: mutationOptions.dirty ?? true,
    })
    try {
      transaction.updateNode(graphNodeId, () => nextSnapshotNode).commit()
    } catch (error) {
      transaction.cancel()
      console.error('[Canvas] Failed to update graph node', error)
      options.onWarning(i18n.global.t('canvas.session.updateNodeFailed'))
      return
    }
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

    const store = options.readCanvasGraphStore()
    const transaction = store.beginTransaction('node-update', {
      recordHistory: false,
      markDirty: false,
    })
    try {
      staleNodeIds.forEach((nodeId) => {
        const record = store.getNode(nodeId as CanvasGraphNodeId)
        if (
          record?.kind === 'fusion' &&
          record.snapshot.type === 'fusion' &&
          record.snapshot.outputStatus === 'ready'
        ) {
          transaction.updateNode(record.id, { outputStatus: 'stale' })
        }
      })
      transaction.commit()
    } catch (error) {
      transaction.cancel()
      console.error('[Canvas] Failed to mark execution nodes stale', error)
      options.onWarning(i18n.global.t('canvas.session.updateNodeStatusFailed'))
      return
    }

  }

  return {
    connectionAdapter,
    createNodeAtPosition,
    updateSnapshotNode,
    onFlowPromptTextUpdate,
    onFlowNodeParamUpdate,
    markFlowExecutionNodeDirty,
  }
}
