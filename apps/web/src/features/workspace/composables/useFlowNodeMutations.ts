import type { Ref } from 'vue'
import type { Connection, Edge, Node } from '@vue-flow/core'
import {
  createEditNodeAt,
  createFusionNodeAt,
  createImageNodeAt,
  createModelNodeAt,
  createPromptNodeAt,
  createViewpointNodeAt,
} from '../utils/canvasNodeFactory'
import { isFlowConnectionAllowed, normalizeFlowConnection } from '../utils/flowConnectionAdapter'
import { resolveFlowNodeDragHandle } from '../utils/flowNodeDragHandle'
import { patchFlowSnapshotParam } from '../utils/flowSnapshotParamMutations'
import { useFlowConnectionMutations } from './useFlowConnectionMutations'
import { getDefaultCanvasNodeTitle } from '../types/canvasNodes'
import type { FlowNodeData } from '../types/flowTypes'
import type { FlowHistorySnapshot, FlowTransactions } from './useFlowTransactions'

export type FlowCreatableNodeKind =
  | 'prompt'
  | 'image'
  | 'edit'
  | 'fusion'
  | 'model'
  | 'viewpoint'

type UseFlowNodeMutationsOptions = {
  nodes: Ref<Array<Record<string, any>>>
  edges: Ref<Array<Record<string, any>>>
  createNodeKind: Ref<FlowCreatableNodeKind>
  commitGraphMutation: FlowTransactions['commitGraphMutation']
  commitNodeMutation: FlowTransactions['commitNodeMutation']
  commitEdgeMutation: FlowTransactions['commitEdgeMutation']
  onWarning: (message: string) => void
}

type UpdateSnapshotNodeOptions = {
  history?: boolean
  dirty?: boolean
  historyBefore?: FlowHistorySnapshot
}

type InitialSnapshotPatch = Record<string, unknown>

