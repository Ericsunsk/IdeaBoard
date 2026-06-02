import { resolvePromptAspectRatioValue, type PromptAspectRatio } from '../config/promptAspectRatio'
import { computeFusionBlendWeights, type FusionBlendPoint } from './fusionBlend'

export type FusionOutputImageSize = {
  width: number
  height: number
}

type FusionOutputSizingInputImage = {
  ratio: number
}

type ResolveFusionOutputSizeOptions = {
  inputImages: FusionOutputSizingInputImage[]
  imageSizes: Array<FusionOutputImageSize | null>
  blendPoint: FusionBlendPoint
  stageWidth: number
  aspectRatio: PromptAspectRatio
}

export type ResolvedFusionOutputSize = {
  sourceSize: FusionOutputImageSize | null
  targetSize: FusionOutputImageSize | null
  targetRatio: number
  weights: number[]
}

const FUSION_WEIGHT_TIE_EPSILON = 0.01

function normalizeSize(
  size: FusionOutputImageSize | null | undefined,
): FusionOutputImageSize | null {
  if (!size) return null
  const width = Math.max(1, Math.round(Number(size.width)))
  const height = Math.max(1, Math.round(Number(size.height)))
  if (!Number.isFinite(width) || !Number.isFinite(height)) return null
  return { width, height }
}

function readImageRatio(
  inputImages: FusionOutputSizingInputImage[],
  imageSizes: Array<FusionOutputImageSize | null>,
  index: number,
): number {
  const size = imageSizes[index]
  if (size && size.width > 0 && size.height > 0) return size.height / size.width
  const ratio = Number(inputImages[index]?.ratio)
  return Number.isFinite(ratio) && ratio > 0 ? ratio : 1
}

function readArea(size: FusionOutputImageSize | null | undefined): number | null {
  if (!size) return null
  const area = size.width * size.height
  return Number.isFinite(area) && area > 0 ? area : null
}

function averageArea(
  indices: number[],
  imageSizes: Array<FusionOutputImageSize | null>,
  fallbackSize: FusionOutputImageSize | null,
): number | null {
  const areas = indices
    .map((index) => readArea(imageSizes[index]))
    .filter((area): area is number => area !== null)
  if (areas.length > 0) return areas.reduce((sum, area) => sum + area, 0) / areas.length
  return readArea(fallbackSize)
}

function geometricMean(values: number[]): number {
  const safeValues = values.filter((value) => Number.isFinite(value) && value > 0)
  if (safeValues.length === 0) return 1
  const logAverage = safeValues.reduce((sum, value) => sum + Math.log(value), 0) / safeValues.length
  return Math.exp(logAverage)
}

function sizeFromArea(area: number | null, ratio: number): FusionOutputImageSize | null {
  if (!area || !Number.isFinite(area) || area <= 0) return null
  const safeRatio = Number.isFinite(ratio) && ratio > 0 ? ratio : 1
  const width = Math.max(1, Math.round(Math.sqrt(area / safeRatio)))
  return {
    width,
    height: Math.max(1, Math.round(width * safeRatio)),
  }
}

export function resolveFusionOutputSize({
  inputImages,
  imageSizes,
  blendPoint,
  stageWidth,
  aspectRatio,
}: ResolveFusionOutputSizeOptions): ResolvedFusionOutputSize {
  const count = Math.max(0, Math.min(inputImages.length, 4))
  const normalizedSizes = inputImages
    .slice(0, count)
    .map((_, index) => normalizeSize(imageSizes[index]))
  const sourceSize = normalizedSizes[0] ?? normalizedSizes.find(Boolean) ?? null
  if (count === 0) {
    return {
      sourceSize,
      targetSize: sourceSize,
      targetRatio: sourceSize ? sourceSize.height / sourceSize.width : 1,
      weights: [],
    }
  }

  const weights = computeFusionBlendWeights(count, blendPoint, stageWidth).slice(0, count)
  const maxWeight = Math.max(...weights)
  const dominantIndices = weights
    .map((weight, index) => ({ weight, index }))
    .filter((item) => maxWeight - item.weight <= FUSION_WEIGHT_TIE_EPSILON)
    .map((item) => item.index)
  const fallbackIndex = dominantIndices[0] ?? 0
  const manualRatio = resolvePromptAspectRatioValue(aspectRatio)

  if (manualRatio === null && dominantIndices.length === 1) {
    const dominantSize = normalizedSizes[fallbackIndex]
    const targetRatio = readImageRatio(inputImages, normalizedSizes, fallbackIndex)
    return {
      sourceSize,
      targetSize: dominantSize ?? sizeFromArea(readArea(sourceSize), targetRatio),
      targetRatio,
      weights,
    }
  }

  const targetRatio =
    manualRatio ??
    geometricMean(
      dominantIndices.map((index) => readImageRatio(inputImages, normalizedSizes, index)),
    )
  const area = averageArea(dominantIndices, normalizedSizes, sourceSize)
  return {
    sourceSize,
    targetSize: sizeFromArea(area, targetRatio),
    targetRatio,
    weights,
  }
}
