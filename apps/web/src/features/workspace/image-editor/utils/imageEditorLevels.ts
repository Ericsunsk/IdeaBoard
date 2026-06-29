import type { ImageEditorLevelsParams } from '../types'
import {
  IMAGE_EDITOR_CHANNEL_MAX,
  IMAGE_EDITOR_CHANNEL_MIN,
  clampImageEditorChannel,
  clampImageEditorChannelRange,
} from './imageEditorChannelScale'

const LEVELS_SHADOWS_DEFAULT = 0
const LEVELS_MIDTONES_DEFAULT = 128
const LEVELS_HIGHLIGHTS_DEFAULT = 255
const LEVELS_OUTPUT_BLACK_DEFAULT = 0
const LEVELS_OUTPUT_WHITE_DEFAULT = 255

export const IMAGE_EDITOR_LEVELS_MIN_HANDLE_SPACING = 1
export const IMAGE_EDITOR_LEVELS_OUTPUT_MIN_SPACING = 1

export type NormalizedLevelsAdjustment = {
  blackPoint: number
  whitePoint: number
  gamma: number
  outputBlack: number
  outputWhite: number
}

export type ImageEditorLevelsInputHandleId = 'shadows' | 'midtones' | 'highlights'
export type ImageEditorLevelsOutputHandleId = 'outputBlack' | 'outputWhite'

export type ImageEditorLevelsMarker = {
  id: ImageEditorLevelsInputHandleId
  position: number
  ariaLabel: string
  ariaValueMin: number
  ariaValueMax: number
}

export type ImageEditorLevelsConstraint = {
  min: number
  max: number
}

