import { i18n } from '../../../../shared/i18n'
import type {
  ImageRetouchKind,
  RetouchEffectLayer,
  RetouchCurvesParams,
  RetouchHslParams,
  RetouchLevelsParams,
  RetouchSharpenParams,
  RetouchThresholdParams,
} from '../types'
import { normalizeCurvesParams, type NormalizedCurvesEffect } from './curves'
import { normalizeLevelsEffect, type NormalizedLevelsEffect } from './levels'
import { normalizeThresholdEffect, type NormalizedThresholdEffect } from './threshold'

export const CHANNEL_MIN = 0
export const CHANNEL_MAX = 255
export const PERCENT_SCALE = 100

export type ImageRetouchNumberRange = {
  label: string
  min: number
  max: number
  defaultValue: number
}

export type NormalizedHslEffect = {
  hueShift: number
  saturationScale: number
  lightnessOffset: number
}

export type NormalizedSharpenEffect = {
  amount: number
  radius: number
  threshold: number
  protectColor: boolean
}

export type NormalizedImageRetouch =
  | { kind: 'hsl'; params: NormalizedHslEffect }
  | { kind: 'threshold'; params: NormalizedThresholdEffect }
  | { kind: 'curves'; params: NormalizedCurvesEffect }
  | { kind: 'levels'; params: NormalizedLevelsEffect }
  | { kind: 'sharpen'; params: NormalizedSharpenEffect }

type ImageRetouchDefaultsByKind = {
  hsl: RetouchHslParams
  threshold: RetouchThresholdParams
  curves: RetouchCurvesParams
  levels: RetouchLevelsParams
  sharpen: RetouchSharpenParams
}

type ImageRetouchRangeByKind = {
  hsl: Record<keyof RetouchHslParams, ImageRetouchNumberRange>
  threshold: Record<'threshold', ImageRetouchNumberRange>
  curves: Record<string, never>
  levels: Record<keyof RetouchLevelsParams, ImageRetouchNumberRange>
  sharpen: Record<Exclude<keyof RetouchSharpenParams, 'protectColor'>, ImageRetouchNumberRange>
}

type ImageRetouchDefinitionByKind = {
  hsl: {
    defaults: ImageRetouchDefaultsByKind['hsl']
    ranges: ImageRetouchRangeByKind['hsl']
    normalize: (params: RetouchHslParams) => NormalizedHslEffect
    isNeutral: (params: NormalizedHslEffect) => boolean
  }
  threshold: {
    defaults: ImageRetouchDefaultsByKind['threshold']
    ranges: ImageRetouchRangeByKind['threshold']
    normalize: (params: RetouchThresholdParams) => NormalizedThresholdEffect
    isNeutral: (params: NormalizedThresholdEffect) => boolean
  }
  curves: {
    defaults: ImageRetouchDefaultsByKind['curves']
    ranges: ImageRetouchRangeByKind['curves']
    normalize: (params: RetouchCurvesParams) => NormalizedCurvesEffect
    isNeutral: (params: NormalizedCurvesEffect) => boolean
  }
  levels: {
    defaults: ImageRetouchDefaultsByKind['levels']
    ranges: ImageRetouchRangeByKind['levels']
    normalize: (params: RetouchLevelsParams) => NormalizedLevelsEffect
    isNeutral: (params: NormalizedLevelsEffect) => boolean
  }
  sharpen: {
    defaults: ImageRetouchDefaultsByKind['sharpen']
    ranges: ImageRetouchRangeByKind['sharpen']
    normalize: (params: RetouchSharpenParams) => NormalizedSharpenEffect
    isNeutral: (params: NormalizedSharpenEffect) => boolean
  }
}

const HSL_RANGES = {
  hue: { label: i18n.global.t('canvas.retouch.params.hue'), min: -180, max: 180, defaultValue: 0 },
  saturation: {
    label: i18n.global.t('canvas.retouch.params.saturation'),
    min: -100,
    max: 100,
    defaultValue: 0,
  },
  lightness: {
    label: i18n.global.t('canvas.retouch.params.lightness'),
    min: -100,
    max: 100,
    defaultValue: 0,
  },
} satisfies ImageRetouchRangeByKind['hsl']

