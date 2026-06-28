import type {
  ImageEditorAdjustmentLayer,
  ImageEditorEditableLayer,
  ImageEditorLayer,
} from '../types'
import { hasEffectiveImageAdjustments } from './imageAdjustmentEngine'

export function flattenVisibleImageEditorAdjustmentLayers(
  layers: readonly ImageEditorLayer[],
): ImageEditorAdjustmentLayer[] {
  return layers.flatMap((layer) => {
    if (layer.type === 'source' || layer.type === 'paint' || !layer.visible) return []
    if (layer.type === 'adjustment') return [layer]
    return flattenVisibleEditableAdjustments(layer.children)
  })
}

export function flattenVisibleImageEditorAdjustmentLayersBeforeLayer(
  layers: readonly ImageEditorLayer[],
  targetLayerId: string,
): ImageEditorAdjustmentLayer[] {
  const result: ImageEditorAdjustmentLayer[] = []
  const foundTargetLayer = collectVisibleAdjustmentsBeforeLayer(layers, targetLayerId, result)
  return foundTargetLayer ? result : []
}

export function hasEffectiveImageEditorAdjustmentLayers(layers: readonly ImageEditorLayer[]) {
  return hasEffectiveImageAdjustments(flattenVisibleImageEditorAdjustmentLayers(layers))
}

function flattenVisibleEditableAdjustments(
  layers: readonly ImageEditorEditableLayer[],
): ImageEditorAdjustmentLayer[] {
  return layers.flatMap((layer) => {
    if (layer.type === 'paint' || !layer.visible) return []
    if (layer.type === 'adjustment') return [layer]
    return flattenVisibleEditableAdjustments(layer.children)
  })
}

function collectVisibleAdjustmentsBeforeLayer(
  layers: readonly (ImageEditorLayer | ImageEditorEditableLayer)[],
  targetLayerId: string,
  result: ImageEditorAdjustmentLayer[],
): boolean {
  for (const layer of layers) {
    if (layer.id === targetLayerId) return true
    if (layer.type === 'source' || layer.type === 'paint' || !layer.visible) continue
    if (layer.type === 'adjustment') {
      result.push(layer)
      continue
    }
    if (collectVisibleAdjustmentsBeforeLayer(layer.children, targetLayerId, result)) return true
  }
  return false
}
