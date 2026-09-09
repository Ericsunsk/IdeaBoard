import type { RetouchEffectLayer, RetouchLayer } from '../types'
import { executeImageRetouchPlan } from './retouchEngine'
import { buildImageRetouchPlanFromEffectLayers } from './retouchPlanPipeline'
import { flattenVisibleRetouchEffectLayersBeforeLayer } from './retouchLayerEffects'
import { cloneImageData } from './imageDataUtils'

export type RetouchHistogramSourceCacheKey = {
  sourceImageData: ImageData
  targetLayerId: string
  upstreamLayers: readonly RetouchEffectLayer[]
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
  const upstreamLayers = flattenVisibleRetouchEffectLayersBeforeLayer(layers, targetLayerId)
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
  const plan = buildImageRetouchPlanFromEffectLayers(upstreamLayers)
  if (plan.length > 0) executeImageRetouchPlan(imageData, plan)

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
  first: readonly RetouchEffectLayer[],
  second: readonly RetouchEffectLayer[],
) {
  return first.length === second.length && first.every((layer, index) => layer === second[index])
}
