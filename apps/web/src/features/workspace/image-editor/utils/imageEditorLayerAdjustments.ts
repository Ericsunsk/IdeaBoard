import type {
  ImageEditorAdjustmentLayer,
  ImageEditorEditableLayer,
  ImageEditorLayer,
} from '../types'

export function flattenVisibleImageEditorAdjustmentLayers(
  layers: readonly ImageEditorLayer[],
): ImageEditorAdjustmentLayer[] {
  return layers.flatMap((layer) => {
    if (layer.type === 'source' || layer.type === 'paint' || !layer.visible) return []
    if (layer.type === 'adjustment') return [layer]
    return flattenVisibleEditableAdjustments(layer.children)
  })
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
