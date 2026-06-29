import type { ImageEditorAdjustmentLayer, ImageEditorLayer } from '../types'
import { executeImageAdjustmentPlan } from './imageAdjustmentEngine'
import { buildImageAdjustmentPlanFromAdjustmentLayers } from './imageAdjustmentPlanPipeline'
import { flattenVisibleImageEditorAdjustmentLayersBeforeLayer } from './imageEditorLayerAdjustments'
import { cloneImageData } from './imageDataUtils'

export type ImageEditorHistogramSourceCacheKey = {
  sourceImageData: ImageData
  targetLayerId: string
  upstreamLayers: readonly ImageEditorAdjustmentLayer[]
}

export type ImageEditorHistogramSourceResolution = {
  imageData: ImageData
  cacheKey: ImageEditorHistogramSourceCacheKey
}

type ResolveImageEditorHistogramSourceOptions = {
  sourceImageData: ImageData
  layers: readonly ImageEditorLayer[]
  targetLayerId: string
  cachedResolution?: ImageEditorHistogramSourceResolution | null
}

export function resolveImageEditorHistogramSource({
  sourceImageData,
  layers,
  targetLayerId,
  cachedResolution,
}: ResolveImageEditorHistogramSourceOptions): ImageEditorHistogramSourceResolution {
  const upstreamLayers = flattenVisibleImageEditorAdjustmentLayersBeforeLayer(layers, targetLayerId)
  const cacheKey: ImageEditorHistogramSourceCacheKey = {
    sourceImageData,
    targetLayerId,
    upstreamLayers,
  }

  if (
    cachedResolution &&
    areSameImageEditorHistogramSourceCacheKeys(cachedResolution.cacheKey, cacheKey)
  ) {
    return cachedResolution
  }

  const imageData = cloneImageData(sourceImageData)
  const plan = buildImageAdjustmentPlanFromAdjustmentLayers(upstreamLayers)
  if (plan.length > 0) executeImageAdjustmentPlan(imageData, plan)

  return {
    imageData,
    cacheKey,
  }
}

export function areSameImageEditorHistogramSourceCacheKeys(
  first: ImageEditorHistogramSourceCacheKey,
  second: ImageEditorHistogramSourceCacheKey,
) {
  return (
    first.sourceImageData === second.sourceImageData &&
    first.targetLayerId === second.targetLayerId &&
    areSameLayerRefs(first.upstreamLayers, second.upstreamLayers)
  )
}

function areSameLayerRefs(
  first: readonly ImageEditorAdjustmentLayer[],
  second: readonly ImageEditorAdjustmentLayer[],
) {
  return first.length === second.length && first.every((layer, index) => layer === second[index])
}
