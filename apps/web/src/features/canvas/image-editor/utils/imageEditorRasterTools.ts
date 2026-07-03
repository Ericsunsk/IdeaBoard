import type {
  ImageEditorLayer,
  ImageEditorPaintLayer,
  ImageEditorRasterContent,
  ImageEditorSourceLayer,
  ImageEditorTool,
} from '../types'

export type ImageEditorRasterTool = Extract<ImageEditorTool, 'brush' | 'eraser'>

export type ImageEditorRasterToolSettings = {
  size: number
  hardness: number
  brushOpacity: number
  eraserOpacity: number
  brushColor: string
}

export type ImageEditorRasterStrokePoint = {
  x: number
  y: number
}

export type ImageEditorRasterStrokeInput = {
  tool: ImageEditorRasterTool
  points: readonly ImageEditorRasterStrokePoint[]
  pointGroups?: readonly (readonly ImageEditorRasterStrokePoint[])[]
  size: number
  hardness: number
  opacity: number
  color?: string
}

export const IMAGE_EDITOR_RASTER_HISTORY_LIMIT = 10
export const IMAGE_EDITOR_RASTER_DEFAULT_SIZE = 32
export const IMAGE_EDITOR_RASTER_MIN_SIZE = 1
export const IMAGE_EDITOR_RASTER_MAX_SIZE = 512
export const IMAGE_EDITOR_RASTER_DEFAULT_HARDNESS = 80
export const IMAGE_EDITOR_RASTER_MIN_HARDNESS = 0
export const IMAGE_EDITOR_RASTER_MAX_HARDNESS = 100
export const IMAGE_EDITOR_RASTER_DEFAULT_OPACITY = 100
export const IMAGE_EDITOR_RASTER_MIN_OPACITY = 1
export const IMAGE_EDITOR_RASTER_MAX_OPACITY = 100
export const IMAGE_EDITOR_RASTER_DEFAULT_COLOR = '#000000'

const HEX_COLOR_PATTERN = /^#(?:[0-9a-f]{3}|[0-9a-f]{6})$/i

export function createDefaultImageEditorRasterToolSettings(): ImageEditorRasterToolSettings {
  return {
    size: IMAGE_EDITOR_RASTER_DEFAULT_SIZE,
    hardness: IMAGE_EDITOR_RASTER_DEFAULT_HARDNESS,
    brushOpacity: IMAGE_EDITOR_RASTER_DEFAULT_OPACITY,
    eraserOpacity: IMAGE_EDITOR_RASTER_DEFAULT_OPACITY,
    brushColor: IMAGE_EDITOR_RASTER_DEFAULT_COLOR,
  }
}

export function clampImageEditorRasterSize(value: number) {
  return clampNumber(
    value,
    IMAGE_EDITOR_RASTER_MIN_SIZE,
    IMAGE_EDITOR_RASTER_MAX_SIZE,
    IMAGE_EDITOR_RASTER_DEFAULT_SIZE,
  )
}

export function clampImageEditorRasterHardness(value: number) {
  return clampNumber(
    value,
    IMAGE_EDITOR_RASTER_MIN_HARDNESS,
    IMAGE_EDITOR_RASTER_MAX_HARDNESS,
    IMAGE_EDITOR_RASTER_DEFAULT_HARDNESS,
  )
}

export function clampImageEditorRasterOpacity(value: number) {
  return clampNumber(
    value,
    IMAGE_EDITOR_RASTER_MIN_OPACITY,
    IMAGE_EDITOR_RASTER_MAX_OPACITY,
    IMAGE_EDITOR_RASTER_DEFAULT_OPACITY,
  )
}

export function normalizeImageEditorRasterColor(value: string) {
  const next = value.trim()
  if (!HEX_COLOR_PATTERN.test(next)) return IMAGE_EDITOR_RASTER_DEFAULT_COLOR
  if (next.length === 4) {
    const [, r = '0', g = '0', b = '0'] = next
    return `#${r}${r}${g}${g}${b}${b}`.toLowerCase()
  }
  return next.toLowerCase()
}

export function isImageEditorRasterTool(tool: ImageEditorTool): tool is ImageEditorRasterTool {
  return tool === 'brush' || tool === 'eraser'
}

export function isImageEditorRasterLayer(
  layer: ImageEditorLayer | null | undefined,
): layer is ImageEditorSourceLayer | ImageEditorPaintLayer {
  return layer?.type === 'source' || layer?.type === 'paint'
}

export function isVisibleImageEditorRasterLayer(layer: ImageEditorLayer | null | undefined) {
  if (!isImageEditorRasterLayer(layer)) return false
  return layer.type === 'source' || layer.visible
}

export function getSelectedImageEditorLayer(
  layers: readonly ImageEditorLayer[],
  selectedLayerId: string,
) {
  return layers.find((layer) => layer.id === selectedLayerId) ?? null
}

export function canUseImageEditorRasterTool(
  layers: readonly ImageEditorLayer[],
  selectedLayerId: string,
) {
  return isVisibleImageEditorRasterLayer(getSelectedImageEditorLayer(layers, selectedLayerId))
}

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

