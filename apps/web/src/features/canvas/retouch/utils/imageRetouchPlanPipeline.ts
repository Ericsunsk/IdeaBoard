import type { RetouchEffectLayer, RetouchLayer } from '../types'
import {
  compileNormalizedImageRetouchPlan,
  normalizeImageRetouchStack,
  type ImageRetouchPlan,
} from './imageRetouchEngine'
import { flattenVisibleRetouchEffectLayers } from './retouchLayerEffects'

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
