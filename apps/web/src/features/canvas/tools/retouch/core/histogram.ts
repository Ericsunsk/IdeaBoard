import type { RetouchCurveChannel } from './model.ts'
import { RETOUCH_CHANNEL_MAX, RETOUCH_PLOT_PERCENT_MAX, formatPathNumber } from './channelScale'

export type RetouchHistogram = {
  channel: RetouchCurveChannel
  bins: Uint32Array
  normalizedBins: Float32Array
  maxBin: number
  total: number
}

export function createRetouchHistogram(
  imageData: ImageData | null | undefined,
  channel: RetouchCurveChannel,
): RetouchHistogram {
  const bins = new Uint32Array(RETOUCH_CHANNEL_MAX + 1)
  const data = imageData?.data
  if (!data || data.length < 4) return createHistogramResult(channel, bins)

  for (let index = 0; index < data.length; index += 4) {
    const value = resolveHistogramValue(data, index, channel)
    bins[value] = (bins[value] ?? 0) + 1
  }

  return createHistogramResult(channel, bins)
}

export function sampleHistogramPath(histogram: RetouchHistogram, sampleCount = 128) {
  if (histogram.maxBin <= 0) return ''

  const count = Math.max(2, Math.round(sampleCount))
  const points: string[] = [`M 0 ${RETOUCH_PLOT_PERCENT_MAX}`]
  for (let index = 0; index < count; index += 1) {
    const binIndex = Math.round((index / (count - 1)) * RETOUCH_CHANNEL_MAX)
    const x = (binIndex / RETOUCH_CHANNEL_MAX) * RETOUCH_PLOT_PERCENT_MAX
    const y =
      RETOUCH_PLOT_PERCENT_MAX -
      (histogram.normalizedBins[binIndex] ?? 0) * RETOUCH_PLOT_PERCENT_MAX
    points.push(`L ${formatPathNumber(x)} ${formatPathNumber(y)}`)
  }
  points.push(`L ${RETOUCH_PLOT_PERCENT_MAX} ${RETOUCH_PLOT_PERCENT_MAX} Z`)
  return points.join(' ')
}

function createHistogramResult(channel: RetouchCurveChannel, bins: Uint32Array): RetouchHistogram {
  const normalizedBins = new Float32Array(RETOUCH_CHANNEL_MAX + 1)
  let maxBin = 0
  let total = 0
  for (const value of bins) {
    if (value > maxBin) maxBin = value
    total += value
  }

  if (maxBin > 0) {
    for (let index = 0; index < bins.length; index += 1) {
      normalizedBins[index] = Math.sqrt((bins[index] ?? 0) / maxBin)
    }
  }

  return {
    channel,
    bins,
    normalizedBins,
    maxBin,
    total,
  }
}

const HISTOGRAM_CHANNEL_OFFSETS: Record<Exclude<RetouchCurveChannel, 'rgb'>, number> = {
  red: 0,
  green: 1,
  blue: 2,
}

function resolveHistogramValue(
  data: Uint8ClampedArray,
  index: number,
  channel: RetouchCurveChannel,
) {
  if (channel === 'rgb') {
    return Math.round(
      0.2126 * (data[index] ?? 0) +
        0.7152 * (data[index + 1] ?? 0) +
        0.0722 * (data[index + 2] ?? 0),
    )
  }
  return data[index + HISTOGRAM_CHANNEL_OFFSETS[channel]] ?? 0
}