export function applyImageEditorRasterStroke(
  imageData: ImageData,
  input: ImageEditorRasterStrokeInput,
): boolean {
  const strokePointGroups = input.pointGroups ?? [input.points]
  const points = strokePointGroups.flatMap((group) =>
    interpolateImageEditorRasterStrokePoints(group, clampImageEditorRasterSize(input.size)),
  )
  if (points.length === 0) return false

  const size = clampImageEditorRasterSize(input.size)
  const radius = size / 2
  const hardness = clampImageEditorRasterHardness(input.hardness)
  const opacity = clampImageEditorRasterOpacity(input.opacity) / 100
  const coverage = createStrokeCoverageMask(
    imageData.width,
    imageData.height,
    points,
    radius,
    hardness,
  )
  const color = parseBrushColor(input.color)
  const data = imageData.data
  let changed = false

  for (let pixelIndex = 0; pixelIndex < coverage.length; pixelIndex += 1) {
    const dabCoverage = coverage[pixelIndex] ?? 0
    if (dabCoverage <= 0) continue

    const alpha = dabCoverage * opacity
    const offset = pixelIndex * 4
    if (input.tool === 'eraser') {
      const previousAlpha = data[offset + 3] ?? 0
      const nextAlpha = Math.round(previousAlpha * (1 - alpha))
      if (nextAlpha !== previousAlpha) changed = true
      data[offset + 3] = nextAlpha
      continue
    }

    const previousRed = data[offset] ?? 0
    const previousGreen = data[offset + 1] ?? 0
    const previousBlue = data[offset + 2] ?? 0
    const previousAlpha = (data[offset + 3] ?? 0) / 255
    const sourceAlpha = alpha
    const outputAlpha = sourceAlpha + previousAlpha * (1 - sourceAlpha)

    if (outputAlpha <= 0) continue

    const nextRed = Math.round(
      (color.r * sourceAlpha + previousRed * previousAlpha * (1 - sourceAlpha)) / outputAlpha,
    )
    const nextGreen = Math.round(
      (color.g * sourceAlpha + previousGreen * previousAlpha * (1 - sourceAlpha)) / outputAlpha,
    )
    const nextBlue = Math.round(
      (color.b * sourceAlpha + previousBlue * previousAlpha * (1 - sourceAlpha)) / outputAlpha,
    )
    const nextAlpha = Math.round(outputAlpha * 255)

    if (
      nextRed !== previousRed ||
      nextGreen !== previousGreen ||
      nextBlue !== previousBlue ||
      nextAlpha !== data[offset + 3]
    ) {
      changed = true
    }
    data[offset] = nextRed
    data[offset + 1] = nextGreen
    data[offset + 2] = nextBlue
    data[offset + 3] = nextAlpha
  }

  return changed
}

export function interpolateImageEditorRasterStrokePoints(
  points: readonly ImageEditorRasterStrokePoint[],
  size: number,
): ImageEditorRasterStrokePoint[] {
  if (points.length === 0) return []
  const spacing = Math.max(1, clampImageEditorRasterSize(size) * 0.25)
  const result: ImageEditorRasterStrokePoint[] = []
  for (let index = 0; index < points.length; index += 1) {
    const point = points[index]
    if (!point) continue
    if (index === 0) {
      result.push(point)
      continue
    }
    const previous = points[index - 1]
    if (!previous) continue
    const distance = Math.hypot(point.x - previous.x, point.y - previous.y)
    const steps = Math.max(1, Math.ceil(distance / spacing))
    for (let step = 1; step <= steps; step += 1) {
      const t = step / steps
      result.push({
        x: previous.x + (point.x - previous.x) * t,
        y: previous.y + (point.y - previous.y) * t,
      })
    }
  }
  return result
}

function hasRasterContentInLayer(layer: ImageEditorLayer): boolean {
  if (layer.type === 'source') return layer.content?.kind === 'raster'
  if (layer.type === 'paint') return layer.content.kind === 'raster'
  if (layer.type === 'merged') return layer.children.some(hasRasterContentInLayer)
  return false
}

function createStrokeCoverageMask(
  width: number,
  height: number,
  points: readonly ImageEditorRasterStrokePoint[],
  radius: number,
  hardness: number,
) {
  const coverage = new Float32Array(width * height)
  const hardnessRatio = hardness / 100
  const innerRadius = radius * hardnessRatio
  const fadeDistance = Math.max(0.001, radius - innerRadius)

  for (const point of points) {
    const minX = Math.max(0, Math.floor(point.x - radius))
    const maxX = Math.min(width - 1, Math.ceil(point.x + radius))
    const minY = Math.max(0, Math.floor(point.y - radius))
    const maxY = Math.min(height - 1, Math.ceil(point.y + radius))

    for (let y = minY; y <= maxY; y += 1) {
      for (let x = minX; x <= maxX; x += 1) {
        const distance = Math.hypot(x + 0.5 - point.x, y + 0.5 - point.y)
        if (distance > radius) continue
        const nextCoverage =
          hardness >= 100 || distance <= innerRadius
            ? 1
            : Math.max(0, Math.min(1, (radius - distance) / fadeDistance))
        const index = y * width + x
        if (nextCoverage > (coverage[index] ?? 0)) coverage[index] = nextCoverage
      }
    }
  }

  return coverage
}

function parseBrushColor(value = IMAGE_EDITOR_RASTER_DEFAULT_COLOR) {
  const normalized = normalizeImageEditorRasterColor(value).slice(1)
  return {
    r: Number.parseInt(normalized.slice(0, 2), 16),
    g: Number.parseInt(normalized.slice(2, 4), 16),
    b: Number.parseInt(normalized.slice(4, 6), 16),
  }
}

function clampNumber(value: number, min: number, max: number, fallback: number) {
  const next = Number(value)
  const resolved = Number.isFinite(next) ? next : fallback
  return Math.max(min, Math.min(max, Math.round(resolved)))
}
