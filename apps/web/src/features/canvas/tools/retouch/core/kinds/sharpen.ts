import { clampWithFallback } from '../../../../../../core/utils/number'
import type { RetouchSharpenParams } from '../model.ts'
import type { ImageRetouchNumberRange } from '../numberRange.ts'

/**
 * Sharpen kind side (second-wave KindTrait cohesion).
 *
 * NormalizedSharpenEffect used to live in processing/definitions.ts next to
 * the registry. It now lives here with its ranges / normalize / isNeutral /
 * guard / clone so each kind owns its Raw <-> Normalized contract; the
 * registry in definitions.ts only assembles the per-kind traits.
 */

export type NormalizedSharpenEffect = {
  amount: number
  radius: number
  threshold: number
  protectColor: boolean
}

export const RETOUCH_SHARPEN_DEFAULT_PARAMS: RetouchSharpenParams = {
  amount: 0,
  radius: 1,
  threshold: 4,
  protectColor: true,
}

export const RETOUCH_SHARPEN_RANGES: Record<
  Exclude<keyof RetouchSharpenParams, 'protectColor'>,
  ImageRetouchNumberRange
> = {
  amount: {
    labelKey: 'canvas.retouch.params.amount',
    min: 0,
    max: 200,
    defaultValue: 0,
  },
  radius: {
    labelKey: 'canvas.retouch.params.radius',
    min: 0.3,
    max: 5,
    defaultValue: 1,
  },
  threshold: {
    labelKey: 'canvas.retouch.params.threshold',
    min: 0,
    max: 50,
    defaultValue: 4,
  },
}

export function normalizeSharpenEffect(params: RetouchSharpenParams): NormalizedSharpenEffect {
  return {
    amount: clampToRange(params.amount, RETOUCH_SHARPEN_RANGES.amount),
    radius: clampToRange(params.radius, RETOUCH_SHARPEN_RANGES.radius),
    threshold: clampToRange(params.threshold, RETOUCH_SHARPEN_RANGES.threshold),
    protectColor: params.protectColor !== false,
  }
}

export function isNeutralSharpenEffect(params: NormalizedSharpenEffect): boolean {
  return params.amount === 0
}

export function isSharpenParams(value: unknown): value is RetouchSharpenParams {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return false
  const record = value as Record<string, unknown>
  // `amount` is exclusive to sharpen (threshold params carry `threshold` but
  // never `amount`), so the guard is order-independent against threshold.
  return (
    'amount' in record && !('hue' in record) && !('channels' in record) && !('shadows' in record)
  )
}

export function cloneSharpenParams(params: RetouchSharpenParams): RetouchSharpenParams {
  // Sanitizing clone: clamp numerics and default protectColor, matching the
  // previous layerStack clone path (normalizeSharpenEffect).
  return normalizeSharpenEffect(params)
}

export function cloneSharpenDefaults(): RetouchSharpenParams {
  return cloneSharpenParams(RETOUCH_SHARPEN_DEFAULT_PARAMS)
}

function clampToRange(value: number, range: ImageRetouchNumberRange) {
  return clampWithFallback(value, range.min, range.max, range.defaultValue)
}
