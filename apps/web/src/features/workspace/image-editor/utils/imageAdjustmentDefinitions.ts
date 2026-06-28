import type {
  ImageAdjustmentKind,
  ImageEditorAdjustmentLayer,
  ImageEditorCurvesParams,
  ImageEditorHslParams,
  ImageEditorInvertParams,
  ImageEditorLevelsParams,
  ImageEditorThresholdParams,
} from '../types'
import {
  normalizeCurvesParams,
  type NormalizedCurvesAdjustment,
} from './imageEditorCurves'

export const CHANNEL_MIN = 0
export const CHANNEL_MAX = 255
export const PERCENT_SCALE = 100

export type ImageAdjustmentNumberRange = {
  label: string
  min: number
  max: number
  defaultValue: number
  unit?: string
}

export type NormalizedHslAdjustment = {
  hueShift: number
  saturationScale: number
  lightnessOffset: number
  contrastFactor: number
}

export type NormalizedThresholdAdjustment = {
  threshold: number
  invert: boolean
}

export type NormalizedLevelsAdjustment = {
  blackPoint: number
  whitePoint: number
  gamma: number
}

export type NormalizedInvertAdjustment = {
  enabled: boolean
}

export type NormalizedImageAdjustment =
  | { kind: 'hsl'; params: NormalizedHslAdjustment }
  | { kind: 'threshold'; params: NormalizedThresholdAdjustment }
  | { kind: 'curves'; params: NormalizedCurvesAdjustment }
  | { kind: 'levels'; params: NormalizedLevelsAdjustment }
  | { kind: 'invert'; params: NormalizedInvertAdjustment }

type ImageAdjustmentDefaultsByKind = {
  hsl: ImageEditorHslParams
  threshold: ImageEditorThresholdParams
  curves: ImageEditorCurvesParams
  levels: ImageEditorLevelsParams
  invert: ImageEditorInvertParams
}

type ImageAdjustmentRangeByKind = {
  hsl: Record<keyof ImageEditorHslParams, ImageAdjustmentNumberRange>
  threshold: Record<'threshold', ImageAdjustmentNumberRange>
  curves: Record<string, never>
  levels: Record<keyof ImageEditorLevelsParams, ImageAdjustmentNumberRange>
  invert: Record<string, never>
}

type ImageAdjustmentDefinitionByKind = {
  hsl: {
    defaults: ImageAdjustmentDefaultsByKind['hsl']
    ranges: ImageAdjustmentRangeByKind['hsl']
    normalize: (params: ImageEditorHslParams) => NormalizedHslAdjustment
    isNeutral: (params: NormalizedHslAdjustment) => boolean
  }
  threshold: {
    defaults: ImageAdjustmentDefaultsByKind['threshold']
    ranges: ImageAdjustmentRangeByKind['threshold']
    normalize: (params: ImageEditorThresholdParams) => NormalizedThresholdAdjustment
    isNeutral: (params: NormalizedThresholdAdjustment) => boolean
  }
  curves: {
    defaults: ImageAdjustmentDefaultsByKind['curves']
    ranges: ImageAdjustmentRangeByKind['curves']
    normalize: (params: ImageEditorCurvesParams) => NormalizedCurvesAdjustment
    isNeutral: (params: NormalizedCurvesAdjustment) => boolean
  }
  levels: {
    defaults: ImageAdjustmentDefaultsByKind['levels']
    ranges: ImageAdjustmentRangeByKind['levels']
    normalize: (params: ImageEditorLevelsParams) => NormalizedLevelsAdjustment
    isNeutral: (params: NormalizedLevelsAdjustment) => boolean
  }
  invert: {
    defaults: ImageAdjustmentDefaultsByKind['invert']
    ranges: ImageAdjustmentRangeByKind['invert']
    normalize: (params: ImageEditorInvertParams) => NormalizedInvertAdjustment
    isNeutral: (params: NormalizedInvertAdjustment) => boolean
  }
}

const HSL_RANGES = {
  hue: { label: '色相', min: -180, max: 180, defaultValue: 0, unit: 'deg' },
  saturation: { label: '饱和度', min: -100, max: 100, defaultValue: 0, unit: '%' },
  brightness: { label: '亮度', min: -100, max: 100, defaultValue: 0, unit: '%' },
  contrast: { label: '对比度', min: -100, max: 100, defaultValue: 0, unit: '%' },
} satisfies ImageAdjustmentRangeByKind['hsl']

