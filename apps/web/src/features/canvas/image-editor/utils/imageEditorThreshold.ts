import type { ImageEditorThresholdParams } from '../types'
import {
  clampImageEditorChannel,
  imageEditorChannelToPlotPercent,
  imageEditorPlotPercentToChannel,
} from './imageEditorChannelScale'

const THRESHOLD_DEFAULT = 128

export const IMAGE_EDITOR_THRESHOLD_MARKER_ID = 'threshold'

export type NormalizedThresholdAdjustment = {
  threshold: number
  invert: boolean
}

export type ImageEditorThresholdMarker = {
  id: typeof IMAGE_EDITOR_THRESHOLD_MARKER_ID
  position: number
  value: number
  ariaLabel: string
}

export function normalizeThresholdAdjustment(
  params: ImageEditorThresholdParams,
): NormalizedThresholdAdjustment {
  return {
    threshold: clampThresholdValue(params.threshold),
    invert: params.invert === true,
  }
}

export function clampThresholdValue(value: unknown) {
  return clampImageEditorChannel(value, THRESHOLD_DEFAULT)
}

export function createThresholdMarker(
  source: Pick<ImageEditorThresholdParams, 'threshold'> | NormalizedThresholdAdjustment | number,
): ImageEditorThresholdMarker {
  const value = clampThresholdValue(resolveThresholdValue(source))
  return {
    id: IMAGE_EDITOR_THRESHOLD_MARKER_ID,
    position: imageEditorChannelToPlotPercent(value),
    value,
    ariaLabel: `阈值 ${value}`,
  }
}

export function moveThresholdHandle(
  params: ImageEditorThresholdParams,
  position: number,
): ImageEditorThresholdParams {
  return {
    ...params,
    threshold: imageEditorPlotPercentToChannel(position, THRESHOLD_DEFAULT),
    invert: params.invert === true,
  }
}

function resolveThresholdValue(
  source: Pick<ImageEditorThresholdParams, 'threshold'> | NormalizedThresholdAdjustment | number,
) {
  return typeof source === 'number' ? source : source.threshold
}
