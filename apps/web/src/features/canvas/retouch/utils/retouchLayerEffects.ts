import type { RetouchEffectLayer, RetouchEditableLayer, RetouchLayer } from '../types'
import { hasEffectiveImageRetouch } from './imageRetouchEngine'

export function flattenVisibleRetouchEffectLayers(
  layers: readonly RetouchLayer[],
): RetouchEffectLayer[] {
  return layers.flatMap((layer) => {
    if (layer.type === 'source' || layer.type === 'paint' || !layer.visible) return []
    if (layer.type === 'effect') return [layer]
    return flattenVisibleEditableEffects(layer.children)
  })
}

export function flattenVisibleRetouchEffectLayersBeforeLayer(
  layers: readonly RetouchLayer[],
  targetLayerId: string,
): RetouchEffectLayer[] {
  const result: RetouchEffectLayer[] = []
  const foundTargetLayer = collectVisibleEffectsBeforeLayer(layers, targetLayerId, result)
  return foundTargetLayer ? result : []
}

export function hasEffectiveRetouchEffectLayers(layers: readonly RetouchLayer[]) {
  return hasEffectiveImageRetouch(flattenVisibleRetouchEffectLayers(layers))
}

function flattenVisibleEditableEffects(
  layers: readonly RetouchEditableLayer[],
): RetouchEffectLayer[] {
  return layers.flatMap((layer) => {
    if (layer.type === 'paint' || !layer.visible) return []
    if (layer.type === 'effect') return [layer]
    return flattenVisibleEditableEffects(layer.children)
  })
}

function collectVisibleEffectsBeforeLayer(
  layers: readonly (RetouchLayer | RetouchEditableLayer)[],
  targetLayerId: string,
  result: RetouchEffectLayer[],
): boolean {
  for (const layer of layers) {
    if (layer.id === targetLayerId) return true
    if (layer.type === 'source' || layer.type === 'paint' || !layer.visible) continue
    if (layer.type === 'effect') {
      result.push(layer)
      continue
    }
    if (collectVisibleEffectsBeforeLayer(layer.children, targetLayerId, result)) return true
  }
  return false
}
