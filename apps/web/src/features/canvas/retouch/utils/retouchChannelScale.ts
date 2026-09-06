export const RETOUCH_CHANNEL_MIN = 0
export const RETOUCH_CHANNEL_MAX = 255
export const RETOUCH_PLOT_PERCENT_MIN = 0
export const RETOUCH_PLOT_PERCENT_MAX = 100

export function clampRetouchChannel(value: unknown, fallback = RETOUCH_CHANNEL_MIN) {
  return clampRetouchChannelRange(value, RETOUCH_CHANNEL_MIN, RETOUCH_CHANNEL_MAX, fallback)
}

export function clampRetouchChannelRange(
  value: unknown,
  min: number,
  max: number,
  fallback: number,
) {
  const resolvedFallback = clampFiniteNumber(fallback, min, max, RETOUCH_CHANNEL_MIN)
  return Math.round(clampFiniteNumber(value, min, max, resolvedFallback))
}

export function clampRetouchPlotPercent(value: unknown, fallback = RETOUCH_PLOT_PERCENT_MIN) {
  const resolvedFallback = clampFiniteNumber(
    fallback,
    RETOUCH_PLOT_PERCENT_MIN,
    RETOUCH_PLOT_PERCENT_MAX,
    RETOUCH_PLOT_PERCENT_MIN,
  )
  return clampFiniteNumber(
    value,
    RETOUCH_PLOT_PERCENT_MIN,
    RETOUCH_PLOT_PERCENT_MAX,
    resolvedFallback,
  )
}

export function clampRetouchRoundedPlotPercent(
  value: unknown,
  fallback = RETOUCH_PLOT_PERCENT_MIN,
) {
  return Math.round(clampRetouchPlotPercent(value, fallback))
}

export function retouchChannelToPlotPercent(value: unknown, fallback = RETOUCH_CHANNEL_MIN) {
  return (clampRetouchChannel(value, fallback) / RETOUCH_CHANNEL_MAX) * RETOUCH_PLOT_PERCENT_MAX
}

export function retouchPlotPercentToChannel(value: unknown, fallbackChannel = RETOUCH_CHANNEL_MIN) {
  if (!isFiniteNumber(value)) return clampRetouchChannel(fallbackChannel)
  return clampRetouchChannel(
    (clampRetouchPlotPercent(value) / RETOUCH_PLOT_PERCENT_MAX) * RETOUCH_CHANNEL_MAX,
    fallbackChannel,
  )
}

function clampFiniteNumber(value: unknown, min: number, max: number, fallback: number) {
  if (!isFiniteNumber(value)) return fallback
  return Math.min(max, Math.max(min, value))
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}
