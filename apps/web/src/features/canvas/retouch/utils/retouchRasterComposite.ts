import type { RetouchEditableLayer, RetouchLayer, RetouchRasterContent } from '../types'
import { applyImageAdjustmentsInPlace } from './imageAdjustmentEngine'
import { cloneImageData } from './imageDataUtils'

export type RetouchRasterLayerImageProvider = (
  content: RetouchRasterContent,
) => ImageData | null

export type RetouchRasterCompositeOptions = {
  getRasterImageData?: RetouchRasterLayerImageProvider
  overrides?: ReadonlyMap<string, ImageData>
}

type CompositeContext = Required<RetouchRasterCompositeOptions> & {
  sourceImageData: ImageData
  output: ImageData
}

export function createTransparentImageData(width: number, height: number): ImageData {
  return new ImageData(new Uint8ClampedArray(width * height * 4), width, height)
}

export function compositeRetouchLayerStack(
  sourceImageData: ImageData,
  layers: readonly RetouchLayer[],
  options: RetouchRasterCompositeOptions = {},
): ImageData {
  const context: CompositeContext = {
    sourceImageData,
    output: cloneImageData(sourceImageData),
    getRasterImageData: options.getRasterImageData ?? (() => null),
    overrides: options.overrides ?? new Map<string, ImageData>(),
  }

  applyLayerSequenceBottomToTop(layers, context)
  return context.output
}

function applyLayerSequenceBottomToTop(
  layers: readonly (RetouchLayer | RetouchEditableLayer)[],
  context: CompositeContext,
) {
  for (let index = layers.length - 1; index >= 0; index -= 1) {
    const layer = layers[index]
    if (!layer) continue
    applyLayerToComposite(layer, context)
  }
}

function applyLayerToComposite(
  layer: RetouchLayer | RetouchEditableLayer,
  context: CompositeContext,
) {
  switch (layer.type) {
    case 'source': {
      const sourceOverride =
        context.overrides.get(layer.id) ?? readRasterContent(layer.content, context)
      if (!sourceOverride) return
      copyImageDataInto(context.output, sourceOverride)
      return
    }
    case 'paint': {
      if (!layer.visible) return
      const imageData = context.overrides.get(layer.id) ?? readRasterContent(layer.content, context)
      if (!imageData) return
      alphaCompositeImageData(context.output, imageData)
      return
    }
    case 'adjustment':
      if (!layer.visible) return
      applyImageAdjustmentsInPlace(context.output, [layer])
      return
    case 'merged':
      if (!layer.visible) return
      applyLayerSequenceBottomToTop(layer.children, context)
  }
}

export function alphaCompositeImageData(base: ImageData, overlay: ImageData): ImageData {
  const width = Math.min(base.width, overlay.width)
  const height = Math.min(base.height, overlay.height)
  const baseData = base.data
  const overlayData = overlay.data

  for (let y = 0; y < height; y += 1) {
    for (let x = 0; x < width; x += 1) {
      const baseOffset = (y * base.width + x) * 4
      const overlayOffset = (y * overlay.width + x) * 4
      const overlayAlpha = (overlayData[overlayOffset + 3] ?? 0) / 255
      if (overlayAlpha <= 0) continue

      const baseAlpha = (baseData[baseOffset + 3] ?? 0) / 255
      const outputAlpha = overlayAlpha + baseAlpha * (1 - overlayAlpha)
      if (outputAlpha <= 0) {
        baseData[baseOffset] = 0
        baseData[baseOffset + 1] = 0
        baseData[baseOffset + 2] = 0
        baseData[baseOffset + 3] = 0
        continue
      }

      baseData[baseOffset] = Math.round(
        ((overlayData[overlayOffset] ?? 0) * overlayAlpha +
          (baseData[baseOffset] ?? 0) * baseAlpha * (1 - overlayAlpha)) /
          outputAlpha,
      )
      baseData[baseOffset + 1] = Math.round(
        ((overlayData[overlayOffset + 1] ?? 0) * overlayAlpha +
          (baseData[baseOffset + 1] ?? 0) * baseAlpha * (1 - overlayAlpha)) /
          outputAlpha,
      )
      baseData[baseOffset + 2] = Math.round(
        ((overlayData[overlayOffset + 2] ?? 0) * overlayAlpha +
          (baseData[baseOffset + 2] ?? 0) * baseAlpha * (1 - overlayAlpha)) /
          outputAlpha,
      )
      baseData[baseOffset + 3] = Math.round(outputAlpha * 255)
    }
  }

  return base
}

function readRasterContent(
  content: { kind: 'empty' } | RetouchRasterContent | undefined,
  context: CompositeContext,
) {
  if (!content || content.kind !== 'raster') return null
  return context.getRasterImageData(content)
}

function copyImageDataInto(target: ImageData, source: ImageData) {
  if (target.width === source.width && target.height === source.height) {
    target.data.set(source.data)
    return
  }

  const width = Math.min(target.width, source.width)
  const height = Math.min(target.height, source.height)
  target.data.fill(0)
  for (let y = 0; y < height; y += 1) {
    const targetStart = y * target.width * 4
    const sourceStart = y * source.width * 4
    target.data.set(source.data.slice(sourceStart, sourceStart + width * 4), targetStart)
  }
}
