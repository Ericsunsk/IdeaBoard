import type { Ref } from 'vue'
import type { Connection, Edge, Node } from '@vue-flow/core'
import { normalizeHexColor } from '../components/nodes/colorPopoverUtils'
import {
  createColorNodeAt,
  createEditNodeAt,
  createEnhanceNodeAt,
  createFusionNodeAt,
  createImageNodeAt,
  createMaskNodeAt,
  createModelNodeAt,
  createPromptNodeAt,
  createViewpointNodeAt,
  createWearNodeAt,
} from '../utils/canvasNodeFactory'
import {
  getCanvasNodeVisibleInputPorts,
  getCanvasNodeVisibleOutputPorts,
} from '../config/canvasNodeConnectionContract'
import { isFlowConnectionAllowed } from '../utils/flowConnectionAdapter'
import { resolveFlowNodeDragHandle } from '../utils/flowNodeDragHandle'
import { patchFlowSnapshotParam } from '../utils/flowSnapshotParamMutations'
import { useFlowConnectionMutations } from './useFlowConnectionMutations'
import type { CanvasBoardSnapshot } from '../types/canvasNodes'
import {
  MAX_COLOR_NODE_SWATCHES,
  isColorNodePlaceholderState,
  normalizeColorNodeColors,
} from '../types/canvasNodes'
import type { FlowNodeData } from '../types/flowTypes'

export type FlowCreatableNodeKind =
  | 'prompt'
  | 'image'
  | 'edit'
  | 'enhance'
  | 'color'
  | 'fusion'
  | 'wear'
  | 'mask'
  | 'model'
  | 'viewpoint'

type UseFlowNodeMutationsOptions = {
  nodes: Ref<Array<Record<string, any>>>
  edges: Ref<Array<Record<string, any>>>
  createNodeKind: Ref<FlowCreatableNodeKind>
  boardBaseSnapshot: Ref<CanvasBoardSnapshot>
  captureHistory: () => { nodes: unknown[]; edges: unknown[] }
  pushHistory: (snapshot: { nodes: unknown[]; edges: unknown[] }) => void
  markDirty: () => void
  refreshSnapshot: () => void
  appendFlowNodes: (nextNodes: Node[]) => void
  commitNodes: (nextNodes: Node[]) => void
  commitEdges: (nextEdges: Edge[]) => void
  onWarning: (message: string) => void
  onNodeDeleted?: (nodeId: string, kind: string) => void
}

