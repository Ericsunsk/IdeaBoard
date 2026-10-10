import type { RetouchEffectLayer, RetouchLayer } from './model.ts'
import {
  compileNormalizedImageRetouchPlan,
  normalizeImageRetouchStack,
  type ImageRetouchPlan,
} from './engine'
import { flattenVisibleRetouchEffectLayers } from './layers/layerEffects'

export function buildImageRetouchPlan(layers: readonly RetouchLayer[]): ImageRetouchPlan {
  const visibleEffectLayers = flattenVisibleRetouchEffectLayers(layers)
  return buildImageRetouchPlanFromEffectLayers(visibleEffectLayers)
}

export function buildImageRetouchPlanFromEffectLayers(
  visibleEffectLayers: readonly RetouchEffectLayer[],
): ImageRetouchPlan {
  const normalizedRetouchStack = normalizeImageRetouchStack(visibleEffectLayers)
  return compileNormalizedImageRetouchPlan(normalizedRetouchStack)
}