const THRESHOLD_RANGES = {
  threshold: {
    label: i18n.global.t('canvas.retouch.params.threshold'),
    min: CHANNEL_MIN,
    max: CHANNEL_MAX,
    defaultValue: 128,
  },
} satisfies ImageRetouchRangeByKind['threshold']

const LEVELS_RANGES = {
  shadows: {
    label: i18n.global.t('canvas.retouch.params.shadows'),
    min: CHANNEL_MIN,
    max: CHANNEL_MAX,
    defaultValue: 0,
  },
  midtones: {
    label: i18n.global.t('canvas.retouch.params.midtones'),
    min: CHANNEL_MIN,
    max: CHANNEL_MAX,
    defaultValue: 128,
  },
  highlights: {
    label: i18n.global.t('canvas.retouch.params.highlights'),
    min: CHANNEL_MIN,
    max: CHANNEL_MAX,
    defaultValue: 255,
  },
  outputBlack: {
    label: i18n.global.t('canvas.retouch.params.outputBlack'),
    min: CHANNEL_MIN,
    max: CHANNEL_MAX,
    defaultValue: 0,
  },
  outputWhite: {
    label: i18n.global.t('canvas.retouch.params.outputWhite'),
    min: CHANNEL_MIN,
    max: CHANNEL_MAX,
    defaultValue: 255,
  },
} satisfies ImageRetouchRangeByKind['levels']

const SHARPEN_RANGES = {
  amount: {
    label: i18n.global.t('canvas.retouch.params.amount'),
    min: 0,
    max: 200,
    defaultValue: 0,
  },
  radius: {
    label: i18n.global.t('canvas.retouch.params.radius'),
    min: 0.3,
    max: 5,
    defaultValue: 1,
  },
  threshold: {
    label: i18n.global.t('canvas.retouch.params.threshold'),
    min: 0,
    max: 50,
    defaultValue: 4,
  },
} satisfies ImageRetouchRangeByKind['sharpen']

export const IMAGE_RETOUCH_DEFINITIONS: ImageRetouchDefinitionByKind = {
  hsl: {
    defaults: { hue: 0, saturation: 0, lightness: 0 },
    ranges: HSL_RANGES,
    normalize: normalizeHslEffect,
    isNeutral: isNeutralHslRetouch,
  },
  threshold: {
    defaults: { threshold: 128, invert: false },
    ranges: THRESHOLD_RANGES,
    normalize: normalizeThresholdEffect,
    isNeutral: isNeutralThresholdRetouch,
  },
  curves: {
    defaults: { channels: {} },
    ranges: {},
    normalize: normalizeCurvesParams,
    isNeutral: isNeutralCurvesRetouch,
  },
  levels: {
    defaults: { shadows: 0, midtones: 128, highlights: 255, outputBlack: 0, outputWhite: 255 },
    ranges: LEVELS_RANGES,
    normalize: normalizeLevelsEffect,
    isNeutral: isNeutralLevelsRetouch,
  },
  sharpen: {
    defaults: { amount: 0, radius: 1, threshold: 4, protectColor: true },
    ranges: SHARPEN_RANGES,
    normalize: normalizeSharpenEffect,
    isNeutral: isNeutralSharpenRetouch,
  },
}

export function normalizeImageEffectLayers(
  layers: readonly RetouchEffectLayer[],
): NormalizedImageRetouch[] {
  return layers.map((layer) => normalizeImageEffectLayer(layer))
}

export function normalizeImageEffectLayer(layer: RetouchEffectLayer): NormalizedImageRetouch {
  switch (layer.kind) {
    case 'hsl':
      return { kind: 'hsl', params: IMAGE_RETOUCH_DEFINITIONS.hsl.normalize(layer.params) }
    case 'threshold':
      return {
        kind: 'threshold',
        params: IMAGE_RETOUCH_DEFINITIONS.threshold.normalize(layer.params),
      }
    case 'curves':
      return {
        kind: 'curves',
        params: IMAGE_RETOUCH_DEFINITIONS.curves.normalize(layer.params),
      }
    case 'levels':
      return {
        kind: 'levels',
        params: IMAGE_RETOUCH_DEFINITIONS.levels.normalize(layer.params),
      }
    case 'sharpen':
      return {
        kind: 'sharpen',
        params: IMAGE_RETOUCH_DEFINITIONS.sharpen.normalize(layer.params),
      }
  }
}