type UpdateSnapshotNodeOptions = {
  history?: boolean
  dirty?: boolean
  historyBefore?: { nodes: unknown[]; edges: unknown[] }
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

  function createSnapshotNodeForKind(kind: FlowCreatableNodeKind, x: number, y: number) {
    const point = { x, y }
    switch (kind) {
      case 'prompt':
        return createPromptNodeAt(point)
      case 'image':
        return createImageNodeAt(point)
      case 'edit':
        return createEditNodeAt(point)
      case 'enhance':
        return createEnhanceNodeAt(point)
      case 'color':
        return createColorNodeAt(point)
      case 'fusion':
        return createFusionNodeAt(point)
      case 'wear':
        return createWearNodeAt(point)
      case 'mask':
        return createMaskNodeAt(point)
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
    captureHistory: options.captureHistory,
    pushHistory: options.pushHistory,
    commitEdges: options.commitEdges,
    markDirty: options.markDirty,
    refreshSnapshot: options.refreshSnapshot,
    onWarning: options.onWarning,
    markFlowExecutionNodeDirty,
  })

  function onAddNode() {
    const kind = options.createNodeKind.value
    const position = resolveNodePosition()
    appendNodeAtPosition(kind, position.x, position.y)
  }

  function appendNodeAtPosition(kind: FlowCreatableNodeKind, x: number, y: number) {
    const before = options.captureHistory()
    const snapshotNode = createSnapshotNodeForKind(kind, x, y)
    const snapshotId = Number(snapshotNode.id)
    if (!Number.isFinite(snapshotId) || snapshotId <= 0) return null
    const flowNodeId = `${kind}-${snapshotId}`
    const flowNode: Node = {
      id: flowNodeId,
      type: 'canvas',
      dragHandle: resolveFlowNodeDragHandle(kind),
      position: { x, y },
      data: {
        kind,
        nodeType: kind,
        title: `${kind[0].toUpperCase()}${kind.slice(1)} #${snapshotId}`,
        inputPorts: getCanvasNodeVisibleInputPorts(kind, snapshotNode).map((port) => port.port),
        outputPorts: getCanvasNodeVisibleOutputPorts(kind, snapshotNode).map((port) => port.port),
        snapshotNode,
      } satisfies FlowNodeData,
    }
    options.appendFlowNodes([flowNode])
    options.pushHistory(before)
    options.markDirty()
    options.refreshSnapshot()
    return flowNodeId
  }

  function createNodeAtPosition(kind: FlowCreatableNodeKind, x: number, y: number) {
    return appendNodeAtPosition(kind, x, y)
  }

  function onFlowDeleteNode(nodeId: string) {
    const flowNodes = options.nodes.value as Array<{ id: string; data?: unknown }>
    const target = flowNodes.find((node) => node.id === nodeId)
    if (!target) return
    const targetKind = (target.data as { kind?: string } | undefined)?.kind
    if (!targetKind || targetKind === 'message') return
    options.onNodeDeleted?.(nodeId, targetKind)
    const before = options.captureHistory()
    const nextNodes = (options.nodes.value as Node[]).filter((node) => node.id !== nodeId)
    const nextEdges = (options.edges.value as Edge[]).filter(
      (edge) => edge.source !== nodeId && edge.target !== nodeId,
    )
    options.commitNodes(nextNodes)
    options.commitEdges(nextEdges)
    options.pushHistory(before)
    options.markDirty()
    options.refreshSnapshot()
  }

  function updateSnapshotNode(
    nodeId: string,
    patcher: (snapshotNode: Record<string, any>) => Record<string, any>,
    mutationOptions: UpdateSnapshotNodeOptions = {},
  ) {
    const shouldPushHistory = mutationOptions.history ?? true
    const shouldMarkDirty = mutationOptions.dirty ?? true
    const before = mutationOptions.historyBefore ?? options.captureHistory()
    const nextNodes = [...options.nodes.value] as Array<Record<string, any>>
    const index = nextNodes.findIndex((node) => node.id === nodeId)
    if (index < 0) return
    const node = nextNodes[index]
    const data = (node.data ?? {}) as { snapshotNode?: unknown }
    if (!data.snapshotNode || typeof data.snapshotNode !== 'object') return
    const snapshotNode = data.snapshotNode as Record<string, any>
    const nextSnapshotNode = patcher(snapshotNode)
    if (nextSnapshotNode === snapshotNode) return
    node.data.snapshotNode = nextSnapshotNode
    if (shouldPushHistory) {
      options.pushHistory(before)
    }
    if (shouldMarkDirty) {
      options.markDirty()
    }
    options.refreshSnapshot()
  }

  function deleteSelectedFlowElements() {
    const flowNodes = options.nodes.value as Array<{ id: string; selected?: boolean }>
    const selectedNodeKinds = new Map(
      (options.nodes.value as Array<{ id: string; selected?: boolean; data?: { kind?: string } }>)
        .filter((node) => !!node.selected && typeof node.data?.kind === 'string')
        .map((node) => [node.id, node.data!.kind as string] as const),
    )
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
    selectedNodeKinds.forEach((kind, nodeId) => {
      if (kind === 'message') return
      options.onNodeDeleted?.(nodeId, kind)
    })
    const before = options.captureHistory()
    const nextNodes = flowNodes.filter((node) => !selectedNodeIds.has(node.id)) as Node[]
    const nextEdges = flowEdges.filter(
      (edge) =>
        !selectedEdgeIds.has(edge.id) &&
        !selectedNodeIds.has(edge.source) &&
        !selectedNodeIds.has(edge.target),
    ) as Edge[]
    options.commitNodes(nextNodes)
    options.commitEdges(nextEdges)
    options.pushHistory(before)
    options.markDirty()
    options.refreshSnapshot()
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

  function onFlowColorApplyPreset(nodeId: string, value: string[]) {
    updateSnapshotNode(nodeId, (snapshotNode) => {
      const baseColors = isColorNodePlaceholderState(snapshotNode)
        ? []
        : Array.isArray(snapshotNode.colors)
          ? snapshotNode.colors
          : []
      const normalizedPreset = value.map((color) => normalizeHexColor(color))
      if (normalizedPreset.length === 0) return snapshotNode
      const nextColors = normalizeColorNodeColors([...baseColors, ...normalizedPreset])
      if (
        nextColors.length === baseColors.length &&
        nextColors.every((color: string, index: number) => color === baseColors[index])
      ) {
        return snapshotNode
      }
      return {
        ...snapshotNode,
        colors: nextColors,
      }
    })
  }

  function onFlowColorRemove(nodeId: string, index: number) {
    updateSnapshotNode(nodeId, (snapshotNode) => {
      const colors = Array.isArray(snapshotNode.colors) ? snapshotNode.colors : []
      if (index < 0 || index >= colors.length) return snapshotNode
      const nextColors = [...colors]
      nextColors.splice(index, 1)
      return { ...snapshotNode, colors: normalizeColorNodeColors(nextColors) }
    })
  }

  function onFlowColorAdd(nodeId: string, value: string) {
    updateSnapshotNode(nodeId, (snapshotNode) => {
      const baseColors = isColorNodePlaceholderState(snapshotNode)
        ? []
        : Array.isArray(snapshotNode.colors)
          ? snapshotNode.colors
          : []
      if (baseColors.length >= MAX_COLOR_NODE_SWATCHES) return snapshotNode
      const nextColors = normalizeColorNodeColors([...baseColors, normalizeHexColor(value)])
      if (
        nextColors.length === baseColors.length &&
        nextColors.every((color: string, index: number) => color === baseColors[index])
      ) {
        return snapshotNode
      }
      return {
        ...snapshotNode,
        colors: nextColors,
      }
    })
  }

  function onFlowMaskToggleInvert(nodeId: string) {
    updateSnapshotNode(nodeId, (snapshotNode) => {
      const nextInvertMask = !snapshotNode.invertMask
      if (snapshotNode.invertMask === nextInvertMask) return snapshotNode
      return {
        ...snapshotNode,
        invertMask: nextInvertMask,
      }
    })
  }

  const EXECUTION_NODE_KINDS = new Set([
    'edit',
    'enhance',
    'color',
    'fusion',
    'wear',
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
    onFlowColorApplyPreset,
    onFlowColorRemove,
    onFlowColorAdd,
    onFlowMaskToggleInvert,
    markFlowExecutionNodeDirty,
  }
}
