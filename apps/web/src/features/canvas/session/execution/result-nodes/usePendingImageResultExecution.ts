import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import { createCanvasNodeAt } from '../../../core/nodes/canvasNodeFactory.ts'
import { requireImageNodeOutputCount, type ImageNodeItem } from '../../../../board/ports/canvas'
import type { FlowNodeData } from '../../../core/nodes/flowTypes.ts'
import { resolveImageNodeFrameSize } from '../../../../board/ports/canvas'
import type {
  ImageExecutionInput,
  ImageExecutionResultOptions,
} from '../planning/imageExecutionResultTypes'
import type {
  CanvasGraphNodeRecord,
  CanvasGraphReadPort,
  CanvasGraphStore,
} from '../../../core/graph'
import type { BoardAttachment } from '../board-attachment'
import { flowConnectionToCanvasConnection } from '../../../session/flow-adapter'
import { canvasNodeToFlowNode } from '../../../session/flow-adapter'
import { createPrefixedId } from '../../../../../core/utils/ids'
import type { CanvasRendererMetricsPort } from '../../../session/flow-adapter'
import { resolveWritableCanvasGraphStore } from '../../../core/graph/canvasGraphStoreWriter'
import {
  readOutputAnchorLayout,
  resolvePendingImageResultPosition,
  type ImageResultLayout,
  type PendingImageResultLookup,
  type PendingImageResultPlacementOptions,
} from './planning'

type UsePendingImageResultNodesOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  readCanvasGraphStore: () => CanvasGraphReadPort
  writeCanvasGraphStore?: (() => CanvasGraphStore) | undefined
  rendererMetrics?: CanvasRendererMetricsPort | undefined
  readCurrentAttachment: () => BoardAttachment | null
  isAttachmentCurrent: (attachment: BoardAttachment) => boolean
  refreshNodeInternals?: ((nodeIds: string[]) => void) | undefined
  isNodeExecutionActive: (nodeId: string) => boolean
}

type PendingImageResultOptions = ImageExecutionResultOptions

type PendingImageResultReservationParams = {
  source: ImageExecutionInput
  sourceNodeId: string
  outputCount: number
  resultOptions?: PendingImageResultOptions | undefined
}

export type PendingImageResultExecution = {
  reservePendingResults: (
    params: PendingImageResultReservationParams,
  ) => PendingImageResultReservation
  attachPendingTaskIds: (
    slots: readonly Readonly<{
      resultNodeId: string
      taskId: string
      resultIndex: number
      clientActionId: string
    }>[],
  ) => void
  removePendingResults: (resultNodeIds: readonly string[]) => void
}

type PendingImageResultReservation = {
  resultNodeIds: string[]
}

type PendingImageResultReservationPlan = {
  resultNodeId: string
  nextNode: Node
  edgeToAppend?: Edge
}

