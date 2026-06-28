import type { ImageEditorLayer } from '../types'
import {
  compileNormalizedImageAdjustmentPlan,
  normalizeImageAdjustmentStack,
  type ImageAdjustmentPlan,
} from './imageAdjustmentEngine'
import { flattenVisibleImageEditorAdjustmentLayers } from './imageEditorLayerAdjustments'

export function buildImageAdjustmentPlan(layers: readonly ImageEditorLayer[]): ImageAdjustmentPlan {
  const visibleAdjustmentLayers = flattenVisibleImageEditorAdjustmentLayers(layers)
  const normalizedAdjustmentStack = normalizeImageAdjustmentStack(visibleAdjustmentLayers)
  return compileNormalizedImageAdjustmentPlan(normalizedAdjustmentStack)
}
