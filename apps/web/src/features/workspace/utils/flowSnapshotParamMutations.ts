import type { PromptNodeInputImage } from '../types/canvasNodes'
import { normalizePromptAspectRatio } from '../config/promptAspectRatio'
import { clampImageNodeOutputCount } from '../types/canvasNodes'
import {
  areFusionBlendPointsEqual,
  constrainFusionBlendPoint,
  normalizeFusionBlendPoint,
} from './fusionBlend'

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
    item.ratio > 0
  )
}

function normalizePromptInputImages(value: unknown): PromptNodeInputImage[] {
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
    }))
    .slice(0, 4)
}

function arePromptInputImagesEqual(a: unknown, b: PromptNodeInputImage[]): boolean {
  const current = normalizePromptInputImages(a)
  return (
    current.length === b.length &&
    current.every(
      (item, index) =>
        item.id === b[index]?.id &&
        item.sourceType === b[index]?.sourceType &&
        item.sourceId === b[index]?.sourceId &&
        item.sourceNodeId === b[index]?.sourceNodeId &&
        item.url === b[index]?.url &&
        item.ratio === b[index]?.ratio,
    )
  )
}
export function patchFlowSnapshotParam(
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- snapshotNode is a union of 3 node shapes handled via switch(key); can't be statically narrowed at this boundary
  snapshotNode: Record<string, any>,
  payload: { key: string; value: unknown },
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- return type mirrors input shape
): Record<string, any> {
  const { key, value } = payload
  switch (key) {
    case 'title': {
      const nextTitle = String(value ?? '').trim()
      if (!nextTitle || snapshotNode.title === nextTitle) return snapshotNode
      return { ...snapshotNode, title: nextTitle }
    }
    case 'outputCount': {
      const nextOutputCount = clampImageNodeOutputCount(value)
      if (snapshotNode.outputCount === nextOutputCount) return snapshotNode
      return { ...snapshotNode, outputCount: nextOutputCount }
    }
    case 'prompt': {
      const nextPrompt = String(value ?? '').trim()
      if (snapshotNode.type !== 'fusion' && !nextPrompt) return snapshotNode
      if (snapshotNode.prompt === nextPrompt) return snapshotNode
      return { ...snapshotNode, prompt: nextPrompt }
    }
    case 'aspectRatio': {
      const nextAspectRatio = normalizePromptAspectRatio(value)
      if (snapshotNode.aspectRatio === nextAspectRatio) return snapshotNode
      return { ...snapshotNode, aspectRatio: nextAspectRatio }
    }
    case 'inputImages': {
      const nextInputImages = normalizePromptInputImages(value)
      const inputUnchanged = arePromptInputImagesEqual(snapshotNode.inputImages, nextInputImages)
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
    case 'strength': {
      const nextStrength = Math.max(0, Math.min(100, Math.round(Number(value) || 0)))
      if (snapshotNode.strength === nextStrength) return snapshotNode
      return { ...snapshotNode, strength: nextStrength }
    }
    default:
      return snapshotNode
  }
}
