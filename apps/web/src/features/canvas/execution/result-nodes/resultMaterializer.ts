import {
  resolveImageNodeFrameSize,
  type ImageNodeItem,
} from '../../../canvas-board/domain'
import {
  type CanvasGraphEdgeId,
  type CanvasGraphNodeId,
  type CanvasGraphStore,
  type CanvasGraphTransactionSummary,
} from '../../graph-runtime'
import type { ResolvedTaskImageResult } from './resolvedTaskImageResult'

export type ResultMaterializerImageResult = ResolvedTaskImageResult & {
  resultNodeId: CanvasGraphNodeId
  taskId?: string
}

export type ResultMaterializerSkipReason =
  | 'missing-node'
  | 'stale-task'
  | 'non-image-node'
  | 'detached-node'

export type ResultMaterializerSkippedResult = {
  resultNodeId: CanvasGraphNodeId
  reason: ResultMaterializerSkipReason
}

export type ResultMaterializerProjectionIntent = {
  updatedEdges: Array<{
    id: CanvasGraphEdgeId
    animated: false
  }>
}

export type ResultMaterializerResult = {
  summary: CanvasGraphTransactionSummary
  materializedNodeIds: CanvasGraphNodeId[]
  skipped: ResultMaterializerSkippedResult[]
  projection: ResultMaterializerProjectionIntent
}

export type MaterializeResolvedImageResultsOptions = {
  store: CanvasGraphStore
  results: ResultMaterializerImageResult[]
  isAttached?: (result: ResultMaterializerImageResult) => boolean
}

function normalizeResolvedImageResult(
  result: ResolvedTaskImageResult,
  currentSnapshot: ImageNodeItem,
): {
  ratio: number
  width: number
} {
  const resolvedImageWidth = Number(result.width)
  const resolvedImageHeight = Number(result.height)
  const hasResolvedSize =
    Number.isFinite(resolvedImageWidth) &&
    resolvedImageWidth > 0 &&
    Number.isFinite(resolvedImageHeight) &&
    resolvedImageHeight > 0
  const ratio = hasResolvedSize
    ? resolvedImageHeight / resolvedImageWidth
    : result.ratio > 0
      ? result.ratio
      : 1

  const currentWidth = Number(currentSnapshot.width)
  const width = hasResolvedSize
    ? resolveImageNodeFrameSize(ratio, true).width
    : Number.isFinite(currentWidth) && currentWidth > 0
      ? Math.max(1, Math.round(currentWidth))
      : resolveImageNodeFrameSize(ratio, true).width

  return { ratio, width }
}

function isStaleTask(snapshot: ImageNodeItem, result: ResultMaterializerImageResult): boolean {
  if (!result.taskId) return false
  return snapshot.pendingTaskId !== result.taskId
}

export function materializeResolvedImageResults(
  options: MaterializeResolvedImageResultsOptions,
): ResultMaterializerResult {
  const transaction = options.store.beginTransaction('result-materialize', {
    recordHistory: false,
    markDirty: true,
  })
  const materializedNodeIds: CanvasGraphNodeId[] = []
  const skipped: ResultMaterializerSkippedResult[] = []
  const animatedEdgeIds = new Set<CanvasGraphEdgeId>()

  for (const result of options.results) {
    const node = options.store.getNode(result.resultNodeId)
    if (!node) {
      skipped.push({ resultNodeId: result.resultNodeId, reason: 'missing-node' })
      continue
    }
    if (node.kind !== 'image' || node.snapshot.type !== 'image-node') {
      skipped.push({ resultNodeId: result.resultNodeId, reason: 'non-image-node' })
      continue
    }
    const snapshot = node.snapshot
    if (isStaleTask(snapshot, result)) {
      skipped.push({ resultNodeId: result.resultNodeId, reason: 'stale-task' })
      continue
    }
    if (options.isAttached && !options.isAttached(result)) {
      skipped.push({ resultNodeId: result.resultNodeId, reason: 'detached-node' })
      continue
    }

    const resolved = normalizeResolvedImageResult(result, snapshot)
    transaction.updateNode(result.resultNodeId, {
      width: resolved.width,
      sourceImageUrl: result.url,
      sourceMediaAssetId: result.mediaAssetId,
      sourceImageRatio: resolved.ratio,
      pendingTaskId: undefined,
      outputStatus: 'ready',
      outputImages: [],
    })
    materializedNodeIds.push(result.resultNodeId)
    options.store.getIncomingEdgeIds(result.resultNodeId).forEach((edgeId) => {
      const edge = options.store.getEdge(edgeId)
      if (edge?.connection.toPort === 'image') animatedEdgeIds.add(edgeId)
    })
  }

  return {
    summary: transaction.commit(),
    materializedNodeIds,
    skipped,
    projection: {
      updatedEdges: Array.from(animatedEdgeIds).map((id) => ({ id, animated: false })),
    },
  }
}
