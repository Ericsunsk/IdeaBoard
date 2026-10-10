import type { RetouchThresholdParams } from '../model.ts'
import {
  clampRetouchChannel,
  retouchChannelToPlotPercent,
  retouchPlotPercentToChannel,
} from '../channelScale'
import { RETOUCH_THRESHOLD_MARKER_ID } from '../ids.ts'

export { RETOUCH_THRESHOLD_MARKER_ID } from '../ids.ts'

const THRESHOLD_DEFAULT = 128

export const RETOUCH_THRESHOLD_DEFAULT_PARAMS: RetouchThresholdParams = {
  threshold: THRESHOLD_DEFAULT,
  invert: false,
}

export type NormalizedThresholdEffect = {
  threshold: number
  invert: boolean
}

export type RetouchThresholdMarker = {
  id: typeof RETOUCH_THRESHOLD_MARKER_ID
  position: number
  value: number
  labelKey: string
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

export function isNeutralThresholdEffect(_params: NormalizedThresholdEffect): boolean {
  // Threshold has no neutral normalized state: any threshold value visibly
  // posterizes, so the plan compiler must always keep the step.
  return false
}

export function isThresholdParams(value: unknown): value is RetouchThresholdParams {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return false
  const record = value as Record<string, unknown>
  // Requires `invert` alongside `threshold` and excludes sharpen's `amount`:
  // sharpen params also carry a `threshold` key with a different meaning, so
  // the guard is order-independent against sharpen.
  return (
    'threshold' in record &&
    'invert' in record &&
    !('amount' in record) &&
    !('hue' in record) &&
    !('channels' in record) &&
    !('shadows' in record)
  )
}

export function cloneThresholdParams(params: RetouchThresholdParams): RetouchThresholdParams {
  // Sanitizing clone: clamp + normalize invert, matching the previous
  // layerStack clone path (normalizeThresholdEffect).
  return normalizeThresholdEffect(params)
}

export function cloneThresholdDefaults(): RetouchThresholdParams {
  return cloneThresholdParams(RETOUCH_THRESHOLD_DEFAULT_PARAMS)
}

export function createThresholdMarker(
  source: Pick<RetouchThresholdParams, 'threshold'> | NormalizedThresholdEffect | number,
): RetouchThresholdMarker {
  const value = clampThresholdValue(resolveThresholdValue(source))
  return {
    id: RETOUCH_THRESHOLD_MARKER_ID,
    position: retouchChannelToPlotPercent(value),
    value,
    labelKey: 'canvas.retouch.thresholdMarkerAria',
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
