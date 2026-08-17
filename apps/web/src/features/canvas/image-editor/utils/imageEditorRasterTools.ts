import type { ImageEditorLayer, ImageEditorRasterContent } from '../types'

export function hasImageEditorRasterEdits(layers: readonly ImageEditorLayer[]): boolean {
  return layers.some(hasRasterContentInLayer)
}

export function flattenImageEditorRasterContents(
  layers: readonly ImageEditorLayer[],
): ImageEditorRasterContent[] {
  return layers.flatMap((layer) => {
    if (layer.type === 'source') return layer.content?.kind === 'raster' ? [layer.content] : []
    if (layer.type === 'paint') return layer.content.kind === 'raster' ? [layer.content] : []
    if (layer.type === 'merged') return flattenImageEditorRasterContents(layer.children)
    return []
  })
}

function hasRasterContentInLayer(layer: ImageEditorLayer): boolean {
  if (layer.type === 'source') return layer.content?.kind === 'raster'
  if (layer.type === 'paint') return layer.content.kind === 'raster'
  if (layer.type === 'merged') return layer.children.some(hasRasterContentInLayer)
  return false
}