export function isNeutralImageRetouch(retouch: NormalizedImageRetouch) {
  switch (retouch.kind) {
    case 'hsl':
      return IMAGE_RETOUCH_DEFINITIONS.hsl.isNeutral(retouch.params)
    case 'threshold':
      return IMAGE_RETOUCH_DEFINITIONS.threshold.isNeutral(retouch.params)
    case 'curves':
      return IMAGE_RETOUCH_DEFINITIONS.curves.isNeutral(retouch.params)
    case 'levels':
      return IMAGE_RETOUCH_DEFINITIONS.levels.isNeutral(retouch.params)
    case 'sharpen':
      return IMAGE_RETOUCH_DEFINITIONS.sharpen.isNeutral(retouch.params)
  }
}

function normalizeHslEffect(params: RetouchHslParams): NormalizedHslEffect {
  const ranges = IMAGE_RETOUCH_DEFINITIONS.hsl.ranges
  return {
    hueShift: clampToRange(params.hue, ranges.hue) / 360,
    saturationScale: Math.max(0, 1 + clampToRange(params.saturation, ranges.saturation) / 100),
    lightnessOffset: clampToRange(params.lightness, ranges.lightness) / 100,
  }
}

function normalizeSharpenEffect(params: RetouchSharpenParams): NormalizedSharpenEffect {
  const ranges = IMAGE_RETOUCH_DEFINITIONS.sharpen.ranges
  return {
    amount: clampToRange(params.amount, ranges.amount),
    radius: clampToRange(params.radius, ranges.radius),
    threshold: clampToRange(params.threshold, ranges.threshold),
    protectColor: params.protectColor !== false,
  }
}

function isNeutralHslRetouch(params: NormalizedHslEffect) {
  return params.hueShift === 0 && params.saturationScale === 1 && params.lightnessOffset === 0
}

function isNeutralThresholdRetouch(_params: NormalizedThresholdEffect) {
  return false
}

function isNeutralCurvesRetouch(params: NormalizedCurvesEffect) {
  return Object.keys(params.channels).length === 0
}

function isNeutralLevelsRetouch(params: NormalizedLevelsEffect) {
  return (
    params.blackPoint === CHANNEL_MIN &&
    params.whitePoint === CHANNEL_MAX &&
    params.gamma === 1 &&
    params.outputBlack === CHANNEL_MIN &&
    params.outputWhite === CHANNEL_MAX
  )
}

function isNeutralSharpenRetouch(params: NormalizedSharpenEffect) {
  return params.amount === 0
}

function clampToRange(value: number, range: ImageRetouchNumberRange) {
  if (!Number.isFinite(value)) return range.defaultValue
  return Math.min(range.max, Math.max(range.min, value))
}

export function getImageRetouchDefaultParams<TKind extends ImageRetouchKind>(
  kind: TKind,
): ImageRetouchDefaultsByKind[TKind] {
  switch (kind) {
    case 'hsl':
      return {
        ...IMAGE_RETOUCH_DEFINITIONS.hsl.defaults,
      } as ImageRetouchDefaultsByKind[TKind]
    case 'threshold':
      return {
        ...IMAGE_RETOUCH_DEFINITIONS.threshold.defaults,
      } as ImageRetouchDefaultsByKind[TKind]
    case 'curves':
      return {
        channels: {},
      } as ImageRetouchDefaultsByKind[TKind]
    case 'levels':
      return {
        ...IMAGE_RETOUCH_DEFINITIONS.levels.defaults,
      } as ImageRetouchDefaultsByKind[TKind]
    case 'sharpen':
      return {
        ...IMAGE_RETOUCH_DEFINITIONS.sharpen.defaults,
      } as ImageRetouchDefaultsByKind[TKind]
    default:
      return assertNever(kind)
  }
}

function assertNever(value: never): never {
  throw new Error(`Unhandled image retouch kind: ${String(value)}`)
}
