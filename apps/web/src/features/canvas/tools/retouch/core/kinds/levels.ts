import { clampWithFallback } from '../../../../../../shared/utils/number'
import type { RetouchLevelsParams } from '../model.ts'
import {
  RETOUCH_CHANNEL_MAX,
  RETOUCH_CHANNEL_MIN,
  clampRetouchChannelRange,
  createChannelLut,
} from '../channelScale'
import {
  RETOUCH_LEVELS_HIGHLIGHTS_MARKER_ID,
  RETOUCH_LEVELS_MIDTONES_MARKER_ID,
  RETOUCH_LEVELS_OUTPUT_BLACK_MARKER_ID,
  RETOUCH_LEVELS_OUTPUT_WHITE_MARKER_ID,
  RETOUCH_LEVELS_SHADOWS_MARKER_ID,
} from '../ids.ts'

export {
  RETOUCH_LEVELS_HIGHLIGHTS_MARKER_ID,
  RETOUCH_LEVELS_MIDTONES_MARKER_ID,
  RETOUCH_LEVELS_OUTPUT_BLACK_MARKER_ID,
  RETOUCH_LEVELS_OUTPUT_WHITE_MARKER_ID,
  RETOUCH_LEVELS_SHADOWS_MARKER_ID,
} from '../ids.ts'

const LEVELS_SHADOWS_DEFAULT = 0
const LEVELS_MIDTONES_DEFAULT = 128
const LEVELS_HIGHLIGHTS_DEFAULT = 255
const LEVELS_OUTPUT_BLACK_DEFAULT = 0
const LEVELS_OUTPUT_WHITE_DEFAULT = 255

const RETOUCH_LEVELS_MIN_HANDLE_SPACING = 1
const RETOUCH_LEVELS_OUTPUT_MIN_SPACING = 1

export const RETOUCH_LEVELS_DEFAULT_PARAMS: RetouchLevelsParams = {
  shadows: LEVELS_SHADOWS_DEFAULT,
  midtones: LEVELS_MIDTONES_DEFAULT,
  highlights: LEVELS_HIGHLIGHTS_DEFAULT,
  outputBlack: LEVELS_OUTPUT_BLACK_DEFAULT,
  outputWhite: LEVELS_OUTPUT_WHITE_DEFAULT,
}

export type NormalizedLevelsEffect = {
  blackPoint: number
  whitePoint: number
  gamma: number
  outputBlack: number
  outputWhite: number
}

export type RetouchLevelsInputHandleId =
  | typeof RETOUCH_LEVELS_SHADOWS_MARKER_ID
  | typeof RETOUCH_LEVELS_MIDTONES_MARKER_ID
  | typeof RETOUCH_LEVELS_HIGHLIGHTS_MARKER_ID
export type RetouchLevelsOutputHandleId =
  typeof RETOUCH_LEVELS_OUTPUT_BLACK_MARKER_ID | typeof RETOUCH_LEVELS_OUTPUT_WHITE_MARKER_ID

export type RetouchLevelsMarker = {
  id: RetouchLevelsInputHandleId
  position: number
  labelKey: string
  ariaValueMin: number
  ariaValueMax: number
}

export type RetouchLevelsConstraint = {
  min: number
  max: number
}

export function normalizeLevelsEffect(params: RetouchLevelsParams): NormalizedLevelsEffect {
  const handles = clampLevelsHandles(params)

  return {
    blackPoint: handles.shadows,
    whitePoint: handles.highlights,
    gamma: createLevelsGamma(handles.midtones, handles.shadows, handles.highlights),
    outputBlack: handles.outputBlack,
    outputWhite: handles.outputWhite,
  }
}

export function createLevelsLut(params: NormalizedLevelsEffect) {
  const { blackPoint, whitePoint, gamma, outputBlack, outputWhite } = params
  const range = Math.max(1, whitePoint - blackPoint)
  const outputRange = Math.max(1, outputWhite - outputBlack)

  return createChannelLut((value) => {
    const normalized = clampUnit((value - blackPoint) / range)
    return outputBlack + Math.pow(normalized, gamma) * outputRange
  })
}

