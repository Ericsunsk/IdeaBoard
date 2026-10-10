import { RETOUCH_CHANNEL_MAX, RETOUCH_CHANNEL_MIN } from './channelScale.ts'
import type {
  ImageRetouchKind,
  RetouchEffectLayer,
  RetouchCurvesParams,
  RetouchHslParams,
  RetouchLevelsParams,
  RetouchSharpenParams,
  RetouchThresholdParams,
} from './model.ts'
import {
  RETOUCH_HSL_DEFAULT_PARAMS,
  RETOUCH_HSL_RANGES,
  cloneHslDefaults,
  cloneHslParams,
  isHslParams,
  isNeutralHslEffect,
  normalizeHslEffect,
  type NormalizedHslEffect,
} from './kinds/hsl'
import {
  RETOUCH_CURVES_DEFAULT_PARAMS,
  cloneCurvesDefaults,
  cloneCurvesParams,
  isCurvesParams,
  isNeutralCurvesEffect,
  normalizeCurvesParams,
  type NormalizedCurvesEffect,
} from './kinds/curves'
import {
  RETOUCH_LEVELS_DEFAULT_PARAMS,
  cloneLevelsDefaults,
  cloneLevelsParams,
  isLevelsParams,
  isNeutralLevelsEffect,
  normalizeLevelsEffect,
  type NormalizedLevelsEffect,
} from './kinds/levels'
import {
  RETOUCH_SHARPEN_DEFAULT_PARAMS,
  RETOUCH_SHARPEN_RANGES,
  cloneSharpenDefaults,
  cloneSharpenParams,
  isNeutralSharpenEffect,
  isSharpenParams,
  normalizeSharpenEffect,
  type NormalizedSharpenEffect,
} from './kinds/sharpen'
import {
  RETOUCH_THRESHOLD_DEFAULT_PARAMS,
  cloneThresholdDefaults,
  cloneThresholdParams,
  isNeutralThresholdEffect,
  isThresholdParams,
  normalizeThresholdEffect,
  type NormalizedThresholdEffect,
} from './kinds/threshold'
import type { ImageRetouchNumberRange } from './numberRange.ts'

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

/**
 * Per-kind trait: the single-dispatch surface for one retouch kind.
 *
 * Each kind module (hsl/threshold/curves/levels/sharpen) owns its Raw <->
 * Normalized contract; the registry below only assembles the five traits so
 * normalize / isNeutral / defaults / guards / clone never fork into
 * per-kind `switch` or order-sensitive `in` chains at call sites.
 */
export type KindTrait<Raw, Normalized> = {
  defaults: Raw
  ranges: unknown
  normalize: (params: Raw) => Normalized
  isNeutral: (params: Normalized) => boolean
  isWholeParams: (value: unknown) => value is Raw
  clone: (params: Raw) => Raw
  cloneDefaults: () => Raw
}

type ImageRetouchTraitByKind = {
  hsl: KindTrait<RetouchHslParams, NormalizedHslEffect> & {
    ranges: ImageRetouchRangeByKind['hsl']
  }
  threshold: KindTrait<RetouchThresholdParams, NormalizedThresholdEffect> & {
    ranges: ImageRetouchRangeByKind['threshold']
  }
  curves: KindTrait<RetouchCurvesParams, NormalizedCurvesEffect> & {
    ranges: ImageRetouchRangeByKind['curves']
  }
  levels: KindTrait<RetouchLevelsParams, NormalizedLevelsEffect> & {
    ranges: ImageRetouchRangeByKind['levels']
  }
  sharpen: KindTrait<RetouchSharpenParams, NormalizedSharpenEffect> & {
    ranges: ImageRetouchRangeByKind['sharpen']
  }
}

const THRESHOLD_RANGES = {
  threshold: {
    labelKey: 'canvas.retouch.params.threshold',
    min: RETOUCH_CHANNEL_MIN,
    max: RETOUCH_CHANNEL_MAX,
    defaultValue: RETOUCH_THRESHOLD_DEFAULT_PARAMS.threshold,
  },
} satisfies ImageRetouchRangeByKind['threshold']

const LEVELS_RANGES = {
  shadows: {
    labelKey: 'canvas.retouch.params.shadows',
    min: RETOUCH_CHANNEL_MIN,
    max: RETOUCH_CHANNEL_MAX,
    defaultValue: RETOUCH_LEVELS_DEFAULT_PARAMS.shadows,
  },
  midtones: {
    labelKey: 'canvas.retouch.params.midtones',
    min: RETOUCH_CHANNEL_MIN,
    max: RETOUCH_CHANNEL_MAX,
    defaultValue: RETOUCH_LEVELS_DEFAULT_PARAMS.midtones,
  },
  highlights: {
    labelKey: 'canvas.retouch.params.highlights',
    min: RETOUCH_CHANNEL_MIN,
    max: RETOUCH_CHANNEL_MAX,
    defaultValue: RETOUCH_LEVELS_DEFAULT_PARAMS.highlights,
  },
  outputBlack: {
    labelKey: 'canvas.retouch.params.outputBlack',
    min: RETOUCH_CHANNEL_MIN,
    max: RETOUCH_CHANNEL_MAX,
    defaultValue: RETOUCH_LEVELS_DEFAULT_PARAMS.outputBlack,
  },
  outputWhite: {
    labelKey: 'canvas.retouch.params.outputWhite',
    min: RETOUCH_CHANNEL_MIN,
    max: RETOUCH_CHANNEL_MAX,
    defaultValue: RETOUCH_LEVELS_DEFAULT_PARAMS.outputWhite,
  },
} satisfies ImageRetouchRangeByKind['levels']

