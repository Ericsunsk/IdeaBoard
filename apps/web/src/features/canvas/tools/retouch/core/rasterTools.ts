import type { RetouchLayer, RetouchRasterContent } from './model.ts'

export function hasRetouchRasterEdits(layers: readonly RetouchLayer[]): boolean {
  return layers.some(hasRasterContentInLayer)
}

export function flattenRetouchRasterContents(
  layers: readonly RetouchLayer[],
): RetouchRasterContent[] {
  return layers.flatMap((layer) => {
    if (layer.type === 'source') return layer.content?.kind === 'raster' ? [layer.content] : []
    if (layer.type === 'paint') return layer.content.kind === 'raster' ? [layer.content] : []
    if (layer.type === 'merged') return flattenRetouchRasterContents(layer.children)
    return []
  })
}

function hasRasterContentInLayer(layer: RetouchLayer): boolean {
  if (layer.type === 'source') return layer.content?.kind === 'raster'
  if (layer.type === 'paint') return layer.content.kind === 'raster'
  if (layer.type === 'merged') return layer.children.some(hasRasterContentInLayer)
  return false
}