export function isNeutralLevelsEffect(params: NormalizedLevelsEffect): boolean {
  return (
    params.blackPoint === RETOUCH_CHANNEL_MIN &&
    params.whitePoint === RETOUCH_CHANNEL_MAX &&
    params.gamma === 1 &&
    params.outputBlack === RETOUCH_CHANNEL_MIN &&
    params.outputWhite === RETOUCH_CHANNEL_MAX
  )
}

export function isLevelsParams(value: unknown): value is RetouchLevelsParams {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return false
  const record = value as Record<string, unknown>
  // `shadows` is exclusive to levels: no other kind carries it, so the guard
  // is order-independent against threshold/sharpen/curves/hsl.
  return (
    'shadows' in record && !('hue' in record) && !('channels' in record) && !('amount' in record)
  )
}

export function cloneLevelsParams(params: RetouchLevelsParams): RetouchLevelsParams {
  // Sanitizing clone: re-clamp handles, matching the previous layerStack
  // clone path (clampLevelsHandles).
  return clampLevelsHandles(params)
}

export function cloneLevelsDefaults(): RetouchLevelsParams {
  return cloneLevelsParams(RETOUCH_LEVELS_DEFAULT_PARAMS)
}

export function clampLevelsHandles(params: RetouchLevelsParams): RetouchLevelsParams {
  const highlights = clampRetouchChannelRange(
    params.highlights,
    RETOUCH_CHANNEL_MIN + RETOUCH_LEVELS_MIN_HANDLE_SPACING,
    RETOUCH_CHANNEL_MAX,
    LEVELS_HIGHLIGHTS_DEFAULT,
  )
  const shadows = clampRetouchChannelRange(
    params.shadows,
    RETOUCH_CHANNEL_MIN,
    highlights - RETOUCH_LEVELS_MIN_HANDLE_SPACING,
    LEVELS_SHADOWS_DEFAULT,
  )
  const midtones = clampMidtonesForHandles(params.midtones, shadows, highlights)
  const outputBlack = clampRetouchChannelRange(
    params.outputBlack,
    RETOUCH_CHANNEL_MIN,
    RETOUCH_CHANNEL_MAX - RETOUCH_LEVELS_OUTPUT_MIN_SPACING,
    LEVELS_OUTPUT_BLACK_DEFAULT,
  )
  const outputWhite = clampRetouchChannelRange(
    params.outputWhite,
    outputBlack + RETOUCH_LEVELS_OUTPUT_MIN_SPACING,
    RETOUCH_CHANNEL_MAX,
    LEVELS_OUTPUT_WHITE_DEFAULT,
  )

  return {
    shadows,
    midtones,
    highlights,
    outputBlack,
    outputWhite,
  }
}

export function createLevelsInputConstraints(
  params: RetouchLevelsParams,
): Record<RetouchLevelsInputHandleId, RetouchLevelsConstraint> {
  const handles = clampLevelsHandles(params)
  return createLevelsInputConstraintsForHandles(handles.shadows, handles.highlights)
}

export function createLevelsOutputConstraints(
  params: RetouchLevelsParams,
): Record<RetouchLevelsOutputHandleId, RetouchLevelsConstraint> {
  const handles = clampLevelsHandles(params)
  return {
    outputBlack: {
      min: RETOUCH_CHANNEL_MIN,
      max: handles.outputWhite - RETOUCH_LEVELS_OUTPUT_MIN_SPACING,
    },
    outputWhite: {
      min: handles.outputBlack + RETOUCH_LEVELS_OUTPUT_MIN_SPACING,
      max: RETOUCH_CHANNEL_MAX,
    },
  }
}

