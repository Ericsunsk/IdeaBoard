import type { RetouchAdjustmentLayer, RetouchLayer } from '../types'
import {
  compileNormalizedImageAdjustmentPlan,
  normalizeImageAdjustmentStack,
  type ImageAdjustmentPlan,
} from './imageAdjustmentEngine'
import { flattenVisibleRetouchAdjustmentLayers } from './retouchLayerAdjustments'

export function buildImageAdjustmentPlan(layers: readonly RetouchLayer[]): ImageAdjustmentPlan {
  const visibleAdjustmentLayers = flattenVisibleRetouchAdjustmentLayers(layers)
  return buildImageAdjustmentPlanFromAdjustmentLayers(visibleAdjustmentLayers)
}

export function buildImageAdjustmentPlanFromAdjustmentLayers(
  visibleAdjustmentLayers: readonly RetouchAdjustmentLayer[],
): ImageAdjustmentPlan {
  const normalizedAdjustmentStack = normalizeImageAdjustmentStack(visibleAdjustmentLayers)
  return compileNormalizedImageAdjustmentPlan(normalizedAdjustmentStack)
}