export const IMAGE_RETOUCH_DEFINITIONS: ImageRetouchTraitByKind = {
  hsl: {
    defaults: RETOUCH_HSL_DEFAULT_PARAMS,
    ranges: RETOUCH_HSL_RANGES,
    normalize: normalizeHslEffect,
    isNeutral: isNeutralHslEffect,
    isWholeParams: isHslParams,
    clone: cloneHslParams,
    cloneDefaults: cloneHslDefaults,
  },
  threshold: {
    defaults: RETOUCH_THRESHOLD_DEFAULT_PARAMS,
    ranges: THRESHOLD_RANGES,
    normalize: normalizeThresholdEffect,
    isNeutral: isNeutralThresholdEffect,
    isWholeParams: isThresholdParams,
    clone: cloneThresholdParams,
    cloneDefaults: cloneThresholdDefaults,
  },
  curves: {
    defaults: RETOUCH_CURVES_DEFAULT_PARAMS,
    ranges: {},
    normalize: normalizeCurvesParams,
    isNeutral: isNeutralCurvesEffect,
    isWholeParams: isCurvesParams,
    clone: cloneCurvesParams,
    cloneDefaults: cloneCurvesDefaults,
  },
  levels: {
    defaults: RETOUCH_LEVELS_DEFAULT_PARAMS,
    ranges: LEVELS_RANGES,
    normalize: normalizeLevelsEffect,
    isNeutral: isNeutralLevelsEffect,
    isWholeParams: isLevelsParams,
    clone: cloneLevelsParams,
    cloneDefaults: cloneLevelsDefaults,
  },
  sharpen: {
    defaults: RETOUCH_SHARPEN_DEFAULT_PARAMS,
    ranges: RETOUCH_SHARPEN_RANGES,
    normalize: normalizeSharpenEffect,
    isNeutral: isNeutralSharpenEffect,
    isWholeParams: isSharpenParams,
    clone: cloneSharpenParams,
    cloneDefaults: cloneSharpenDefaults,
  },
}

export function getImageRetouchTrait<TKind extends ImageRetouchKind>(
  kind: TKind,
): ImageRetouchTraitByKind[TKind] {
  const trait = (
    IMAGE_RETOUCH_DEFINITIONS as Record<
      ImageRetouchKind,
      ImageRetouchTraitByKind[TKind] | undefined
    >
  )[kind]
  if (!trait) return assertNever(kind as never)
  return trait
}

/**
 * Single-dispatch whole-params guard for layerStack/commands owners.
 *
 * Replaces order-sensitive `in` chains: each trait's isWholeParams is
 * mutually exclusive by construction (distinctive keys + cross-kind
 * exclusions), so dispatch order no longer matters.
 */
export function isRetouchParamsForKind<TKind extends ImageRetouchKind>(
  kind: TKind,
  value: unknown,
): value is ImageRetouchDefaultsByKind[TKind] {
  const trait = (
    IMAGE_RETOUCH_DEFINITIONS as Record<
      ImageRetouchKind,
      { isWholeParams: (v: unknown) => boolean }
    >
  )[kind]
  if (!trait) return assertNever(kind as never)
  return trait.isWholeParams(value)
}

/**
 * Single-dispatch sanitizing clone for layerStack/commands owners.
 *
 * Replaces the private `cloneRetouchParams` if-chain: layerStack's copy/merge
 * paths can call `cloneRetouchParamsForKind(layer.kind, layer.params)` and
 * stay sanitized (clamped/normalized/endpoint-locked) per kind.
 */
export function cloneRetouchParamsForKind<TKind extends ImageRetouchKind>(
  kind: TKind,
  params: ImageRetouchDefaultsByKind[TKind],
): ImageRetouchDefaultsByKind[TKind] {
  const trait = getImageRetouchTrait(kind) as unknown as {
    clone: (params: unknown) => ImageRetouchDefaultsByKind[TKind]
  }
  return trait.clone(params)
}

export function normalizeImageEffectLayers(
  layers: readonly RetouchEffectLayer[],
): NormalizedImageRetouch[] {
  return layers.map((layer) => normalizeImageEffectLayer(layer))
}

export function normalizeImageEffectLayer(layer: RetouchEffectLayer): NormalizedImageRetouch {
  const definition = IMAGE_RETOUCH_DEFINITIONS[layer.kind] as unknown as
    { normalize: (params: never) => NormalizedImageRetouch['params'] } | undefined
  if (!definition) return assertNever(layer.kind as never)
  const params = definition.normalize((layer as unknown as { params: never }).params)
  return { kind: layer.kind, params } as NormalizedImageRetouch
}

export function isNeutralImageRetouch(retouch: NormalizedImageRetouch): boolean {
  const definition = IMAGE_RETOUCH_DEFINITIONS[retouch.kind] as unknown as
    { isNeutral: (params: never) => boolean } | undefined
  if (!definition) return assertNever(retouch.kind as never)
  return definition.isNeutral((retouch as unknown as { params: never }).params)
}

export function getImageRetouchDefaultParams<TKind extends ImageRetouchKind>(
  kind: TKind,
): ImageRetouchDefaultsByKind[TKind] {
  // Deep copy via the owning trait so callers can never alias (and mutate)
  // the registry defaults — curves included (`channels` gets a fresh object).
  // The indexed-trait call erases per-kind narrowing, so the cast restores the
  // TKind-indexed return (same pattern the first-wave forwarding used).
  return getImageRetouchTrait(kind).cloneDefaults() as ImageRetouchDefaultsByKind[TKind]
}

function assertNever(value: never): never {
  throw new Error(`Unhandled image retouch kind: ${String(value)}`)
}