export function useFlowNodeMutations(options: UseFlowNodeMutationsOptions) {
  function resolveNodePosition(baseX = 120, baseY = 100) {
    if (options.nodes.value.length === 0) return { x: baseX, y: baseY }
    let maxY = baseY
    for (const node of options.nodes.value) {
      if (node.position.y > maxY) maxY = node.position.y
    }
    return { x: baseX, y: maxY + 40 }
  }

  function createSnapshotNodeForKind(kind: FlowCreatableNodeKind, x: number, y: number) {
    const point = { x, y }
    switch (kind) {
      case 'prompt':
        return createPromptNodeAt(point)
      case 'image':
        return createImageNodeAt(point)
      case 'edit':
        return createEditNodeAt(point)
      case 'fusion':
        return createFusionNodeAt(point)
      case 'model':
        return createModelNodeAt(point)
      case 'viewpoint':
        return createViewpointNodeAt(point)
    }
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
    allowsMultipleTargetConnections: (connection) =>
      (lookupData(connection.target ?? '')?.kind === 'prompt' ||
        lookupData(connection.target ?? '')?.kind === 'fusion') &&
      connection.targetHandle === 'in:image',
    validateConnectionMutation: (connection, existingEdges) => {
      const targetKind = lookupData(connection.target ?? '')?.kind
      if ((targetKind !== 'prompt' && targetKind !== 'fusion') || connection.targetHandle !== 'in:image') {
        return null
      }
      const imageInputCount = existingEdges.filter(
        (edge) => edge.target === connection.target && edge.targetHandle === 'in:image',
      ).length
      if (imageInputCount >= 4) {
        return targetKind === 'fusion' ? 'Fusion 最多接入 4 张图片' : 'Prompt 最多接入 4 张图片'
      }
      return null
    },
    commitEdgeMutation: options.commitEdgeMutation,
    onWarning: options.onWarning,
    markFlowExecutionNodeDirty,
  })

  function onAddNode() {
    const kind = options.createNodeKind.value
    const position = resolveNodePosition()
    appendNodeAtPosition(kind, position.x, position.y)
  }

  function appendNodeAtPosition(
    kind: FlowCreatableNodeKind,
    x: number,
    y: number,
    initialSnapshotPatch?: InitialSnapshotPatch,
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
    options.commitNodeMutation((items) => [...items, flowNode])
    return flowNodeId
  }

  function createNodeAtPosition(
    kind: FlowCreatableNodeKind,
    x: number,
    y: number,
    initialSnapshotPatch?: InitialSnapshotPatch,
  ) {
    return appendNodeAtPosition(kind, x, y, initialSnapshotPatch)
  }

  function onFlowDeleteNode(nodeId: string) {
    const flowNodes = options.nodes.value as Array<{ id: string; data?: unknown }>
    const target = flowNodes.find((node) => node.id === nodeId)
    if (!target) return
    options.commitGraphMutation((items, currentEdges) => ({
      nodes: items.filter((node) => node.id !== nodeId),
      edges: currentEdges.filter((edge) => edge.source !== nodeId && edge.target !== nodeId),
    }))
  }

  function updateSnapshotNode(
    nodeId: string,
    patcher: (snapshotNode: Record<string, any>) => Record<string, any>,
    mutationOptions: UpdateSnapshotNodeOptions = {},
  ) {
    const shouldPushHistory = mutationOptions.history ?? true
    const shouldMarkDirty = mutationOptions.dirty ?? true
    options.commitNodeMutation(
      (items) => {
        const nextNodes = [...items]
        const index = nextNodes.findIndex((node) => node.id === nodeId)
        if (index < 0) return items
        const node = nextNodes[index]
        const data = (node.data ?? {}) as { snapshotNode?: unknown; title?: string }
        if (!data.snapshotNode || typeof data.snapshotNode !== 'object') return items
        const snapshotNode = data.snapshotNode as Record<string, any>
        const nextSnapshotNode = patcher(snapshotNode)
        if (nextSnapshotNode === snapshotNode) return items
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
        return nextNodes
      },
      {
        recordHistory: shouldPushHistory,
        historyBefore: mutationOptions.historyBefore,
        markDirty: shouldMarkDirty,
      },
    )
  }

  function deleteSelectedFlowElements() {
    const flowNodes = options.nodes.value as Array<{ id: string; selected?: boolean }>
    const flowEdges = options.edges.value as Array<{
      id: string
      selected?: boolean
      source: string
      target: string
    }>
    const selectedNodeIds = new Set(
      flowNodes.filter((node) => !!node.selected).map((node) => node.id),
    )
    const selectedEdgeIds = new Set(
      flowEdges.filter((edge) => !!edge.selected).map((edge) => edge.id),
    )
    if (selectedNodeIds.size === 0 && selectedEdgeIds.size === 0) return
    options.commitGraphMutation((items, currentEdges) => ({
      nodes: items.filter((node) => !selectedNodeIds.has(node.id)),
      edges: currentEdges.filter(
        (edge) =>
          !selectedEdgeIds.has(edge.id) &&
          !selectedNodeIds.has(edge.source) &&
          !selectedNodeIds.has(edge.target),
      ),
    }))
  }

  function onFlowPromptTextUpdate(nodeId: string, value: string) {
    updateSnapshotNode(nodeId, (snapshotNode) => {
      if (snapshotNode.text === value) return snapshotNode
      return { ...snapshotNode, text: value }
    })
  }

  function onFlowNodeParamUpdate(nodeId: string, payload: { key: string; value: unknown }) {
    updateSnapshotNode(nodeId, (snapshotNode) => patchFlowSnapshotParam(snapshotNode, payload))
  }

  const EXECUTION_NODE_KINDS = new Set([
    'edit',
    'fusion',
    'model',
    'viewpoint',
  ])

  function markFlowExecutionNodeDirty(
    nodeId: string,
    { includeSelf = false }: { includeSelf?: boolean } = {},
  ) {
    const visited = new Set<string>()
    const queue: string[] = []
    if (includeSelf) {
      const selfData = lookupData(nodeId)
      if (selfData && EXECUTION_NODE_KINDS.has(selfData.kind)) {
        updateSnapshotNode(
          nodeId,
          (snapshotNode) => {
            if (snapshotNode.outputStatus === 'ready')
              return { ...snapshotNode, outputStatus: 'stale' }
            return snapshotNode
          },
          { history: false },
        )
      }
    }
    visited.add(nodeId)
    for (const edge of options.edges.value as unknown as Edge[]) {
      if (edge.source === nodeId) {
        if (!visited.has(edge.target)) {
          queue.push(edge.target)
          visited.add(edge.target)
        }
      }
    }
    while (queue.length > 0) {
      const currentId = queue.shift()!
      const data = lookupData(currentId)
      if (data && EXECUTION_NODE_KINDS.has(data.kind)) {
        updateSnapshotNode(
          currentId,
          (snapshotNode) => {
            if (snapshotNode.outputStatus === 'ready')
              return { ...snapshotNode, outputStatus: 'stale' }
            return snapshotNode
          },
          { history: false },
        )
      }
      for (const edge of options.edges.value as unknown as Edge[]) {
        if (edge.source === currentId && !visited.has(edge.target)) {
          queue.push(edge.target)
          visited.add(edge.target)
        }
      }
    }
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
    deleteSelectedFlowElements,
    onFlowPromptTextUpdate,
    onFlowNodeParamUpdate,
    markFlowExecutionNodeDirty,
  }
}
