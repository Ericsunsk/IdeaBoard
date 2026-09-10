import {
  rebuildBoardMediaRefs,
  type BoardSnapshot,
  type ImageNodeOutputItem,
  type PromptNodeInputImage,
} from '../../board/domain'

type MediaAssetReference = Pick<ImageNodeOutputItem, 'url' | 'mediaAssetId'>

function hasSameMediaAsset(left: MediaAssetReference, right: MediaAssetReference): boolean {
  return left.url === right.url && left.mediaAssetId === right.mediaAssetId
}

function applyStabilizedMediaAsset<T extends MediaAssetReference>(
  current: T,
  captured: MediaAssetReference,
  stabilized: MediaAssetReference,
): T {
  if (!hasSameMediaAsset(current, captured) || hasSameMediaAsset(captured, stabilized))
    return current
  const next = { ...current, url: stabilized.url }
  if (stabilized.mediaAssetId === undefined) delete next.mediaAssetId
  else next.mediaAssetId = stabilized.mediaAssetId
  return next
}

function rebaseIndexedMediaAssets<T extends MediaAssetReference>(
  current: T[],
  captured: MediaAssetReference[],
  stabilized: MediaAssetReference[],
): T[] {
  return current.map((item, index) => {
    const capturedItem = captured[index]
    const stabilizedItem = stabilized[index]
    return capturedItem && stabilizedItem
      ? applyStabilizedMediaAsset(item, capturedItem, stabilizedItem)
      : item
  })
}

function rebaseInputMediaAssets(
  current: PromptNodeInputImage[],
  captured: PromptNodeInputImage[],
  stabilized: PromptNodeInputImage[],
): PromptNodeInputImage[] {
  const capturedById = new Map(captured.map((item) => [item.id, item] as const))
  const stabilizedById = new Map(stabilized.map((item) => [item.id, item] as const))
  return current.map((item) => {
    const capturedItem = capturedById.get(item.id)
    const stabilizedItem = stabilizedById.get(item.id)
    return capturedItem && stabilizedItem
      ? applyStabilizedMediaAsset(item, capturedItem, stabilizedItem)
      : item
  })
}

/** Reconciles server-normalized media URLs while preserving concurrent local edits. */
export function rebaseStabilizedMedia(
  captured: BoardSnapshot,
  current: BoardSnapshot,
  stabilized: BoardSnapshot,
): BoardSnapshot {
  const capturedImageNodes = new Map(captured.imageNodes.map((node) => [node.id, node] as const))
  const stabilizedImageNodes = new Map(
    stabilized.imageNodes.map((node) => [node.id, node] as const),
  )
  const capturedPromptNodes = new Map(captured.promptNodes.map((node) => [node.id, node] as const))
  const stabilizedPromptNodes = new Map(
    stabilized.promptNodes.map((node) => [node.id, node] as const),
  )
  const capturedFusionNodes = new Map(captured.fusionNodes.map((node) => [node.id, node] as const))
  const stabilizedFusionNodes = new Map(
    stabilized.fusionNodes.map((node) => [node.id, node] as const),
  )
  const rebased: BoardSnapshot = {
    ...current,
    imageNodes: current.imageNodes.map((node) => {
      const capturedNode = capturedImageNodes.get(node.id)
      const stabilizedNode = stabilizedImageNodes.get(node.id)
      if (!capturedNode || !stabilizedNode) return node
      const source = applyStabilizedMediaAsset(
        { url: node.sourceImageUrl, mediaAssetId: node.sourceMediaAssetId },
        { url: capturedNode.sourceImageUrl, mediaAssetId: capturedNode.sourceMediaAssetId },
        { url: stabilizedNode.sourceImageUrl, mediaAssetId: stabilizedNode.sourceMediaAssetId },
      )
      return {
        ...node,
        sourceImageUrl: source.url,
        sourceMediaAssetId: source.mediaAssetId,
        outputImages: rebaseIndexedMediaAssets(
          node.outputImages,
          capturedNode.outputImages,
          stabilizedNode.outputImages,
        ),
      }
    }),
    promptNodes: current.promptNodes.map((node) => {
      const capturedNode = capturedPromptNodes.get(node.id)
      const stabilizedNode = stabilizedPromptNodes.get(node.id)
      return capturedNode && stabilizedNode
        ? {
            ...node,
            inputImages: rebaseInputMediaAssets(
              node.inputImages,
              capturedNode.inputImages,
              stabilizedNode.inputImages,
            ),
          }
        : node
    }),
    fusionNodes: current.fusionNodes.map((node) => {
      const capturedNode = capturedFusionNodes.get(node.id)
      const stabilizedNode = stabilizedFusionNodes.get(node.id)
      return capturedNode && stabilizedNode
        ? {
            ...node,
            inputImages: rebaseInputMediaAssets(
              node.inputImages,
              capturedNode.inputImages,
              stabilizedNode.inputImages,
            ),
            outputImages: rebaseIndexedMediaAssets(
              node.outputImages,
              capturedNode.outputImages,
              stabilizedNode.outputImages,
            ),
          }
        : node
    }),
    mediaRefs: current.mediaRefs?.map((ref) => ({ ...ref })),
  }
  rebuildBoardMediaRefs(rebased, current)
  return rebased
}
