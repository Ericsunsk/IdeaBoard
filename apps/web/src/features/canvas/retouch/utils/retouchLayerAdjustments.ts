import type { RetouchAdjustmentLayer, RetouchEditableLayer, RetouchLayer } from '../types'
import { hasEffectiveImageAdjustments } from './imageAdjustmentEngine'

export function flattenVisibleRetouchAdjustmentLayers(
  layers: readonly RetouchLayer[],
): RetouchAdjustmentLayer[] {
  return layers.flatMap((layer) => {
    if (layer.type === 'source' || layer.type === 'paint' || !layer.visible) return []
    if (layer.type === 'adjustment') return [layer]
    return flattenVisibleEditableAdjustments(layer.children)
  })
}

export function flattenVisibleRetouchAdjustmentLayersBeforeLayer(
  layers: readonly RetouchLayer[],
  targetLayerId: string,
): RetouchAdjustmentLayer[] {
  const result: RetouchAdjustmentLayer[] = []
  const foundTargetLayer = collectVisibleAdjustmentsBeforeLayer(layers, targetLayerId, result)
  return foundTargetLayer ? result : []
}

export function hasEffectiveRetouchAdjustmentLayers(layers: readonly RetouchLayer[]) {
  return hasEffectiveImageAdjustments(flattenVisibleRetouchAdjustmentLayers(layers))
}

function flattenVisibleEditableAdjustments(
  layers: readonly RetouchEditableLayer[],
): RetouchAdjustmentLayer[] {
  return layers.flatMap((layer) => {
    if (layer.type === 'paint' || !layer.visible) return []
    if (layer.type === 'adjustment') return [layer]
    return flattenVisibleEditableAdjustments(layer.children)
  })
}

function collectVisibleAdjustmentsBeforeLayer(
  layers: readonly (RetouchLayer | RetouchEditableLayer)[],
  targetLayerId: string,
  result: RetouchAdjustmentLayer[],
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
