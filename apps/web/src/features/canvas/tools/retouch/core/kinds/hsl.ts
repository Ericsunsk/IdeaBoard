import { clampWithFallback } from '../../../../../../core/utils/number'
import type { RetouchHslParams } from '../model.ts'
import type { ImageRetouchNumberRange } from '../numberRange.ts'

/**
 * HSL kind side (second-wave KindTrait cohesion).
 *
 * NormalizedHslEffect used to live in processing/definitions.ts next to the
 * registry. It now lives here with its ranges / normalize / isNeutral /
 * guard / clone so each kind owns its Raw <-> Normalized contract; the
 * registry in definitions.ts only assembles the per-kind traits.
 */

export type NormalizedHslEffect = {
  hueShift: number
  saturationScale: number
  lightnessOffset: number
}

export const RETOUCH_HSL_DEFAULT_PARAMS: RetouchHslParams = {
  hue: 0,
  saturation: 0,
  lightness: 0,
}

export const RETOUCH_HSL_RANGES: Record<keyof RetouchHslParams, ImageRetouchNumberRange> = {
  hue: { labelKey: 'canvas.retouch.params.hue', min: -180, max: 180, defaultValue: 0 },
  saturation: {
    labelKey: 'canvas.retouch.params.saturation',
    min: -100,
    max: 100,
    defaultValue: 0,
  },
  lightness: {
    labelKey: 'canvas.retouch.params.lightness',
    min: -100,
    max: 100,
    defaultValue: 0,
  },
}

export function normalizeHslEffect(params: RetouchHslParams): NormalizedHslEffect {
  // NOTE: ranges are read from this module's own RETOUCH_HSL_RANGES, not via
  // IMAGE_RETOUCH_DEFINITIONS.hsl.ranges. The previous self-reference through
  // the registry table was fragile at module-init time (function hoisting
  // masked it because normalization only runs post-init); the kind-local
  // table makes the dependency one-directional: definitions -> hsl.
  return {
    hueShift: clampToRange(params.hue, RETOUCH_HSL_RANGES.hue) / 360,
    saturationScale: Math.max(
      0,
      1 + clampToRange(params.saturation, RETOUCH_HSL_RANGES.saturation) / 100,
    ),
    lightnessOffset: clampToRange(params.lightness, RETOUCH_HSL_RANGES.lightness) / 100,
  }
}

export function isNeutralHslEffect(params: NormalizedHslEffect): boolean {
  return params.hueShift === 0 && params.saturationScale === 1 && params.lightnessOffset === 0
}

export function isHslParams(value: unknown): value is RetouchHslParams {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return false
  const record = value as Record<string, unknown>
  // `hue` is exclusive to hsl: no other kind carries it, so the guard is
  // order-independent against threshold/sharpen/curves/levels.
  return 'hue' in record && 'saturation' in record && 'lightness' in record
}

export function cloneHslParams(params: RetouchHslParams): RetouchHslParams {
  return { ...params }
}

export function cloneHslDefaults(): RetouchHslParams {
  return cloneHslParams(RETOUCH_HSL_DEFAULT_PARAMS)
}

function clampToRange(value: number, range: ImageRetouchNumberRange) {
  return clampWithFallback(value, range.min, range.max, range.defaultValue)
}
