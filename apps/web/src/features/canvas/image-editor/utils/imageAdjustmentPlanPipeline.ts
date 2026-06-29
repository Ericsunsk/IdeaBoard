import type { ImageEditorAdjustmentLayer, ImageEditorLayer } from '../types'
import {
  compileNormalizedImageAdjustmentPlan,
  normalizeImageAdjustmentStack,
  type ImageAdjustmentPlan,
} from './imageAdjustmentEngine'
import { flattenVisibleImageEditorAdjustmentLayers } from './imageEditorLayerAdjustments'

export function buildImageAdjustmentPlan(layers: readonly ImageEditorLayer[]): ImageAdjustmentPlan {
  const visibleAdjustmentLayers = flattenVisibleImageEditorAdjustmentLayers(layers)
  return buildImageAdjustmentPlanFromAdjustmentLayers(visibleAdjustmentLayers)
}

export function buildImageAdjustmentPlanFromAdjustmentLayers(
  visibleAdjustmentLayers: readonly ImageEditorAdjustmentLayer[],
): ImageAdjustmentPlan {
  const normalizedAdjustmentStack = normalizeImageAdjustmentStack(visibleAdjustmentLayers)
  return compileNormalizedImageAdjustmentPlan(normalizedAdjustmentStack)
}