export function createLevelsMarkers(params: RetouchLevelsParams): RetouchLevelsMarker[] {
  const handles = clampLevelsHandles(params)
  const constraints = createLevelsInputConstraintsForHandles(handles.shadows, handles.highlights)
  return [
    {
      id: RETOUCH_LEVELS_SHADOWS_MARKER_ID,
      position: handles.shadows,
      labelKey: 'canvas.retouch.shadowsMarkerAria',
      ariaValueMin: constraints.shadows.min,
      ariaValueMax: constraints.shadows.max,
    },
    {
      id: RETOUCH_LEVELS_MIDTONES_MARKER_ID,
      position: handles.midtones,
      labelKey: 'canvas.retouch.midtonesMarkerAria',
      ariaValueMin: constraints.midtones.min,
      ariaValueMax: constraints.midtones.max,
    },
    {
      id: RETOUCH_LEVELS_HIGHLIGHTS_MARKER_ID,
      position: handles.highlights,
      labelKey: 'canvas.retouch.highlightsMarkerAria',
      ariaValueMin: constraints.highlights.min,
      ariaValueMax: constraints.highlights.max,
    },
  ]
}

export function moveLevelsHandle(
  params: RetouchLevelsParams,
  handleId: RetouchLevelsInputHandleId,
  position: number,
): RetouchLevelsParams {
  return moveConstrainedHandle(params, handleId, position, (handles) =>
    createLevelsInputConstraintsForHandles(handles.shadows, handles.highlights),
  )
}

export function moveLevelsOutputHandle(
  params: RetouchLevelsParams,
  handleId: RetouchLevelsOutputHandleId,
  value: number,
): RetouchLevelsParams {
  return moveConstrainedHandle(params, handleId, value, (handles) =>
    createLevelsOutputConstraints(handles),
  )
}

function moveConstrainedHandle<THandleId extends keyof RetouchLevelsParams>(
  params: RetouchLevelsParams,
  handleId: THandleId,
  value: number,
  resolveConstraints: (handles: RetouchLevelsParams) => Record<THandleId, RetouchLevelsConstraint>,
): RetouchLevelsParams {
  const handles = clampLevelsHandles(params)
  const constraint = resolveConstraints(handles)[handleId]
  return clampLevelsHandles({
    ...handles,
    [handleId]: clampRetouchChannelRange(value, constraint.min, constraint.max, handles[handleId]),
  })
}

function clampMidtonesForHandles(value: unknown, shadows: number, highlights: number) {
  const { min, max } = createMidtonesConstraint(shadows, highlights)
  const fallback = (shadows + highlights) / 2
  return clampRetouchChannelRange(
    value,
    min,
    max,
    Number.isFinite(fallback) ? fallback : LEVELS_MIDTONES_DEFAULT,
  )
}

function createLevelsGamma(midtones: number, shadows: number, highlights: number) {
  const range = highlights - shadows
  if (!Number.isFinite(midtones) || range <= 0) return 1
  if (midtones === Math.round((shadows + highlights) / 2)) return 1
  const normalized = (midtones - shadows) / range
  if (normalized <= 0 || normalized >= 1) return 1
  return Math.log(0.5) / Math.log(normalized)
}

function createLevelsInputConstraintsForHandles(
  shadows: number,
  highlights: number,
): Record<RetouchLevelsInputHandleId, RetouchLevelsConstraint> {
  return {
    shadows: {
      min: RETOUCH_CHANNEL_MIN,
      max: highlights - RETOUCH_LEVELS_MIN_HANDLE_SPACING,
    },
    midtones: createMidtonesConstraint(shadows, highlights),
    highlights: {
      min: shadows + RETOUCH_LEVELS_MIN_HANDLE_SPACING,
      max: RETOUCH_CHANNEL_MAX,
    },
  }
}

function createMidtonesConstraint(shadows: number, highlights: number): RetouchLevelsConstraint {
  if (highlights - shadows > RETOUCH_LEVELS_MIN_HANDLE_SPACING) {
    return {
      min: shadows + RETOUCH_LEVELS_MIN_HANDLE_SPACING,
      max: highlights - RETOUCH_LEVELS_MIN_HANDLE_SPACING,
    }
  }

  return {
    min: shadows,
    max: highlights,
  }
}

function clampUnit(value: number) {
  return clampWithFallback(value, 0, 1, 0)
}
