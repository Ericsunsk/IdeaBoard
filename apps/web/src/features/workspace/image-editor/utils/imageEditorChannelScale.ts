export const IMAGE_EDITOR_CHANNEL_MIN = 0
export const IMAGE_EDITOR_CHANNEL_MAX = 255
export const IMAGE_EDITOR_PLOT_PERCENT_MIN = 0
export const IMAGE_EDITOR_PLOT_PERCENT_MAX = 100

export function clampImageEditorChannel(value: unknown, fallback = IMAGE_EDITOR_CHANNEL_MIN) {
  return clampImageEditorChannelRange(
    value,
    IMAGE_EDITOR_CHANNEL_MIN,
    IMAGE_EDITOR_CHANNEL_MAX,
    fallback,
  )
}

export function clampImageEditorChannelRange(
  value: unknown,
  min: number,
  max: number,
  fallback: number,
) {
  const resolvedFallback = clampFiniteNumber(fallback, min, max, IMAGE_EDITOR_CHANNEL_MIN)
  return Math.round(clampFiniteNumber(value, min, max, resolvedFallback))
}

export function clampImageEditorPlotPercent(
  value: unknown,
  fallback = IMAGE_EDITOR_PLOT_PERCENT_MIN,
) {
  const resolvedFallback = clampFiniteNumber(
    fallback,
    IMAGE_EDITOR_PLOT_PERCENT_MIN,
    IMAGE_EDITOR_PLOT_PERCENT_MAX,
    IMAGE_EDITOR_PLOT_PERCENT_MIN,
  )
  return clampFiniteNumber(
    value,
    IMAGE_EDITOR_PLOT_PERCENT_MIN,
    IMAGE_EDITOR_PLOT_PERCENT_MAX,
    resolvedFallback,
  )
}

export function clampImageEditorRoundedPlotPercent(
  value: unknown,
  fallback = IMAGE_EDITOR_PLOT_PERCENT_MIN,
) {
  return Math.round(clampImageEditorPlotPercent(value, fallback))
}

export function imageEditorChannelToPlotPercent(
  value: unknown,
  fallback = IMAGE_EDITOR_CHANNEL_MIN,
) {
  return (
    (clampImageEditorChannel(value, fallback) / IMAGE_EDITOR_CHANNEL_MAX) *
    IMAGE_EDITOR_PLOT_PERCENT_MAX
  )
}

export function imageEditorPlotPercentToChannel(
  value: unknown,
  fallbackChannel = IMAGE_EDITOR_CHANNEL_MIN,
) {
  if (!isFiniteNumber(value)) return clampImageEditorChannel(fallbackChannel)
  return clampImageEditorChannel(
    (clampImageEditorPlotPercent(value) / IMAGE_EDITOR_PLOT_PERCENT_MAX) * IMAGE_EDITOR_CHANNEL_MAX,
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
