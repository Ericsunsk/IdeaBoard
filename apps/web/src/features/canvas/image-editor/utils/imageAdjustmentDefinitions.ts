import { i18n } from '../../../../shared/i18n'
import type {
  ImageAdjustmentKind,
  ImageEditorAdjustmentLayer,
  ImageEditorCurvesParams,
  ImageEditorHslParams,
  ImageEditorLevelsParams,
  ImageEditorSharpenParams,
  ImageEditorThresholdParams,
} from '../types'
import { normalizeCurvesParams, type NormalizedCurvesAdjustment } from './imageEditorCurves'
import { normalizeLevelsAdjustment, type NormalizedLevelsAdjustment } from './imageEditorLevels'
import {
  normalizeThresholdAdjustment,
  type NormalizedThresholdAdjustment,
} from './imageEditorThreshold'

export const CHANNEL_MIN = 0
export const CHANNEL_MAX = 255
export const PERCENT_SCALE = 100

export type ImageAdjustmentNumberRange = {
  label: string
  min: number
  max: number
  defaultValue: number
}

export type NormalizedHslAdjustment = {
  hueShift: number
  saturationScale: number
  lightnessOffset: number
}

export type NormalizedSharpenAdjustment = {
  amount: number
  radius: number
  threshold: number
  protectColor: boolean
}

export type NormalizedImageAdjustment =
  | { kind: 'hsl'; params: NormalizedHslAdjustment }
  | { kind: 'threshold'; params: NormalizedThresholdAdjustment }
  | { kind: 'curves'; params: NormalizedCurvesAdjustment }
  | { kind: 'levels'; params: NormalizedLevelsAdjustment }
  | { kind: 'sharpen'; params: NormalizedSharpenAdjustment }

type ImageAdjustmentDefaultsByKind = {
  hsl: ImageEditorHslParams
  threshold: ImageEditorThresholdParams
  curves: ImageEditorCurvesParams
  levels: ImageEditorLevelsParams
  sharpen: ImageEditorSharpenParams
}

type ImageAdjustmentRangeByKind = {
  hsl: Record<keyof ImageEditorHslParams, ImageAdjustmentNumberRange>
  threshold: Record<'threshold', ImageAdjustmentNumberRange>
  curves: Record<string, never>
  levels: Record<keyof ImageEditorLevelsParams, ImageAdjustmentNumberRange>
  sharpen: Record<
    Exclude<keyof ImageEditorSharpenParams, 'protectColor'>,
    ImageAdjustmentNumberRange
  >
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
  sharpen: {
    defaults: ImageAdjustmentDefaultsByKind['sharpen']
    ranges: ImageAdjustmentRangeByKind['sharpen']
    normalize: (params: ImageEditorSharpenParams) => NormalizedSharpenAdjustment
    isNeutral: (params: NormalizedSharpenAdjustment) => boolean
  }
}

const HSL_RANGES = {
  hue: { label: i18n.global.t('canvas.imageEditor.params.hue'), min: -180, max: 180, defaultValue: 0 },
  saturation: {
    label: i18n.global.t('canvas.imageEditor.params.saturation'),
    min: -100,
    max: 100,
    defaultValue: 0,
  },
  lightness: {
    label: i18n.global.t('canvas.imageEditor.params.lightness'),
    min: -100,
    max: 100,
    defaultValue: 0,
  },
} satisfies ImageAdjustmentRangeByKind['hsl']

const THRESHOLD_RANGES = {
  threshold: {
    label: i18n.global.t('canvas.imageEditor.params.threshold'),
    min: CHANNEL_MIN,
    max: CHANNEL_MAX,
    defaultValue: 128,
  },
} satisfies ImageAdjustmentRangeByKind['threshold']

const LEVELS_RANGES = {
  shadows: { label: i18n.global.t('canvas.imageEditor.params.shadows'), min: CHANNEL_MIN, max: CHANNEL_MAX, defaultValue: 0 },
  midtones: {
    label: i18n.global.t('canvas.imageEditor.params.midtones'),
    min: CHANNEL_MIN,
    max: CHANNEL_MAX,
    defaultValue: 128,
  },
  highlights: {
    label: i18n.global.t('canvas.imageEditor.params.highlights'),
    min: CHANNEL_MIN,
    max: CHANNEL_MAX,
    defaultValue: 255,
  },
  outputBlack: {
    label: i18n.global.t('canvas.imageEditor.params.outputBlack'),
    min: CHANNEL_MIN,
    max: CHANNEL_MAX,
    defaultValue: 0,
  },
  outputWhite: {
    label: i18n.global.t('canvas.imageEditor.params.outputWhite'),
    min: CHANNEL_MIN,
    max: CHANNEL_MAX,
    defaultValue: 255,
  },
} satisfies ImageAdjustmentRangeByKind['levels']

