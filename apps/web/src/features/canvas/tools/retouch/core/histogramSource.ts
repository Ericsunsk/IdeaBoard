import type { RetouchEffectLayer, RetouchLayer } from './model.ts'
import { executeImageRetouchPlan } from './engine'
import { buildImageRetouchPlanFromEffectLayers } from './planPipeline'
import {
  flattenVisibleRetouchEffectLayers,
  flattenVisibleRetouchEffectLayersBeforeLayer,
} from './layers/layerEffects'
import { clonePixelBuffer } from './clonePixelBuffer'

type RetouchHistogramSourceCacheKey = {
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
  /*
   * Explicit pending slot. When the target is a staged pending layer (not yet
   * in `layers`), the resolver must not fall into the id-miss path, which
   * accidentally returns the raw source and is only correct when the committed
   * stack is empty. A pending commits just above source, so its input is the
   * full committed stack output. With the deterministic `retouch:<kind>` ids,
   * this yields the same cache key as the post-commit resolution, keeping the
   * mountain frozen across the pending -> committed transition.
   */
  pendingEffectLayer?: RetouchEffectLayer | null
  cachedResolution?: RetouchHistogramSourceResolution | null
}

export function resolveRetouchHistogramSource({
  sourceImageData,
  layers,
  targetLayerId,
  pendingEffectLayer,
  cachedResolution,
}: ResolveRetouchHistogramSourceOptions): RetouchHistogramSourceResolution {
  const isPendingTarget =
    pendingEffectLayer != null &&
    pendingEffectLayer.id === targetLayerId &&
    !layers.some((layer) => layer.id === targetLayerId)
  const upstreamLayers = isPendingTarget
    ? flattenVisibleRetouchEffectLayers(layers)
    : flattenVisibleRetouchEffectLayersBeforeLayer(layers, targetLayerId)
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

  const imageData = clonePixelBuffer(sourceImageData)
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