const THRESHOLD_RANGES = {
  threshold: { label: '阈值', min: CHANNEL_MIN, max: CHANNEL_MAX, defaultValue: 128 },
} satisfies ImageAdjustmentRangeByKind['threshold']

const LEVELS_RANGES = {
  shadows: { label: '黑场', min: 0, max: PERCENT_SCALE, defaultValue: 0, unit: '%' },
  midtones: { label: '中间', min: 1, max: 99, defaultValue: 50, unit: '%' },
  highlights: { label: '白场', min: 0, max: PERCENT_SCALE, defaultValue: 100, unit: '%' },
} satisfies ImageAdjustmentRangeByKind['levels']

export const IMAGE_ADJUSTMENT_DEFINITIONS: ImageAdjustmentDefinitionByKind = {
  hsl: {
    defaults: { hue: 0, saturation: 0, brightness: 0, contrast: 0 },
    ranges: HSL_RANGES,
    normalize: normalizeHslAdjustment,
    isNeutral: isNeutralHslAdjustment,
  },
  threshold: {
    defaults: { threshold: 128, invert: false },
    ranges: THRESHOLD_RANGES,
    normalize: normalizeThresholdAdjustment,
    isNeutral: isNeutralThresholdAdjustment,
  },
  curves: {
    defaults: { channels: {} },
    ranges: {},
    normalize: normalizeCurvesParams,
    isNeutral: isNeutralCurvesAdjustment,
  },
  levels: {
    defaults: { shadows: 0, midtones: 50, highlights: 100 },
    ranges: LEVELS_RANGES,
    normalize: normalizeLevelsAdjustment,
    isNeutral: isNeutralLevelsAdjustment,
  },
  invert: {
    defaults: { enabled: true },
    ranges: {},
    normalize: normalizeInvertAdjustment,
    isNeutral: isNeutralInvertAdjustment,
  },
}

export function normalizeImageAdjustmentLayers(
  layers: readonly ImageEditorAdjustmentLayer[],
): NormalizedImageAdjustment[] {
  return layers.map((layer) => normalizeImageAdjustmentLayer(layer))
}

export function normalizeImageAdjustmentLayer(
  layer: ImageEditorAdjustmentLayer,
): NormalizedImageAdjustment {
  switch (layer.kind) {
    case 'hsl':
      return { kind: 'hsl', params: IMAGE_ADJUSTMENT_DEFINITIONS.hsl.normalize(layer.params) }
    case 'threshold':
      return {
        kind: 'threshold',
        params: IMAGE_ADJUSTMENT_DEFINITIONS.threshold.normalize(layer.params),
      }
    case 'curves':
      return {
        kind: 'curves',
        params: IMAGE_ADJUSTMENT_DEFINITIONS.curves.normalize(layer.params),
      }
    case 'levels':
      return {
        kind: 'levels',
        params: IMAGE_ADJUSTMENT_DEFINITIONS.levels.normalize(layer.params),
      }
    case 'invert':
      return {
        kind: 'invert',
        params: IMAGE_ADJUSTMENT_DEFINITIONS.invert.normalize(layer.params),
      }
  }
}

export function isNeutralImageAdjustment(adjustment: NormalizedImageAdjustment) {
  switch (adjustment.kind) {
    case 'hsl':
      return IMAGE_ADJUSTMENT_DEFINITIONS.hsl.isNeutral(adjustment.params)
    case 'threshold':
      return IMAGE_ADJUSTMENT_DEFINITIONS.threshold.isNeutral(adjustment.params)
    case 'curves':
      return IMAGE_ADJUSTMENT_DEFINITIONS.curves.isNeutral(adjustment.params)
    case 'levels':
      return IMAGE_ADJUSTMENT_DEFINITIONS.levels.isNeutral(adjustment.params)
    case 'invert':
      return IMAGE_ADJUSTMENT_DEFINITIONS.invert.isNeutral(adjustment.params)
  }
}