const SHARPEN_RANGES = {
  amount: { label: i18n.global.t('canvas.imageEditor.params.amount'), min: 0, max: 200, defaultValue: 0 },
  radius: { label: i18n.global.t('canvas.imageEditor.params.radius'), min: 0.3, max: 5, defaultValue: 1 },
  threshold: {
    label: i18n.global.t('canvas.imageEditor.params.threshold'),
    min: 0,
    max: 50,
    defaultValue: 4,
  },
} satisfies ImageAdjustmentRangeByKind['sharpen']

export const IMAGE_ADJUSTMENT_DEFINITIONS: ImageAdjustmentDefinitionByKind = {
  hsl: {
    defaults: { hue: 0, saturation: 0, lightness: 0 },
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
    defaults: { shadows: 0, midtones: 128, highlights: 255, outputBlack: 0, outputWhite: 255 },
    ranges: LEVELS_RANGES,
    normalize: normalizeLevelsAdjustment,
    isNeutral: isNeutralLevelsAdjustment,
  },
  sharpen: {
    defaults: { amount: 0, radius: 1, threshold: 4, protectColor: true },
    ranges: SHARPEN_RANGES,
    normalize: normalizeSharpenAdjustment,
    isNeutral: isNeutralSharpenAdjustment,
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
    case 'sharpen':
      return {
        kind: 'sharpen',
        params: IMAGE_ADJUSTMENT_DEFINITIONS.sharpen.normalize(layer.params),
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
    case 'sharpen':
      return IMAGE_ADJUSTMENT_DEFINITIONS.sharpen.isNeutral(adjustment.params)
  }
}

function normalizeHslAdjustment(params: ImageEditorHslParams): NormalizedHslAdjustment {
  const ranges = IMAGE_ADJUSTMENT_DEFINITIONS.hsl.ranges
  return {
    hueShift: clampToRange(params.hue, ranges.hue) / 360,
    saturationScale: Math.max(0, 1 + clampToRange(params.saturation, ranges.saturation) / 100),
    lightnessOffset: clampToRange(params.lightness, ranges.lightness) / 100,
  }
}

function normalizeSharpenAdjustment(params: ImageEditorSharpenParams): NormalizedSharpenAdjustment {
  const ranges = IMAGE_ADJUSTMENT_DEFINITIONS.sharpen.ranges
  return {
    amount: clampToRange(params.amount, ranges.amount),
    radius: clampToRange(params.radius, ranges.radius),
    threshold: clampToRange(params.threshold, ranges.threshold),
    protectColor: params.protectColor !== false,
  }
}

function isNeutralHslAdjustment(params: NormalizedHslAdjustment) {
  return params.hueShift === 0 && params.saturationScale === 1 && params.lightnessOffset === 0
}

function isNeutralThresholdAdjustment(_params: NormalizedThresholdAdjustment) {
  return false
}

function isNeutralCurvesAdjustment(params: NormalizedCurvesAdjustment) {
  return Object.keys(params.channels).length === 0
}

function isNeutralLevelsAdjustment(params: NormalizedLevelsAdjustment) {
  return (
    params.blackPoint === CHANNEL_MIN &&
    params.whitePoint === CHANNEL_MAX &&
    params.gamma === 1 &&
    params.outputBlack === CHANNEL_MIN &&
    params.outputWhite === CHANNEL_MAX
  )
}

function isNeutralSharpenAdjustment(params: NormalizedSharpenAdjustment) {
  return params.amount === 0
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
      return {
        ...IMAGE_ADJUSTMENT_DEFINITIONS.hsl.defaults,
      } as ImageAdjustmentDefaultsByKind[TKind]
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
    case 'sharpen':
      return {
        ...IMAGE_ADJUSTMENT_DEFINITIONS.sharpen.defaults,
      } as ImageAdjustmentDefaultsByKind[TKind]
    default:
      return assertNever(kind)
  }
}

function assertNever(value: never): never {
  throw new Error(`Unhandled image adjustment kind: ${String(value)}`)
}