export function usePendingImageResultExecution(
  options: UsePendingImageResultNodesOptions,
): PendingImageResultExecution {
  const rendererMetrics: CanvasRendererMetricsPort = options.rendererMetrics ?? {
    readNodeMetrics: (nodeId) => {
      const node = options.nodes.value.find((candidate) => candidate.id === nodeId)
      if (!node) return null
      const dimensions = (node as Node & { dimensions?: { width?: number; height?: number } })
        .dimensions
      const width = Number(dimensions?.width ?? node.width ?? 0)
      const height = Number(dimensions?.height ?? node.height ?? 0)
      return {
        nodeId,
        position: { x: node.position.x, y: node.position.y },
        dimensions: { width, height },
        computedPosition: { x: node.position.x, y: node.position.y },
        isMeasured: width > 0 && height > 0,
      }
    },
  }
  const writeGraphStore = () =>
    resolveWritableCanvasGraphStore(options.readCanvasGraphStore, options.writeCanvasGraphStore)
  function buildPendingResultLookup(): PendingImageResultLookup {
    const nodeById = new Map<string, CanvasGraphNodeRecord>()
    const imageChildIdsBySource = new Map<string, string[]>()
    const graph = options.readCanvasGraphStore()

    graph.getNodeIds().forEach((nodeId) => {
      const node = graph.getNode(nodeId)
      if (node) nodeById.set(node.id, node)
    })

    graph.getEdgeIds().forEach((edgeId) => {
      const edge = graph.getEdge(edgeId)
      if (!edge || edge.connection.fromNodeType !== 'image' || edge.connection.toPort !== 'image')
        return
      const source = `${edge.connection.fromNodeType}-${edge.connection.fromNodeId}`
      const target = `${edge.connection.toNodeType}-${edge.connection.toNodeId}`
      const current = imageChildIdsBySource.get(source)
      if (current) {
        current.push(target)
      } else {
        imageChildIdsBySource.set(source, [target])
      }
    })

    return {
      nodeById,
      imageChildIdsBySource,
    }
  }

  function isEmptyReusableImageNode(node: CanvasGraphNodeRecord): boolean {
    if (node.kind !== 'image') return false
    const snapshot = node.snapshot as ImageNodeItem
    if (!snapshot || typeof snapshot !== 'object') return false
    const sourceUrl = `${snapshot.sourceImageUrl ?? ''}`.trim()
    const outputImages = Array.isArray(snapshot.outputImages) ? snapshot.outputImages : []
    return !sourceUrl && outputImages.length === 0 && !options.isNodeExecutionActive(node.id)
  }

  function findReusableOutputImageNodeId(
    anchorNodeId: string,
    lookup: PendingImageResultLookup,
  ): string | null {
    const candidateIds = lookup.imageChildIdsBySource.get(anchorNodeId) ?? []
    for (const candidateId of candidateIds) {
      const node = lookup.nodeById.get(candidateId)
      if (node && isEmptyReusableImageNode(node)) return candidateId
    }
    return null
  }

  function buildPendingImageNodeSnapshot(
    base: ImageNodeItem,
    params: { sourceImageRatio: number; width: number },
  ): ImageNodeItem {
    return {
      ...base,
      sourceImageUrl: '',
      sourceImageRatio: params.sourceImageRatio,
      width: params.width,
      pendingTaskId: undefined,
      pendingResultIndex: undefined,
      pendingClientActionId: undefined,
      outputStatus: 'running',
      outputImages: [],
    }
  }

  function resolvePendingResultDimensions(
    source: ImageExecutionInput,
    optionsOverride?: PendingImageResultPlacementOptions,
  ): { ratio: number; width: number } {
    const sourceRatio = source.ratio > 0 ? source.ratio : 1
    const ratio =
      Number.isFinite(optionsOverride?.targetRatio) && (optionsOverride?.targetRatio as number) > 0
        ? (optionsOverride?.targetRatio as number)
        : sourceRatio
    const width =
      Number.isFinite(optionsOverride?.targetWidth) && (optionsOverride?.targetWidth as number) > 0
        ? Math.max(1, Math.round(optionsOverride?.targetWidth as number))
        : resolveImageNodeFrameSize(ratio, true).width
    return { ratio, width }
  }

  function reusePreferredPendingNode(
    preferredNodeId: string,
    lookup: PendingImageResultLookup,
    pendingImageNode: ImageNodeItem,
  ): { resultNodeId: string; nextNode: Node } {
    const currentNode = lookup.nodeById.get(preferredNodeId)
    if (currentNode?.kind === 'image') {
      const nextSnapshot = buildPendingImageNodeSnapshot(currentNode.snapshot as ImageNodeItem, {
        sourceImageRatio: pendingImageNode.sourceImageRatio,
        width: pendingImageNode.width,
      })
      const updatedNode = canvasNodeToFlowNode('image', nextSnapshot) as Node
      lookup.nodeById.set(preferredNodeId, { ...currentNode, snapshot: nextSnapshot })
      return { resultNodeId: preferredNodeId, nextNode: updatedNode }
    }
    throw new Error(`Reusable pending result node is no longer available: ${preferredNodeId}`)
  }

  function planPendingImageResultNodeWithLookup(
    source: ImageExecutionInput,
    anchorNodeId: string,
    lookup: PendingImageResultLookup,
    optionsOverride?: PendingImageResultPlacementOptions,
    preferredNodeId?: string | null,
  ): PendingImageResultReservationPlan {
    const { ratio: sourceRatio, width: fallbackWidth } = resolvePendingResultDimensions(
      source,
      undefined,
    )
    const fallbackAnchor: ImageResultLayout = {
      x: 0,
      y: 0,
      width: fallbackWidth,
      ratio: sourceRatio,
    }
    const anchor = readOutputAnchorLayout(
      anchorNodeId,
      fallbackAnchor,
      lookup,
      rendererMetrics,
      optionsOverride?.anchor,
    )
    const { ratio: resolvedRatio, width: resolvedWidth } = resolvePendingResultDimensions(
      source,
      optionsOverride,
    )
    const position = resolvePendingImageResultPosition(anchor, lookup, rendererMetrics, {
      width: resolvedWidth,
      ratio: resolvedRatio,
    })
    const imageNode = createCanvasNodeAt('image', position)
    const pendingImageNode = buildPendingImageNodeSnapshot(imageNode, {
      sourceImageRatio: resolvedRatio,
      width: resolvedWidth,
    })

    if (preferredNodeId) {
      return reusePreferredPendingNode(preferredNodeId, lookup, pendingImageNode)
    }

    const nextNode = canvasNodeToFlowNode('image', pendingImageNode) as Node<FlowNodeData>
    lookup.nodeById.set(nextNode.id, {
      id: nextNode.id as `image-${number}`,
      kind: 'image',
      snapshot: pendingImageNode,
      isSelected: false,
    })
    return {
      resultNodeId: nextNode.id,
      nextNode: nextNode as Node,
      edgeToAppend: {
        id: createPrefixedId('edge', '-'),
        source: anchorNodeId,
        sourceHandle: 'out:image',
        target: nextNode.id,
        targetHandle: 'in:image',
      },
    }
  }

  function reservePendingResults(
    params: PendingImageResultReservationParams,
  ): PendingImageResultReservation {
    const count = requireImageNodeOutputCount(params.outputCount)
    const resultOptions = params.resultOptions ?? {}
    const anchorNodeId = resultOptions.anchor?.outputAnchorNodeId ?? params.sourceNodeId
    const lookup = buildPendingResultLookup()
    const reusableNodeId = findReusableOutputImageNodeId(anchorNodeId, lookup)
    const plans = Array.from({ length: count }, (_, index) =>
      planPendingImageResultNodeWithLookup(
        params.source,
        anchorNodeId,
        lookup,
        {
          anchor: resultOptions.anchor,
          targetRatio: resultOptions.targetRatio,
          targetWidth: resultOptions.targetWidth,
        },
        index === 0 ? reusableNodeId : null,
      ),
    )
    const resultNodeIds = plans.map((plan) => plan.resultNodeId)
    const store = writeGraphStore()
    const attachment = options.readCurrentAttachment()
    if (!attachment || !options.isAttachmentCurrent(attachment)) {
      throw new Error('Pending result reservation requires the current Board attachment')
    }
    let nextConnectionId =
      store.getEdgeIds().reduce((highest, edgeId) => {
        const match = /^connection-(\d+)$/.exec(edgeId)
        return match ? Math.max(highest, Number(match[1])) : highest
      }, 0) + 1
    const pendingEdges = plans.flatMap((plan) => (plan.edgeToAppend ? [plan.edgeToAppend] : []))
    const canonicalEdges = pendingEdges.map((edge) => {
      const connection = flowConnectionToCanvasConnection(nextConnectionId++, edge)
      if (!connection) {
        throw new Error('Pending result edge has invalid Canvas endpoints')
      }
      return connection
    })
    const transaction = store.beginTransaction('node-update', {
      shouldRecordHistory: false,
      shouldMarkDirty: true,
    })
    try {
      plans.forEach((plan) => {
        const snapshot = (plan.nextNode.data as FlowNodeData).snapshotNode
        if (!snapshot || typeof snapshot !== 'object') return
        const nodeId = plan.resultNodeId as `image-${number}`
        if (store.getNode(nodeId)) transaction.updateNode(nodeId, () => snapshot as ImageNodeItem)
        else transaction.addNode(snapshot as ImageNodeItem)
      })
      canonicalEdges.forEach((connection) => transaction.addEdge(connection))
      transaction.commit()
    } catch (error) {
      transaction.cancel()
      console.error('[Canvas] Failed to reserve pending result nodes', error)
      throw new Error('Unable to reserve pending result nodes', { cause: error })
    }
    options.refreshNodeInternals?.([anchorNodeId, ...resultNodeIds])
    options.refreshNodeInternals?.([anchorNodeId, ...resultNodeIds])
    return { resultNodeIds }
  }

  function attachPendingTaskIds(
    slots: readonly Readonly<{
      resultNodeId: string
      taskId: string
      resultIndex: number
      clientActionId: string
    }>[],
  ) {
    const store = writeGraphStore()
    const transaction = store.beginTransaction('node-update', {
      shouldRecordHistory: false,
      shouldMarkDirty: true,
    })
    for (const slot of slots) {
      const taskId = slot.taskId.trim()
      if (!taskId) continue
      const record = store.getNode(slot.resultNodeId as `image-${number}`)
      if (record?.kind !== 'image') continue
      const nextSnapshot: ImageNodeItem = {
        ...(record.snapshot as ImageNodeItem),
        pendingTaskId: taskId,
        pendingResultIndex: slot.resultIndex,
        pendingClientActionId: slot.clientActionId,
      }
      transaction.updateNode(slot.resultNodeId as `image-${number}`, () => nextSnapshot)
    }
    try {
      transaction.commit()
    } catch (error) {
      transaction.cancel()
      console.error('[Canvas] Failed to attach pending task ids', error)
      return
    }
  }

  function removePendingResults(resultNodeIds: readonly string[]) {
    const store = writeGraphStore()
    const transaction = store.beginTransaction('delete', {
      shouldRecordHistory: false,
      shouldMarkDirty: true,
    })
    resultNodeIds.forEach((resultNodeId) =>
      transaction.removeNode(resultNodeId as `image-${number}`, {
        shouldRemoveConnectedEdges: true,
      }),
    )
    try {
      transaction.commit()
    } catch (error) {
      transaction.cancel()
      console.error('[Canvas] Failed to remove pending result nodes', error)
      return
    }
  }

  return {
    reservePendingResults,
    attachPendingTaskIds,
    removePendingResults,
  }
}