function normalizeHslAdjustment(params: ImageEditorHslParams): NormalizedHslAdjustment {
  const ranges = IMAGE_ADJUSTMENT_DEFINITIONS.hsl.ranges
  return {
    hueShift: clampToRange(params.hue, ranges.hue) / 360,
    saturationScale: Math.max(0, 1 + clampToRange(params.saturation, ranges.saturation) / 100),
    lightnessOffset: clampToRange(params.brightness, ranges.brightness) / 100,
    contrastFactor: createContrastFactor(clampToRange(params.contrast, ranges.contrast)),
  }
}

function normalizeThresholdAdjustment(
  params: ImageEditorThresholdParams,
): NormalizedThresholdAdjustment {
  return {
    threshold: Math.round(
      clampToRange(params.threshold, IMAGE_ADJUSTMENT_DEFINITIONS.threshold.ranges.threshold),
    ),
    invert: params.invert === true,
  }
}

function normalizeLevelsAdjustment(params: ImageEditorLevelsParams): NormalizedLevelsAdjustment {
  const ranges = IMAGE_ADJUSTMENT_DEFINITIONS.levels.ranges
  const blackPoint = percentToChannel(params.shadows, ranges.shadows)
  const whitePoint = percentToChannel(params.highlights, ranges.highlights)
  const midtones = clampToRange(params.midtones, ranges.midtones) / PERCENT_SCALE
  return {
    blackPoint,
    whitePoint,
    gamma: createLevelsGamma(midtones),
  }
}

function normalizeInvertAdjustment(params: ImageEditorInvertParams): NormalizedInvertAdjustment {
  return {
    enabled: params.enabled === true,
  }
}

function isNeutralHslAdjustment(params: NormalizedHslAdjustment) {
  return (
    params.hueShift === 0 &&
    params.saturationScale === 1 &&
    params.lightnessOffset === 0 &&
    params.contrastFactor === 1
  )
}

function isNeutralThresholdAdjustment(_params: NormalizedThresholdAdjustment) {
  return false
}

function isNeutralCurvesAdjustment(params: NormalizedCurvesAdjustment) {
  return Object.keys(params.channels).length === 0
}

function isNeutralLevelsAdjustment(params: NormalizedLevelsAdjustment) {
  return params.blackPoint === CHANNEL_MIN && params.whitePoint === CHANNEL_MAX && params.gamma === 1
}

function isNeutralInvertAdjustment(params: NormalizedInvertAdjustment) {
  return !params.enabled
}

function createContrastFactor(value: number) {
  const normalized = value * 2.55
  return (259 * (normalized + 255)) / (255 * (259 - normalized))
}

function createLevelsGamma(midtones: number) {
  if (!Number.isFinite(midtones) || midtones <= 0 || midtones >= 1) return 1
  return Math.log(0.5) / Math.log(midtones)
}

function percentToChannel(value: number, range: ImageAdjustmentNumberRange) {
  return Math.round((clampToRange(value, range) / PERCENT_SCALE) * CHANNEL_MAX)
}

function clampToRange(value: number, range: ImageAdjustmentNumberRange) {
  if (!Number.isFinite(value)) return range.defaultValue
  return Math.min(range.max, Math.max(range.min, value))
}

export function getImageAdjustmentDefaultParams<TKind extends ImageAdjustmentKind>(
  kind: TKind,
): ImageAdjustmentDefaultsByKind[TKind] {
  switch (kind) {
    case 'hsl':
      return { ...IMAGE_ADJUSTMENT_DEFINITIONS.hsl.defaults } as ImageAdjustmentDefaultsByKind[TKind]
    case 'threshold':
      return {
        ...IMAGE_ADJUSTMENT_DEFINITIONS.threshold.defaults,
      } as ImageAdjustmentDefaultsByKind[TKind]
    case 'curves':
      return {
        channels: {},
      } as ImageAdjustmentDefaultsByKind[TKind]
    case 'levels':
      return {
        ...IMAGE_ADJUSTMENT_DEFINITIONS.levels.defaults,
      } as ImageAdjustmentDefaultsByKind[TKind]
    case 'invert':
      return {
        ...IMAGE_ADJUSTMENT_DEFINITIONS.invert.defaults,
      } as ImageAdjustmentDefaultsByKind[TKind]
    default:
      return assertNever(kind)
  }
}

function assertNever(value: never): never {
  throw new Error(`Unhandled image adjustment kind: ${String(value)}`)
}
