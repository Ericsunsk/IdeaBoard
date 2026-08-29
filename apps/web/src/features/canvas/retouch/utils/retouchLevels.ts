import { i18n } from '../../../../shared/i18n'
import type { RetouchLevelsParams } from '../types'
import {
  RETOUCH_CHANNEL_MAX,
  RETOUCH_CHANNEL_MIN,
  clampRetouchChannel,
  clampRetouchChannelRange,
} from './retouchChannelScale'

const LEVELS_SHADOWS_DEFAULT = 0
const LEVELS_MIDTONES_DEFAULT = 128
const LEVELS_HIGHLIGHTS_DEFAULT = 255
const LEVELS_OUTPUT_BLACK_DEFAULT = 0
const LEVELS_OUTPUT_WHITE_DEFAULT = 255

export const RETOUCH_LEVELS_MIN_HANDLE_SPACING = 1
export const RETOUCH_LEVELS_OUTPUT_MIN_SPACING = 1

export type NormalizedLevelsAdjustment = {
  blackPoint: number
  whitePoint: number
  gamma: number
  outputBlack: number
  outputWhite: number
}

export type RetouchLevelsInputHandleId = 'shadows' | 'midtones' | 'highlights'
export type RetouchLevelsOutputHandleId = 'outputBlack' | 'outputWhite'

export type RetouchLevelsMarker = {
  id: RetouchLevelsInputHandleId
  position: number
  ariaLabel: string
  ariaValueMin: number
  ariaValueMax: number
}

export type RetouchLevelsConstraint = {
  min: number
  max: number
}

export function normalizeLevelsAdjustment(
  params: RetouchLevelsParams,
): NormalizedLevelsAdjustment {
  const handles = clampLevelsHandles(params)

  return {
    blackPoint: handles.shadows,
    whitePoint: handles.highlights,
    gamma: createLevelsGamma(handles.midtones, handles.shadows, handles.highlights),
    outputBlack: handles.outputBlack,
    outputWhite: handles.outputWhite,
  }
}

export function createLevelsLut(params: NormalizedLevelsAdjustment) {
  const { blackPoint, whitePoint, gamma, outputBlack, outputWhite } = params
  const range = Math.max(1, whitePoint - blackPoint)
  const outputRange = Math.max(1, outputWhite - outputBlack)

  return createLut((value) => {
    const normalized = clampUnit((value - blackPoint) / range)
    return outputBlack + Math.pow(normalized, gamma) * outputRange
  })
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
      id: 'shadows',
      position: handles.shadows,
      ariaLabel: i18n.global.t('canvas.retouch.shadowsMarkerAria', {
        value: formatMarkerValue(handles.shadows),
      }),
      ariaValueMin: constraints.shadows.min,
      ariaValueMax: constraints.shadows.max,
    },
    {
      id: 'midtones',
      position: handles.midtones,
      ariaLabel: i18n.global.t('canvas.retouch.midtonesMarkerAria', {
        value: formatMarkerValue(handles.midtones),
      }),
      ariaValueMin: constraints.midtones.min,
      ariaValueMax: constraints.midtones.max,
    },
    {
      id: 'highlights',
      position: handles.highlights,
      ariaLabel: i18n.global.t('canvas.retouch.highlightsMarkerAria', {
        value: formatMarkerValue(handles.highlights),
      }),
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
  const handles = clampLevelsHandles(params)
  const constraints = createLevelsInputConstraintsForHandles(handles.shadows, handles.highlights)

  switch (handleId) {
    case 'shadows':
      return clampLevelsHandles({
        ...handles,
        shadows: clampRetouchChannelRange(
          position,
          constraints.shadows.min,
          constraints.shadows.max,
          handles.shadows,
        ),
      })
    case 'midtones':
      return clampLevelsHandles({
        ...handles,
        midtones: clampRetouchChannelRange(
          position,
          constraints.midtones.min,
          constraints.midtones.max,
          handles.midtones,
        ),
      })
    case 'highlights':
      return clampLevelsHandles({
        ...handles,
        highlights: clampRetouchChannelRange(
          position,
          constraints.highlights.min,
          constraints.highlights.max,
          handles.highlights,
        ),
      })
  }
}

export function moveLevelsOutputHandle(
  params: RetouchLevelsParams,
  handleId: RetouchLevelsOutputHandleId,
  value: number,
): RetouchLevelsParams {
  const handles = clampLevelsHandles(params)
  const constraints = createLevelsOutputConstraints(handles)

  switch (handleId) {
    case 'outputBlack':
      return clampLevelsHandles({
        ...handles,
        outputBlack: clampRetouchChannelRange(
          value,
          constraints.outputBlack.min,
          constraints.outputBlack.max,
          handles.outputBlack,
        ),
      })
    case 'outputWhite':
      return clampLevelsHandles({
        ...handles,
        outputWhite: clampRetouchChannelRange(
          value,
          constraints.outputWhite.min,
          constraints.outputWhite.max,
          handles.outputWhite,
        ),
      })
  }
}

function clampMidtonesForHandles(value: unknown, shadows: number, highlights: number) {
  const { min, max } = createMidtonesConstraint(shadows, highlights)
  const fallback = clampRetouchChannelRange(
    (shadows + highlights) / 2,
    min,
    max,
    LEVELS_MIDTONES_DEFAULT,
  )
  return clampRetouchChannelRange(value, min, max, fallback)
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

function createMidtonesConstraint(
  shadows: number,
  highlights: number,
): RetouchLevelsConstraint {
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
  return clampFloat(value, 0, 1, 0)
}

function createLut(resolve: (value: number) => number) {
  const lut = new Uint8ClampedArray(256)
  for (let index = RETOUCH_CHANNEL_MIN; index <= RETOUCH_CHANNEL_MAX; index += 1) {
    lut[index] = clampRetouchChannel(resolve(index))
  }
  return lut
}

function clampFloat(value: unknown, min: number, max: number, fallback: number) {
  if (typeof value !== 'number' || !Number.isFinite(value)) return fallback
  return Math.min(max, Math.max(min, value))
}

function formatMarkerValue(value: number) {
  return String(value)
}
