import type {
  CanvasNodeItem,
  FusionNodeItem,
  ImageNodeItem,
  PromptAspectRatio,
  PromptNodeInputImage,
  PromptNodeItem,
} from '../../canvas-board/domain'
import { normalizeOutputResolution, type OutputResolution } from '../../../shared/domain/generationContract'
import {
  areFusionBlendPointsEqual,
  requireImageNodeOutputCount,
  constrainFusionBlendPoint,
  getCanvasNodeImageInputLimit,
  normalizePromptAspectRatio,
  normalizeFusionBlendPoint,
  type FusionBlendPoint,
} from '../../canvas-board/domain'

export type FlowSnapshotParamPatch =
  | { key: 'title'; value: string }
  | { key: 'outputCount'; value: number }
  | { key: 'outputResolution'; value: OutputResolution }
  | { key: 'prompt'; value: string }
  | { key: 'aspectRatio'; value: PromptAspectRatio }
  | { key: 'inputImages'; value: PromptNodeInputImage[] }
  | { key: 'blendPoint'; value: FusionBlendPoint }

type NodeWithOutputCount = ImageNodeItem | PromptNodeItem | FusionNodeItem
type NodeWithAspectRatio = PromptNodeItem | FusionNodeItem
type NodeWithInputImages = PromptNodeItem | FusionNodeItem

function hasOutputCount(snapshotNode: CanvasNodeItem): snapshotNode is NodeWithOutputCount {
  return 'outputCount' in snapshotNode
}

function hasAspectRatio(snapshotNode: CanvasNodeItem): snapshotNode is NodeWithAspectRatio {
  return snapshotNode.type === 'prompt' || snapshotNode.type === 'fusion'
}

function hasInputImages(snapshotNode: CanvasNodeItem): snapshotNode is NodeWithInputImages {
  return snapshotNode.type === 'prompt' || snapshotNode.type === 'fusion'
}

function isPromptNodeInputImage(value: unknown): value is PromptNodeInputImage {
  if (!value || typeof value !== 'object') return false
  const item = value as Partial<PromptNodeInputImage>
  return (
    typeof item.id === 'string' &&
    item.id.length > 0 &&
    (item.sourceType === 'node' || item.sourceType === 'external') &&
    typeof item.sourceId === 'string' &&
    item.sourceId.length > 0 &&
    (item.sourceNodeId === undefined || typeof item.sourceNodeId === 'string') &&
    typeof item.url === 'string' &&
    item.url.trim().length > 0 &&
    typeof item.ratio === 'number' &&
    Number.isFinite(item.ratio) &&
    item.ratio > 0 &&
    (item.mediaAssetId === undefined ||
      (Number.isInteger(item.mediaAssetId) && item.mediaAssetId > 0))
  )
}

function normalizePromptInputImages(value: unknown, inputLimit: number): PromptNodeInputImage[] {
  if (!Array.isArray(value)) return []
  return value
    .filter(isPromptNodeInputImage)
    .map((item) => ({
      id: item.id,
      sourceType: item.sourceType,
      sourceId: item.sourceId,
      sourceNodeId: item.sourceNodeId,
      url: item.url.trim(),
      ratio: item.ratio,
      ...(item.mediaAssetId === undefined ? {} : { mediaAssetId: item.mediaAssetId }),
    }))
    .slice(0, inputLimit)
}

function arePromptInputImagesEqual(
  a: unknown,
  b: PromptNodeInputImage[],
  inputLimit: number,
): boolean {
  const current = normalizePromptInputImages(a, inputLimit)
  return (
    current.length === b.length &&
    current.every(
      (item, index) =>
        item.id === b[index]?.id &&
        item.sourceType === b[index]?.sourceType &&
        item.sourceId === b[index]?.sourceId &&
        item.sourceNodeId === b[index]?.sourceNodeId &&
        item.url === b[index]?.url &&
        item.ratio === b[index]?.ratio &&
        item.mediaAssetId === b[index]?.mediaAssetId,
    )
  )
}
export function patchFlowSnapshotParam(
  snapshotNode: CanvasNodeItem,
  payload: FlowSnapshotParamPatch,
): CanvasNodeItem {
  const { key, value } = payload
  switch (key) {
    case 'title': {
      const nextTitle = String(value ?? '').trim()
      if (!nextTitle || snapshotNode.title === nextTitle) return snapshotNode
      return { ...snapshotNode, title: nextTitle }
    }
    case 'outputCount': {
      if (!hasOutputCount(snapshotNode)) return snapshotNode
      const nextOutputCount = requireImageNodeOutputCount(value)
      if (snapshotNode.outputCount === nextOutputCount) return snapshotNode
      return { ...snapshotNode, outputCount: nextOutputCount }
    }
    case 'outputResolution': {
      const nextResolution = normalizeOutputResolution(value)
      if (snapshotNode.outputResolution === nextResolution) return snapshotNode
      return { ...snapshotNode, outputResolution: nextResolution }
    }
    case 'prompt': {
      if (snapshotNode.type !== 'fusion') return snapshotNode
      const nextPrompt = String(value ?? '').trim()
      if (snapshotNode.prompt === nextPrompt) return snapshotNode
      return { ...snapshotNode, prompt: nextPrompt }
    }
    case 'aspectRatio': {
      if (!hasAspectRatio(snapshotNode)) return snapshotNode
      const nextAspectRatio = normalizePromptAspectRatio(value)
      if (snapshotNode.aspectRatio === nextAspectRatio) return snapshotNode
      return { ...snapshotNode, aspectRatio: nextAspectRatio }
    }
    case 'inputImages': {
      if (!hasInputImages(snapshotNode)) return snapshotNode
      const inputLimit = getCanvasNodeImageInputLimit(snapshotNode.type)
      const nextInputImages = normalizePromptInputImages(value, inputLimit)
      const inputUnchanged = arePromptInputImagesEqual(
        snapshotNode.inputImages,
        nextInputImages,
        inputLimit,
      )
      if (snapshotNode.type !== 'fusion') {
        if (inputUnchanged) return snapshotNode
        return { ...snapshotNode, inputImages: nextInputImages }
      }

      const nextBlendPoint = constrainFusionBlendPoint(
        snapshotNode.blendPoint,
        nextInputImages.length,
        snapshotNode.width,
      )
      const currentBlendPoint = normalizeFusionBlendPoint(snapshotNode.blendPoint)
      const blendUnchanged = areFusionBlendPointsEqual(currentBlendPoint, nextBlendPoint)
      if (inputUnchanged && blendUnchanged) return snapshotNode
      return {
        ...snapshotNode,
        inputImages: nextInputImages,
        ...(blendUnchanged ? {} : { blendPoint: nextBlendPoint }),
      }
    }
    case 'blendPoint': {
      if (snapshotNode.type !== 'fusion') return snapshotNode
      const nextBlendPoint = constrainFusionBlendPoint(
        value,
        Array.isArray(snapshotNode.inputImages) ? snapshotNode.inputImages.length : 0,
        snapshotNode.width,
      )
      const currentBlendPoint = normalizeFusionBlendPoint(snapshotNode.blendPoint)
      if (areFusionBlendPointsEqual(currentBlendPoint, nextBlendPoint)) return snapshotNode
      return { ...snapshotNode, blendPoint: nextBlendPoint }
    }
    default:
      return snapshotNode
  }
}