export function normalizeLevelsAdjustment(
  params: ImageEditorLevelsParams,
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

export function clampLevelsHandles(params: ImageEditorLevelsParams): ImageEditorLevelsParams {
  const highlights = clampImageEditorChannelRange(
    params.highlights,
    IMAGE_EDITOR_CHANNEL_MIN + IMAGE_EDITOR_LEVELS_MIN_HANDLE_SPACING,
    IMAGE_EDITOR_CHANNEL_MAX,
    LEVELS_HIGHLIGHTS_DEFAULT,
  )
  const shadows = clampImageEditorChannelRange(
    params.shadows,
    IMAGE_EDITOR_CHANNEL_MIN,
    highlights - IMAGE_EDITOR_LEVELS_MIN_HANDLE_SPACING,
    LEVELS_SHADOWS_DEFAULT,
  )
  const midtones = clampMidtonesForHandles(params.midtones, shadows, highlights)
  const outputBlack = clampImageEditorChannelRange(
    params.outputBlack,
    IMAGE_EDITOR_CHANNEL_MIN,
    IMAGE_EDITOR_CHANNEL_MAX - IMAGE_EDITOR_LEVELS_OUTPUT_MIN_SPACING,
    LEVELS_OUTPUT_BLACK_DEFAULT,
  )
  const outputWhite = clampImageEditorChannelRange(
    params.outputWhite,
    outputBlack + IMAGE_EDITOR_LEVELS_OUTPUT_MIN_SPACING,
    IMAGE_EDITOR_CHANNEL_MAX,
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
  params: ImageEditorLevelsParams,
): Record<ImageEditorLevelsInputHandleId, ImageEditorLevelsConstraint> {
  const handles = clampLevelsHandles(params)
  return createLevelsInputConstraintsForHandles(handles.shadows, handles.highlights)
}

export function createLevelsOutputConstraints(
  params: ImageEditorLevelsParams,
): Record<ImageEditorLevelsOutputHandleId, ImageEditorLevelsConstraint> {
  const handles = clampLevelsHandles(params)
  return {
    outputBlack: {
      min: IMAGE_EDITOR_CHANNEL_MIN,
      max: handles.outputWhite - IMAGE_EDITOR_LEVELS_OUTPUT_MIN_SPACING,
    },
    outputWhite: {
      min: handles.outputBlack + IMAGE_EDITOR_LEVELS_OUTPUT_MIN_SPACING,
      max: IMAGE_EDITOR_CHANNEL_MAX,
    },
  }
}

export function createLevelsMarkers(params: ImageEditorLevelsParams): ImageEditorLevelsMarker[] {
  const handles = clampLevelsHandles(params)
  const constraints = createLevelsInputConstraintsForHandles(handles.shadows, handles.highlights)
  return [
    {
      id: 'shadows',
      position: handles.shadows,
      ariaLabel: `黑场 ${formatMarkerValue(handles.shadows)}`,
      ariaValueMin: constraints.shadows.min,
      ariaValueMax: constraints.shadows.max,
    },
    {
      id: 'midtones',
      position: handles.midtones,
      ariaLabel: `中点 ${formatMarkerValue(handles.midtones)}`,
      ariaValueMin: constraints.midtones.min,
      ariaValueMax: constraints.midtones.max,
    },
    {
      id: 'highlights',
      position: handles.highlights,
      ariaLabel: `白场 ${formatMarkerValue(handles.highlights)}`,
      ariaValueMin: constraints.highlights.min,
      ariaValueMax: constraints.highlights.max,
    },
  ]
}

export function moveLevelsHandle(
  params: ImageEditorLevelsParams,
  handleId: ImageEditorLevelsInputHandleId,
  position: number,
): ImageEditorLevelsParams {
  const handles = clampLevelsHandles(params)
  const constraints = createLevelsInputConstraintsForHandles(handles.shadows, handles.highlights)

  switch (handleId) {
    case 'shadows':
      return clampLevelsHandles({
        ...handles,
        shadows: clampImageEditorChannelRange(
          position,
          constraints.shadows.min,
          constraints.shadows.max,
          handles.shadows,
        ),
      })
    case 'midtones':
      return clampLevelsHandles({
        ...handles,
        midtones: clampImageEditorChannelRange(
          position,
          constraints.midtones.min,
          constraints.midtones.max,
          handles.midtones,
        ),
      })
    case 'highlights':
      return clampLevelsHandles({
        ...handles,
        highlights: clampImageEditorChannelRange(
          position,
          constraints.highlights.min,
          constraints.highlights.max,
          handles.highlights,
        ),
      })
  }
}

export function moveLevelsOutputHandle(
  params: ImageEditorLevelsParams,
  handleId: ImageEditorLevelsOutputHandleId,
  value: number,
): ImageEditorLevelsParams {
  const handles = clampLevelsHandles(params)
  const constraints = createLevelsOutputConstraints(handles)

  switch (handleId) {
    case 'outputBlack':
      return clampLevelsHandles({
        ...handles,
        outputBlack: clampImageEditorChannelRange(
          value,
          constraints.outputBlack.min,
          constraints.outputBlack.max,
          handles.outputBlack,
        ),
      })
    case 'outputWhite':
      return clampLevelsHandles({
        ...handles,
        outputWhite: clampImageEditorChannelRange(
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
  const fallback = clampImageEditorChannelRange(
    (shadows + highlights) / 2,
    min,
    max,
    LEVELS_MIDTONES_DEFAULT,
  )
  return clampImageEditorChannelRange(value, min, max, fallback)
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
): Record<ImageEditorLevelsInputHandleId, ImageEditorLevelsConstraint> {
  return {
    shadows: {
      min: IMAGE_EDITOR_CHANNEL_MIN,
      max: highlights - IMAGE_EDITOR_LEVELS_MIN_HANDLE_SPACING,
    },
    midtones: createMidtonesConstraint(shadows, highlights),
    highlights: {
      min: shadows + IMAGE_EDITOR_LEVELS_MIN_HANDLE_SPACING,
      max: IMAGE_EDITOR_CHANNEL_MAX,
    },
  }
}

function createMidtonesConstraint(
  shadows: number,
  highlights: number,
): ImageEditorLevelsConstraint {
  if (highlights - shadows > IMAGE_EDITOR_LEVELS_MIN_HANDLE_SPACING) {
    return {
      min: shadows + IMAGE_EDITOR_LEVELS_MIN_HANDLE_SPACING,
      max: highlights - IMAGE_EDITOR_LEVELS_MIN_HANDLE_SPACING,
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
  for (let index = IMAGE_EDITOR_CHANNEL_MIN; index <= IMAGE_EDITOR_CHANNEL_MAX; index += 1) {
    lut[index] = clampImageEditorChannel(resolve(index))
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
