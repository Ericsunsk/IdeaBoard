import { i18n } from '../../../../shared/i18n'
import type { RetouchThresholdParams } from '../types'
import {
  clampRetouchChannel,
  retouchChannelToPlotPercent,
  retouchPlotPercentToChannel,
} from './channelScale'

const THRESHOLD_DEFAULT = 128

export const RETOUCH_THRESHOLD_MARKER_ID = 'threshold'

export type NormalizedThresholdEffect = {
  threshold: number
  invert: boolean
}

export type RetouchThresholdMarker = {
  id: typeof RETOUCH_THRESHOLD_MARKER_ID
  position: number
  value: number
  ariaLabel: string
}

export function normalizeThresholdEffect(
  params: RetouchThresholdParams,
): NormalizedThresholdEffect {
  return {
    threshold: clampThresholdValue(params.threshold),
    invert: params.invert === true,
  }
}

export function clampThresholdValue(value: unknown) {
  return clampRetouchChannel(value, THRESHOLD_DEFAULT)
}

export function createThresholdMarker(
  source: Pick<RetouchThresholdParams, 'threshold'> | NormalizedThresholdEffect | number,
): RetouchThresholdMarker {
  const value = clampThresholdValue(resolveThresholdValue(source))
  return {
    id: RETOUCH_THRESHOLD_MARKER_ID,
    position: retouchChannelToPlotPercent(value),
    value,
    ariaLabel: i18n.global.t('canvas.retouch.thresholdMarkerAria', { value }),
  }
}

export function moveThresholdHandle(
  params: RetouchThresholdParams,
  position: number,
): RetouchThresholdParams {
  return {
    ...params,
    threshold: retouchPlotPercentToChannel(position, THRESHOLD_DEFAULT),
    invert: params.invert === true,
  }
}

function resolveThresholdValue(
  source: Pick<RetouchThresholdParams, 'threshold'> | NormalizedThresholdEffect | number,
) {
  return typeof source === 'number' ? source : source.threshold
}
