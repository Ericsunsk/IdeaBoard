import type { RetouchAdjustmentLayer, RetouchLayer } from '../types'
import { executeImageAdjustmentPlan } from './imageAdjustmentEngine'
import { buildImageAdjustmentPlanFromAdjustmentLayers } from './imageAdjustmentPlanPipeline'
import { flattenVisibleRetouchAdjustmentLayersBeforeLayer } from './retouchLayerAdjustments'
import { cloneImageData } from './imageDataUtils'

export type RetouchHistogramSourceCacheKey = {
  sourceImageData: ImageData
  targetLayerId: string
  upstreamLayers: readonly RetouchAdjustmentLayer[]
}

export type RetouchHistogramSourceResolution = {
  imageData: ImageData
  cacheKey: RetouchHistogramSourceCacheKey
}

type ResolveRetouchHistogramSourceOptions = {
  sourceImageData: ImageData
  layers: readonly RetouchLayer[]
  targetLayerId: string
  cachedResolution?: RetouchHistogramSourceResolution | null
}

export function resolveRetouchHistogramSource({
  sourceImageData,
  layers,
  targetLayerId,
  cachedResolution,
}: ResolveRetouchHistogramSourceOptions): RetouchHistogramSourceResolution {
  const upstreamLayers = flattenVisibleRetouchAdjustmentLayersBeforeLayer(layers, targetLayerId)
  const cacheKey: RetouchHistogramSourceCacheKey = {
    sourceImageData,
    targetLayerId,
    upstreamLayers,
  }

  if (
    cachedResolution &&
    areSameRetouchHistogramSourceCacheKeys(cachedResolution.cacheKey, cacheKey)
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

function areSameRetouchHistogramSourceCacheKeys(
  first: RetouchHistogramSourceCacheKey,
  second: RetouchHistogramSourceCacheKey,
) {
  return (
    first.sourceImageData === second.sourceImageData &&
    first.targetLayerId === second.targetLayerId &&
    areSameLayerRefs(first.upstreamLayers, second.upstreamLayers)
  )
}

function areSameLayerRefs(
  first: readonly RetouchAdjustmentLayer[],
  second: readonly RetouchAdjustmentLayer[],
) {
  return first.length === second.length && first.every((layer, index) => layer === second[index])
}
